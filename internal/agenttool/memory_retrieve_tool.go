// This file implements the memory_retrieve AgentTool (Phase 2 CCR): a read-only
// tool that retrieves original text stored in the Compressed Content Repository
// (CCR) cache after compaction. The model can use this to recover full context
// that was summarized during automatic compaction.
package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ai-Kota/aikota/internal/agentcore"
	"github.com/Ai-Kota/aikota/internal/memory"
)

// MemoryRetrieveTool retrieves original text from the CCR cache by ID.
type MemoryRetrieveTool struct {
	// Store is the persistent memory store containing the CCR cache.
	// When nil, Execute degrades to a friendly no-op result.
	Store *memory.Store
}

// memoryRetrieveArgs is the decoded argument shape for memory_retrieve.
type memoryRetrieveArgs struct {
	// ID is the CCR cache ID returned during compaction.
	ID string `json:"id"`
}

// Name implements AgentTool.
func (t *MemoryRetrieveTool) Name() string { return "memory_retrieve" }

// Description implements AgentTool.
func (t *MemoryRetrieveTool) Description() string {
	return "Retrieve original text from the compressed content repository (CCR) cache. " +
		"Use this when you need to access full context from messages that were summarized " +
		"during automatic compaction. The ID should be obtained from a previous compaction " +
		"event or memory_search result. Returns the original text if found and not expired."
}

// Schema implements AgentTool.
func (t *MemoryRetrieveTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": {"type": "string", "description": "The CCR cache ID to retrieve. Format: UUID."}
  },
  "required": ["id"],
  "additionalProperties": false
}`)
}

// ExecutionMode implements AgentTool. memory_retrieve is read-only, so it runs in
// the default parallel mode.
func (t *MemoryRetrieveTool) ExecutionMode() agentcore.ToolExecutionMode {
	return agentcore.ToolExecutionParallel
}

// Execute implements AgentTool. It decodes the args, looks up the CCR entry by ID,
// and returns the original text if found and not expired. A nil Store or invalid
// ID degrades to a friendly no-op result rather than erroring, so the loop keeps running.
func (t *MemoryRetrieveTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	a, bad := decodeArgs[memoryRetrieveArgs](args, "memory_retrieve")
	if bad != nil {
		return *bad, nil
	}

	cacheID := strings.TrimSpace(a.ID)
	if t.Store == nil {
		return agentcore.AgentToolResult{
			Content: agentcore.ContentList{agentcore.NewTextContent(
				"memory_retrieve: no memory store configured; CCR cache unavailable.")},
		}, nil
	}
	if cacheID == "" {
		return agentcore.AgentToolResult{
			Content: agentcore.ContentList{agentcore.NewTextContent(
				"memory_retrieve: empty ID; provide a valid CCR cache ID.")},
		}, nil
	}

	ccr := t.Store.CCRCache()
	if ccr == nil {
		return agentcore.AgentToolResult{
			Content: agentcore.ContentList{agentcore.NewTextContent(
				"memory_retrieve: CCR cache not initialized.")},
		}, nil
	}

	original, found := ccr.Retrieve(cacheID)
	if !found {
		return agentcore.AgentToolResult{
			Content: agentcore.ContentList{agentcore.NewTextContent(
				fmt.Sprintf("memory_retrieve: ID %q not found or expired in CCR cache.", cacheID))},
		}, nil
	}

	return agentcore.AgentToolResult{
		Content: agentcore.ContentList{agentcore.NewTextContent(
			fmt.Sprintf("memory_retrieve: found original text for ID %q:\n\n%s", cacheID, original))},
	}, nil
}
