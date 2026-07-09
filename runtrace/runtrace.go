package runtrace

import (
	"encoding/json"

	"github.com/tclasen/Exaptra/config"
	"github.com/tclasen/Exaptra/mcp"
	"github.com/tclasen/Exaptra/meta"
	"github.com/tclasen/Exaptra/stream"
	"github.com/tclasen/Exaptra/tracker"
)

// Snapshot captures inspectable state for a run.
type Snapshot struct {
	Config   config.Config         `json:"config"`
	Stream   stream.Trajectory     `json:"stream"`
	Registry mcp.DiscoveryState    `json:"registry"`
	Audits   []meta.AuditRecord    `json:"audits"`
	Tracker  []tracker.AuditRecord `json:"tracker"`
}

// NewSnapshot collects a redacted, serializable run snapshot.
func NewSnapshot(cfg config.Config, s *stream.Stream, catalog *mcp.Catalog, audits []meta.AuditRecord, trackerAudits []tracker.AuditRecord) Snapshot {
	var registry mcp.DiscoveryState
	if catalog != nil {
		registry = catalog.Snapshot()
	}
	var trajectory stream.Trajectory
	if s != nil {
		trajectory = s.Trajectory()
	}
	return Snapshot{
		Config:   cfg.Redacted(),
		Stream:   trajectory,
		Registry: registry,
		Audits:   cloneAudits(audits),
		Tracker:  cloneTrackerAudits(trackerAudits),
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
