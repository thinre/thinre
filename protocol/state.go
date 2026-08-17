package protocol

// Transport mapping (decision G6): desired state travels as ONE JSON
// document — this file's DesiredState — carried in a Link `state`
// envelope (see link.go). One document keeps package version and
// configuration revision atomic, which the bundle consistency rule
// requires. The Supervisor reports back with ObservedState, whose
// Generation echo tells the cloud which desired state a report is about.

// DesiredState is what the cloud wants the runtime to become. Package and
// bundle travel in ONE document on purpose: the bundle-consistency rule
// (architecture §10) needs version and configuration to be atomic.
type DesiredState struct {
	SchemaVersion string `json:"schema_version"`
	// Generation increments on every desired-state mutation; the
	// Supervisor echoes it in ObservedState so the cloud can tell current
	// reports from stale ones (RT-STATE-002).
	Generation int64 `json:"generation"`
	// Package is the desired software version, when one is set.
	Package *DesiredPackage `json:"package,omitempty"`
	// Bundle is the desired configuration bundle, when one is set. File
	// contents are embedded: configuration is small, and embedding keeps
	// the complete revision atomic — the Supervisor never fetches parts.
	Bundle *DesiredBundle `json:"bundle,omitempty"`
}

// DesiredBundle is one complete configuration revision (RT-CONFIG-003).
type DesiredBundle struct {
	Revision int64 `json:"revision"`
	// ManifestHash is the SHA-256 over the sorted (file id, sha256) pairs:
	// the identity of the complete set.
	ManifestHash string       `json:"manifest_hash"`
	Files        []BundleFile `json:"files"`
}

// BundleFile carries one configuration file. Content is raw bytes
// (base64 on the wire via JSON encoding).
type BundleFile struct {
	// ID is the stable file identifier from the Integration manifest.
	ID string `json:"id"`
	// Destination is the absolute path the file belongs at.
	Destination string `json:"destination"`
	SHA256      string `json:"sha256"`
	Content     []byte `json:"content"`
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

// Package status values a Supervisor may report (RT-LINK-006), plus the
// configuration phases (RT-LINK-007).
const (
	StatusIdle        = "idle"
	StatusDownloading = "downloading"
	StatusVerifying   = "verifying"
	StatusUpgrading   = "upgrading"
	StatusInstalled   = "installed"
	StatusFailed      = "failed"
	StatusRolledBack  = "rolled-back"
	StatusStaging     = "staging"
	StatusApplying    = "applying"
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
