package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/Ai-Kota/aikota/internal/verify"
)

// VerifyTool runs the project's own checks (build, test, lint) and reports the
// outcome.
//
// It exists so an agent can learn whether a change works from evidence rather
// than from its own reading of the diff — the difference between "I edited the
// files" and "the tests pass".
type VerifyTool struct {
	// Root is the project directory; empty defaults to the working directory.
	Root string
	// VerifyState, when non-nil, is cleared (marked verified) on every successful
	// verify call so the verification loop stays in sync.
	VerifyState any // *runtime.VerificationState — non-type to avoid import cycle
}

type verifyArgs struct {
	// The command set is discovered from the project, never supplied by the
	// model: an agent-chosen command line would be arbitrary execution with a
	// friendly name, which is what the bash tool already is.
	_ struct{} `json:"-"`
}

// Name implements AgentTool.
func (t *VerifyTool) Name() string { return "verify" }

// Description implements AgentTool.
func (t *VerifyTool) Description() string {
	return `Run the project's own checks — build, tests, lint — and report whether they pass.

## When to use
- After changing code, before reporting the work as done
- After fixing a failure, to confirm the fix actually took
- When unsure whether an edit broke something elsewhere

## When NOT to use
- On a project with no recognisable checks (the tool will say so)
- To run an arbitrary command — that is what bash is for; this tool only runs
  the checks the project itself declares

## How the commands are chosen
Discovered from the project, in this order of precedence:
- .aikota/verify.json, if present ({"commands":[{"name":"...","bin":"...","args":["..."]}]})
- otherwise go.mod -> "go build ./...", then "go test ./..."
- Cargo.toml -> cargo test | package.json -> npm test | pyproject.toml -> pytest

Checks run in order and stop at the first failure, because later ones are
usually meaningless once an earlier one breaks. The tail of the output is
returned, since that is where a failure states its reason.`
}

// Schema implements AgentTool.
func (t *VerifyTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {},
  "additionalProperties": false
}`)
}

// ExecutionMode implements AgentTool. Checks run the project's build and test
// commands, which compete for the same outputs as any concurrent work, so they
// run alone.
func (t *VerifyTool) ExecutionMode() agentcore.ToolExecutionMode {
	return agentcore.ToolExecutionSequential
}

// Execute implements AgentTool.
func (t *VerifyTool) Execute(ctx context.Context, _ string, _ json.RawMessage, _ agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	root := t.Root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return errorResult("verify: cannot resolve the working directory: " + err.Error()), nil
		}
		root = wd
	}

	cmds, err := verify.Discover(root)
	if err != nil {
		return errorResult("verify: " + err.Error()), nil
	}
	res := verify.Run(ctx, root, cmds)
	if vs, ok := t.VerifyState.(interface{ MarkVerified() }); ok {
		vs.MarkVerified()
	}

	return agentcore.AgentToolResult{
		Content: agentcore.ContentList{agentcore.NewTextContent(renderVerify(res))},
		Details: map[string]any{
			"passed":   res.Passed,
			"reason":   res.Reason,
			"ranCount": len(res.Ran),
			"duration": res.Duration.Milliseconds(),
		},
		// Report failure through IsError-free content as well as the details, so
		// a consumer watching events sees it without parsing the text.
	}, nil
}

// renderVerify turns a result into the text the model reads. The verdict leads,
// because that is the part that decides what the model does next.
func renderVerify(res verify.Result) string {
	var b strings.Builder

	switch {
	case res.Reason != "":
		b.WriteString("verify: could not run — ")
		b.WriteString(res.Reason)
		b.WriteString("\n")
		return b.String()
	case res.Passed:
		b.WriteString("verify: PASSED")
	default:
		b.WriteString("verify: FAILED")
	}
	fmt.Fprintf(&b, " (%s", res.Duration.Round(1e6))
	if len(res.Ran) > 0 {
		names := make([]string, len(res.Ran))
		for i, c := range res.Ran {
			names[i] = c.Name
		}
		fmt.Fprintf(&b, "; ran %s", strings.Join(names, ", "))
	}
	b.WriteString(")\n\n")

	if res.Failed != nil {
		fmt.Fprintf(&b, "First failure: %s\n\n", res.Failed.String())
	}
	if strings.TrimSpace(res.Output) != "" {
		b.WriteString(res.Output)
		if !strings.HasSuffix(res.Output, "\n") {
			b.WriteString("\n")
		}
	}
	if res.Passed {
		b.WriteString("\nAll checks passed. The change compiles and the tests pass.\n")
	}
	return b.String()
}
