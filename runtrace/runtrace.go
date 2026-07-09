package runtrace

import (
	"encoding/json"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/meta"
	"github.com/tclasen/Exaptra/orchestration"
	"github.com/tclasen/Exaptra/profiles"
	"github.com/tclasen/Exaptra/spend"
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
	Spend         *spend.Report            `json:"spend,omitempty"`
}

// NewSnapshot collects a redacted, serializable run snapshot.
func NewSnapshot(cfg config.Config, s *stream.Stream, catalog *mcp.Catalog, audits []meta.AuditRecord, trackerAudits []tracker.AuditRecord, profile *profiles.Selection, workspaceSnapshot *workspace.Snapshot, orchestrationAggregate *orchestration.Aggregate, workflowTrace *workflow.Trace, spendReport *spend.Report) Snapshot {
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
		Spend:         cloneSpendReport(spendReport),
	}
}

// MarshalJSON keeps the serialized shape explicit and stable.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	type alias Snapshot
	return json.Marshal(alias(s))
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

func cloneSpendReport(in *spend.Report) *spend.Report {
	if in == nil {
		return nil
	}
	cloned := *in
	if len(in.Windows) != 0 {
		cloned.Windows = make([]spend.Window, len(in.Windows))
		for i, window := range in.Windows {
			cloned.Windows[i] = window
			cloned.Windows[i].Alerts = append([]spend.Alert(nil), window.Alerts...)
		}
	}
	return &cloned
}
