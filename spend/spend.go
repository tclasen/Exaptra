package spend

import (
	"sort"
	"time"
)

const AlertStatusBreached = "breached"

// Usage records token usage for one run without storing prompt or response text.
type Usage struct {
	RunID            string    `json:"run_id"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	ObservedAt       time.Time `json:"observed_at"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
}

// Budget defines an operator threshold for a provider/model window.
type Budget struct {
	Name       string  `json:"name"`
	Provider   string  `json:"provider,omitempty"`
	Model      string  `json:"model,omitempty"`
	MaxTokens  int     `json:"max_tokens,omitempty"`
	MaxCostUSD float64 `json:"max_cost_usd,omitempty"`
}

// Alert reports a budget threshold breach.
type Alert struct {
	Status     string  `json:"status"`
	Budget     string  `json:"budget"`
	Metric     string  `json:"metric"`
	Observed   float64 `json:"observed"`
	Threshold  float64 `json:"threshold"`
	Provider   string  `json:"provider,omitempty"`
	Model      string  `json:"model,omitempty"`
	WindowFrom string  `json:"window_from"`
	WindowTo   string  `json:"window_to"`
}

// Window summarizes spend for one provider/model pair over one time bucket.
type Window struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	WindowFrom       string  `json:"window_from"`
	WindowTo         string  `json:"window_to"`
	Runs             int     `json:"runs"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	Alerts           []Alert `json:"alerts,omitempty"`
}

// Report is the operator-facing token spend trend.
type Report struct {
	WindowSize string   `json:"window_size"`
	Windows    []Window `json:"windows"`
}

// EstimateCostUSD estimates model spend from token counts and per-thousand rates.
func EstimateCostUSD(inputTokens, outputTokens int, inputPerThousand, outputPerThousand float64) float64 {
	return (float64(inputTokens)/1000)*inputPerThousand + (float64(outputTokens)/1000)*outputPerThousand
}

// Summarize groups usage by provider, model, and time window, then evaluates budgets.
func Summarize(records []Usage, budgets []Budget, windowSize time.Duration) Report {
	if windowSize <= 0 {
		windowSize = time.Hour
	}
	grouped := make(map[string]*Window)
	for _, record := range records {
		if record.ObservedAt.IsZero() {
			continue
		}
		start := record.ObservedAt.UTC().Truncate(windowSize)
		end := start.Add(windowSize)
		key := record.Provider + "\x00" + record.Model + "\x00" + start.Format(time.RFC3339)
		window, ok := grouped[key]
		if !ok {
			window = &Window{
				Provider:   record.Provider,
				Model:      record.Model,
				WindowFrom: start.Format(time.RFC3339),
				WindowTo:   end.Format(time.RFC3339),
			}
			grouped[key] = window
		}
		window.Runs++
		window.InputTokens += record.InputTokens
		window.OutputTokens += record.OutputTokens
		window.TotalTokens += record.InputTokens + record.OutputTokens
		window.EstimatedCostUSD += record.EstimatedCostUSD
	}

	windows := make([]Window, 0, len(grouped))
	for _, window := range grouped {
		window.Alerts = evaluateBudgets(*window, budgets)
		windows = append(windows, *window)
	}
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].WindowFrom != windows[j].WindowFrom {
			return windows[i].WindowFrom < windows[j].WindowFrom
		}
		if windows[i].Provider != windows[j].Provider {
			return windows[i].Provider < windows[j].Provider
		}
		return windows[i].Model < windows[j].Model
	})
	return Report{
		WindowSize: windowSize.String(),
		Windows:    windows,
	}
}

func evaluateBudgets(window Window, budgets []Budget) []Alert {
	var alerts []Alert
	for _, budget := range budgets {
		if !budgetMatches(window, budget) {
			continue
		}
		if budget.MaxTokens > 0 && window.TotalTokens > budget.MaxTokens {
			alerts = append(alerts, alert(window, budget, "tokens", float64(window.TotalTokens), float64(budget.MaxTokens)))
		}
		if budget.MaxCostUSD > 0 && window.EstimatedCostUSD > budget.MaxCostUSD {
			alerts = append(alerts, alert(window, budget, "estimated_cost_usd", window.EstimatedCostUSD, budget.MaxCostUSD))
		}
	}
	return alerts
}

func budgetMatches(window Window, budget Budget) bool {
	if budget.Provider != "" && budget.Provider != window.Provider {
		return false
	}
	if budget.Model != "" && budget.Model != window.Model {
		return false
	}
	return true
}

func alert(window Window, budget Budget, metric string, observed, threshold float64) Alert {
	return Alert{
		Status:     AlertStatusBreached,
		Budget:     budget.Name,
		Metric:     metric,
		Observed:   observed,
		Threshold:  threshold,
		Provider:   window.Provider,
		Model:      window.Model,
		WindowFrom: window.WindowFrom,
		WindowTo:   window.WindowTo,
	}
}
