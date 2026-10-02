// Package agenttool implements aikota's built-in tools.
// This file implements LSP tools for semantic code queries.
package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

// LspToolBase is the common base for all LSP tools.
type LspToolBase struct {
	Root       string
	ServerPath string
	ToolName   string
	ToolDesc   string
	SchemaJSON string
	Method     string
}

// --- lsp_definition ---

// LspDefinitionTool finds the definition of a symbol.
type LspDefinitionTool struct {
	LspToolBase
}

func NewLspDefinitionTool(root, serverPath string) *LspDefinitionTool {
	return &LspDefinitionTool{
		LspToolBase: LspToolBase{
			Root:       root,
			ServerPath: serverPath,
			ToolName:   "lsp_definition",
			ToolDesc:   "Find the definition location of a symbol at the given file and position.",
			SchemaJSON: `{
				"type": "object",
				"properties": {
					"file": {"type": "string", "description": "Absolute path to the source file."},
					"line": {"type": "integer", "description": "1-based line number."},
					"column": {"type": "integer", "description": "1-based column number."}
				},
				"required": ["file", "line", "column"],
				"additionalProperties": false
			}`,
			Method: "textDocument/definition",
		},
	}
}

func (t *LspDefinitionTool) Name() string { return t.ToolName }
func (t *LspDefinitionTool) Description() string { return t.ToolDesc }
func (t *LspDefinitionTool) Schema() json.RawMessage { return json.RawMessage(t.SchemaJSON) }
func (t *LspDefinitionTool) ExecutionMode() agentcore.ToolExecutionMode { return agentcore.ToolExecutionParallel }

func (t *LspDefinitionTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	var a struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult(t.ToolName + ": invalid args: " + err.Error()), nil
	}
	if a.File == "" || a.Line < 1 || a.Column < 1 {
		return errorResult(t.ToolName + ": file, line, column are required"), nil
	}

	fullPath, err := resolveWithin(t.Root, a.File)
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}

	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uriFromPath(fullPath)},
		"position":     map[string]interface{}{"line": a.Line - 1, "character": a.Column - 1},
	}

	client, err := NewLspClient(LspClientOptions{ServerPath: t.ServerPath})
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}
	defer client.Close()

	go client.readResponses()

	resp, err := client.Call(ctx, t.Method, params)
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}

	return formatLocations(resp)
}

// --- lsp_references ---

// LspReferencesTool finds all references to a symbol.
type LspReferencesTool struct {
	LspToolBase
}

func NewLspReferencesTool(root, serverPath string) *LspReferencesTool {
	return &LspReferencesTool{
		LspToolBase: LspToolBase{
			Root:       root,
			ServerPath: serverPath,
			ToolName:   "lsp_references",
			ToolDesc:   "Find all references to a symbol at the given file and position.",
			SchemaJSON: `{
				"type": "object",
				"properties": {
					"file": {"type": "string", "description": "Absolute path to the source file."},
					"line": {"type": "integer", "description": "1-based line number."},
					"column": {"type": "integer", "description": "1-based column number."},
					"include_declaration": {"type": "boolean", "description": "Include the declaration itself.", "default": false}
				},
				"required": ["file", "line", "column"],
				"additionalProperties": false
			}`,
			Method: "textDocument/references",
		},
	}
}

func (t *LspReferencesTool) Name() string { return t.ToolName }
func (t *LspReferencesTool) Description() string { return t.ToolDesc }
func (t *LspReferencesTool) Schema() json.RawMessage { return json.RawMessage(t.SchemaJSON) }
func (t *LspReferencesTool) ExecutionMode() agentcore.ToolExecutionMode { return agentcore.ToolExecutionParallel }

