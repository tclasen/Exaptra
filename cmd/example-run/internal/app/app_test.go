package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/profiles"
	"github.com/tclasen/Exaptra/stream"
)

func TestRunProducesSerializedExampleSnapshot(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
		"model": {"provider":"local","name":"example-model","api_key":{"env":"EXAPTRA_MODEL_API_KEY"}},
		"mcp_providers": [{"name":"localrun","command":"builtin","execution":{"kind":"local"}}],
		"tool_policy": {"mode":"allow_all"},
		"permissions": {"mode":"deny_by_default"},
		"spend": {"window":"1h","currency":"USD","budgets":[{"name":"example-hourly","provider":"local","model":"example-model","max_tokens":100}]},
		"debug": {"trace": true, "audit": true}
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("EXAPTRA_MODEL_API_KEY", "example-secret")

	var stdout bytes.Buffer
	if err := Run([]string{"-config", configPath}, &stdout); err != nil {
		t.Fatalf("run example: %v", err)
	}

	output := stdout.String()
	if strings.Contains(output, "example-secret") {
		t.Fatalf("example run leaked secret: %s", output)
	}
	for _, needle := range []string{`"type": "function_call"`, `"type": "function_call_output"`, `"type": "exaptra:meta_transition"`, `"type": "exaptra:tracker_comment"`, `"type": "exaptra:tracker_pr_link"`, `"state": "review_ready"`, `"pull_request": {`, `"profile": {`, `"orchestration": {`, `"workflow": {`, `"spend": {`, `"total_tokens":`, `"status": "breached"`, `"kind": "gate"`, `"[local/example-model:research] summarize the lookup output"`, `"[local/example-model:validate] confirm handoff state and tracker writes"`, `"shared_workspace": true`, `"availability": "exposed"`, `"api_key": ""`} {
		if !strings.Contains(output, needle) {
			t.Fatalf("example output missing %q: %s", needle, output)
		}
	}
	if !strings.Contains(output, `"execution": {`) || !strings.Contains(output, `"kind": "local"`) {
		t.Fatalf("example output missing execution config: %s", output)
	}
	if !strings.Contains(output, `"workspace": {`) || !strings.Contains(output, `".exaptra/workspaces/tclasen/exaptra/52"`) {
		t.Fatalf("example output missing workspace config: %s", output)
	}
}

func TestObservedSpendUsageReflectsRunState(t *testing.T) {
	s := stream.New()
	if err := s.Append(stream.UserMessage("u1", 1, "short", nil)); err != nil {
		t.Fatalf("append short message: %v", err)
	}
	short := observedSpendUsage("run-1", "local", "example-model", s, nil, nil, nil)

	if err := s.Append(stream.UserMessage("u2", 2, "this is a longer observed request body", nil)); err != nil {
		t.Fatalf("append longer message: %v", err)
	}
	long := observedSpendUsage("run-1", "local", "example-model", s, nil, nil, nil)

	if len(short) != 1 || len(long) != 1 {
		t.Fatalf("usage record counts = %d, %d", len(short), len(long))
	}
	if long[0].InputTokens <= short[0].InputTokens {
		t.Fatalf("usage did not grow with observed stream: short=%#v long=%#v", short[0], long[0])
	}
	if long[0].Provider != "local" || long[0].Model != "example-model" || long[0].RunID != "run-1" {
		t.Fatalf("usage attribution not preserved: %#v", long[0])
	}
}

func TestExposeProfileToolsRequiresLookupDiscovery(t *testing.T) {
	catalog := mcp.NewCatalog()
	profile := profiles.Selection{
		Name:        "local-example",
		ToolSurface: []string{"lookup"},
	}

	err := exposeProfileTools(catalog, mcp.Identity{Name: "filesystem", Index: 0}, profile)
	if err == nil || !strings.Contains(err.Error(), "not discovered") {
		t.Fatalf("expected missing lookup discovery error, got %v", err)
	}
}
