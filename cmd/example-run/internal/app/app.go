package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/examples/localrun"
	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/meta"
	"github.com/tclasen/Exaptra/orchestration"
	"github.com/tclasen/Exaptra/runtrace"
	"github.com/tclasen/Exaptra/stream"
	"github.com/tclasen/Exaptra/tracker"
	"github.com/tclasen/Exaptra/workflow"
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
	if err := catalog.Expose(identity, "lookup"); err != nil {
		return err
	}

	dispatcher := mcp.NewDispatcher(catalog, resolverMap{
		identity.String(): provider,
	})

	compactor, err := meta.NewStreamCompactor(meta.NewValidator("compact"), s, 3, meta.Identity{Name: "agent", Index: 1}, meta.Identity{Name: identity.Name, Index: identity.Index})
	if err != nil {
		return err
	}

	trackerStore := tracker.NewStore(nil)
	issue := tracker.IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 52}

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
					{ID: "research", Prompt: "summarize the lookup output", Workspace: "shared", SharedWorkspace: true, Provenance: &stream.Provenance{Source: "orchestrator", Provider: cfg.Model.Provider}},
					{ID: "validate", Prompt: "confirm handoff state and tracker writes", Workspace: "review", SharedWorkspace: false, Provenance: &stream.Provenance{Source: "orchestrator", Provider: cfg.Model.Provider}},
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

	snapshot := runtrace.NewSnapshot(cfg, s, catalog, compactor.Audits(), trackerStore.Audits(), fanoutAggregate, &workflowTrace)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return err
	}
	return nil
}

type resolverMap map[string]mcp.ToolCaller

func (r resolverMap) ResolveToolCaller(identity mcp.Identity) (mcp.ToolCaller, bool) {
	caller, ok := r[identity.String()]
	return caller, ok
}
