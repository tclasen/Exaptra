package workflowdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromRepoRoot(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, filepath.Join(dir, defaultFilename), `---
policy:
  mode: gate
  labels:
    - phase:5-orchestration
  blockers:
    - waiting on docs
prompts:
  orchestrator: coordinate the slice
runtime:
  shared_workspace: true
  max_concurrency: 2
telemetry:
  enabled: true
  sampling_rate: 1
  high_risk_sampling_rate: 0.25
  retention_days: 14
  allowed_readers:
    - maintainers
  export_requires_approval: true
  redact_attributes:
    - prompt
---
# {{.Issue.Title}}
Issue #{{.Issue.Number}} for {{.Issue.Owner}}/{{.Issue.Repo}}
Labels: {{join .Issue.Labels ", "}}
Blockers: {{join .Issue.Blockers ", "}}
Instructions: {{.Issue.Instructions}}
Policy mode: {{.FrontMatter.Policy.Mode}}
Telemetry retention: {{.FrontMatter.Telemetry.RetentionDays}}
`)

	doc, err := Load(dir, "")
	if err != nil {
		t.Fatalf("load workflow from repo root: %v", err)
	}
	if doc.Path != filepath.Join(dir, defaultFilename) {
		t.Fatalf("unexpected workflow path: %s", doc.Path)
	}
	if doc.FrontMatter.Policy.Mode != "gate" {
		t.Fatalf("unexpected policy mode: %#v", doc.FrontMatter.Policy)
	}
	if !doc.FrontMatter.Runtime.SharedWorkspace || doc.FrontMatter.Runtime.MaxConcurrency != 2 {
		t.Fatalf("unexpected runtime config: %#v", doc.FrontMatter.Runtime)
	}
	if !doc.FrontMatter.Telemetry.Enabled || doc.FrontMatter.Telemetry.HighRiskSamplingRate != 0.25 {
		t.Fatalf("unexpected telemetry config: %#v", doc.FrontMatter.Telemetry)
	}

	rendered, err := doc.Render(RenderContext{
		Issue: IssueContext{
			Owner:        "tclasen",
			Repo:         "Exaptra",
			Number:       46,
			Title:        "Define workflow manifests",
			Instructions: "treat the workflow file as the contract",
			Labels:       []string{"enhancement", "phase:5-orchestration"},
			Blockers:     []string{"docs"},
		},
		FrontMatter: doc.FrontMatter,
	})
	if err != nil {
		t.Fatalf("render workflow body: %v", err)
	}
	for _, needle := range []string{
		"Define workflow manifests",
		"Issue #46 for tclasen/Exaptra",
		"Labels: enhancement, phase:5-orchestration",
		"Blockers: docs",
		"Instructions: treat the workflow file as the contract",
		"Policy mode: gate",
		"Telemetry retention: 14",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("rendered body missing %q: %s", needle, rendered)
		}
	}
}

func TestLoadRejectsUnsafeTelemetryFrontMatter(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, filepath.Join(dir, defaultFilename), `---
policy:
  mode: gate
runtime:
  shared_workspace: true
telemetry:
  enabled: true
  sampling_rate: 1.1
  retention_days: 14
  allowed_readers:
    - maintainers
  export_requires_approval: true
---
Body
`)

	_, err := Load(dir, "")
	if err == nil || !strings.Contains(err.Error(), "telemetry.sampling_rate must be between 0 and 1") {
		t.Fatalf("expected telemetry validation error, got %v", err)
	}
}

func TestLoadFromExplicitPath(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "workflows", "review.md")
	writeWorkflow(t, explicit, `---
policy:
  mode: review
runtime:
  shared_workspace: false
---
Review {{.Issue.Number}}
`)

	doc, err := Load(dir, filepath.Join("workflows", "review.md"))
	if err != nil {
		t.Fatalf("load workflow from explicit path: %v", err)
	}
	if doc.Path != explicit {
		t.Fatalf("unexpected workflow path: %s", doc.Path)
	}
}

func TestLoadRejectsUnknownFrontMatterFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, defaultFilename)
	writeWorkflow(t, path, `---
policy:
  mode: gate
unknown_field: true
runtime:
  shared_workspace: true
---
Body
`)

	_, err := Load(dir, "")
	if err == nil || !strings.Contains(err.Error(), "field unknown_field not found") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestRenderRejectsMissingVariables(t *testing.T) {
	doc := Document{
		Path: "WORKFLOW.md",
		FrontMatter: FrontMatter{
			Policy:  Policy{Mode: "gate"},
			Runtime: Runtime{},
		},
		Body: "{{.Issue.DoesNotExist}}",
	}

	_, err := doc.Render(RenderContext{})
	if err == nil || !strings.Contains(err.Error(), "DoesNotExist") {
		t.Fatalf("expected missing variable error, got %v", err)
	}
}

func TestLoadRejectsMissingPolicyMode(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, filepath.Join(dir, defaultFilename), `---
runtime:
  shared_workspace: true
---
Body
`)

	_, err := Load(dir, "")
	if err == nil || !strings.Contains(err.Error(), "policy.mode is required") {
		t.Fatalf("expected policy validation error, got %v", err)
	}
}

func writeWorkflow(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
