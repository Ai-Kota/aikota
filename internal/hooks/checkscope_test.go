package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

// mkTestHook builds a CheckScopeHook rooted at tmp, pre-populating docs/tasks
// with the supplied task definition when defs is non-empty.
func mkTestHook(t *testing.T, tmp string, defs map[string]string) *CheckScopeHook {
	t.Helper()
	h := &CheckScopeHook{Root: tmp}
	if defs == nil {
		return h
	}
	for name, body := range defs {
		p := filepath.Join(tmp, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

// assertBlocked verifies that Check returns a non-nil Blocked with the given
// rule. A nil Blocked means the call was allowed, which is a failure.
func assertBlocked(t *testing.T, got *Blocked, wantRule string) {
	t.Helper()
	if got == nil {
		t.Fatalf("expected block, got nil")
	}
	if wantRule != "" && got.Rule != wantRule {
		t.Fatalf("expected rule %q, got %q", wantRule, got.Rule)
	}
}

// assertAllowed verifies that Check returns nil (call allowed).
func assertAllowed(t *testing.T, got *Blocked) {
	t.Helper()
	if got != nil {
		t.Fatalf("expected allow, got block: %s", got.Reason)
	}
}

func TestCheckScopeHook_InterceptsToolCall(t *testing.T) {
	tmp := t.TempDir()
	defs := map[string]string{
		"docs/tasks/TASKS.md": "# 任务清单\n| T-1 | Fix auth bug | active |\n",
		"docs/tasks/T-1.md":   "允许修改文件：foo.go, bar.go\n禁止修改文件：secret.go\n",
		"docs/tasks/SESSION-STATE.md": `{"task_id":"T-1"}`,
	}
	h := mkTestHook(t, tmp, defs)

	// Allowed path passes.
	call := agentcore.AgentToolCall{Name: "write", Arguments: []byte(`{"path":"foo.go"}`)}
	assertAllowed(t, h.Check(call))

	// Denied path blocks.
	call = agentcore.AgentToolCall{Name: "write", Arguments: []byte(`{"path":"secret.go"}`)}
	assertBlocked(t, h.Check(call), "denied-path")

	// Path not in allowed list blocks.
	call = agentcore.AgentToolCall{Name: "edit", Arguments: []byte(`{"path":"other.go"}`)}
	assertBlocked(t, h.Check(call), "allowed-path")
}

func TestCheckScopeHook_AllowsSafeOperations(t *testing.T) {
	tmp := t.TempDir()
	defs := map[string]string{
		"docs/tasks/TASKS.md": "# 任务清单\n| T-1 | Add feature | active |\n",
		"docs/tasks/T-1.md":   "允许修改文件：main.go\n",
		"docs/tasks/SESSION-STATE.md": `{"task_id":"T-1"}`,
	}
	h := mkTestHook(t, tmp, defs)

	// Bash calls are not scoped — always allowed.
	call := agentcore.AgentToolCall{Name: "bash", Arguments: []byte(`{"command":"ls -la"}`)}
	assertAllowed(t, h.Check(call))

	// Allowed write path passes.
	call = agentcore.AgentToolCall{Name: "write", Arguments: []byte(`{"path":"main.go"}`)}
	assertAllowed(t, h.Check(call))

	// Root markdown is exempt even without task.
	h2 := &CheckScopeHook{Root: tmp}
	call = agentcore.AgentToolCall{Name: "write", Arguments: []byte(`{"path":"README.md"}`)}
	assertAllowed(t, h2.Check(call))
}

func TestCheckScopeHook_SPTIntegration(t *testing.T) {
	tmp := t.TempDir()
	defs := map[string]string{
		"docs/tasks/TASKS.md": "# 任务清单\n| T-42 | Refactor login | active |\n",
		"docs/tasks/T-42.md": "允许修改文件：pkg/auth/login.go,pkg/auth/session.go\n" +
			"禁止修改文件：pkg/billing/*,internal/cache/*\n" +
			"允许新增文件目录：pkg/auth/test/\n",
		"docs/tasks/SESSION-STATE.md": `{"task_id":"T-42"}`,
	}
	h := mkTestHook(t, tmp, defs)

	// Existing allowed files pass.
	call := agentcore.AgentToolCall{Name: "write", Arguments: []byte(`{"path":"pkg/auth/login.go"}`)}
	assertAllowed(t, h.Check(call))
	call = agentcore.AgentToolCall{Name: "edit", Arguments: []byte(`{"path":"pkg/auth/session.go"}`)}
	assertAllowed(t, h.Check(call))

	// New file in allowed dir passes.
	call = agentcore.AgentToolCall{Name: "write", Arguments: []byte(`{"path":"pkg/auth/test/helper.go"}`)}
	assertAllowed(t, h.Check(call))

	// Denied patterns block — prefix match for pkg/billing/.
	call = agentcore.AgentToolCall{Name: "write", Arguments: []byte(`{"path":"pkg/billing/invoice.go"}`)}
	assertBlocked(t, h.Check(call), "denied-path")
	call = agentcore.AgentToolCall{Name: "edit", Arguments: []byte(`{"path":"internal/cache/lru.go"}`)}
	assertBlocked(t, h.Check(call), "denied-path")

	// File outside allowed list blocks.
	call = agentcore.AgentToolCall{Name: "write", Arguments: []byte(`{"path":"pkg/web/handler.go"}`)}
	assertBlocked(t, h.Check(call), "allowed-path")
}
