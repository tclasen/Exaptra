package telemetry

import (
	"testing"

	"github.com/tclasen/Exaptra/config"
)

func TestApplyGovernanceRedactsSensitiveAttributes(t *testing.T) {
	decision := ApplyGovernance(config.TelemetryConfig{
		Enabled:                true,
		SamplingRate:           1,
		HighRiskSamplingRate:   0.5,
		RetentionDays:          14,
		AllowedReaders:         []string{"maintainers"},
		ExportRequiresApproval: true,
		RedactAttributes:       []string{"custom_field"},
	}, Event{
		Kind: "span",
		Name: "model.call",
		Attributes: map[string]string{
			"custom_field": "private",
			"prompt":       "summarize this secret",
			"token_count":  "12",
		},
	}, true)

	if !decision.Exported {
		t.Fatalf("decision exported = false, reason %q", decision.DiscardedReason)
	}
	if decision.Status != StatusExported {
		t.Fatalf("decision status = %q, want %q", decision.Status, StatusExported)
	}
	if decision.Attributes["custom_field"] != "[redacted]" {
		t.Fatalf("custom field not redacted: %#v", decision.Attributes)
	}
	if decision.Attributes["prompt"] != "[redacted]" {
		t.Fatalf("prompt not redacted: %#v", decision.Attributes)
	}
	if decision.Attributes["token_count"] != "12" {
		t.Fatalf("non-sensitive attribute redacted: %#v", decision.Attributes)
	}
	if len(decision.RedactedAttributes) != 2 {
		t.Fatalf("redacted attributes = %#v, want 2 entries", decision.RedactedAttributes)
	}
}

func TestApplyGovernanceDiscardsBySamplingPolicy(t *testing.T) {
	decision := ApplyGovernance(config.TelemetryConfig{
		Enabled:                true,
		SamplingRate:           1,
		HighRiskSamplingRate:   0,
		RetentionDays:          14,
		AllowedReaders:         []string{"maintainers"},
		ExportRequiresApproval: true,
	}, Event{
		Kind: "metric",
		Name: "risk.run",
		Risk: RiskHigh,
		Attributes: map[string]string{
			"secret": "must-not-export",
		},
	}, true)

	if decision.Exported {
		t.Fatalf("high-risk event exported despite zero sampling: %#v", decision)
	}
	if decision.Status != StatusDiscarded {
		t.Fatalf("decision status = %q, want %q", decision.Status, StatusDiscarded)
	}
	if decision.DiscardedReason != "sampling policy discarded event" {
		t.Fatalf("discard reason = %q", decision.DiscardedReason)
	}
	if len(decision.Attributes) != 0 {
		t.Fatalf("discarded event should not retain export attributes: %#v", decision.Attributes)
	}
}

func TestApplyGovernanceEnforcesFractionalSamplingPolicy(t *testing.T) {
	event := Event{
		Kind: "span",
		Name: "fractional.sample",
		Attributes: map[string]string{
			"phase": "test",
		},
	}
	score := samplingScore(event)
	rate := score / 2
	if rate == 0 {
		rate = 0.000001
	}

	decision := ApplyGovernance(config.TelemetryConfig{
		Enabled:                true,
		SamplingRate:           rate,
		HighRiskSamplingRate:   rate,
		RetentionDays:          14,
		AllowedReaders:         []string{"maintainers"},
		ExportRequiresApproval: false,
	}, event, false)

	if decision.Exported {
		t.Fatalf("event exported despite sample score %f >= rate %f: %#v", score, rate, decision)
	}
	if decision.DiscardedReason != "sampling policy discarded event" {
		t.Fatalf("discard reason = %q", decision.DiscardedReason)
	}
}

func TestApplyGovernanceRequiresExplicitExportApproval(t *testing.T) {
	decision := ApplyGovernance(config.TelemetryConfig{
		Enabled:                true,
		SamplingRate:           1,
		HighRiskSamplingRate:   1,
		RetentionDays:          14,
		AllowedReaders:         []string{"maintainers"},
		ExportRequiresApproval: true,
	}, Event{
		Kind: "span",
		Name: "approval.required",
		Attributes: map[string]string{
			"prompt": "must be redacted before pending record",
		},
	}, false)

	if decision.Exported {
		t.Fatalf("event exported without approval: %#v", decision)
	}
	if decision.Status != StatusPending {
		t.Fatalf("decision status = %q, want %q", decision.Status, StatusPending)
	}
	if decision.PendingReason != "export approval required" {
		t.Fatalf("pending reason = %q", decision.PendingReason)
	}
	if decision.Attributes["prompt"] != "[redacted]" {
		t.Fatalf("pending decision did not redact attributes: %#v", decision.Attributes)
	}
}
