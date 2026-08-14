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
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	configPath := flag.String("config", supervisor.DefaultConfigPath, "path of the supervisor configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("thinre-supervisor", version)
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log, *configPath); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, configPath string) error {
	cfg, err := supervisor.LoadConfig(configPath)
	if err != nil {
		return err
	}

	layout := supervisor.NewLayout(cfg.DataDir)
	if err := layout.Ensure(); err != nil {
		return err
	}

	// Fail fast on a broken integration manifest: a Supervisor that cannot
	// execute its lifecycle contract has nothing useful to do.
	manifestData, err := os.ReadFile(cfg.IntegrationManifest)
	if err != nil {
		return fmt.Errorf("read integration manifest: %w", err)
	}
	manifest, err := integrationspec.Parse(manifestData)
	if err != nil {
		return err
	}

	id, err := identity.Load(layout.Identity)
	if err != nil {
		return err
	}

	log.Info("thinre-supervisor starting",
		"version", version,
		"integration", manifest.Metadata.Name,
		"data_dir", cfg.DataDir,
		"enrolled", id != nil,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if id == nil {
		if cfg.EnrollmentToken == "" {
			return fmt.Errorf("not enrolled and no enrollment token configured (set enrollment_token or THINRE_ENROLLMENT_TOKEN)")
		}
		resp, err := enroll.Do(ctx, cfg.APIURL, protocol.EnrollRequest{
			Token:             cfg.EnrollmentToken,
			Name:              cfg.Name,
			IntegrationName:   manifest.Metadata.Name,
			SupervisorVersion: version,
		})
		if err != nil {
			return err
		}
		newID := identity.Identity{
			RuntimeID:      resp.RuntimeID,
			OrganizationID: resp.OrganizationID,
			MachineToken:   resp.MachineToken,
			EnrolledAt:     time.Now().UTC(),
		}
		if err := identity.Save(layout.Identity, newID); err != nil {
			return err
		}
		id = &newID
		log.Info("enrolled", "runtime_id", id.RuntimeID, "organization_id", id.OrganizationID)
	} else {
		log.Info("identity loaded", "runtime_id", id.RuntimeID, "organization_id", id.OrganizationID)
	}

	// The OpAMP connection loop (step 2.5) replaces this wait.
	<-ctx.Done()
	log.Info("shutting down")
	return nil
}
