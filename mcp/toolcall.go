package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tclasen/Exaptra/stream"
)

// ToolRequest is sent to the provider that owns the requested tool.
type ToolRequest struct {
	Name       string
	CallID     string
	Arguments  json.RawMessage
	Provenance *stream.Provenance
}

// ToolResponse is the normalized provider response.
type ToolResponse struct {
	Output     json.RawMessage
	Provenance *stream.Provenance
}

// ToolCaller executes a tool for a specific MCP provider.
type ToolCaller interface {
	CallTool(ctx context.Context, request ToolRequest) (ToolResponse, error)
}

// ToolResolver finds the caller that owns a provider identity.
type ToolResolver interface {
	ResolveToolCaller(identity Identity) (ToolCaller, bool)
}

// Dispatcher resolves discovered tools to providers, executes them, and writes
// normalized call/result records into the stream.
type Dispatcher struct {
	catalog  *Catalog
	resolver ToolResolver
}

func NewDispatcher(catalog *Catalog, resolver ToolResolver) *Dispatcher {
	return &Dispatcher{catalog: catalog, resolver: resolver}
}

// Invoke appends the tool call and normalized output to the stream.
func (d *Dispatcher) Invoke(ctx context.Context, s *stream.Stream, call stream.Item) (stream.Item, error) {
	if d == nil || d.catalog == nil {
		return stream.Item{}, newError(ErrorCategoryTool, "", "invoke", "dispatcher catalog is required", nil)
	}
	if d.resolver == nil {
		return stream.Item{}, newError(ErrorCategoryTool, "", "invoke", "tool resolver is required", nil)
	}
	if s == nil {
		return stream.Item{}, newError(ErrorCategoryTool, "", "invoke", "stream is required", nil)
	}
	if call.Type != stream.ItemTypeFunctionCall {
		return stream.Item{}, newError(ErrorCategoryTool, "", "invoke", fmt.Sprintf("unsupported call type %q", call.Type), nil)
	}
	if err := call.Validate(); err != nil {
		return stream.Item{}, newError(ErrorCategoryTool, "", "invoke", "invalid tool call", err)
	}

	tool, lookupErr := d.catalog.LookupExposed(call.Name)
	if lookupErr != nil {
		return stream.Item{}, newError(ErrorCategoryTool, "", "invoke", "resolve exposed tool", lookupErr)
	}
	if tool.Name == "" {
		return stream.Item{}, newError(ErrorCategoryTool, "", "invoke", fmt.Sprintf("tool %q is not exposed", call.Name), nil)
	}

	caller, ok := d.resolver.ResolveToolCaller(tool.Provider)
	if !ok {
		return stream.Item{}, newError(ErrorCategoryTool, tool.Provider.String(), "invoke", "tool caller not available", nil)
	}

	if err := s.Append(call); err != nil {
		return stream.Item{}, newError(ErrorCategoryTool, tool.Provider.String(), "invoke", "append tool call", err)
	}

	response, err := caller.CallTool(ctx, ToolRequest{
		Name:       call.Name,
		CallID:     call.CallID,
		Arguments:  json.RawMessage(call.Arguments),
		Provenance: call.Provenance,
	})
	if err != nil {
		failure := callFailureOutput(call, tool, err)
		_ = s.Append(failure)
		return stream.Item{}, newError(ErrorCategoryTool, tool.Provider.String(), "invoke", "tool call failed", err)
	}

	result, err := normalizeToolResult(call, tool, response)
	if err != nil {
		failure := callFailureOutput(call, tool, err)
		_ = s.Append(failure)
		return stream.Item{}, newError(ErrorCategoryTool, tool.Provider.String(), "invoke", "normalize tool result", err)
	}
	if err := s.Append(result); err != nil {
		return stream.Item{}, newError(ErrorCategoryTool, tool.Provider.String(), "invoke", "append tool result", err)
	}
	return result, nil
}

func normalizeToolResult(call stream.Item, tool ToolMetadata, response ToolResponse) (stream.Item, error) {
	output := response.Output
	if len(output) == 0 {
		output = json.RawMessage(`{}`)
	}
	normalized, err := normalizeJSON(output)
	if err != nil {
		return stream.Item{}, err
	}

	provenance := mergeToolProvenance(call.Provenance, tool, response.Provenance)
	return stream.FunctionCallOutput(
		call.CallID+"-result",
		call.Sequence+1,
		call.CallID,
		string(normalized),
		provenance,
	), nil
}

func callFailureOutput(call stream.Item, tool ToolMetadata, err error) stream.Item {
	payload, marshalErr := json.Marshal(map[string]any{
		"error": map[string]any{
			"category": string(ErrorCategoryTool),
			"message":  err.Error(),
		},
	})
	if marshalErr != nil {
		payload = json.RawMessage(`{"error":{"category":"tool","message":"failed to encode error"}}`)
	}
	return stream.FunctionCallOutput(
		call.CallID+"-error",
		call.Sequence+1,
		call.CallID,
		string(payload),
		mergeToolProvenance(call.Provenance, tool, nil),
	)
}

func mergeToolProvenance(base *stream.Provenance, tool ToolMetadata, response *stream.Provenance) *stream.Provenance {
	provenance := cloneStreamProvenance(base)
	if provenance == nil {
		provenance = &stream.Provenance{}
	}
	provenance.Provider = tool.Provider.String()
	provenance.Component = tool.Name
	if response != nil {
		if response.Source != "" {
			provenance.Source = response.Source
		}
		if response.TraceID != "" {
			provenance.TraceID = response.TraceID
		}
		if response.Model != "" {
			provenance.Model = response.Model
		}
	}
	return provenance
}

func cloneStreamProvenance(provenance *stream.Provenance) *stream.Provenance {
	if provenance == nil {
		return nil
	}
	cloned := *provenance
	return &cloned
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
