package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunProducesSerializedExampleSnapshot(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
		"model": {"provider":"local","name":"example-model","api_key":{"env":"EXAPTRA_MODEL_API_KEY"}},
		"mcp_providers": [{"name":"localrun","command":"builtin"}],
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
	for _, needle := range []string{`"type": "function_call"`, `"type": "function_call_output"`, `"type": "exaptra:meta_transition"`, `"type": "exaptra:tracker_comment"`, `"type": "exaptra:tracker_pr_link"`, `"state": "review_ready"`, `"pull_request": {`, `"orchestration": {`, `"workflow": {`, `"kind": "gate"`, `"shared_workspace": true`, `"availability": "exposed"`, `"api_key": ""`} {
		if !strings.Contains(output, needle) {
			t.Fatalf("example output missing %q: %s", needle, output)
		}
	}
}
