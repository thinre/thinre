package protocol

// EnrollPath is the cloud API endpoint a Supervisor calls exactly once to
// exchange its enrollment token for a machine identity.
const EnrollPath = "/api/v1/enroll"

// MachineTokenHeader carries the Supervisor's machine token on the OpAMP
// connection and on authenticated machine-to-cloud HTTP calls.
const MachineTokenHeader = "X-Thinre-Machine-Token"

// EnrollRequest is the one-time enrollment exchange. The token is
// organization-scoped, expiring, and single-use; a successful exchange
// invalidates it.
type EnrollRequest struct {
	Token string `json:"token"`
	// Name is the runtime's display name (typically the hostname).
	Name string `json:"name"`
	// IntegrationName selects which of the organization's integrations
	// this Supervisor manages.
	IntegrationName string `json:"integration_name"`
	// SupervisorVersion is reported for fleet visibility.
	SupervisorVersion string `json:"supervisor_version"`
}

// EnrollResponse binds the Supervisor to exactly one organization. The
// machine token is shown once here and stored hashed by the cloud; the
// Supervisor persists it locally with 0600 permissions.
type EnrollResponse struct {
	RuntimeID      string `json:"runtime_id"`
	OrganizationID string `json:"organization_id"`
	MachineToken   string `json:"machine_token"`
}
