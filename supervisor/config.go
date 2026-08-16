package supervisor

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is where the Supervisor looks for its configuration
// unless told otherwise.
const DefaultConfigPath = "/etc/thinre/supervisor.yaml"

// Config is the Supervisor's static configuration
// (/etc/thinre/supervisor.yaml). Everything dynamic — desired state,
// artifacts, configuration bundles — arrives over OpAMP instead.
type Config struct {
	// APIURL is the cloud REST endpoint, used only for enrollment.
	APIURL string `yaml:"api_url"`
	// OpAMPURL is the WebSocket endpoint of the OpAMP gateway.
	OpAMPURL string `yaml:"opamp_url"`
	// EnrollmentToken is consumed exactly once on first start; the
	// THINRE_ENROLLMENT_TOKEN environment variable overrides it so
	// tokens can be injected without editing the file.
	EnrollmentToken string `yaml:"enrollment_token,omitempty"`
	// IntegrationManifest is the path of the Integration v1 manifest
	// describing the managed software.
	IntegrationManifest string `yaml:"integration_manifest"`
	// DataDir is the Supervisor's writable state directory.
	DataDir string `yaml:"data_dir,omitempty"`
	// Name is the runtime display name; defaults to the hostname.
	Name string `yaml:"name,omitempty"`
	// Labels are operator-defined tags (environment, datacenter, rack …)
	// reported to the cloud with the host identification. The
	// THINRE_LABELS environment variable ("key=value,key=value") merges
	// over the file's labels so containers can inject them.
	Labels map[string]string `yaml:"labels,omitempty"`
}

// LoadConfig reads and validates the configuration file. Unknown fields are
// rejected for the same reason the integration spec rejects them: a typo
// must fail at startup, not silently change behavior.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if env := os.Getenv("THINRE_ENROLLMENT_TOKEN"); env != "" {
		cfg.EnrollmentToken = env
	}
	if env := os.Getenv("THINRE_LABELS"); env != "" {
		if cfg.Labels == nil {
			cfg.Labels = make(map[string]string)
		}
		for _, pair := range strings.Split(env, ",") {
			key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
			if !ok || key == "" {
				return nil, fmt.Errorf("THINRE_LABELS: %q is not key=value", pair)
			}
			cfg.Labels[key] = value
		}
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/var/lib/thinre"
	}
	if cfg.Name == "" {
		host, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("no name configured and hostname unavailable: %w", err)
		}
		cfg.Name = host
	}

	if cfg.APIURL == "" || cfg.OpAMPURL == "" {
		return nil, fmt.Errorf("config %s: api_url and opamp_url are required", path)
	}
	if cfg.IntegrationManifest == "" {
		return nil, fmt.Errorf("config %s: integration_manifest is required", path)
	}
	return &cfg, nil
}
