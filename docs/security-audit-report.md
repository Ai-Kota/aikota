# aikota 安全审计报告

> 审计日期：2026-09-24
> 审计范围：完整代码库安全扫描
> 审计方法：静态代码分析、数据流追踪、网络通信审查

---

## 执行摘要

aikota 作为本地运行的 AI Agent 工具，整体安全架构设计良好，但存在**一个关键安全隐患**需要立即修复。

| 风险等级 | 数量 | 说明 |
|---------|------|------|
| 🔴 严重 | 1 | 插件系统未清洗环境变量 |
| 🟡 中等 | 2 | Hook 系统环境继承、插件无输入验证 |
| 🟢 低 | 3 | 日志可能包含敏感信息 |
| ✅ 安全 | - | 核心功能无后门 |

---

## 一、未发现的行为

### 1.1 无隐蔽后门

经过全面代码审查，**aikota 不存在任何故意植入的后门或恶意代码**。代码开源、可审计，未发现：
- 硬编码的远端回调地址
- 隐蔽的数据外传机制
- 未文档化的网络连接
- 恶意代码注入点

### 1.2 网络通信透明

所有网络请求均指向官方 API 服务：

| 用途 | 目标地址 | 状态 |
|------|---------|------|
| 模型提供商 | api.anthropic.com, api.openai.com 等 | ✅ 正常 |
| 搜索引擎 | api.tavily.com, api.search.brave.com | ✅ 正常 |
| 自更新检查 | github.com/Ai-Kota/aikota | ✅ 正常 |
| 本地通信 | 127.0.0.1 (LAN) | ✅ 正常 |

### 1.3 遥测数据本地化

`internal/runtime/telemetry.go` 中的遥测功能仅收集：
- 工具调用次数和耗时
- 上下文 token 使用量
- 压缩事件计数

**这些数据仅输出到 stdout/stderr（供脚本消费），不会发送到任何外部服务器。**

---

## 二、发现的安全隐患

### 🔴 P0：插件系统环境变量泄漏

**位置**：`internal/plugin/plugin.go:40-46`

```go
func Load(command string, args []string, stderr io.Writer) (*Plugin, error) {
    client, err := jsonrpc.NewClient(jsonrpc.Config{
        Command: command,
        Args:    args,
        Stderr:  stderr,
        // ⚠️ Env 未设置，默认为 nil
    })
```

**问题**：
- `jsonrpc.Config.Env` 为 nil，子进程**继承父进程完整环境变量**
- 插件可以读取所有环境变量，包括：
  - `ANTHROPIC_API_KEY`
  - `OPENAI_API_KEY`
  - `DEEPSEEK_API_KEY`
  - `AIKOTA_HOME`
  - 其他敏感变量（SSH keys、数据库凭证等）

**影响**：
- 恶意插件可窃取 API 密钥
- 用户安装的第三方插件无法信任

**修复建议**：
```go
// 修复方案：使用 ScrubEnv 清洗环境变量
env := provider.ScrubEnv(os.Environ(), nil)
client, err := jsonrpc.NewClient(jsonrpc.Config{
    Command: command,
    Args:    args,
    Env:     env,  // 传入清洗后的环境
    Stderr:  stderr,
})
```

---

### 🟡 P1：Hook 系统环境变量继承

**位置**：`internal/hooks/runner.go:124-130`

```go
func (r *Runner) env(input HookInput) []string {
    env := append([]string(nil), os.Environ()...)  // ⚠️ 继承完整环境
    env = append(env,
        "AIKOTA_SESSION_ID="+input.SessionID,
        "AIKOTA_PROJECT_DIR="+r.ProjectDir,
        "AIKOTA_EVENT_TYPE="+input.EventType,
    )
    return env
}
```

**问题**：Hook 命令同样继承完整环境变量，包括 API 密钥。

**影响**：用户配置的自定义 Hook 可以访问所有敏感环境变量。

**修复建议**：
```go
func (r *Runner) env(input HookInput) []string {
    env := provider.ScrubEnv(os.Environ(), nil)  // 清洗环境变量
    env = append(env,
        "AIKOTA_SESSION_ID="+input.SessionID,
        "AIKOTA_PROJECT_DIR="+r.ProjectDir,
        "AIKOTA_EVENT_TYPE="+input.EventType,
    )
    return env
}
```

---

### 🟡 P2：插件输入无验证

**位置**：`internal/plugin/plugin.go:96-103`

```go
func (p *Plugin) call(ctx context.Context, name string, args json.RawMessage) (CallResult, error) {
    raw, err := p.client.Call(ctx, "tools/call", CallParams{Name: name, Arguments: args})
```

