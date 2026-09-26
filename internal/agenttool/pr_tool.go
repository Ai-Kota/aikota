// This file implements the PR tool (US-026): a wrapper around GitHub's `gh` CLI
// that lets the model create, update, and manage pull requests without
// invoking bash directly.
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

// PRTool wraps gh pr commands so the model can create and manage PRs.
type PRTool struct {
	// Root is the git repo root. Commands run with cwd=Root. Empty defaults to
	// the process cwd.
	Root string
}

// prCmdArgs is the decoded argument shape for PRTool.
type prCmdArgs struct {
	Action  string   `json:"action"` // create, list, view, merge, close
	Title   string   `json:"title,omitempty"`
	Body    string   `json:"body,omitempty"`
	Base    string   `json:"base,omitempty"`     // target branch
	Head    string   `json:"head,omitempty"`     // source branch (default: current)
	Draft   bool     `json:"draft,omitempty"`
	Labels  []string `json:"labels,omitempty"`
	Reviewers []string `json:"reviewers,omitempty"`
	PRNumber int     `json:"pr_number,omitempty"`
	Comment string   `json:"comment,omitempty"` // for list/view actions
}

// Name implements AgentTool.
func (t *PRTool) Name() string { return "pr" }

// Description implements AgentTool.
func (t *PRTool) Description() string {
	return "Create and manage GitHub pull requests via gh CLI. " +
		"Actions: create, list, view, merge, close. " +
		"Requires gh auth and a git repo with remote."
}

// Schema implements AgentTool.
func (t *PRTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["create", "list", "view", "merge", "close"], "description": "PR action."},
    "title": {"type": "string", "description": "PR title (required for create)."},
    "body": {"type": "string", "description": "PR body/description."},
    "base": {"type": "string", "description": "Target branch (default: main/master)."},
    "head": {"type": "string", "description": "Source branch (default: current)."},
    "draft": {"type": "boolean", "description": "Create as draft PR."},
    "labels": {"type": "array", "items": {"type": "string"}, "description": "Labels to apply."},
    "reviewers": {"type": "array", "items": {"type": "string"}, "description": "Reviewers to request."},
    "pr_number": {"type": "integer", "description": "PR number (for view/merge/close)."},
    "comment": {"type": "string", "description": "Comment to post (for view action)."}
  },
  "required": ["action"],
  "additionalProperties": false
}`)
}

// ExecutionMode implements AgentTool. PR touches network state → sequential.
func (t *PRTool) ExecutionMode() agentcore.ToolExecutionMode {
	return agentcore.ToolExecutionSequential
}

// Execute implements AgentTool.
func (t *PRTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	a, bad := decodeArgs[prCmdArgs](args, "pr")
	if bad != nil {
		return *bad, nil
	}
	if a.Action == "" {
		return errorResult("pr: action is required"), nil
	}

	root := t.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return errorResult("pr: cannot resolve working directory: " + err.Error()), nil
		}
	}

	switch a.Action {
	case "create":
		return t.create(root, a)
	case "list":
		return t.list(root)
	case "view":
		return t.view(root, a.PRNumber, a.Comment)
	case "merge":
		return t.merge(root, a.PRNumber)
	case "close":
		return t.close(root, a.PRNumber)
	default:
		return errorResult(fmt.Sprintf("pr: unknown action %q", a.Action)), nil
	}
}

func (t *PRTool) runGH(root string, extra ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "gh", append([]string{"-R", root}, extra...)...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("gh exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

func (t *PRTool) create(root string, a prCmdArgs) (agentcore.AgentToolResult, error) {
	if a.Title == "" {
		return errorResult("pr create: title is required"), nil
	}

	args := []string{"pr", "create", "--title", a.Title}
	if a.Body != "" {
		args = append(args, "--body", a.Body)
	}
	if a.Base != "" {
		args = append(args, "--base", a.Base)
	}
	if a.Head != "" {
		args = append(args, "--head", a.Head)
	}
	if a.Draft {
		args = append(args, "--draft")
	}
	if len(a.Labels) > 0 {
		args = append(args, append([]string{"--label"}, a.Labels...)...)
	}
	if len(a.Reviewers) > 0 {
		args = append(args, append([]string{"--reviewer"}, a.Reviewers...)...)
	}

	out, err := t.runGH(root, args...)
	if err != nil {
		return errorResult("pr create: " + err.Error()), nil
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(strings.TrimSpace(out))}}, nil
}

func (t *PRTool) list(root string) (agentcore.AgentToolResult, error) {
	out, err := t.runGH(root, "pr", "list", "--json", "number,title,url,state")
	if err != nil {
		return errorResult("pr list: " + err.Error()), nil
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(out)}}, nil
}

func (t *PRTool) view(root string, num int, comment string) (agentcore.AgentToolResult, error) {
	if num == 0 {
		return errorResult("pr view: pr_number is required"), nil
	}

	var out string
	var err error
	if comment != "" {
		out, err = t.runGH(root, "pr", "view", fmt.Sprintf("%d", num), "--json", "number,title,body,url,state,labels,reviewDecision")
	} else {
		out, err = t.runGH(root, "pr", "view", fmt.Sprintf("%d", num))
	}
	if err != nil {
		return errorResult("pr view: " + err.Error()), nil
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(out)}}, nil
}

func (t *PRTool) merge(root string, num int) (agentcore.AgentToolResult, error) {
	if num == 0 {
		return errorResult("pr merge: pr_number is required"), nil
	}
	out, err := t.runGH(root, "pr", "merge", fmt.Sprintf("%d", num))
	if err != nil {
		return errorResult("pr merge: " + err.Error()), nil
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(strings.TrimSpace(out))}}, nil
}

func (t *PRTool) close(root string, num int) (agentcore.AgentToolResult, error) {
	if num == 0 {
		return errorResult("pr close: pr_number is required"), nil
	}
	out, err := t.runGH(root, "pr", "close", fmt.Sprintf("%d", num))
	if err != nil {
		return errorResult("pr close: " + err.Error()), nil
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(strings.TrimSpace(out))}}, nil
}
