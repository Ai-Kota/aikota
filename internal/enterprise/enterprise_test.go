package enterprise

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditLoggerCreation(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewAuditLogger(tmpDir, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestAuditLoggerLogEntry(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)

	err := logger.LogEntry("user1", "session1", EventTypeToolCall, LevelInfo, map[string]string{
		"tool": "bash",
	})
	if err != nil {
		t.Fatalf("unexpected error logging entry: %v", err)
	}

	entries := logger.GetEntries("", 10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].UserID != "user1" {
		t.Errorf("expected user1, got %s", entries[0].UserID)
	}
}

func TestAuditLoggerMaxEntries(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 5)

	for i := 0; i < 10; i++ {
		_ = logger.LogEntry("user1", "", EventTypeUserLogin, LevelInfo, nil)
	}

	entries := logger.GetEntries("", 100)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries (max), got %d", len(entries))
	}
}

func TestAuditLoggerPersistToDisk(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)

	_ = logger.LogEntry("user1", "", EventTypeUserLogin, LevelInfo, nil)
	_ = logger.LogEntry("user1", "", EventTypeSecurityAlert, LevelWarning, nil)

	// Check that log file was created
	_, err := os.Stat(filepath.Join(tmpDir, time.Now().UTC().Format("2006-01-02")+".jsonl"))
	if err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

func TestAuditLoggerReport(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)

	_ = logger.LogEntry("user1", "", EventTypeToolCall, LevelInfo, nil)
	_ = logger.LogEntry("user2", "", EventTypeSecurityAlert, LevelCritical, nil)
	_ = logger.LogEntry("user1", "", EventTypeUserLogin, LevelInfo, nil)

	report := logger.GenerateReport(7)
	if report == "" {
		t.Error("expected non-empty report")
	}
}

func TestAccessControllerCreateUser(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)
	controller := NewAccessController(logger)

	user, err := controller.CreateUser("u1", "alice", "alice@example.com", RoleDeveloper)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Role != RoleDeveloper {
		t.Errorf("expected RoleDeveloper, got %s", user.Role)
	}
	if !user.IsActive {
		t.Error("expected user to be active")
	}
}

func TestAccessControllerCheckPermission(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)
	controller := NewAccessController(logger)

	_, _ = controller.CreateUser("u1", "alice", "alice@example.com", RoleDeveloper)

	allowed, err := controller.CheckPermission("u1", "read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected read permission for developer")
	}

	allowed, err = controller.CheckPermission("u1", "delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected no delete permission for developer")
	}
}

func TestAccessControllerUpdateRole(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)
	controller := NewAccessController(logger)

	_, _ = controller.CreateUser("u1", "alice", "alice@example.com", RoleViewer)

	err := controller.UpdateUserRole("u1", RoleDeveloper)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, _ := controller.GetUser("u1")
	if user.Role != RoleDeveloper {
		t.Errorf("expected RoleDeveloper, got %s", user.Role)
	}
}

func TestAccessControllerDeactivateUser(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)
	controller := NewAccessController(logger)

	_, _ = controller.CreateUser("u1", "alice", "alice@example.com", RoleDeveloper)

	err := controller.DeactivateUser("u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allowed, err := controller.CheckPermission("u1", "read")
	if err == nil && allowed {
		t.Error("expected inactive user to have no permissions")
	}
}

func TestTeamCollaboratorCreateTask(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)
	tc := NewTeamCollaborator(tmpDir, logger)

	task, err := tc.CreateTask("Test Task", "Description", "user1", []string{"user1", "user2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Status != StatusPending {
		t.Errorf("expected StatusPending, got %s", task.Status)
	}
}

func TestTeamCollaboratorUpdateTaskStatus(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)
	tc := NewTeamCollaborator(tmpDir, logger)

	task, _ := tc.CreateTask("Test Task", "Desc", "user1", []string{"user1"})

	err := tc.UpdateTaskStatus(task.ID, "user1", string(StatusInProgress))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.Status != StatusInProgress {
		t.Errorf("expected StatusInProgress, got %s", task.Status)
	}
}

func TestTeamCollaboratorShareSession(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)
	tc := NewTeamCollaborator(tmpDir, logger)

	expiresAt := time.Now().Add(24 * time.Hour)
	share, err := tc.ShareSession("sess1", "user1", []string{"user2"}, "read", expiresAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if share.SessionID != "sess1" {
		t.Errorf("expected sess1, got %s", share.SessionID)
	}
	if share.AccessLevel != "read" {
		t.Errorf("expected read, got %s", share.AccessLevel)
	}
}

func TestTeamCollaboratorSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := NewAuditLogger(tmpDir, 100)
	tc := NewTeamCollaborator(tmpDir, logger)

	_, _ = tc.CreateTask("Task 1", "Desc", "user1", []string{"user1"})
	_, _ = tc.ShareSession("sess1", "user1", []string{"user2"}, "read", time.Now().Add(24*time.Hour))

	err := tc.SaveToDisk(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	tc2 := NewTeamCollaborator(tmpDir, logger)
	err = tc2.LoadFromDisk(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}

	if len(tc2.tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tc2.tasks))
	}
}
