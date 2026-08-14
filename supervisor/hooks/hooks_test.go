package hooks

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	integrationspec "github.com/thinre/thinre/integration-spec"
)

// The executor runs POSIX executables; these tests need /bin/sh and run
// everywhere except Windows (CI and the testbed cover them there).
func requirePOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hook execution tests need a POSIX shell")
	}
}

func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunCapturesStdout(t *testing.T) {
	requirePOSIX(t)
	h := &integrationspec.Hook{Executable: script(t, `echo "  1.2.3  "`)}
	out, err := Run(context.Background(), h)
	if err != nil {
		t.Fatal(err)
	}
	if out != "1.2.3" {
		t.Fatalf("stdout = %q, want trimmed 1.2.3", out)
	}
}

func TestRunFailureCarriesStderr(t *testing.T) {
	requirePOSIX(t)
	h := &integrationspec.Hook{Executable: script(t, `echo "boom" >&2; exit 3`)}
	_, err := Run(context.Background(), h)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should carry stderr, got %v", err)
	}
}

func TestRunTimeout(t *testing.T) {
	requirePOSIX(t)
	h := &integrationspec.Hook{
		Executable: script(t, `sleep 5`),
		Timeout:    integrationspec.Duration(200 * time.Millisecond),
	}
	start := time.Now()
	_, err := Run(context.Background(), h)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout did not interrupt the hook promptly")
	}
}

func TestRunBoundsOutput(t *testing.T) {
	requirePOSIX(t)
	// 1 MiB of output must not blow past the 64 KiB capture bound.
	h := &integrationspec.Hook{Executable: script(t, `head -c 1048576 /dev/zero | tr '\0' 'x'`)}
	out, err := Run(context.Background(), h)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > maxOutput {
		t.Fatalf("captured %d bytes, bound is %d", len(out), maxOutput)
	}
}

func TestRunRestrictsEnvironment(t *testing.T) {
	requirePOSIX(t)
	t.Setenv("SUPERVISOR_SECRET", "leak")
	h := &integrationspec.Hook{Executable: script(t, `printf '%s' "${SUPERVISOR_SECRET:-clean}"`)}
	out, err := Run(context.Background(), h)
	if err != nil {
		t.Fatal(err)
	}
	if out != "clean" {
		t.Fatalf("environment leaked into hook: %q", out)
	}
}

func TestRunNilHook(t *testing.T) {
	if _, err := Run(context.Background(), nil); err == nil {
		t.Fatal("nil hook accepted")
	}
}
