package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/tclasen/Exaptra/stream"
)

const (
	// ErrorCategoryDiscovery identifies tool-discovery failures.
	ErrorCategoryDiscovery ErrorCategory = "discovery"
)

// ToolAvailability describes the registry state of a discovered tool.
type ToolAvailability string

const (
	ToolAvailabilityDiscovered  ToolAvailability = "discovered"
	ToolAvailabilityExposed     ToolAvailability = "exposed"
	ToolAvailabilityHidden      ToolAvailability = "hidden"
	ToolAvailabilityUnavailable ToolAvailability = "unavailable"
)

// ToolMetadata describes one discovered tool and the provider that exposed it.
type ToolMetadata struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Provider    Identity        `json:"provider"`
	Scope       string          `json:"scope,omitempty"`
}

// ToolRecord is the serialized registry record for a tool.
type ToolRecord struct {
	ToolMetadata
	Availability ToolAvailability   `json:"availability"`
	Reason       string             `json:"reason,omitempty"`
	Provenance   *stream.Provenance `json:"provenance,omitempty"`
}

// DiscoveryState captures discovered tools separately from model exposure.
type DiscoveryState struct {
	Records    []ToolRecord   `json:"records"`
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
	mu      sync.Mutex
	records map[string]catalogEntry
}

func NewCatalog() *Catalog {
	return &Catalog{
		records: make(map[string]catalogEntry),
	}
}

type catalogEntry struct {
	tool         ToolMetadata
	availability ToolAvailability
	reason       string
	provenance   *stream.Provenance
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
		c.records[toolKey(identity, tool.Name)] = catalogEntry{
			tool:         cloneToolMetadata(tool),
			availability: ToolAvailabilityDiscovered,
			reason:       "discovered",
			provenance:   &stream.Provenance{Provider: identity.Name, Component: tool.Name},
		}
	}
	c.mu.Unlock()

	return cloned, nil
}

// Expose makes one discovered tool visible to the model.
func (c *Catalog) Expose(identity Identity, name string) error {
	return c.setAvailability(identity, name, ToolAvailabilityExposed, "exposed to model")
}

// Hide marks a tool as hidden while retaining its discovery record.
func (c *Catalog) Hide(identity Identity, name, reason string) error {
	return c.setAvailability(identity, name, ToolAvailabilityHidden, reason)
}

// MarkUnavailable marks a tool as unavailable while retaining its discovery record.
func (c *Catalog) MarkUnavailable(identity Identity, name, reason string) error {
	return c.setAvailability(identity, name, ToolAvailabilityUnavailable, reason)
}

func (c *Catalog) setAvailability(identity Identity, name string, availability ToolAvailability, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := toolKey(identity, name)
	entry, ok := c.records[key]
	if !ok {
		return newError(ErrorCategoryDiscovery, identity.String(), "update", fmt.Sprintf("tool %q is not discovered", name), nil)
	}
	entry.availability = availability
	entry.reason = reason
	c.records[key] = entry
	return nil
}

// Snapshot returns the discovered and model-visible tool sets.
func (c *Catalog) Snapshot() DiscoveryState {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := DiscoveryState{}
	for _, entry := range c.records {
		state.Records = append(state.Records, entry.toRecord())
		state.Discovered = append(state.Discovered, cloneToolMetadata(entry.tool))
		if entry.availability == ToolAvailabilityExposed {
			state.Exposed = append(state.Exposed, cloneToolMetadata(entry.tool))
		}
	}
	sort.Slice(state.Records, func(i, j int) bool {
		return recordSortKey(state.Records[i]) < recordSortKey(state.Records[j])
	})
	sort.Slice(state.Discovered, func(i, j int) bool {
		return metadataSortKey(state.Discovered[i]) < metadataSortKey(state.Discovered[j])
	})
	sort.Slice(state.Exposed, func(i, j int) bool {
		return metadataSortKey(state.Exposed[i]) < metadataSortKey(state.Exposed[j])
	})
	return state
}

// LookupExposed returns the currently exposed tool with the given name.
func (c *Catalog) LookupExposed(name string) (ToolMetadata, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var (
		found ToolMetadata
		ok    bool
	)
	for _, entry := range c.records {
		if entry.tool.Name != name || entry.availability != ToolAvailabilityExposed {
			continue
		}
		if ok {
			return ToolMetadata{}, fmt.Errorf("tool %q is exposed by multiple providers", name)
		}
		found = cloneToolMetadata(entry.tool)
		ok = true
	}
	if !ok {
		return ToolMetadata{}, nil
	}
	return found, nil
}

func toolKey(identity Identity, name string) string {
	return identity.String() + ":" + name
}

func recordSortKey(record ToolRecord) string {
	return metadataSortKey(record.ToolMetadata) + ":" + string(record.Availability)
}

func metadataSortKey(metadata ToolMetadata) string {
	return metadata.Provider.String() + ":" + metadata.Name
}

func cloneToolMetadata(tool ToolMetadata) ToolMetadata {
	tool.InputSchema = append(json.RawMessage(nil), tool.InputSchema...)
	return tool
}

func (e catalogEntry) toRecord() ToolRecord {
	return ToolRecord{
		ToolMetadata: cloneToolMetadata(e.tool),
		Availability: e.availability,
		Reason:       e.reason,
		Provenance:   cloneStreamProvenance(e.provenance),
	}
}
