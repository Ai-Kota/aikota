# SPEC: LSP Tool（语言服务器协议客户端）

> Technical specification derived from: `docs/capability-roadmap.md` §2 (LSP) + `tasks/prd-tui-agent.md` §工具卡片
> Generated: 2026-10-01 | Target branch: `master` | Base commit: 560d326

## 1. Summary

### 1.1 What This SPEC Covers
本 SPEC 定义 aikota 的 LSP Tool：一个通过标准 LSP（Language Server Protocol, JSON-RPC 2.0）与外部语言服务器通信的 agent 工具集。覆盖 Go 语言的 `gopls`、Python 的 `pyright`、TypeScript 的 `typescript-language-server` 等常见 server。**aikota 只实现通用 LSP client，不内置任何具体 LSP server**。server 由用户通过 `config.toml` 的 `[lsp]` 段配置路径或保持"自动检测"。

范围：
- `internal/agenttool/lsp_tool.go`：工具定义（`lsp_definition`、`lsp_references`、`lsp_symbols`、`lsp_hover`）
- `internal/agenttool/lsp_client.go`：LSP JSON-RPC over stdio 客户端
- `internal/agenttool/lsp_types.go`：LSP 协议类型定义
- `internal/cli/config/config.go`：新增 `[lsp]` 配置段

**不覆盖**：
- LSP server 的实现或分发（用户自行安装）
- 多语言自动探测（初始仅支持 Go，其他语言通过扩展 server_path 配置）
- LSP workspace 配置（如 `.vscode/settings.json`）
- 增量同步（`textDocument/didChange` 等）—— 仅需一次性查询

### 1.2 PRD Reference
- Source: `docs/capability-roadmap.md` §2 LSP 行
- Design Principle: `§5.5` — 通用能力进产品，基础设施耦合留外面

### 1.3 Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Client vs Server | 内置 client，server 外部 | server 是第三方工具（gopls/pyright），不属于 aikota 也不属于 AImlyForge 基础设施；用户独立管理版本和安装 |
| LSP protocol | JSON-RPC 2.0 over stdio | LSP 官方协议，gopls 等主流 server 均支持 |
| 多语言支持 | 初始 Go，扩展通过 `server_path` | Go 是 aikota 主力语言；其他语言通过 `[lsp].server_path = "/path/to/server"` 配置即可 |
| 工具粒度 | 4 个独立工具（definition/references/symbols/hover） | 每个工具对应一个 LSP method，schema 独立，便于 model 按需调用 |
| 进程生命周期 | 每个 Execute 启动新进程 | 简单、无状态共享问题；LSP server 启动成本 (~50ms) 可接受；未来可优化为长连接 |
| 错误处理 | server 不可用 → 提示用户安装 | 与 `ReadTool` 风格一致：encode error in result，不抛 Go error |
| 路径校验 | 复用 `resolveWithin` | 与现有 file tools 保持一致的安全边界 |

---

## 2. Architecture

### 2.1 System Context

```
用户配置                          aikota agent                      LSP Server (gopls)
─────────────                   ──────────────                    ──────────────
config.toml                     internal/agenttool/                 (external process)
  [lsp]                         ├── lsp_tool.go                   gopls
    enabled = true              │     ├── LspDefinitionTool         pyright
    auto_detect = true          │     ├── LspReferencesTool         typescript-language-server
    server_path = ""            │     ├── LspSymbolsTool            (用户安装，aikota 不管理)
                                │     └── LspHoverTool
                                │     └── lsp_client.go
                                │           └── Client.Start()    ──stdin/stdout──► JSON-RPC
                                │           └── Client.Call()     ◄──response─────────────
                                │
internal/cli/run/run.go         internal/cli/config/config.go
  ├── BuiltinTools()            └── FileConfig struct
       └── append lsp tools    └── DreamConfig + LspConfig
```

### 2.2 Component Design

#### 2.2.1 `internal/agenttool/lsp_types.go` — LSP 协议类型

