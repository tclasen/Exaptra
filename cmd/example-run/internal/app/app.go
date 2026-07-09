package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/examples/localrun"
	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/meta"
	"github.com/tclasen/Exaptra/orchestration"
	"github.com/tclasen/Exaptra/profiles"
	"github.com/tclasen/Exaptra/runtrace"
	"github.com/tclasen/Exaptra/spend"
	"github.com/tclasen/Exaptra/stream"
	"github.com/tclasen/Exaptra/tracker"
	"github.com/tclasen/Exaptra/workflow"
	"github.com/tclasen/Exaptra/workflowdoc"
	"github.com/tclasen/Exaptra/workspace"
)

// Run executes the runnable example and writes the serialized run snapshot.
func Run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("example-run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "examples/localrun/config.example.json", "path to example config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFile(*configPath)
	if err != nil {
		return err
	}

	activeProfile, err := profiles.DefaultRegistry().Resolve(profiles.Input{
		Provider: cfg.Model.Provider,
		Model:    cfg.Model.Name,
		Workflow: "example",
	})
	if err != nil {
		return err
	}

	s := stream.New()
	provenance := &stream.Provenance{Source: "assistant", Provider: cfg.Model.Provider, Model: cfg.Model.Name}
	if err := s.Append(stream.UserMessage("msg_1", 1, "find the example record", provenance)); err != nil {
		return err
	}

	catalog := mcp.NewCatalog()
	catalog.Permissions().GrantMutations("example run")
	provider := localrun.Provider{}
	identity := mcp.Identity{Name: cfg.MCP[0].Name, Index: 0}
	if _, err := catalog.DiscoverFrom(context.Background(), identity, provider); err != nil {
		return err
	}

	dispatcher := mcp.NewDispatcher(catalog, resolverMap{
		identity.String(): provider,
	})

	if err := exposeProfileTools(catalog, identity, activeProfile); err != nil {
		return err
	}
	workflowRoot, err := findWorkflowRoot(".")
	if err != nil {
		return err
	}
	workflowManifest, err := workflowdoc.Load(workflowRoot, "")
	if err != nil {
		return err
	}
	if _, err := workflowManifest.Render(workflowdoc.RenderContext{
		Issue: workflowdoc.IssueContext{
			Owner:        "tclasen",
			Repo:         "Exaptra",
			Number:       46,
			Title:        "Define repository-owned workflow manifests",
			Instructions: "validate the repository-owned workflow contract before running the example harness",
			Labels:       []string{"enhancement", "phase:5-orchestration"},
			Blockers:     []string{"missing prerequisite work"},
		},
		FrontMatter: workflowManifest.FrontMatter,
	}); err != nil {
		return err
	}
	compactor, err := meta.NewStreamCompactor(meta.NewValidator("compact"), s, 3, meta.Identity{Name: "agent", Index: 1}, meta.Identity{Name: identity.Name, Index: identity.Index})
	if err != nil {
		return err
	}

	trackerStore := tracker.NewStore(nil)
	issue := tracker.IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 52}
	workspaceManager := workspace.NewManager(".exaptra/workspaces")
	workspaceState, err := workspaceManager.Claim(issue, "example-run")
	if err != nil {
		return err
	}

	executor := orchestration.NewExecutor(orchestration.WorkerFunc(func(ctx context.Context, task orchestration.Task) (orchestration.TaskResult, error) {
		payload, err := json.Marshal(map[string]any{
			"parent_run_id":    "example-run",
			"task":             task.ID,
			"prompt":           task.Prompt,
			"workspace":        task.Workspace,
			"shared_workspace": task.SharedWorkspace,
			"source":           "subagent",
			"source_trace":     "example-run:" + task.ID,
		})
		if err != nil {
			return orchestration.TaskResult{}, err
		}
		return orchestration.TaskResult{
			Output: payload,
			Provenance: &stream.Provenance{
				Source:    "subagent",
				Provider:  cfg.Model.Provider,
				Component: task.ID,
				TraceID:   "example-run:" + task.ID,
			},
		}, nil
	}), 2)
	var fanoutAggregate *orchestration.Aggregate
	researchPrompt, err := activeProfile.ComposePrompt("research", "summarize the lookup output")
	if err != nil {
		return err
	}
	validatePrompt, err := activeProfile.ComposePrompt("validate", "confirm handoff state and tracker writes")
	if err != nil {
		return err
	}
	workflowExecutor := workflow.NewExecutor(workflow.NodeRunnerFunc(func(ctx context.Context, node workflow.Node) (workflow.TaskResult, error) {
		switch node.Action {
		case "lookup":
			call, err := stream.FunctionCall("fc_1", 2, "lookup", "call_1", json.RawMessage(`{"query":"example"}`), provenance)
			if err != nil {
				return workflow.TaskResult{}, err
			}
			result, err := dispatcher.Invoke(ctx, s, call)
			if err != nil {
				return workflow.TaskResult{}, err
			}
			if err := s.Append(stream.AssistantMessage("msg_2", 4, "the example record was found", provenance)); err != nil {
				return workflow.TaskResult{}, err
			}
			return workflow.TaskResult{
				Output:     json.RawMessage(result.Output),
				Provenance: result.Provenance,
			}, nil
		case "compact":
			if _, err := compactor.Compact(s); err != nil {
				return workflow.TaskResult{}, err
			}
			payload, err := json.Marshal(map[string]any{
				"items":            len(s.Trajectory().Items),
				"meta_transitions": len(s.Trajectory().MetaTransitions),
			})
			if err != nil {
				return workflow.TaskResult{}, err
			}
			return workflow.TaskResult{
				Output: payload,
				Provenance: &stream.Provenance{
					Source:    "workflow",
					Provider:  cfg.Model.Provider,
					Component: "compact",
				},
			}, nil
		case "tracker_comment":
			audit, err := trackerStore.Comment(ctx, tracker.CommentRequest{
				RunID: "example-run",
				Issue: issue,
				Body:  "compaction complete; ready for review",
				Provenance: tracker.Provenance{
					RunID:     "example-run",
					Source:    "example-run",
					Component: "tracker",
				},
			})
			if err != nil {
				return workflow.TaskResult{}, err
			}
			payload, err := json.Marshal(audit)
			if err != nil {
				return workflow.TaskResult{}, err
			}
			return workflow.TaskResult{Output: payload}, nil
		case "tracker_state":
			audit, err := trackerStore.SetState(ctx, tracker.StateRequest{
				RunID:  "example-run",
				Issue:  issue,
				State:  tracker.HandoffStateReview,
				Reason: "workflow-defined review state",
				Provenance: tracker.Provenance{
					RunID:     "example-run",
					Source:    "example-run",
					Component: "tracker",
				},
			})
			if err != nil {
				return workflow.TaskResult{}, err
			}
			payload, err := json.Marshal(audit)
			if err != nil {
				return workflow.TaskResult{}, err
			}
			return workflow.TaskResult{Output: payload}, nil
		case "tracker_link":
			audit, err := trackerStore.LinkPullRequest(ctx, tracker.PullRequestLinkRequest{
				RunID: "example-run",
				Issue: issue,
				PullRequest: tracker.PullRequestRef{
					Owner:  "tclasen",
					Repo:   "Exaptra",
					Number: 99,
					URL:    "https://github.com/tclasen/Exaptra/pull/99",
				},
				State: tracker.HandoffStateReview,
				Provenance: tracker.Provenance{
					RunID:     "example-run",
					Source:    "example-run",
					Component: "tracker",
				},
			})
			if err != nil {
				return workflow.TaskResult{}, err
			}
			payload, err := json.Marshal(audit)
			if err != nil {
				return workflow.TaskResult{}, err
			}
			return workflow.TaskResult{Output: payload}, nil
		case "fanout":
			aggregate, err := executor.Execute(ctx, orchestration.Batch{
				ParentRunID: "example-run",
				Tasks: []orchestration.Task{
					{ID: "research", Prompt: researchPrompt, Workspace: "shared", SharedWorkspace: true, Provenance: &stream.Provenance{Source: "orchestrator", Provider: cfg.Model.Provider}},
					{ID: "validate", Prompt: validatePrompt, Workspace: "review", SharedWorkspace: false, Provenance: &stream.Provenance{Source: "orchestrator", Provider: cfg.Model.Provider}},
				},
			})
			if err != nil {
				return workflow.TaskResult{}, err
			}
			fanoutAggregate = &aggregate
			payload, err := json.Marshal(aggregate)
			if err != nil {
				return workflow.TaskResult{}, err
			}
			return workflow.TaskResult{Output: payload}, nil
		default:
			return workflow.TaskResult{}, fmt.Errorf("unknown workflow action %q", node.Action)
		}
	}))
	workflowTrace, err := workflowExecutor.Execute(context.Background(), workflow.Plan{
		ID:    "example",
		Start: "lookup",
		Nodes: []workflow.Node{
			{ID: "lookup", Kind: workflow.NodeKindTask, Action: "lookup", OnSuccess: "check_lookup"},
			{ID: "check_lookup", Kind: workflow.NodeKindGate, OutputContains: "lookup example", OnMatch: "compact"},
			{ID: "compact", Kind: workflow.NodeKindTask, Action: "compact", OnSuccess: "handoff"},
			{ID: "handoff", Kind: workflow.NodeKindSubplan, Subplan: "handoff"},
		},
		Subplans: []workflow.Plan{{
			ID:    "handoff",
			Start: "comment",
			Nodes: []workflow.Node{
				{ID: "comment", Kind: workflow.NodeKindTask, Action: "tracker_comment", OnSuccess: "state"},
				{ID: "state", Kind: workflow.NodeKindTask, Action: "tracker_state", OnSuccess: "link"},
				{ID: "link", Kind: workflow.NodeKindTask, Action: "tracker_link", OnSuccess: "fanout"},
				{ID: "fanout", Kind: workflow.NodeKindTask, Action: "fanout"},
			},
		}},
	})
	if err != nil {
		return err
	}

	spendWindow, err := time.ParseDuration(cfg.Spend.Window)
	if err != nil {
		return fmt.Errorf("example run: parse spend window: %w", err)
	}
	spendReport := spend.Summarize([]spend.Usage{
		{
			RunID:            "example-run",
			Provider:         cfg.Model.Provider,
			Model:            cfg.Model.Name,
			ObservedAt:       time.Date(2026, 7, 9, 20, 0, 0, 0, time.UTC),
			InputTokens:      320,
			OutputTokens:     140,
			EstimatedCostUSD: spend.EstimateCostUSD(320, 140, 0.0, 0.0),
		},
		{
			RunID:            "example-run",
			Provider:         cfg.Model.Provider,
			Model:            cfg.Model.Name,
			ObservedAt:       time.Date(2026, 7, 9, 20, 15, 0, 0, time.UTC),
			InputTokens:      120,
			OutputTokens:     60,
			EstimatedCostUSD: spend.EstimateCostUSD(120, 60, 0.0, 0.0),
		},
	}, spendBudgets(cfg.Spend.Budgets), spendWindow)

	snapshot := runtrace.NewSnapshot(cfg, s, catalog, compactor.Audits(), trackerStore.Audits(), &activeProfile, &workspace.Snapshot{Root: ".exaptra/workspaces", States: []workspace.State{workspaceState}}, fanoutAggregate, &workflowTrace, &spendReport)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return err
	}
	return nil
}

