package stream

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Adapter converts the internal Open Responses trajectory to provider-specific
// request formats. Provider packages can implement this interface without
// changing the stream model.
type Adapter interface {
	Format() string
	Convert(Trajectory) ([]byte, error)
}

// ChatMessage is a provider-neutral chat message shape used by ChatAdapter.
type ChatMessage struct {
	Role       string         `json:"role,omitempty"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
	ToolCalls  []ChatToolCall `json:"tool_calls,omitempty"`
}

type ChatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ChatToolFunction `json:"function"`
}

type ChatToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatAdapter converts portable Open Responses items to a simple chat-message
// transcript for providers that do not accept Open Responses inputs directly.
type ChatAdapter struct{}

func (ChatAdapter) Format() string { return "chat_messages" }

func (ChatAdapter) Convert(t Trajectory) ([]byte, error) {
	messages := make([]ChatMessage, 0, len(t.Items))
	for _, item := range t.Items {
		switch item.Type {
		case ItemTypeMessage:
			messages = append(messages, ChatMessage{
				Role:    item.Role,
				Content: item.Text(),
			})
		case ItemTypeFunctionCall:
			messages = append(messages, ChatMessage{
				Role: "assistant",
				ToolCalls: []ChatToolCall{{
					ID:   item.CallID,
					Type: "function",
					Function: ChatToolFunction{
						Name:      item.Name,
						Arguments: item.Arguments,
					},
				}},
			})
		case ItemTypeFunctionCallOutput:
			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    item.Output,
				ToolCallID: item.CallID,
			})
		default:
			return nil, fmt.Errorf("stream: unsupported item type %q for chat adapter", item.Type)
		}
	}

	return json.Marshal(messages)
}

func (m ChatMessage) MarshalJSON() ([]byte, error) {
	type alias ChatMessage
	encoded := struct {
		alias
		Content any `json:"content,omitempty"`
	}{
		alias: alias(m),
	}
	if m.Content != "" || len(m.ToolCalls) == 0 {
		encoded.Content = m.Content
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(encoded); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}
