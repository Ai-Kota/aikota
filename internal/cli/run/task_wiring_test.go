package run

// Tests for the generic task tool wiring (US-004, #454): the nesting guard.
// BuiltinToolsExcept backs the child sub-agent registry, from which "task" is
// removed so a child cannot spawn further sub-agents.

import "testing"

// TestBuiltinToolsExceptExcludesTask verifies the child tool set produced for a
// task sub-agent (builtins minus "task") never contains "task", and that
// excluding a name actually present removes it.
func TestBuiltinToolsExceptExcludesTask(t *testing.T) {
	child := BuiltinToolsExcept("/tmp", ToolOpts{}, "task")
	if len(child) == 0 {
		t.Fatal("expected builtin tools in the child set")
	}
	for _, tl := range child {
		if tl.Name() == "task" {
			t.Fatal("child tool set must not contain 'task'")
		}
	}
	// Excluding a name that IS present shrinks the set by exactly that tool.
	full := BuiltinToolsExcept("/tmp", ToolOpts{})
	dropped := BuiltinToolsExcept("/tmp", ToolOpts{}, full[0].Name())
	if len(dropped) != len(full)-1 {
		t.Errorf("excluding %q: got %d tools, want %d", full[0].Name(), len(dropped), len(full)-1)
	}
}

// TestBuiltinToolsExceptNoExcept verifies that with no except names the result
// matches BuiltinTools, and that a disabled tool set stays empty.
func TestBuiltinToolsExceptNoExcept(t *testing.T) {
	if got, want := len(BuiltinToolsExcept("/tmp", ToolOpts{}, "task")), len(BuiltinTools("/tmp", ToolOpts{})); got != want {
		t.Errorf("no-except size = %d, want %d", got, want)
	}
	if got := BuiltinToolsExcept("/tmp", ToolOpts{Disabled: true}, "task"); got != nil {
		t.Errorf("disabled tools should be nil, got %v", got)
	}
}
