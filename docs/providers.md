# Provider 层：架构与内部机制

> **本文讲"内部怎么实现的"**：驱动分层、端点拼接、传输重试、失败模型。
> **"我该写哪个参数"** 属于用户视角，见 README 的[模型与 Provider](../README.md#模型与-provider)（解析链、按模型名推断、凭证引用、示例）。两处刻意不重复。

---

## 1. 概览：38 个预设，3 个驱动

`internal/provider/registry.go` 有 **38 个 provider 条目**，`presets.go` 有 **~114 条模型预设**——但它们**全部塌缩到三个驱动**：

```
38 provider 条目 ──┐
114 模型预设     ──┼──→  openAICompatDriver     (绝大多数)
（只有认证与端点不同）│     anthropicCompatDriver   (Anthropic 原生 + Bedrock)
                      └──→  responsesDriver        (OpenAI Responses API)
```

**这个塌缩是整个 provider 层的设计核心**：新增一个 OpenAI 兼容网关，是**加一条配置**，不是写一个客户端。

### 三个驱动

| 驱动 | 线路格式 | 端点 | 解码器 | 代表 provider |
|---|---|---|---|---|
| `openAICompatDriver` | OpenAI Chat Completions | `{baseURL}/chat/completions` | `OpenAIDecoder`（自写 SSE） | openrouter · ollama · nvidia · deepseek · groq · xai · 大多数 |
| `anthropicCompatDriver` | Anthropic Messages | `{baseURL}/messages` | `AnthropicDecoder` | anthropic · amazon-bedrock |
| `responsesDriver` | OpenAI Responses API | 经官方 `openai-go` SDK | — | azure-openai-responses |

### 认证方案的枚举

`ProviderSpec.AuthScheme` 取值：`bearer` · `x-api-key` · `aws` · `azure` · `special`。

> **路径可覆盖**：`anthropicCompatDriver` 的端点路径是函数 `pathFor(model)` 而非常量——因为 **Bedrock 把 model id 编进路径**（`/model/{id}/invoke`），写死会坏。

---

## 2. 端点拼接规则（⚠️ 最常踩的坑）

两个驱动都是**朴素的字符串拼接**，不做路径归一化：

```go
openAICompatDriver:     baseURL + "/chat/completions"
anthropicCompatDriver:  baseURL + "/messages"
```

### ⇒ `--base-url` 必须带 `/v1`

```bash
# ✅ 正确
--base-url https://api.deepseek.com/anthropic/v1

# ❌ 错误：会打到 https://api.deepseek.com/anthropic/chat/completions（404）
--base-url https://api.deepseek.com/anthropic
```

**填错的症状很具体**：如果对方在那个路径上挂了 Web 控制台，你会收到一页 **HTML** 而不是 SSE——表现为流式响应没有任何内容，或者一个语焉不详的解析错误。

> 这条坑与 pi 的**完全相同**（拼接方式一致），所以你在 pi 上踩过的经验可以直接沿用。

---

## 3. 认证

### 解析顺序

```
OAuth token（若该 provider 注册了；自动刷新，提前 30s 视为过期）
    ↓ 没有
--api-key 显式覆盖
    ↓ 没有
环境变量 <PROVIDER>_API_KEY（registry 的 EnvVars 按优先级顺序取第一个非空）
    ↓ 没有
配置文件
```

**额外的 base URL 覆盖**：`ProviderSpec.BaseURLEnvVars` 可为单个 provider 声明专用环境变量（如 `AZURE_OPENAI_BASE_URL`）；通用的 `<PROVIDER>_BASE_URL` 约定由调用方处理。

### 两条硬约束

- **凭证从不进日志或错误文本**——错误里只出现 provider 名，绝不出现 key
- **子进程默认继承已清洗的环境**——进程隔离子 Agent 拿不到 `*_API_KEY` / `*_TOKEN` / `*_SECRET` 形态的变量，也拿不到 `AIKOTA_*` 内部变量

---

## 4. 传输层

所有驱动最终都交给同一个 `StreamRequest`，重试与看门狗在这一层统一实现。

### 4.1 连接期重试

| 项 | 值 |
|---|---|
| 可重试状态码 | **429** · **503** · **529** |
| 退避 | 尊重 `Retry-After`；无该头则指数退避 |
| 默认次数 | **2**（`TransportConfig.MaxConnectRetries` 可覆盖） |

> **529 是 Cloudflare 的"站点过载"**，有些上游也会发，所以与 429/503 同级对待。

### 4.2 绝不重放已消费的流

重试**只发生在连接期**——即响应字节尚未吐出之前。

**流一旦开始，失败就作为终态错误事件骑在流上**，不做不透明重试。原因是：重放一个已经开始流式返回的请求，会让**工具调用被执行两次**。宁可让这一轮失败，也不造成副作用重复。

### 4.3 双看门狗

| 看门狗 | 默认 | 作用 |
|---|---|---|
| 空闲超时 | **5 分钟**（`AIKOTA_STREAM_IDLE_TIMEOUT` 可改） | 完全没数据可读 |
| **内容停滞** | 空闲 × **1.2** | 有数据但**没有实质内容** |

**第二个是必要的**：有些上游会持续发送保活/心跳字节，让空闲看门狗永远不触发，但实际一个 token 都不产出。按"空闲 × 1.2"给停滞留一点余量，既杀掉假活跃，又不误伤慢速但确实在推进的流。

### 4.4 双失败模型 ⭐

```go
// 只有"连流都建不起来"才 return error：
//    缺 API key、请求体构造失败
//
// 所有运行时失败都骑在返回的流上，作为终态错误事件
//    （HTTP 4xx/5xx、超时、上游错误体、解析失败……）
```

**这带来一个重要的调用方契约**：调用方**只需处理一个通道**，不用同时判断 error 返回值和流内容。

**错误体截断到 4096 字节**——错误响应本身可能是巨大的 HTML，不设上限会污染上下文。

---

## 5. 编码细节

### Anthropic：同一轮的多个工具结果必须合并

一轮里调用了多个工具时，**所有 `tool_result` 必须在紧跟 assistant 的**同一条** user 消息里**：

```
{role: assistant, content: [tool_use#1, tool_use#2]}
{role: user, content: [tool_result#1, tool_result#2]}   ← 一条，两个 block
```

发成两条独立消息会被 API 直接拒绝：

```
messages.1: `tool_use` ids were found without `tool_result` blocks
immediately after: ...
```

`encodeAnthropicRequest` 会合并**相邻**的工具结果；且**只允许"纯 tool_result"的消息吸收新结果**——否则会把结果挂到普通 user 轮次上，在 API 找不到答案的位置再犯同一个错。

> **OpenAI 线路无此约束**（逐条 `role:"tool"` 正是它的原生格式），所以这个合并只作用于 Anthropic 路径。

### 推理强度映射

统一的 `ThinkingLevel`（`off`…`max`）按线路各自翻译：

| 线路 | 字段 | 映射 |
|---|---|---|
| OpenAI Chat Completions | `reasoning_effort` | minimal/low/medium/high；xhigh、max → high |
| OpenAI Responses | `reasoning.effort` | minimal → low；medium → medium；high/xhigh/max → high；并强制 `summary: auto` |
| Anthropic Messages | `thinking.budget_tokens` | minimal=1024 … max=65536；同时把 `max_tokens` 抬到预算之上 +4096 余量 |

---

## 6. 相关文档

| 文档 | 讲什么 |
|---|---|
| README [模型与 Provider](../README.md#模型与-provider) | **用户视角**：解析链与优先级、按模型名推断的对照表、凭证引用、使用示例 |
| `internal/provider/registry.go` | provider 注册表（38 条，含 EnvVars / 默认端点 / 协议 / 认证方案） |
| `internal/provider/presets.go` | 模型预设目录（~114 条），按 provider 分组 |
| `internal/provider/transport.go` | 重试、双看门狗、双失败模型的实现 |
