// This file implements the plan tool (US-025): a structured planning interface
// that lets the model create, update, and track multi-step plans before
// executing them.
package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Ai-Kota/aikota/internal/agentcore"
)

// PlanTool manages a structured plan for multi-step work. The model should
// create a plan before starting complex tasks, update it as progress is made,
// and mark it complete when done.
type PlanTool struct {
	mu      sync.Mutex
	plans   map[string]*PlanState
	nextID  atomic.Int64
}

// PlanState is the in-memory representation of a single plan.
type PlanState struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"` // draft, active, complete
	Steps       []PlanStep `json:"steps"`
}

// PlanStep represents one step in a plan.
type PlanStep struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Category    string   `json:"category"` // design, implement, test, review
	Description string   `json:"description"`
	Deps        []string `json:"deps,omitempty"`
	Complexity  string   `json:"complexity"` // low, medium, high
	Done        bool     `json:"done"`
}

// planCmdArgs is the decoded argument shape for PlanTool.
type planCmdArgs struct {
	Action      string          `json:"action"` // create, update, list, get
	ID          string          `json:"id,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Steps       []planStepArg   `json:"steps,omitempty"`
	UpdateSteps []planStepArg   `json:"update_steps,omitempty"`
}

type planStepArg struct {
	ID          string   `json:"id,omitempty"`
	Title       string   `json:"title"`
	Category    string   `json:"category,omitempty"`
	Description string   `json:"description,omitempty"`
	Deps        []string `json:"deps,omitempty"`
	Complexity  string   `json:"complexity,omitempty"`
	Done        bool     `json:"done,omitempty"`
}

var _ agentcore.AgentTool = (*PlanTool)(nil)

// Name implements AgentTool.
func (t *PlanTool) Name() string { return "plan" }

// Description implements AgentTool.
func (t *PlanTool) Description() string {
	return "Create and manage structured plans for multi-step work. " +
		"Actions: create (start a new plan), update (modify steps), " +
		"list (show all plans), get (show plan details). " +
		"Use this before starting complex tasks to outline the approach."
}

// Schema implements AgentTool.
func (t *PlanTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["create", "update", "list", "get"], "description": "Plan action to perform."},
    "id": {"type": "string", "description": "Plan ID (for update/get). Auto-generated for create."},
    "title": {"type": "string", "description": "Plan title (required for create)."},
    "description": {"type": "string", "description": "Plan description."},
    "steps": {"type": "array", "items": {"$ref": "#/definitions/PlanStep"}, "description": "Steps to create."},
    "update_steps": {"type": "array", "items": {"$ref": "#/definitions/PlanStep"}, "description": "Steps to update."}
  },
  "required": ["action"],
  "additionalProperties": false,
  "definitions": {
    "PlanStep": {
      "type": "object",
      "properties": {
        "id": {"type": "string"},
        "title": {"type": "string", "description": "Step title."},
        "category": {"type": "string", "enum": ["design", "implement", "test", "review"]},
        "description": {"type": "string"},
        "deps": {"type": "array", "items": {"type": "string"}},
        "complexity": {"type": "string", "enum": ["low", "medium", "high"]},
        "done": {"type": "boolean", "description": "Mark step complete."}
      },
      "required": ["title"]
    }
  }
}`)
}

// ExecutionMode implements AgentTool. Plan modifies state → sequential.
func (t *PlanTool) ExecutionMode() agentcore.ToolExecutionMode {
	return agentcore.ToolExecutionSequential
}

// Execute implements AgentTool.
func (t *PlanTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
	a, bad := decodeArgs[planCmdArgs](args, "plan")
	if bad != nil {
		return *bad, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch a.Action {
	case "create":
		return t.create(a)
	case "update":
		return t.update(a)
	case "list":
		return t.list()
	case "get":
		return t.get(a.ID)
	default:
		return errorResult(fmt.Sprintf("plan: unknown action %q", a.Action)), nil
	}
}

func (t *PlanTool) create(a planCmdArgs) (agentcore.AgentToolResult, error) {
	if a.Title == "" {
		return errorResult("plan create: title is required"), nil
	}

	id := fmt.Sprintf("plan-%d", t.nextID.Add(1))
	plan := &PlanState{
		ID:          id,
		Title:       a.Title,
		Description: a.Description,
		Status:      "draft",
		Steps:       make([]PlanStep, 0, len(a.Steps)),
	}

	for _, s := range a.Steps {
		stepID := s.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step-%d", len(plan.Steps)+1)
		}
		plan.Steps = append(plan.Steps, PlanStep{
			ID:          stepID,
			Title:       s.Title,
			Category:    s.Category,
			Description: s.Description,
			Deps:        s.Deps,
			Complexity:  s.Complexity,
			Done:        false,
		})
	}

	t.plans[id] = plan
	return renderPlan(id, plan), nil
}

