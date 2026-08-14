// Package state persists the Supervisor's reconciliation state across
// restarts: the observed version, the last desired-state generation acted
// on, and any in-flight operation. Writes are atomic (temp file + rename)
// so a crash leaves either the previous state or the new one — never a
// torn file (RT-SUP-005).
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const fileName = "reconcile.json"

// Operation describes a lifecycle operation in progress, persisted before
// the upgrade hook runs so a crashed Supervisor can tell "I was mid-upgrade"
// from "I was idle".
type Operation struct {
	Generation   int64  `json:"generation"`
	Version      string `json:"version"`
	Phase        string `json:"phase"`
	ArtifactPath string `json:"artifact_path,omitempty"`
}

// Local is the persisted reconciliation state.
type Local struct {
	ObservedVersion string     `json:"observed_version,omitempty"`
	LastGeneration  int64      `json:"last_generation"`
	InFlight        *Operation `json:"in_flight,omitempty"`
}

// Load reads the state from dir; a missing file is an empty state.
func Load(dir string) (Local, error) {
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if errors.Is(err, fs.ErrNotExist) {
		return Local{}, nil
	}
	if err != nil {
		return Local{}, fmt.Errorf("read reconcile state: %w", err)
	}
	var l Local
	if err := json.Unmarshal(data, &l); err != nil {
		return Local{}, fmt.Errorf("parse reconcile state: %w", err)
	}
	return l, nil
}

// Save writes the state atomically.
func Save(dir string, l Local) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, fileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write reconcile state: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, fileName)); err != nil {
		return fmt.Errorf("commit reconcile state: %w", err)
	}
	return nil
}
