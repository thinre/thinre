package link

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	integrationspec "github.com/thinre/thinre/integration-spec"
	"github.com/thinre/thinre/protocol"
	"github.com/thinre/thinre/supervisor"
)

// fixture builds a miniature managed app (same pattern as the reconcile
// tests): VERSION file plus version/upgrade/health hooks operating on it.
// The upgrade hook writes the artifact's content into VERSION.
func fixture(t *testing.T) (*integrationspec.Integration, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("link client tests need a POSIX shell (Windows is covered by the native smoke)")
	}
	app := t.TempDir()
	if err := os.WriteFile(filepath.Join(app, "VERSION"), []byte("1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) string {
		path := filepath.Join(app, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	version := write("version.sh", `cat "`+app+`/VERSION"`)
	upgrade := write("upgrade.sh", `cp "$1" "`+app+`/VERSION"`)
	health := write("health.sh", `exit 0`)

	manifest := &integrationspec.Integration{
		APIVersion: "thinre.io/v1",
		Kind:       "Integration",
		Metadata:   integrationspec.Metadata{Name: "linktest"},
		Package: integrationspec.Package{
			Upgrade: &integrationspec.Hook{Executable: upgrade, Args: []string{"{{ artifact.path }}"}},
			Version: &integrationspec.Hook{Executable: version},
		},
		Health: integrationspec.Health{Check: &integrationspec.Hook{Executable: health}},
	}
	return manifest, app
}

// gateway is a scripted Link server: it records hellos and observed
// reports, and lets the test push state envelopes.
type gateway struct {
	t *testing.T

	mu       sync.Mutex
	conns    []*websocket.Conn
	hellos   []protocol.LinkHello
	observed []protocol.ObservedState
	tokens   []string
}

func (g *gateway) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.tokens = append(g.tokens, r.Header.Get(protocol.MachineTokenHeader))
		g.mu.Unlock()
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		g.mu.Lock()
		g.conns = append(g.conns, conn)
		g.mu.Unlock()
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var env protocol.LinkEnvelope
			if err := json.Unmarshal(data, &env); err != nil {
				g.t.Errorf("malformed message from client: %v", err)
				continue
			}
			g.mu.Lock()
			switch env.Type {
			case protocol.LinkTypeHello:
				g.hellos = append(g.hellos, *env.Hello)
			case protocol.LinkTypeObserved:
				g.observed = append(g.observed, *env.Observed)
			}
			g.mu.Unlock()
		}
	})
}

func (g *gateway) pushState(doc protocol.DesiredState) {
	g.mu.Lock()
	conn := g.conns[len(g.conns)-1]
	g.mu.Unlock()
	env := protocol.LinkEnvelope{Type: protocol.LinkTypeState, State: &doc}
	data, _ := json.Marshal(env)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		g.t.Errorf("push state: %v", err)
	}
}

func (g *gateway) snapshot() (hellos []protocol.LinkHello, observed []protocol.ObservedState, conns int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]protocol.LinkHello{}, g.hellos...), append([]protocol.ObservedState{}, g.observed...), len(g.conns)
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ensuredLayout mirrors what cmd/supervisor does at startup: the layout's
// directories exist before the client runs.
func ensuredLayout(t *testing.T) supervisor.Layout {
	t.Helper()
	layout := supervisor.NewLayout(t.TempDir())
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return layout
}

func waitFor(t *testing.T, desc string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", desc)
}

func TestClientHelloStateObserved(t *testing.T) {
	manifest, app := fixture(t)
	g := &gateway{t: t}
	srv := httptest.NewServer(g.handler())
	defer srv.Close()

	// The artifact "content is the version" — serve it over HTTP for the
	// downloader.
	artifact := []byte("2.0.0\n")
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(artifact)
	}))
	defer files.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Params{
			Log:               testLogger(t),
			LinkURL:           "ws" + strings.TrimPrefix(srv.URL, "http"),
			MachineToken:      "mt-test",
			SupervisorVersion: "test",
			Labels:            map[string]string{"env": "unit"},
			Manifest:          manifest,
			Layout:            ensuredLayout(t),
		})
	}()

	// Hello arrives with identification, labels, and generation 0.
	waitFor(t, "hello", func() bool { h, _, _ := g.snapshot(); return len(h) == 1 })
	hellos, _, _ := g.snapshot()
	h := hellos[0]
	if h.LinkVersion != protocol.LinkVersion || h.Integration != "linktest" ||
		h.Labels["env"] != "unit" || h.AppliedGeneration != 0 || h.OS != runtime.GOOS {
		t.Fatalf("unexpected hello: %+v", h)
	}
	g.mu.Lock()
	tok := g.tokens[0]
	g.mu.Unlock()
	if tok != "mt-test" {
		t.Fatalf("machine token not sent on upgrade request: %q", tok)
	}

	// The initial observed report shows 1.0.0.
	waitFor(t, "initial observed", func() bool { _, o, _ := g.snapshot(); return len(o) >= 1 })

	// A pushed desired state is reconciled and reported with the
	// generation echo.
	sum := sha256Hex(artifact)
	g.pushState(protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    7,
		Package: &protocol.DesiredPackage{
			Version:  "2.0.0",
			Artifact: protocol.Artifact{URL: files.URL + "/a", SHA256: sum},
		},
	})
	waitFor(t, "upgrade reported", func() bool {
		_, o, _ := g.snapshot()
		for _, st := range o {
			if st.Version == "2.0.0" && st.Generation == 7 && st.Status == protocol.StatusInstalled {
				return true
			}
		}
		return false
	})
	if got, _ := os.ReadFile(filepath.Join(app, "VERSION")); string(got) != "2.0.0\n" {
		t.Fatalf("VERSION = %q, want 2.0.0", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop on context cancellation")
	}
}

func TestClientReconnects(t *testing.T) {
	manifest, _ := fixture(t)
	g := &gateway{t: t}
	srv := httptest.NewServer(g.handler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = Run(ctx, Params{
			Log:               testLogger(t),
			LinkURL:           "ws" + strings.TrimPrefix(srv.URL, "http"),
			MachineToken:      "mt-test",
			SupervisorVersion: "test",
			Manifest:          manifest,
			Layout:            ensuredLayout(t),
		})
	}()

	waitFor(t, "first connection", func() bool { _, _, n := g.snapshot(); return n == 1 })
	// Kill the connection server-side: the client must dial again and
	// send a fresh hello.
	g.mu.Lock()
	_ = g.conns[0].Close(websocket.StatusGoingAway, "test restart")
	g.mu.Unlock()
	waitFor(t, "reconnect with fresh hello", func() bool {
		h, _, n := g.snapshot()
		return n == 2 && len(h) == 2
	})
}
