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

// artifactPathVar is the template variable manifests may use in hook args
// to receive the verified artifact's local path.
const artifactPathVar = "{{ artifact.path }}"

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
// hooks plus the persisted generation. Hook failures degrade the report
// rather than aborting it — an unreachable hook is itself a signal.
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
	st.Version = r.currentVersion(ctx, local)
	if _, err := hooks.Run(ctx, r.manifest.Health.Check); err == nil {
		st.Health = protocol.HealthHealthy
	} else {
		st.Health = protocol.HealthUnhealthy
	}
	return st
}

// Apply converges toward one desired-state document. It reports every
// phase transition so the cloud's package-status view stays truthful
// (RT-OPAMP-006). Failures are reported and left in place; rollback policy
// arrives with plan step 3.5.
func (r *Reconciler) Apply(ctx context.Context, doc protocol.DesiredState) {
	if doc.Package == nil {
		return
	}
	desired := doc.Package.Version

	local, err := state.Load(r.layout.State)
	if err != nil {
		r.log.Error("load reconcile state", "err", err)
	}
	if local.InFlight != nil {
		// A previous run died mid-operation (RT-SUP-012). Recovery is
		// re-application: downloads are content-addressed (free resume)
		// and the upgrade hook re-runs against the verified artifact.
		r.log.Warn("resuming after interrupted operation",
			"phase", local.InFlight.Phase, "version", local.InFlight.Version)
		local.InFlight = nil
		r.save(local)
	}

	current := r.currentVersion(ctx, local)
	if current == desired {
		// Converged already (or still): remember the generation so stale
		// desired-state replays are recognizable, and say so.
		local.ObservedVersion = current
		local.LastGeneration = doc.Generation
		local.LastStatus = protocol.StatusInstalled
		local.LastMessage = ""
		local.InFlight = nil
		r.save(local)
		r.reportPhase(ctx, doc, current, protocol.StatusInstalled, "")
		return
	}

	r.log.Info("reconciling", "current", current, "desired", desired, "generation", doc.Generation)
	r.reportPhase(ctx, doc, current, protocol.StatusDownloading, "")

	path, err := artifacts.Fetch(ctx, doc.Package.Artifact.URL, doc.Package.Artifact.SHA256, r.layout.Staging, r.layout.Artifacts)
	if err != nil {
		r.log.Error("artifact fetch", "err", err)
		local.LastGeneration = doc.Generation
		local.LastStatus = protocol.StatusFailed
		local.LastMessage = err.Error()
		r.save(local)
		r.reportPhase(ctx, doc, current, protocol.StatusFailed, err.Error())
		return
	}

	// Persist the in-flight marker before mutating the managed software:
	// a crash between here and success must be distinguishable from idle.
	local.InFlight = &state.Operation{
		Generation:   doc.Generation,
		Version:      desired,
		Phase:        protocol.StatusUpgrading,
		ArtifactPath: path,
	}
	r.save(local)
	r.reportPhase(ctx, doc, current, protocol.StatusUpgrading, "")

	if _, err := hooks.Run(ctx, substitute(r.manifest.Package.Upgrade, artifactPathVar, path)); err != nil {
		r.log.Error("upgrade hook", "err", err)
		r.rollback(ctx, doc, &local, "upgrade hook failed: "+err.Error())
		return
	}

	// Trust the black box, verify the outcome: the version hook decides
	// what is actually installed now.
	installed := r.currentVersion(ctx, local)
	healthy := true
	if _, err := hooks.Run(ctx, r.manifest.Health.Check); err != nil {
		healthy = false
	}

	switch {
	case installed != desired:
		r.rollback(ctx, doc, &local,
			fmt.Sprintf("upgrade hook succeeded but installed version is %q, expected %q", installed, desired))
	case !healthy:
		r.rollback(ctx, doc, &local, "health check failed after upgrade")
	default:
		local.ObservedVersion = installed
		local.LastGeneration = doc.Generation
		local.LastStatus = protocol.StatusInstalled
		local.LastMessage = ""
		local.InFlight = nil
		r.save(local)
		r.log.Info("reconciled", "version", installed, "generation", doc.Generation)
		r.reportPhase(ctx, doc, installed, protocol.StatusInstalled, "")
	}
}

// rollback restores the previous version after a failed upgrade
// (RT-SUP-009): run the rollback hook if the integration defines one,
// verify the outcome, and report rolled-back — or failed when no rollback
// exists or the rollback itself fails. The desired-state generation is
// recorded either way, so the cloud can tell this outcome belongs to the
// current desired state.
func (r *Reconciler) rollback(ctx context.Context, doc protocol.DesiredState, local *state.Local, reason string) {
	finish := func(status, version, message string) {
		local.ObservedVersion = version
		local.LastGeneration = doc.Generation
		local.LastStatus = status
		local.LastMessage = message
		local.InFlight = nil
		r.save(*local)
		r.reportPhase(ctx, doc, version, status, message)
	}

	if r.manifest.Package.Rollback == nil {
		finish(protocol.StatusFailed, r.currentVersion(ctx, *local), reason+" (no rollback hook defined)")
		return
	}

	r.log.Warn("rolling back", "reason", reason)
	if _, err := hooks.Run(ctx, r.manifest.Package.Rollback); err != nil {
		r.log.Error("rollback hook", "err", err)
		finish(protocol.StatusFailed, r.currentVersion(ctx, *local), reason+"; rollback also failed: "+err.Error())
		return
	}
	restored := r.currentVersion(ctx, *local)
	r.log.Info("rolled back", "version", restored, "reason", reason)
	finish(protocol.StatusRolledBack, restored, reason)
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
// terminal phases (installed/failed) — mid-flight health is noise while
// the software is being replaced.
func (r *Reconciler) reportPhase(ctx context.Context, doc protocol.DesiredState, version, status, message string) {
	st := protocol.ObservedState{
		SchemaVersion: protocol.SchemaVersion,
		Generation:    doc.Generation,
		Version:       version,
		Health:        protocol.HealthUnknown,
		Status:        status,
		Message:       message,
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
