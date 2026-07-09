package meta

import (
	"encoding/json"
	"testing"
)

func TestStoreAppliesAuthorizedTransitionsAndRecordsAudit(t *testing.T) {
	store := NewStore(NewValidator("compact"), json.RawMessage(`{"items":4}`))
	request := Request{
		Type:      MetaToolRequestType,
		Operation: "compact",
		Caller:    Identity{Name: "agent", Index: 1},
		Provider:  Identity{Name: "mcp", Index: 2},
		Target:    "stream.context",
		Input:     json.RawMessage(`{"limit":2}`),
	}

	audit, err := store.Apply(request, json.RawMessage(`{"items":2}`))
	if err != nil {
		t.Fatalf("apply authorized transition: %v", err)
	}
	if audit.FinalResult != "applied" {
		t.Fatalf("final result = %q, want applied", audit.FinalResult)
	}
	if !audit.Transition.Applied {
		t.Fatal("authorized transition was not marked applied")
	}
	if got := string(store.State()); got != `{"items":2}` {
		t.Fatalf("state = %s, want {\"items\":2}", got)
	}
	if len(store.Audits()) != 1 {
		t.Fatalf("audit len = %d, want 1", len(store.Audits()))
	}
}

func TestStoreRecordsDeniedTransitionsWithoutMutatingState(t *testing.T) {
	store := NewStore(NewValidator("compact"), json.RawMessage(`{"items":4}`))
	request := Request{
		Type:      MetaToolRequestType,
		Operation: "delete",
		Caller:    Identity{Name: "agent", Index: 1},
		Provider:  Identity{Name: "mcp", Index: 2},
		Target:    "stream.context",
		Input:     json.RawMessage(`{"limit":0}`),
	}

	audit, err := store.Apply(request, json.RawMessage(`{"items":0}`))
	if err == nil {
		t.Fatal("apply denied transition unexpectedly succeeded")
	}
	structured, ok := AsError(err)
	if !ok {
		t.Fatalf("expected structured error, got %T", err)
	}
	if structured.Category != ErrorCategoryPermission {
		t.Fatalf("category = %q, want %q", structured.Category, ErrorCategoryPermission)
	}
	if audit.Transition.Applied {
		t.Fatal("denied transition was marked applied")
	}
	if audit.Validation.Allowed {
		t.Fatal("denied transition validation reported allowed")
	}
	if got := string(store.State()); got != `{"items":4}` {
		t.Fatalf("state mutated on denied transition: %s", got)
	}
	if len(store.Audits()) != 1 {
		t.Fatalf("audit len = %d, want 1", len(store.Audits()))
	}
}

func TestValidatorRejectsInvalidRequestType(t *testing.T) {
	validator := NewValidator("compact")
	_, _, err := validator.Validate(Request{
		Type:      "message",
		Operation: "compact",
		Target:    "stream.context",
	}, nil, nil)
	if err == nil {
		t.Fatal("validator accepted invalid request type")
	}
	structured, ok := AsError(err)
	if !ok {
		t.Fatalf("expected structured error, got %T", err)
	}
	if structured.Category != ErrorCategoryValidation {
		t.Fatalf("category = %q, want %q", structured.Category, ErrorCategoryValidation)
	}
}
