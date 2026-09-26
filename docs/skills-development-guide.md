# Skills Development Guide

This guide explains how to create new skills for aikota.

## Skill Structure

Each skill is a directory containing a `SKILL.md` file:

```
skills/
  skill-name/
    SKILL.md      # Skill definition and instructions
    README.md     # Optional extended documentation
    references/   # Optional reference materials
```

## SKILL.md Format

```yaml
---
name: skill-name
description: "Brief description of what this skill does."
user-invocable: true
allowed-tools:
  - Bash(git:*)
  - Bash(go:*)
---
```

### Required Fields

| Field | Description |
|-------|-------------|
| `name` | Unique skill identifier (kebab-case) |
| `description` | What the skill does and when to use it |
| `user-invocable` | Set to `true` if users can trigger this skill |

### Optional Fields

| Field | Description |
|-------|-------------|
| `allowed-tools` | List of tools this skill may use |
| `metadata` | Additional metadata (version, author, etc.) |

## Skill Components

### 1. Overview Section

Briefly describe what the skill does and when to use it.

```markdown
## When to Use

- After code review identifies issues
- When user asks for: skill-name
- Before deployment to production
```

### 2. Step-by-Step Instructions

Break down the skill into clear steps:

```markdown
## Step 1: Analyze Current State

...

## Step 2: Apply Changes

...

## Step 3: Verify Results

...
```

### 3. Code Examples

Provide concrete examples:

```go
// Before
func BadExample() {
    // ...
}

// After
func GoodExample() {
    // ...
}
```

### 4. Output Format

Specify where results should be saved:

```markdown
## Output Format

Create report at `tasks/report-[timestamp].md`:
```

### 5. Rules and Constraints

Define what the skill should and shouldn't do:

```markdown
## Rules

- Always run tests after changes
- Never commit without confirmation
- Preserve existing behavior
```

## Skill Categories

### Code Quality
- `review-it` — Code review
- `clean-code` — Refactoring
- `smell` — Architecture analysis
- `perf-optimize` — Performance tuning

### Testing
- `test-gen` — Test generation
- `loop-it` — Automated issue loop

### Documentation
- `note-it` — Documentation
- `code-to-spec` — Spec generation
- `prd-to-spec` — PRD to spec

### Architecture
- `architecture-diagram` — Diagram generation
- `to-design` — Design conversion
- `to-issues` — Issue creation

### Deployment
- `ship-it` — Shipping code
- `pr` — PR management

### Utilities
- `weather` — Weather lookup
- `graph` — Graph analysis

## Creating a New Skill

1. Create directory: `internal/builtinskills/skills/my-skill/`
2. Add `SKILL.md` with frontmatter
3. Write step-by-step instructions
4. Add code examples
5. Test the skill manually
6. Commit with clear message

## Example: Minimal Skill

```markdown
---
name: hello-world
description: "Prints a greeting message."
user-invocable: true
---

# Hello World

## Usage

Run this skill to print a greeting.

## Steps

1. Print hello message
2. Include current date

## Output

Print to stdout: "Hello, world!"
```

## Best Practices

- Keep skills focused on one task
- Use clear, actionable language
- Provide concrete examples
- Define success criteria
- Document edge cases
- Update when behavior changes

## Testing Skills

Test each skill manually:
1. Create a test project
2. Run the skill
3. Verify output
4. Check for errors
5. Document results
