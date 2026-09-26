package agenttool

import (
	"os"
	"testing"
)

func TestCommandFilterAllowAll(t *testing.T) {
	f, err := NewCommandFilter(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allow, reason := f.AllowCommand("git status")
	if !allow {
		t.Errorf("expected command to be allowed, got reason: %s", reason)
	}

	allow, reason = f.AllowCommand("go test ./...")
	if !allow {
		t.Errorf("expected command to be allowed, got reason: %s", reason)
	}
}

func TestCommandFilterDenyPattern(t *testing.T) {
	f, err := NewCommandFilter(&CommandFilterConfig{
		DenyPatterns: []string{`rm\s+-rf`},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allow, reason := f.AllowCommand("rm -rf /")
	if allow {
		t.Error("expected command to be denied")
	}
	if reason == "" {
		t.Error("expected denial reason")
	}
}

func TestCommandFilterDenylist(t *testing.T) {
	f, err := NewCommandFilter(&CommandFilterConfig{
		DeniedCommands: []string{"sudo", "dd"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allow, _ := f.AllowCommand("sudo reboot")
	if allow {
		t.Error("expected sudo to be denied")
	}

	allow, _ = f.AllowCommand("dd if=/dev/zero")
	if allow {
		t.Error("expected dd to be denied")
	}

	allow, _ = f.AllowCommand("git push")
	if !allow {
		t.Error("expected git to be allowed")
	}
}

func TestCommandFilterAllowlist(t *testing.T) {
	f, err := NewCommandFilter(&CommandFilterConfig{
		AllowedCommands: []string{"git", "go", "cat"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allow, _ := f.AllowCommand("git status")
	if !allow {
		t.Error("expected git to be allowed")
	}

	allow, _ = f.AllowCommand("go test")
	if !allow {
		t.Error("expected go to be allowed")
	}

	allow, reason := f.AllowCommand("rm -rf /tmp")
	if allow {
		t.Error("expected rm to be denied by allowlist")
	}
	if reason == "" {
		t.Error("expected denial reason")
	}
}

func TestLoadCommandFilterFromEnv(t *testing.T) {
	os.Setenv("AIKOTA_ALLOW_COMMANDS", "git,go")
	os.Setenv("AIKOTA_DENY_COMMANDS", "sudo,rm")
	defer os.Unsetenv("AIKOTA_ALLOW_COMMANDS")
	defer os.Unsetenv("AIKOTA_DENY_COMMANDS")

	f, err := LoadCommandFilterFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allow, _ := f.AllowCommand("git status")
	if !allow {
		t.Error("expected git to be allowed")
	}

	allow, _ = f.AllowCommand("sudo rm")
	if allow {
		t.Error("expected sudo to be denied")
	}
}

func TestCommandFilterEmptyCommand(t *testing.T) {
	f, err := NewCommandFilter(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allow, _ := f.AllowCommand("")
	if !allow {
		t.Error("expected empty command to be allowed")
	}
}