```go
package agenttool

// LspLocation 表示代码中的一个位置（文件+行+列）。
type LspLocation struct {
    Uri     string `json:"uri"`
    Line    int    `json:"line"`
    Column  int    `json:"column"`
}

// LspSymbolKind 表示符号类型（参考 LSP 规范）。
type LspSymbolKind int

const (
    SymbolKindFile        LspSymbolKind = 1
    SymbolKindModule      LspSymbolKind = 2
    SymbolKindNamespace   LspSymbolKind = 3
    SymbolKindPackage     LspSymbolKind = 4
    SymbolKindClass       LspSymbolKind = 5
    SymbolKindMethod      LspSymbolKind = 6
    SymbolKindProperty    LspSymbolKind = 7
    SymbolKindField       LspSymbolKind = 8
    SymbolKindConstructor LspSymbolKind = 9
    SymbolKindEnum        LspSymbolKind = 10
    SymbolKindInterface   LspSymbolKind = 11
    SymbolKindFunction    LspSymbolKind = 12
    SymbolKindVariable    LspSymbolKind = 13
    SymbolKindConstant    LspSymbolKind = 14
    SymbolKindString      LspSymbolKind = 15
    SymbolKindNumber      LspSymbolKind = 16
    SymbolKindBoolean     LspSymbolKind = 17
    SymbolKindArray       LspSymbolKind = 18
    SymbolKindObject      LspSymbolKind = 19
    SymbolKindKey         LspSymbolKind = 20
    SymbolKindNull        LspSymbolKind = 21
    SymbolKindEnumMember  LspSymbolKind = 22
    SymbolKindStruct      LspSymbolKind = 23
    SymbolKindEvent       LspSymbolKind = 24
    SymbolKindOperator    LspSymbolKind = 25
    SymbolKindTypeParameter LspSymbolKind = 26
)

// LspSymbol 表示一个代码符号。
type LspSymbol struct {
    Name          string       `json:"name"`
    Kind          LspSymbolKind `json:"kind"`
    Location      LspLocation  `json:"location"`
    ContainerName string       `json:"containerName,omitempty"`
}

// LspRange 表示代码中的一个范围。
type LspRange struct {
    Start LspLocation `json:"start"`
    End   LspLocation `json:"end"`
}

// LspMarkedString 表示带格式的字符串（代码块或纯文本）。
type LspMarkedString struct {
    Language string `json:"language,omitempty"`
    Value    string `json:"value"`
}

// LspHover 表示悬停信息。
type LspHover struct {
    Contents []LspMarkedString `json:"contents"`
    Range    *LspRange         `json:"range,omitempty"`
}
```

#### 2.2.2 `internal/agenttool/lsp_client.go` — LSP JSON-RPC 客户端

