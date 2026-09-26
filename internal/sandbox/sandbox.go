// Package sandbox provides Docker-based isolation for command execution.
// It runs commands in ephemeral containers with restricted filesystem access,
// preventing potentially harmful operations from affecting the host system.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SandboxConfig holds the configuration for sandboxed execution.
type SandboxConfig struct {
	// Image is the Docker image to use (default: alpine:latest)
	Image string
	// WorkDir is the workspace directory to mount into the container
	WorkDir string
	// Timeout is the maximum execution time (default: 5 minutes)
	Timeout time.Duration
	// ReadWriteWorkdir controls whether the workspace is writable (default: true)
	ReadWriteWorkdir bool
	// NetworkDisabled disables network access (default: true)
	NetworkDisabled bool
	// Privileged disables security restrictions (default: false, NOT recommended)
	Privileged bool
}

// DefaultSandboxConfig returns sensible defaults.
func DefaultSandboxConfig(workDir string) *SandboxConfig {
	return &SandboxConfig{
		Image:            "alpine:latest",
		WorkDir:          workDir,
		Timeout:          5 * time.Minute,
		ReadWriteWorkdir: true,
		NetworkDisabled:  true,
		Privileged:       false,
	}
}

// SandboxResult holds the output from a sandboxed command.
type SandboxResult struct {
	ExitCode     int
	Stdout       string
	Stderr       string
	Duration     time.Duration
	ContainerID  string
}

// Executor runs commands inside a Docker sandbox.
type Executor struct {
	config *SandboxConfig
}

// NewExecutor creates a new sandbox executor with the given config.
func NewExecutor(config *SandboxConfig) (*Executor, error) {
	if config == nil {
		config = DefaultSandboxConfig(".")
	}

	// Verify Docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("sandbox: docker not found in PATH: %w", err)
	}

	// Test Docker connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sandbox: docker daemon not reachable: %w", err)
	}

	return &Executor{config: config}, nil
}

// Run executes a command in a sandboxed container.
func (e *Executor) Run(ctx context.Context, command string) (*SandboxResult, error) {
	start := time.Now()

	// Prepare mount options
	mountOpts := "ro"
	if e.config.ReadWriteWorkdir {
		mountOpts = "rw"
	}

	// Resolve absolute path for workdir
	workDir := e.config.WorkDir
	if !strings.HasPrefix(workDir, "/") {
		abs, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("sandbox: resolve workdir: %w", err)
		}
		workDir = abs
	}

	// Build docker run command
	args := []string{
		"run",
		"--rm", // Automatically remove container on exit
		"--workdir", "/workspace",
		"--volume", fmt.Sprintf("%s:/workspace:%s", workDir, mountOpts),
		"--interactive",
	}

	if e.config.NetworkDisabled {
		args = append(args, "--network", "none")
	}

	if e.config.Privileged {
		args = append(args, "--privileged")
	} else {
		// Drop all capabilities for security
		args = append(args, "--cap-drop", "ALL")
		// Add only necessary capabilities
		args = append(args, "--cap-add", "CHOWN")
		args = append(args, "--cap-add", "FOWNER")
		args = append(args, "--cap-add", "SETUID")
		args = append(args, "--cap-add", "SETGID")
	}

	args = append(args, e.config.Image, "sh", "-c", command)

	// Execute with timeout
	execCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	result := &SandboxResult{
		Stdout:      stdout.String(),
		Stderr:      stderr.String(),
		Duration:    duration,
		ExitCode:    0,
		ContainerID: "", // --rm removes it, so we don't have it
	}

	if err != nil {
		// Extract exit code if available
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, fmt.Errorf("sandbox: execute command: %w", err)
		}
	}

	return result, nil
}

// HealthCheck verifies the sandbox is operational.
func (e *Executor) HealthCheck(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "ping", "-c", "1", "8.8.8.8")
	cmd.Env = os.Environ()
	return cmd.Run()
}

// GetContainerInfo returns info about a running container (if any).
func GetContainerInfo(containerID string) (map[string]interface{}, error) {
	cmd := exec.Command("docker", "inspect", containerID)
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("sandbox: inspect container: %w", err)
	}

	var info []map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("sandbox: parse container info: %w", err)
	}

	if len(info) == 0 {
		return nil, fmt.Errorf("sandbox: no info for container %s", containerID)
	}

	return info[0], nil
}
