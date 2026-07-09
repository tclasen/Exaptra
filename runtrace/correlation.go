package runtrace

import (
	"fmt"
	"sort"

	"github.com/tclasen/Exaptra/orchestration"
	"github.com/tclasen/Exaptra/stream"
	"github.com/tclasen/Exaptra/tracker"
	"github.com/tclasen/Exaptra/workflow"
)

// CorrelationPath is the non-sensitive decision path for matching local audit
// records to exported telemetry traces.
type CorrelationPath struct {
	RunID    string            `json:"run_id"`
	ThreadID string            `json:"thread_id,omitempty"`
	Issue    tracker.IssueRef  `json:"issue"`
	Links    []CorrelationLink `json:"links"`
}

// CorrelationLink identifies one ordered step in the run tree.
type CorrelationLink struct {
	Sequence    int               `json:"sequence"`
	Kind        string            `json:"kind"`
	ID          string            `json:"id"`
	ParentID    string            `json:"parent_id,omitempty"`
	RunID       string            `json:"run_id"`
	ThreadID    string            `json:"thread_id,omitempty"`
	Issue       string            `json:"issue,omitempty"`
	TraceID     string            `json:"trace_id"`
	Source      string            `json:"source,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	Model       string            `json:"model,omitempty"`
	Component   string            `json:"component,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Terminal    bool              `json:"terminal,omitempty"`
	TerminalFor string            `json:"terminal_for,omitempty"`
}

// NewCorrelationPath builds a replayable, content-free path across run records.
func NewCorrelationPath(runID, threadID string, issue tracker.IssueRef, trajectory stream.Trajectory, workflowTrace *workflow.Trace, aggregate *orchestration.Aggregate, trackerAudits []tracker.AuditRecord) *CorrelationPath {
	path := &CorrelationPath{
		RunID:    runID,
		ThreadID: threadID,
		Issue:    issue,
	}
	issueID := issue.String()
	path.Links = append(path.Links, CorrelationLink{
		Kind:      "run",
		ID:        runID,
		RunID:     runID,
		ThreadID:  threadID,
		Issue:     issueID,
		TraceID:   runID,
		Source:    "run",
		Component: "entrypoint",
	})

	for _, item := range trajectory.Items {
		path.Links = append(path.Links, linkFromProvenance("stream."+item.Type, item.ID, runID, threadID, issueID, runID, item.Provenance, map[string]string{
			"sequence": fmt.Sprintf("%d", item.Sequence),
			"status":   item.Status,
		}))
	}
	for _, transition := range trajectory.MetaTransitions {
		path.Links = append(path.Links, linkFromProvenance("meta."+transition.Operation, transition.ID, runID, threadID, issueID, runID, transition.Provenance, map[string]string{
			"sequence": fmt.Sprintf("%d", transition.Sequence),
			"target":   transition.Target,
		}))
	}
	if workflowTrace != nil {
		for _, record := range workflowTrace.Records {
			path.Links = append(path.Links, linkFromProvenance("workflow."+record.Node.Kind, record.Node.ID, runID, threadID, issueID, runID, record.Provenance, map[string]string{
				"plan_id": workflowTrace.PlanID,
				"status":  record.Status,
			}))
		}
	}
	if aggregate != nil {
		for _, outcome := range aggregate.Outcomes {
			parentID := aggregate.ParentRunID
			if parentID == "" {
				parentID = runID
			}
			path.Links = append(path.Links, linkFromProvenance("orchestration.subagent", outcome.Task.ID, runID, threadID, issueID, parentID, outcome.Provenance, map[string]string{
				"status": outcome.Status,
			}))
		}
	}
	for _, audit := range trackerAudits {
		path.Links = append(path.Links, CorrelationLink{
			Kind:      "tracker." + audit.Operation,
			ID:        fmt.Sprintf("tracker-%d", audit.Sequence),
			ParentID:  runID,
			RunID:     runID,
			ThreadID:  threadID,
			Issue:     issueID,
			TraceID:   firstNonEmpty(audit.Provenance.TraceID, audit.RunID, runID),
			Source:    audit.Provenance.Source,
			Component: audit.Provenance.Component,
			Attributes: map[string]string{
				"operation": audit.Operation,
				"result":    audit.Result,
			},
		})
	}
	if len(path.Links) != 0 {
		path.Links[len(path.Links)-1].Terminal = true
		path.Links[len(path.Links)-1].TerminalFor = runID
	}
	for i := range path.Links {
		path.Links[i].Sequence = i + 1
	}
	return path
}

func linkFromProvenance(kind, id, runID, threadID, issue, parentID string, provenance *stream.Provenance, attrs map[string]string) CorrelationLink {
	link := CorrelationLink{
		Kind:       kind,
		ID:         id,
		ParentID:   parentID,
		RunID:      runID,
		ThreadID:   threadID,
		Issue:      issue,
		TraceID:    id,
		Attributes: cloneStringMap(attrs),
	}
	if provenance != nil {
		link.TraceID = firstNonEmpty(provenance.TraceID, id)
		link.Source = provenance.Source
		link.Provider = provenance.Provider
		link.Model = provenance.Model
		link.Component = provenance.Component
	}
	return link
}

func cloneCorrelationPath(in *CorrelationPath) *CorrelationPath {
	if in == nil {
		return nil
	}
	cloned := *in
	cloned.Links = make([]CorrelationLink, len(in.Links))
	for i, link := range in.Links {
		cloned.Links[i] = link
		cloned.Links[i].Attributes = cloneStringMap(link.Attributes)
	}
	return &cloned
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(in))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
