package localrun

import (
	"context"
	"encoding/json"

	"github.com/tclasen/Exaptra/mcp"
)

// Provider is a simple local MCP provider for the runnable example.
type Provider struct{}

func (Provider) DiscoverTools(ctx context.Context) ([]mcp.ToolMetadata, error) {
	return []mcp.ToolMetadata{{
		Name:        "lookup",
		Description: "lookup an example record",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		Scope:       "read",
	}}, nil
}

func (Provider) CallTool(ctx context.Context, request mcp.ToolRequest) (mcp.ToolResponse, error) {
	var input struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(request.Arguments, &input)
	encoded, _ := json.Marshal(struct {
		Result string `json:"result"`
		Query  string `json:"query"`
	}{
		Result: "lookup example",
		Query:  input.Query,
	})
	return mcp.ToolResponse{
		Output: encoded,
	}, nil
}
