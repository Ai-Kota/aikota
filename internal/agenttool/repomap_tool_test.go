package agenttool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

func seedRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("core/engine.go", "package core\n\ntype Engine struct{}\n\nfunc New() *Engine { return &Engine{} }\n")
	write("cmd/main.go", "package main\n\nimport \"core\"\n\nfunc main() { _ = core.New() }\n")
	return root
}

func runRepoMap(t *testing.T, root, args string) agentcore.AgentToolResult {
	t.Helper()
	tool := &RepoMapTool{Root: root}
	res, err := tool.Execute(context.Background(), "1", json.RawMessage(args), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return res
}

// TestRepoMapToolRendersRankedMap: the tool's whole value is the rendered map,
// so assert on its content rather than only that it returned.
func TestRepoMapToolRendersRankedMap(t *testing.T) {
	root := seedRepo(t)
	res := runRepoMap(t, root, `{}`)

	if len(res.Content) == 0 {
		t.Fatal("empty content")
	}
	body := agentcore.ContentToText(res.Content)
	if !strings.Contains(body, "core/engine.go") {
		t.Errorf("map missing the depended-upon file:\n%s", body)
	}
	if !strings.Contains(body, "type Engine struct") {
		t.Errorf("map missing a signature:\n%s", body)
	}
	// The body must never be rendered — only the signature.
	if strings.Contains(body, "return &Engine{}") {
		t.Errorf("map leaked a function body:\n%s", body)
	}
	// A reader needs to know the map is partial.
	if !strings.Contains(body, "of") || !strings.Contains(body, "files shown") {
		t.Errorf("map should state how many files it covers:\n%s", body)
	}
}

// TestRepoMapToolEmptyRepoIsNotAnError: an empty or all-skipped tree is a
// legitimate answer. Returning a Go error would surface as a tool failure the
// model would retry pointlessly.
func TestRepoMapToolEmptyRepoIsNotAnError(t *testing.T) {
	root := t.TempDir()
	res := runRepoMap(t, root, `{}`)
	body := agentcore.ContentToText(res.Content)
	if !strings.Contains(body, "no Go source files") {
		t.Errorf("empty repo should say so plainly, got: %s", body)
	}
}

// TestRepoMapToolRejectsBadArgs: schema violations become error results, not
// Go errors, so the model sees a fixable message.
func TestRepoMapToolRejectsBadArgs(t *testing.T) {
	root := seedRepo(t)
	res := runRepoMap(t, root, `{"max_tokens": "not a number"}`)
	body := agentcore.ContentToText(res.Content)
	if !strings.Contains(body, "repo_map") {
		t.Errorf("bad args should produce an error result naming the tool, got: %s", body)
	}
}

// TestRepoMapToolFocusLiftsFile: focus_files must actually change the ranking,
// otherwise the parameter is a lie in the description.
func TestRepoMapToolFocusLiftsFile(t *testing.T) {
	root := seedRepo(t)
	// A budget small enough that only one file fits, so focus decides the pick.
	res := runRepoMap(t, root, `{"max_tokens": 256, "focus_files": ["cmd/"]}`)
	body := agentcore.ContentToText(res.Content)
	if !strings.Contains(body, "cmd/main.go") {
		t.Errorf("focus_files on cmd/ should surface cmd/main.go first:\n%s", body)
	}
}

// TestRepoMapToolDeclaration pins the surface the model sees: a name the
// allowlist can reference, a description that says when NOT to use it, and a
// read-only execution mode so it can batch with other readers.
func TestRepoMapToolDeclaration(t *testing.T) {
	tool := &RepoMapTool{}
	if tool.Name() != "repo_map" {
		t.Errorf("Name() = %q", tool.Name())
	}
	desc := tool.Description()
	for _, want := range []string{"When to use", "When NOT to use", "max_tokens", "focus_symbols"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description missing %q", want)
		}
	}
	if tool.ExecutionMode() != agentcore.ToolExecutionParallel {
		t.Error("repo_map only reads; it should be parallel-safe")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
}
