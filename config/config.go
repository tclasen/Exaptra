package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ErrorCategory identifies config failures for caller-side branching.
type ErrorCategory string

const (
	ErrorCategoryConfig ErrorCategory = "config"
)

// Error is the structured error returned by config loading.
type Error struct {
	Category ErrorCategory `json:"category"`
	Op       string        `json:"op,omitempty"`
	Message  string        `json:"message"`
	Err      error         `json:"-"`
}

func (e *Error) Error() string {
	switch {
	case e == nil:
		return "<nil>"
	case e.Op != "" && e.Err != nil:
		return fmt.Sprintf("%s %s: %s: %v", e.Category, e.Op, e.Message, e.Err)
	case e.Op != "":
		return fmt.Sprintf("%s %s: %s", e.Category, e.Op, e.Message)
	case e.Err != nil:
		return fmt.Sprintf("%s: %s: %v", e.Category, e.Message, e.Err)
	default:
		return fmt.Sprintf("%s: %s", e.Category, e.Message)
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newConfigError(op, message string, err error) error {
	return &Error{
		Category: ErrorCategoryConfig,
		Op:       op,
		Message:  message,
		Err:      err,
	}
}

// AsError extracts a structured config error when possible.
func AsError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// Config is the resolved runtime configuration used by the orchestrator.
type Config struct {
	Model       ModelConfig     `json:"model"`
	MCP         []MCPProvider   `json:"mcp_providers"`
	ToolPolicy  ToolPolicy      `json:"tool_policy"`
	Permissions MetaPermissions `json:"permissions"`
	Debug       DebugConfig     `json:"debug"`
}

// Redacted returns a copy of the config with secret values removed.
func (c Config) Redacted() Config {
	redacted := c
	redacted.Model.APIKey = ""
	redacted.MCP = append([]MCPProvider(nil), c.MCP...)
	for i := range redacted.MCP {
		redacted.MCP[i].Env = cloneRedactedEnv(redacted.MCP[i].Env)
	}
	return redacted
}

// ModelConfig selects a model provider and resolves its secret credentials.
type ModelConfig struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	APIKey   string `json:"api_key"`
}

// MCPProvider configures one external MCP provider process.
type MCPProvider struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ToolPolicy controls which ordinary tools the runtime exposes initially.
type ToolPolicy struct {
	Mode  string   `json:"mode"`
	Tools []string `json:"tools,omitempty"`
}

// MetaPermissions controls which meta-tools the runtime permits.
type MetaPermissions struct {
	Mode  string   `json:"mode"`
	Tools []string `json:"tools,omitempty"`
}

// DebugConfig controls trace and audit behavior.
type DebugConfig struct {
	Trace bool `json:"trace"`
	Audit bool `json:"audit"`
}

// LoadFile reads a JSON configuration file, validates required fields, and
// resolves secret references from the environment.
func LoadFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, newConfigError("read", fmt.Sprintf("read config %q", path), err)
	}
	return Load(raw)
}

// Load parses and resolves a JSON configuration document.
func Load(raw []byte) (Config, error) {
	var parsed rawConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return Config{}, newConfigError("parse", "decode config", err)
	}

	resolved, err := parsed.resolve()
	if err != nil {
		return Config{}, err
	}
	return resolved, nil
}

type rawConfig struct {
	Model       rawModelConfig     `json:"model"`
	MCP         []rawMCPProvider   `json:"mcp_providers"`
	ToolPolicy  rawToolPolicy      `json:"tool_policy"`
	Permissions rawMetaPermissions `json:"permissions"`
	Debug       rawDebugConfig     `json:"debug"`
}

type rawModelConfig struct {
	Provider *string    `json:"provider"`
	Name     *string    `json:"name"`
	APIKey   *secretRef `json:"api_key"`
}

