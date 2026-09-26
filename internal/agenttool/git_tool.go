// This file implements the git tool (US-024): a thin wrapper around `git` that
// lets the model inspect and commit changes without invoking bash directly.
//
// It deliberately does NOT run destructive commands (push, reset --hard, force
// push). Those remain bash-only so the user/toolkit still controls the blast
// radius — we only add convenience for the common read / staging / commit path.
package agenttool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

// GitTool wraps git commands so the model can inspect status, diff, add, and
// commit without spelling out `bash` every time. All paths are resolved against
// Root before git is invoked.
type GitTool struct {
	// Root is the working tree root. Commands run with cwd=Root (git needs this
	// to resolve relative paths correctly). Empty defaults to the process cwd.
	Root string
}

// gitCmdArgs is the decoded argument shape for GitTool.
type gitCmdArgs struct {
	// Action is the git sub-command to run. Recognised: status, diff, diffAll,
	// log, show, add, commit. Other values are rejected.
	Action string `json:"action"`
	// Paths are optional file/directory arguments forwarded to git. For add and
	// commit they are treated as "also stage these paths" rather than "commit
	// only these paths", matching the model's mental model of the workflow.
	Paths []string `json:"paths,omitempty"`
	// Message is required for commit; ignored for other actions.
	Message string `json:"message,omitempty"`
	// All flag for add: stage every tracked change (default is interactive/no).
	All bool `json:"all,omitempty"`
}

// Name implements AgentTool.
func (t *GitTool) Name() string { return "git" }

// Description implements AgentTool.
func (t *GitTool) Description() string {
	return "Inspect and commit changes via git. Supported actions: " +
		"status (short), diff, diffAll, log, show, add, commit. " +
		"Destructive actions (push, reset --hard, force) are not exposed here — use bash."
}

// Schema implements AgentTool.
func (t *GitTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["status", "diff", "diffAll", "log", "show", "add", "commit"], "description": "Git sub-command to run."},
    "paths":  {"type": "array",  "items": {"type": "string"}, "description": "Optional path filters."},
    "message":{"type": "string", "description": "Commit message (required for action=commit)."},
    "all":    {"type": "boolean","description": "Stage all tracked changes (default false)."}
  },
  "required": ["action"],
  "additionalProperties": false
}`)
}

// ExecutionMode implements AgentTool. Git touches the working tree or runs
// external processes → sequential.
func (t *GitTool) ExecutionMode() agentcore.ToolExecutionMode {
	return agentcore.ToolExecutionSequential
}

// Execute implements AgentTool.
func (t *GitTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	a, bad := decodeArgs[gitCmdArgs](args, "git")
	if bad != nil {
		return *bad, nil
	}
	if a.Action == "" {
		return errorResult("git: action is required"), nil
	}

	root := t.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return errorResult("git: cannot resolve working directory: " + err.Error()), nil
		}
	}

	switch a.Action {
	case "status":
		return t.status(root, a.Paths)
	case "diff":
		return t.diff(root, a.Paths, false)
	case "diffAll":
		return t.diff(root, a.Paths, true)
	case "log":
		return t.log(root, a.Paths)
	case "show":
		return t.show(root, a.Paths)
	case "add":
		return t.add(root, a.Paths, a.All)
	case "commit":
		if a.Message == "" {
			return errorResult("git commit: message is required"), nil
		}
		return t.commit(root, a.Message, a.Paths)
	default:
		return errorResult(fmt.Sprintf("git: unknown action %q", a.Action)), nil
	}
}

// runGit runs a git command in root and returns its stdout. Errors (non-zero
// exit, missing git) are encoded as error results so the model can react.
func (t *GitTool) runGit(root string, extra ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", root}, extra...)...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

// status runs `git status --short` when paths are empty, otherwise delegates
// to the normal status with pathspec.
func (t *GitTool) status(root string, paths []string) (agentcore.AgentToolResult, error) {
	args := []string{"status"}
	if len(paths) == 0 {
		args = append(args, "--short")
	}
	for _, p := range paths {
		args = append(args, p)
	}
	out, err := t.runGit(root, args...)
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(out)}}, err
}

// diff runs `git diff` or `git diff --cached` depending on stagedOnly.
func (t *GitTool) diff(root string, paths []string, stagedOnly bool) (agentcore.AgentToolResult, error) {
	args := []string{"diff"}
	if stagedOnly {
		args = append(args, "--cached")
	}
	for _, p := range paths {
		args = append(args, p)
	}
	out, err := t.runGit(root, args...)
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(out)}}, err
}

// log runs `git log --oneline -20`.
func (t *GitTool) log(root string, paths []string) (agentcore.AgentToolResult, error) {
	args := []string{"log", "--oneline", "-20"}
	for _, p := range paths {
		args = append(args, p)
	}
	out, err := t.runGit(root, args...)
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(out)}}, err
}

// show runs `git show <ref>` with optional path.
func (t *GitTool) show(root string, paths []string) (agentcore.AgentToolResult, error) {
	args := []string{"show"}
	if len(paths) > 0 {
		args = append(args, paths[0])
	}
	out, err := t.runGit(root, args...)
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(out)}}, err
}

// add stages paths. When all is true, it stages all tracked changes.
func (t *GitTool) add(root string, paths []string, all bool) (agentcore.AgentToolResult, error) {
	args := []string{"add"}
	if all {
		args = append(args, "-A")
	} else {
		for _, p := range paths {
			args = append(args, p)
		}
	}
	out, err := t.runGit(root, args...)
	msg := strings.TrimSpace(out)
	if err != nil {
		return errorResult("git add: " + err.Error()), nil
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(msg)}}, nil
}

// commit creates a commit with the given message, optionally adding paths first.
func (t *GitTool) commit(root string, message string, paths []string) (agentcore.AgentToolResult, error) {
	// Stage any explicitly requested paths first.
	if len(paths) > 0 {
		args := append([]string{"add"}, paths...)
		if _, err := t.runGit(root, args...); err != nil {
			return errorResult("git commit: staging failed: " + err.Error()), nil
		}
	}
	args := []string{"commit", "-m", message}
	out, err := t.runGit(root, args...)
	if err != nil {
		return errorResult("git commit: " + err.Error()), nil
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(strings.TrimSpace(out))}}, nil
}
