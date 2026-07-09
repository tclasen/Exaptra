package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tclasen/Exaptra/stream"
)

// ModelProvider generates the next assistant turn from an authorized request.
type ModelProvider interface {
	Generate(ctx context.Context, request ModelRequest) (ModelResponse, error)
}

// ToolProvider executes a tool call outside the runtime core.
type ToolProvider interface {
	Call(ctx context.Context, request ToolRequest) (ToolResponse, error)
}

// ModelRequest is the provider-neutral input passed to a model provider.
// It contains only the stream context and tool set that the runtime has
// already authorized for that call.
type ModelRequest struct {
	Context ModelContext `json:"context"`
}

// ModelContext is the filtered state visible to a model provider.
type ModelContext struct {
	Items []stream.Item    `json:"items"`
	Tools []ToolDescriptor `json:"tools"`
}

// ModelResponse is the provider-neutral output returned by a model provider.
type ModelResponse struct {
	Items        []stream.Item `json:"items"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

// ToolRequest represents a single tool invocation.
type ToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolResponse represents the result of a tool invocation.
type ToolResponse struct {
	Output json.RawMessage `json:"output,omitempty"`
}

// ToolDescriptor exposes the tool metadata the runtime is willing to reveal to
// a model provider.
type ToolDescriptor struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Source      string          `json:"source,omitempty"`
}

// NewModelRequest clones the authorized stream context and tool set so callers
// can safely build requests from owned slices without leaking later mutations.
func NewModelRequest(items []stream.Item, tools []ToolDescriptor) ModelRequest {
	clonedItems, err := deepCopyItems(items)
	if err != nil {
		panic(fmt.Sprintf("runtime: clone model request items: %v", err))
	}
	clonedTools, err := deepCopyToolDescriptors(tools)
	if err != nil {
		panic(fmt.Sprintf("runtime: clone model request tools: %v", err))
	}
	return ModelRequest{
		Context: ModelContext{
			Items: clonedItems,
			Tools: clonedTools,
		},
	}
}

func deepCopyItems(items []stream.Item) ([]stream.Item, error) {
	if items == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var decoded []stream.Item
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func deepCopyToolDescriptors(tools []ToolDescriptor) ([]ToolDescriptor, error) {
	if tools == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return nil, err
	}
	var decoded []ToolDescriptor
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}
