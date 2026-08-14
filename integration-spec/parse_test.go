package integrationspec

import (
	"os"
	"strings"
	"testing"
	"time"
)

// valid is a complete, correct manifest; invalid cases below are built by
// mutating it textually so each case isolates exactly one rule.
const valid = `
apiVersion: thinre.io/v1
kind: Integration
metadata:
  name: acme-agent
package:
  upgrade:
    executable: /opt/acme/hooks/upgrade.sh
    args: ["{{ artifact.path }}"]
    timeout: 300s
  rollback:
    executable: /opt/acme/hooks/rollback.sh
  version:
    executable: /opt/acme/hooks/version.sh
    timeout: 10s
configuration:
  files:
    - id: main
      destination: /etc/acme/agent.yaml
    - id: logging
      destination: /etc/acme/logging.yaml
  validate:
    executable: /opt/acme/hooks/validate-config.sh
  apply:
    executable: /opt/acme/hooks/apply-config.sh
lifecycle:
  restart:
    executable: /opt/acme/hooks/restart.sh
health:
  check:
    executable: /opt/acme/hooks/health.sh
    timeout: 10s
`

func TestParseValid(t *testing.T) {
	in, err := Parse([]byte(valid))
	if err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if in.Metadata.Name != "acme-agent" {
		t.Errorf("name = %q", in.Metadata.Name)
	}
	if got := time.Duration(in.Package.Upgrade.Timeout); got != 300*time.Second {
		t.Errorf("upgrade timeout = %s, want 300s", got)
	}
	if in.Package.Version == nil {
		t.Error("version hook not parsed")
	}
	if len(in.Configuration.Files) != 2 {
		t.Errorf("files = %d, want 2", len(in.Configuration.Files))
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(string) string
		wantHint string
	}{
		{"wrong apiVersion", replace("thinre.io/v1", "thinre.io/v2"), "apiVersion"},
		{"wrong kind", replace("kind: Integration", "kind: Thing"), "kind"},
		{"bad name", replace("name: acme-agent", "name: Acme_Agent"), "metadata.name"},
		{"missing upgrade", cut("  upgrade:\n    executable: /opt/acme/hooks/upgrade.sh\n    args: [\"{{ artifact.path }}\"]\n    timeout: 300s\n"), "package.upgrade is required"},
		{"relative executable", replace("/opt/acme/hooks/upgrade.sh", "hooks/upgrade.sh"), "absolute path"},
		{"missing health", cut("health:\n  check:\n    executable: /opt/acme/hooks/health.sh\n    timeout: 10s\n"), "health.check is required"},
		{"duplicate file id", replace("id: logging", "id: main"), "duplicate id"},
		{"duplicate destination", replace("destination: /etc/acme/logging.yaml", "destination: /etc/acme/agent.yaml"), "duplicate destination"},
		{"relative destination", replace("destination: /etc/acme/agent.yaml", "destination: etc/agent.yaml"), "absolute path"},
		{"excessive timeout", replace("timeout: 300s", "timeout: 25h"), "must be between"},
		{"unknown field", replace("lifecycle:", "lifecycel:"), "field lifecycel not found"},
		{"shell command form", replace("executable: /opt/acme/hooks/rollback.sh", "command: /bin/sh -c 'rollback'"), "not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.mutate(valid)))
			if err == nil {
				t.Fatal("invalid manifest accepted")
			}
			if !strings.Contains(err.Error(), c.wantHint) {
				t.Fatalf("error %q does not mention %q", err, c.wantHint)
			}
		})
	}
}

func replace(old, new string) func(string) string {
	return func(s string) string { return strings.Replace(s, old, new, 1) }
}

func cut(section string) func(string) string {
	return func(s string) string { return strings.Replace(s, section, "", 1) }
}

// TestFixtureManifest keeps the shipped blackbox fixture manifest valid: it
// is both a working example and the end-to-end test integration.
func TestFixtureManifest(t *testing.T) {
	data, err := os.ReadFile("../sdk/fixtures/blackbox/blackbox.yaml")
	if err != nil {
		t.Fatalf("read fixture manifest: %v", err)
	}
	in, err := Parse(data)
	if err != nil {
		t.Fatalf("fixture manifest invalid: %v", err)
	}
	if in.Metadata.Name != "blackbox" {
		t.Errorf("fixture name = %q", in.Metadata.Name)
	}
	if in.Package.Version == nil {
		t.Error("fixture must define the version hook — the demo depends on observed-version reporting")
	}
}
