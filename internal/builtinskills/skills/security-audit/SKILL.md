---
name: security-audit
description: "Security audit for Go codebases. Detects vulnerabilities, injection risks, unsafe patterns, and compliance issues. Triggers on: security-audit, security-check, find-vulnerabilities, audit-security."
user-invocable: true
---

# Security Audit — Vulnerability Scanner

Comprehensive security audit for Go applications detecting vulnerabilities and unsafe patterns.

---

## When to Use

- Before production deployment
- After significant code changes
- When user reports: security-audit, security-check, find-vulnerabilities
- Quarterly security reviews

---

## Common Vulnerabilities to Check

### 1. SQL Injection
```go
// ❌ Vulnerable
db.Query("SELECT * FROM users WHERE name = '" + name + "'")

// ✅ Safe
db.Query("SELECT * FROM users WHERE name = ?", name)
```

### 2. Command Injection
```go
// ❌ Vulnerable
exec.Command("sh", "-c", "echo " + userInput)

// ✅ Safe
exec.Command("echo", userInput)
```

### 3. Hardcoded Secrets
```go
// ❌ Bad
password := "admin123"
apiKey := "sk-1234567890"

// ✅ Good
password := os.Getenv("DB_PASSWORD")
```

### 4. Race Conditions
```go
// ❌ Unsafe
var counter int
go func() { counter++ }()

// ✅ Safe
var mu sync.Mutex
var counter int
go func() {
    mu.Lock()
    counter++
    mu.Unlock()
}()
```

---

## Step 1: Static Analysis

Run automated security scanners:
```bash
# gosec - Go security checker
gosec ./...

# bandit for Python
bandit -r .

# semgrep for general patterns
semgrep --config=p/security
```

---

## Step 2: Dependency Audit

```bash
# Check for vulnerable dependencies
go list -m all | grep -i vuln
govulncheck ./...

# Review go.mod for risky dependencies
cat go.mod | grep -E "crypto|http|auth"
```

---

## Step 3: Manual Review

Check these high-risk areas:

| Area | What to Check |
|------|---------------|
| Authentication | Weak passwords, session management |
| Authorization | Access control, RBAC |
| Input Validation | Sanitization, type checking |
| Cryptography | Weak algorithms, key management |
| Error Handling | Information leakage |
| Logging | Sensitive data exposure |

---

## Step 4: Report Findings

Create report at `tasks/security-audit-[timestamp].md`:

```markdown
# Security Audit Report

## Critical Findings
1. [CVE-2024-XXXX] in package X
   - Location: file.go:42
   - Fix: Update dependency

## Medium Findings
1. SQL query built with string concatenation
   - Location: db/query.go:15
   - Fix: Use parameterized queries

## Low Findings
1. Hardcoded API key
   - Location: config.go:8
   - Fix: Use environment variable

## Compliance Status
- OWASP Top 10: X violations
- CIS Benchmark: Pass/Fail
```

---

## Rules

- Never commit fixes without verification
- Run full test suite after changes
- Document all findings with severity
- Prioritize by exploitability and impact
- Keep audit logs for compliance
