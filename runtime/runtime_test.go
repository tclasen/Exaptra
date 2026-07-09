package runtime

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/tclasen/Exaptra/stream"
)

func TestNewModelRequestClonesAuthorizedContext(t *testing.T) {
	items := []stream.Item{
		stream.UserMessage("msg_1", 1, "hello", nil),
	}
	tools := []ToolDescriptor{
		{Name: "lookup", Description: "lookup records"},
	}

	request := NewModelRequest(items, tools)

	items[0].ID = "mutated"
	tools[0].Name = "mutated"
	request.Context.Items[0].Role = "mutated"
	request.Context.Tools[0].Description = "mutated"
	request.Context.Items[0].Content[0].Text = "mutated"

	if got := request.Context.Items[0].ID; got != "msg_1" {
		t.Fatalf("item id mutated through request copy: %q", got)
	}
	if got := request.Context.Tools[0].Name; got != "lookup" {
		t.Fatalf("tool name mutated through request copy: %q", got)
	}
	if got := items[0].Content[0].Text; got != "hello" {
		t.Fatalf("item content mutated through request copy: %q", got)
	}
}

func TestStructuredErrorsExposeCategoryAndWrapCause(t *testing.T) {
	cause := errors.New("dial tcp timeout")
	err := NewModelError("generate", "provider request failed", cause)

	got, ok := AsError(err)
	if !ok {
		t.Fatal("expected structured error")
	}
	if got.Category != ErrorCategoryModel {
		t.Fatalf("category = %q, want %q", got.Category, ErrorCategoryModel)
	}
	if got.Op != "generate" {
		t.Fatalf("op = %q, want generate", got.Op)
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause not preserved")
	}
}

func TestStructuredErrorConstructorsCoverAllCategories(t *testing.T) {
	cases := []struct {
		name string
		err  error
		cat  ErrorCategory
	}{
		{"tool", NewToolError("call", "tool failed", nil), ErrorCategoryTool},
		{"config", NewConfigError("load", "bad config", nil), ErrorCategoryConfig},
		{"permission", NewPermissionError("scope", "denied", nil), ErrorCategoryPermission},
		{"transition", NewTransitionError("apply", "rejected", nil), ErrorCategoryTransition},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := AsError(tc.err)
			if !ok {
				t.Fatal("expected structured error")
			}
			if got.Category != tc.cat {
				t.Fatalf("category = %q, want %q", got.Category, tc.cat)
			}
		})
	}
}

func TestToolRequestAndResponseJSONShapes(t *testing.T) {
	req := ToolRequest{
		Name:      "lookup",
		Arguments: json.RawMessage(`{"q":"hello"}`),
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	const wantReq = `{"name":"lookup","arguments":{"q":"hello"}}`
	if string(b) != wantReq {
		t.Fatalf("request json mismatch\n got: %s\nwant: %s", b, wantReq)
	}

	resp := ToolResponse{Output: json.RawMessage(`{"ok":true}`)}
	b, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	const wantResp = `{"output":{"ok":true}}`
	if string(b) != wantResp {
		t.Fatalf("response json mismatch\n got: %s\nwant: %s", b, wantResp)
	}
}
