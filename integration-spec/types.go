package integrationspec

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// APIVersion is the only Integration schema version this package accepts.
const APIVersion = "thinre.io/v1"

// Kind is the only document kind this package accepts.
const Kind = "Integration"

// Integration is the parsed Integration v1 manifest: the contract that
// tells the Supervisor how to manage one black-box software product.
type Integration struct {
	APIVersion    string         `yaml:"apiVersion"`
	Kind          string         `yaml:"kind"`
	Metadata      Metadata       `yaml:"metadata"`
	Package       Package        `yaml:"package"`
	Configuration *Configuration `yaml:"configuration,omitempty"`
	Lifecycle     *Lifecycle     `yaml:"lifecycle,omitempty"`
	Health        Health         `yaml:"health"`
}

// Metadata identifies the integration.
type Metadata struct {
	Name string `yaml:"name"`
}

// Package defines the package lifecycle hooks. Upgrade is mandatory —
// an integration that cannot upgrade anything manages nothing. Version is
// optional but strongly recommended: it prints the currently installed
// version to stdout and is how the Supervisor learns observed state.
type Package struct {
	Upgrade  *Hook `yaml:"upgrade"`
	Rollback *Hook `yaml:"rollback,omitempty"`
	Version  *Hook `yaml:"version,omitempty"`
}

// Configuration defines the managed configuration files and the optional
// bundle-level validate/apply hooks. Hooks operate on the complete bundle,
// never on individual files (bundle consistency rule).
type Configuration struct {
	Files    []ConfigFile `yaml:"files"`
	Validate *Hook        `yaml:"validate,omitempty"`
	Apply    *Hook        `yaml:"apply,omitempty"`
}

// ConfigFile maps a stable file ID to its destination path on disk.
type ConfigFile struct {
	ID          string `yaml:"id"`
	Destination string `yaml:"destination"`
}

// Lifecycle defines process-level operations.
type Lifecycle struct {
	Restart *Hook `yaml:"restart,omitempty"`
}

// Health defines the health check. Mandatory: the upgrade flow is gated on
// it, and rollout health gates depend on it.
type Health struct {
	Check *Hook `yaml:"check"`
}

// Hook is one lifecycle operation: an explicit executable with an explicit
// argument array. There is deliberately no shell-command form — implicit
// `sh -c` invocation is the main command-injection surface this contract
// is designed to exclude (architecture §7 security rule).
type Hook struct {
	Executable string   `yaml:"executable"`
	Args       []string `yaml:"args,omitempty"`
	Timeout    Duration `yaml:"timeout,omitempty"`
}

// Duration wraps time.Duration to parse YAML strings like "300s" or "5m".
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML implements yaml.Marshaler so round-tripping preserves the
// human-readable form.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}
