package cli

import (
	"testing"
	"github.com/Ai-Kota/aikota/internal/agentcore"
)

func TestTelemetryHolder_FoldCost(t *testing.T) {
	h := NewTelemetryHolder()

	// First run with cost
	ev1 := agentcore.TelemetryEvent{
		Turns:              2,
		InputTokens:        1000,
		OutputTokens:       500,
		EstimatedCostUSD:   5.5,
		Provider:           "anthropic",
	}
	h.Fold(ev1)

	if h.CumulativeEstimatedCostUSD() != 5.5 {
		t.Errorf("CumulativeEstimatedCostUSD = %v, want 5.5", h.CumulativeEstimatedCostUSD())
	}
	if h.LastEstimatedCostUSD() != 5.5 {
		t.Errorf("LastEstimatedCostUSD = %v, want 5.5", h.LastEstimatedCostUSD())
	}

	// Second run with different cost
	ev2 := agentcore.TelemetryEvent{
		Turns:              3,
		InputTokens:        2000,
		OutputTokens:       1000,
		EstimatedCostUSD:   10.0,
		Provider:           "openai",
	}
	h.Fold(ev2)

	if h.CumulativeEstimatedCostUSD() != 15.5 {
		t.Errorf("CumulativeEstimatedCostUSD = %v, want 15.5", h.CumulativeEstimatedCostUSD())
	}
	if h.LastEstimatedCostUSD() != 10.0 {
		t.Errorf("LastEstimatedCostUSD = %v, want 10.0", h.LastEstimatedCostUSD())
	}
}

func TestTelemetryHolder_LastEstimateWhenNil(t *testing.T) {
	h := NewTelemetryHolder()
	if h.LastEstimatedCostUSD() != 0 {
		t.Errorf("LastEstimatedCostUSD = %v, want 0 when no telemetry", h.LastEstimatedCostUSD())
	}
}

func TestTelemetryHolder_ResetCost(t *testing.T) {
	h := NewTelemetryHolder()
	h.Fold(agentcore.TelemetryEvent{EstimatedCostUSD: 5.0})
	h.Reset()
	if h.CumulativeEstimatedCostUSD() != 0 {
		t.Errorf("CumulativeEstimatedCostUSD = %v, want 0 after reset", h.CumulativeEstimatedCostUSD())
	}
	if h.LastEstimatedCostUSD() != 0 {
		t.Errorf("LastEstimatedCostUSD = %v, want 0 after reset", h.LastEstimatedCostUSD())
	}
}
