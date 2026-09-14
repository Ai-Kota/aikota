// Package safetygate is aikota's built-in safety floor: a small, high-confidence
// set of rules that refuse tool calls which are essentially never legitimate
// for a coding agent — recursive deletion of a filesystem root, raw disk writes,
// privilege escalation, and modification of credential or shell-init files.
//
// It is deliberately NOT part of the user-configurable hook layer
// (internal/hooks). This gate is a product-level floor that:
//
//   - applies in every mode, including the embedded SDK (agent/), which does
//     not wire hooks at all;
//   - runs as the base of the BeforeToolCall chain, so no policy layer and no
//     --approve can widen it — the chain combinator stops at the first block;
//   - costs no process spawn, because the rules are static string checks.
//
// A blocked call is reported back to the model as that tool's error result, so
// it sees the reason and can choose another approach instead of failing
// silently.
//
// Scope is intentionally narrow. A gate that blocks legitimate work is worse
// than no gate: every rule here must be something a coding agent should never
// do, not merely something suspicious.
package safetygate

import (
	"context"
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

// Blocked describes a refused tool call. Rule names the rule that fired and is
// used in tests and diagnostics; Reason is the text handed to the model.
type Blocked struct {
	Rule   string
	Reason string
}

// Gate adapts Check to the tool-executor's BeforeToolCall seam.
func Gate() agentcore.BeforeToolCallFunc {
	return func(_ context.Context, call agentcore.AgentToolCall) *agentcore.BeforeToolCallDecision {
		b := Check(call)
		if b == nil {
			return nil
		}
		content := agentcore.ContentList{agentcore.NewTextContent(b.Reason)}
		return &agentcore.BeforeToolCallDecision{Block: true, Content: &content}
	}
}

// Check evaluates the built-in rules against call, returning nil when the call
// is allowed.
//
// Malformed arguments are allowed through: schema validation has already run
// upstream, and this gate refuses on recognised dangerous content rather than
// on parse failure. Failing closed here would turn a malformed tool call into a
// confusing safety block.
func Check(call agentcore.AgentToolCall) *Blocked {
	switch call.Name {
	case "bash":
		return checkCommand(call.Arguments)
	case "write", "edit":
		return checkWritePath(call.Arguments)
	default:
		return nil
	}
}

// checkCommand applies the command rules to a bash call's "command" field.
func checkCommand(args json.RawMessage) *Blocked {
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &in); err != nil || in.Command == "" {
		return nil
	}
	cmd := in.Command
	// Backslash-escaped newlines would otherwise let a dangerous second line
	// hide from an anchored pattern; collapsing whitespace keeps the checks on
	// the whole command line.
	flat := strings.Join(strings.Fields(cmd), " ")
	for _, r := range commandRules {
		if r.match.MatchString(flat) {
			return &Blocked{Rule: r.name, Reason: r.reason}
		}
	}
	return nil
}

type commandRule struct {
	name   string
	match  *regexp.Regexp
	reason string
}

const commandBlockPrefix = "aikota safety: "

// cmdPos matches the start of a command: the beginning of the line, or just
// after a shell separator. Requiring a command position is what keeps these
// rules from firing on a dangerous-looking *string used as data* —
// `grep sudo file` and `echo "rm -rf /"` are both legitimate.
//
// The trade-off, taken deliberately: a quoted invocation such as
// `bash -c "rm -rf /"` is not caught. This gate is a floor against an
// over-eager agent, not a sandbox against a hostile one; a sandbox is what the
// container/sandboxing path is for.
const cmdPos = `(?:^|[;&|]\s*)`

