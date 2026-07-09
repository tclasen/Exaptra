package runtrace

import (
	"encoding/json"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/meta"
	"github.com/tclasen/Exaptra/orchestration"
	"github.com/tclasen/Exaptra/profiles"
	"github.com/tclasen/Exaptra/stream"
	"github.com/tclasen/Exaptra/tracker"
	"github.com/tclasen/Exaptra/workflow"
	"github.com/tclasen/Exaptra/workspace"
)

// Snapshot captures inspectable state for a run.
type Snapshot struct {
	Config        config.Config            `json:"config"`
	Stream        stream.Trajectory        `json:"stream"`
	Registry      mcp.DiscoveryState       `json:"registry"`
	Audits        []meta.AuditRecord       `json:"audits"`
	Tracker       []tracker.AuditRecord    `json:"tracker"`
	Profile       *profiles.Selection      `json:"profile,omitempty"`
	Workspace     *workspace.Snapshot      `json:"workspace,omitempty"`
	Orchestration *orchestration.Aggregate `json:"orchestration,omitempty"`
	Workflow      *workflow.Trace          `json:"workflow,omitempty"`
}

// NewSnapshot collects a redacted, serializable run snapshot.
func NewSnapshot(cfg config.Config, s *stream.Stream, catalog *mcp.Catalog, audits []meta.AuditRecord, trackerAudits []tracker.AuditRecord, profile *profiles.Selection, workspaceSnapshot *workspace.Snapshot, orchestrationAggregate *orchestration.Aggregate, workflowTrace *workflow.Trace) Snapshot {
	var registry mcp.DiscoveryState
	if catalog != nil {
		registry = catalog.Snapshot()
	}
	var trajectory stream.Trajectory
	if s != nil {
		trajectory = s.Trajectory()
	}
	return Snapshot{
		Config:        cfg.Redacted(),
		Stream:        trajectory,
		Registry:      registry,
		Audits:        cloneAudits(audits),
		Tracker:       cloneTrackerAudits(trackerAudits),
		Profile:       profiles.CloneSelection(profile),
		Workspace:     cloneWorkspaceSnapshot(workspaceSnapshot),
		Orchestration: orchestration.CloneAggregate(orchestrationAggregate),
		Workflow:      workflow.CloneTrace(workflowTrace),
	}
}

// MarshalJSON keeps the serialized shape explicit and stable.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	type alias Snapshot
	return json.Marshal(alias(s))
}

// Redacted returns a copy of the snapshot with secret config values removed.
func (s Snapshot) Redacted() Snapshot {
	s.Config = s.Config.Redacted()
	s.Audits = cloneAudits(s.Audits)
	s.Tracker = cloneTrackerAudits(s.Tracker)
	s.Profile = profiles.CloneSelection(s.Profile)
	s.Workspace = cloneWorkspaceSnapshot(s.Workspace)
	s.Orchestration = orchestration.CloneAggregate(s.Orchestration)
	s.Workflow = workflow.CloneTrace(s.Workflow)
	return s
}

func cloneAudits(in []meta.AuditRecord) []meta.AuditRecord {
	if in == nil {
		return nil
	}
	out := make([]meta.AuditRecord, len(in))
	copy(out, in)
	return out
}

func cloneTrackerAudits(in []tracker.AuditRecord) []tracker.AuditRecord {
	if in == nil {
		return nil
	}
	out := make([]tracker.AuditRecord, len(in))
	copy(out, in)
	return out
}

func cloneWorkspaceSnapshot(in *workspace.Snapshot) *workspace.Snapshot {
	if in == nil {
		return nil
	}
	cloned := *in
	if len(in.States) != 0 {
		cloned.States = make([]workspace.State, len(in.States))
		copy(cloned.States, in.States)
	}
	return &cloned
}
