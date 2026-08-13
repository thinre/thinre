// Package protocol defines the wire contracts shared between the Thinre
// Supervisor and Thinre Cloud: enrollment request/response payloads and the
// desired-state document carried over OpAMP.
//
// This package is the single source of truth for these contracts. Thinre
// Cloud imports it; it must never depend on anything cloud-side.
package protocol

// SchemaVersion identifies the current version of the Thinre wire contracts
// defined by this package. It is embedded in exchanged documents so both
// sides can detect incompatible peers.
const SchemaVersion = "v1"
