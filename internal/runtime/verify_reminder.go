package runtime

import (
	"context"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/Ai-Kota/aikota/internal/verify"
)

// VerifyReminderProvider nudges the model to run the project's checks after it
// has changed files and before it declares the work done.
//
// This is what makes verification a loop rather than an optional tool: the
// model does not have to remember to check its own work, because the prompt
// arrives unbidden while there is unverified work on the table. It deliberately
// does not run the checks itself — an automatic test run would fire on every
// turn and, worse, the loop would have to decide what to do with the failure.
// Reminding keeps the judgement with the model and the loop simple.
type VerifyReminderProvider struct {
	// hasChecks is resolved once, at construction. Reminding a project with no
	// discoverable checks would be asking for something impossible.
	hasChecks bool
}

// NewVerifyReminderProvider returns a provider for the project at root, or nil
// when the project declares no checks — a nil ReminderProvider is ignored by
// the registry, so callers can register the result unconditionally.
func NewVerifyReminderProvider(root string) *VerifyReminderProvider {
	cmds, err := verify.Discover(root)
	if err != nil || len(cmds) == 0 {
		return nil
	}
	return &VerifyReminderProvider{hasChecks: true}
}

// Name implements ReminderProvider.
func (p *VerifyReminderProvider) Name() string { return "verify" }

// Reminder implements ReminderProvider.
//
// It scans the current turn backwards and lets the most recent relevant action
// decide: a verify call means the work has been checked, a write or edit means
// it has not. Reaching the turn's opening user message means nothing was
// changed this turn.
func (p *VerifyReminderProvider) Reminder(_ context.Context, msgs agentcore.MessageList) (string, bool) {
	if p == nil || !p.hasChecks {
		return "", false
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		switch m := msgs[i].(type) {
		case agentcore.UserMessage:
			return "", false
		case agentcore.AssistantMessage:
			calls := m.ToolCalls()
			// Within one message, the last call is the most recent action.
			for j := len(calls) - 1; j >= 0; j-- {
				switch calls[j].Name {
				case "verify":
					return "", false
				case "write", "edit":
					return "You changed files this turn but have not run the project's checks.\n\n" +
						"Call the verify tool before reporting the work as done, and fix anything it reports. " +
						"Claiming success from reading the diff is not the same as the tests passing.", true
				}
			}
		}
	}
	return "", false
}
