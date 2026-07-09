package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type stubDiscoverer struct {
	tools []ToolMetadata
	err   error
}

func (s stubDiscoverer) DiscoverTools(ctx context.Context) ([]ToolMetadata, error) {
	return append([]ToolMetadata(nil), s.tools...), s.err
}

func TestCatalogDiscoversAndSeparatesExposureState(t *testing.T) {
	catalog := NewCatalog()
	identity := Identity{Name: "sleepy", Index: 0}

	discovered, err := catalog.DiscoverFrom(context.Background(), identity, stubDiscoverer{
		tools: []ToolMetadata{{
			Name:        "lookup",
			Description: "lookup records",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Scope:       "read",
		}},
	})
	if err != nil {
		t.Fatalf("discover tools: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("discovered len = %d, want 1", len(discovered))
	}
	if discovered[0].Provider != identity {
		t.Fatalf("provider = %+v, want %+v", discovered[0].Provider, identity)
	}

	snapshot := catalog.Snapshot()
	if len(snapshot.Discovered) != 1 {
		t.Fatalf("snapshot discovered len = %d, want 1", len(snapshot.Discovered))
	}
	if len(snapshot.Exposed) != 0 {
		t.Fatalf("snapshot exposed len = %d, want 0", len(snapshot.Exposed))
	}
	if len(snapshot.Records) != 1 {
		t.Fatalf("snapshot records len = %d, want 1", len(snapshot.Records))
	}
	if snapshot.Records[0].Availability != ToolAvailabilityDiscovered {
		t.Fatalf("availability = %q, want %q", snapshot.Records[0].Availability, ToolAvailabilityDiscovered)
	}
	if snapshot.Records[0].Reason != "discovered" {
		t.Fatalf("reason = %q, want discovered", snapshot.Records[0].Reason)
	}
	if snapshot.Records[0].Provenance == nil || snapshot.Records[0].Provenance.Provider != identity.Name {
		t.Fatalf("provenance provider = %+v, want %q", snapshot.Records[0].Provenance, identity.Name)
	}
	if snapshot.Records[0].Scope != "read" {
		t.Fatalf("scope = %q, want read", snapshot.Records[0].Scope)
	}

	if err := catalog.Expose(identity, "lookup"); err != nil {
		t.Fatalf("expose tool: %v", err)
	}
	snapshot = catalog.Snapshot()
	if len(snapshot.Exposed) != 1 {
		t.Fatalf("snapshot exposed len = %d, want 1", len(snapshot.Exposed))
	}
	if snapshot.Records[0].Availability != ToolAvailabilityExposed {
		t.Fatalf("availability = %q, want %q", snapshot.Records[0].Availability, ToolAvailabilityExposed)
	}
	if snapshot.Records[0].Reason != "exposed to model" {
		t.Fatalf("reason = %q, want exposed to model", snapshot.Records[0].Reason)
	}
}

func TestCatalogTracksHiddenAndUnavailableReasonsInSnapshot(t *testing.T) {
	catalog := NewCatalog()
	identity := Identity{Name: "filesystem", Index: 1}

	_, err := catalog.DiscoverFrom(context.Background(), identity, stubDiscoverer{
		tools: []ToolMetadata{
			{Name: "lookup", Description: "lookup records", Scope: "read"},
			{Name: "mutate", Description: "mutate state", Scope: "write"},
		},
	})
	if err != nil {
		t.Fatalf("discover tools: %v", err)
	}
	if err := catalog.Hide(identity, "lookup", "hidden by policy"); err != nil {
		t.Fatalf("hide tool: %v", err)
	}
	if err := catalog.MarkUnavailable(identity, "mutate", "provider disconnected"); err != nil {
		t.Fatalf("mark unavailable: %v", err)
	}

	snapshot := catalog.Snapshot()
	if len(snapshot.Records) != 2 {
		t.Fatalf("snapshot records len = %d, want 2", len(snapshot.Records))
	}
	if len(snapshot.Exposed) != 0 {
		t.Fatalf("snapshot exposed len = %d, want 0", len(snapshot.Exposed))
	}

	records := map[string]ToolRecord{}
	for _, record := range snapshot.Records {
		records[record.Name] = record
	}

	lookup := records["lookup"]
	if lookup.Availability != ToolAvailabilityHidden {
		t.Fatalf("lookup availability = %q, want %q", lookup.Availability, ToolAvailabilityHidden)
	}
	if lookup.Reason != "hidden by policy" {
		t.Fatalf("lookup reason = %q, want hidden by policy", lookup.Reason)
	}
	if lookup.Scope != "read" {
		t.Fatalf("lookup scope = %q, want read", lookup.Scope)
	}

	mutate := records["mutate"]
	if mutate.Availability != ToolAvailabilityUnavailable {
		t.Fatalf("mutate availability = %q, want %q", mutate.Availability, ToolAvailabilityUnavailable)
	}
	if mutate.Reason != "provider disconnected" {
		t.Fatalf("mutate reason = %q, want provider disconnected", mutate.Reason)
	}
	if mutate.Provenance == nil || mutate.Provenance.Component != "mutate" {
		t.Fatalf("mutate provenance = %+v, want component mutate", mutate.Provenance)
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("snapshot json invalid: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"availability":"hidden"`) || !strings.Contains(string(encoded), `"reason":"hidden by policy"`) {
		t.Fatalf("snapshot json does not include hidden record details: %s", encoded)
	}
}

func TestCatalogReturnsStructuredErrorForDiscoveryFailure(t *testing.T) {
	catalog := NewCatalog()
	identity := Identity{Name: "broken", Index: 1}
	cause := errors.New("provider unavailable")

	_, err := catalog.DiscoverFrom(context.Background(), identity, stubDiscoverer{err: cause})
	if err == nil {
		t.Fatal("discover succeeded unexpectedly")
	}

	structured, ok := AsError(err)
	if !ok {
		t.Fatalf("expected structured error, got %T", err)
	}
	if structured.Category != ErrorCategoryDiscovery {
		t.Fatalf("category = %q, want %q", structured.Category, ErrorCategoryDiscovery)
	}
	if structured.Identity != identity.String() {
		t.Fatalf("identity = %q, want %q", structured.Identity, identity.String())
	}
	if !errors.Is(err, cause) {
		t.Fatal("discovery error does not wrap cause")
	}
}
