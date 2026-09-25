package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/Ai-Kota/aikota/internal/verify"
)

// VerificationState tracks whether the agent has verified its recent changes.
//
// It is shared across the run: write/edit increment "pending", verify clears
// the pending flag. PreCommitGate reads it to block dangerous commands when
// there are unverified changes.
//
// Zero value is usable (no-op); callers can share a single instance across
// tools without allocating a pointer first.
type VerificationState struct {
	mu      sync.Mutex
	pending bool // true when there are unverified changes
}

// NewVerificationState returns a ready-to-use state. Callers may also use the
// zero value directly; the struct is intentionally zero-value-safe.
func NewVerificationState() *VerificationState {
	return &VerificationState{}
}

// MarkChanged records that the agent has performed a mutating operation
// (write/edit) since the last verify. Multiple changes are coalesced into a
// single pending flag — we only care whether the latest change has been
// verified, not how many changes there are.
func (s *VerificationState) MarkChanged() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = true
}

// MarkVerified clears the pending flag after the agent runs the project's
// checks and reports success. If there was nothing pending the call is a
// no-op.
func (s *VerificationState) MarkVerified() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = false
}

// Pending reports whether there are unverified changes on the table. A nil
// receiver is treated as "no pending changes" so callers can gate without
// checking for nil first.
func (s *VerificationState) Pending() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending
}

// HasChecks reports whether the project at root declares any discoverable
// verification commands. This lets the gate suppress warnings on projects
// that have no checks at all — reminding a project with nothing to verify
// would just confuse the model.
func HasChecks(root string) bool {
	cmds, err := verify.Discover(root)
	return err == nil && len(cmds) > 0
}

// PreCommitGate builds a BeforeToolCallFunc that blocks git commit/push when
// the agent has made changes but hasn't verified them yet. The gate fires
// only when the project has discoverable checks; otherwise it stays silent
// (returning nil means "allow the call through unchanged").
//
// The gate does NOT block the write/edit tools themselves — it only blocks
// the follow-up actions that would ship unverified changes (commit, push).
// The reminder layer (VerifyReminderProvider) still nudges the model to run
// verify after editing; this gate is the structural backup that catches
// attempts to bypass that nudge.
func PreCommitGate(state *VerificationState, root string) agentcore.BeforeToolCallFunc {
	if state == nil || !HasChecks(root) {
		return nil
	}
	return func(_ context.Context, call agentcore.AgentToolCall) *agentcore.BeforeToolCallDecision {
		if call.Name != "bash" {
			return nil
		}
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.Command == "" {
			return nil
		}
		cmd := strings.TrimSpace(args.Command)
		// Match git commit/push commands. Keep the patterns narrow: a command
		// like "git diff --stat" must not trigger the gate.
		switch {
		case strings.HasPrefix(cmd, "git commit"),
			strings.HasPrefix(cmd, "git push"),
			strings.HasPrefix(cmd, "git push -u"):
			// No-op on these; the gate is interested in the "ship" commands.
		default:
			return nil
		}
		if !state.Pending() {
			return nil
		}
		msg := "blocked: you have unverified changes.\n\n" +
			"Run the verify tool before committing or pushing:\n" +
			"  verify\n\n" +
			"Verify confirms your changes compile and tests pass. " +
			"Shipping unverified code risks breaking the build."
		content := agentcore.ContentList{agentcore.NewTextContent(msg)}
		return &agentcore.BeforeToolCallDecision{Block: true, Content: &content}
	}
}
