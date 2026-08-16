package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "supervisor.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validConfig = `
api_url: https://api.example.test
opamp_url: wss://opamp.example.test
integrations:
  - manifest: /etc/thinre/integrations/blackbox.yaml
data_dir: /var/lib/thinre
name: test-runtime
`

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if cfg.Name != "test-runtime" || cfg.DataDir != "/var/lib/thinre" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if len(cfg.Integrations) != 1 || cfg.Integrations[0].Manifest != "/etc/thinre/integrations/blackbox.yaml" {
		t.Fatalf("unexpected integrations: %+v", cfg.Integrations)
	}
}

func TestLoadConfigMultipleIntegrations(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
api_url: https://api.example.test
opamp_url: wss://opamp.example.test
integrations:
  - manifest: /etc/thinre/integrations/app-a.yaml
  - manifest: /etc/thinre/integrations/app-b.yaml
    name: b-custom
`))
	if err != nil {
		t.Fatalf("multi-integration config rejected: %v", err)
	}
	if len(cfg.Integrations) != 2 || cfg.Integrations[1].Name != "b-custom" {
		t.Fatalf("unexpected integrations: %+v", cfg.Integrations)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
api_url: https://api.example.test
opamp_url: wss://opamp.example.test
integrations:
  - manifest: /etc/thinre/integrations/blackbox.yaml
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "/var/lib/thinre" {
		t.Errorf("data_dir default = %q", cfg.DataDir)
	}
	if cfg.Name == "" {
		t.Error("name should default to the hostname")
	}
}

func TestLoadConfigRejects(t *testing.T) {
	cases := []struct {
		name, content, hint string
	}{
		{"missing api_url", strings.Replace(validConfig, "api_url: https://api.example.test", "", 1), "api_url"},
		{"missing integrations", strings.Replace(validConfig, "integrations:\n  - manifest: /etc/thinre/integrations/blackbox.yaml", "", 1), "integrations"},
		{"entry without manifest", strings.Replace(validConfig, "- manifest: /etc/thinre/integrations/blackbox.yaml", "- name: only-a-name", 1), "manifest is required"},
		{"removed singular field", validConfig + "\nintegration_manifest: /etc/thinre/x.yaml\n", "integration_manifest"},
		{"unknown field", validConfig + "\nopamp_urll: typo\n", "opamp_urll"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, c.content))
			if err == nil || !strings.Contains(err.Error(), c.hint) {
				t.Fatalf("want error mentioning %q, got %v", c.hint, err)
			}
		})
	}
}

func TestEnrollmentTokenEnvOverride(t *testing.T) {
	t.Setenv("THINRE_ENROLLMENT_TOKEN", "from-env")
	cfg, err := LoadConfig(writeConfig(t, validConfig+"enrollment_token: from-file\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EnrollmentToken != "from-env" {
		t.Fatalf("env override not applied: %q", cfg.EnrollmentToken)
	}
}

func TestLayoutEnsure(t *testing.T) {
	layout := NewLayout(t.TempDir())
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{layout.Identity, layout.State, layout.Artifacts, layout.Staging, layout.Rollback} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Errorf("missing layout dir %s: %v", dir, err)
		}
	}
}

func TestLoadConfigLabels(t *testing.T) {
	path := writeConfig(t, validConfig+`
labels:
  env: staging
  rack: r1
`)
	// The environment variable merges over (and can override) the file.
	t.Setenv("THINRE_LABELS", "rack=r7, dc=paris")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("labels rejected: %v", err)
	}
	want := map[string]string{"env": "staging", "rack": "r7", "dc": "paris"}
	if len(cfg.Labels) != len(want) {
		t.Fatalf("unexpected labels: %+v", cfg.Labels)
	}
	for k, v := range want {
		if cfg.Labels[k] != v {
			t.Fatalf("label %s = %q, want %q (all: %+v)", k, cfg.Labels[k], v, cfg.Labels)
		}
	}

	t.Setenv("THINRE_LABELS", "notapair")
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("malformed THINRE_LABELS accepted")
	}
}
