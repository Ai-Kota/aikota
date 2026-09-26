package feedback

import (
	"testing"
)

func TestRecordAndRetrieveFeedback(t *testing.T) {
	tmpDir := t.TempDir()
	collector, err := NewFeedbackCollector(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fb, err := collector.RecordFeedback("sess1", "msg1", FeedbackLike, 5, "Great response!")
	if err != nil {
		t.Fatalf("failed to record feedback: %v", err)
	}
	if fb.ID == "" {
		t.Error("expected non-empty ID")
	}
	if fb.Rating != 5 {
		t.Errorf("expected rating 5, got %d", fb.Rating)
	}
}

func TestRecordPreference(t *testing.T) {
	tmpDir := t.TempDir()
	collector, err := NewFeedbackCollector(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, err := collector.RecordPreference("sess1", "original code", "improved code", "improved", "context")
	if err != nil {
		t.Fatalf("failed to record preference: %v", err)
	}
	if rec.Preference != "improved" {
		t.Errorf("expected preference 'improved', got %s", rec.Preference)
	}
}

func TestInvalidRating(t *testing.T) {
	tmpDir := t.TempDir()
	collector, err := NewFeedbackCollector(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = collector.RecordFeedback("sess1", "msg1", FeedbackDislike, 0, "")
	if err == nil {
		t.Error("expected error for invalid rating")
	}

	_, err = collector.RecordFeedback("sess1", "msg1", FeedbackDislike, 6, "")
	if err == nil {
		t.Error("expected error for invalid rating")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	collector, err := NewFeedbackCollector(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = collector.RecordFeedback("sess1", "msg1", FeedbackLike, 5, "Good")
	_, _ = collector.RecordPreference("sess1", "old", "new", "improved", "ctx")

	collector2, err := NewFeedbackCollector(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = collector2.Load()
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	feedbacks := collector2.GetRecentFeedback(10)
	if len(feedbacks) != 1 {
		t.Errorf("expected 1 feedback, got %d", len(feedbacks))
	}

	prefs := collector2.GetPreferences()
	if len(prefs) != 1 {
		t.Errorf("expected 1 preference, got %d", len(prefs))
	}
}

func TestExportForTraining(t *testing.T) {
	tmpDir := t.TempDir()
	collector, err := NewFeedbackCollector(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = collector.RecordFeedback("sess1", "msg1", FeedbackLike, 5, "")
	_, _ = collector.RecordPreference("sess1", "a", "b", "improved", "")

	export, err := collector.ExportForTraining()
	if err != nil {
		t.Fatalf("failed to export: %v", err)
	}

	if export["total"] != 2 {
		t.Errorf("expected total 2, got %v", export["total"])
	}
}
