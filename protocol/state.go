package protocol

// Transport mapping (decision G6): desired state travels as ONE JSON
// document — this file's DesiredState — carried in the OpAMP
// AgentRemoteConfig config map under RemoteConfigKey. One document keeps
// package version and configuration revision atomic, which the bundle
// consistency rule requires. The Supervisor acknowledges via the standard
// OpAMP RemoteConfigStatus and reports rich observed state as a custom
// message of type ObservedStateMessageType.

// RemoteConfigKey is the OpAMP config-map key holding the desired-state
// document.
const RemoteConfigKey = "thinre-desired-state"

// ObservedStateMessageType identifies the Supervisor's observed-state
// report in OpAMP custom messages.
const ObservedStateMessageType = "thinre.observed-state"

// CustomCapability is the OpAMP custom capability string both sides
// announce to negotiate Thinre's message exchange.
const CustomCapability = "io.thinre.supervisor"

// DesiredState is what the cloud wants the runtime to become.
type DesiredState struct {
	SchemaVersion string `json:"schema_version"`
	// Generation increments on every desired-state mutation; the
	// Supervisor echoes it in ObservedState so the cloud can tell current
	// reports from stale ones (RT-STATE-002).
	Generation int64 `json:"generation"`
	// Package is the desired software version, when one is set.
	Package *DesiredPackage `json:"package,omitempty"`
	// ConfigRevision is the desired configuration bundle revision.
	// Bundles arrive with milestone M4; the field exists now so adding
	// them does not change the document shape.
	ConfigRevision *int64 `json:"config_revision,omitempty"`
}

// DesiredPackage names a version and where to get it.
type DesiredPackage struct {
	Version  string   `json:"version"`
	Artifact Artifact `json:"artifact"`
}

// Artifact tells the Supervisor how to download and verify a package. For
// External Artifacts the URL is vendor infrastructure; for Managed
// Artifacts it is a short-lived signed URL minted by the cloud. The
// Supervisor treats both identically: download, verify, never execute
// anything that fails verification.
type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// Health values a Supervisor may report.
const (
	HealthHealthy   = "healthy"
	HealthUnhealthy = "unhealthy"
	HealthUnknown   = "unknown"
)

// Package status values a Supervisor may report (RT-OPAMP-006).
const (
	StatusIdle        = "idle"
	StatusDownloading = "downloading"
	StatusVerifying   = "verifying"
	StatusUpgrading   = "upgrading"
	StatusInstalled   = "installed"
	StatusFailed      = "failed"
	StatusRolledBack  = "rolled-back"
)

// ObservedState is what the Supervisor reports as currently true.
type ObservedState struct {
	SchemaVersion string `json:"schema_version"`
	// Generation is the desired-state generation this report responds to;
	// 0 means "no desired state received yet".
	Generation int64 `json:"generation"`
	// Version is the currently installed software version, as learned
	// from the integration's version hook.
	Version        string `json:"version,omitempty"`
	ConfigRevision *int64 `json:"config_revision,omitempty"`
	Health         string `json:"health"`
	Status         string `json:"status"`
	// Message carries a bounded human-readable detail for failures.
	Message string `json:"message,omitempty"`
}
