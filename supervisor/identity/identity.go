// Package identity persists the Supervisor's machine identity: the
// runtime/organization binding and machine token obtained at enrollment.
// The identity file is the Supervisor's only credential, stored 0600 and
// written atomically so a crash can never leave it half-written.
package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const fileName = "identity.json"

// Identity is the persisted machine identity.
type Identity struct {
	RuntimeID      string    `json:"runtime_id"`
	OrganizationID string    `json:"organization_id"`
	MachineToken   string    `json:"machine_token"`
	EnrolledAt     time.Time `json:"enrolled_at"`
}

// Load reads the identity from dir. A missing file returns (nil, nil):
// "not enrolled yet" is a normal state, not an error.
func Load(dir string) (*Identity, error) {
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read identity: %w", err)
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}
	if id.RuntimeID == "" || id.OrganizationID == "" || id.MachineToken == "" {
		return nil, fmt.Errorf("identity file is incomplete")
	}
	return &id, nil
}

// Save writes the identity atomically (temp file + rename) with 0600
// permissions, so a crash mid-write leaves either the old identity or the
// new one — never a torn file.
func Save(dir string, id Identity) error {
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, fileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, fileName)); err != nil {
		return fmt.Errorf("commit identity: %w", err)
	}
	return nil
}
