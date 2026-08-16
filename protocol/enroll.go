package protocol

// EnrollPath is the cloud API endpoint a Supervisor calls exactly once to
// exchange its enrollment token for machine identities.
const EnrollPath = "/api/v1/enroll"

// MachineTokenHeader carries the Supervisor's machine token on the OpAMP
// connection and on authenticated machine-to-cloud HTTP calls.
const MachineTokenHeader = "X-Thinre-Machine-Token"

// EnrollRequest is the one-time enrollment exchange. The token is
// organization-scoped, expiring, and single-use; one successful exchange
// invalidates it and enrolls every listed integration at once — a host
// running several managed applications enrolls them all with one token.
type EnrollRequest struct {
	Token string `json:"token"`
	// SupervisorVersion is reported for fleet visibility.
	SupervisorVersion string `json:"supervisor_version"`
	// Integrations lists the applications this Supervisor manages; each
	// becomes its own runtime.
	Integrations []EnrollIntegration `json:"integrations"`
}

// EnrollIntegration names one application to enroll.
type EnrollIntegration struct {
	// IntegrationName selects one of the organization's integrations.
	IntegrationName string `json:"integration_name"`
	// Name is the runtime's display name (typically the hostname, or
	// "<hostname>/<app>" on multi-application hosts).
	Name string `json:"name"`
}

// EnrollResponse binds the Supervisor to exactly one organization, with
// one runtime identity per requested integration. Machine tokens are
// shown once here and stored hashed by the cloud; the Supervisor persists
// them locally with 0600 permissions.
type EnrollResponse struct {
	OrganizationID string            `json:"organization_id"`
	Runtimes       []EnrolledRuntime `json:"runtimes"`
}

// EnrolledRuntime is one runtime identity created by enrollment.
type EnrolledRuntime struct {
	IntegrationName string `json:"integration_name"`
	RuntimeID       string `json:"runtime_id"`
	MachineToken    string `json:"machine_token"`
}
