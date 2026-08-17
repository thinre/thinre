//go:build windows

package main

// Windows service (SCM) integration: the supervisor runs under the
// service manager with Stop/Shutdown mapped onto context cancellation,
// and -service install|uninstall|start|stop manages the registration.
// Fatal errors in service mode go to the Windows event log, because a
// service has no console for them.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "thinre-supervisor"

// runService runs under SCM when the process was started by it; handled
// is false in a normal console session.
func runService(log *slog.Logger, configPath string) (handled bool, err error) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, err
	}
	err = svc.Run(serviceName, &serviceHandler{log: log, configPath: configPath})
	return true, err
}

type serviceHandler struct {
	log        *slog.Logger
	configPath string
}

// Execute is the SCM callback: it runs the supervisor until SCM asks for
// Stop or Shutdown, which cancel the context exactly like SIGTERM does in
// a console session.
func (h *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- run(ctx, h.log, h.configPath) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				<-done
				return false, 0
			}
		case err := <-done:
			// The supervisor exited on its own — an error, since nothing
			// asked it to stop. Event log is the only visible channel.
			if err != nil {
				h.log.Error("fatal", "err", err)
				logServiceError(fmt.Sprintf("thinre-supervisor failed: %v", err))
				return false, 1
			}
			return false, 0
		}
	}
}

// logServiceError writes to the Windows event log, best-effort.
func logServiceError(msg string) {
	el, err := eventlog.Open(serviceName)
	if err != nil {
		return
	}
	defer func() { _ = el.Close() }()
	_ = el.Error(1, msg)
}

// serviceControl implements -service install|uninstall|start|stop.
func serviceControl(verb, configPath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to the service manager (administrator required): %w", err)
	}
	defer func() { _ = m.Disconnect() }()

	switch verb {
	case "install":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		s, err := m.CreateService(serviceName, exe, mgr.Config{
			DisplayName: "Thinre Supervisor",
			Description: "Thinre edge supervisor: reconciles managed applications against their desired state.",
			StartType:   mgr.StartAutomatic,
		}, "-config", configPath)
		if err != nil {
			return fmt.Errorf("create service: %w", err)
		}
		defer s.Close()
		// The event source lets service-mode fatals reach the event log.
		if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
			fmt.Fprintf(os.Stderr, "warning: event log source not installed: %v\n", err)
		}
		fmt.Printf("installed service %s (config %s)\n", serviceName, configPath)
		return nil

	case "uninstall":
		s, err := m.OpenService(serviceName)
		if err != nil {
			return fmt.Errorf("open service: %w", err)
		}
		defer s.Close()
		if err := s.Delete(); err != nil {
			return fmt.Errorf("delete service: %w", err)
		}
		_ = eventlog.Remove(serviceName)
		fmt.Printf("uninstalled service %s\n", serviceName)
		return nil

	case "start":
		s, err := m.OpenService(serviceName)
		if err != nil {
			return fmt.Errorf("open service: %w", err)
		}
		defer s.Close()
		if err := s.Start(); err != nil {
			return fmt.Errorf("start service: %w", err)
		}
		fmt.Printf("started service %s\n", serviceName)
		return nil

	case "stop":
		s, err := m.OpenService(serviceName)
		if err != nil {
			return fmt.Errorf("open service: %w", err)
		}
		defer s.Close()
		st, err := s.Control(svc.Stop)
		if err != nil {
			return fmt.Errorf("stop service: %w", err)
		}
		// Wait briefly for the stop to complete so the verb is truthful.
		for i := 0; i < 20 && st.State != svc.Stopped; i++ {
			time.Sleep(500 * time.Millisecond)
			if st, err = s.Query(); err != nil {
				return err
			}
		}
		fmt.Printf("stopped service %s\n", serviceName)
		return nil

	default:
		return fmt.Errorf("unknown -service verb %q (install|uninstall|start|stop)", verb)
	}
}
