//go:build windows

// Document + guard tests for the broken-WSL state observed on this host
// (2026-09-16).
//
// `wsl -e /bin/sh -c "exit 0"` returns exit 0 on this host while the WSL
// layer dumps errors to stderr:
//
//	wsl: Unknown key 'automount.crossDistro' in /etc/wsl.conf:3
//	<3>WSL (11 - Relay) ERROR: CreateProcessCommon:818:
//	      execvpe(D:/Git/usr/bin/sh) failed: No such file or directory
//
// The default distro cannot start /bin/sh. Any tool launch that routes the
// user's bash-syntax command through WSL fails even though the process exit
// code is 0 — the agent's bash tool retried in that broken state.
//
// This host also has a healthy Git Bash at D:\Git\usr\bin\bash.exe, which is
// what the tool's resolver picks (exec.LookPath("bash")). These tests pin the
// broken WSL state and guard the healthy fallback.
package agenttool

import (
	"os/exec"
	"strings"
	"testing"
)

// TestWslDefaultDistroStartsShellFails confirms the defect is present: the
// WSL layer reports an unknown /etc/wsl.conf key and/or a
// CreateProcessCommon failure for the default-distro /bin/sh launch. If the
// host has been repaired since, the test skips rather than fails (host state
// is not a property of this repo).
func TestWslDefaultDistroStartsShellFails(t *testing.T) {
	if _, err := exec.LookPath("wsl"); err != nil {
		t.Skip("wsl.exe not on PATH; this host has no WSL launcher")
	}
	out, _ := exec.Command("wsl", "-e", "/bin/sh", "-c", "exit 0").CombinedOutput()
	joined := strings.ToLower(string(out))
	if strings.Contains(joined, "createprocesscommon") ||
		strings.Contains(joined, "unknown key") ||
		strings.Contains(joined, "automount.crossdistro") {
		t.Logf("defect confirmed: default WSL distro cannot start /bin/sh. output=%q", string(out))
		return
	}
	t.Skip("default WSL distro currently starts /bin/sh cleanly; defect not present, nothing to pin")
}

// TestGitBashWorks guards the healthy fallback that resolveShell selects on
// this host.
func TestGitBashWorks(t *testing.T) {
	p, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash on PATH on this host")
	}
	out, err := exec.Command(p, "-c", "echo ok").CombinedOutput()
	if err != nil {
		t.Fatalf("bash -c 'echo ok' failed (fallback broken?): %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "ok" {
		t.Errorf("output = %q, want ok", got)
	}
}
