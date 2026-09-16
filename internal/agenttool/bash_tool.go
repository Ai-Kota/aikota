// This file implements the bash tool (US-018): run a shell command, streaming
// stdout/stderr back as tool_execution_update partials, honoring a timeout and
// context cancellation (which kills the child process group). A non-zero exit
// is surfaced as an error (isError) whose message carries the captured output.
package agenttool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

// bashDefaultTimeout bounds a command that does not specify one.
const bashDefaultTimeout = 2 * time.Minute

// bashMaxTimeout caps any requested timeout.
const bashMaxTimeout = 10 * time.Minute

// bashMaxOutputBytes caps how many bytes of combined stdout/stderr the bash tool
// returns to the model. A single command can emit megabytes (build logs, a big
// cat), which — unlike the timeout cap — would otherwise flow into context whole
// and blow the window. Output past this size is truncated to a head + tail
// preview (see truncateBashOutput), mirroring search's searchMaxResults/"[truncated
// …]" convention. This is the tool's own inner cap; a later executor-layer budget
// may impose a stricter outer limit.
const bashMaxOutputBytes = 30_000

// truncateBashOutput caps s at bashMaxOutputBytes using the shared
// truncateToBudget idiom (head + "[truncated N bytes]" marker + tail, cut on
// UTF-8 rune boundaries). It is the bash tool's own inner cap; the executor
// layer applies a separate, uniform outer budget afterward.
func truncateBashOutput(s string) string {
	return truncateToBudget(s, bashMaxOutputBytes)
}

