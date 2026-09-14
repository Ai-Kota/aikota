package runtime

import (
	"testing"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

func turnEndMessage(stopReason, errMsg, text string) agentcore.TurnEndEvent {
	return agentcore.TurnEndEvent{Message: agentcore.AssistantMessage{
		RoleField:    agentcore.RoleAssistant,
		StopReason:   stopReason,
		ErrorMessage: errMsg,
		Content:      agentcore.ContentList{agentcore.NewTextContent(text)},
	}}
}

// TestEventEnvelopeCarriesTheFailureReason: a consumer that sees only
// stopReason "error" knows a run failed but not why, which is exactly what it
// needs in order to react. The message already carries the reason, so the
// envelope must not drop it.
func TestEventEnvelopeCarriesTheFailureReason(t *testing.T) {
	env := eventEnvelope(turnEndMessage(agentcore.StopReasonError, "upstream 500: rate limited", ""))

	if env["stopReason"] != agentcore.StopReasonError {
		t.Errorf("stopReason = %v", env["stopReason"])
	}
	got, ok := env["errorMessage"]
	if !ok {
		t.Fatal("errorMessage missing; a failed turn must say why")
	}
	if got != "upstream 500: rate limited" {
		t.Errorf("errorMessage = %v", got)
	}
}

// TestEventEnvelopeOmitsEmptyError: a successful turn must not carry an empty
// errorMessage field, or every consumer has to test for it being blank.
func TestEventEnvelopeOmitsEmptyError(t *testing.T) {
	env := eventEnvelope(turnEndMessage("end_turn", "", "all done"))
	if _, ok := env["errorMessage"]; ok {
		t.Error("a successful turn should not carry errorMessage")
	}
	if env["text"] != "all done" {
		t.Errorf("text = %v", env["text"])
	}
}

// TestEventEnvelopeCarriesAbortReason: an abort is a terminal failure too, and
// the reason is how a caller tells "the user stopped it" from "the model died".
func TestEventEnvelopeCarriesAbortReason(t *testing.T) {
	env := eventEnvelope(turnEndMessage(agentcore.StopReasonAborted, "cancelled by user", ""))
	if env["errorMessage"] != "cancelled by user" {
		t.Errorf("errorMessage = %v", env["errorMessage"])
	}
}
