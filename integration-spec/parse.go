package integrationspec

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// nameRe constrains integration names to DNS-label style: stable, safe in
// URLs, object-storage keys, and file names.
var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// maxTimeout bounds hook timeouts: a lifecycle hook that needs more than an
// hour is a process, not a hook.
const maxTimeout = time.Hour

// Parse decodes and validates an Integration v1 manifest. Unknown fields
// are rejected: a typo in a hook name must fail loudly at validation time,
// not silently skip the hook at upgrade time.
func Parse(data []byte) (*Integration, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var in Integration
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("parse integration manifest: %w", err)
	}
	if err := Validate(&in); err != nil {
		return nil, err
	}
	return &in, nil
}

// Validate checks an Integration against the v1 rules (RT-INT-002) and
// reports every problem at once rather than stopping at the first.
func Validate(in *Integration) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if in.APIVersion != APIVersion {
		add("apiVersion must be %q, got %q", APIVersion, in.APIVersion)
	}
	if in.Kind != Kind {
		add("kind must be %q, got %q", Kind, in.Kind)
	}
	if !nameRe.MatchString(in.Metadata.Name) {
		add("metadata.name %q must be a lowercase DNS-style label", in.Metadata.Name)
	}

	if in.Package.Upgrade == nil {
		add("package.upgrade is required")
	}
	checkHook(add, "package.upgrade", in.Package.Upgrade)
	checkHook(add, "package.rollback", in.Package.Rollback)
	checkHook(add, "package.version", in.Package.Version)

	if in.Health.Check == nil {
		add("health.check is required")
	}
	checkHook(add, "health.check", in.Health.Check)

	if in.Lifecycle != nil {
		checkHook(add, "lifecycle.restart", in.Lifecycle.Restart)
	}

	if in.Configuration != nil {
		if len(in.Configuration.Files) == 0 {
			add("configuration.files must not be empty when configuration is present")
		}
		ids := map[string]bool{}
		dests := map[string]bool{}
		for i, f := range in.Configuration.Files {
			if f.ID == "" {
				add("configuration.files[%d].id is required", i)
			} else if ids[f.ID] {
				add("configuration.files: duplicate id %q", f.ID)
			}
			ids[f.ID] = true
			if !isAbsAnyOS(f.Destination) {
				add("configuration.files[%d].destination %q must be an absolute path", i, f.Destination)
			} else if dests[f.Destination] {
				add("configuration.files: duplicate destination %q", f.Destination)
			}
			dests[f.Destination] = true
		}
		checkHook(add, "configuration.validate", in.Configuration.Validate)
		checkHook(add, "configuration.apply", in.Configuration.Apply)
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid integration manifest:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// isAbsAnyOS reports whether p is an absolute path on SOME operating
// system — Unix (/opt/…) or Windows (C:\…). Manifests travel across
// platforms (a manifest for a Windows host is published to a Linux
// cloud), so validation cannot use the validating host's own rules.
func isAbsAnyOS(p string) bool {
	if path.IsAbs(p) {
		return true
	}
	// Drive-letter form: "C:\…" or "C:/…".
	return len(p) >= 3 &&
		(p[0] >= 'A' && p[0] <= 'Z' || p[0] >= 'a' && p[0] <= 'z') &&
		p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// checkHook validates one hook definition; nil hooks are allowed here —
// requiredness is decided by the caller.
func checkHook(add func(string, ...any), name string, h *Hook) {
	if h == nil {
		return
	}
	if !isAbsAnyOS(h.Executable) {
		add("%s.executable %q must be an absolute path", name, h.Executable)
	}
	if t := time.Duration(h.Timeout); t < 0 || t > maxTimeout {
		add("%s.timeout %s must be between 0 and %s", name, t, maxTimeout)
	}
}