func (t *PlanTool) update(a planCmdArgs) (agentcore.AgentToolResult, error) {
	if a.ID == "" {
		return errorResult("plan update: id is required"), nil
	}
	plan, ok := t.plans[a.ID]
	if !ok {
		return errorResult(fmt.Sprintf("plan update: plan %q not found", a.ID)), nil
	}

	if a.Title != "" {
		plan.Title = a.Title
	}
	if a.Description != "" {
		plan.Description = a.Description
	}

	for _, us := range a.UpdateSteps {
		for i := range plan.Steps {
			if plan.Steps[i].ID == us.ID || plan.Steps[i].Title == us.Title {
				if us.Title != "" {
					plan.Steps[i].Title = us.Title
				}
				if us.Category != "" {
					plan.Steps[i].Category = us.Category
				}
				if us.Description != "" {
					plan.Steps[i].Description = us.Description
				}
				if len(us.Deps) > 0 {
					plan.Steps[i].Deps = us.Deps
				}
				if us.Complexity != "" {
					plan.Steps[i].Complexity = us.Complexity
				}
				plan.Steps[i].Done = us.Done
			}
		}
	}

	plan.Status = planStatus(plan)
	return renderPlan(a.ID, plan), nil
}

func (t *PlanTool) list() (agentcore.AgentToolResult, error) {
	if len(t.plans) == 0 {
		return agentcore.AgentToolResult{
			Content: agentcore.ContentList{agentcore.NewTextContent("No plans exist. Use action=create to create one.")},
		}, nil
	}

	var sb strings.Builder
	for _, p := range t.plans {
		done := countDone(p.Steps)
		fmt.Fprintf(&sb, "- %s (%s): %s — %d/%d steps\n", p.ID, p.Status, p.Title, done, len(p.Steps))
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(sb.String())}}, nil
}

func (t *PlanTool) get(id string) (agentcore.AgentToolResult, error) {
	if id == "" {
		return errorResult("plan get: id is required"), nil
	}
	plan, ok := t.plans[id]
	if !ok {
		return errorResult(fmt.Sprintf("plan get: plan %q not found", id)), nil
	}
	return renderPlan(id, plan), nil
}

func planStatus(p *PlanState) string {
	if len(p.Steps) == 0 {
		return "draft"
	}
	done := countDone(p.Steps)
	if done == len(p.Steps) {
		return "complete"
	}
	if done > 0 {
		return "active"
	}
	return "draft"
}

func renderPlan(id string, p *PlanState) agentcore.AgentToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Plan: %s (%s)\n", p.Title, p.Status)
	if p.Description != "" {
		fmt.Fprintf(&sb, "\n%s\n", p.Description)
	}
	fmt.Fprintf(&sb, "\nSteps (%d/%d):\n", countDone(p.Steps), len(p.Steps))
	for _, s := range p.Steps {
		mark := "[ ]"
		if s.Done {
			mark = "[x]"
		}
		fmt.Fprintf(&sb, "  %s %s", mark, s.Title)
		if s.Category != "" {
			fmt.Fprintf(&sb, " (%s)", s.Category)
		}
		if s.Complexity != "" {
			fmt.Fprintf(&sb, " [%s]", s.Complexity)
		}
		if len(s.Deps) > 0 {
			fmt.Fprintf(&sb, " deps:[%s]", strings.Join(s.Deps, ","))
		}
		fmt.Fprintf(&sb, "\n")
		if s.Description != "" {
			fmt.Fprintf(&sb, "    %s\n", s.Description)
		}
	}
	return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(sb.String())}}
}

func countDone(steps []PlanStep) int {
	n := 0
	for _, s := range steps {
		if s.Done {
			n++
		}
	}
	return n
}