**问题**：
- `args` 直接传递给插件，未经任何验证
- 恶意插件可能利用注入攻击
- 未限制 `args` 大小，可能导致 DoS

**影响**：低（需要模型配合生成恶意参数）

---

### 🟢 P3：日志可能包含敏感信息

**位置**：多处错误处理

```go
return nil, fmt.Errorf("plugin: launch %q: %w", command, err)
```

**问题**：虽然凭证值本身不记录，但路径和配置信息可能在错误日志中暴露。

**影响**：低（需要访问日志文件）

---

## 三、安全措施验证

### 3.1 凭证清洗机制 ✅

`internal/provider/credenv.go` 实现了完善的环境变量清洗：

```go
var credentialNameSuffixes = []string{
    "_API_KEY", "_APIKEY", "_AUTH", "_CREDENTIALS",
}
var credentialNameSubstrings = []string{
    "PASSWORD", "CREDENTIAL", "PRIVATE_KEY", "SECRET", "TOKEN",
}
```

**子 Agent 正确使用此机制**：
```go
// internal/runtime/subagent.go:419
env = provider.ScrubEnv(os.Environ(), nil)
```

### 3.2 自更新安全验证 ✅

`internal/selfupdate/update.go` 实现安全的二进制更新：

- SHA256 校验和验证
- 原子性文件替换（先写临时文件，再 rename）
- 失败回滚保护

### 3.3 安全闸门（SafetyGate）✅

`internal/safetygate/` 实现了命令拦截：
- 阻止 `rm -rf /`、`mkfs`、`dd if=/dev/zero` 等危险命令
- 阻止权限提升（`sudo`）
- 阻止管道到 shell（`curl | sh`）

### 3.4 信任系统 ✅

`internal/trust/` 实现目录信任管理：
- 三态信任（未信任/信任/已批准）
- 破坏性操作需要确认
- `--approve` 只能跳过信任提示，不能绕过 SafetyGate

---

## 四、数据流分析

### 4.1 敏感数据流向

| 数据类型 | 存储位置 | 是否加密 | 外传风险 |
|---------|---------|---------|---------|
| API Key | `$AIKOTA_HOME/.credentials.yaml` (mode 0600) | 否（明文存储） | 中（需本地访问） |
| 会话历史 | `$AIKOTA_HOME/sessions/` | 否 | 低 |
| 记忆文件 | `$AIKOTA_HOME/memory/` | 否 | 低 |
| 遥测数据 | stdout/stderr | N/A | 无（仅本地） |

### 4.2 环境变量传播图

```
父进程环境变量
    │
    ├──→ 子 Agent ✅ (已清洗)
    │
    ├──→ Hook 命令 ⚠️ (未清洗)
    │
    └──→ 插件 ⚠️ (未清洗)
```

---

## 五、修复建议

### 立即修复（P0）

修改 `internal/plugin/plugin.go`：

```go
import (
    // ... 现有导入 ...
    "github.com/Ai-Kota/aikota/internal/provider"
)

func Load(command string, args []string, stderr io.Writer) (*Plugin, error) {
    // 清洗环境变量，防止凭证泄漏
    env := provider.ScrubEnv(os.Environ(), nil)
    
    client, err := jsonrpc.NewClient(jsonrpc.Config{
        Command: command,
        Args:    args,
        Env:     env,  // 使用清洗后的环境
        Stderr:  stderr,
    })
    // ... 其余代码不变
}
```

### 建议修复（P1）

修改 `internal/hooks/runner.go`：

```go
import (
    // ... 现有导入 ...
    "github.com/Ai-Kota/aikota/internal/provider"
)

func (r *Runner) env(input HookInput) []string {
    // 清洗环境变量
    env := provider.ScrubEnv(os.Environ(), nil)
    env = append(env,
        "AIKOTA_SESSION_ID="+input.SessionID,
        "AIKOTA_PROJECT_DIR="+r.ProjectDir,
        "AIKOTA_EVENT_TYPE="+input.EventType,
    )
    return env
}
```

---

## 六、结论

aikota **不存在故意植入的后门或恶意行为**，核心架构安全设计良好。但存在**两个环境影响变量泄漏的漏洞**需要修复：

1. **插件系统未清洗环境变量**（严重）
2. **Hook 系统未清洗环境变量**（中等）

这些问题属于**设计疏忽**而非恶意植入，修复后安全性将大幅提升。

---

*报告生成时间：2026-09-24*
*审计工具：静态代码分析 + 数据流追踪*
