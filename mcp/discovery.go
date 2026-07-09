package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

const (
	// ErrorCategoryDiscovery identifies tool-discovery failures.
	ErrorCategoryDiscovery ErrorCategory = "discovery"
)

// ToolMetadata describes one discovered tool and the provider that exposed it.
type ToolMetadata struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Provider    Identity        `json:"provider"`
	Scope       string          `json:"scope,omitempty"`
}

// DiscoveryState captures discovered tools separately from model exposure.
type DiscoveryState struct {
	Discovered []ToolMetadata `json:"discovered"`
	Exposed    []ToolMetadata `json:"exposed"`
}

// Discoverer exposes discovered tools for a provider connection.
type Discoverer interface {
	DiscoverTools(ctx context.Context) ([]ToolMetadata, error)
}

// Catalog stores discovered tools and the subset currently exposed to the
// model.
type Catalog struct {
	mu         sync.Mutex
	discovered map[string]ToolMetadata
	exposed    map[string]ToolMetadata
}

func NewCatalog() *Catalog {
	return &Catalog{
		discovered: make(map[string]ToolMetadata),
		exposed:    make(map[string]ToolMetadata),
	}
}

// DiscoverFrom records discovered tools for a specific provider identity.
func (c *Catalog) DiscoverFrom(ctx context.Context, identity Identity, discoverer Discoverer) ([]ToolMetadata, error) {
	if discoverer == nil {
		return nil, newError(ErrorCategoryProvider, identity.String(), "discover", "discoverer is required", nil)
	}

	tools, err := discoverer.DiscoverTools(ctx)
	if err != nil {
		return nil, newError(ErrorCategoryDiscovery, identity.String(), "discover", "discover provider tools", err)
	}

	cloned := make([]ToolMetadata, len(tools))
	for i, tool := range tools {
		tool.Provider = identity
		cloned[i] = cloneToolMetadata(tool)
	}

	c.mu.Lock()
	for _, tool := range cloned {
		c.discovered[toolKey(identity, tool.Name)] = cloneToolMetadata(tool)
	}
	c.mu.Unlock()

	return cloned, nil
}

// Expose makes one discovered tool visible to the model.
func (c *Catalog) Expose(identity Identity, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := toolKey(identity, name)
	tool, ok := c.discovered[key]
	if !ok {
		return newError(ErrorCategoryDiscovery, identity.String(), "expose", fmt.Sprintf("tool %q is not discovered", name), nil)
	}
	c.exposed[key] = cloneToolMetadata(tool)
	return nil
}

// Snapshot returns the discovered and model-visible tool sets.
func (c *Catalog) Snapshot() DiscoveryState {
	c.mu.Lock()
	defer c.mu.Unlock()

	return DiscoveryState{
		Discovered: cloneToolMetadataSlice(c.discovered),
		Exposed:    cloneToolMetadataSlice(c.exposed),
	}
}

func toolKey(identity Identity, name string) string {
	return identity.String() + ":" + name
}

func cloneToolMetadata(tool ToolMetadata) ToolMetadata {
	tool.InputSchema = append(json.RawMessage(nil), tool.InputSchema...)
	return tool
}

func cloneToolMetadataSlice(in map[string]ToolMetadata) []ToolMetadata {
	out := make([]ToolMetadata, 0, len(in))
	for _, tool := range in {
		out = append(out, cloneToolMetadata(tool))
	}
	return out
}
