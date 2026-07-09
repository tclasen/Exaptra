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

// ToolTransition records a change to a tool's registry state.
type ToolTransition struct {
	Operation string      `json:"operation"`
	Identity  Identity    `json:"identity"`
	Name      string      `json:"name"`
	Reason    string      `json:"reason,omitempty"`
	Before    *ToolRecord `json:"before,omitempty"`
	After     *ToolRecord `json:"after,omitempty"`
}

// PermissionCapability identifies a permission boundary decision.
type PermissionCapability string

const (
	PermissionCapabilityReadState   PermissionCapability = "read_state"
	PermissionCapabilityMutateState PermissionCapability = "mutate_state"
)

// PermissionDecision captures an authorization decision for inspection.
type PermissionDecision struct {
	Capability PermissionCapability `json:"capability"`
	Target     string               `json:"target,omitempty"`
	Allowed    bool                 `json:"allowed"`
	Reason     string               `json:"reason,omitempty"`
}

// PermissionPolicy is a deny-by-default mutation policy for the registry.
type PermissionPolicy struct {
	mu             sync.Mutex
	allowMutations bool
	decisions      []PermissionDecision
}

func NewPermissionPolicy() *PermissionPolicy {
	return &PermissionPolicy{}
}

func (p *PermissionPolicy) GrantMutations(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.allowMutations = true
	p.decisions = append(p.decisions, PermissionDecision{
		Capability: PermissionCapabilityMutateState,
		Allowed:    true,
		Reason:     reason,
	})
}

func (p *PermissionPolicy) Authorize(capability PermissionCapability, target, reason string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	allowed := true
	if capability == PermissionCapabilityMutateState {
		allowed = p.allowMutations
	}
	p.decisions = append(p.decisions, PermissionDecision{
		Capability: capability,
		Target:     target,
		Allowed:    allowed,
		Reason:     reason,
	})
	return allowed
}

func (p *PermissionPolicy) Decisions() []PermissionDecision {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]PermissionDecision, len(p.decisions))
	copy(out, p.decisions)
	return out
}

// DiscoveryState captures discovered tools separately from model exposure.
type DiscoveryState struct {
	Records     []ToolRecord         `json:"records"`
	Transitions []ToolTransition     `json:"transitions"`
	Permissions []PermissionDecision `json:"permissions"`
	Discovered  []ToolMetadata       `json:"discovered"`
	Exposed     []ToolMetadata       `json:"exposed"`
}

// Discoverer exposes discovered tools for a provider connection.
type Discoverer interface {
	DiscoverTools(ctx context.Context) ([]ToolMetadata, error)
}

// Catalog stores discovered tools and the subset currently exposed to the
// model.
type Catalog struct {
	mu          sync.Mutex
	records     map[string]catalogEntry
	transitions []ToolTransition
	permissions *PermissionPolicy
}

func NewCatalog() *Catalog {
	return &Catalog{
		records:     make(map[string]catalogEntry),
		permissions: NewPermissionPolicy(),
	}
}

// Permissions returns the registry permission policy.
func (c *Catalog) Permissions() *PermissionPolicy {
	return c.permissions
}

type catalogEntry struct {
	tool         ToolMetadata
	availability ToolAvailability
	reason       string
	provenance   *stream.Provenance
}

func (c *Catalog) recordLocked(identity Identity, tool ToolMetadata, availability ToolAvailability, reason, operation string, before *ToolRecord) {
	key := toolKey(identity, tool.Name)
	entry := catalogEntry{
		tool:         cloneToolMetadata(tool),
		availability: availability,
		reason:       reason,
		provenance:   &stream.Provenance{Provider: identity.Name, Component: tool.Name},
	}
	c.records[key] = entry
	c.transitions = append(c.transitions, ToolTransition{
		Operation: operation,
		Identity:  identity,
		Name:      tool.Name,
		Reason:    reason,
		Before:    cloneToolRecordPtr(before),
		After:     recordPtr(entry.toRecord(), true),
	})
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
		c.recordLocked(identity, tool, ToolAvailabilityDiscovered, "discovered", "discover", nil)
	}
	c.mu.Unlock()

	return cloned, nil
}