type rawMCPProvider struct {
	Name    *string           `json:"name"`
	Command *string           `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type rawToolPolicy struct {
	Mode  *string  `json:"mode"`
	Tools []string `json:"tools,omitempty"`
}

type rawMetaPermissions struct {
	Mode  *string  `json:"mode"`
	Tools []string `json:"tools,omitempty"`
}

type rawDebugConfig struct {
	Trace *bool `json:"trace"`
	Audit *bool `json:"audit"`
}

type secretRef struct {
	Env *string `json:"env"`
}

func (r rawConfig) resolve() (Config, error) {
	model, err := r.Model.resolve()
	if err != nil {
		return Config{}, err
	}
	if len(r.MCP) == 0 {
		return Config{}, newConfigError("mcp_providers", "at least one mcp provider is required", nil)
	}

	mcpProviders := make([]MCPProvider, len(r.MCP))
	for i, provider := range r.MCP {
		resolved, err := provider.resolve(i)
		if err != nil {
			return Config{}, err
		}
		mcpProviders[i] = resolved
	}

	policy, err := r.ToolPolicy.resolve()
	if err != nil {
		return Config{}, err
	}

	permissions, err := r.Permissions.resolve()
	if err != nil {
		return Config{}, err
	}

	debug, err := r.Debug.resolve()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Model:       model,
		MCP:         mcpProviders,
		ToolPolicy:  policy,
		Permissions: permissions,
		Debug:       debug,
	}, nil
}

func (r rawModelConfig) resolve() (ModelConfig, error) {
	if r.Provider == nil || *r.Provider == "" {
		return ModelConfig{}, newConfigError("model.provider", "missing model provider", nil)
	}
	if r.Name == nil || *r.Name == "" {
		return ModelConfig{}, newConfigError("model.name", "missing model name", nil)
	}
	if r.APIKey == nil {
		return ModelConfig{}, newConfigError("model.api_key", "missing model api key reference", nil)
	}
	apiKey, err := r.APIKey.resolve("model.api_key")
	if err != nil {
		return ModelConfig{}, err
	}

	return ModelConfig{
		Provider: *r.Provider,
		Name:     *r.Name,
		APIKey:   apiKey,
	}, nil
}

func (r rawMCPProvider) resolve(index int) (MCPProvider, error) {
	if r.Name == nil || *r.Name == "" {
		return MCPProvider{}, newConfigError(fmt.Sprintf("mcp_providers[%d].name", index), "missing mcp provider name", nil)
	}
	if r.Command == nil || *r.Command == "" {
		return MCPProvider{}, newConfigError(fmt.Sprintf("mcp_providers[%d].command", index), "missing mcp provider command", nil)
	}
	return MCPProvider{
		Name:    *r.Name,
		Command: *r.Command,
		Args:    append([]string(nil), r.Args...),
		Env:     cloneStringMap(r.Env),
	}, nil
}

func (r rawToolPolicy) resolve() (ToolPolicy, error) {
	if r.Mode == nil || *r.Mode == "" {
		return ToolPolicy{}, newConfigError("tool_policy.mode", "missing tool exposure mode", nil)
	}
	return ToolPolicy{
		Mode:  *r.Mode,
		Tools: append([]string(nil), r.Tools...),
	}, nil
}

func (r rawMetaPermissions) resolve() (MetaPermissions, error) {
	if r.Mode == nil || *r.Mode == "" {
		return MetaPermissions{}, newConfigError("permissions.mode", "missing meta-tool permission mode", nil)
	}
	return MetaPermissions{
		Mode:  *r.Mode,
		Tools: append([]string(nil), r.Tools...),
	}, nil
}

func (r rawDebugConfig) resolve() (DebugConfig, error) {
	if r.Trace == nil {
		return DebugConfig{}, newConfigError("debug.trace", "missing debug trace flag", nil)
	}
	if r.Audit == nil {
		return DebugConfig{}, newConfigError("debug.audit", "missing debug audit flag", nil)
	}
	return DebugConfig{
		Trace: *r.Trace,
		Audit: *r.Audit,
	}, nil
}

func (r secretRef) resolve(field string) (string, error) {
	if r.Env == nil || *r.Env == "" {
		return "", newConfigError(field, "missing secret environment reference", nil)
	}
	value, ok := os.LookupEnv(*r.Env)
	if !ok || value == "" {
		return "", newConfigError(field, fmt.Sprintf("secret environment variable %q is not set", *r.Env), nil)
	}
	return value, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRedactedEnv(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k := range in {
		out[k] = "[redacted]"
	}
	return out
}
