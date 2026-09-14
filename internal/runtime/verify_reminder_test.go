package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

func assistantWithCalls(names ...string) agentcore.AssistantMessage {
	content := make(agentcore.ContentList, 0, len(names))
	for i, n := range names {
		content = append(content, agentcore.NewToolCallContent(
			"call_"+n, n, json.RawMessage(`{}`)))
		_ = i
	}
	return agentcore.AssistantMessage{RoleField: agentcore.RoleAssistant, Content: content}
}

func userMsg(s string) agentcore.UserMessage {
	return agentcore.UserMessage{
		RoleField: agentcore.RoleUser,
		Content:   agentcore.ContentList{agentcore.NewTextContent(s)},
	}
}

func providerForGoProject(t *testing.T) *VerifyReminderProvider {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewVerifyReminderProvider(root)
	if p == nil {
		t.Fatal("a Go project must yield a provider")
	}
	return p
}

// TestVerifyReminderAfterUnverifiedEdit is the property that makes this a loop
// rather than a tool the model has to remember: an edit with no verification
// behind it produces a prompt.
func TestVerifyReminderAfterUnverifiedEdit(t *testing.T) {
	p := providerForGoProject(t)
	msgs := agentcore.MessageList{
		userMsg("change the config"),
		assistantWithCalls("read", "edit"),
	}
	body, ok := p.Reminder(context.Background(), msgs)
	if !ok {
		t.Fatal("an unverified edit should raise a reminder")
	}
	if !strings.Contains(body, "verify tool") {
		t.Errorf("reminder should name the tool to call, got: %s", body)
	}
}

// TestVerifyReminderSilentAfterVerification: once the model has checked its
// work, the nudge must stop — otherwise it would fire forever.
func TestVerifyReminderSilentAfterVerification(t *testing.T) {
	p := providerForGoProject(t)
	msgs := agentcore.MessageList{
		userMsg("change the config"),
		assistantWithCalls("edit"),
		assistantWithCalls("verify"),
	}
	if _, ok := p.Reminder(context.Background(), msgs); ok {
		t.Error("a verified edit should not raise a reminder")
	}
}

// TestVerifyReminderOrderMatters: verifying and *then* editing is unverified
// work, even though a verify call exists in the turn.
func TestVerifyReminderOrderMatters(t *testing.T) {
	p := providerForGoProject(t)
	msgs := agentcore.MessageList{
		userMsg("change the config"),
		assistantWithCalls("verify"),
		assistantWithCalls("edit"),
	}
	if _, ok := p.Reminder(context.Background(), msgs); !ok {
		t.Error("an edit after the last verify is still unverified")
	}
}

// TestVerifyReminderSilentOnReadOnlyTurns: reading is not a change, and
// reminding about it would train the model to ignore the reminder.
func TestVerifyReminderSilentOnReadOnlyTurns(t *testing.T) {
	p := providerForGoProject(t)
	msgs := agentcore.MessageList{
		userMsg("what does this do"),
		assistantWithCalls("read", "grep", "repo_map"),
	}
	if _, ok := p.Reminder(context.Background(), msgs); ok {
		t.Error("a read-only turn should not raise a reminder")
	}
}

// TestVerifyReminderOnlyWithinTheCurrentTurn: work verified in an earlier turn
// must not be re-reminded about.
func TestVerifyReminderOnlyWithinTheCurrentTurn(t *testing.T) {
	p := providerForGoProject(t)
	msgs := agentcore.MessageList{
		userMsg("first task"),
		assistantWithCalls("edit"),
		assistantWithCalls("verify"),
		userMsg("now explain it"),
		assistantWithCalls("read"),
	}
	if _, ok := p.Reminder(context.Background(), msgs); ok {
		t.Error("a new read-only turn should be silent")
	}
}

// TestVerifyReminderNilWithoutChecks: a project that declares no checks cannot
// be verified, so nudging would ask for the impossible.
func TestVerifyReminderNilWithoutChecks(t *testing.T) {
	if p := NewVerifyReminderProvider(t.TempDir()); p != nil {
		t.Error("a project with no checks should yield no provider")
	}
	// The nil provider must also be safe to call, since callers register the
	// constructor's result unconditionally.
	var p *VerifyReminderProvider
	if _, ok := p.Reminder(context.Background(), nil); ok {
		t.Error("a nil provider must stay silent")
	}
	if p.Name() == "" {
		t.Error("a nil provider must still report a name")
	}
}