```go
package agenttool

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "os/exec"
    "sync"
)

// LspClient 是与单个 LSP server 交互的客户端。
// 每次 Execute 调用启动新进程，调用结束后终止。
type LspClient struct {
    // serverPath 是 LSP server 的可执行文件路径。
    serverPath string
    // args 是启动 server 时的额外参数。
    args []string
    // cmd 是当前运行的进程，nil 表示未运行。
    cmd *exec.Cmd
    // stdin 是写入 server 的管道。
    stdin io.WriteCloser
    // stdout 是读取 server 响应的管道。
    stdout io.ReadCloser
    // scanner 用于逐行读取响应。
    scanner *bufio.Scanner
    // nextID 是递增的请求 ID。
    mu      sync.Mutex
    nextID  int
    // results 是 pending 请求的回调。
    pending map[int]chan json.RawMessage
}

// LspClientOptions 是 LspClient 的构建选项。
type LspClientOptions struct {
    // ServerPath 是 LSP server 路径。为空时尝试自动检测（gopls）。
    ServerPath string
    // Args 是启动时的额外参数。
    Args []string
    // Timeout 是每个请求的超时时间（默认 10s）。
    Timeout interface{ Seconds() float64 }
}

// NewLspClient 创建一个新的 LSP 客户端。
func NewLspClient(opts LspClientOptions) (*LspClient, error) {
    path := opts.ServerPath
    if path == "" {
        // 自动检测 gopls
        var err error
        path, err = lookPath("gopls")
        if err != nil {
            return nil, fmt.Errorf("lsp: gopls not found; set [lsp].server_path in config.toml: %w", err)
        }
    }

    cmd := exec.Command(path, opts.Args...)
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, fmt.Errorf("lsp: cannot create stdin pipe: %w", err)
    }
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, fmt.Errorf("lsp: cannot create stdout pipe: %w", err)
    }

    c := &LspClient{
        serverPath: path,
        args:       opts.Args,
        stdin:      stdin,
        stdout:     stdout,
        scanner:    bufio.NewScanner(stdout),
        pending:    make(map[int]chan json.RawMessage),
    }
    if err := cmd.Start(); err != nil {
        stdin.Close()
        stdout.Close()
        return nil, fmt.Errorf("lsp: cannot start %q: %w", path, err)
    }
    c.cmd = cmd
    return c, nil
}

// Call 发送一个 LSP 请求并等待响应。
// method 是 LSP method 名称（如 "textDocument/definition"）。
// params 是请求参数（JSON 对象）。
func (c *LspClient) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
    id := c.nextID()
    req := map[string]interface{}{
        "jsonrpc": "2.0",
        "method":  method,
        "id":      id,
        "params":  params,
    }
    data, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("lsp: marshal request: %w", err)
    }
    data = append(data, '\n')

    // 发送请求
    if _, err := c.stdin.Write(data); err != nil {
        return nil, fmt.Errorf("lsp: write request: %w", err)
    }

    // 等待响应
    ch := make(chan json.RawMessage, 1)
    c.mu.Lock()
    c.pending[id] = ch
    c.mu.Unlock()
    defer func() {
        c.mu.Lock()
        delete(c.pending, id)
        c.mu.Unlock()
    }()

    select {
    case resp := <-ch:
        return resp, nil
    case <-ctx.Done():
        return nil, fmt.Errorf("lsp: request %s timed out", method)
    }
}

// ReadResponses 读取 server 发来的所有响应（从 stdout 流式读取）。
// 应在单独 goroutine 中运行。
func (c *LspClient) ReadResponses(ctx context.Context) {
    for c.scanner.Scan() {
        line := c.scanner.Bytes()
        if len(line) == 0 {
            continue
        }
        var resp struct {
            ID *int `json:"id"`
        }
        if err := json.Unmarshal(line, &resp); err != nil {
            continue // 忽略无法解析的行
        }
        if resp.ID == nil {
            continue // notifications have no id
        }
        c.mu.Lock()
        ch, ok := c.pending[*resp.ID]
        c.mu.Unlock()
        if !ok {
            continue
        }
        ch <- line
    }
}

// Close 终止 LSP server 进程。
func (c *LspClient) Close() error {
    if c.cmd == nil {
        return nil
    }
    c.stdin.Close()
    c.stdout.Close()
    // 等待最多 2s，然后 kill
    done := make(chan error, 1)
    go func() {
        done <- c.cmd.Wait()
    }()
    select {
    case err := <-done:
        return err
    case <-time.After(2 * time.Second):
        c.cmd.Process.Kill()
        return c.cmd.Wait()
    }
}

// lookPath 类似 exec.LookPath，但在 PATH 中查找。
func lookPath(name string) (string, error) {
    return exec.LookPath(name)
}

// nextID 返回递增的请求 ID。
func (c *LspClient) nextID() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    id := c.nextID
    c.nextID++
    return id
}
```

**关键设计说明：**
- `Call` 方法阻塞等待单个响应，适合简单查询
- `ReadResponses` 在后台 goroutine 中运行，消费所有响应
- 超时通过 context 控制
- 进程关闭时先尝试 graceful，2s 后强制 kill

#### 2.2.3 `internal/agenttool/lsp_tool.go` — 工具实现

```go
package agenttool

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "github.com/Ai-Kota/aikota/internal/agentcore"
)

// LspDefinitionTool 查询符号的定义位置。
type LspDefinitionTool struct {
    Root      string
    ServerPath string
}

func (t *LspDefinitionTool) Name() string { return "lsp_definition" }

func (t *LspDefinitionTool) Description() string {
    return "Find the definition location of a symbol at the given file and position. " +
        "Returns the file path and line/column of the definition."
}

func (t *LspDefinitionTool) Schema() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "file": {"type": "string", "description": "Absolute path to the source file."},
            "line": {"type": "integer", "description": "1-based line number."},
            "column": {"type": "integer", "description": "1-based column number."}
        },
        "required": ["file", "line", "column"],
        "additionalProperties": false
    }`)
}

func (t *LspDefinitionTool) ExecutionMode() agentcore.ToolExecutionMode {
    return agentcore.ToolExecutionParallel
}

