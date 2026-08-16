package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/thinre/thinre/bundle"
	"github.com/thinre/thinre/protocol"
	"github.com/thinre/thinre/supervisor/hooks"
	"github.com/thinre/thinre/supervisor/state"
)

// applyBundle converges the configuration bundle (RT-CONFIG-004..007,
// RT-SUP-011); true means the complete revision is active. The flow is
// verify everything → stage everything → validate hook → place everything
// (with backups) → apply hook → health — and any failure after placement
// restores the previous files, so the revision is active completely or not
// at all (architecture §10 bundle-consistency rule).
func (r *Reconciler) applyBundle(ctx context.Context, doc protocol.DesiredState, local *state.Local) bool {
	b := doc.Bundle
	if local.ConfigRevision == b.Revision {
		return true
	}

	fail := func(message string) bool {
		r.log.Error("bundle application failed", "revision", b.Revision, "err", message)
		r.finish(ctx, doc, local, protocol.StatusFailed, r.currentVersion(ctx, *local), message)
		return false
	}

	r.log.Info("applying configuration bundle", "revision", b.Revision, "files", len(b.Files))
	r.reportPhase(ctx, doc.Generation, r.currentVersion(ctx, *local), local.ConfigRevision, protocol.StatusStaging, "")

	// Verify every file and the manifest hash before touching anything.
	if len(b.Files) == 0 {
		return fail(fmt.Sprintf("bundle revision %d has no files", b.Revision))
	}
	shas := make(map[string]string, len(b.Files))
	for _, f := range b.Files {
		if !path.IsAbs(f.Destination) {
			return fail(fmt.Sprintf("file %q destination %q is not absolute", f.ID, f.Destination))
		}
		sum := sha256.Sum256(f.Content)
		if hex.EncodeToString(sum[:]) != f.SHA256 {
			return fail(fmt.Sprintf("file %q failed verification", f.ID))
		}
		shas[f.ID] = f.SHA256
	}
	if got := bundle.ManifestHash(shas); got != b.ManifestHash {
		return fail(fmt.Sprintf("bundle manifest hash mismatch: revision %d is not a complete set", b.Revision))
	}

	// Stage the complete bundle.
	stageDir := filepath.Join(r.layout.Staging, fmt.Sprintf("bundle-%d", b.Revision))
	if err := os.RemoveAll(stageDir); err != nil {
		return fail("clear staging: " + err.Error())
	}
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return fail("create staging: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(stageDir) }()
	for _, f := range b.Files {
		if err := os.WriteFile(filepath.Join(stageDir, f.ID), f.Content, 0o644); err != nil {
			return fail("stage " + f.ID + ": " + err.Error())
		}
	}

	// Optional validate hook, on the staged (not yet live) bundle.
	if cfg := r.manifest.Configuration; cfg != nil && cfg.Validate != nil {
		if _, err := hooks.Run(ctx, substitute(cfg.Validate, bundlePathVar, stageDir)); err != nil {
			return fail("validate hook rejected revision: " + err.Error())
		}
	}

	// Place all files, backing up what they replace. A placement failure
	// restores everything placed so far.
	backupDir := filepath.Join(r.layout.Rollback, fmt.Sprintf("config-%d", b.Revision))
	if err := os.RemoveAll(backupDir); err != nil {
		return fail("clear backup dir: " + err.Error())
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return fail("create backup dir: " + err.Error())
	}

	type placement struct {
		file        protocol.BundleFile
		hadPrevious bool
	}
	var placed []placement
	restore := func() {
		for _, p := range placed {
			if p.hadPrevious {
				previous, err := os.ReadFile(filepath.Join(backupDir, p.file.ID))
				if err == nil {
					err = atomicWrite(p.file.Destination, previous)
				}
				if err != nil {
					r.log.Error("restore previous configuration", "file", p.file.ID, "err", err)
				}
			} else if err := os.Remove(p.file.Destination); err != nil {
				r.log.Error("remove partially placed file", "file", p.file.ID, "err", err)
			}
		}
	}

	for _, f := range b.Files {
		hadPrevious := false
		if previous, err := os.ReadFile(f.Destination); err == nil {
			hadPrevious = true
			if err := os.WriteFile(filepath.Join(backupDir, f.ID), previous, 0o600); err != nil {
				restore()
				return fail("back up " + f.ID + ": " + err.Error())
			}
		}
		if err := os.MkdirAll(filepath.Dir(f.Destination), 0o755); err != nil {
			restore()
			return fail("create destination dir for " + f.ID + ": " + err.Error())
		}
		if err := atomicWrite(f.Destination, f.Content); err != nil {
			restore()
			return fail("place " + f.ID + ": " + err.Error())
		}
		placed = append(placed, placement{file: f, hadPrevious: hadPrevious})
	}

	// Optional apply hook, then the health gate; failures restore the
	// previous configuration and re-run apply best-effort so the restored
	// files are active again.
	r.reportPhase(ctx, doc.Generation, r.currentVersion(ctx, *local), local.ConfigRevision, protocol.StatusApplying, "")
	applyHook := func() error {
		if cfg := r.manifest.Configuration; cfg != nil && cfg.Apply != nil {
			_, err := hooks.Run(ctx, cfg.Apply)
			return err
		}
		return nil
	}
	if err := applyHook(); err != nil {
		restore()
		_ = applyHook()
		return fail("apply hook failed: " + err.Error())
	}
	if _, err := hooks.Run(ctx, r.manifest.Health.Check); err != nil {
		restore()
		_ = applyHook()
		return fail("health check failed after configuration apply")
	}

	local.ConfigRevision = b.Revision
	r.save(*local)
	r.log.Info("configuration bundle active", "revision", b.Revision)
	return true
}

// atomicWrite writes content via a same-directory temp file and rename, so
// a crash mid-write can never leave a torn configuration file.
func atomicWrite(dest string, content []byte) error {
	tmp := dest + ".thinre-tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
