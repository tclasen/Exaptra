package meta

import "encoding/json"

const (
	MetaToolRequestType = "exaptra:meta_tool_request"
	MetaTransitionType  = "exaptra:meta_transition"
	MetaAuditRecordType = "exaptra:meta_audit"
)

// Identity identifies a caller or provider involved in a meta-tool action.
type Identity struct {
	Name  string `json:"name"`
	Index int    `json:"index,omitempty"`
}

// ValidationOutcome captures whether a meta-tool operation was authorized.
type ValidationOutcome struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Request describes a meta-tool operation distinct from ordinary tool calls.
type Request struct {
	Type      string          `json:"type"`
	Operation string          `json:"operation"`
	Caller    Identity        `json:"caller"`
	Provider  Identity        `json:"provider"`
	Target    string          `json:"target"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// Transition records the requested state change and its applied result.
type Transition struct {
	Type       string            `json:"type"`
	Operation  string            `json:"operation"`
	Target     string            `json:"target"`
	Before     json.RawMessage   `json:"before,omitempty"`
	After      json.RawMessage   `json:"after,omitempty"`
	Validation ValidationOutcome `json:"validation"`
	Applied    bool              `json:"applied"`
	Result     string            `json:"result,omitempty"`
}

// AuditRecord captures the full meta-tool audit trail.
type AuditRecord struct {
	Type        string            `json:"type"`
	Request     Request           `json:"request"`
	Transition  Transition        `json:"transition"`
	Validation  ValidationOutcome `json:"validation"`
	FinalResult string            `json:"final_result,omitempty"`
}
