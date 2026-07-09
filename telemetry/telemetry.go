package telemetry

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strings"

	"github.com/tclasen/Exaptra/config"
)

const (
	DecisionRecordType = "exaptra:telemetry_governance"
	StatusDiscarded    = "discarded"
	StatusExported     = "exported"
	StatusPending      = "pending_approval"
	RiskNormal         = "normal"
	RiskHigh           = "high"
)

// Event is the sanitized boundary shape for telemetry considered for export.
type Event struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Risk       string            `json:"risk,omitempty"`
}

// Decision records whether telemetry was exported, discarded, or redacted.
type Decision struct {
	Type                   string            `json:"type"`
	Event                  Event             `json:"event"`
	Status                 string            `json:"status"`
	Exported               bool              `json:"exported"`
	DiscardedReason        string            `json:"discarded_reason,omitempty"`
	PendingReason          string            `json:"pending_reason,omitempty"`
	SamplingRate           float64           `json:"sampling_rate"`
	SamplingScore          float64           `json:"sampling_score"`
	RetentionDays          int               `json:"retention_days,omitempty"`
	AllowedReaders         []string          `json:"allowed_readers,omitempty"`
	ExportRequiresApproval bool              `json:"export_requires_approval"`
	RedactedAttributes     []string          `json:"redacted_attributes,omitempty"`
	Attributes             map[string]string `json:"attributes,omitempty"`
}

// ApplyGovernance applies the configured export policy to one telemetry event.
func ApplyGovernance(policy config.TelemetryConfig, event Event, exportApproved bool) Decision {
	rate := policy.SamplingRate
	if event.Risk == RiskHigh {
		rate = policy.HighRiskSamplingRate
	}
	score := samplingScore(event)
	decision := Decision{
		Type:                   DecisionRecordType,
		Event:                  cloneEvent(event),
		Status:                 StatusDiscarded,
		SamplingRate:           rate,
		SamplingScore:          score,
		RetentionDays:          policy.RetentionDays,
		AllowedReaders:         append([]string(nil), policy.AllowedReaders...),
		ExportRequiresApproval: policy.ExportRequiresApproval,
	}
	if !policy.Enabled {
		decision.DiscardedReason = "telemetry disabled"
		return decision
	}
	if rate <= 0 {
		decision.DiscardedReason = "sampling policy discarded event"
		return decision
	}
	if score >= rate {
		decision.DiscardedReason = "sampling policy discarded event"
		return decision
	}
	if policy.RetentionDays <= 0 {
		decision.DiscardedReason = "retention policy forbids export"
		return decision
	}
	if len(policy.AllowedReaders) == 0 {
		decision.DiscardedReason = "access policy has no readers"
		return decision
	}

	decision.Attributes, decision.RedactedAttributes = redactAttributes(event.Attributes, policy.RedactAttributes)
	if policy.ExportRequiresApproval && !exportApproved {
		decision.Status = StatusPending
		decision.PendingReason = "export approval required"
		return decision
	}

	decision.Status = StatusExported
	decision.Exported = true
	return decision
}

func samplingScore(event Event) float64 {
	hash := sha256.New()
	hash.Write([]byte(event.Kind))
	hash.Write([]byte{0})
	hash.Write([]byte(event.Name))
	hash.Write([]byte{0})
	hash.Write([]byte(event.Risk))

	keys := make([]string, 0, len(event.Attributes))
	for key := range event.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		hash.Write([]byte{0})
		hash.Write([]byte(key))
		hash.Write([]byte("="))
		hash.Write([]byte(event.Attributes[key]))
	}

	sum := hash.Sum(nil)
	value := binary.BigEndian.Uint64(sum[:8])
	return float64(value) / float64(^uint64(0))
}

func cloneEvent(event Event) Event {
	event.Attributes = nil
	return event
}

func redactAttributes(attributes map[string]string, configured []string) (map[string]string, []string) {
	out := cloneStringMap(attributes)
	if len(out) == 0 {
		return nil, nil
	}
	configuredKeys := make(map[string]struct{}, len(configured))
	for _, key := range configured {
		configuredKeys[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}

	var redacted []string
	for key := range out {
		normalized := strings.ToLower(key)
		_, configured := configuredKeys[normalized]
		if configured || isSensitiveAttribute(normalized) {
			out[key] = "[redacted]"
			redacted = append(redacted, key)
		}
	}
	sort.Strings(redacted)
	return out, redacted
}

func isSensitiveAttribute(key string) bool {
	for _, marker := range []string{"api_key", "apikey", "authorization", "credential", "message", "password", "prompt", "secret"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return key == "token" || strings.HasSuffix(key, "_token") || strings.HasSuffix(key, ".token")
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
