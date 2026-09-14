package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/Ai-Kota/aikota/internal/repomap"
)

// RepoMapTool builds a structural map of the repository: the files that matter,
// ranked by how much the rest of the codebase depends on them, with their
// signatures.
//
// It answers "how is this repo organised / where does this concept live" in one
// call, which otherwise costs the model a dozen grep-and-read round trips.
type RepoMapTool struct {
	// Root bounds the map; empty defaults to the current working directory.
	Root string
}

type repoMapArgs struct {
	MaxTokens    int      `json:"max_tokens"`
	FocusFiles   []string `json:"focus_files"`
	FocusSymbols []string `json:"focus_symbols"`
}

// Name implements AgentTool.
func (t *RepoMapTool) Name() string { return "repo_map" }

// Description implements AgentTool.
func (t *RepoMapTool) Description() string {
	return `Build a structural map of the repository: the highest-ranked files with their key signatures (types, functions, methods).

## When to use
- At the start of work on an unfamiliar repository, to learn its architecture
- Before a cross-file refactor, to find which files are structurally connected
- When looking for where a concept lives and you do not know the file name
- When asked "how is this repo organised" or "what are the important files"

## When NOT to use
- To read a specific file — use read
- To search for exact text or a pattern — use grep
- To locate files by name — use find
- When you already know which files to look at

## Output
Signatures only, never bodies. Files are ranked by structural importance: a file many others import or reference ranks highest. A "⋮" marks where definitions were elided.

## Parameters
- max_tokens: budget for the map. 1024 for an overview, 4096+ to go wider. Default 1024.
- focus_files: repo-relative paths or directories to boost (e.g. ["internal/runtime/"]).
- focus_symbols: symbol names whose defining files to boost (e.g. ["ToolRegistry"]).

Focus is a hint about where to look; it lifts a file but does not hide the rest.`
}

// Schema implements AgentTool.
func (t *RepoMapTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "max_tokens":    {"type": "integer", "minimum": 256, "maximum": 16384, "description": "Token budget for the rendered map. Default 1024."},
    "focus_files":   {"type": "array", "items": {"type": "string"}, "description": "Repo-relative files or directories to boost in the ranking."},
    "focus_symbols": {"type": "array", "items": {"type": "string"}, "description": "Symbol names whose defining files should rank higher."}
  },
  "additionalProperties": false
}`)
}

// ExecutionMode implements AgentTool. The map only reads the filesystem, so it
// can run alongside other tools in a batch.
func (t *RepoMapTool) ExecutionMode() agentcore.ToolExecutionMode {
	return agentcore.ToolExecutionParallel
}

// Execute implements AgentTool.
func (t *RepoMapTool) Execute(_ context.Context, _ string, args json.RawMessage, _ agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	a, bad := decodeArgs[repoMapArgs](args, "repo_map")
	if bad != nil {
		return *bad, nil
	}

	root := t.Root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return errorResult("repo_map: cannot resolve the working directory: " + err.Error()), nil
		}
		root = wd
	}

	m, err := repomap.Build(repomap.Options{
		Root:         root,
		MaxTokens:    a.MaxTokens,
		FocusFiles:   a.FocusFiles,
		FocusSymbols: a.FocusSymbols,
	})
	if err != nil {
		return errorResult("repo_map: " + err.Error()), nil
	}
	if m.FileCount == 0 {
		// Not an error: an empty repository, or one whose files are all in
		// skipped directories. Say so rather than returning a blank result the
		// model would have to interpret.
		return agentcore.AgentToolResult{
			Content: agentcore.ContentList{agentcore.NewTextContent(
				fmt.Sprintf("repo_map: no Go source files found under %s (parsed %d, skipped %d).", root, m.TotalFileCount, m.IgnoredCount))},
			Details: map[string]any{"files": 0, "total": m.TotalFileCount},
		}, nil
	}

	body := fmt.Sprintf("%s\n(%d of %d files shown, ~%d tokens; use read to see a file's contents)",
		m.Rendered, m.FileCount, m.TotalFileCount, m.TokenCount)
	return agentcore.AgentToolResult{
		Content: agentcore.ContentList{agentcore.NewTextContent(body)},
		Details: map[string]any{
			"files":        m.FileCount,
			"totalFiles":   m.TotalFileCount,
			"tokens":       m.TokenCount,
			"buildMs":      m.BuildDuration.Milliseconds(),
			"skippedFiles": m.IgnoredCount,
		},
	}, nil
}