func (t *LspDefinitionTool) Execute(ctx context.Context, id string, args json.RawMessage, onUpdate agentcore.ToolUpdateFunc) (agentcore.AgentToolResult, error) {
    var a struct {
        File   string `json:"file"`
        Line   int    `json:"line"`
        Column int    `json:"column"`
    }
    if err := json.Unmarshal(args, &a); err != nil {
        return errorResult("lsp_definition: invalid args: " + err.Error()), nil
    }
    if a.File == "" || a.Line < 1 || a.Column < 1 {
        return errorResult("lsp_definition: file, line, column are required"), nil
    }

    client, err := NewLspClient(LspClientOptions{ServerPath: t.ServerPath})
    if err != nil {
        return errorResult("lsp_definition: " + err.Error()), nil
    }
    defer client.Close()

    // 启动响应读取 goroutine
    go client.ReadResponses(ctx)

    params := map[string]interface{}{
        "textDocument": map[string]string{"uri": "file://" + a.File},
        "position":     map[string]interface{}{"line": a.Line - 1, "character": a.Column - 1},
    }

    resp, err := client.Call(ctx, "textDocument/definition", params)
    if err != nil {
        return errorResult("lsp_definition: " + err.Error()), nil
    }

    // 解析响应
    var locations []LspLocation
    if err := json.Unmarshal(resp, &locations); err != nil {
        // 可能是单个 location 或 LocationLink
        var single LspLocation
        if err2 := json.Unmarshal(resp, &single); err2 == nil {
            locations = []LspLocation{single}
        } else {
            return errorResult("lsp_definition: cannot parse response: " + err.Error()), nil
        }
    }

    if len(locations) == 0 {
        return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent("No definition found.")}}, nil
    }

    var b strings.Builder
    for i, loc := range locations {
        if i > 0 {
            b.WriteString("\n")
        }
        b.WriteString(fmt.Sprintf("%d. %s:%d:%d", i+1, loc.Uri, loc.Line+1, loc.Column+1))
    }
    return agentcore.AgentToolResult{Content: agentcore.ContentList{agentcore.NewTextContent(b.String())}}, nil
}
```

其他三个工具（`LspReferencesTool`、`LspSymbolsTool`、`LspHoverTool`）结构类似，只需替换 method 名称和参数。

#### 2.2.4 `internal/cli/config/config.go` — 配置扩展

```go
// FileConfig 新增字段：
type FileConfig struct {
    // ... 现有字段 ...
    Dream DreamConfig `toml:"dream"`
    Lsp   LspConfig  `toml:"lsp,omitempty"`
}

// LspConfig 是 [lsp] TOML 表。
type LspConfig struct {
    // Enabled 是否启用 LSP 工具。nil = 跟随默认（true）。
    Enabled *bool `toml:"enabled,omitempty"`
    // ServerPath 是 LSP server 的可执行文件路径。为空时自动检测 gopls。
    ServerPath string `toml:"server_path,omitempty"`
}
```

### 2.3 Module Interactions

- **工具注册**：`internal/cli/run/run.go` 的 `BuiltinTools()` 中根据配置决定是否添加 LSP 工具
- **配置加载**：`internal/cli/config/config.go` 解析 `[lsp]` 段
- **进程管理**：每个 Execute 独立启动/关闭 server，无长连接共享状态
- **错误传递**：server 不可用 → encode error in result，不抛 Go error

### 2.4 File Structure

```
internal/agenttool/
├── lsp_tool.go        [NEW: LspDefinitionTool 等四个工具]
├── lsp_tool_test.go   [NEW: 单元测试]
├── lsp_client.go      [NEW: LSP JSON-RPC 客户端]
├── lsp_client_test.go [NEW: 客户端测试]
├── lsp_types.go       [NEW: LSP 协议类型]
internal/cli/config/
├── config.go          [MODIFY: 新增 LspConfig]
internal/cli/run/
├── run.go             [MODIFY: BuiltinTools 根据配置添加 LSP 工具]
```

---

## 3. Data Model

### 3.1 Config Types (`internal/cli/config/config.go`)

```go
type LspConfig struct {
    Enabled    *bool   `toml:"enabled,omitempty"`
    ServerPath string  `toml:"server_path,omitempty"`
}
```

### 3.2 Tool Arguments

**lsp_definition:**
```json
{
  "file": "/absolute/path/to/file.go",
  "line": 10,
  "column": 5
}
```

**lsp_references:**
```json
{
  "file": "/absolute/path/to/file.go",
  "line": 10,
  "column": 5,
  "includeDeclaration": true
}
```

**lsp_symbols:**
```json
{
  "file": "/absolute/path/to/file.go",
  "symbolKind": 12  // 可选，过滤符号类型
}
```

**lsp_hover:**
```json
{
  "file": "/absolute/path/to/file.go",
  "line": 10,
  "column": 5
}
```

### 3.3 Tool Results

所有工具返回 `agentcore.AgentToolResult`，其中 `Content` 是文本格式的 LSP 结果摘要：
- `lsp_definition`：`1. file:///path/to/def.go:20:5\n2. file:///path/to/other.go:30:10`
- `lsp_references`：每行一个位置
- `lsp_symbols`：每行一个符号（格式：`Name (Kind) at line:col`）
- `lsp_hover`：格式化后的文档字符串

