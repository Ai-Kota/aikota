package cli

import (
	"testing"
	"github.com/Ai-Kota/aikota/internal/agentcore"
)

func TestTelemetryHolder_DiagnosticErrors(t *testing.T) {
	h := NewTelemetryHolder()

	// Initially no errors
	if h.DiagnosticErrors() != nil {
		t.Error("expected nil when no telemetry")
	}

	// Set up a telemetry event
	h.Fold(agentcore.TelemetryEvent{
		Turns:    2,
		ToolErrorCount: 1,
	})

	// DiagnosticErrors should return empty when no errors tracked
	errs := h.DiagnosticErrors()
	if errs != nil && len(errs) > 0 {
		t.Error("expected empty errors when none recorded")
	}
}
