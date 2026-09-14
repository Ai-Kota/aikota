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

func runVerify(t *testing.T, root string) agentcore.AgentToolResult {
	t.Helper()
	tool := &VerifyTool{Root: root}
	res, err := tool.Execute(context.Background(), "1", json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return res
}

// TestVerifyToolReportsPassAndFailure: the verdict must lead the text, because
// that is what decides the model's next action.
func TestVerifyToolReportsPassAndFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".aikota"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".aikota", "verify.json"),
		[]byte(`{"commands":[{"name":"ok","bin":"go","args":["version"]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	body := agentcore.ContentToText(runVerify(t, root).Content)
	if !strings.HasPrefix(body, "verify: PASSED") {
		t.Errorf("pass should lead the output, got: %s", body)
	}

	if err := os.WriteFile(filepath.Join(root, ".aikota", "verify.json"),
		[]byte(`{"commands":[{"name":"bad","bin":"go","args":["definitely-not-a-subcommand"]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	body = agentcore.ContentToText(runVerify(t, root).Content)
	if !strings.HasPrefix(body, "verify: FAILED") {
		t.Errorf("failure should lead the output, got: %s", body)
	}
	if !strings.Contains(body, "First failure:") {
		t.Errorf("failure should name the failing command, got: %s", body)
	}
}

// TestVerifyToolExplainsWhenItCannotRun: a project with no checks must say so
// rather than return nothing, which would read as success.
func TestVerifyToolExplainsWhenItCannotRun(t *testing.T) {
	body := agentcore.ContentToText(runVerify(t, t.TempDir()).Content)
	if !strings.Contains(body, "could not run") {
		t.Errorf("a project with no checks should say so, got: %s", body)
	}
}

// TestVerifyToolRunsProjectChecksEndToEnd: the interesting case is a real Go
// module, where discovery has to find the conventions without any override.
func TestVerifyToolRunsProjectChecksEndToEnd(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module probe\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "probe.go"), []byte("package probe\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := runVerify(t, root)
	body := agentcore.ContentToText(res.Content)
	if !strings.HasPrefix(body, "verify: PASSED") {
		t.Errorf("a clean module should pass, got: %s", body)
	}
	if ran, _ := res.Details.(map[string]any)["ranCount"].(int); ran != 2 {
		t.Errorf("ranCount = %v, want 2 (build then test)", res.Details)
	}

	// Now break it: the same tool must report the failure.
	if err := os.WriteFile(filepath.Join(root, "probe.go"), []byte("package probe\n\nfunc Add( {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body = agentcore.ContentToText(runVerify(t, root).Content)
	if !strings.HasPrefix(body, "verify: FAILED") {
		t.Errorf("a broken module must fail, got: %s", body)
	}
}

func TestVerifyToolDeclaration(t *testing.T) {
	tool := &VerifyTool{}
	if tool.Name() != "verify" {
		t.Errorf("Name() = %q", tool.Name())
	}
	desc := tool.Description()
	for _, want := range []string{"When to use", "When NOT to use", ".aikota/verify.json"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description missing %q", want)
		}
	}
	// Checks run the build and tests, which contend for outputs.
	if tool.ExecutionMode() != agentcore.ToolExecutionSequential {
		t.Error("verify should run sequentially")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
}
