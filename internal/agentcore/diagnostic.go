package agentcore

import (
	"sync"
	"time"
)

// DiagnosticEntry records a single error event for diagnostic purposes.
type DiagnosticEntry struct {
	Timestamp int64  // Unix timestamp in milliseconds
	ToolName  string // Name of the tool that failed
	ErrorType string // "tool", "api", "auth", "timeout", etc.
	Message   string // Human-readable error message
}

// DiagnosticState holds the error history for a run.
type DiagnosticState struct {
	mu     sync.Mutex
	errors []DiagnosticEntry
}

// NewDiagnosticState creates a new empty DiagnosticState.
func NewDiagnosticState() *DiagnosticState {
	return &DiagnosticState{}
}

// AddError appends an error to the diagnostic state, keeping only the last 5.
func (d *DiagnosticState) AddError(toolName, errorType, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entry := DiagnosticEntry{
		Timestamp: time.Now().UnixMilli(),
		ToolName:  toolName,
		ErrorType: errorType,
		Message:   message,
	}
	d.errors = append(d.errors, entry)
	if len(d.errors) > 5 {
		d.errors = d.errors[1:]
	}
}

// Errors returns a copy of the recent errors.
func (d *DiagnosticState) Errors() []DiagnosticEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]DiagnosticEntry, len(d.errors))
	copy(result, d.errors)
	return result
}

// HasErrors returns true if any errors have been recorded.
func (d *DiagnosticState) HasErrors() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.errors) > 0
}

// ErrorCount returns the number of recorded errors.
func (d *DiagnosticState) ErrorCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.errors)
}
