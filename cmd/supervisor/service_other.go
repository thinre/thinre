//go:build !windows

package main

import (
	"fmt"
	"log/slog"
)

// runService is Windows-only; other platforms always run in the console
// (service managers like systemd supervise the plain process).
func runService(*slog.Logger, string) (bool, error) { return false, nil }

// serviceControl is Windows-only.
func serviceControl(string, string) error {
	return fmt.Errorf("-service is only supported on Windows; use your service manager (systemd) instead")
}
