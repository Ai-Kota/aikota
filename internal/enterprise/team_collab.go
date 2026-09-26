// Package enterprise provides team collaboration features for aikota.
package enterprise

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CollaborationStatus represents the status of a shared task.
type CollaborationStatus string

const (
	StatusPending   CollaborationStatus = "pending"
	StatusInProgress CollaborationStatus = "in_progress"
	StatusCompleted CollaborationStatus = "completed"
	StatusBlocked   CollaborationStatus = "blocked"
)

// SharedTask represents a collaborative task assigned to team members.
type SharedTask struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	AssignedTo  []string             `json:"assigned_to"`
	CreatedBy   string               `json:"created_by"`
	Status      CollaborationStatus  `json:"status"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Metadata    map[string]string    `json:"metadata,omitempty"`
}

// SessionShare represents a shared session between team members.
type SessionShare struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"session_id"`
	SharedBy      string    `json:"shared_by"`
	SharedWith    []string  `json:"shared_with"`
	AccessLevel   string    `json:"access_level"` // "read", "write", "admin"
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	IsEncrypted   bool      `json:"is_encrypted"`
}

// TeamCollaborator manages team-based collaboration features.
type TeamCollaborator struct {
	mu            sync.Mutex
	tasks         map[string]*SharedTask
	sessions      map[string]*SessionShare
	projectRoot   string
	auditLog      *AuditLogger
}

// NewTeamCollaborator creates a new team collaborator manager.
func NewTeamCollaborator(projectRoot string, log *AuditLogger) *TeamCollaborator {
	return &TeamCollaborator{
		tasks:       make(map[string]*SharedTask),
		sessions:    make(map[string]*SessionShare),
		projectRoot: projectRoot,
		auditLog:    log,
	}
}

// CreateTask creates a new collaborative task.
func (tc *TeamCollaborator) CreateTask(title, description, createdBy string, assignedTo []string) (*SharedTask, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	task := &SharedTask{
		ID:          taskID,
		Title:       title,
		Description: description,
		AssignedTo:  assignedTo,
		CreatedBy:   createdBy,
		Status:      StatusPending,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Metadata:    make(map[string]string),
	}

	tc.tasks[taskID] = task

	if tc.auditLog != nil {
		_ = tc.auditLog.LogEntry(createdBy, "", EventTypeConfigChange, LevelInfo, map[string]string{
			"action":     "create_task",
			"task_id":    taskID,
			"title":      title,
			"assigned_to": fmt.Sprintf("%v", assignedTo),
		})
	}

	return task, nil
}

// UpdateTaskStatus updates the status of a task.
func (tc *TeamCollaborator) UpdateTaskStatus(taskID, userID, status string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	task, ok := tc.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}

	task.Status = CollaborationStatus(status)
	task.UpdatedAt = time.Now().UTC()

	if tc.auditLog != nil {
		_ = tc.auditLog.LogEntry(userID, "", EventTypeConfigChange, LevelInfo, map[string]string{
			"action":  "update_task_status",
			"task_id": taskID,
			"status":  status,
		})
	}

	return nil
}

// ShareSession shares a session with team members.
func (tc *TeamCollaborator) ShareSession(sessionID, sharedBy string, sharedWith []string, accessLevel string, expiresAt time.Time) (*SessionShare, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	shareID := fmt.Sprintf("share-%d", time.Now().UnixNano())
	share := &SessionShare{
		ID:          shareID,
		SessionID:   sessionID,
		SharedBy:    sharedBy,
		SharedWith:  sharedWith,
		AccessLevel: accessLevel,
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   expiresAt,
		IsEncrypted: true,
	}

	tc.sessions[shareID] = share

	if tc.auditLog != nil {
		_ = tc.auditLog.LogEntry(sharedBy, sessionID, EventTypePermissionGrant, LevelInfo, map[string]string{
			"action":       "share_session",
			"session_id":   sessionID,
			"shared_with":  fmt.Sprintf("%v", sharedWith),
			"access_level": accessLevel,
		})
	}

	return share, nil
}

// GetTeamTasks returns all tasks for a user.
func (tc *TeamCollaborator) GetTeamTasks(userID string, filterStatus CollaborationStatus) []*SharedTask {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	var result []*SharedTask
	for _, task := range tc.tasks {
		for _, assigned := range task.AssignedTo {
			if assigned == userID {
				if filterStatus == "" || task.Status == filterStatus {
					result = append(result, task)
				}
				break
			}
		}
	}
	return result
}

// GetActiveSessions returns active session shares.
func (tc *TeamCollaborator) GetActiveSessions(userID string) []*SessionShare {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	var result []*SessionShare
	now := time.Now().UTC()

	for _, share := range tc.sessions {
		if now.After(share.ExpiresAt) {
			continue
		}
		for _, member := range share.SharedWith {
			if member == userID {
				result = append(result, share)
				break
			}
		}
	}
	return result
}

// SaveToDisk persists all collaboration data.
func (tc *TeamCollaborator) SaveToDisk(dir string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tasksData, _ := json.Marshal(tc.tasks)
	sessionsData, _ := json.Marshal(tc.sessions)

	if err := os.WriteFile(filepath.Join(dir, "tasks.json"), tasksData, 0o644); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "sessions.json"), sessionsData, 0o644)
}

// LoadFromDisk loads collaboration data from disk.
func (tc *TeamCollaborator) LoadFromDisk(dir string) error {
	tasksData, err := os.ReadFile(filepath.Join(dir, "tasks.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(tasksData) > 0 {
		json.Unmarshal(tasksData, &tc.tasks)
	}

	sessionsData, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(sessionsData) > 0 {
		json.Unmarshal(sessionsData, &tc.sessions)
	}

	return nil
}
