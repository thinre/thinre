package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/thinre/thinre/bundle"
	integrationspec "github.com/thinre/thinre/integration-spec"
	"github.com/thinre/thinre/protocol"
)

// mkBundle builds a valid DesiredBundle from id → (destination, content).
func mkBundle(revision int64, files map[string][2]string) *protocol.DesiredBundle {
	b := &protocol.DesiredBundle{Revision: revision}
	shas := map[string]string{}
	for id, dc := range files {
		sum := sha256.Sum256([]byte(dc[1]))
		sha := hex.EncodeToString(sum[:])
		shas[id] = sha
		b.Files = append(b.Files, protocol.BundleFile{
			ID:          id,
			Destination: dc[0],
			SHA256:      sha,
			Content:     []byte(dc[1]),
		})
	}
	b.ManifestHash = bundle.ManifestHash(shas)
	return b
}

// withConfigHooks adds validate/apply hooks to the fixture manifest: the
// validate hook fails when a "reject" marker exists next to the app.
func withConfigHooks(t *testing.T, manifest *integrationspec.Integration, app string) {
	t.Helper()
	validate := filepath.Join(app, "validate.sh")
	if err := os.WriteFile(validate, []byte("#!/bin/sh\ntest ! -e \""+app+"/reject\" && test -d \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	apply := filepath.Join(app, "apply.sh")
	if err := os.WriteFile(apply, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest.Configuration = &integrationspec.Configuration{
		Files:    []integrationspec.ConfigFile{{ID: "main", Destination: filepath.Join(app, "config", "app.conf")}},
		Validate: &integrationspec.Hook{Executable: validate, Args: []string{"{{ bundle.path }}"}},
		Apply:    &integrationspec.Hook{Executable: apply},
	}
}

func TestApplyBundle(t *testing.T) {
	manifest, app := fixture(t, false)
	withConfigHooks(t, manifest, app)
	rec, reports := testReconciler(t, manifest)
	dest := filepath.Join(app, "config", "app.conf")

	doc := protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    10,
		Bundle:        mkBundle(1, map[string][2]string{"main": {dest, "setting=A\n"}}),
	}
	rec.Apply(context.Background(), doc)

	got := statuses(*reports)
	want := []string{protocol.StatusStaging, protocol.StatusApplying, protocol.StatusInstalled}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("phases = %v, want %v", got, want)
	}
	final := (*reports)[len(*reports)-1]
	if final.ConfigRevision == nil || *final.ConfigRevision != 1 {
		t.Fatalf("final report revision = %v, want 1", final.ConfigRevision)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "setting=A\n" {
		t.Fatalf("placed file = %q (%v)", data, err)
	}

	// Revision 2 replaces the content.
	doc.Generation, doc.Bundle = 11, mkBundle(2, map[string][2]string{"main": {dest, "setting=B\n"}})
	rec.Apply(context.Background(), doc)
	data, _ = os.ReadFile(dest)
	if string(data) != "setting=B\n" {
		t.Fatalf("revision 2 not placed: %q", data)
	}

	// Re-applying the active revision is a no-op success.
	before := len(*reports)
	rec.Apply(context.Background(), doc)
	if final := (*reports)[len(*reports)-1]; final.Status != protocol.StatusInstalled {
		t.Fatalf("re-apply status = %s", final.Status)
	}
	if len(*reports) != before+1 { // just the terminal report, no phases
		t.Fatalf("re-apply emitted phases: %v", statuses((*reports)[before:]))
	}
}

func TestApplyBundleValidateRejection(t *testing.T) {
	manifest, app := fixture(t, false)
	withConfigHooks(t, manifest, app)
	rec, reports := testReconciler(t, manifest)
	dest := filepath.Join(app, "config", "app.conf")

	if err := os.WriteFile(filepath.Join(app, "reject"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	rec.Apply(context.Background(), protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    12,
		Bundle:        mkBundle(1, map[string][2]string{"main": {dest, "setting=A\n"}}),
	})

	final := (*reports)[len(*reports)-1]
	if final.Status != protocol.StatusFailed {
		t.Fatalf("final status = %s, want failed", final.Status)
	}
	// Validation happens before placement: the destination must not exist.
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("file was placed despite validation rejection")
	}
}

func TestApplyBundleHealthFailureRestores(t *testing.T) {
	manifest, app := fixture(t, false)
	withConfigHooks(t, manifest, app)
	dest := filepath.Join(app, "config", "app.conf")

	// Health fails when the config contains "poison".
	health := filepath.Join(app, "health3.sh")
	if err := os.WriteFile(health, []byte("#!/bin/sh\n! grep -q poison \""+dest+"\" 2>/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest.Health.Check = &integrationspec.Hook{Executable: health}
	rec, reports := testReconciler(t, manifest)

	// Revision 1 is fine.
	rec.Apply(context.Background(), protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    13,
		Bundle:        mkBundle(1, map[string][2]string{"main": {dest, "setting=A\n"}}),
	})
	// Revision 2 poisons the app: health fails, the previous file returns.
	rec.Apply(context.Background(), protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    14,
		Bundle:        mkBundle(2, map[string][2]string{"main": {dest, "poison\n"}}),
	})

	final := (*reports)[len(*reports)-1]
	if final.Status != protocol.StatusFailed {
		t.Fatalf("final status = %s, want failed", final.Status)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "setting=A\n" {
		t.Fatalf("previous configuration not restored: %q", data)
	}
	if final.ConfigRevision == nil || *final.ConfigRevision != 1 {
		t.Fatalf("active revision after restore = %v, want 1", final.ConfigRevision)
	}
}

func TestApplyPackageAndBundleTogether(t *testing.T) {
	manifest, app := fixture(t, false)
	withConfigHooks(t, manifest, app)
	url, sha := artifactServer(t, "2.0.0\n")
	rec, reports := testReconciler(t, manifest)
	dest := filepath.Join(app, "config", "app.conf")

	rec.Apply(context.Background(), protocol.DesiredState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    15,
		Package:       &protocol.DesiredPackage{Version: "2.0.0", Artifact: protocol.Artifact{URL: url, SHA256: sha}},
		Bundle:        mkBundle(1, map[string][2]string{"main": {dest, "setting=A\n"}}),
	})

	final := (*reports)[len(*reports)-1]
	if final.Status != protocol.StatusInstalled || final.Version != "2.0.0" ||
		final.ConfigRevision == nil || *final.ConfigRevision != 1 {
		t.Fatalf("combined apply final = %+v", final)
	}
	got := statuses(*reports)
	// Package phases first, then bundle phases, one terminal.
	want := []string{protocol.StatusDownloading, protocol.StatusUpgrading, protocol.StatusStaging, protocol.StatusApplying, protocol.StatusInstalled}
	if len(got) != len(want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
}
