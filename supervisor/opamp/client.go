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
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/open-telemetry/opamp-go/client"
	"github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"

	integrationspec "github.com/thinre/thinre/integration-spec"
	"github.com/thinre/thinre/protocol"
	"github.com/thinre/thinre/supervisor/hooks"
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
	Manifest        *integrationspec.Integration
}

// Run connects and blocks until ctx is canceled. The desired-state
// documents received here are logged only; reconciliation plugs into
// OnMessage with milestone M3.
func Run(ctx context.Context, p Params) error {
	runtimeUID, err := uuid.Parse(p.RuntimeID)
	if err != nil {
		return fmt.Errorf("runtime id is not a UUID: %w", err)
	}

	c := client.NewWebSocket(&slogAdapter{log: p.Log})

	if err := c.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{
			strAttr("service.name", "thinre-supervisor"),
			strAttr("service.instance.id", p.RuntimeID),
		},
		NonIdentifyingAttributes: []*protobufs.KeyValue{
			strAttr("thinre.integration", p.Manifest.Metadata.Name),
			strAttr("thinre.supervisor.version", p.SupervisorVersion),
		},
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

	report := func(reason string) {
		state := observe(ctx, p.Manifest)
		data, err := json.Marshal(state)
		if err != nil {
			p.Log.Error("marshal observed state", "err", err)
			return
		}
		if _, err := c.SendCustomMessage(&protobufs.CustomMessage{
			Capability: protocol.CustomCapability,
			Type:       protocol.ObservedStateMessageType,
			Data:       data,
		}); err != nil {
			// ErrCustomMessagePending: the previous report is still in
			// flight; the next tick will carry the fresh state.
			p.Log.Debug("observed-state report skipped", "reason", reason, "err", err)
			return
		}
		_ = c.SetHealth(&protobufs.ComponentHealth{Healthy: state.Health == protocol.HealthHealthy})
		p.Log.Info("observed state reported", "reason", reason, "version", state.Version, "health", state.Health)
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
				// Reconciliation (milestone M3) consumes this document;
				// for now receipt is logged and acknowledged.
				body := msg.RemoteConfig.GetConfig().GetConfigMap()[protocol.RemoteConfigKey].GetBody()
				p.Log.Info("desired state received", "bytes", len(body))
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

// observe builds the current observed state from the integration's version
// and health hooks. Hook failures degrade the report rather than aborting
// it: an unreachable version hook is itself a signal the cloud should see.
func observe(ctx context.Context, manifest *integrationspec.Integration) protocol.ObservedState {
	state := protocol.ObservedState{
		SchemaVersion: protocol.SchemaVersion,
		Health:        protocol.HealthUnknown,
		Status:        protocol.StatusIdle,
	}
	if manifest.Package.Version != nil {
		if out, err := hooks.Run(ctx, manifest.Package.Version); err == nil {
			state.Version = out
		} else {
			state.Message = "version hook: " + err.Error()
		}
	}
	if _, err := hooks.Run(ctx, manifest.Health.Check); err == nil {
		state.Health = protocol.HealthHealthy
	} else {
		state.Health = protocol.HealthUnhealthy
	}
	return state
}

// strAttr builds an OpAMP string attribute.
func strAttr(key, value string) *protobufs.KeyValue {
	return &protobufs.KeyValue{
		Key:   key,
		Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: value}},
	}
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
