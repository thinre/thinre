// Package opamp maintains the Supervisor's connection to the Thinre Cloud
// OpAMP gateway: it authenticates with the machine token, reports the agent
// description and observed state, and receives desired-state documents.
// Reconnection with backoff is handled by the opamp-go client itself.
package opamp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"

	integrationspec "github.com/thinre/thinre/integration-spec"
	"github.com/thinre/thinre/protocol"
	"github.com/thinre/thinre/supervisor"
	"github.com/thinre/thinre/supervisor/reconcile"
)

// reportInterval paces periodic observed-state reports; each one also
// refreshes the runtime's last-seen timestamp in the cloud.
const reportInterval = 30 * time.Second

// Params carries everything the connection needs.
type Params struct {
	Log             *slog.Logger
	OpAMPURL        string
	MachineToken    string
	RuntimeID       string
	SupervisorVersion string
	// Labels are the operator-defined tags from the configuration,
	// reported as thinre.label.* attributes.
	Labels   map[string]string
	Manifest *integrationspec.Integration
	Layout          supervisor.Layout
}

// Run connects and blocks until ctx is canceled. Received desired-state
// documents are handed to the reconciler through a latest-wins queue:
// while an upgrade runs, newer documents replace any queued one, so the
// reconciler always converges on the most recent desired state.
func Run(ctx context.Context, p Params) error {
	runtimeUID, err := uuid.Parse(p.RuntimeID)
	if err != nil {
		return fmt.Errorf("runtime id is not a UUID: %w", err)
	}

	c := client.NewWebSocket(&slogAdapter{log: p.Log})

	attrs := []*protobufs.KeyValue{
		strAttr("thinre.integration", p.Manifest.Metadata.Name),
		strAttr("thinre.supervisor.version", p.SupervisorVersion),
		// Host identification (OTel semantic-convention names) so the
		// cloud can show which machine a runtime actually is.
		strAttr("host.name", hostname()),
		strAttr("os.type", runtime.GOOS),
		strAttr("host.arch", runtime.GOARCH),
		strAttr("host.ip", localIP()),
	}
	// Operator-defined tags travel namespaced so the cloud can separate
	// them from the well-known attributes above.
	for key, value := range p.Labels {
		attrs = append(attrs, strAttr("thinre.label."+key, value))
	}

	if err := c.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{
			strAttr("service.name", "thinre-supervisor"),
			strAttr("service.instance.id", p.RuntimeID),
		},
		NonIdentifyingAttributes: attrs,
	}); err != nil {
		return err
	}
	if err := c.SetCustomCapabilities(&protobufs.CustomCapabilities{
		Capabilities: []string{protocol.CustomCapability},
	}); err != nil {
		return err
	}
	// Health must be set BEFORE declaring the ReportsHealth capability:
	// SetCapabilities validates immediately and rejects a nil health.
	// The value is refreshed with every observed-state report.
	if err := c.SetHealth(&protobufs.ComponentHealth{Healthy: false, LastError: "starting"}); err != nil {
		return err
	}
	caps := protobufs.AgentCapabilities_AgentCapabilities_ReportsStatus |
		protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig |
		protobufs.AgentCapabilities_AgentCapabilities_ReportsRemoteConfig |
		protobufs.AgentCapabilities_AgentCapabilities_ReportsHealth
	if err := c.SetCapabilities(&caps); err != nil {
		return err
	}

	// send delivers one observed-state document, retrying briefly when a
	// previous custom message is still in flight — phase reports during an
	// upgrade must not be silently lost.
	send := func(ctx context.Context, st protocol.ObservedState) {
		data, err := json.Marshal(st)
		if err != nil {
			p.Log.Error("marshal observed state", "err", err)
			return
		}
		for attempt := 0; attempt < 5; attempt++ {
			_, err = c.SendCustomMessage(&protobufs.CustomMessage{
				Capability: protocol.CustomCapability,
				Type:       protocol.ObservedStateMessageType,
				Data:       data,
			})
			if err == nil {
				_ = c.SetHealth(&protobufs.ComponentHealth{Healthy: st.Health == protocol.HealthHealthy})
				p.Log.Info("observed state reported", "version", st.Version, "health", st.Health, "status", st.Status)
				return
			}
			select {
			case <-time.After(300 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}
		p.Log.Warn("observed-state report dropped", "err", err, "status", st.Status)
	}

	rec := reconcile.New(p.Log, p.Manifest, p.Layout, reconcile.Report(send))

	// Latest-wins queue: a buffered channel of one where a newer document
	// evicts an unconsumed older one.
	docCh := make(chan protocol.DesiredState, 1)
	submit := func(doc protocol.DesiredState) {
		for {
			select {
			case docCh <- doc:
				return
			default:
				select {
				case <-docCh: // evict the stale document
				default:
				}
			}
		}
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case doc := <-docCh:
				rec.Apply(ctx, doc)
			}
		}
	}()

	report := func(reason string) {
		send(ctx, rec.Observe(ctx))
		_ = reason
	}

	settings := types.StartSettings{
		OpAMPServerURL: p.OpAMPURL,
		Header:         http.Header{protocol.MachineTokenHeader: []string{p.MachineToken}},
		InstanceUid:    types.InstanceUid(runtimeUID),
		Callbacks: types.Callbacks{
			OnConnect: func(ctx context.Context) {
				p.Log.Info("connected to gateway", "url", p.OpAMPURL)
				report("connected")
			},
			OnConnectFailed: func(_ context.Context, err error) {
				p.Log.Warn("connection failed; will retry", "err", err)
			},
			OnError: func(_ context.Context, resp *protobufs.ServerErrorResponse) {
				p.Log.Error("server error", "message", resp.GetErrorMessage())
			},
			OnMessage: func(_ context.Context, msg *types.MessageData) {
				if msg.RemoteConfig == nil {
					return
				}
				body := msg.RemoteConfig.GetConfig().GetConfigMap()[protocol.RemoteConfigKey].GetBody()
				var doc protocol.DesiredState
				if err := json.Unmarshal(body, &doc); err != nil {
					p.Log.Error("malformed desired state", "err", err)
					_ = c.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
						LastRemoteConfigHash: msg.RemoteConfig.GetConfigHash(),
						Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
						ErrorMessage:         "malformed desired-state document",
					})
					return
				}
				p.Log.Info("desired state received", "generation", doc.Generation)
				submit(doc)
				// APPLIED here means "accepted for reconciliation"; the
				// truthful application outcome travels in observed-state
				// reports, which carry generation, phase, and health.
				_ = c.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
					LastRemoteConfigHash: msg.RemoteConfig.GetConfigHash(),
					Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
				})
			},
		},
	}

	if err := c.Start(ctx, settings); err != nil {
		return fmt.Errorf("start opamp client: %w", err)
	}

	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			report("periodic")
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return c.Stop(stopCtx)
		}
	}
}

// strAttr builds an OpAMP string attribute.
func strAttr(key, value string) *protobufs.KeyValue {
	return &protobufs.KeyValue{
		Key:   key,
		Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: value}},
	}
}

// hostname is best-effort: identification must never block a connection.
func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

// localIP returns the machine's primary outbound IPv4/IPv6 address,
// best-effort. The UDP "connection" never sends a packet — it only asks
// the kernel which source address the default route would use, which
// beats scanning interfaces on multi-homed hosts.
func localIP() string {
	conn, err := net.Dial("udp", "203.0.113.1:9") // TEST-NET-3, never routed
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	return addr.IP.String()
}

// slogAdapter bridges opamp-go's logger interface onto slog.
type slogAdapter struct {
	log *slog.Logger
}

func (a *slogAdapter) Debugf(_ context.Context, format string, v ...any) {
	a.log.Debug(fmt.Sprintf(format, v...))
}

func (a *slogAdapter) Errorf(_ context.Context, format string, v ...any) {
	a.log.Error(fmt.Sprintf(format, v...))
}
