// Package hooks implements aikota's user-extensible lifecycle hook system: a
// config-driven way to run shell commands at agent lifecycle points (tool
// calls, prompt submission, session start/end, etc.) without writing Go or
// compiling a plugin. It is a leaf package depending only on the standard
// library, so it can be composed into the runtime/cli layers without creating
// an import cycle.
package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

// CheckScopeHook is a built-in PreToolUse hook that enforces task-scope
// boundaries. It reads the project's docs/tasks/T-*.md definition files and
// validates that write/edit tool calls stay within the declared scope for the
// active task. When no active task is declared, project-file writes are blocked
// to prevent accidental sprawl.
type CheckScopeHook struct {
	// Root is the project root directory. When empty the hook falls back to cwd.
	Root string
	// AllowedPaths is an optional override list. When non-empty only these paths
	// (or paths under them) are writable regardless of task scope.
	AllowedPaths []string
}

// BeforeToolCall adapts CheckScopeHook to the BeforeToolCall seam. Returns nil
// when the call is allowed, or a blocking decision with an explanatory reason
// when it is not.
func (h *CheckScopeHook) BeforeToolCall(ctx context.Context, call agentcore.AgentToolCall) *agentcore.BeforeToolCallDecision {
	b := h.Check(call)
	if b == nil {
		return nil
	}
	content := agentcore.ContentList{agentcore.NewTextContent(b.Reason)}
	return &agentcore.BeforeToolCallDecision{Block: true, Content: &content}
}

// Blocked describes a refused tool call.
type Blocked struct {
	Rule   string
	Reason string
}

// Check evaluates the scope rules against call, returning nil when the call is
// allowed. A nil returned value means "proceed".
func (h *CheckScopeHook) Check(call agentcore.AgentToolCall) *Blocked {
	switch call.Name {
	case "write", "edit":
		return h.checkWritePath(call.Arguments)
	default:
		return nil
	}
}

// checkWritePath applies the scope rules to a write/edit call.
func (h *CheckScopeHook) checkWritePath(args json.RawMessage) *Blocked {
	var in struct {
		Path    string `json:"path"`
		OldText string `json:"old_string,omitempty"`
		NewText string `json:"new_string,omitempty"`
	}
	if err := json.Unmarshal(args, &in); err != nil || in.Path == "" {
		return nil
	}
	p := filepath.ToSlash(filepath.Clean(in.Path))

	// Allowlist override: when set, bypass all scope checks.
	if len(h.AllowedPaths) > 0 {
		if h.pathAllowed(p, h.AllowedPaths) {
			return nil
		}
	}

	// Read the active task from the project's docs/tasks directory.
	root := h.Root
	if root == "" {
		root = "."
	}
	taskID, allowed, denied, newDirs := h.loadTaskBoundaries(root, p)

	// If no active task is declared, block writes to project files.
	if taskID == "" {
		if h.isBlockedPath(p, root, allowed, denied, newDirs) {
			return &Blocked{
				Rule:   "intent-gate",
				Reason: "check-scope: no active task declared; project-file writes are blocked until a task is started with begin-task.",
			}
		}
		return nil
	}

	// Check denied list first.
	if h.pathInList(p, denied) {
		return &Blocked{
			Rule:   "denied-path",
			Reason: "check-scope: " + p + " is in the denied list for task " + taskID,
		}
	}
	// Root-level markdown files are always exempt so users can still
	// scaffold the project (README.md, etc.) even when a task is active.
	if h.isRootMarkdown(p) {
		return nil
	}
	// Allow writes to paths that are explicitly in the allowed list.
	if h.pathInList(p, allowed) {
		return nil
	}
	// Allow new files that live under an explicitly permitted directory.
	if !h.fileExists(p) && len(newDirs) > 0 && h.pathInDirList(p, newDirs) {
		return nil
	}
	// If the allowed list is non-empty and the path is not in it, block.
	if len(allowed) > 0 {
		return &Blocked{
			Rule:   "allowed-path",
			Reason: "check-scope: " + p + " is not in the allowed list for task " + taskID,
		}
	}
	return nil
}

// pathAllowed reports whether p matches any entry in allow. Prefix entries
// (ending with "/") match anything under that prefix; exact entries match p
// verbatim; glob patterns are matched with filepath.Match.
func (h *CheckScopeHook) pathAllowed(p string, allow []string) bool {
	for _, a := range allow {
		a = filepath.ToSlash(a)
		if strings.HasSuffix(a, "/") {
			if strings.HasPrefix(p, a) {
				return true
			}
		} else if p == a {
			return true
		} else {
			if matched, _ := filepath.Match(a, p); matched {
				return true
			}
		}
	}
	return false
}

// pathInList reports whether p matches any entry in list using the same
// prefix/exact semantics as pathAllowed.
func (h *CheckScopeHook) pathInList(p string, list []string) bool {
	return h.pathAllowed(p, list)
}

// pathInDirList reports whether p lives under any directory in dirs.
func (h *CheckScopeHook) pathInDirList(p string, dirs []string) bool {
	for _, d := range dirs {
		d = strings.TrimRight(filepath.ToSlash(d), "/") + "/"
		if strings.HasPrefix(p, d) {
			return true
		}
	}
	return false
}

