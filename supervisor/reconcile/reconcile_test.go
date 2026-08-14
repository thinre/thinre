package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	integrationspec "github.com/thinre/thinre/integration-spec"
	"github.com/thinre/thinre/protocol"
	"github.com/thinre/thinre/supervisor"
)

// The reconciler executes POSIX hooks; these tests run everywhere except
// Windows (CI and the testbed cover them there).

// fixture builds a miniature managed app in a temp dir: VERSION file plus
// version/upgrade/health hooks operating on it. The upgrade hook writes
// the artifact's content into VERSION — the artifact IS the new version
// string, which keeps the whole flow observable.
func fixture(t *testing.T, failUpgrade bool) (*integrationspec.Integration, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("reconcile tests need a POSIX shell")
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
	health := write("health.sh", `test -r "`+app+`/VERSION"`)
	upgradeBody := `cp "$1" "` + app + `/VERSION"`
	if failUpgrade {
		upgradeBody = `echo "upgrade exploded" >&2; exit 1`
	}
	upgrade := write("upgrade.sh", upgradeBody)

	return &integrationspec.Integration{
		APIVersion: integrationspec.APIVersion,
		Kind:       integrationspec.Kind,
		Metadata:   integrationspec.Metadata{Name: "mini"},
		Package: integrationspec.Package{
			Upgrade: &integrationspec.Hook{Executable: upgrade, Args: []string{"{{ artifact.path }}"}},
			Version: &integrationspec.Hook{Executable: version},
		},
		Health: integrationspec.Health{Check: &integrationspec.Hook{Executable: health}},
	}, app
}

// artifactServer serves the given payload and returns (url, sha256).
func artifactServer(t *testing.T, payload string) (string, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256([]byte(payload))
	return srv.URL, hex.EncodeToString(sum[:])
}

func testReconciler(t *testing.T, manifest *integrationspec.Integration) (*Reconciler, *[]protocol.ObservedState) {
	t.Helper()
	layout := supervisor.NewLayout(t.TempDir())
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	var reports []protocol.ObservedState
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rec := New(log, manifest, layout, func(_ context.Context, st protocol.ObservedState) {
		reports = append(reports, st)
	})
	return rec, &reports
}

func statuses(reports []protocol.ObservedState) []string {
	out := make([]string, len(reports))
	for i, r := range reports {
		out[i] = r.Status
	}
	return out
}

func TestApplyUpgrades(t *testing.T) {
	manifest, _ := fixture(t, false)
	url, sha := artifactServer(t, "2.0.0\n")
	rec, reports := testReconciler(t, manifest)

	rec.Apply(context.Background(), protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    3,
		Package:       &protocol.DesiredPackage{Version: "2.0.0", Artifact: protocol.Artifact{URL: url, SHA256: sha}},
	})

	got := statuses(*reports)
	want := []string{protocol.StatusDownloading, protocol.StatusUpgrading, protocol.StatusInstalled}
	if len(got) != len(want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phases = %v, want %v", got, want)
		}
	}
	final := (*reports)[len(*reports)-1]
	if final.Version != "2.0.0" || final.Health != protocol.HealthHealthy || final.Generation != 3 {
		t.Fatalf("final report = %+v", final)
	}

	// The observation now reflects the new version and generation.
	obs := rec.Observe(context.Background())
	if obs.Version != "2.0.0" || obs.Generation != 3 {
		t.Fatalf("observe after upgrade = %+v", obs)
	}
}

func TestApplyAlreadyConverged(t *testing.T) {
	manifest, _ := fixture(t, false)
	rec, reports := testReconciler(t, manifest)

	rec.Apply(context.Background(), protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    5,
		Package:       &protocol.DesiredPackage{Version: "1.0.0", Artifact: protocol.Artifact{URL: "http://unused.invalid", SHA256: "00"}},
	})

	got := statuses(*reports)
	if len(got) != 1 || got[0] != protocol.StatusInstalled {
		t.Fatalf("phases = %v, want [installed] with no download", got)
	}
}

func TestApplyVerificationFailure(t *testing.T) {
	manifest, app := fixture(t, false)
	url, _ := artifactServer(t, "2.0.0\n")
	rec, reports := testReconciler(t, manifest)

	rec.Apply(context.Background(), protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    4,
		Package:       &protocol.DesiredPackage{Version: "2.0.0", Artifact: protocol.Artifact{URL: url, SHA256: "deadbeef" + "00"}},
	})

	final := (*reports)[len(*reports)-1]
	if final.Status != protocol.StatusFailed {
		t.Fatalf("final status = %s, want failed", final.Status)
	}
	// The managed app was never touched: verification failures must stop
	// everything before the upgrade hook.
	data, _ := os.ReadFile(filepath.Join(app, "VERSION"))
	if string(data) != "1.0.0\n" {
		t.Fatalf("app mutated despite verification failure: %q", data)
	}
}

func TestApplyUpgradeHookFailure(t *testing.T) {
	manifest, _ := fixture(t, true)
	url, sha := artifactServer(t, "2.0.0\n")
	rec, reports := testReconciler(t, manifest)

	rec.Apply(context.Background(), protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    6,
		Package:       &protocol.DesiredPackage{Version: "2.0.0", Artifact: protocol.Artifact{URL: url, SHA256: sha}},
	})

	final := (*reports)[len(*reports)-1]
	if final.Status != protocol.StatusFailed || final.Version != "1.0.0" {
		t.Fatalf("final report = %+v, want failed at 1.0.0", final)
	}
}
