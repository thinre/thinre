// Package reconcile implements the Supervisor's core loop: compare desired
// state against observed state and converge by executing the integration's
// lifecycle hooks. The cloud decides WHAT; this package decides HOW
// (RT-SUP-008). It never interprets the managed software — everything
// software-specific goes through the manifest's hooks.
package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	integrationspec "github.com/thinre/thinre/integration-spec"
	"github.com/thinre/thinre/protocol"
	"github.com/thinre/thinre/supervisor"
	"github.com/thinre/thinre/supervisor/artifacts"
	"github.com/thinre/thinre/supervisor/hooks"
	"github.com/thinre/thinre/supervisor/state"
)

// artifactPathVar and bundlePathVar are the template variables manifests
// may use in hook args: the verified artifact's path (upgrade hook) and
// the staged bundle directory (validate hook).
const (
	artifactPathVar = "{{ artifact.path }}"
	bundlePathVar   = "{{ bundle.path }}"
)

// Report delivers an observed-state document to the cloud. Implemented by
// the transport (the opamp package); reconciliation never talks wire
// protocols itself.
type Report func(ctx context.Context, st protocol.ObservedState)

// Reconciler converges one runtime toward desired state.
type Reconciler struct {
	log      *slog.Logger
	manifest *integrationspec.Integration
	layout   supervisor.Layout
	report   Report
}

// New builds a reconciler.
func New(log *slog.Logger, manifest *integrationspec.Integration, layout supervisor.Layout, report Report) *Reconciler {
	return &Reconciler{log: log, manifest: manifest, layout: layout, report: report}
}

// Observe builds the current observed state from the version and health
// hooks plus the persisted generation and configuration revision. Hook
// failures degrade the report rather than aborting it — an unreachable
// hook is itself a signal.
func (r *Reconciler) Observe(ctx context.Context) protocol.ObservedState {
	local, err := state.Load(r.layout.State)
	if err != nil {
		r.log.Error("load reconcile state", "err", err)
	}
	st := protocol.ObservedState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    local.LastGeneration,
		Health:        protocol.HealthUnknown,
		Status:        protocol.StatusIdle,
		Message:       local.LastMessage,
	}
	// Keep showing the last completed outcome; "idle" is only for a
	// supervisor that has not applied anything yet.
	if local.LastStatus != "" {
		st.Status = local.LastStatus
	}
	if local.ConfigRevision > 0 {
		rev := local.ConfigRevision
		st.ConfigRevision = &rev
	}
	st.Version = r.currentVersion(ctx, local)
	if _, err := hooks.Run(ctx, r.manifest.Health.Check); err == nil {
		st.Health = protocol.HealthHealthy
	} else {
		st.Health = protocol.HealthUnhealthy
	}
	return st
}

// Apply converges toward one desired-state document: the package part
// first, then the configuration bundle — software in a failed state is
// never reconfigured. Every phase transition is reported (RT-OPAMP-006/7);
// the single terminal "installed" report covers both parts.
func (r *Reconciler) Apply(ctx context.Context, doc protocol.DesiredState) {
	if doc.Package == nil && doc.Bundle == nil {
		return
	}

	local, err := state.Load(r.layout.State)
	if err != nil {
		r.log.Error("load reconcile state", "err", err)
	}
	if local.InFlight != nil {
		// A previous run died mid-operation (RT-SUP-012). Recovery is
		// re-application: downloads are content-addressed (free resume)
		// and hooks re-run against verified inputs.
		r.log.Warn("resuming after interrupted operation",
			"phase", local.InFlight.Phase, "version", local.InFlight.Version)
		local.InFlight = nil
		r.save(local)
	}

	if doc.Package != nil {
		if !r.applyPackage(ctx, doc, &local) {
			return // terminal failure/rollback already reported
		}
	}
	if doc.Bundle != nil {
		if !r.applyBundle(ctx, doc, &local) {
			return // terminal failure already reported
		}
	}

	local.LastGeneration = doc.Generation
	local.LastStatus = protocol.StatusInstalled
	local.LastMessage = ""
	local.InFlight = nil
	r.save(local)
	r.log.Info("reconciled", "version", local.ObservedVersion, "config_revision", local.ConfigRevision, "generation", doc.Generation)
	r.reportPhase(ctx, doc.Generation, r.currentVersion(ctx, local), local.ConfigRevision, protocol.StatusInstalled, "")
}

