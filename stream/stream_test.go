package stream

import (
	"encoding/json"
	"testing"
)

func TestStreamConstructionInspectionAndSerialization(t *testing.T) {
	s := New()
	provenance := &Provenance{Source: "test", Provider: "openresponses", Model: "gpt-openresponses-1"}

	if err := s.Append(UserMessage("msg_user_1", 1, "Find the current weather.", provenance)); err != nil {
		t.Fatalf("append user message: %v", err)
	}

	call, err := FunctionCall("fc_1", 2, "get_weather", "call_1", json.RawMessage(`{"city":"San Francisco"}`), provenance)
	if err != nil {
		t.Fatalf("build function call: %v", err)
	}
	if err := s.Append(call); err != nil {
		t.Fatalf("append function call: %v", err)
	}

	if err := s.Append(FunctionCallOutput("fco_1", 3, "call_1", `{"temp_f":64}`, provenance)); err != nil {
		t.Fatalf("append function call output: %v", err)
	}

	if err := s.Append(AssistantMessage("msg_asst_1", 4, "It is 64F.", provenance)); err != nil {
		t.Fatalf("append assistant message: %v", err)
	}

	transition, err := NewMetaTransition(
		"mt_1",
		5,
		"compact",
		"stream.context",
		json.RawMessage(`{"items":4}`),
		json.RawMessage(`{"items":2}`),
		&Provenance{Component: "compactor"},
	)
	if err != nil {
		t.Fatalf("build meta transition: %v", err)
	}
	if err := s.AppendMetaTransition(transition); err != nil {
		t.Fatalf("append meta transition: %v", err)
	}

	items := s.Items()
	if len(items) != 4 {
		t.Fatalf("items len = %d, want 4", len(items))
	}
	if items[0].Role != RoleUser {
		t.Fatalf("first role = %q, want %q", items[0].Role, RoleUser)
	}
	if items[1].Type != ItemTypeFunctionCall {
		t.Fatalf("second type = %q, want %q", items[1].Type, ItemTypeFunctionCall)
	}
	if items[2].Type != ItemTypeFunctionCallOutput {
		t.Fatalf("third type = %q, want %q", items[2].Type, ItemTypeFunctionCallOutput)
	}

	metaTransitions := s.MetaTransitions()
	if len(metaTransitions) != 1 {
		t.Fatalf("meta transitions len = %d, want 1", len(metaTransitions))
	}
	if metaTransitions[0].Type != ItemTypeMetaTransition {
		t.Fatalf("meta transition type = %q, want %q", metaTransitions[0].Type, ItemTypeMetaTransition)
	}

	got, err := s.Serialize()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	const want = `{"format":"open_responses","items":[{"id":"msg_user_1","type":"message","status":"completed","sequence":1,"role":"user","content":[{"type":"input_text","text":"Find the current weather."}],"provenance":{"source":"test","provider":"openresponses","model":"gpt-openresponses-1"}},{"id":"fc_1","type":"function_call","status":"completed","sequence":2,"name":"get_weather","call_id":"call_1","arguments":"{\"city\":\"San Francisco\"}","provenance":{"source":"test","provider":"openresponses","model":"gpt-openresponses-1"}},{"id":"fco_1","type":"function_call_output","status":"completed","sequence":3,"call_id":"call_1","output":"{\"temp_f\":64}","provenance":{"source":"test","provider":"openresponses","model":"gpt-openresponses-1"}},{"id":"msg_asst_1","type":"message","status":"completed","sequence":4,"role":"assistant","content":[{"type":"output_text","text":"It is 64F."}],"provenance":{"source":"test","provider":"openresponses","model":"gpt-openresponses-1"}}],"meta_transitions":[{"id":"mt_1","type":"exaptra:meta_transition","status":"completed","sequence":5,"operation":"compact","target":"stream.context","before":{"items":4},"after":{"items":2},"provenance":{"component":"compactor"}}]}`
	if string(got) != want {
		t.Fatalf("serialized stream mismatch\n got: %s\nwant: %s", got, want)
	}

	again, err := s.Serialize()
	if err != nil {
		t.Fatalf("serialize again: %v", err)
	}
	if string(again) != string(got) {
		t.Fatalf("serialization is not deterministic\nfirst:  %s\nsecond: %s", got, again)
	}
}

func TestInspectionReturnsCopies(t *testing.T) {
	s := New()
	item := UserMessage("msg_user_1", 1, "hello", &Provenance{Source: "caller"})
	if err := s.Append(item); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	item.Content[0].Text = "mutated before inspection"
	item.Provenance.Source = "mutated before inspection"

	items := s.Items()
	items[0].ID = "mutated"
	items[0].Content[0].Text = "mutated after inspection"
	items[0].Provenance.Source = "mutated after inspection"

	stored := s.Items()[0]
	if got := stored.ID; got != "msg_user_1" {
		t.Fatalf("stream item mutated through inspection copy: %q", got)
	}
	if got := stored.Content[0].Text; got != "hello" {
		t.Fatalf("stream item content mutated through caller copy: %q", got)
	}
	if got := stored.Provenance.Source; got != "caller" {
		t.Fatalf("stream item provenance mutated through caller copy: %q", got)
	}
}

func TestChatAdapterConvertsOpenResponsesItems(t *testing.T) {
	s := New()
	if err := s.Append(UserMessage("msg_user_1", 1, "hello", nil)); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	call, err := FunctionCall("fc_1", 2, "lookup", "call_1", json.RawMessage(`{"q":"hello"}`), nil)
	if err != nil {
		t.Fatalf("build function call: %v", err)
	}
	if err := s.Append(call); err != nil {
		t.Fatalf("append function call: %v", err)
	}
	if err := s.Append(FunctionCallOutput("fco_1", 3, "call_1", "world", nil)); err != nil {
		t.Fatalf("append function call output: %v", err)
	}

	got, err := s.Convert(ChatAdapter{})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	const want = `[{"role":"user","content":"hello"},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"hello\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"world"}]`
	if string(got) != want {
		t.Fatalf("chat adapter output mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestJSONNormalizationPreservesLargeNumbers(t *testing.T) {
	call, err := FunctionCall("fc_1", 1, "lookup", "call_1", json.RawMessage(`{"id":9007199254740993}`), nil)
	if err != nil {
		t.Fatalf("build function call: %v", err)
	}
	if call.Arguments != `{"id":9007199254740993}` {
		t.Fatalf("arguments lost numeric precision: %s", call.Arguments)
	}

	transition, err := NewMetaTransition(
		"mt_1",
		2,
		"update",
		"state.counter",
		json.RawMessage(`{"counter":9007199254740993}`),
		json.RawMessage(`{"counter":9007199254740994}`),
		nil,
	)
	if err != nil {
		t.Fatalf("build meta transition: %v", err)
	}
	if string(transition.Before) != `{"counter":9007199254740993}` {
		t.Fatalf("before state lost numeric precision: %s", transition.Before)
	}
	if string(transition.After) != `{"counter":9007199254740994}` {
		t.Fatalf("after state lost numeric precision: %s", transition.After)
	}
}

func TestValidationRejectsUnsupportedItems(t *testing.T) {
	s := New()
	err := s.Append(Item{ID: "x", Type: "provider:special", Status: StatusCompleted, Sequence: 1})
	if err == nil {
		t.Fatal("append unsupported item succeeded")
	}
}

func TestFunctionCallRequiresJSONObjectArguments(t *testing.T) {
	_, err := FunctionCall("fc_1", 1, "lookup", "call_1", json.RawMessage(`["not","object"]`), nil)
	if err == nil {
		t.Fatal("function call accepted non-object arguments")
	}
}
