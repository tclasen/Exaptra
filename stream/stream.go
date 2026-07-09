package stream

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

const (
	ItemTypeMessage            = "message"
	ItemTypeFunctionCall       = "function_call"
	ItemTypeFunctionCallOutput = "function_call_output"
	ItemTypeMetaTransition     = "exaptra:meta_transition"

	ContentTypeInputText  = "input_text"
	ContentTypeOutputText = "output_text"

	RoleUser      = "user"
	RoleAssistant = "assistant"

	StatusCompleted = "completed"
)

type Provenance struct {
	Source    string `json:"source,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Component string `json:"component,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
}

type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Item struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Status     string        `json:"status"`
	Sequence   int64         `json:"sequence"`
	Role       string        `json:"role,omitempty"`
	Content    []ContentPart `json:"content,omitempty"`
	Name       string        `json:"name,omitempty"`
	CallID     string        `json:"call_id,omitempty"`
	Arguments  string        `json:"arguments,omitempty"`
	Output     string        `json:"output,omitempty"`
	Provenance *Provenance   `json:"provenance,omitempty"`
}

func UserMessage(id string, sequence int64, content string, provenance *Provenance) Item {
	return message(id, sequence, RoleUser, ContentTypeInputText, content, provenance)
}

func AssistantMessage(id string, sequence int64, content string, provenance *Provenance) Item {
	return message(id, sequence, RoleAssistant, ContentTypeOutputText, content, provenance)
}

func FunctionCall(id string, sequence int64, name string, callID string, arguments json.RawMessage, provenance *Provenance) (Item, error) {
	normalized, err := normalizeJSONObject(arguments)
	if err != nil {
		return Item{}, err
	}

	return Item{
		ID:         id,
		Type:       ItemTypeFunctionCall,
		Status:     StatusCompleted,
		Sequence:   sequence,
		Name:       name,
		CallID:     callID,
		Arguments:  normalized,
		Provenance: cloneProvenance(provenance),
	}, nil
}

func FunctionCallOutput(id string, sequence int64, callID string, output string, provenance *Provenance) Item {
	return Item{
		ID:         id,
		Type:       ItemTypeFunctionCallOutput,
		Status:     StatusCompleted,
		Sequence:   sequence,
		CallID:     callID,
		Output:     output,
		Provenance: cloneProvenance(provenance),
	}
}

func (i Item) Text() string {
	var b bytes.Buffer
	for _, part := range i.Content {
		b.WriteString(part.Text)
	}
	return b.String()
}

func (i Item) Validate() error {
	if i.ID == "" {
		return errors.New("stream: item id is required")
	}
	if i.Type == "" {
		return errors.New("stream: item type is required")
	}
	if i.Status == "" {
		return errors.New("stream: item status is required")
	}

	switch i.Type {
	case ItemTypeMessage:
		if i.Role != RoleUser && i.Role != RoleAssistant {
			return fmt.Errorf("stream: unsupported message role %q", i.Role)
		}
		if len(i.Content) == 0 {
			return errors.New("stream: message content is required")
		}
	case ItemTypeFunctionCall:
		if i.Name == "" {
			return errors.New("stream: function call name is required")
		}
		if i.CallID == "" {
			return errors.New("stream: function call call_id is required")
		}
		if i.Arguments == "" {
			return errors.New("stream: function call arguments are required")
		}
	case ItemTypeFunctionCallOutput:
		if i.CallID == "" {
			return errors.New("stream: function call output call_id is required")
		}
	default:
		return fmt.Errorf("stream: unsupported item type %q", i.Type)
	}

	return nil
}

