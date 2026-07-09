package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/profiles"
)

func TestRunProducesSerializedExampleSnapshot(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
		"model": {"provider":"local","name":"example-model","api_key":{"env":"EXAPTRA_MODEL_API_KEY"}},
		"mcp_providers": [{"name":"localrun","command":"builtin","execution":{"kind":"local"}}],
		"tool_policy": {"mode":"allow_all"},
		"permissions": {"mode":"deny_by_default"},
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
	for _, needle := range []string{`"type": "function_call"`, `"type": "function_call_output"`, `"type": "exaptra:meta_transition"`, `"type": "exaptra:tracker_comment"`, `"type": "exaptra:tracker_pr_link"`, `"state": "review_ready"`, `"pull_request": {`, `"profile": {`, `"orchestration": {`, `"workflow": {`, `"correlation": {`, `"thread_id": "thread-example"`, `"kind": "tracker.comment"`, `"kind": "gate"`, `"[local/example-model:research] summarize the lookup output"`, `"[local/example-model:validate] confirm handoff state and tracker writes"`, `"shared_workspace": true`, `"availability": "exposed"`, `"api_key": ""`} {
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