func (t *LspReferencesTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	var a struct {
		File                 string `json:"file"`
		Line                 int    `json:"line"`
		Column               int    `json:"column"`
		IncludeDeclaration bool   `json:"include_declaration"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult(t.ToolName + ": invalid args: " + err.Error()), nil
	}
	if a.File == "" || a.Line < 1 || a.Column < 1 {
		return errorResult(t.ToolName + ": file, line, column are required"), nil
	}

	fullPath, err := resolveWithin(t.Root, a.File)
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}

	params := map[string]interface{}{
		"textDocument":       map[string]string{"uri": uriFromPath(fullPath)},
		"position":           map[string]interface{}{"line": a.Line - 1, "character": a.Column - 1},
		"includeDeclaration": a.IncludeDeclaration,
	}

	client, err := NewLspClient(LspClientOptions{ServerPath: t.ServerPath})
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}
	defer client.Close()

	go client.readResponses()

	resp, err := client.Call(ctx, t.Method, params)
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}

	return formatLocations(resp)
}

// --- lsp_symbols ---

// LspSymbolsTool lists symbols in a file.
type LspSymbolsTool struct {
	LspToolBase
}

func NewLspSymbolsTool(root, serverPath string) *LspSymbolsTool {
	return &LspSymbolsTool{
		LspToolBase: LspToolBase{
			Root:       root,
			ServerPath: serverPath,
			ToolName:   "lsp_symbols",
			ToolDesc:   "List all top-level symbols in a file with their kinds and locations.",
			SchemaJSON: `{
				"type": "object",
				"properties": {
					"file": {"type": "string", "description": "Absolute path to the source file."},
					"symbol_kind": {"type": "integer", "description": "Optional: filter by symbol kind."}
				},
				"required": ["file"],
				"additionalProperties": false
			}`,
			Method: "textDocument/documentSymbol",
		},
	}
}

func (t *LspSymbolsTool) Name() string { return t.ToolName }
func (t *LspSymbolsTool) Description() string { return t.ToolDesc }
func (t *LspSymbolsTool) Schema() json.RawMessage { return json.RawMessage(t.SchemaJSON) }
func (t *LspSymbolsTool) ExecutionMode() agentcore.ToolExecutionMode { return agentcore.ToolExecutionParallel }

func (t *LspSymbolsTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	var a struct {
		File       string `json:"file"`
		SymbolKind *int   `json:"symbol_kind"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult(t.ToolName + ": invalid args: " + err.Error()), nil
	}
	if a.File == "" {
		return errorResult(t.ToolName + ": file is required"), nil
	}

	fullPath, err := resolveWithin(t.Root, a.File)
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errorResult(t.ToolName + ": file does not exist: " + a.File), nil
		}
		return errorResult(t.ToolName + ": cannot stat file: " + err.Error()), nil
	}
	if info.IsDir() {
		return errorResult(t.ToolName + ": path is a directory: " + a.File), nil
	}

	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uriFromPath(fullPath)},
	}
	if a.SymbolKind != nil {
		params["filterKind"] = *a.SymbolKind
	}

	client, err := NewLspClient(LspClientOptions{ServerPath: t.ServerPath})
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}
	defer client.Close()

	go client.readResponses()

	resp, err := client.Call(ctx, t.Method, params)
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}

	return formatSymbols(resp)
}

// --- lsp_hover ---

// LspHoverTool shows documentation for a symbol.
type LspHoverTool struct {
	LspToolBase
}

func NewLspHoverTool(root, serverPath string) *LspHoverTool {
	return &LspHoverTool{
		LspToolBase: LspToolBase{
			Root:       root,
			ServerPath: serverPath,
			ToolName:   "lsp_hover",
			ToolDesc:   "Show documentation/hover information for a symbol at the given file and position.",
			SchemaJSON: `{
				"type": "object",
				"properties": {
					"file": {"type": "string", "description": "Absolute path to the source file."},
					"line": {"type": "integer", "description": "1-based line number."},
					"column": {"type": "integer", "description": "1-based column number."}
				},
				"required": ["file", "line", "column"],
				"additionalProperties": false
			}`,
			Method: "textDocument/hover",
		},
	}
}

