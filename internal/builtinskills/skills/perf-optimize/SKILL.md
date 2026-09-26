---
name: perf-optimize
description: "Performance profiling and optimization for Go codebases. Identifies bottlenecks, reduces latency, and improves throughput. Triggers on: perf-optimize, performance, optimize, profile, bottleneck, slow-code."
user-invocable: true
---

# Perf Optimize — Performance Optimization

Profile and optimize Go code for better performance, reduced latency, and improved throughput.

---

## When to Use

- After code review identifies performance issues
- When user reports slowness or timeouts
- Before deployment to production
- When metrics show degradation

---

## Profiling Tools

### CPU Profiling
```bash
# Profile for 30 seconds
go test -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof

# In web UI
go tool pprof -http=:8080 cpu.prof
```

### Memory Profiling
```bash
# Heap profile
go test -memprofile=mem.prof ./...
go tool pprof mem.prof

# Alloc profile
go test -alloc_space ./...
```

### Goroutine Profiling
```bash
# Block profile
go test -blockprofile=block.prof ./...

# Mutex contention
go test -mutexprofile=mutex.prof ./...
```

### Tracing
```bash
# Execution trace
go test -trace=trace.out ./...
go tool trace trace.out
```

---

## Common Optimization Patterns

### 1. Reduce Allocations
```go
// Before
func Process(items []string) []string {
    result := make([]string, 0)
    for _, item := range items {
        result = append(result, strings.ToUpper(item))
    }
    return result
}

// After - pre-allocate, reuse buffers
func Process(items []string) []string {
    result := make([]string, len(items))
    for i, item := range items {
        result[i] = strings.ToUpper(item)
    }
    return result
}
```

### 2. Use Sync.Pool for Reusable Objects
```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func Process() {
    buf := bufferPool.Get().(*bytes.Buffer)
    defer bufferPool.Put(buf)
    buf.Reset()
    // ... use buf
}
```

### 3. Avoid String Concatenation
```go
// Before
result := ""
for _, s := range items {
    result += s
}

// After
var sb strings.Builder
for _, s := range items {
    sb.WriteString(s)
}
result := sb.String()
```

### 4. Parallel Processing
```go
// Before
for _, item := range items {
    process(item)
}

// After - using goroutines
var wg sync.WaitGroup
for _, item := range items {
    wg.Add(1)
    go func(i Item) {
        defer wg.Done()
        process(i)
    }(*item)
}
wg.Wait()
```

---

## Step 1: Establish Baseline

Measure current performance:
```bash
# Benchmark current implementation
go test -bench=. -benchmem ./...

# Profile under load
wrk -t12 -c400 -d30s http://localhost:8080/
```

---

## Step 2: Identify Bottlenecks

Use profiling data to find hotspots:
- Top CPU consumers
- Memory allocation hotspots
- Goroutine contention
- I/O bottlenecks

---

## Step 3: Apply Optimizations

Focus on high-impact areas first:

### Algorithm Optimization
- Reduce complexity (O(n²) → O(n log n))
- Use appropriate data structures
- Memoize expensive computations

### Concurrency Optimization
- Proper worker pool sizing
- Minimize lock contention
- Use lock-free structures when possible

### I/O Optimization
- Buffer reads/writes
- Connection pooling
- Async I/O where applicable

---

## Step 4: Verify Improvements

Run benchmarks to confirm:
```bash
# Compare before/after
go test -bench=. -benchmem -run=^$ ./...

# Check allocation reduction
go test -bench=. -benchmem -memprofile=after.prof ./...
```

---

## Output Format

Create report at `tasks/perf-report-[timestamp].md`:

```markdown
# Performance Optimization Report

## Baseline Metrics
- Request latency: X ms (p99)
- Throughput: Y req/s
- Memory usage: Z MB

## Bottlenecks Found
1. [Issue] in [File]:[Line]
   - Impact: High/Medium/Low
   - Root cause: Description

## Optimizations Applied
1. [Optimization]
   - File: path/to/file.go
   - Change: Description
   - Impact: X% improvement

## Results
- Request latency: X ms → Y ms (-Z%)
- Throughput: Y req/s → Z req/s (+W%)
- Allocations reduced by: V%
```

---

## Rules

- Always measure before optimizing
- Optimize one thing at a time
- Verify improvements with benchmarks
- Document performance characteristics
- Consider readability vs. performance trade-offs
