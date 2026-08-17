package protocol

// Thinre Link is the supervisor↔cloud protocol: a single outbound
// WebSocket carrying JSON messages. It is deliberately tiny — the whole
// exchange is three message types:
//
//	hello     client→server, once per connection: who this runtime is
//	          (identification, labels, supervisor version) and the last
//	          desired-state generation it applied.
//	state     server→client: the full desired-state document. Sent when
//	          the hello's applied generation differs from the current
//	          one, and again on every change while connected. The client
//	          applies latest-wins.
//	observed  client→server: the observed-state document (with its
//	          generation echo). Sent whenever reconciliation progresses
//	          and at least every 30 seconds, which doubles as liveness.
//
// Framing: every WebSocket text message is one LinkEnvelope. Receivers
// MUST ignore envelopes whose Type they do not know — that is how the
// protocol grows without breaking older supervisors. Authentication is
// the machine token, sent as the MachineTokenHeader on the HTTP upgrade
// request; the connection is always dialed outbound by the supervisor.

// LinkPath is the gateway's WebSocket endpoint.
const LinkPath = "/v1/link"

// LinkVersion is the protocol version announced in hello. Additive
// changes (new envelope types, new optional fields) do not bump it;
// incompatible ones do.
const LinkVersion = 1

// Link envelope types.
const (
	LinkTypeHello    = "hello"
	LinkTypeState    = "state"
	LinkTypeObserved = "observed"
)

// LinkEnvelope frames every Link message: Type selects which single
// payload field is set. Unknown types must be ignored.
type LinkEnvelope struct {
	Type     string         `json:"type"`
	Hello    *LinkHello     `json:"hello,omitempty"`
	State    *DesiredState  `json:"state,omitempty"`
	Observed *ObservedState `json:"observed,omitempty"`
}

// LinkHello introduces the runtime once per connection.
type LinkHello struct {
	LinkVersion       int    `json:"link_version"`
	SupervisorVersion string `json:"supervisor_version"`
	// Integration is the manifest's metadata.name.
	Integration string `json:"integration"`
	// Host identification, best-effort (shown in the console).
	Hostname string `json:"hostname,omitempty"`
	IP       string `json:"ip,omitempty"`
	OS       string `json:"os,omitempty"`
	Arch     string `json:"arch,omitempty"`
	// Labels are the operator-defined tags from the configuration.
	Labels map[string]string `json:"labels,omitempty"`
	// AppliedGeneration is the last desired-state generation this
	// runtime applied (0 = none): the server sends state immediately
	// when it differs from the current generation.
	AppliedGeneration int64 `json:"applied_generation"`
}
