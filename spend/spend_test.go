package spend

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSummarizeAggregatesByProviderModelAndWindow(t *testing.T) {
	base := time.Date(2026, 7, 9, 10, 15, 0, 0, time.UTC)

	report := Summarize([]Usage{
		{
			RunID:            "run-1",
			Provider:         "openai",
			Model:            "gpt-4.1",
			ObservedAt:       base,
			InputTokens:      100,
			OutputTokens:     50,
			EstimatedCostUSD: EstimateCostUSD(100, 50, 0.01, 0.03),
		},
		{
			RunID:            "run-2",
			Provider:         "openai",
			Model:            "gpt-4.1",
			ObservedAt:       base.Add(10 * time.Minute),
			InputTokens:      300,
			OutputTokens:     150,
			EstimatedCostUSD: EstimateCostUSD(300, 150, 0.01, 0.03),
		},
		{
			RunID:            "run-3",
			Provider:         "local",
			Model:            "example-model",
			ObservedAt:       base.Add(time.Hour),
			InputTokens:      20,
			OutputTokens:     10,
			EstimatedCostUSD: 0,
		},
	}, nil, time.Hour)

	if report.WindowSize != "1h0m0s" {
		t.Fatalf("window size = %q", report.WindowSize)
	}
	if len(report.Windows) != 2 {
		t.Fatalf("window len = %d, want 2: %#v", len(report.Windows), report.Windows)
	}
	first := report.Windows[0]
	if first.Provider != "openai" || first.Model != "gpt-4.1" {
		t.Fatalf("first attribution = %s/%s", first.Provider, first.Model)
	}
	if first.Runs != 2 || first.InputTokens != 400 || first.OutputTokens != 200 || first.TotalTokens != 600 {
		t.Fatalf("first totals = %#v", first)
	}
}

func TestSummarizeSurfacesBudgetBreaches(t *testing.T) {
	report := Summarize([]Usage{{
		RunID:            "run-1",
		Provider:         "openai",
		Model:            "gpt-4.1",
		ObservedAt:       time.Date(2026, 7, 9, 10, 15, 0, 0, time.UTC),
		InputTokens:      800,
		OutputTokens:     300,
		EstimatedCostUSD: 0.25,
	}}, []Budget{{
		Name:       "daily-openai",
		Provider:   "openai",
		Model:      "gpt-4.1",
		MaxTokens:  1000,
		MaxCostUSD: 0.10,
	}}, time.Hour)

	if len(report.Windows) != 1 {
		t.Fatalf("window len = %d, want 1", len(report.Windows))
	}
	alerts := report.Windows[0].Alerts
	if len(alerts) != 2 {
		t.Fatalf("alert len = %d, want 2: %#v", len(alerts), alerts)
	}
	if alerts[0].Status != AlertStatusBreached || alerts[0].Budget != "daily-openai" {
		t.Fatalf("unexpected alert: %#v", alerts[0])
	}
}

func TestReportDoesNotExposePromptContent(t *testing.T) {
	report := Summarize([]Usage{{
		RunID:            "run-without-content",
		Provider:         "openai",
		Model:            "gpt-4.1",
		ObservedAt:       time.Date(2026, 7, 9, 10, 15, 0, 0, time.UTC),
		InputTokens:      42,
		OutputTokens:     7,
		EstimatedCostUSD: 0.01,
	}}, nil, time.Hour)

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if strings.Contains(string(encoded), "prompt") || strings.Contains(string(encoded), "message") {
		t.Fatalf("report exposes content-bearing fields: %s", encoded)
	}
}
