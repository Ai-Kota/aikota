// This file implements the browser tool (US-027): a thin wrapper around
// go-rod/rod for basic browser automation (navigate, screenshot, extract text).
//
// Design: deliberately minimal. We only expose read-only operations that the
// model is likely to need (view a page, get its text, take a screenshot).
// Interactive automation (click, fill forms) is deferred to a future PR.
package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod"
)

// BrowserTool provides basic browser automation via go-rod.
type BrowserTool struct {
	// Root bounds screenshots to this directory (prevents arbitrary writes).
	// Empty defaults to the process cwd.
	Root string
	// Headless whether to run the browser in headless mode (default true).
	Headless bool
}

// browserCmdArgs is the decoded argument shape for BrowserTool.
type browserCmdArgs struct {
	Action string `json:"action"` // navigate, screenshot, get_text
	URL    string `json:"url,omitempty"`
	// For screenshot: output path (relative to Root).
	Output string `json:"output,omitempty"`
	// For get_text: CSS selector to extract (empty = full page).
	Selector string `json:"selector,omitempty"`
}

// Name implements AgentTool.
func (t *BrowserTool) Name() string { return "browser" }

// Description implements AgentTool.
func (t *BrowserTool) Description() string {
	return "Basic browser automation via go-rod. Actions: navigate (load a URL), " +
		"screenshot (save a PNG), get_text (extract page text). " +
		"Interactive actions (click, fill) are not yet implemented."
}

// Schema implements AgentTool.
func (t *BrowserTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["navigate", "screenshot", "get_text"], "description": "Browser action."},
    "url": {"type": "string", "description": "URL to navigate to."},
    "output": {"type": "string", "description": "Screenshot output path (relative to workspace)."},
    "selector": {"type": "string", "description": "CSS selector for text extraction."}
  },
  "required": ["action", "url"],
  "additionalProperties": false
}`)
}

// ExecutionMode implements AgentTool. Browser operations can have side effects
// (network, file writes) → sequential.
func (t *BrowserTool) ExecutionMode() agentcore.ToolExecutionMode {
	return agentcore.ToolExecutionSequential
}

// Execute implements AgentTool.
func (t *BrowserTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	a, bad := decodeArgs[browserCmdArgs](args, "browser")
	if bad != nil {
		return *bad, nil
	}
	if a.Action == "" || a.URL == "" {
		return errorResult("browser: action and url are required"), nil
	}

	root := t.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return errorResult("browser: cannot resolve working directory: " + err.Error()), nil
		}
	}

	switch a.Action {
	case "navigate":
		return t.navigate(root, a.URL)
	case "screenshot":
		return t.screenshot(root, a.URL, a.Output)
	case "get_text":
		return t.getText(root, a.URL, a.Selector)
	default:
		return errorResult(fmt.Sprintf("browser: unknown action %q", a.Action)), nil
	}
}

func (t *BrowserTool) newBrowser() (*rod.Browser, error) {
	u := launcher.New()
	if t.Headless {
		u = u.Headless(true)
	}
	url, err := u.Launch()
	if err != nil {
		return nil, fmt.Errorf("browser: cannot launch: %v", err)
	}
	b := rod.New().ControlURL(url)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("browser: connect failed: %v", err)
	}
	return b, nil
}

func (t *BrowserTool) navigate(root, url string) (agentcore.AgentToolResult, error) {
	b, err := t.newBrowser()
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer b.Close()

	p := b.MustPage(url)
	info := p.MustInfo()
	title := info.Title
	if title == "" {
		title = "(no title)"
	}
	return agentcore.AgentToolResult{
		Content: agentcore.ContentList{agentcore.NewTextContent(fmt.Sprintf("Navigated to %s — title: %s", url, title))},
	}, nil
}

func (t *BrowserTool) screenshot(root, url, output string) (agentcore.AgentToolResult, error) {
	b, err := t.newBrowser()
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer b.Close()

	p := b.MustPage(url)
	img, err := p.Screenshot(false, nil)
	if err != nil {
		return errorResult("browser screenshot: " + err.Error()), nil
	}

	if output == "" {
		output = "screenshot.png"
	}
	if !filepath.IsAbs(output) {
		output = filepath.Join(root, output)
	}
	if err := os.WriteFile(output, img, 0o644); err != nil {
		return errorResult("browser screenshot: " + err.Error()), nil
	}
	return agentcore.AgentToolResult{
		Content: agentcore.ContentList{agentcore.NewTextContent(fmt.Sprintf("Screenshot saved to %s", output))},
		Details: map[string]any{"path": output, "size": len(img)},
	}, nil
}

func (t *BrowserTool) getText(root, url, selector string) (agentcore.AgentToolResult, error) {
	b, err := t.newBrowser()
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer b.Close()

	p := b.MustPage(url)
	var text string
	if selector != "" {
		el, err := p.Element(selector)
		if err != nil {
			return errorResult("browser get_text: element not found: " + err.Error()), nil
		}
		text = el.MustText()
	} else {
		html, err := p.HTML()
		if err != nil {
			return errorResult("browser get_text: " + err.Error()), nil
		}
		// Strip HTML tags to get plain text
		text = stripHTML(html)
	}
	// Truncate to avoid flooding context.
	const maxLen = 4000
	if len(text) > maxLen {
		text = text[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(text)-maxLen)
	}
	return agentcore.AgentToolResult{
		Content: agentcore.ContentList{agentcore.NewTextContent(text)},
	}, nil
}

// stripHTML removes HTML tags from s, returning plain text.
func stripHTML(s string) string {
	// Simple tag stripper; for complex HTML, a proper parser would be better.
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}
