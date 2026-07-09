package profiles

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// Input identifies the active runtime context for profile resolution.
type Input struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Workflow string `json:"workflow"`
}

// Style customizes prompt composition for one stage.
type Style struct {
	Prefix   string `json:"prefix,omitempty"`
	Template string `json:"template,omitempty"`
	Suffix   string `json:"suffix,omitempty"`
}

// Definition declares one provider-aligned profile.
type Definition struct {
	Name        string           `json:"name"`
	Provider    string           `json:"provider,omitempty"`
	ModelPrefix string           `json:"model_prefix,omitempty"`
	Workflow    string           `json:"workflow,omitempty"`
	ToolSurface []string         `json:"tool_surface,omitempty"`
	ToolMode    string           `json:"tool_mode,omitempty"`
	Styles      map[string]Style `json:"styles,omitempty"`
}

// Selection is the resolved profile used by a run.
type Selection struct {
	Name        string           `json:"name"`
	Provider    string           `json:"provider"`
	Model       string           `json:"model"`
	Workflow    string           `json:"workflow"`
	ToolSurface []string         `json:"tool_surface,omitempty"`
	ToolMode    string           `json:"tool_mode,omitempty"`
	Styles      map[string]Style `json:"styles,omitempty"`
}

// Registry resolves profiles from runtime input.
type Registry struct {
	definitions []Definition
}

// NewRegistry constructs a profile registry.
func NewRegistry(definitions ...Definition) *Registry {
	return &Registry{definitions: append([]Definition(nil), definitions...)}
}

// DefaultRegistry returns the built-in profile set used by the example run.
func DefaultRegistry() *Registry {
	return NewRegistry(
		Definition{
			Name:        "local-example",
			Provider:    "local",
			ModelPrefix: "example-model",
			Workflow:    "example",
			ToolSurface: []string{"lookup"},
			ToolMode:    "allowlist",
			Styles: map[string]Style{
				"default": {
					Prefix: "[local/example-model]",
				},
				"research": {
					Prefix: "[local/example-model:research]",
				},
				"validate": {
					Prefix: "[local/example-model:validate]",
				},
			},
		},
		Definition{
			Name:        "openai-general",
			Provider:    "openai",
			ModelPrefix: "gpt-",
			ToolSurface: []string{"lookup", "search"},
			ToolMode:    "allowlist",
			Styles: map[string]Style{
				"default": {
					Prefix: "[openai]",
				},
				"research": {
					Prefix: "[openai:research]",
				},
				"validate": {
					Prefix: "[openai:validate]",
				},
			},
		},
	)
}

// Resolve selects the best matching profile for the given input.
func (r *Registry) Resolve(input Input) (Selection, error) {
	if r == nil || len(r.definitions) == 0 {
		return Selection{}, fmt.Errorf("profiles: no definitions available for provider %q model %q workflow %q", input.Provider, input.Model, input.Workflow)
	}

	bestIndex := -1
	bestScore := -1
	for i := range r.definitions {
		def := r.definitions[i]
		if !def.matches(input) {
			continue
		}
		score := def.specificity()
		if score > bestScore {
			bestIndex = i
			bestScore = score
		}
	}

	if bestIndex < 0 {
		return Selection{}, fmt.Errorf("profiles: no profile matches provider %q model %q workflow %q", input.Provider, input.Model, input.Workflow)
	}

	return r.definitions[bestIndex].selection(input), nil
}

func (d Definition) matches(input Input) bool {
	if d.Provider != "" && d.Provider != input.Provider {
		return false
	}
	if d.ModelPrefix != "" && !strings.HasPrefix(input.Model, d.ModelPrefix) {
		return false
	}
	if d.Workflow != "" && d.Workflow != input.Workflow {
		return false
	}
	return true
}

func (d Definition) specificity() int {
	score := 0
	if d.Provider != "" {
		score++
	}
	if d.ModelPrefix != "" {
		score++
	}
	if d.Workflow != "" {
		score++
	}
	if d.ToolMode != "" {
		score++
	}
	if len(d.ToolSurface) != 0 {
		score++
	}
	if len(d.Styles) != 0 {
		score++
	}
	return score
}

func (d Definition) selection(input Input) Selection {
	return Selection{
		Name:        d.Name,
		Provider:    input.Provider,
		Model:       input.Model,
		Workflow:    input.Workflow,
		ToolSurface: append([]string(nil), d.ToolSurface...),
		ToolMode:    d.ToolMode,
		Styles:      cloneStyles(d.Styles),
	}
}

// ComposePrompt applies the stage stylesheet to the given base prompt.
func (s Selection) ComposePrompt(stage, base string) (string, error) {
	style := s.styleFor(stage)
	if style.Template != "" {
		tmpl, err := template.New("profile-prompt").Parse(style.Template)
		if err != nil {
			return "", fmt.Errorf("profiles: parse prompt template for stage %q: %w", stage, err)
		}
		var rendered bytes.Buffer
		if err := tmpl.Execute(&rendered, map[string]string{
			"Base":     base,
			"Provider": s.Provider,
			"Model":    s.Model,
			"Workflow": s.Workflow,
			"Stage":    stage,
		}); err != nil {
			return "", fmt.Errorf("profiles: render prompt template for stage %q: %w", stage, err)
		}
		return rendered.String(), nil
	}

	var parts []string
	if style.Prefix != "" {
		parts = append(parts, style.Prefix)
	}
	if base != "" {
		parts = append(parts, base)
	}
	if style.Suffix != "" {
		parts = append(parts, style.Suffix)
	}
	return strings.Join(parts, " "), nil
}

// AllowsTool reports whether the profile exposes the named tool.
func (s Selection) AllowsTool(name string) bool {
	for _, tool := range s.ToolSurface {
		if tool == name {
			return true
		}
	}
	return false
}

func (s Selection) styleFor(stage string) Style {
	if len(s.Styles) == 0 {
		return Style{}
	}
	if style, ok := s.Styles[stage]; ok {
		return style
	}
	if style, ok := s.Styles["default"]; ok {
		return style
	}
	return Style{}
}

// CloneSelection returns a deep copy of a selection.
func CloneSelection(selection *Selection) *Selection {
	if selection == nil {
		return nil
	}
	cloned := *selection
	cloned.ToolSurface = append([]string(nil), selection.ToolSurface...)
	cloned.Styles = cloneStyles(selection.Styles)
	return &cloned
}

func cloneStyles(in map[string]Style) map[string]Style {
	if in == nil {
		return nil
	}
	out := make(map[string]Style, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
