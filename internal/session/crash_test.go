package session

import (
	"testing"
	"time"
	"github.com/Ai-Kota/aikota/internal/agentcore"
)

func TestStore_IsCrashed(t *testing.T) {
	tmp := t.TempDir()
	s := mustNewStore(t, tmp)

	// Non-existent session returns an error (not false)
	_, err := s.IsCrashed("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent session")
	}

	// Create a clean session
	header := SessionHeader{
		ID:        "test-session",
		Version:   SchemaVersion,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.Save(header, agentcore.MessageList{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Should not be marked crashed initially
	crashed, err := s.IsCrashed("test-session")
	if err != nil {
		t.Fatalf("IsCrashed: %v", err)
	}
	if crashed {
		t.Error("expected IsCrashed=false after fresh save")
	}

	// Mark as crashed
	header.Crashed = true
	if err := s.Save(header, agentcore.MessageList{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Should now report crashed
	crashed, err = s.IsCrashed("test-session")
	if err != nil {
		t.Fatalf("IsCrashed: %v", err)
	}
	if !crashed {
		t.Error("expected IsCrashed=true after marking crashed")
	}
}

func TestStore_ClearCrashed(t *testing.T) {
	tmp := t.TempDir()
	s := mustNewStore(t, tmp)

	// Create a session and mark it crashed
	header := SessionHeader{
		ID:        "crash-test",
		Version:   SchemaVersion,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Crashed:   true,
	}
	if err := s.Save(header, agentcore.MessageList{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify it's crashed
	crashed, err := s.IsCrashed("crash-test")
	if err != nil || !crashed {
		t.Fatal("expected session to be crashed")
	}

	// Clear the crashed flag
	if err := s.ClearCrashed("crash-test"); err != nil {
		t.Fatalf("ClearCrashed: %v", err)
	}

	// Verify it's no longer crashed
	crashed, err = s.IsCrashed("crash-test")
	if err != nil {
		t.Fatalf("IsCrashed: %v", err)
	}
	if crashed {
		t.Error("expected IsCrashed=false after ClearCrashed")
	}
}

func mustNewStore(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}
