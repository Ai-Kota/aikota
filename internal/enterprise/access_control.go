// Package enterprise provides enterprise-grade access control for aikota.
package enterprise

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Role represents a user role with specific permissions.
type Role string

const (
	RoleAdmin     Role = "admin"
	RoleDeveloper Role = "developer"
	RoleViewer    Role = "viewer"
	RoleAuditor   Role = "auditor"
)

// Permission defines what actions a role can perform.
type Permission struct {
	CanRead        bool
	CanWrite       bool
	CanExecute     bool
	CanDelete      bool
	CanAudit       bool
	CanManageUsers bool
	CanConfigure   bool
}

// DefaultPermissions maps roles to their default permission sets.
var DefaultPermissions = map[Role]Permission{
	RoleAdmin: {
		CanRead: true, CanWrite: true, CanExecute: true,
		CanDelete: true, CanAudit: true, CanManageUsers: true, CanConfigure: true,
	},
	RoleDeveloper: {
		CanRead: true, CanWrite: true, CanExecute: true,
		CanDelete: false, CanAudit: false, CanManageUsers: false, CanConfigure: false,
	},
	RoleViewer: {
		CanRead: true, CanWrite: false, CanExecute: false,
		CanDelete: false, CanAudit: false, CanManageUsers: false, CanConfigure: false,
	},
	RoleAuditor: {
		CanRead: true, CanWrite: false, CanExecute: false,
		CanDelete: false, CanAudit: true, CanManageUsers: false, CanConfigure: false,
	},
}

// User represents a team member in the system.
type User struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Role        Role       `json:"role"`
	Permissions Permission `json:"permissions"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLogin   time.Time  `json:"last_login,omitempty"`
	IsActive    bool       `json:"is_active"`
}

// AccessController manages user roles and permissions.
type AccessController struct {
	mu    sync.Mutex
	users map[string]*User
	roles map[Role]Permission
	log   *AuditLogger
}

// NewAccessController creates a new access controller with the given audit logger.
func NewAccessController(log *AuditLogger) *AccessController {
	return &AccessController{
		users: make(map[string]*User),
		roles: DefaultPermissions,
		log:   log,
	}
}

// CreateUser adds a new user with the specified role.
func (ac *AccessController) CreateUser(id, username, email string, role Role) (*User, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if _, exists := ac.users[id]; exists {
		return nil, fmt.Errorf("user %s already exists", id)
	}

	perms, ok := ac.roles[role]
	if !ok {
		return nil, fmt.Errorf("unknown role: %s", role)
	}

	user := &User{
		ID:          id,
		Username:    username,
		Email:       email,
		Role:        role,
		Permissions: perms,
		CreatedAt:   time.Now().UTC(),
		IsActive:    true,
	}

	ac.users[id] = user

	if ac.log != nil {
		_ = ac.log.LogEntry(id, "", EventTypeUserLogin, LevelInfo, map[string]string{
			"action": "create_user",
			"role":   string(role),
		})
	}

	return user, nil
}

// CheckPermission verifies if a user can perform an action.
func (ac *AccessController) CheckPermission(userID, action string) (bool, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	user, ok := ac.users[userID]
	if !ok {
		return false, fmt.Errorf("user %s not found", userID)
	}

	if !user.IsActive {
		return false, fmt.Errorf("user %s is inactive", userID)
	}

	switch action {
	case "read":
		return user.Permissions.CanRead, nil
	case "write":
		return user.Permissions.CanWrite, nil
	case "execute":
		return user.Permissions.CanExecute, nil
	case "delete":
		return user.Permissions.CanDelete, nil
	case "audit":
		return user.Permissions.CanAudit, nil
	case "manage_users":
		return user.Permissions.CanManageUsers, nil
	case "configure":
		return user.Permissions.CanConfigure, nil
	default:
		return false, fmt.Errorf("unknown action: %s", action)
	}
}

// UpdateUserRole changes a user's role and updates permissions accordingly.
func (ac *AccessController) UpdateUserRole(userID string, newRole Role) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	user, ok := ac.users[userID]
	if !ok {
		return fmt.Errorf("user %s not found", userID)
	}

	perms, ok := ac.roles[newRole]
	if !ok {
		return fmt.Errorf("unknown role: %s", newRole)
	}

	oldRole := user.Role
	user.Role = newRole
	user.Permissions = perms

	if ac.log != nil {
		_ = ac.log.LogEntry(userID, "", EventTypeConfigChange, LevelInfo, map[string]string{
			"action":   "role_change",
			"old_role": string(oldRole),
			"new_role": string(newRole),
		})
	}

	return nil
}

// DeactivateUser disables a user account.
func (ac *AccessController) DeactivateUser(userID string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	user, ok := ac.users[userID]
	if !ok {
		return fmt.Errorf("user %s not found", userID)
	}

	user.IsActive = false

	if ac.log != nil {
		_ = ac.log.LogEntry(userID, "", EventTypeSecurityAlert, LevelWarning, map[string]string{
			"action": "user_deactivated",
		})
	}

	return nil
}

// SaveUsers persists user data to disk.
func (ac *AccessController) SaveUsers(dir string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	data, err := json.Marshal(ac.users)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "users.json"), data, 0o644)
}

// LoadUsers loads user data from disk.
func (ac *AccessController) LoadUsers(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "users.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &ac.users)
}

// GetUser returns a user by ID.
func (ac *AccessController) GetUser(userID string) (*User, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	user, ok := ac.users[userID]
	if !ok {
		return nil, fmt.Errorf("user %s not found", userID)
	}
	return user, nil
}