type MetaTransition struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	Sequence   int64           `json:"sequence"`
	Operation  string          `json:"operation"`
	Target     string          `json:"target"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	Provenance *Provenance     `json:"provenance,omitempty"`
}

func NewMetaTransition(id string, sequence int64, operation string, target string, before, after json.RawMessage, provenance *Provenance) (MetaTransition, error) {
	normalizedBefore, err := normalizeOptionalJSON(before)
	if err != nil {
		return MetaTransition{}, fmt.Errorf("stream: invalid meta transition before value: %w", err)
	}
	normalizedAfter, err := normalizeOptionalJSON(after)
	if err != nil {
		return MetaTransition{}, fmt.Errorf("stream: invalid meta transition after value: %w", err)
	}

	return MetaTransition{
		ID:         id,
		Type:       ItemTypeMetaTransition,
		Status:     StatusCompleted,
		Sequence:   sequence,
		Operation:  operation,
		Target:     target,
		Before:     normalizedBefore,
		After:      normalizedAfter,
		Provenance: cloneProvenance(provenance),
	}, nil
}

func (m MetaTransition) Validate() error {
	if m.ID == "" {
		return errors.New("stream: meta transition id is required")
	}
	if m.Type != ItemTypeMetaTransition {
		return fmt.Errorf("stream: meta transition type must be %q", ItemTypeMetaTransition)
	}
	if m.Status == "" {
		return errors.New("stream: meta transition status is required")
	}
	if m.Operation == "" {
		return errors.New("stream: meta transition operation is required")
	}
	if m.Target == "" {
		return errors.New("stream: meta transition target is required")
	}
	return nil
}

type Trajectory struct {
	Format          string           `json:"format"`
	Items           []Item           `json:"items"`
	MetaTransitions []MetaTransition `json:"meta_transitions"`
}

type Stream struct {
	items           []Item
	metaTransitions []MetaTransition
}

func New() *Stream {
	return &Stream{}
}

func (s *Stream) Append(item Item) error {
	if err := item.Validate(); err != nil {
		return err
	}
	s.items = append(s.items, cloneItem(item))
	return nil
}

func (s *Stream) AppendMetaTransition(transition MetaTransition) error {
	if err := transition.Validate(); err != nil {
		return err
	}
	s.metaTransitions = append(s.metaTransitions, cloneMetaTransition(transition))
	return nil
}

func (s *Stream) Items() []Item {
	return cloneItems(s.items)
}

func (s *Stream) MetaTransitions() []MetaTransition {
	return cloneMetaTransitions(s.metaTransitions)
}

func (s *Stream) Trajectory() Trajectory {
	return Trajectory{
		Format:          "open_responses",
		Items:           s.Items(),
		MetaTransitions: s.MetaTransitions(),
	}
}

func (s *Stream) Serialize() ([]byte, error) {
	return json.Marshal(s.Trajectory())
}

func (s *Stream) Convert(adapter Adapter) ([]byte, error) {
	if adapter == nil {
		return nil, errors.New("stream: adapter is required")
	}
	return adapter.Convert(s.Trajectory())
}

// Compact retains the most recent items and discards the older prefix.
func (s *Stream) Compact(retain int) error {
	if retain < 0 {
		return errors.New("stream: retain count must be non-negative")
	}
	if retain >= len(s.items) {
		return nil
	}
	start := len(s.items) - retain
	s.items = cloneItems(s.items[start:])
	return nil
}

func message(id string, sequence int64, role string, contentType string, content string, provenance *Provenance) Item {
	return Item{
		ID:       id,
		Type:     ItemTypeMessage,
		Status:   StatusCompleted,
		Sequence: sequence,
		Role:     role,
		Content: []ContentPart{{
			Type: contentType,
			Text: content,
		}},
		Provenance: cloneProvenance(provenance),
	}
}

func cloneProvenance(provenance *Provenance) *Provenance {
	if provenance == nil {
		return nil
	}
	copied := *provenance
	return &copied
}

func cloneItems(items []Item) []Item {
	cloned := slices.Clone(items)
	for i := range cloned {
		cloned[i] = cloneItem(cloned[i])
	}
	return cloned
}

func cloneItem(item Item) Item {
	item.Content = slices.Clone(item.Content)
	item.Provenance = cloneProvenance(item.Provenance)
	return item
}

func cloneMetaTransitions(transitions []MetaTransition) []MetaTransition {
	cloned := slices.Clone(transitions)
	for i := range cloned {
		cloned[i] = cloneMetaTransition(cloned[i])
	}
	return cloned
}

func cloneMetaTransition(transition MetaTransition) MetaTransition {
	transition.Before = slices.Clone(transition.Before)
	transition.After = slices.Clone(transition.After)
	transition.Provenance = cloneProvenance(transition.Provenance)
	return transition
}

func normalizeJSONObject(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	normalized, err := normalizeJSON(raw)
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(normalized, []byte("{")) {
		return "", errors.New("arguments must be a JSON object")
	}
	return string(normalized), nil
}

func normalizeOptionalJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	return normalizeJSON(raw)
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