---

## 4. Functional Requirements

### FR-1: LSP 工具注册
当 `[lsp].enabled = true` 且 server 可检测到时，`BuiltinTools()` 返回包含 4 个 LSP 工具的工具集。

### FR-2: Server 路径配置
支持三种方式指定 server：
1. `config.toml` 中 `[lsp].server_path = "/path/to/gopls"`
2. 环境变量 `AIKOTA_LSP_SERVER=/path/to/gopls`
3. 自动检测：在 PATH 中查找 `gopls`

### FR-3: Server 不可用时的行为
- server 路径不存在 → 工具不注册（静默跳过）
- server 启动失败 → 返回错误 result，不 panics

### FR-4: 请求超时
单个 LSP 请求默认超时 10s，可通过 context 覆盖。

### FR-5: 路径校验
所有 `file` 参数通过 `resolveWithin` 校验，确保在 workspace 内。

---

## 5. Non-Goals

- **多语言自动探测**：初始仅支持 Go (gopls)，其他语言需手动配置 server_path
- **增量同步**：不实现 `textDocument/didChange` 等通知
- **Workspace 配置**：不读取 `.vscode/settings.json` 等编辑器配置
- **长连接**：每个 Execute 启动新进程，不维护持久连接
- **自定义 LSP methods**：仅实现标准 LSP methods（definition、references、symbols、hover）

---

## 6. Testing Strategy

### 6.1 Unit Tests

**lsp_client_test.go:**
- `TestLspClient_StartAndStop` — 验证进程生命周期
- `TestLspClient_CallTimeout` — 验证超时处理
- `TestLspClient_ServerNotFound` — 验证 server 不可用时的错误

**lsp_tool_test.go:**
- `TestLspDefinitionTool_Name` — 验证工具名
- `TestLspDefinitionTool_Schema` — 验证 JSON Schema
- `TestLspDefinitionTool_Execute` — 端到端测试（mock server）

### 6.2 Integration Tests

- 使用 `httptest` 风格的 mock LSP server（通过 stdin/stdout 通信）
- 验证请求/响应格式正确
- 验证错误处理路径

### 6.3 Golden Tests

- 准备固定输入，验证输出格式匹配 golden 文件
- golden 文件位置：`internal/agenttool/testdata/lsp/`

---

## 7. Implementation Plan

### Phase 1: 基础框架 (1 天)
1. 创建 `internal/agenttool/lsp_types.go`
2. 创建 `internal/agenttool/lsp_client.go`（基础实现）
3. 编写 `lsp_client_test.go`（mock server）

### Phase 2: 工具实现 (1 天)
1. 创建 `internal/agenttool/lsp_tool.go`
2. 实现四个工具
3. 编写 `lsp_tool_test.go`

### Phase 3: 配置集成 (0.5 天)
1. 修改 `internal/cli/config/config.go`
2. 修改 `internal/cli/run/run.go`
3. 集成测试

### Phase 4: 文档和测试 (0.5 天)
1. 编写 golden tests
2. 更新 README
3. 更新 development-plan

**总计：3 天**

---

## 8. References

- LSP Specification: https://microsoft.github.io/language-server-protocol/
- gopls: https://github.com/golang/tools/tree/master/gopls
- JSON-RPC 2.0: https://www.jsonrpc.org/specification

---

*Generated: 2026-10-01*
*Next: 实现 Phase 4.1 LSP Tool*
