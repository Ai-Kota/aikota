// Package feedback implements RLHF-style feedback collection for improving
// aikota's behavior over time.
package feedback

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FeedbackType string

const (
	FeedbackLike      FeedbackType = "like"
	FeedbackDislike   FeedbackType = "dislike"
	FeedbackCorrect   FeedbackType = "correct"
	FeedbackImprovement FeedbackType = "improvement"
)

type ResponseFeedback struct {
	ID           string       `json:"id"`
	SessionID    string       `json:"session_id"`
	MessageID    string       `json:"message_id"`
	FeedbackType FeedbackType `json:"feedback_type"`
	Rating       int          `json:"rating"`
	Comment      string       `json:"comment,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type PreferenceRecord struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Original   string    `json:"original"`
	Improved   string    `json:"improved"`
	Preference string    `json:"preference"`
	Context    string    `json:"context"`
	CreatedAt  time.Time `json:"created_at"`
}

type FeedbackCollector struct {
	mu          sync.Mutex
	records     []ResponseFeedback
	preferences []PreferenceRecord
	baseDir     string
}

func NewFeedbackCollector(baseDir string) (*FeedbackCollector, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("feedback: create dir: %w", err)
	}
	return &FeedbackCollector{baseDir: baseDir}, nil
}

func (fc *FeedbackCollector) RecordFeedback(sessionID, messageID string, feedbackType FeedbackType, rating int, comment string) (*ResponseFeedback, error) {
	if rating < 1 || rating > 5 {
		return nil, fmt.Errorf("feedback: rating must be 1-5")
	}
	fb := &ResponseFeedback{
		ID:           fmt.Sprintf("fb-%d", time.Now().UnixNano()),
		SessionID:    sessionID,
		MessageID:    messageID,
		FeedbackType: feedbackType,
		Rating:       rating,
		Comment:      comment,
		CreatedAt:    time.Now().UTC(),
	}
	fc.mu.Lock()
	fc.records = append(fc.records, *fb)
	fc.mu.Unlock()
	if err := fc.save(); err != nil {
		return fb, err
	}
	return fb, nil
}

func (fc *FeedbackCollector) RecordPreference(sessionID, original, improved, preference, context string) (*PreferenceRecord, error) {
	if preference != "original" && preference != "improved" {
		return nil, fmt.Errorf("feedback: preference must be 'original' or 'improved'")
	}
	rec := &PreferenceRecord{
		ID:         fmt.Sprintf("pref-%d", time.Now().UnixNano()),
		SessionID:  sessionID,
		Original:   original,
		Improved:   improved,
		Preference: preference,
		Context:    context,
		CreatedAt:  time.Now().UTC(),
	}
	fc.mu.Lock()
	fc.preferences = append(fc.preferences, *rec)
	fc.mu.Unlock()
	if err := fc.save(); err != nil {
		return rec, err
	}
	return rec, nil
}

func (fc *FeedbackCollector) GetRecentFeedback(limit int) []ResponseFeedback {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	start := 0
	if len(fc.records) > limit {
		start = len(fc.records) - limit
	}
	result := make([]ResponseFeedback, len(fc.records)-start)
	copy(result, fc.records[start:])
	return result
}

func (fc *FeedbackCollector) GetPreferences() []PreferenceRecord {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	result := make([]PreferenceRecord, len(fc.preferences))
	copy(result, fc.preferences)
	return result
}

func (fc *FeedbackCollector) ExportForTraining() (map[string]interface{}, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return map[string]interface{}{
		"feedback":     fc.records,
		"preferences":  fc.preferences,
		"total":        len(fc.records) + len(fc.preferences),
		"exported_at":  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (fc *FeedbackCollector) save() error {
	data, err := json.MarshalIndent(map[string]interface{}{
		"feedback":    fc.records,
		"preferences": fc.preferences,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("feedback: marshal: %w", err)
	}
	return os.WriteFile(filepath.Join(fc.baseDir, "feedback.json"), data, 0o644)
}

func (fc *FeedbackCollector) Load() error {
	data, err := os.ReadFile(filepath.Join(fc.baseDir, "feedback.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("feedback: read: %w", err)
	}
	var content struct {
		Feedback    []ResponseFeedback    `json:"feedback"`
		Preferences []PreferenceRecord    `json:"preferences"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("feedback: unmarshal: %w", err)
	}
	fc.mu.Lock()
	fc.records = content.Feedback
	fc.preferences = content.Preferences
	fc.mu.Unlock()
	return nil
}
