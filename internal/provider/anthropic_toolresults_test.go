package provider

import (
	"encoding/json"
	"testing"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

func toolResult(id, text string) agentcore.ToolResultMessage {
	return agentcore.ToolResultMessage{
		RoleField:  agentcore.RoleToolResult,
		ToolCallID: id,
		ToolName:   "bash",
		Content:    agentcore.ContentList{agentcore.NewTextContent(text)},
	}
}

func assistantWithCalls(ids ...string) agentcore.AssistantMessage {
	content := make(agentcore.ContentList, 0, len(ids))
	for _, id := range ids {
		content = append(content, agentcore.NewToolCallContent(id, "t_"+id, json.RawMessage(`{}`)))
	}
	return agentcore.AssistantMessage{RoleField: agentcore.RoleAssistant, Content: content}
}

func userText(s string) agentcore.UserMessage {
	return agentcore.UserMessage{RoleField: agentcore.RoleUser, Content: agentcore.ContentList{agentcore.NewTextContent(s)}}
}

// anthropicMessages encodes req and returns the parsed messages array plus the
// tool_use ids and tool_result ids it contains, in order.
func anthropicMessages(t *testing.T, req CompletionRequest) (count int, role []string, toolUses, toolResults []string) {
	t.Helper()
	body, err := encodeAnthropicRequest(req, nil)
	if err != nil {
		t.Fatalf("encodeAnthropicRequest: %v", err)
	}
	var parsed struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	for _, m := range parsed.Messages {
		role = append(role, m.Role)
		var blocks []map[string]any
		if err := json.Unmarshal(m.Content, &blocks); err != nil {
			continue // a plain string content (a text-only user turn)
		}
		for _, b := range blocks {
			switch b["type"] {
			case "tool_use":
				toolUses = append(toolUses, b["id"].(string))
			case "tool_result":
				toolResults = append(toolResults, b["tool_use_id"].(string))
			}
		}
	}
	return len(parsed.Messages), role, toolUses, toolResults
}

// TestEncodeAnthropicRequestMergesConsecutiveToolResults is the regression that
// matters: a turn calling several tools must answer all of them in ONE message.
// Emitting one message per result makes the API reject the whole request with
// "tool_use ids were found without tool_result blocks immediately after", which
// means the run dies on exactly the turns that did the most work.
func TestEncodeAnthropicRequestMergesConsecutiveToolResults(t *testing.T) {
	req := CompletionRequest{
		Model: "m",
		Context: LlmContext{
			Messages: agentcore.MessageList{
				userText("map this repo then find the config"),
				assistantWithCalls("c1", "c2", "c3"),
				toolResult("c1", "map"),
				toolResult("c2", "hits"),
				toolResult("c3", "more"),
			},
		},
	}

	n, role, uses, results := anthropicMessages(t, req)
	if n != 3 {
		t.Fatalf("encoded %d messages, want 3 (user, assistant, ONE merged result turn); roles=%v", n, role)
	}
	if role[1] != "assistant" || role[2] != "user" {
		t.Errorf("roles = %v, want [user assistant user]", role)
	}
	if len(uses) != 3 {
		t.Errorf("tool_use blocks = %v, want the three the assistant emitted", uses)
	}
	if len(results) != 3 {
		t.Fatalf("tool_result blocks = %v, want all three — a missing one is the 400 this guards against", results)
	}
	for i, id := range uses {
		if results[i] != id {
			t.Errorf("tool_result[%d] = %q, want %q (order must follow the tool_use order)", i, results[i], id)
		}
	}
}

// TestEncodeAnthropicRequestSingleToolResultUnchanged: the common single-call
// turn must keep the shape it always had.
func TestEncodeAnthropicRequestSingleToolResultUnchanged(t *testing.T) {
	req := CompletionRequest{
		Model: "m",
		Context: LlmContext{Messages: agentcore.MessageList{
			userText("go"),
			assistantWithCalls("c1"),
			toolResult("c1", "out"),
		}},
	}
	n, _, uses, results := anthropicMessages(t, req)
	if n != 3 || len(uses) != 1 || len(results) != 1 {
		t.Errorf("got %d messages, %d tool_use, %d tool_result; want 3/1/1", n, len(uses), len(results))
	}
}

// TestEncodeAnthropicRequestDoesNotMergeIntoTextTurn: a tool_result may only
// join a message that is already nothing but tool_results. Attaching it to an
// ordinary user turn would put the answer where the API does not look for it —
// the same 400, reached the other way.
func TestEncodeAnthropicRequestDoesNotMergeIntoTextTurn(t *testing.T) {
	req := CompletionRequest{
		Model: "m",
		Context: LlmContext{Messages: agentcore.MessageList{
			userText("first"),
			userText("second"),
			assistantWithCalls("c1"),
			toolResult("c1", "out"),
		}},
	}
	n, role, _, results := anthropicMessages(t, req)
	if n != 4 {
		t.Fatalf("encoded %d messages, want 4 (the two user turns stay separate); roles=%v", n, role)
	}
	if len(results) != 1 {
		t.Errorf("tool_result count = %d, want 1", len(results))
	}
}

// TestEncodeAnthropicRequestSeparatesNonAdjacentResults: results only merge when
// they are adjacent in the context. A message in between ends the batch, which
// is what keeps the assistant turn's shape honest.
func TestEncodeAnthropicRequestSeparatesNonAdjacentResults(t *testing.T) {
	req := CompletionRequest{
		Model: "m",
		Context: LlmContext{Messages: agentcore.MessageList{
			userText("go"),
			assistantWithCalls("c1"),
			toolResult("c1", "first"),
			toolResult("c2", "second"),
		}},
	}
	n, _, _, results := anthropicMessages(t, req)
	if n != 3 {
		t.Fatalf("encoded %d messages, want 3", n)
	}
	if len(results) != 2 {
		t.Errorf("both results should land in the same merged message; got %v", results)
	}
}