// isBlockedPath reports whether a project-file write is blocked when no valid
// task is declared. Exemptions:
//   - .aikota/ internal state so the hooks can manage themselves;
//   - task definition files docs/tasks/T-*.md (planning layer);
//   - an initialization window: while the project has no task definitions yet
//     (fresh template), scaffolding docs/ and root markdown is allowed.
func (h *CheckScopeHook) isBlockedPath(p, root string, allowed, denied, newDirs []string) bool {
	if strings.HasPrefix(p, ".aikota/") || strings.HasPrefix(p, ".claude/") {
		return false
	}
	if h.isTaskDefinitionPath(p) {
		return false
	}
	// Root-level markdown files (README.md, etc.) are always exempt so
	// users can still scaffold the project even before any task is declared.
	if h.isRootMarkdown(p) {
		return false
	}
	if !h.hasTaskDefinitions(root) {
		if strings.HasPrefix(p, "docs/") {
			return false
		}
	}
	return true
}

// isTaskDefinitionPath reports whether p is a per-task definition file
// (docs/tasks/T-*.md) or the task board index (docs/tasks/TASKS.md).
func (h *CheckScopeHook) isTaskDefinitionPath(p string) bool {
	base := filepath.Base(p)
	return strings.HasPrefix(p, "docs/tasks/") && strings.HasSuffix(p, ".md") &&
		(strings.HasPrefix(base, "T-") || base == "TASKS.md")
}

// hasTaskDefinitions reports whether docs/tasks/TASKS.md exists and contains
// at least one task row.
func (h *CheckScopeHook) hasTaskDefinitions(root string) bool {
	data, err := h.readFile(root, "docs/tasks/TASKS.md")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "| T-")
}

// loadTaskBoundaries reads the active task definition and returns the parsed
// boundaries. When no active task can be resolved it returns ("", nil, nil, nil)
// and the caller should fall back to the intent-gate behavior.
func (h *CheckScopeHook) loadTaskBoundaries(root, p string) (taskID string, allowed, denied, newDirs []string) {
	// 1. Read SESSION-STATE.md to get the active task ID (JSON format).
	if data, err := h.readFile(root, "docs/tasks/SESSION-STATE.md"); err == nil {
		if id := h.extractTaskIDJSON(string(data)); id != "" {
			return h.loadTaskFile(root, id)
		}
	}
	// 2. Fall back to scanning TASKS.md for the most recently declared task.
	if data, err := h.readFile(root, "docs/tasks/TASKS.md"); err == nil {
		if id := h.extractLatestTaskID(string(data)); id != "" {
			return h.loadTaskFile(root, id)
		}
	}
	return "", nil, nil, nil
}

// loadTaskFile reads a single task definition file and returns its parsed
// boundaries. On error it returns the taskID with empty boundaries so the
// caller treats it as "no scope declared" (fail-open for task config errors).
// The file name follows the SPT convention "T-{id}.md" where {id} is the task
// identifier without the leading "T-" prefix (so a session state with
// "task_id":"T-1" is accepted and resolved to "T-T-1.md").
func (h *CheckScopeHook) loadTaskFile(root, taskID string) (string, []string, []string, []string) {
	// Normalize: strip leading "T-" if the session state already includes it.
	id := strings.TrimPrefix(taskID, "T-")
	relPath := filepath.Join("docs/tasks", "T-"+id+".md")
	data, err := h.readFile(root, relPath)
	if err != nil {
		return taskID, nil, nil, nil
	}
	body := string(data)
	return taskID,
		h.parseFileList(body, "允许修改文件"),
		h.parseFileList(body, "禁止修改文件"),
		h.parseFileList(body, "允许新增文件目录")
}

// extractTaskIDJSON looks for "task_id" in a SESSION-STATE.md body.
func (h *CheckScopeHook) extractTaskIDJSON(body string) string {
	var in struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		return ""
	}
	return strings.TrimSpace(in.TaskID)
}

// extractLatestTaskID scans TASKS.md for the last non-empty task row and
// returns its ID (the "T-XXX" token in the first column).
func (h *CheckScopeHook) extractLatestTaskID(body string) string {
	var last string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "| T-") {
			continue
		}
		for _, field := range strings.Split(line, "|") {
			field = strings.TrimSpace(field)
			if strings.HasPrefix(field, "T-") {
				last = field
			}
		}
	}
	return last
}

// parseFileList extracts lines that follow a given Chinese label (keyed on the
// colon separator). Returns the comma-separated values after the label, cleaned.
func (h *CheckScopeHook) parseFileList(body, label string) []string {
	var result []string
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, label) {
			continue
		}
		// Find the separator: try ASCII colon first, then CJK colon.
		idx := strings.Index(line, ":")
		skip := 1
		if idx < 0 {
			idx = strings.Index(line, "：")
			skip = len("：") // CJK colon is 3 bytes
		}
		if idx < 0 {
			continue
		}
		// Slice past the separator using its byte length.
		val := strings.TrimSpace(line[idx+skip:])
		if val == "" {
			continue
		}
		for _, s := range strings.Split(val, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				result = append(result, filepath.ToSlash(s))
			}
		}
	}
	return result
}

// isRootMarkdown reports whether p is a markdown file at the repo root.
func (h *CheckScopeHook) isRootMarkdown(p string) bool {
	return strings.HasSuffix(p, ".md") && !strings.Contains(p, "/")
}

// fileExists reports whether p exists on disk.
func (h *CheckScopeHook) fileExists(p string) bool {
	if info, err := os.Stat(p); err == nil {
		return !info.IsDir()
	}
	return false
}

// readFile reads a file relative to root and returns its contents.
func (h *CheckScopeHook) readFile(root, rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, rel))
}
