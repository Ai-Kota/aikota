package interactive

import (
	"testing"
	"time"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/Ai-Kota/aikota/internal/session"
)

func TestNewRunSession_WarnsOnCrashedResume(t *testing.T) {
	tmp := t.TempDir()
	store, err := session.NewStore(tmp)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create a crashed session
	header := session.SessionHeader{
		ID:        "crashed-session",
		Version:   session.SchemaVersion,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Crashed:   true,
	}
	if err := store.Save(header, agentcore.MessageList{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify IsCrashed reports true
	crashed, err := store.IsCrashed("crashed-session")
	if err != nil {
		t.Fatalf("IsCrashed: %v", err)
	}
	if !crashed {
		t.Error("expected IsCrashed=true for crashed session")
	}

	// Resuming a crashed session should succeed (warning goes to stderr, not error)
	opts := Options{
		Model:    "test-model",
		ResumeID: "crashed-session",
		Tools:    []agentcore.AgentTool{},
		SysPrompt: "test",
	}
	_, _, err = newRunSessionWithStore(store, opts)
	if err != nil {
		t.Fatalf("newRunSessionWithStore: %v", err)
	}
}

func TestNewRunSession_NoWarningForCleanResume(t *testing.T) {
	tmp := t.TempDir()
	store, err := session.NewStore(tmp)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create a clean (non-crashed) session
	header := session.SessionHeader{
		ID:        "clean-session",
		Version:   session.SchemaVersion,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Crashed:   false,
	}
	if err := store.Save(header, agentcore.MessageList{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify IsCrashed reports false
	crashed, err := store.IsCrashed("clean-session")
	if err != nil {
		t.Fatalf("IsCrashed: %v", err)
	}
	if crashed {
		t.Error("expected IsCrashed=false for clean session")
	}

	// Resuming a clean session should succeed
	opts := Options{
		Model:     "test-model",
		ResumeID:  "clean-session",
		Tools:     []agentcore.AgentTool{},
		SysPrompt: "test",
	}
	_, _, err = newRunSessionWithStore(store, opts)
	if err != nil {
		t.Fatalf("newRunSessionWithStore: %v", err)
	}
}

func TestClearCrashedFlag(t *testing.T) {
	tmp := t.TempDir()
	store, err := session.NewStore(tmp)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Create a crashed session
	header := session.SessionHeader{
		ID:        "clear-test",
		Version:   session.SchemaVersion,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Crashed:   true,
	}
	if err := store.Save(header, agentcore.MessageList{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify it's crashed
	crashed, err := store.IsCrashed("clear-test")
	if err != nil || !crashed {
		t.Fatal("expected session to be crashed")
	}

	// Clear the crashed flag
	if err := store.ClearCrashed("clear-test"); err != nil {
		t.Fatalf("ClearCrashed: %v", err)
	}

	// Verify it's no longer crashed
	crashed, err = store.IsCrashed("clear-test")
	if err != nil {
		t.Fatalf("IsCrashed: %v", err)
	}
	if crashed {
		t.Error("expected IsCrashed=false after ClearCrashed")
	}
}
