package run

// Wiring tests for the built-in safety gate. safetygate's own tests pin the
// rules; these pin the two properties that only hold at the assembly point:
// that NewConfig installs the gate at all, and that it stays the base of the
// BeforeToolCall chain once the configurable hook layer chains onto it.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/Ai-Kota/aikota/internal/agenttool"
	"github.com/Ai-Kota/aikota/internal/provider"
)

// newGateTestConfig builds a RunConfig through the same constructor the CLI and
// the embedded SDK both use. The provider is nil on purpose: nothing here
// streams, and constructing the StreamFn closure does not dereference it.
func newGateTestConfig(t *testing.T) agentcore.BeforeToolCallFunc {
	t.Helper()
	cfg := NewConfig(
		"test-model", "test-provider", agentcore.ThinkingMedium,
		nil, provider.NewCredentialStore(nil), agenttool.NewToolRegistry(), nil, nil, nil,
	)
	if cfg.Batch.BeforeToolCall == nil {
		t.Fatal("NewConfig did not install the built-in safety gate")
	}
	return cfg.Batch.BeforeToolCall
}

func bashArgs(cmd string) agentcore.AgentToolCall {
	b, _ := json.Marshal(map[string]string{"command": cmd})
	return agentcore.AgentToolCall{ID: "1", Name: "bash", Arguments: b}
}

// TestNewConfigInstallsSafetyGate: the floor must be present in any run the
// product assembles — including the embedded SDK, which never calls the CLI's
// hook installer.
func TestNewConfigInstallsSafetyGate(t *testing.T) {
	gate := newGateTestConfig(t)
	ctx := context.Background()

	if dec := gate(ctx, bashArgs("rm -rf /")); dec == nil || !dec.Block {
		t.Fatalf("gate(rm -rf /) = %+v, want a blocking decision", dec)
	}
	if dec := gate(ctx, bashArgs("go build ./...")); dec != nil {
		t.Errorf("gate(go build ./...) = %+v, want nil (allowed)", dec)
	}
}

// TestSafetyGateStaysBaseOfTheChain pins the ordering. InstallSeams chains the
// configurable hook layer on with chainBeforeToolCall(prev, next), which returns
// prev's decision the moment it blocks — so the gate must be prev. If a future
// change installs the gate *after* the hooks, a permissive hook (or --approve)
// could hand a dangerous call straight through, which is the whole thing this
// gate exists to prevent.
func TestSafetyGateStaysBaseOfTheChain(t *testing.T) {
	gate := newGateTestConfig(t)

	// A maximally permissive hook: runs last, blocks nothing.
	allowEverything := func(context.Context, agentcore.AgentToolCall) *agentcore.BeforeToolCallDecision {
		return nil
	}
	chained := chainBeforeToolCall(gate, allowEverything)

	if dec := chained(context.Background(), bashArgs("rm -rf /")); dec == nil || !dec.Block {
		t.Fatalf("after hook chaining, gate(rm -rf /) = %+v, want still blocked", dec)
	}
}
