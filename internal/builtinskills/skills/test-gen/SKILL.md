---
name: test-gen
description: "Generate comprehensive unit tests, integration tests, and benchmarks for Go codebases. Covers edge cases, error paths, and performance benchmarks. Triggers on: test-gen, generate-tests, write-tests, add-tests."
user-invocable: true
---

# Test Gen — Test Generation

Generate comprehensive tests for Go code following best practices.

---

## When to Use

- After writing new functions/methods
- Before merging code
- When test coverage is low
- When user asks: test-gen, generate-tests, write-tests

---

## Test Patterns

### Table-Driven Tests
```go
func TestProcess(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {"valid-input", "hello", "HELLO", false},
        {"empty-input", "", "", false},
        {"invalid-input", "", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Process(tt.input)
            if (tt.wantErr && got == "") || (!tt.wantErr && got != tt.expected) {
                t.Errorf("Process() = %v, want %v", got, tt.expected)
            }
        })
    }
}
```

### Mocking Dependencies
```go
// Use interfaces for testability
type Reader interface {
    Read(p []byte) (n int, err error)
}

// Mock in tests
type mockReader struct {
    data string
    pos  int
}

func (m *mockReader) Read(p []byte) (n int, err error) {
    // Implementation
}
```

### Benchmark Tests
```go
func BenchmarkProcess(b *testing.B) {
    data := make([]string, 1000)
    for i := range data {
        data[i] = fmt.Sprintf("item-%d", i)
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        Process(data)
    }
}
```

---

## Step 1: Identify Testable Units

Analyze code structure:
```bash
# Find all functions
grep -rn "^func " --include="*.go" .

# Check existing tests
find . -name "*_test.go"
```

---

## Step 2: Generate Test Cases

For each function, create:

| Category | Coverage |
|----------|----------|
| Happy path | Normal inputs, expected outputs |
| Edge cases | Empty, nil, zero, max values |
| Error paths | Invalid inputs, timeouts, failures |
| Concurrency | Race conditions, goroutines |
| Performance | Benchmarks for hot paths |

---

## Step 3: Write Tests

Generate test file with:
- Proper test function naming (`TestXxx`)
- Table-driven test structure
- Subtests for each case
- Error assertions
- Cleanup with `t.Cleanup()`

---

## Step 4: Run and Verify

```bash
# Run tests with coverage
go test -cover ./...

# Find untested code
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Race detection
go test -race ./...
```

---

## Output Format

Create at `tasks/test-gen-report-[timestamp].md`:

```markdown
# Test Generation Report

## Files Covered
- src/service.go: +45 tests
- src/utils.go: +23 tests

## Coverage
- Package coverage: 87%
- Function coverage: 92%
- Line coverage: 85%

## New Tests Added
1. TestProcess_ValidInput
2. TestProcess_EmptyInput
3. TestProcess_ErrorPath
4. BenchmarkProcess_Optimized
```

---

## Rules

- Always run tests after generation
- Maintain existing test style
- Use table-driven tests where possible
- Include error cases
- Add benchmarks for performance-critical code
- Don't test implementation details, test behavior
