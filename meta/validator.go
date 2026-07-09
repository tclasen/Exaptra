package meta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ErrorCategory classifies meta validation failures.
type ErrorCategory string

const (
	ErrorCategoryValidation ErrorCategory = "validation"
	ErrorCategoryPermission ErrorCategory = "permission"
)

// Error is the structured error returned by the meta validator and store.
type Error struct {
	Category ErrorCategory `json:"category"`
	Op       string        `json:"op,omitempty"`
	Message  string        `json:"message"`
	Err      error         `json:"-"`
}

func (e *Error) Error() string {
	switch {
	case e == nil:
		return "<nil>"
	case e.Op != "" && e.Err != nil:
		return fmt.Sprintf("%s %s: %s: %v", e.Category, e.Op, e.Message, e.Err)
	case e.Op != "":
		return fmt.Sprintf("%s %s: %s", e.Category, e.Op, e.Message)
	case e.Err != nil:
		return fmt.Sprintf("%s: %s: %v", e.Category, e.Message, e.Err)
	default:
		return fmt.Sprintf("%s: %s", e.Category, e.Message)
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// AsError extracts a structured meta error when possible.
func AsError(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

func newError(category ErrorCategory, op, message string, err error) error {
	return &Error{
		Category: category,
		Op:       op,
		Message:  message,
		Err:      err,
	}
}

// Validator checks whether a meta-tool transition may be applied.
type Validator struct {
	AllowedOperations map[string]bool
}

// NewValidator creates a validator that allows the supplied operations.
func NewValidator(allowedOperations ...string) *Validator {
	allowed := make(map[string]bool, len(allowedOperations))
	for _, operation := range allowedOperations {
		allowed[operation] = true
	}
	return &Validator{AllowedOperations: allowed}
}

// Validate verifies permissions and runtime invariants before a transition is applied.
func (v *Validator) Validate(request Request, before, after json.RawMessage) (Transition, ValidationOutcome, error) {
	if request.Type != MetaToolRequestType {
		return Transition{}, ValidationOutcome{Allowed: false, Reason: "invalid request type"}, newError(ErrorCategoryValidation, "validate", "meta tool request type is required", nil)
	}
	if request.Operation == "" {
		return Transition{}, ValidationOutcome{Allowed: false, Reason: "missing operation"}, newError(ErrorCategoryValidation, "validate", "meta tool operation is required", nil)
	}
	if request.Target == "" {
		return Transition{}, ValidationOutcome{Allowed: false, Reason: "missing target"}, newError(ErrorCategoryValidation, "validate", "meta tool target is required", nil)
	}
	if len(before) != 0 {
		if _, err := normalizeJSON(before); err != nil {
			return Transition{}, ValidationOutcome{Allowed: false, Reason: "invalid before state"}, newError(ErrorCategoryValidation, "validate", "invalid before state", err)
		}
	}
	if len(after) != 0 {
		if _, err := normalizeJSON(after); err != nil {
			return Transition{}, ValidationOutcome{Allowed: false, Reason: "invalid after state"}, newError(ErrorCategoryValidation, "validate", "invalid after state", err)
		}
	}

	allowed := v == nil || v.AllowedOperations[request.Operation]
	outcome := ValidationOutcome{
		Allowed: allowed,
	}
	if allowed {
		outcome.Reason = "authorized"
	} else {
		outcome.Reason = "operation not authorized"
	}

	transition := Transition{
		Type:       MetaTransitionType,
		Operation:  request.Operation,
		Target:     request.Target,
		Before:     cloneJSON(before),
		After:      cloneJSON(after),
		Validation: outcome,
		Applied:    allowed,
	}
	if allowed {
		transition.Result = "applied"
	} else {
		transition.Result = "denied"
	}

	if !allowed {
		return transition, outcome, newError(ErrorCategoryPermission, "validate", fmt.Sprintf("operation %q is not authorized", request.Operation), nil)
	}

	return transition, outcome, nil
}

// Store applies validated transitions atomically where practical and keeps an audit log.
type Store struct {
	mu        sync.Mutex
	state     json.RawMessage
	audits    []AuditRecord
	validator *Validator
}

// NewStore creates a meta state store with the supplied initial state.
func NewStore(validator *Validator, initial json.RawMessage) *Store {
	return &Store{
		state:     cloneJSON(initial),
		validator: validator,
	}
}

// State returns the current normalized state.
func (s *Store) State() json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneJSON(s.state)
}

// Audits returns the recorded audit trail.
func (s *Store) Audits() []AuditRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuditRecord, len(s.audits))
	copy(out, s.audits)
	return out
}

// Apply validates the request and either updates the state or records a denied transition.
func (s *Store) Apply(request Request, after json.RawMessage) (AuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := cloneJSON(s.state)
	transition, outcome, err := s.validator.Validate(request, before, after)
	if err != nil {
		if transition.Type == "" {
			transition = Transition{
				Type:       MetaTransitionType,
				Operation:  request.Operation,
				Target:     request.Target,
				Before:     before,
				After:      cloneJSON(after),
				Validation: outcome,
				Applied:    false,
				Result:     "denied",
			}
		}
		audit := AuditRecord{
			Type:        MetaAuditRecordType,
			Request:     request,
			Transition:  transition,
			Validation:  outcome,
			FinalResult: "denied",
		}
		s.audits = append(s.audits, audit)
		return audit, err
	}

	s.state = cloneJSON(after)
	audit := AuditRecord{
		Type:        MetaAuditRecordType,
		Request:     request,
		Transition:  transition,
		Validation:  outcome,
		FinalResult: "applied",
	}
	s.audits = append(s.audits, audit)
	return audit, nil
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}

func normalizeJSON(raw json.RawMessage) (json.RawMessage, error) {
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}
