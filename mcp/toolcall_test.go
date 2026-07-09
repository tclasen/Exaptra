package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tclasen/Exaptra/stream"
)

type stubToolCaller struct {
	response ToolResponse
	err      error
}

func (s stubToolCaller) CallTool(ctx context.Context, request ToolRequest) (ToolResponse, error) {
	return s.response, s.err
}

type stubToolResolver map[string]ToolCaller

func (s stubToolResolver) ResolveToolCaller(identity Identity) (ToolCaller, bool) {
	caller, ok := s[identity.String()]
	return caller, ok
}

func TestDispatcherInvokesDiscoveredToolAndRecordsStreamItems(t *testing.T) {
	catalog := NewCatalog()
	identity := Identity{Name: "filesystem", Index: 0}
	_, err := catalog.DiscoverFrom(context.Background(), identity, stubDiscoverer{
		tools: []ToolMetadata{{
			Name:        "lookup",
			Description: "lookup records",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Scope:       "read",
		}},
	})
	if err != nil {
		t.Fatalf("discover tool: %v", err)
	}
	if err := catalog.Expose(identity, "lookup"); err != nil {
		t.Fatalf("expose tool: %v", err)
	}

	streamState := stream.New()
	dispatcher := NewDispatcher(catalog, stubToolResolver{
		identity.String(): stubToolCaller{
			response: ToolResponse{Output: json.RawMessage(`{"value":"ok"}`)},
		},
	})

	call, err := stream.FunctionCall("fc_1", 1, "lookup", "call_1", json.RawMessage(`{"q":"hello"}`), &stream.Provenance{Source: "assistant"})
	if err != nil {
		t.Fatalf("build function call: %v", err)
	}

	result, err := dispatcher.Invoke(context.Background(), streamState, call)
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if result.Type != stream.ItemTypeFunctionCallOutput {
		t.Fatalf("result type = %q, want %q", result.Type, stream.ItemTypeFunctionCallOutput)
	}

	items := streamState.Items()
	if len(items) != 2 {
		t.Fatalf("stream items len = %d, want 2", len(items))
	}
	if items[0].Type != stream.ItemTypeFunctionCall {
		t.Fatalf("first item type = %q, want function_call", items[0].Type)
	}
	if items[1].Type != stream.ItemTypeFunctionCallOutput {
		t.Fatalf("second item type = %q, want function_call_output", items[1].Type)
	}
	if items[1].Provenance.Provider != identity.String() {
		t.Fatalf("provider provenance = %q, want %q", items[1].Provenance.Provider, identity.String())
	}
	if items[1].Provenance.Component != "lookup" {
		t.Fatalf("component provenance = %q, want lookup", items[1].Provenance.Component)
	}
	if got := items[1].Output; got != `{"value":"ok"}` {
		t.Fatalf("normalized output = %q, want {\"value\":\"ok\"}", got)
	}
}

func TestDispatcherRecordsFailureAsStructuredToolOutput(t *testing.T) {
	catalog := NewCatalog()
	identity := Identity{Name: "filesystem", Index: 0}
	_, err := catalog.DiscoverFrom(context.Background(), identity, stubDiscoverer{
		tools: []ToolMetadata{{Name: "lookup", Description: "lookup records"}},
	})
	if err != nil {
		t.Fatalf("discover tool: %v", err)
	}
	if err := catalog.Expose(identity, "lookup"); err != nil {
		t.Fatalf("expose tool: %v", err)
	}

	streamState := stream.New()
	dispatcher := NewDispatcher(catalog, stubToolResolver{
		identity.String(): stubToolCaller{err: errors.New("provider unavailable")},
	})

	call, err := stream.FunctionCall("fc_1", 1, "lookup", "call_1", json.RawMessage(`{"q":"hello"}`), nil)
	if err != nil {
		t.Fatalf("build function call: %v", err)
	}

	_, err = dispatcher.Invoke(context.Background(), streamState, call)
	if err == nil {
		t.Fatal("invoke succeeded unexpectedly")
	}
	structured, ok := AsError(err)
	if !ok {
		t.Fatalf("expected structured error, got %T", err)
	}
	if structured.Category != ErrorCategoryTool {
		t.Fatalf("category = %q, want %q", structured.Category, ErrorCategoryTool)
	}

	items := streamState.Items()
	if len(items) != 2 {
		t.Fatalf("stream items len = %d, want 2", len(items))
	}
	if items[1].Type != stream.ItemTypeFunctionCallOutput {
		t.Fatalf("second item type = %q, want function_call_output", items[1].Type)
	}
	if items[1].Output == "" {
		t.Fatal("failure output was not recorded")
	}
}
