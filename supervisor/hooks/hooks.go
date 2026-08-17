// Package hooks executes integration lifecycle hooks: an explicit
// executable with an explicit argument array, a timeout, a minimal
// environment, and bounded output. There is deliberately no shell
// involvement anywhere — hooks are exec'd directly (architecture §7
// security rule).
package hooks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	integrationspec "github.com/thinre/thinre/integration-spec"
)

// DefaultTimeout applies when a hook does not declare one.
const DefaultTimeout = 60 * time.Second

// maxOutput bounds captured hook output; anything beyond is discarded so a
// chatty hook cannot exhaust the Supervisor's memory.
const maxOutput = 64 * 1024

// minimalEnv is the only environment hooks receive: no inherited secrets,
// no supervisor internals — just what a well-behaved program needs to run
// at all. On Windows "minimal" still means the platform's base variables:
// PowerShell and the .NET runtime hang or fail obscurely without
// SystemRoot, the temp directories, and the user-profile trio, so those
// pass through (none of them carries secrets).
func minimalEnv() []string {
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		env := []string{
			"SystemRoot=" + root,
			"SystemDrive=" + filepath.VolumeName(root),
			`PATH=` + root + `\System32;` + root + `\System32\WindowsPowerShell\v1.0`,
			"TEMP=" + os.TempDir(),
			"TMP=" + os.TempDir(),
		}
		for _, k := range []string{
			"USERPROFILE", "APPDATA", "LOCALAPPDATA", "ProgramData",
			"ProgramFiles", "PSModulePath", "PATHEXT", "COMSPEC",
			"NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE",
		} {
			if v := os.Getenv(k); v != "" {
				env = append(env, k+"="+v)
			}
		}
		return env
	}
	return []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
}

// Run executes the hook with optional extra arguments appended after the
// manifest-declared ones. It returns trimmed stdout; on failure the error
// carries the exit status and captured stderr.
func Run(ctx context.Context, h *integrationspec.Hook, extraArgs ...string) (string, error) {
	if h == nil {
		return "", fmt.Errorf("hook is not defined")
	}
	timeout := time.Duration(h.Timeout)
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.Executable, append(append([]string{}, h.Args...), extraArgs...)...)
	cmd.Env = minimalEnv()
	// When the timeout kills the hook, grandchild processes it spawned may
	// keep the output pipes open; WaitDelay stops Wait from blocking on
	// them forever. (Full process-group termination is an RT-SEC-002
	// hardening item.)
	cmd.WaitDelay = 2 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &boundedWriter{w: &stdout}
	cmd.Stderr = &boundedWriter{w: &stderr}

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("hook %s timed out after %s", h.Executable, timeout)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("hook %s failed: %w (%s)", h.Executable, err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// boundedWriter keeps the first maxOutput bytes and silently drops the
// rest; the process keeps running (its writes still succeed).
type boundedWriter struct {
	w *bytes.Buffer
}

func (b *boundedWriter) Write(p []byte) (int, error) {
	if remaining := maxOutput - b.w.Len(); remaining > 0 {
		if len(p) > remaining {
			b.w.Write(p[:remaining])
		} else {
			b.w.Write(p)
		}
	}
	return len(p), nil // report full write so the child never sees EPIPE
}

var _ io.Writer = (*boundedWriter)(nil)
