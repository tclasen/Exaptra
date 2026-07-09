package workflowdoc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

const defaultFilename = "WORKFLOW.md"

// Document is a repository-owned workflow manifest with typed front matter and a templated body.
type Document struct {
	Path        string      `json:"path"`
	FrontMatter FrontMatter `json:"front_matter"`
	Body        string      `json:"body"`
}

// FrontMatter declares orchestration policy, prompt fragments, hooks, and runtime settings.
type FrontMatter struct {
	Policy  Policy  `json:"policy" yaml:"policy"`
	Prompts Prompts `json:"prompts" yaml:"prompts"`
	Hooks   []Hook  `json:"hooks,omitempty" yaml:"hooks,omitempty"`
	Runtime Runtime `json:"runtime" yaml:"runtime"`
}

// Policy controls the run contract.
type Policy struct {
	Mode      string   `json:"mode" yaml:"mode"`
	Labels    []string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Blockers  []string `json:"blockers,omitempty" yaml:"blockers,omitempty"`
	Target    string   `json:"target,omitempty" yaml:"target,omitempty"`
	AllowSkip bool     `json:"allow_skip,omitempty" yaml:"allow_skip,omitempty"`
}

// Prompts captures reusable prompt fragments for a workflow.
type Prompts struct {
	Orchestrator string `json:"orchestrator,omitempty" yaml:"orchestrator,omitempty"`
	Research     string `json:"research,omitempty" yaml:"research,omitempty"`
	Validate     string `json:"validate,omitempty" yaml:"validate,omitempty"`
}

// Hook defines one workflow hook.
type Hook struct {
	Event   string   `json:"event" yaml:"event"`
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args,omitempty" yaml:"args,omitempty"`
}

// Runtime captures execution settings for a workflow manifest.
type Runtime struct {
	SharedWorkspace bool `json:"shared_workspace,omitempty" yaml:"shared_workspace,omitempty"`
	MaxConcurrency  int  `json:"max_concurrency,omitempty" yaml:"max_concurrency,omitempty"`
}

// IssueContext contains the issue-specific values rendered into the workflow body.
type IssueContext struct {
	Owner        string   `json:"owner"`
	Repo         string   `json:"repo"`
	Number       int      `json:"number"`
	Title        string   `json:"title,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Blockers     []string `json:"blockers,omitempty"`
}

// RenderContext combines the issue context with the parsed front matter.
type RenderContext struct {
	Issue       IssueContext `json:"issue"`
	FrontMatter FrontMatter  `json:"front_matter"`
}

// Load resolves and parses a workflow document from a repository root or explicit path.
func Load(root, explicit string) (Document, error) {
	path, err := resolvePath(root, explicit)
	if err != nil {
		return Document{}, err
	}
	return LoadFile(path)
}

// LoadFile parses a workflow document from a file path.
func LoadFile(path string) (Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("workflowdoc: read %q: %w", path, err)
	}
	return Parse(path, raw)
}

// Parse parses a workflow document from raw bytes.
func Parse(path string, raw []byte) (Document, error) {
	frontMatterRaw, body, err := splitFrontMatter(raw)
	if err != nil {
		return Document{}, fmt.Errorf("workflowdoc: parse %q: %w", path, err)
	}

	var frontMatter FrontMatter
	decoder := yaml.NewDecoder(bytes.NewReader(frontMatterRaw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&frontMatter); err != nil {
		return Document{}, fmt.Errorf("workflowdoc: decode front matter in %q: %w", path, err)
	}
	if err := frontMatter.validate(); err != nil {
		return Document{}, fmt.Errorf("workflowdoc: invalid front matter in %q: %w", path, err)
	}
	if strings.TrimSpace(body) == "" {
		return Document{}, fmt.Errorf("workflowdoc: body in %q is required", path)
	}

	return Document{
		Path:        path,
		FrontMatter: frontMatter,
		Body:        body,
	}, nil
}

// Render executes the workflow body with strict variable handling.
func (d Document) Render(ctx RenderContext) (string, error) {
	tmpl, err := template.New(filepath.Base(d.Path)).
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"join": strings.Join,
		}).
		Parse(d.Body)
	if err != nil {
		return "", fmt.Errorf("workflowdoc: parse body in %q: %w", d.Path, err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, ctx); err != nil {
		return "", fmt.Errorf("workflowdoc: render body in %q: %w", d.Path, err)
	}
	return rendered.String(), nil
}

func (f FrontMatter) validate() error {
	var errs []error
	if strings.TrimSpace(f.Policy.Mode) == "" {
		errs = append(errs, errors.New("policy.mode is required"))
	}
	for i, hook := range f.Hooks {
		if strings.TrimSpace(hook.Event) == "" {
			errs = append(errs, fmt.Errorf("hooks[%d].event is required", i))
		}
		if strings.TrimSpace(hook.Command) == "" {
			errs = append(errs, fmt.Errorf("hooks[%d].command is required", i))
		}
	}
	if f.Runtime.MaxConcurrency < 0 {
		errs = append(errs, errors.New("runtime.max_concurrency must be zero or positive"))
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func resolvePath(root, explicit string) (string, error) {
	if explicit != "" {
		if filepath.IsAbs(explicit) || root == "" {
			return explicit, nil
		}
		return filepath.Join(root, explicit), nil
	}

	for _, candidate := range []string{
		filepath.Join(root, defaultFilename),
		filepath.Join(root, ".exaptra", defaultFilename),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("workflowdoc: stat %q: %w", candidate, err)
		}
	}
	return "", fmt.Errorf("workflowdoc: could not find %s under %q", defaultFilename, root)
}

func splitFrontMatter(raw []byte) ([]byte, string, error) {
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", errors.New("expected YAML front matter at the top of the file")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, "", errors.New("missing closing front matter delimiter")
	}

	frontMatter := strings.Join(lines[1:end], "\n")
	body := strings.Join(lines[end+1:], "\n")
	return []byte(frontMatter), body, nil
}
