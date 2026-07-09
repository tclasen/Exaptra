package meta

import (
	"encoding/json"
	"fmt"

	"github.com/tclasen/Exaptra/stream"
)

// StreamCompactor applies an authorized compaction transition to a stream.
type StreamCompactor struct {
	store    *Store
	retain   int
	caller   Identity
	provider Identity
}

// NewStreamCompactor creates a compactor bound to the current stream state.
func NewStreamCompactor(validator *Validator, initial *stream.Stream, retain int, caller, provider Identity) (*StreamCompactor, error) {
	if initial == nil {
		return nil, newError(ErrorCategoryValidation, "new_stream_compactor", "stream is required", nil)
	}
	if retain < 0 {
		return nil, newError(ErrorCategoryValidation, "new_stream_compactor", "retain count must be non-negative", nil)
	}
	encoded, err := json.Marshal(initial.Trajectory())
	if err != nil {
		return nil, newError(ErrorCategoryValidation, "new_stream_compactor", "marshal initial stream", err)
	}
	return &StreamCompactor{
		store:    NewStore(validator, encoded),
		retain:   retain,
		caller:   caller,
		provider: provider,
	}, nil
}

// Compact performs an authorized compaction and records the audit trail.
func (c *StreamCompactor) Compact(s *stream.Stream) (AuditRecord, error) {
	if c == nil || c.store == nil {
		return AuditRecord{}, newError(ErrorCategoryValidation, "compact", "compactor is required", nil)
	}
	if s == nil {
		return AuditRecord{}, newError(ErrorCategoryValidation, "compact", "stream is required", nil)
	}

	before := s.Trajectory()
	after := before
	if c.retain < len(after.Items) {
		after.Items = append([]stream.Item(nil), after.Items[len(after.Items)-c.retain:]...)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return AuditRecord{}, newError(ErrorCategoryValidation, "compact", "marshal after stream", err)
	}

	request := Request{
		Type:      MetaToolRequestType,
		Operation: "compact",
		Caller:    c.caller,
		Provider:  c.provider,
		Target:    "stream.context",
		Input:     json.RawMessage(fmt.Sprintf(`{"retain":%d}`, c.retain)),
	}

	audit, err := c.store.Apply(request, afterJSON)
	if err != nil {
		return audit, err
	}

	if err := s.Compact(c.retain); err != nil {
		return AuditRecord{}, newError(ErrorCategoryValidation, "compact", "apply stream compaction", err)
	}

	transition, transitionErr := stream.NewMetaTransition(
		fmt.Sprintf("mt_compact_%d", len(s.MetaTransitions())+1),
		int64(len(s.MetaTransitions())+1),
		"compact",
		"stream.context",
		countsJSON(before),
		countsJSON(s.Trajectory()),
		&stream.Provenance{Source: "meta", Provider: c.provider.Name, Component: "compaction"},
	)
	if transitionErr != nil {
		return AuditRecord{}, newError(ErrorCategoryValidation, "compact", "build meta transition", transitionErr)
	}
	if err := s.AppendMetaTransition(transition); err != nil {
		return AuditRecord{}, newError(ErrorCategoryValidation, "compact", "record meta transition", err)
	}

	return audit, nil
}

// Audits returns the compactor audit trail.
func (c *StreamCompactor) Audits() []AuditRecord {
	if c == nil || c.store == nil {
		return nil
	}
	return c.store.Audits()
}

func countsJSON(t stream.Trajectory) json.RawMessage {
	payload := fmt.Sprintf(`{"items":%d,"meta_transitions":%d}`, len(t.Items), len(t.MetaTransitions))
	return json.RawMessage(payload)
}
