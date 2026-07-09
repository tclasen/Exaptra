package runtrace

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/orchestration"
	"github.com/tclasen/Exaptra/stream"
	"github.com/tclasen/Exaptra/tracker"
	"github.com/tclasen/Exaptra/workflow"
)

func TestCorrelationPathLinksRunTreeAcrossBoundaries(t *testing.T) {
	s := stream.New()
	provenance := &stream.Provenance{Source: "assistant", Provider: "local", Model: "example-model", Component: "entry", TraceID: "trace-entry"}
	if err := s.Append(stream.UserMessage("msg-1", 1, "sensitive prompt content", provenance)); err != nil {
		t.Fatalf("append message: %v", err)
	}
	transition, err := stream.NewMetaTransition("meta-1", 2, "compact", "stream.context", json.RawMessage(`{"items":2}`), json.RawMessage(`{"items":1}`), &stream.Provenance{Source: "meta", Component: "compact", TraceID: "trace-compact"})
	if err != nil {
		t.Fatalf("build transition: %v", err)
	}
	if err := s.AppendMetaTransition(transition); err != nil {
		t.Fatalf("append transition: %v", err)
	}

	issue := tracker.IssueRef{Owner: "tclasen", Repo: "Exaptra", Number: 66}
	store := tracker.NewStore(nil)
	if _, err := store.Comment(context.Background(), tracker.CommentRequest{
		RunID: "run-1",
		Issue: issue,
		Body:  "sensitive tracker comment",
		Provenance: tracker.Provenance{
			RunID:     "run-1",
			Source:    "workflow",
			Component: "tracker",
			TraceID:   "trace-tracker",
		},
	}); err != nil {
		t.Fatalf("tracker comment: %v", err)
	}

	workflowTrace := &workflow.Trace{
		PlanID: "example",
		Records: []workflow.Record{{
			PlanID: "example",
			Node: workflow.Node{
				ID:   "lookup",
				Kind: workflow.NodeKindTask,
			},
			Status:     workflow.StatusCompleted,
			Provenance: &stream.Provenance{Source: "workflow", Component: "lookup", TraceID: "trace-workflow"},
		}},
	}
	aggregate := &orchestration.Aggregate{
		ParentRunID: "run-1",
		Outcomes: []orchestration.Outcome{{
			Task:       orchestration.Task{ID: "research"},
			Status:     orchestration.StatusCompleted,
			Provenance: &stream.Provenance{Source: "subagent", Component: "research", TraceID: "trace-subagent"},
		}},
	}

	path := NewCorrelationPath("run-1", "thread-1", issue, s.Trajectory(), workflowTrace, aggregate, store.Audits())
	encoded, err := json.Marshal(path)
	if err != nil {
		t.Fatalf("marshal path: %v", err)
	}
	for _, needle := range []string{"trace-entry", "trace-compact", "trace-workflow", "trace-subagent", "trace-tracker", "thread-1", "tclasen/Exaptra#66"} {
		if !strings.Contains(string(encoded), needle) {
			t.Fatalf("correlation path missing %q: %s", needle, encoded)
		}
	}
	for _, leaked := range []string{"sensitive prompt content", "sensitive tracker comment"} {
		if strings.Contains(string(encoded), leaked) {
			t.Fatalf("correlation path leaked content %q: %s", leaked, encoded)
		}
	}
	if !path.Links[len(path.Links)-1].Terminal {
		t.Fatalf("last link not marked terminal: %#v", path.Links[len(path.Links)-1])
	}
}

func TestCorrelationPathIsClonedInSnapshot(t *testing.T) {
	path := &CorrelationPath{
		RunID:    "run-1",
		ThreadID: "thread-1",
		Links: []CorrelationLink{{
			Kind:    "run",
			ID:      "run-1",
			RunID:   "run-1",
			TraceID: "run-1",
			Attributes: map[string]string{
				"status": "completed",
			},
		}},
	}

	snapshot := NewSnapshot(config.Config{}, nil, nil, nil, nil, nil, nil, nil, nil, path)
	snapshot.Correlation.Links[0].Attributes["status"] = "mutated"

	if path.Links[0].Attributes["status"] != "completed" {
		t.Fatalf("snapshot mutated original correlation path: %#v", path.Links[0].Attributes)
	}
}
