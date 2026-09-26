// Package enterprise provides enterprise-grade features for aikota including
// audit logging, access control, and team collaboration capabilities.
package enterprise

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEventType defines the type of action being logged.
type AuditEventType string

const (
	EventTypeToolCall      AuditEventType = "tool_call"
	EventTypeSessionStart  AuditEventType = "session_start"
	EventTypeSessionEnd    AuditEventType = "session_end"
	EventTypePermissionGrant AuditEventType = "permission_grant"
	EventTypeSecurityAlert AuditEventType = "security_alert"
	EventTypeUserLogin     AuditEventType = "user_login"
	EventTypeConfigChange  AuditEventType = "config_change"
)

// AuditLevel represents the severity of an audit entry.
type AuditLevel string

const (
	LevelInfo     AuditLevel = "INFO"
	LevelWarning  AuditLevel = "WARNING"
	LevelCritical AuditLevel = "CRITICAL"
)

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	ID          string            `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	UserID      string            `json:"user_id"`
	EventType   AuditEventType    `json:"event_type"`
	Level       AuditLevel        `json:"level"`
	Details     map[string]string `json:"details,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
}

// AuditLogger manages centralized audit logging for enterprise operations.
type AuditLogger struct {
	mu        sync.Mutex
	logDir    string
	entries   []AuditEntry
	maxEntries int
}

// NewAuditLogger creates a new audit logger that writes to the specified directory.
func NewAuditLogger(logDir string, maxEntries int) (*AuditLogger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("audit logger: create directory %q: %w", logDir, err)
	}
	return &AuditLogger{
		logDir:     logDir,
		maxEntries: maxEntries,
	}, nil
}

// LogEntry records an audit event with the provided details.
func (al *AuditLogger) LogEntry(userID, sessionID string, eventType AuditEventType, level AuditLevel, details map[string]string) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	entry := AuditEntry{
		ID:        generateID(),
		Timestamp: time.Now().UTC(),
		UserID:    userID,
		EventType: eventType,
		Level:     level,
		SessionID: sessionID,
		Details:   details,
	}

	al.entries = append(al.entries, entry)

	// Enforce max entries by removing oldest
	if al.maxEntries > 0 && len(al.entries) > al.maxEntries {
		al.entries = al.entries[len(al.entries)-al.maxEntries:]
	}

	// Write to file
	return al.persistEntry(entry)
}

// persistEntry writes a single audit entry to the log file.
func (al *AuditLogger) persistEntry(entry AuditEntry) error {
	filename := entry.Timestamp.Format("2006-01-02") + ".jsonl"
	path := filepath.Join(al.logDir, filename)

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit logger: marshal entry: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("audit logger: open log file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(string(data) + "\n"); err != nil {
		return fmt.Errorf("audit logger: write entry: %w", err)
	}

	return nil
}

// GetEntries returns recent audit entries filtered by type.
func (al *AuditLogger) GetEntries(filterType AuditEventType, limit int) []AuditEntry {
	al.mu.Lock()
	defer al.mu.Unlock()

	var filtered []AuditEntry
	for i := len(al.entries) - 1; i >= 0 && len(filtered) < limit; i-- {
		if filterType == "" || al.entries[i].EventType == filterType {
			filtered = append(filtered, al.entries[i])
		}
	}
	return filtered
}

// GenerateReport creates a summary report of audit activity.
func (al *AuditLogger) GenerateReport(days int) string {
	al.mu.Lock()
	defer al.mu.Unlock()

	report := fmt.Sprintf("=== Enterprise Audit Report (Last %d Days) ===\n\n", days)

	// Count by event type
	typeCounts := make(map[AuditEventType]int)
	levelCounts := make(map[AuditLevel]int)
	userCounts := make(map[string]int)

	for _, entry := range al.entries {
		typeCounts[entry.EventType]++
		levelCounts[entry.Level]++
		userCounts[entry.UserID]++
	}

	report += "Events by Type:\n"
	for t, c := range typeCounts {
		report += fmt.Sprintf("  %-20s %d\n", t, c)
	}

	report += "\nEvents by Level:\n"
	for l, c := range levelCounts {
		report += fmt.Sprintf("  %-20s %d\n", l, c)
	}

	report += "\nActivity by User:\n"
	for u, c := range userCounts {
		report += fmt.Sprintf("  %-20s %d\n", u, c)
	}

	// Critical alerts
	criticalCount := levelCounts[LevelCritical]
	if criticalCount > 0 {
		report += fmt.Sprintf("\n⚠️  CRITICAL ALERTS: %d events requiring review\n", criticalCount)
	}

	return report
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
