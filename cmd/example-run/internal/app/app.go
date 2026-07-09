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

	call, err := stream.FunctionCall("fc_1", 2, "lookup", "call_1", json.RawMessage(`{"query":"example"}`), provenance)
	if err != nil {
		return err
	}
	if _, err := dispatcher.Invoke(context.Background(), s, call); err != nil {
		return err
	}
	if err := s.Append(stream.AssistantMessage("msg_2", 4, "the example record was found", provenance)); err != nil {
		return err
	}

	compactor, err := meta.NewStreamCompactor(meta.NewValidator("compact"), s, 3, meta.Identity{Name: "agent", Index: 1}, meta.Identity{Name: identity.Name, Index: identity.Index})
	if err != nil {
		return err
	}
	if _, err := compactor.Compact(s); err != nil {
		return err
	}

	trackerStore := tracker.NewStore(nil)
	issue := tracker.IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 52}
	if _, err := trackerStore.Comment(context.Background(), tracker.CommentRequest{
		RunID: "example-run",
		Issue: issue,
		Body:  "compaction complete; ready for review",
		Provenance: tracker.Provenance{
			RunID:     "example-run",
			Source:    "example-run",
			Component: "tracker",
		},
	}); err != nil {
		return err
	}
	if _, err := trackerStore.SetState(context.Background(), tracker.StateRequest{
		RunID:  "example-run",
		Issue:  issue,
		State:  tracker.HandoffStateReview,
		Reason: "workflow-defined review state",
		Provenance: tracker.Provenance{
			RunID:     "example-run",
			Source:    "example-run",
			Component: "tracker",
		},
	}); err != nil {
		return err
	}
	if _, err := trackerStore.LinkPullRequest(context.Background(), tracker.PullRequestLinkRequest{
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
	}); err != nil {
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
	batch, err := executor.Execute(context.Background(), orchestration.Batch{
		ParentRunID: "example-run",
		Tasks: []orchestration.Task{
			{ID: "research", Prompt: "summarize the lookup output", Workspace: "shared", SharedWorkspace: true, Provenance: &stream.Provenance{Source: "orchestrator", Provider: cfg.Model.Provider}},
			{ID: "validate", Prompt: "confirm handoff state and tracker writes", Workspace: "review", SharedWorkspace: false, Provenance: &stream.Provenance{Source: "orchestrator", Provider: cfg.Model.Provider}},
		},
	})
	if err != nil {
		return err
	}

	snapshot := runtrace.NewSnapshot(cfg, s, catalog, compactor.Audits(), trackerStore.Audits(), &batch)
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
