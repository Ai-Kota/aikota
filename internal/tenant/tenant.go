// Package tenant provides multi-tenancy support for aikota, enabling
// isolated workspaces, audit logs, and resource quotas per organization.
package tenant

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Tenant represents an isolated organizational unit with its own resources.
type Tenant struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Settings    TenantSettings    `json:"settings"`
	Quotas      QuotaConfig       `json:"quotas"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// TenantSettings holds configuration specific to a tenant.
type TenantSettings struct {
	// DefaultModel is the preferred LLM model for this tenant
	DefaultModel string `json:"default_model"`
	// MaxTokensPerSession limits context window size
	MaxTokensPerSession int `json:"max_tokens_per_session"`
	// AuditLogEnabled controls whether operations are logged
	AuditLogEnabled bool `json:"audit_log_enabled"`
	// SandboxEnabled controls whether commands run in Docker
	SandboxEnabled bool `json:"sandbox_enabled"`
	// AllowedTools is a denylist of tools to disable
	DeniedTools []string `json:"denied_tools,omitempty"`
}

// QuotaConfig defines resource limits for a tenant.
type QuotaConfig struct {
	// MaxSessions limits concurrent sessions
	MaxSessions int `json:"max_sessions"`
	// MaxDailyTokens limits daily token consumption
	MaxDailyTokens int64 `json:"max_daily_tokens"`
	// MaxStorageMB limits workspace storage
	MaxStorageMB int64 `json:"max_storage_mb"`
	// MaxPlugins limits installed plugins
	MaxPlugins int `json:"max_plugins"`
}

// TenantManager handles tenant lifecycle and isolation.
type TenantManager struct {
	mu      sync.Mutex
	tenants map[string]*Tenant
	baseDir string
}

// NewTenantManager creates a new tenant manager.
func NewTenantManager(baseDir string) (*TenantManager, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("tenant: create base dir: %w", err)
	}
	return &TenantManager{
		tenants: make(map[string]*Tenant),
		baseDir: baseDir,
	}, nil
}

// CreateTenant creates a new tenant with default settings.
func (tm *TenantManager) CreateTenant(id, name, description string) (*Tenant, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tenants[id]; exists {
		return nil, fmt.Errorf("tenant: id %q already exists", id)
	}

	now := time.Now().UTC()
	tenant := &Tenant{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Settings: TenantSettings{
			DefaultModel:        "claude-opus-4-8",
			MaxTokensPerSession: 128000,
			AuditLogEnabled:     true,
			SandboxEnabled:      false,
		},
		Quotas: QuotaConfig{
			MaxSessions:    10,
			MaxDailyTokens: 1_000_000,
			MaxStorageMB:   1024,
			MaxPlugins:     5,
		},
		Metadata: make(map[string]string),
	}

	tm.tenants[id] = tenant
	return tenant, tm.saveTenant(tenant)
}

// GetTenant returns a tenant by ID.
func (tm *TenantManager) GetTenant(id string) (*Tenant, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, ok := tm.tenants[id]
	if !ok {
		return nil, fmt.Errorf("tenant: %q not found", id)
	}
	return tenant, nil
}

// ListTenants returns all tenants.
func (tm *TenantManager) ListTenants() []*Tenant {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	result := make([]*Tenant, 0, len(tm.tenants))
	for _, t := range tm.tenants {
		result = append(result, t)
	}
	return result
}

// DeleteTenant removes a tenant and its data.
func (tm *TenantManager) DeleteTenant(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, ok := tm.tenants[id]; !ok {
		return fmt.Errorf("tenant: %q not found", id)
	}

	delete(tm.tenants, id)
	return os.RemoveAll(filepath.Join(tm.baseDir, id))
}

// UpdateSettings modifies tenant configuration.
func (tm *TenantManager) UpdateSettings(id string, settings TenantSettings) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tenant, ok := tm.tenants[id]
	if !ok {
		return fmt.Errorf("tenant: %q not found", id)
	}

	tenant.Settings = settings
	tenant.UpdatedAt = time.Now().UTC()
	return tm.saveTenant(tenant)
}

// WorkspaceDir returns the isolated workspace directory for a tenant.
func (tm *TenantManager) WorkspaceDir(id string) string {
	return filepath.Join(tm.baseDir, id, "workspace")
}

// AuditDir returns the audit log directory for a tenant.
func (tm *TenantManager) AuditDir(id string) string {
	return filepath.Join(tm.baseDir, id, "audit")
}

// saveTenant persists a tenant to disk.
func (tm *TenantManager) saveTenant(tenant *Tenant) error {
	data, err := json.MarshalIndent(tenant, "", "  ")
	if err != nil {
		return fmt.Errorf("tenant: marshal: %w", err)
	}

	dir := filepath.Join(tm.baseDir, tenant.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("tenant: create dir: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, "tenant.json"), data, 0o644)
}

// LoadAll loads all tenants from disk.
func (tm *TenantManager) LoadAll() error {
	entries, err := os.ReadDir(tm.baseDir)
	if err != nil {
		return fmt.Errorf("tenant: read dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		data, err := os.ReadFile(filepath.Join(tm.baseDir, id, "tenant.json"))
		if err != nil {
			continue // skip malformed entries
		}
		var tenant Tenant
		if err := json.Unmarshal(data, &tenant); err != nil {
			continue
		}
		tm.tenants[id] = &tenant
	}
	return nil
}