// commandRules are matched against the whitespace-collapsed command line.
var commandRules = []commandRule{
	{
		name: "rm-root",
		// Recursive AND force, however the flags are spelled: combined (-rf,
		// -fr, -Rf) or split into separate tokens (-r -f, -f -r). Case is
		// ignored so -R counts as recursive.
		match:  regexp.MustCompile(`(?i)` + cmdPos + `rm\s+(?:-[a-z]*(?:rf|fr)[a-z]*|-[a-z]*r[a-z]*\s+-\S*f\S*|-[a-z]*f[a-z]*\s+-\S*r\S*)\s+(?:-\S+\s+)*(?:/|/\*|~|~/\*|\$HOME/?|\$\{HOME\}/?|/home|/Users)(?:\s|$|[;&|])`),
		reason: commandBlockPrefix + "refusing a recursive force-delete aimed at a filesystem root or home directory. Delete a specific subdirectory instead.",
	},
	{
		name:   "mkfs",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `mkfs(\.[a-z0-9]+)?(\s|$)`),
		reason: commandBlockPrefix + "refusing to format a filesystem (mkfs). This destroys data irreversibly.",
	},
	{
		name:   "dd-to-device",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `dd\s+[^;&|]*\bof=/dev/`),
		reason: commandBlockPrefix + "refusing a raw dd write to a device node. This destroys data irreversibly.",
	},
	{
		name:   "redirect-to-device",
		match:  regexp.MustCompile(`(?i)>\s*/dev/(sd|hd|nvme|vd|disk|mmcblk)`),
		reason: commandBlockPrefix + "refusing a redirect into a raw device node.",
	},
	{
		name:   "fork-bomb",
		match:  regexp.MustCompile(`:\s*\(\s*\)\s*\{.*\|.*&.*\}\s*;\s*:`),
		reason: commandBlockPrefix + "refusing a fork bomb.",
	},
	{
		name:   "shutdown",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `(shutdown|reboot|halt|poweroff)(\s|$)`),
		reason: commandBlockPrefix + "refusing to shut down or reboot the host.",
	},
	{
		name:   "privilege-escalation",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `sudo\s`),
		reason: commandBlockPrefix + "refusing to escalate privileges with sudo. Run the command without elevation, or ask the user to run it.",
	},
	{
		name:   "chmod-777",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `chmod\s+(-[a-zA-Z]+\s+)*777(\s|$)`),
		reason: commandBlockPrefix + "refusing chmod 777 (world-writable). Grant the narrowest permission that works.",
	},
	{
		name:   "pipe-to-shell",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `(curl|wget)\s[^;&|]*\|\s*(sudo\s+)?(sh|bash|zsh|dash|python3?|perl|ruby)(\s|$)`),
		reason: commandBlockPrefix + "refusing to pipe a downloaded script straight into a shell. Download it, inspect it, then run it.",
	},
	{
		name:   "global-package-install",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `(apt|apt-get|brew|choco|winget)\s+(install|add)\s`),
		reason: commandBlockPrefix + "refusing a system-wide package install. Use a project-local dependency, or ask the user to install it.",
	},
	{
		name:   "windows-format",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `format\s+[a-z]:`),
		reason: commandBlockPrefix + "refusing to format a volume.",
	},
	{
		name:   "windows-mass-delete",
		match:  regexp.MustCompile(`(?i)` + cmdPos + `(del|erase|rd|rmdir)\s+(/[a-z]+\s+)*[a-z]:\\`),
		reason: commandBlockPrefix + "refusing a recursive delete rooted at a drive root.",
	},
}

// checkWritePath applies the protected-path rules to a write/edit call's "path".
//
// The file tools already refuse paths outside the workspace (resolveWithin), so
// the remaining risk is a sensitive file *inside* it: .git/config carries
// credentials, and the agent's own settings files can loosen its constraints.
func checkWritePath(args json.RawMessage) *Blocked {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil || in.Path == "" {
		return nil
	}
	// Normalise to forward slashes and collapse "..", so "./.git/../.git/config"
	// and ".git\\config" reach the same decision as ".git/config".
	clean := path.Clean(strings.ReplaceAll(in.Path, `\`, "/"))
	lower := strings.ToLower(clean)

	for _, r := range protectedPaths {
		if r.match(lower) {
			return &Blocked{Rule: r.name, Reason: r.reason}
		}
	}
	return nil
}

type pathRule struct {
	name   string
	match  func(lowerSlashPath string) bool
	reason string
}

// hasSegment reports whether any path segment equals seg exactly, so ".git" is
// matched as a directory component while "legit.gitignore" is not.
func hasSegment(p, seg string) bool {
	for _, s := range strings.Split(p, "/") {
		if s == seg {
			return true
		}
	}
	return false
}

var protectedPaths = []pathRule{
	{
		name:   "git-config",
		match:  func(p string) bool { return hasSegment(p, ".git") && path.Base(p) == "config" },
		reason: commandBlockPrefix + "refusing to write .git/config, which holds the repository's remotes and credentials. Edit it directly if you really mean to change it.",
	},
	{
		name:   "git-hooks",
		match:  func(p string) bool { return hasSegment(p, ".git") && hasSegment(p, "hooks") },
		reason: commandBlockPrefix + "refusing to write into .git/hooks, where a file would run on the next git command.",
	},
	{
		name:   "agent-settings",
		match: func(p string) bool {
			base := path.Base(p)
			return base == "settings.json" && (hasSegment(p, ".claude") || hasSegment(p, ".aikota"))
		},
		reason: commandBlockPrefix + "refusing to write the agent's own settings file. Changing its own constraints is what this gate exists to prevent; edit it directly if you mean to.",
	},
}
