---
name: clean-code
description: "Apply clean code principles to refactor messy code. Detects and fixes common code smells, improves readability, and enforces best practices. Triggers on: clean-code, refactor, code-cleanup, improve-code-quality, fix-smells."
user-invocable: true
---

# Clean Code — Code Quality Refactor

Apply clean code principles to improve code quality, readability, and maintainability. Detects and fixes common code smells.

---

## When to Use

- After reviewing code that has quality issues
- When user asks for: clean-code, refactor, code-cleanup, improve-code-quality
- When code smells are detected during review
- Before merging code that doesn't meet quality standards

---

## Core Principles

### 1. Meaningful Names
- Variables, functions, and classes should reveal their purpose
- Avoid abbreviations and vague names (e.g., `data`, `temp`, `handler`)
- Use domain-specific terminology

### 2. Single Responsibility
- Functions should do one thing and do it well
- Limit function length (max 20-30 lines)
- Each function should have one reason to change

### 3. DRY (Don't Repeat Yourself)
- Eliminate duplication through abstraction
- Extract repeated logic into reusable functions
- Maintain single source of truth

### 4. SOLID Principles
- **S**ingle Responsibility
- **O**pen/Closed
- **L**iskov Substitution
- **I**nterface Segregation
- **D**ependency Inversion

### 5. Readability
- Code should read like well-written prose
- Use consistent formatting
- Add comments only when necessary (why, not what)

---

## Step 1: Analyze Current Code

Before refactoring, understand the current state:

```bash
# Check for common issues
grep -n "TODO\|FIXME\|HACK\|XXX" <file>
wc -l <file>  # Check file length
# Identify long functions
awk '/^func/{f=$0} length($0)>100{print NR": "f}' <file>
```

### Code Smell Checklist

| Smell | Detection | Fix |
|-------|-----------|-----|
| Long function | > 30 lines | Extract methods |
| Large class | > 500 lines | Split responsibilities |
| Duplicate code | Similar blocks | Extract to function |
| God object | Many dependencies | Decompose |
| Feature envy | Accessing other objects' data | Move method |
| Dead code | Unused imports/functions | Remove |
| Magic numbers | Hardcoded values | Extract constants |
| Long parameter list | > 3 params | Use object/struct |

---

## Step 2: Prioritize Issues

Rank code smells by impact:

1. **High Priority**
   - Bug fixes
   - Security issues
   - Performance problems

2. **Medium Priority**
   - Readability issues
   - Duplication
   - Complex logic

3. **Low Priority**
   - Style inconsistencies
   - Minor naming improvements

---

## Step 3: Refactor

### Safe Refactoring Techniques

1. **Extract Method**
   ```go
   // Before
   func ProcessData(data []string) {
       // 50 lines of code...
   }
   
   // After
   func ProcessData(data []string) {
       validateInput(data)
       transformData(data)
       saveResults(data)
   }
   ```

2. **Rename Variable/Function**
   - Use descriptive names
   - Maintain semantic meaning
   - Update all references

3. **Extract Class**
   - Group related functionality
   - Create focused types

4. **Remove Duplication**
   - Identify common patterns
   - Create reusable abstractions

---

## Step 4: Verify

After refactoring:

```bash
# Run tests
go test ./...

# Check linting
golangci-lint run

# Verify no regressions
git diff --stat
```

---

## Output Format

Create a report at `tasks/clean-code-report-[timestamp].md`:

```markdown
# Clean Code Report

## Summary
- Files analyzed: X
- Issues found: Y
- Issues fixed: Z

## High Priority Fixes
1. [Issue description]
   - File: path/to/file.go
   - Line: 42
   - Fix: Extract method `validateInput`

## Medium Priority
...

## Changes Made
- Refactored `ProcessData` into smaller functions
- Renamed `d` to `dataItems`
- Added constants for magic numbers

## Verification
- All tests passing
- Linting clean
- No regressions detected
```

---

## Rules

- Never commit uncommitted changes without confirmation
- Run full test suite after each significant change
- Preserve existing behavior
- Update documentation if APIs change
- Keep changes atomic and reversible
- Ask for clarification if scope is unclear
