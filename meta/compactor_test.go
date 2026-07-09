package meta

import (
	"testing"

	"github.com/tclasen/Exaptra/stream"
)

func TestStreamCompactorAppliesAuthorizedCompaction(t *testing.T) {
	s := stream.New()
	provenance := &stream.Provenance{Source: "assistant", Provider: "openai", Component: "model"}
	if err := s.Append(stream.UserMessage("u1", 1, "one", provenance)); err != nil {
		t.Fatalf("append item: %v", err)
	}
	if err := s.Append(stream.UserMessage("u2", 2, "two", provenance)); err != nil {
		t.Fatalf("append item: %v", err)
	}
	if err := s.Append(stream.UserMessage("u3", 3, "three", provenance)); err != nil {
		t.Fatalf("append item: %v", err)
	}

	compactor, err := NewStreamCompactor(NewValidator("compact"), s, 1, Identity{Name: "agent", Index: 1}, Identity{Name: "mcp", Index: 2})
	if err != nil {
		t.Fatalf("new compactor: %v", err)
	}

	audit, err := compactor.Compact(s)
	if err != nil {
		t.Fatalf("compact stream: %v", err)
	}
	if audit.FinalResult != "applied" {
		t.Fatalf("final result = %q, want applied", audit.FinalResult)
	}
	if got := len(s.Items()); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if got := s.Items()[0].Text(); got != "three" {
		t.Fatalf("retained item text = %q, want three", got)
	}
	if got := len(s.MetaTransitions()); got != 1 {
		t.Fatalf("meta transition len = %d, want 1", got)
	}
	if s.MetaTransitions()[0].Type != stream.ItemTypeMetaTransition {
		t.Fatalf("meta transition type = %q, want %q", s.MetaTransitions()[0].Type, stream.ItemTypeMetaTransition)
	}

	encoded, err := s.Serialize()
	if err != nil {
		t.Fatalf("serialize stream: %v", err)
	}
	const want = `{"format":"open_responses","items":[{"id":"u3","type":"message","status":"completed","sequence":3,"role":"user","content":[{"type":"input_text","text":"three"}],"provenance":{"source":"assistant","provider":"openai","component":"model"}}],"meta_transitions":[{"id":"mt_compact_1","type":"exaptra:meta_transition","status":"completed","sequence":1,"operation":"compact","target":"stream.context","before":{"items":3,"meta_transitions":0},"after":{"items":1,"meta_transitions":0},"provenance":{"source":"meta","provider":"mcp","component":"compaction"}}]}`
	if string(encoded) != want {
		t.Fatalf("serialized stream mismatch\n got: %s\nwant: %s", encoded, want)
	}
}

func TestStreamCompactorAuditsDeniedCompactionWithoutMutatingStream(t *testing.T) {
	s := stream.New()
	if err := s.Append(stream.UserMessage("u1", 1, "one", nil)); err != nil {
		t.Fatalf("append item: %v", err)
	}
	compactor, err := NewStreamCompactor(NewValidator(), s, 0, Identity{Name: "agent", Index: 1}, Identity{Name: "mcp", Index: 2})
	if err != nil {
		t.Fatalf("new compactor: %v", err)
	}

	audit, err := compactor.Compact(s)
	if err == nil {
		t.Fatal("denied compaction unexpectedly succeeded")
	}
	structured, ok := AsError(err)
	if !ok {
		t.Fatalf("expected structured error, got %T", err)
	}
	if structured.Category != ErrorCategoryPermission {
		t.Fatalf("category = %q, want %q", structured.Category, ErrorCategoryPermission)
	}
	if audit.Transition.Applied {
		t.Fatal("denied compaction marked applied")
	}
	if audit.FinalResult != "denied" {
		t.Fatalf("final result = %q, want denied", audit.FinalResult)
	}
	if got := len(s.Items()); got != 1 {
		t.Fatalf("stream mutated on denied compaction, items len = %d", got)
	}
	if got := len(s.MetaTransitions()); got != 0 {
		t.Fatalf("meta transitions recorded on denied compaction, len = %d", got)
	}
}
