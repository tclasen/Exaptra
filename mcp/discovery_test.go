package mcp

import (
	"context"
	"encoding/json"
	"errors"
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

	if err := catalog.Expose(identity, "lookup"); err != nil {
		t.Fatalf("expose tool: %v", err)
	}
	snapshot = catalog.Snapshot()
	if len(snapshot.Exposed) != 1 {
		t.Fatalf("snapshot exposed len = %d, want 1", len(snapshot.Exposed))
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
