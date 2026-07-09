package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tclasen/Exaptra/stream"
)

func TestExecutorRunsNestedSubplanWithGateRouting(t *testing.T) {
	runner := NodeRunnerFunc(func(ctx context.Context, node Node) (TaskResult, error) {
		switch node.Action {
		case "lookup":
			return TaskResult{
				Output: json.RawMessage(`{"result":"example record found"}`),
				Provenance: &stream.Provenance{
					Source:    "example",
					Component: "lookup",
					TraceID:   "lookup-trace",
				},
			}, nil
		case "compact":
			return TaskResult{
				Output: json.RawMessage(`{"items":1}`),
				Provenance: &stream.Provenance{
					Source:    "example",
					Component: "compact",
					TraceID:   "compact-trace",
				},
			}, nil
		case "comment":
			return TaskResult{Output: json.RawMessage(`{"commented":true}`)}, nil
		case "state":
			return TaskResult{Output: json.RawMessage(`{"state":"review_ready"}`)}, nil
		case "link":
			return TaskResult{Output: json.RawMessage(`{"linked":true}`)}, nil
		default:
			return TaskResult{}, errors.New("unexpected action: " + node.Action)
		}
	})

	plan := Plan{
		ID:    "example",
		Start: "lookup",
		Nodes: []Node{
			{ID: "lookup", Kind: NodeKindTask, Action: "lookup", OnSuccess: "check"},
			{ID: "check", Kind: NodeKindGate, OutputContains: "example record", OnMatch: "compact"},
			{ID: "compact", Kind: NodeKindTask, Action: "compact", OnSuccess: "handoff"},
			{ID: "handoff", Kind: NodeKindSubplan, Subplan: "handoff"},
		},
		Subplans: []Plan{
			{
				ID:    "handoff",
				Start: "comment",
				Nodes: []Node{
					{ID: "comment", Kind: NodeKindTask, Action: "comment", OnSuccess: "state"},
					{ID: "state", Kind: NodeKindTask, Action: "state", OnSuccess: "link"},
					{ID: "link", Kind: NodeKindTask, Action: "link"},
				},
			},
		},
	}

	executor := NewExecutor(runner)
	trace, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	if trace.Completed != 6 {
		t.Fatalf("completed = %d, want 6", trace.Completed)
	}
	if trace.Failed != 0 {
		t.Fatalf("failed = %d, want 0", trace.Failed)
	}
	if len(trace.Records) != 7 {
		t.Fatalf("record len = %d, want 7", len(trace.Records))
	}
	if trace.Records[1].Status != StatusPassed || !trace.Records[1].Matched {
		t.Fatalf("gate record = %#v, want passed and matched", trace.Records[1])
	}
	if trace.Records[3].Depth != 1 || trace.Records[3].PlanID != "handoff" {
		t.Fatalf("nested subplan record depth/plan = %#v", trace.Records[3])
	}
	if trace.Records[6].Depth != 0 || trace.Records[6].Node.ID != "handoff" {
		t.Fatalf("subplan summary record = %#v", trace.Records[6])
	}
	if trace.Plan == nil || trace.Plan.ID != "example" {
		t.Fatalf("trace plan = %#v, want example plan", trace.Plan)
	}
	if trace.Records[0].Provenance == nil || trace.Records[0].Provenance.TraceID != "lookup-trace" {
		t.Fatalf("lookup provenance = %#v", trace.Records[0].Provenance)
	}
}

func TestPlanValidationRejectsMissingReferencesAndCycles(t *testing.T) {
	missing := Plan{
		ID:    "missing",
		Start: "start",
		Nodes: []Node{{ID: "start", Kind: NodeKindTask, Action: "lookup", OnSuccess: "nope"}},
	}
	if err := missing.Validate(); err == nil || !strings.Contains(err.Error(), "unknown node") {
		t.Fatalf("missing reference validation = %v", err)
	}

	cyclic := Plan{
		ID:    "cycle",
		Start: "a",
		Nodes: []Node{
			{ID: "a", Kind: NodeKindTask, Action: "lookup", OnSuccess: "b"},
			{ID: "b", Kind: NodeKindTask, Action: "compact", OnSuccess: "a"},
		},
	}
	if err := cyclic.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle validation = %v", err)
	}

	duplicateSubplans := Plan{
		ID:    "dup",
		Start: "start",
		Nodes: []Node{{ID: "start", Kind: NodeKindTask, Action: "lookup"}},
		Subplans: []Plan{
			{ID: "shared", Start: "a", Nodes: []Node{{ID: "a", Kind: NodeKindTask, Action: "compact"}}},
			{ID: "shared", Start: "b", Nodes: []Node{{ID: "b", Kind: NodeKindTask, Action: "compact"}}},
		},
	}
	if err := duplicateSubplans.Validate(); err == nil || !strings.Contains(err.Error(), "defined more than once") {
		t.Fatalf("duplicate subplan validation = %v", err)
	}
}

func TestCloneTraceDeepCopies(t *testing.T) {
	trace := &Trace{
		PlanID: "plan-1",
		Records: []Record{{
			PlanID: "plan-1",
			Node: Node{
				ID:         "n1",
				Kind:       NodeKindTask,
				Action:     "lookup",
				Provenance: &stream.Provenance{Source: "workflow", Component: "n1", TraceID: "trace-1"},
			},
			Output:     json.RawMessage(`{"ok":true}`),
			Provenance: &stream.Provenance{TraceID: "trace-2"},
		}},
		Plan: &Plan{
			ID:    "plan-1",
			Start: "n1",
			Nodes: []Node{{ID: "n1", Kind: NodeKindTask, Action: "lookup"}},
		},
	}

	cloned := CloneTrace(trace)
	cloned.Records[0].Output[0] = 'x'
	cloned.Records[0].Node.Provenance.Source = "mutated"
	cloned.Plan.Nodes[0].Action = "changed"

	if string(trace.Records[0].Output) != `{"ok":true}` {
		t.Fatalf("trace output mutated: %s", trace.Records[0].Output)
	}
	if trace.Records[0].Node.Provenance.Source != "workflow" {
		t.Fatalf("trace provenance mutated: %#v", trace.Records[0].Node.Provenance)
	}
	if trace.Plan.Nodes[0].Action != "lookup" {
		t.Fatalf("trace plan mutated: %#v", trace.Plan.Nodes[0])
	}
}