func spendBudgets(budgets []config.SpendBudget) []spend.Budget {
	out := make([]spend.Budget, len(budgets))
	for i, budget := range budgets {
		out[i] = spend.Budget{
			Name:       budget.Name,
			Provider:   budget.Provider,
			Model:      budget.Model,
			MaxTokens:  budget.MaxTokens,
			MaxCostUSD: budget.MaxCostUSD,
		}
	}
	return out
}

type resolverMap map[string]mcp.ToolCaller

func (r resolverMap) ResolveToolCaller(identity mcp.Identity) (mcp.ToolCaller, bool) {
	caller, ok := r[identity.String()]
	return caller, ok
}

func exposeProfileTools(catalog *mcp.Catalog, identity mcp.Identity, profile profiles.Selection) error {
	discoveredTools := make(map[string]struct{})
	for _, tool := range catalog.Snapshot().Discovered {
		discoveredTools[tool.Name] = struct{}{}
	}

	lookupExposed := false
	for _, toolName := range profile.ToolSurface {
		if _, ok := discoveredTools[toolName]; !ok {
			continue
		}
		if err := catalog.Expose(identity, toolName); err != nil {
			return err
		}
		if toolName == "lookup" {
			lookupExposed = true
		}
	}

	if !profile.AllowsTool("lookup") {
		return fmt.Errorf("profile %q does not allow the lookup tool required by the example workflow", profile.Name)
	}
	if !lookupExposed {
		return fmt.Errorf("profile %q requires the lookup tool but it was not discovered", profile.Name)
	}
	return nil
}

func findWorkflowRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "WORKFLOW.md")); err == nil {
			return dir, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("example run: could not locate WORKFLOW.md from %q", start)
}
