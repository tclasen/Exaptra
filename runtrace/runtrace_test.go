package runtrace

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/meta"
	"github.com/tclasen/Exaptra/stream"
	"github.com/tclasen/Exaptra/tracker"
)

type stubDiscoverer struct {
	tools []mcp.ToolMetadata
	err   error
}

func (s stubDiscoverer) DiscoverTools(ctx context.Context) ([]mcp.ToolMetadata, error) {
	return append([]mcp.ToolMetadata(nil), s.tools...), s.err
}

func TestSnapshotIncludesRunStateAndRedactsSecrets(t *testing.T) {
	cfg := config.Config{
		Model: config.ModelConfig{
			Provider: "openai",
			Name:     "gpt-4.1",
			APIKey:   "secret-model-key",
		},
		MCP: []config.MCPProvider{{
			Name:    "filesystem",
			Command: "npx",
			Env:     map[string]string{"EXAPTRA_TOKEN": "secret-provider-token"},
		}},
		Debug: config.DebugConfig{Trace: true, Audit: true},
	}

	s := stream.New()
	provenance := &stream.Provenance{Source: "assistant", Provider: "openai", Component: "model"}
	if err := s.Append(stream.UserMessage("msg_1", 1, "hello", provenance)); err != nil {
		t.Fatalf("append message: %v", err)
	}
	call, err := stream.FunctionCall("fc_1", 2, "lookup", "call_1", json.RawMessage(`{"q":"hello"}`), provenance)
	if err != nil {
		t.Fatalf("build function call: %v", err)
	}
	if err := s.Append(call); err != nil {
		t.Fatalf("append function call: %v", err)
	}
	if err := s.Append(stream.FunctionCallOutput("fco_1", 3, "call_1", `{"value":"ok"}`, provenance)); err != nil {
		t.Fatalf("append function call output: %v", err)
	}
	transition, err := stream.NewMetaTransition("mt_1", 4, "compact", "stream.context", json.RawMessage(`{"items":3}`), json.RawMessage(`{"items":1}`), provenance)
	if err != nil {
		t.Fatalf("build meta transition: %v", err)
	}
	if err := s.AppendMetaTransition(transition); err != nil {
		t.Fatalf("append meta transition: %v", err)
	}

	trackerStore := tracker.NewStore(nil)
	trackerIssue := tracker.IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 52}
	if _, err := trackerStore.Comment(context.Background(), tracker.CommentRequest{
		RunID: "run-1",
		Issue: trackerIssue,
		Body:  "recorded progress",
		Provenance: tracker.Provenance{
			RunID:     "run-1",
			Source:    "orchestrator",
			Component: "tracker",
		},
	}); err != nil {
		t.Fatalf("record tracker comment: %v", err)
	}
	if _, err := trackerStore.LinkPullRequest(context.Background(), tracker.PullRequestLinkRequest{
		RunID: "run-1",
		Issue: trackerIssue,
		PullRequest: tracker.PullRequestRef{
			Owner:  "tclasen",
			Repo:   "Exaptra",
			Number: 99,
			URL:    "https://github.com/tclasen/Exaptra/pull/99",
		},
		State: tracker.HandoffStateReview,
		Provenance: tracker.Provenance{
			RunID:     "run-1",
			Source:    "orchestrator",
			Component: "tracker",
		},
	}); err != nil {
		t.Fatalf("record tracker PR link: %v", err)
	}

	catalog := mcp.NewCatalog()
	catalog.Permissions().GrantMutations("test")
	_, err = catalog.DiscoverFrom(context.Background(), mcp.Identity{Name: "filesystem", Index: 0}, stubDiscoverer{tools: []mcp.ToolMetadata{{Name: "lookup", Description: "lookup records", Scope: "read"}}})
	if err != nil {
		t.Fatalf("discover tools: %v", err)
	}
	if err := catalog.Expose(mcp.Identity{Name: "filesystem", Index: 0}, "lookup"); err != nil {
		t.Fatalf("expose tool: %v", err)
	}

	store := meta.NewStore(meta.NewValidator("compact"), json.RawMessage(`{"items":3}`))
	audit, err := store.Apply(meta.Request{
		Type:      meta.MetaToolRequestType,
		Operation: "compact",
		Caller:    meta.Identity{Name: "agent", Index: 1},
		Provider:  meta.Identity{Name: "mcp", Index: 2},
		Target:    "stream.context",
		Input:     json.RawMessage(`{"retain":1}`),
	}, json.RawMessage(`{"items":1}`))
	if err != nil {
		t.Fatalf("apply audit transition: %v", err)
	}

	snapshot := NewSnapshot(cfg, s, catalog, []meta.AuditRecord{audit}, trackerStore.Audits())
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("snapshot json invalid: %s", encoded)
	}
	if strings.Contains(string(encoded), "secret-model-key") || strings.Contains(string(encoded), "secret-provider-token") {
		t.Fatalf("snapshot leaked secrets: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"type":"function_call"`) || !strings.Contains(string(encoded), `"type":"function_call_output"`) {
		t.Fatalf("snapshot missing tool activity: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"type":"exaptra:meta_transition"`) || !strings.Contains(string(encoded), `"validation":{"allowed":true`) {
		t.Fatalf("snapshot missing meta audit data: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"type":"exaptra:tracker_comment"`) || !strings.Contains(string(encoded), `"recorded progress"`) {
		t.Fatalf("snapshot missing tracker audit data: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"type":"exaptra:tracker_pr_link"`) || !strings.Contains(string(encoded), `"pull_request":{"owner":"tclasen","repo":"Exaptra","number":99,"url":"https://github.com/tclasen/Exaptra/pull/99"}`) {
		t.Fatalf("snapshot missing tracker PR link data: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"availability":"exposed"`) {
		t.Fatalf("snapshot missing registry state: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"api_key":""`) || !strings.Contains(string(encoded), `"EXAPTRA_TOKEN":"[redacted]"`) {
		t.Fatalf("snapshot config was not redacted: %s", encoded)
	}
}
