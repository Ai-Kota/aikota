// This file implements structured telemetry collection for a loop run
// (observability -- structured telemetry collection). A lightweight accumulator observes the AgentEvents the loop
// already emits and folds them into a compact summary — per-tool wall-clock
// durations, turn count, truncation count, compaction count, and the latest
// context-utilization ratio — that is surfaced once at run end as a
// TelemetryEvent (just before agent_end).
//
// The design is deliberately additive: telemetry rides the existing AgentEvent
// family and the existing stream-json output path, so no new dependency
// (Prometheus/OTLP) is introduced and existing stream-json consumers that do
// not know the "telemetry" event type keep working unchanged. Collection is
// passive — observing an event never changes loop behavior.
package runtime

import (
	"strings"
	"sync"
	"time"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

// telemetry is the per-run accumulator. Most fields are folded from events on
// the single runLoop goroutine, but tool_execution_* events fire from the
// parallel tool-batch goroutines (ExecuteToolCalls), so all mutation goes
// through mu.
type telemetry struct {
	mu              sync.Mutex
	turns           int
	truncationCount int
	compactionCount int

	// toolStarts maps an in-flight tool call id to the wall-clock time its
	// execution began, so the matching end event can compute a duration. Keying
	// by call id (not tool name) keeps parallel tool batches correct.
	toolStarts map[string]time.Time
	// toolTimings aggregates finished tool durations by tool name.
	toolTimings map[string]agentcore.ToolTiming

	// contextTokens / contextWindow capture the most recent context accounting so
	// the summary can report the latest utilization ratio. contextWindow == 0
	// means the window is unknown (utilization is then reported as 0).
	contextTokens int
	contextWindow int

	// inputTokens / outputTokens accumulate token usage across all turns.
	inputTokens  int
	outputTokens int

	// lastErrorType categorizes the most recent assistant failure observed this
	// run (StopReasonError/Aborted). Values: "api", "auth", "timeout", "network",
	// "internal", or "" when none. Set from TurnEndEvent.
	lastErrorType string
	// lastErrorMsg carries the provider/tool diagnostic text for the most recent
	// error. Set from TurnEndEvent.ErrorMessage.
	lastErrorMsg string
	// toolErrorCount is how many tool executions terminated with IsError=true.
	toolErrorCount int

	// now is the clock, injectable for deterministic tests. Defaults to
	// time.Now.
	now func() time.Time
}

// newTelemetry constructs an empty accumulator using the wall clock.
func newTelemetry() *telemetry {
	return &telemetry{
		toolStarts:  make(map[string]time.Time),
		toolTimings: make(map[string]agentcore.ToolTiming),
		now:         time.Now,
	}
}

// observe folds a single emitted event into the accumulator. It is a no-op for
// event types that carry no telemetry signal, and it never mutates the event.
func (t *telemetry) observe(ev agentcore.AgentEvent) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch e := ev.(type) {
	case agentcore.TurnStartEvent:
		t.turns++
	case agentcore.ToolExecutionStartEvent:
		t.toolStarts[e.ToolCallID] = t.now()
	case agentcore.ToolExecutionEndEvent:
		start, ok := t.toolStarts[e.ToolCallID]
		if !ok {
			return
		}
		delete(t.toolStarts, e.ToolCallID)
		elapsed := t.now().Sub(start).Milliseconds()
		if elapsed < 0 {
			elapsed = 0
		}
		agg := t.toolTimings[e.ToolName]
		agg.Count++
		agg.TotalMs += elapsed
		t.toolTimings[e.ToolName] = agg
		if e.IsError {
			t.toolErrorCount++
		}
	case agentcore.TurnEndEvent:
		if e.Message.StopReason == agentcore.StopReasonLength {
			t.truncationCount++
		}
		// Accumulate token usage from assistant message
		if e.Message.Usage != nil {
			t.inputTokens += e.Message.Usage.InputTokens
			t.outputTokens += e.Message.Usage.OutputTokens
		}
		// Track last provider error for diagnostics.
		if e.Message.StopReason == agentcore.StopReasonError && e.Message.ErrorMessage != "" {
			t.lastErrorType = classifyProviderError(e.Message.ErrorMessage)
			t.lastErrorMsg = e.Message.ErrorMessage
		}
	case agentcore.CompactionEvent:
		// Count only successful compactions; a failed one (ErrorMessage set) left
		// the context unchanged.
		if e.ErrorMessage == "" {
			t.compactionCount++
		}
	}
}

// recordContext captures the latest context-token usage and window so the
// summary can report the current utilization ratio. A non-positive window is
// treated as unknown.
func (t *telemetry) recordContext(tokens, window int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.contextTokens = tokens
	if window > 0 {
		t.contextWindow = window
	}
}

// summary materializes the accumulated metrics into a TelemetryEvent. The
// per-tool map is copied so the emitted event does not alias the accumulator's
// live state.
func (t *telemetry) summary() agentcore.TelemetryEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	timings := make(map[string]agentcore.ToolTiming, len(t.toolTimings))
	for name, v := range t.toolTimings {
		timings[name] = v
	}
	var utilization float64
	if t.contextWindow > 0 {
		utilization = float64(t.contextTokens) / float64(t.contextWindow)
	}
	return agentcore.TelemetryEvent{
		Turns:              t.turns,
		ToolDurationsMs:    timings,
		TruncationCount:    t.truncationCount,
		CompactionCount:    t.compactionCount,
		ContextUtilization: utilization,
		ContextTokens:      t.contextTokens,
		ContextWindow:      t.contextWindow,
		InputTokens:        t.inputTokens,
		OutputTokens:       t.outputTokens,
		LastErrorType:      t.lastErrorType,
		LastErrorMsg:       t.lastErrorMsg,
		ToolErrorCount:     t.toolErrorCount,
	}
}

// classifyProviderError categorizes an error message string into one of the
// recognized failure modes. It looks for keyword substrings in a best-effort
// heuristic — the text itself may be opaque to the caller, so this is a
// diagnostic aid, not a precision classifier. Empty string on unrecognized
// patterns.
func classifyProviderError(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "401") || strings.Contains(lower, "403") ||
		strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "invalid.*key") || strings.Contains(lower, "expired") ||
		strings.Contains(lower, "token") && strings.Contains(lower, "expir"):
		return "auth"
	case strings.Contains(lower, "429") || strings.Contains(lower, "rate.?limit") ||
		strings.Contains(lower, "too many request") || strings.Contains(lower, "over quota"):
		return "rate_limit"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") ||
		strings.Contains(lower, "timed out") || strings.Contains(lower, "i/o timeout"):
		return "timeout"
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "network") || strings.Contains(lower, "dns") ||
		strings.Contains(lower, "tls.") || strings.Contains(lower, "ssl.") ||
		strings.Contains(lower, "could not resolve"):
		return "network"
	case strings.Contains(lower, "400") || strings.Contains(lower, "422") ||
		strings.Contains(lower, "invalid request") || strings.Contains(lower, "schema") ||
		strings.Contains(lower, "malformed") || strings.Contains(lower, "bad request"):
		return "api"
	case strings.Contains(lower, "500") || strings.Contains(lower, "502") ||
		strings.Contains(lower, "503") || strings.Contains(lower, "504") ||
		strings.Contains(lower, "internal server") || strings.Contains(lower, "service unavailable"):
		return "provider"
	default:
		return "unknown"
	}
}
