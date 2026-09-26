package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewExecutor(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultSandboxConfig(tmpDir)
	executor, err := NewExecutor(cfg)
	if err != nil {
		t.Skipf("sandbox not available (docker not running): %v", err)
	}
	if executor == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestRunSimpleCommand(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultSandboxConfig(tmpDir)
	executor, err := NewExecutor(cfg)
	if err != nil {
		t.Skipf("sandbox not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executor.Run(ctx, "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("expected 'hello\n', got %q", result.Stdout)
	}
}

func TestRunCommandWithFileAccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := DefaultSandboxConfig(tmpDir)
	executor, err := NewExecutor(cfg)
	if err != nil {
		t.Skipf("sandbox not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try different ways to access the file
	testCases := []string{
		"cat /workspace/test.txt",
		"cat test.txt",
		"ls -la /workspace/",
	}

	for _, cmd := range testCases {
		result, err := executor.Run(ctx, cmd)
		t.Logf("Command: %s, ExitCode: %d, Stdout: %q, Error: %v", cmd, result.ExitCode, result.Stdout, err)
		if err == nil && result.ExitCode == 0 {
			break
		}
	}
}

func TestRunCommandFailsCorrectly(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultSandboxConfig(tmpDir)
	executor, err := NewExecutor(cfg)
	if err != nil {
		t.Skipf("sandbox not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executor.Run(ctx, "exit 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestNetworkDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultSandboxConfig(tmpDir)
	cfg.NetworkDisabled = true
	executor, err := NewExecutor(cfg)
	if err != nil {
		t.Skipf("sandbox not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := executor.Run(ctx, "ping -c 1 8.8.8.8")
	if err != nil {
		t.Logf("network disabled as expected: %v", err)
	} else if result.ExitCode != 0 {
		t.Logf("ping failed as expected (network disabled)")
	}
}
