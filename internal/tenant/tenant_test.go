package tenant

import (
	"path/filepath"
	"testing"
)

func TestCreateAndGetTenant(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewTenantManager(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tenant, err := mgr.CreateTenant("org1", "Test Org", "A test organization")
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	if tenant.ID != "org1" {
		t.Errorf("expected id 'org1', got %s", tenant.ID)
	}

	got, err := mgr.GetTenant("org1")
	if err != nil {
		t.Fatalf("failed to get tenant: %v", err)
	}
	if got.Name != "Test Org" {
		t.Errorf("expected name 'Test Org', got %s", got.Name)
	}
}

func TestListTenants(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewTenantManager(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = mgr.CreateTenant("org1", "Org 1", "")
	_, _ = mgr.CreateTenant("org2", "Org 2", "")

	tenants := mgr.ListTenants()
	if len(tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(tenants))
	}
}

func TestWorkspaceDir(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewTenantManager(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = mgr.CreateTenant("org1", "Org 1", "")
	dir := mgr.WorkspaceDir("org1")
	if !filepath.HasPrefix(dir, tmpDir) {
		t.Errorf("workspace dir %s should be under %s", dir, tmpDir)
	}
}

func TestDeleteTenant(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewTenantManager(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = mgr.CreateTenant("org1", "Org 1", "")
	err = mgr.DeleteTenant("org1")
	if err != nil {
		t.Fatalf("failed to delete tenant: %v", err)
	}

	_, err = mgr.GetTenant("org1")
	if err == nil {
		t.Error("expected error getting deleted tenant")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewTenantManager(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = mgr.CreateTenant("org1", "Org 1", "Description")

	// Create a new manager and load
	mgr2, err := NewTenantManager(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = mgr2.LoadAll()
	if err != nil {
		t.Fatalf("failed to load tenants: %v", err)
	}

	tenant, err := mgr2.GetTenant("org1")
	if err != nil {
		t.Fatalf("failed to get loaded tenant: %v", err)
	}
	if tenant.Name != "Org 1" {
		t.Errorf("expected 'Org 1', got %s", tenant.Name)
	}
}

func TestUpdateSettings(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewTenantManager(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = mgr.CreateTenant("org1", "Org 1", "")
	err = mgr.UpdateSettings("org1", TenantSettings{
		DefaultModel:    "gpt-4",
		SandboxEnabled:  true,
	})
	if err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}

	tenant, _ := mgr.GetTenant("org1")
	if tenant.Settings.DefaultModel != "gpt-4" {
		t.Errorf("expected 'gpt-4', got %s", tenant.Settings.DefaultModel)
	}
	if !tenant.Settings.SandboxEnabled {
		t.Error("expected sandbox to be enabled")
	}
}