func (t *LspHoverTool) Name() string { return t.ToolName }
func (t *LspHoverTool) Description() string { return t.ToolDesc }
func (t *LspHoverTool) Schema() json.RawMessage { return json.RawMessage(t.SchemaJSON) }
func (t *LspHoverTool) ExecutionMode() agentcore.ToolExecutionMode { return agentcore.ToolExecutionParallel }

func (t *LspHoverTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	var a struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult(t.ToolName + ": invalid args: " + err.Error()), nil
	}
	if a.File == "" || a.Line < 1 || a.Column < 1 {
		return errorResult(t.ToolName + ": file, line, column are required"), nil
	}

	fullPath, err := resolveWithin(t.Root, a.File)
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}

	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uriFromPath(fullPath)},
		"position":     map[string]interface{}{"line": a.Line - 1, "character": a.Column - 1},
	}

	client, err := NewLspClient(LspClientOptions{ServerPath: t.ServerPath})
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}
	defer client.Close()

	go client.readResponses()

	resp, err := client.Call(ctx, t.Method, params)
	if err != nil {
		return errorResult(t.ToolName + ": " + err.Error()), nil
	}

	return formatHover(resp)
}

// --- Formatters ---

func formatLocations(resp json.RawMessage) (agentcore.AgentToolResult, error) {
	var locations []LspLocation
	if err := json.Unmarshal(resp, &locations); err != nil {
		var loc LspLocation
		if err2 := json.Unmarshal(resp, &loc); err2 == nil {
			locations = []LspLocation{loc}
		} else {
			return errorResult("lsp: cannot parse response"), nil
		}
	}

	if len(locations) == 0 {
		return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent("No results found.")}}, nil
	}

	var b strings.Builder
	for i, loc := range locations {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("%d. %s:%d:%d", i+1, loc.Uri, loc.Range.Start.Line+1, loc.Range.Start.Character+1))
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(b.String())}}, nil
}

func formatSymbols(resp json.RawMessage) (agentcore.AgentToolResult, error) {
	var symbols []struct {
		Name          string `json:"name"`
		Kind          int    `json:"kind"`
		Location      struct {
			Uri    string `json:"uri"`
			Range  struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
			} `json:"range"`
		} `json:"location"`
		ContainerName string `json:"containerName,omitempty"`
	}
	if err := json.Unmarshal(resp, &symbols); err != nil {
		return errorResult("lsp: cannot parse response"), nil
	}

	if len(symbols) == 0 {
		return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent("No symbols found.")}}, nil
	}

	var b strings.Builder
	for _, s := range symbols {
		kindName := LspSymbolKindName(LspSymbolKind(s.Kind))
		loc := fmt.Sprintf("%s:%d:%d", s.Location.Uri, s.Location.Range.Start.Line+1, s.Location.Range.Start.Character+1)
		if s.ContainerName != "" {
			b.WriteString(fmt.Sprintf("%s (%s) in %s — %s\n", s.Name, kindName, s.ContainerName, loc))
		} else {
			b.WriteString(fmt.Sprintf("%s (%s) — %s\n", s.Name, kindName, loc))
		}
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(strings.TrimSpace(b.String()))}}, nil
}

func formatHover(resp json.RawMessage) (agentcore.AgentToolResult, error) {
	var hover LspHover
	if err := json.Unmarshal(resp, &hover); err != nil {
		return errorResult("lsp: cannot parse response"), nil
	}

	var b strings.Builder
	for _, content := range hover.Contents {
		if content.Language != "" {
			b.WriteString(fmt.Sprintf("```%s\n", content.Language))
		}
		b.WriteString(content.Value)
		if content.Language != "" {
			b.WriteString("\n```\n")
		}
		b.WriteString("\n")
	}

	result := strings.TrimSpace(b.String())
	if result == "" {
		return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent("No hover information available.")}}, nil
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(result)}}, nil
}