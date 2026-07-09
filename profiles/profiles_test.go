package profiles

import "testing"

func TestRegistryResolvesProviderAlignedProfile(t *testing.T) {
	registry := DefaultRegistry()

	selection, err := registry.Resolve(Input{
		Provider: "local",
		Model:    "example-model",
		Workflow: "example",
	})
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if selection.Name != "local-example" {
		t.Fatalf("selection name = %q, want local-example", selection.Name)
	}
	if !selection.AllowsTool("lookup") || selection.AllowsTool("search") {
		t.Fatalf("tool surface = %#v", selection.ToolSurface)
	}

	research, err := selection.ComposePrompt("research", "summarize the lookup output")
	if err != nil {
		t.Fatalf("compose research prompt: %v", err)
	}
	if research != "[local/example-model:research] summarize the lookup output" {
		t.Fatalf("research prompt = %q", research)
	}

	validate, err := selection.ComposePrompt("validate", "confirm handoff state")
	if err != nil {
		t.Fatalf("compose validate prompt: %v", err)
	}
	if validate != "[local/example-model:validate] confirm handoff state" {
		t.Fatalf("validate prompt = %q", validate)
	}
}

func TestRegistrySelectsProviderSpecificProfiles(t *testing.T) {
	registry := NewRegistry(
		Definition{Name: "fallback", ToolSurface: []string{"lookup"}},
		Definition{Name: "openai", Provider: "openai", ModelPrefix: "gpt-", ToolSurface: []string{"lookup", "search"}},
	)

	selection, err := registry.Resolve(Input{Provider: "openai", Model: "gpt-4.1", Workflow: "example"})
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if selection.Name != "openai" {
		t.Fatalf("selection name = %q, want openai", selection.Name)
	}
	if !selection.AllowsTool("search") {
		t.Fatalf("openai selection should allow search: %#v", selection.ToolSurface)
	}
}

func TestCloneSelectionDeepCopiesStyles(t *testing.T) {
	selection := &Selection{
		Name:        "local-example",
		Provider:    "local",
		Model:       "example-model",
		Workflow:    "example",
		ToolSurface: []string{"lookup"},
		Styles: map[string]Style{
			"default": {
				Prefix: "[local]",
			},
		},
	}

	cloned := CloneSelection(selection)
	cloned.ToolSurface[0] = "search"
	style := cloned.Styles["default"]
	style.Prefix = "[mutated]"
	cloned.Styles["default"] = style

	if selection.ToolSurface[0] != "lookup" {
		t.Fatalf("tool surface mutated: %#v", selection.ToolSurface)
	}
	if selection.Styles["default"].Prefix != "[local]" {
		t.Fatalf("styles mutated: %#v", selection.Styles)
	}
}
