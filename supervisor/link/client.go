// Package link maintains the Supervisor's connection to the Thinre Cloud
// gateway over the Link protocol (protocol/link.go): one outbound
// WebSocket per managed application, a hello on every connection,
// desired-state documents in, observed-state documents out. Reconnection
// with jittered exponential backoff is owned here.
package link

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/coder/websocket"

	integrationspec "github.com/thinre/thinre/integration-spec"
	"github.com/thinre/thinre/protocol"
	"github.com/thinre/thinre/supervisor"
	"github.com/thinre/thinre/supervisor/reconcile"
)

// reportInterval paces periodic observed-state reports; each one also
// refreshes the runtime's last-seen timestamp in the cloud and doubles as
// application-level liveness.
const reportInterval = 30 * time.Second

// maxBackoff caps the reconnect delay.
const maxBackoff = 60 * time.Second

// writeTimeout bounds any single WebSocket write.
const writeTimeout = 10 * time.Second

// Params carries everything the connection needs.
type Params struct {
	Log               *slog.Logger
	LinkURL           string
	MachineToken      string
	SupervisorVersion string
	// Labels are the operator-defined tags from the configuration,
	// reported in hello.
	Labels   map[string]string
	Manifest *integrationspec.Integration
	Layout   supervisor.Layout
}

// Run connects and blocks until ctx is canceled, reconnecting with
// jittered exponential backoff. Received desired-state documents are
// handed to the reconciler through a latest-wins queue: while an upgrade
// runs, newer documents replace any queued one, so the reconciler always
// converges on the most recent desired state.
func Run(ctx context.Context, p Params) error {
	c := &client{p: p}
	c.rec = reconcile.New(p.Log, p.Manifest, p.Layout, reconcile.Report(c.send))

	// Latest-wins queue: a buffered channel of one where a newer document
	// evicts an unconsumed older one.
	docCh := make(chan protocol.DesiredState, 1)
	c.submit = func(doc protocol.DesiredState) {
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
				c.rec.Apply(ctx, doc)
			}
		}
	}()

	// Periodic reporting is independent of connection state: while
	// disconnected the report is dropped and the next one after reconnect
	// carries the current truth.
	go func() {
		ticker := time.NewTicker(reportInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.send(ctx, c.rec.Observe(ctx))
				c.ping(ctx)
			}
		}
	}()

	backoff := time.Second
	for {
		started := time.Now()
		err := c.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		// A session that held for a while means the problem is fresh —
		// start the backoff over instead of compounding old failures.
		if time.Since(started) > maxBackoff {
			backoff = time.Second
		}
		p.Log.Warn("link disconnected; will retry", "err", err, "backoff", backoff.Round(time.Second))
		// Full jitter spreads reconnect storms after a gateway restart.
		delay := time.Duration(rand.Int63n(int64(backoff)))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

type client struct {
	p      Params
	rec    *reconcile.Reconciler
	submit func(protocol.DesiredState)

	mu   sync.Mutex
	conn *websocket.Conn
}

// session runs one connection lifetime: dial, hello, then read until the
// connection dies.
func (c *client) session(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	conn, _, err := websocket.Dial(dialCtx, c.p.LinkURL, &websocket.DialOptions{
		HTTPHeader: http.Header{protocol.MachineTokenHeader: []string{c.p.MachineToken}},
	})
	cancel()
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	hello := protocol.LinkEnvelope{Type: protocol.LinkTypeHello, Hello: &protocol.LinkHello{
		LinkVersion:       protocol.LinkVersion,
		SupervisorVersion: c.p.SupervisorVersion,
		Integration:       c.p.Manifest.Metadata.Name,
		Hostname:          hostname(),
		IP:                localIP(),
		OS:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		Labels:            c.p.Labels,
		AppliedGeneration: c.rec.Observe(ctx).Generation,
	}}
	if err := c.write(ctx, hello); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	c.p.Log.Info("connected to gateway", "url", c.p.LinkURL)
	c.send(ctx, c.rec.Observe(ctx))

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var env protocol.LinkEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			c.p.Log.Warn("malformed link message", "err", err)
			continue
		}
		switch env.Type {
		case protocol.LinkTypeState:
			if env.State == nil {
				continue
			}
			c.p.Log.Info("desired state received", "generation", env.State.Generation)
			c.submit(*env.State)
		default:
			// Unknown types are how the protocol grows: ignore them.
		}
	}
}

// send delivers one observed-state document; while disconnected it is
// dropped — the periodic report after reconnect carries the current truth.
func (c *client) send(ctx context.Context, st protocol.ObservedState) {
	env := protocol.LinkEnvelope{Type: protocol.LinkTypeObserved, Observed: &st}
	if err := c.write(ctx, env); err != nil {
		c.p.Log.Warn("observed-state report dropped", "err", err, "status", st.Status)
		return
	}
	c.p.Log.Info("observed state reported", "version", st.Version, "health", st.Health, "status", st.Status)
}

// ping proves the connection is alive between reports; a failed ping
// closes the connection so the session loop reconnects promptly instead
// of waiting for a dead TCP peer to time out.
func (c *client) ping(ctx context.Context) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	pingCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := conn.Ping(pingCtx); err != nil {
		_ = conn.Close(websocket.StatusGoingAway, "ping failed")
	}
}

func (c *client) write(ctx context.Context, env protocol.LinkEnvelope) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, data)
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
	defer func() { _ = conn.Close() }()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	return addr.IP.String()
}