// trimUTF8Prefix drops trailing bytes of s that form an incomplete rune, so the
// returned prefix ends on a rune boundary.
func trimUTF8Prefix(s string) string {
	for len(s) > 0 {
		if r, size := utf8.DecodeLastRuneInString(s); r == utf8.RuneError && size <= 1 {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

// trimUTF8Suffix drops leading bytes of s that form an incomplete rune, so the
// returned suffix starts on a rune boundary.
func trimUTF8Suffix(s string) string {
	for len(s) > 0 {
		if r, size := utf8.DecodeRuneInString(s); r == utf8.RuneError && size <= 1 {
			s = s[1:]
			continue
		}
		break
	}
	return s
}

// BashTool runs shell commands. Dir bounds the working directory (empty = the
// process CWD). Shell selects the interpreter (empty = "bash -c").
type BashTool struct {
	// Dir is the working directory for commands. Empty uses the process CWD.
	Dir string
	// Shell is the interpreter path. Empty defaults to "bash".
	Shell string
	// Jobs holds background jobs launched with run_in_background. When nil,
	// run_in_background is rejected (the front-end did not wire a store).
	Jobs *BashJobStore
}

// bashToolArgs is the decoded argument shape for BashTool.
type bashToolArgs struct {
	// Command is the shell command line to run.
	Command string `json:"command"`
	// TimeoutMs optionally overrides the default timeout (milliseconds).
	TimeoutMs int `json:"timeout_ms,omitempty"`
	// RunInBackground detaches the command from the turn: it keeps running after
	// Execute returns, and its output is drained later via bash_output. A
	// background command has no default timeout (so dev servers/watchers run
	// indefinitely); timeout_ms still caps it if given.
	RunInBackground bool `json:"run_in_background,omitempty"`
}

// Name implements AgentTool.
func (t *BashTool) Name() string { return "bash" }

// Description implements AgentTool.
func (t *BashTool) Description() string {
	return "Run a shell command, streaming stdout/stderr. Supports a timeout " +
		"and cancellation. A non-zero exit code is reported as an error. " +
		"Set run_in_background=true for long-running commands (dev servers, " +
		"watchers): it returns immediately with a bash_id you drain with " +
		"bash_output and stop with kill_bash. " +
		"On Windows the command runs under bash if available (Git Bash/WSL), " +
		"else PowerShell, else cmd — prefer portable commands."
}

// Schema implements AgentTool.
func (t *BashTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command":    {"type": "string", "description": "Shell command line to run."},
    "timeout_ms": {"type": "integer", "description": "Timeout in milliseconds (capped at 10 minutes). Ignored in background unless set.", "minimum": 0},
    "run_in_background": {"type": "boolean", "description": "Run detached and return immediately with a bash_id; drain output with bash_output, stop with kill_bash."}
  },
  "required": ["command"],
  "additionalProperties": false
}`)
}

// ExecutionMode implements AgentTool. Commands can have side effects → sequential.
func (t *BashTool) ExecutionMode() agentcore.ToolExecutionMode {
	return agentcore.ToolExecutionSequential
}

// shellLookPath resolves a program on PATH. It is a package var so tests can
// simulate a Windows box with or without bash installed.
var shellLookPath = exec.LookPath

// shellProbe verifies a candidate interpreter can actually run a no-op command.
// It is a package var so tests stub it; the default (defaultShellProbe) runs
// "exit 0" through the candidate and treats any failure as "not usable".
var shellProbe = defaultShellProbe

// gitBashCandidates are well-known MSYS2/Git Bash install points on Windows.
// They are consulted before "bash" on PATH because the PATH entry can be a
// broken WindowsApps alias (C:\Windows\System32\bash.exe routes to WSL and
// fails when no distro is installed, or when WSL itself is misconfigured);
// the real MSYS binary in the install tree never depends on WSL. A missing
// install point is simply not a candidate rather than an error. It is a
// package var so tests can point it at temporary install trees.
var gitBashCandidates = []string{
	`C:\Program Files\Git\usr\bin\bash.exe`,
	`C:\Program Files (x86)\Git\usr\bin\bash.exe`,
	`C:\Program Files\Git for Windows\usr\bin\bash.exe`,
	`C:\Program Files (x86)\Git for Windows\usr\bin\bash.exe`,
}

// fileExists reports whether path is an existing regular file. It is a package
// var so tests can stub the install-tree existence check without touching the
// host filesystem.
var fileExists = func(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// resolveShell picks the interpreter and the flag that makes it read the command
// from the next argument. An explicit shell (BashTool.Shell) is always honored
// as a POSIX-style "<shell> -c <command>".
//
// On Windows with no explicit shell, the naive "bash -c" hardcode fails on
// stock machines that have no bash on PATH — the model then retries bash
// blindly and every call errors (issue #518). Worse, "bash" on PATH is not
// guaranteed to be a healthy bash: the App Execution alias
// (C:\Windows\System32\bash.exe) routes into WSL, and a WSL install with no
// usable distro (or a misconfigured /etc/wsl.conf) makes every launch fail.
// So the Windows candidate order is:
//
//  1. MSYS/Git Bash from its install tree (native, no WSL dependency),
//  2. "bash" on PATH (healthy WSL/MSYS2 installs land here),
//  3. "wsl" launcher (bash syntax inside a distro),
//  4. "powershell", then "cmd",
//
// and each candidate is probed with a no-op command ("exit 0") so a broken
// candidate is skipped instead of failing every tool call. The first candidate
// that probes clean wins.
func resolveShell(explicit, goos string, lookPath func(string) (string, error)) (shell, flag string) {
	if explicit != "" {
		return explicit, "-c"
	}
	if goos != "windows" {
		// POSIX platforms keep the historical behavior: bash as-is, no probe
		// (a missing bash still surfaces as the actionable *exec.Error in
		// Execute, which is the correct signal there).
		return "bash", "-c"
	}

	type candidate struct {
		shell string
		flag  string
	}
	var cands []candidate

	// 1. Git Bash / MSYS2 from the install tree: never routes through WSL.
	for _, p := range gitBashCandidates {
		if fileExists(p) {
			cands = append(cands, candidate{p, "-c"})
		}
	}
	// 2. "bash" on PATH: healthy WSL or MSYS installs register here.
	if p, err := lookPath("bash"); err == nil {
		cands = append(cands, candidate{p, "-c"})
	}
	// 3. The WSL launcher itself: runs bash syntax inside a distro.
	if p, err := lookPath("wsl"); err == nil {
		cands = append(cands, candidate{p, "-e /bin/sh -c"})
	}
	// 4. Windows-native fallbacks.
	if p, err := lookPath("powershell"); err == nil {
		cands = append(cands, candidate{p, "-Command"})
	}
	cands = append(cands, candidate{`C:\Windows\System32\cmd.exe`, "/C"})

	for _, c := range cands {
		if shellProbe(c.shell, c.flag) {
			return c.shell, c.flag
		}
	}
	// Nothing probed clean: return the first candidate anyway so Execute's
	// *exec.Error path produces the actionable guidance message.
	return cands[0].shell, cands[0].flag
}

// defaultShellProbe runs a no-op command ("exit 0") through the candidate
// interpreter, capped at 5 s. A non-zero exit — or the interpreter not
// existing at all — marks the candidate unusable so resolveShell moves on to
// the next one. The 5 s cap protects against a wedged WSL launcher that
// neither succeeds nor fails promptly; a probe timeout counts as "broken" and
// the candidate is skipped rather than the agent frozen.
func defaultShellProbe(shell, flag string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// For the WSL launcher the flag is "-e /bin/sh -c"; the probe therefore
	// becomes: wsl -e /bin/sh -c exit 0, which is the exact shape of the
	// commands the tool would actually run.
	args := strings.Fields(flag)
	args = append(args, "exit 0")
	return exec.CommandContext(ctx, shell, args...).Run() == nil
}

// streamWriter forwards each written chunk to onUpdate as a growing partial
// result while accumulating the full output. It is safe for concurrent use so
// stdout and stderr can share the same combined buffer.
type streamWriter struct {
	mu       *sync.Mutex
	buf      *bytes.Buffer
	onUpdate agentcore.ToolUpdateFunc
}

func (w streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	snapshot := w.buf.String()
	w.mu.Unlock()
	if w.onUpdate != nil {
		w.onUpdate(agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(snapshot)}})
	}
	return len(p), nil
}

// Execute implements AgentTool. It streams combined stdout/stderr via onUpdate,
// enforces a timeout, and kills the process on context cancellation. A non-zero
// exit returns a Go error (→ isError) carrying the exit code and output.
func (t *BashTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	a, bad := decodeArgs[bashToolArgs](args, "bash")
	if bad != nil {
		return *bad, nil
	}
	if a.Command == "" {
		return errorResult("bash: command is required"), nil
	}

	if a.RunInBackground {
		return t.startBackground(a)
	}

	timeout := bashDefaultTimeout
	if a.TimeoutMs > 0 {
		timeout = time.Duration(a.TimeoutMs) * time.Millisecond
	}
	if timeout > bashMaxTimeout {
		timeout = bashMaxTimeout
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell, flag := resolveShell(t.Shell, runtime.GOOS, shellLookPath)
	cmd := exec.CommandContext(runCtx, shell, flag, a.Command)
	if t.Dir != "" {
		cmd.Dir = t.Dir
	}

	var mu sync.Mutex
	var combined bytes.Buffer
	sw := streamWriter{mu: &mu, buf: &combined, onUpdate: onUpdate}
	cmd.Stdout = sw
	cmd.Stderr = sw

	err := cmd.Run()

	mu.Lock()
	output := combined.String()
	mu.Unlock()

	// Cap the output before it enters any ToolResult / error message, so a single
	// command's huge output cannot blow the model's context. Truncation keeps a
	// head + tail preview with a "[truncated N bytes]" marker in the middle.
	output = truncateBashOutput(output)

	// Context cancellation / timeout takes precedence in the message.
	if runCtx.Err() == context.DeadlineExceeded {
		return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(output)}},
			fmt.Errorf("bash: command timed out after %s\n%s", timeout, output)
	}
	if ctx.Err() == context.Canceled {
		return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(output)}},
			fmt.Errorf("bash: command canceled\n%s", output)
	}

	// A missing interpreter (no bash/powershell/cmd on PATH) surfaces as an
	// *exec.Error before the command ever runs. Report it with actionable
	// guidance instead of a bare "code -1", so the model stops retrying blindly.
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(output)}},
			fmt.Errorf("bash: could not start shell %q: %v. On Windows install Git Bash or WSL (or configure a shell); commands are bash syntax", shell, execErr.Err)
	}

	if err != nil {
		exitCode := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		}
		return agentcore.AgentToolResult{
				Content: agentcore.ContentList{agentcore.NewTextContent(output)},
				Details: map[string]any{"exitCode": exitCode},
			},
			fmt.Errorf("bash: command exited with code %d\n%s", exitCode, output)
	}

	return agentcore.AgentToolResult{
		Content: agentcore.ContentList{agentcore.NewTextContent(output)},
		Details: map[string]any{"exitCode": 0},
	}, nil
}

// startBackground launches the command detached from the turn context and
// returns immediately with a bash_id. The job runs under its own cancelable
// context (rooted at context.Background(), not the turn ctx which is canceled
// when the turn ends), so it survives past Execute. A background command has no
// default timeout — a dev server or watcher is expected to run indefinitely —
// but an explicit timeout_ms still caps it. Its combined output accumulates in
// the job's buffer for bash_output to drain; kill_bash cancels its context.
func (t *BashTool) startBackground(a bashToolArgs) (agentcore.AgentToolResult, error) {
	if t.Jobs == nil {
		return errorResult("bash: run_in_background is not available in this environment"), nil
	}

	var jobCtx context.Context
	var cancel context.CancelFunc
	if a.TimeoutMs > 0 {
		timeout := time.Duration(a.TimeoutMs) * time.Millisecond
		if timeout > bashMaxTimeout {
			timeout = bashMaxTimeout
		}
		jobCtx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		jobCtx, cancel = context.WithCancel(context.Background())
	}

	shell, flag := resolveShell(t.Shell, runtime.GOOS, shellLookPath)
	cmd := exec.CommandContext(jobCtx, shell, flag, a.Command)
	if t.Dir != "" {
		cmd.Dir = t.Dir
	}

	job := t.Jobs.create(a.Command, cancel)
	w := job.writer()
	cmd.Stdout = w
	cmd.Stderr = w

	if err := cmd.Start(); err != nil {
		cancel()
		job.finish(-1, err.Error())
		return errorResult(fmt.Sprintf("bash: could not start background command: %v", err)), nil
	}

	go func() {
		err := cmd.Wait()
		cancel()
		exitCode := 0
		errMsg := ""
		if err != nil {
			exitCode = -1
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				exitCode = ee.ExitCode()
			}
			errMsg = err.Error()
		}
		job.finish(exitCode, errMsg)
	}()

	msg := fmt.Sprintf("started background command %s: %s\nuse bash_output %q to read its output, kill_bash %q to stop it", job.ID, a.Command, job.ID, job.ID)
	return agentcore.AgentToolResult{
		Content: agentcore.ContentList{agentcore.NewTextContent(msg)},
		Details: map[string]any{"bash_id": job.ID, "background": true},
	}, nil
}
