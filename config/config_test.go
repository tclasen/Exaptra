package config

import (
	"errors"
	"testing"
)

func TestLoadResolvesEnvironmentSecretReference(t *testing.T) {
	t.Setenv("EXAPTRA_MODEL_API_KEY", "secret-from-env")

	raw := []byte(`{
		"model": {
			"provider": "openai",
			"name": "gpt-4.1",
			"api_key": {"env": "EXAPTRA_MODEL_API_KEY"}
		},
		"mcp_providers": [
			{
				"name": "filesystem",
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem"]
			}
		],
		"tool_policy": {"mode": "allow_list", "tools": ["filesystem.read"]},
		"permissions": {"mode": "deny_by_default", "tools": ["compact"]},
		"debug": {"trace": true, "audit": false}
	}`)

	cfg, err := Load(raw)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got := cfg.Model.APIKey; got != "secret-from-env" {
		t.Fatalf("api key = %q, want secret-from-env", got)
	}
	if got := cfg.Model.Provider; got != "openai" {
		t.Fatalf("provider = %q, want openai", got)
	}
	if got := len(cfg.MCP); got != 1 {
		t.Fatalf("mcp providers len = %d, want 1", got)
	}
	if !cfg.Debug.Trace || cfg.Debug.Audit {
		t.Fatalf("debug config not loaded correctly: %+v", cfg.Debug)
	}
}

func TestLoadRejectsMissingRequiredFields(t *testing.T) {
	raw := []byte(`{
		"model": {"name": "gpt-4.1", "api_key": {"env": "EXAPTRA_MODEL_API_KEY"}},
		"mcp_providers": [],
		"tool_policy": {"mode": "allow_list"},
		"permissions": {"mode": "deny_by_default"},
		"debug": {"trace": true, "audit": false}
	}`)

	_, err := Load(raw)
	if err == nil {
		t.Fatal("load succeeded with missing required fields")
	}

	structured, ok := AsError(err)
	if !ok {
		t.Fatalf("expected structured config error, got %T", err)
	}
	if structured.Category != ErrorCategoryConfig {
		t.Fatalf("category = %q, want %q", structured.Category, ErrorCategoryConfig)
	}
}

func TestLoadFailsWhenSecretEnvVarIsUnset(t *testing.T) {
	raw := []byte(`{
		"model": {
			"provider": "openai",
			"name": "gpt-4.1",
			"api_key": {"env": "EXAPTRA_MISSING_API_KEY"}
		},
		"mcp_providers": [
			{"name": "filesystem", "command": "npx"}
		],
		"tool_policy": {"mode": "allow_list"},
		"permissions": {"mode": "deny_by_default"},
		"debug": {"trace": true, "audit": false}
	}`)

	_, err := Load(raw)
	if err == nil {
		t.Fatal("load succeeded with missing secret env var")
	}

	var configErr *Error
	if !errors.As(err, &configErr) {
		t.Fatalf("expected config error, got %T", err)
	}
	if configErr.Category != ErrorCategoryConfig {
		t.Fatalf("category = %q, want %q", configErr.Category, ErrorCategoryConfig)
	}
}
