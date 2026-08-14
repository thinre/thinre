package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
)

// Layout is the Supervisor's on-disk working layout under the data dir.
type Layout struct {
	// Identity holds the machine identity file (0600).
	Identity string
	// State holds the crash-safe reconciliation state.
	State string
	// Artifacts holds verified downloaded packages.
	Artifacts string
	// Staging holds in-progress downloads and unpacked bundles.
	Staging string
	// Rollback holds data needed to restore the previous version.
	Rollback string
}

// NewLayout maps the layout under dataDir.
func NewLayout(dataDir string) Layout {
	return Layout{
		Identity:  filepath.Join(dataDir, "identity"),
		State:     filepath.Join(dataDir, "state"),
		Artifacts: filepath.Join(dataDir, "artifacts"),
		Staging:   filepath.Join(dataDir, "staging"),
		Rollback:  filepath.Join(dataDir, "rollback"),
	}
}

// Ensure creates every directory. 0700: everything under the data dir is
// either credentials or lifecycle state nothing else should touch.
func (l Layout) Ensure() error {
	for _, dir := range []string{l.Identity, l.State, l.Artifacts, l.Staging, l.Rollback} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}