// applyPackage converges the software version; true means converged.
// Failure and rollback paths report terminally themselves.
func (r *Reconciler) applyPackage(ctx context.Context, doc protocol.DesiredState, local *state.Local) bool {
	desired := doc.Package.Version

	current := r.currentVersion(ctx, *local)
	if current == desired {
		local.ObservedVersion = current
		return true
	}

	r.log.Info("reconciling package", "current", current, "desired", desired, "generation", doc.Generation)
	r.reportPhase(ctx, doc.Generation, current, local.ConfigRevision, protocol.StatusDownloading, "")

	path, err := artifacts.Fetch(ctx, doc.Package.Artifact.URL, doc.Package.Artifact.SHA256, r.layout.Staging, r.layout.Artifacts)
	if err != nil {
		r.log.Error("artifact fetch", "err", err)
		r.finish(ctx, doc, local, protocol.StatusFailed, current, err.Error())
		return false
	}

	// Persist the in-flight marker before mutating the managed software:
	// a crash between here and success must be distinguishable from idle.
	local.InFlight = &state.Operation{
		Generation:   doc.Generation,
		Version:      desired,
		Phase:        protocol.StatusUpgrading,
		ArtifactPath: path,
	}
	r.save(*local)
	r.reportPhase(ctx, doc.Generation, current, local.ConfigRevision, protocol.StatusUpgrading, "")

	if _, err := hooks.Run(ctx, substitute(r.manifest.Package.Upgrade, artifactPathVar, path)); err != nil {
		r.log.Error("upgrade hook", "err", err)
		r.rollback(ctx, doc, local, "upgrade hook failed: "+err.Error())
		return false
	}

	// Trust the black box, verify the outcome: the version hook decides
	// what is actually installed now.
	installed := r.currentVersion(ctx, *local)
	healthy := true
	if _, err := hooks.Run(ctx, r.manifest.Health.Check); err != nil {
		healthy = false
	}

	switch {
	case installed != desired:
		r.rollback(ctx, doc, local,
			fmt.Sprintf("upgrade hook succeeded but installed version is %q, expected %q", installed, desired))
		return false
	case !healthy:
		r.rollback(ctx, doc, local, "health check failed after upgrade")
		return false
	}
	local.ObservedVersion = installed
	r.save(*local)
	return true
}

// rollback restores the previous version after a failed upgrade
// (RT-SUP-009): run the rollback hook if the integration defines one,
// verify the outcome, and report rolled-back — or failed when no rollback
// exists or the rollback itself fails. The desired-state generation is
// recorded either way, so the cloud can tell this outcome belongs to the
// current desired state.
func (r *Reconciler) rollback(ctx context.Context, doc protocol.DesiredState, local *state.Local, reason string) {
	if r.manifest.Package.Rollback == nil {
		r.finish(ctx, doc, local, protocol.StatusFailed, r.currentVersion(ctx, *local), reason+" (no rollback hook defined)")
		return
	}

	r.log.Warn("rolling back", "reason", reason)
	if _, err := hooks.Run(ctx, r.manifest.Package.Rollback); err != nil {
		r.log.Error("rollback hook", "err", err)
		r.finish(ctx, doc, local, protocol.StatusFailed, r.currentVersion(ctx, *local), reason+"; rollback also failed: "+err.Error())
		return
	}
	restored := r.currentVersion(ctx, *local)
	r.log.Info("rolled back", "version", restored, "reason", reason)
	r.finish(ctx, doc, local, protocol.StatusRolledBack, restored, reason)
}

// finish records and reports a terminal outcome.
func (r *Reconciler) finish(ctx context.Context, doc protocol.DesiredState, local *state.Local, status, version, message string) {
	local.ObservedVersion = version
	local.LastGeneration = doc.Generation
	local.LastStatus = status
	local.LastMessage = message
	local.InFlight = nil
	r.save(*local)
	r.reportPhase(ctx, doc.Generation, version, local.ConfigRevision, status, message)
}

// currentVersion asks the integration's version hook; without one (or on
// failure) it falls back to the version recorded at the last successful
// supervisor-driven upgrade.
func (r *Reconciler) currentVersion(ctx context.Context, local state.Local) string {
	if r.manifest.Package.Version != nil {
		if out, err := hooks.Run(ctx, r.manifest.Package.Version); err == nil {
			return out
		}
		r.log.Warn("version hook failed; using last recorded version")
	}
	return local.ObservedVersion
}

// reportPhase sends one phase transition, running the health hook only for
// terminal phases — mid-flight health is noise while the software or its
// configuration is being replaced.
func (r *Reconciler) reportPhase(ctx context.Context, generation int64, version string, configRevision int64, status, message string) {
	st := protocol.ObservedState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    generation,
		Version:       version,
		Health:        protocol.HealthUnknown,
		Status:        status,
		Message:       message,
	}
	if configRevision > 0 {
		rev := configRevision
		st.ConfigRevision = &rev
	}
	if status == protocol.StatusInstalled || status == protocol.StatusFailed || status == protocol.StatusRolledBack {
		if _, err := hooks.Run(ctx, r.manifest.Health.Check); err == nil {
			st.Health = protocol.HealthHealthy
		} else {
			st.Health = protocol.HealthUnhealthy
		}
	}
	r.report(ctx, st)
}

func (r *Reconciler) save(l state.Local) {
	if err := state.Save(r.layout.State, l); err != nil {
		r.log.Error("save reconcile state", "err", err)
	}
}

// substitute returns a copy of the hook with the template variable replaced
// in every argument. The executable itself is never templated.
func substitute(h *integrationspec.Hook, variable, value string) *integrationspec.Hook {
	if h == nil {
		return nil
	}
	out := *h
	out.Args = make([]string, len(h.Args))
	for i, a := range h.Args {
		out.Args[i] = strings.ReplaceAll(a, variable, value)
	}
	return &out
}
