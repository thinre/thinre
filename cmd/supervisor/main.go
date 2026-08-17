// Command supervisor is the Thinre Supervisor: the open-source edge agent
// that runs next to a managed black-box application, receives desired state
// from Thinre Cloud over OpAMP, and reconciles it by executing locally
// defined lifecycle hooks.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	integrationspec "github.com/thinre/thinre/integration-spec"
	"github.com/thinre/thinre/protocol"
	"github.com/thinre/thinre/supervisor"
	"github.com/thinre/thinre/supervisor/enroll"
	"github.com/thinre/thinre/supervisor/identity"
	"github.com/thinre/thinre/supervisor/opamp"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configPath := flag.String("config", supervisor.DefaultConfigPath(), "path of the supervisor configuration file")
	serviceVerb := flag.String("service", "", "Windows service control: install|uninstall|start|stop")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("thinre-supervisor", version)
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if *serviceVerb != "" {
		if err := serviceControl(*serviceVerb, *configPath); err != nil {
			log.Error("service control", "verb", *serviceVerb, "err", err)
			os.Exit(1)
		}
		return
	}

	// Under the Windows service manager the lifecycle (start/stop) comes
	// from SCM instead of signals; runService handles it there.
	if handled, err := runService(log, *configPath); handled {
		if err != nil {
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, log, *configPath); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// app is one managed application: its manifest, its own data-dir layout,
// and (once enrolled) its own runtime identity.
type app struct {
	name     string // runtime display name
	manifest *integrationspec.Integration
	layout   supervisor.Layout
	id       *identity.Identity
}

func run(ctx context.Context, log *slog.Logger, configPath string) error {
	cfg, err := supervisor.LoadConfig(configPath)
	if err != nil {
		return err
	}

	// Fail fast on any broken integration manifest: a Supervisor that
	// cannot execute its lifecycle contract has nothing useful to do.
	apps := make([]*app, 0, len(cfg.Integrations))
	seen := map[string]bool{}
	for _, ref := range cfg.Integrations {
		manifestData, err := os.ReadFile(ref.Manifest)
		if err != nil {
			return fmt.Errorf("read integration manifest: %w", err)
		}
		manifest, err := integrationspec.Parse(manifestData)
		if err != nil {
			return err
		}
		appName := manifest.Metadata.Name
		if seen[appName] {
			return fmt.Errorf("integration %q is listed twice", appName)
		}
		seen[appName] = true

		// Single-app hosts keep the plain host name; multi-app hosts
		// qualify each runtime so names stay unique across the fleet.
		name := ref.Name
		if name == "" {
			name = cfg.Name
			if len(cfg.Integrations) > 1 {
				name = cfg.Name + "/" + appName
			}
		}

		layout := supervisor.NewAppLayout(cfg.DataDir, appName)
		if err := layout.Ensure(); err != nil {
			return err
		}
		id, err := identity.Load(layout.Identity)
		if err != nil {
			return err
		}
		apps = append(apps, &app{name: name, manifest: manifest, layout: layout, id: id})
	}

	if err := enrollMissing(ctx, log, cfg, apps); err != nil {
		return err
	}

	log.Info("thinre-supervisor starting",
		"version", version,
		"applications", len(apps),
		"data_dir", cfg.DataDir,
	)

	// One independent reconcile loop + OpAMP connection per application.
	// The first loop to fail cancels the rest; the process exits so the
	// service manager can restart everything together.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, len(apps))
	for _, a := range apps {
		go func(a *app) {
			appLog := log.With("integration", a.manifest.Metadata.Name)
			err := opamp.Run(runCtx, opamp.Params{
				Layout:            a.layout,
				Log:               appLog,
				OpAMPURL:          cfg.OpAMPURL,
				MachineToken:      a.id.MachineToken,
				RuntimeID:         a.id.RuntimeID,
				SupervisorVersion: version,
				Labels:            cfg.Labels,
				Manifest:          a.manifest,
			})
			errCh <- err
		}(a)
	}

	var firstErr error
	for range apps {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
			cancel()
		}
	}
	log.Info("shutting down")
	return firstErr
}

// enrollMissing exchanges the enrollment token for identities of every
// not-yet-enrolled application — one API call, one single-use token,
// however many applications the host runs.
func enrollMissing(ctx context.Context, log *slog.Logger, cfg *supervisor.Config, apps []*app) error {
	missing := make([]*app, 0, len(apps))
	req := protocol.EnrollRequest{Token: cfg.EnrollmentToken, SupervisorVersion: version}
	for _, a := range apps {
		if a.id != nil {
			log.Info("identity loaded", "integration", a.manifest.Metadata.Name, "runtime_id", a.id.RuntimeID)
			continue
		}
		missing = append(missing, a)
		req.Integrations = append(req.Integrations, protocol.EnrollIntegration{
			IntegrationName: a.manifest.Metadata.Name,
			Name:            a.name,
		})
	}
	if len(missing) == 0 {
		return nil
	}
	if cfg.EnrollmentToken == "" {
		return fmt.Errorf("not enrolled and no enrollment token configured (set enrollment_token or THINRE_ENROLLMENT_TOKEN)")
	}

	resp, err := enroll.Do(ctx, cfg.APIURL, req)
	if err != nil {
		return err
	}
	byIntegration := make(map[string]protocol.EnrolledRuntime, len(resp.Runtimes))
	for _, rt := range resp.Runtimes {
		byIntegration[rt.IntegrationName] = rt
	}
	for _, a := range missing {
		rt, ok := byIntegration[a.manifest.Metadata.Name]
		if !ok {
			return fmt.Errorf("enrollment response is missing integration %q", a.manifest.Metadata.Name)
		}
		newID := identity.Identity{
			RuntimeID:      rt.RuntimeID,
			OrganizationID: resp.OrganizationID,
			MachineToken:   rt.MachineToken,
			EnrolledAt:     time.Now().UTC(),
		}
		if err := identity.Save(a.layout.Identity, newID); err != nil {
			return err
		}
		a.id = &newID
		log.Info("enrolled", "integration", a.manifest.Metadata.Name, "runtime_id", rt.RuntimeID, "organization_id", resp.OrganizationID)
	}
	return nil
}