// RefreshFrom updates the registry from a new discovery pass while preserving
// existing exposure state and recording add/remove/refresh transitions.
func (c *Catalog) RefreshFrom(ctx context.Context, identity Identity, discoverer Discoverer) ([]ToolMetadata, error) {
	if discoverer == nil {
		return nil, newError(ErrorCategoryProvider, identity.String(), "refresh", "discoverer is required", nil)
	}
	if !c.permissions.Authorize(PermissionCapabilityMutateState, identity.String(), "refresh registry") {
		return nil, newError(ErrorCategoryPermission, identity.String(), "refresh", "refresh denied by policy", nil)
	}

	tools, err := discoverer.DiscoverTools(ctx)
	if err != nil {
		return nil, newError(ErrorCategoryDiscovery, identity.String(), "refresh", "refresh provider tools", err)
	}

	cloned := make([]ToolMetadata, len(tools))
	for i, tool := range tools {
		tool.Provider = identity
		cloned[i] = cloneToolMetadata(tool)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[string]struct{}, len(cloned))
	for _, tool := range cloned {
		key := toolKey(identity, tool.Name)
		seen[key] = struct{}{}

		before, existed := c.records[key]
		after := catalogEntry{
			tool:         cloneToolMetadata(tool),
			availability: ToolAvailabilityDiscovered,
			reason:       "refreshed from provider",
			provenance:   &stream.Provenance{Provider: identity.Name, Component: tool.Name},
		}
		if existed {
			after.availability = before.availability
			after.reason = before.reason
		}

		c.records[key] = after
		c.transitions = append(c.transitions, ToolTransition{
			Operation: "refresh",
			Identity:  identity,
			Name:      tool.Name,
			Reason:    "refreshed from provider",
			Before:    recordPtr(before.toRecord(), existed),
			After:     recordPtr(after.toRecord(), true),
		})
	}

	for key, entry := range c.records {
		if entry.tool.Provider != identity {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		before := entry.toRecord()
		entry.availability = ToolAvailabilityUnavailable
		entry.reason = "removed during refresh"
		c.records[key] = entry
		c.transitions = append(c.transitions, ToolTransition{
			Operation: "remove",
			Identity:  identity,
			Name:      entry.tool.Name,
			Reason:    entry.reason,
			Before:    recordPtr(before, true),
			After:     recordPtr(entry.toRecord(), true),
		})
	}

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
	if !c.permissions.Authorize(PermissionCapabilityMutateState, toolKey(identity, name), reason) {
		return newError(ErrorCategoryPermission, identity.String(), availabilityOperation(availability), "mutation denied by policy", nil)
	}

	key := toolKey(identity, name)
	entry, ok := c.records[key]
	if !ok {
		return newError(ErrorCategoryDiscovery, identity.String(), "update", fmt.Sprintf("tool %q is not discovered", name), nil)
	}
	before := entry.toRecord()
	entry.availability = availability
	entry.reason = reason
	c.records[key] = entry
	c.transitions = append(c.transitions, ToolTransition{
		Operation: availabilityOperation(availability),
		Identity:  identity,
		Name:      name,
		Reason:    reason,
		Before:    recordPtr(before, true),
		After:     recordPtr(entry.toRecord(), true),
	})
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
	for _, transition := range c.transitions {
		state.Transitions = append(state.Transitions, transition.clone())
	}
	state.Permissions = c.permissions.Decisions()
	sort.Slice(state.Records, func(i, j int) bool {
		return recordSortKey(state.Records[i]) < recordSortKey(state.Records[j])
	})
	sort.Slice(state.Transitions, func(i, j int) bool {
		return transitionSortKey(state.Transitions[i]) < transitionSortKey(state.Transitions[j])
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
	c.permissions.Authorize(PermissionCapabilityReadState, name, "lookup exposed tool")
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

func cloneToolRecord(record ToolRecord) ToolRecord {
	record.ToolMetadata = cloneToolMetadata(record.ToolMetadata)
	record.Provenance = cloneStreamProvenance(record.Provenance)
	return record
}

func (t ToolTransition) clone() ToolTransition {
	return ToolTransition{
		Operation: t.Operation,
		Identity:  t.Identity,
		Name:      t.Name,
		Reason:    t.Reason,
		Before:    cloneToolRecordPtr(t.Before),
		After:     cloneToolRecordPtr(t.After),
	}
}

func recordPtr(record ToolRecord, ok bool) *ToolRecord {
	if !ok {
		return nil
	}
	cloned := cloneToolRecord(record)
	return &cloned
}

func cloneToolRecordPtr(record *ToolRecord) *ToolRecord {
	if record == nil {
		return nil
	}
	cloned := cloneToolRecord(*record)
	return &cloned
}

func transitionSortKey(transition ToolTransition) string {
	return transition.Identity.String() + ":" + transition.Name + ":" + transition.Operation
}

func availabilityOperation(availability ToolAvailability) string {
	switch availability {
	case ToolAvailabilityExposed:
		return "expose"
	case ToolAvailabilityHidden:
		return "hide"
	case ToolAvailabilityUnavailable:
		return "remove"
	default:
		return "update"
	}
}
