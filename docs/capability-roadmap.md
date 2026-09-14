# aikota 能力路线（Capability Roadmap）

> 状态：living document（活文档）。本文是 aikota 的**定位基线**，回答一个问题：
> **aikota 往哪个方向长，才叫"更强大"而不是"更臃肿"。**
>
> 关联文档：
> - 对标基线：`docs/harness-capability-matrix.md`（harness 六维度现状与缺口）
> - pi 源码映射：`tasks/research-pi-agent-loop-go-mapping.md`（TS→Go 控制流与钩子契约）
>
> 更新方式：每落地一层，回到 §4 改对应行的"现状"。**不新增章节，只更新状态。**

---

## 0. 定位

aikota 的定位是**本机主力的、可定制的 AI Agent**。

需要说清楚的是它的**存在理由**：

> 不是"像 Claude Code"，而是**能与自有基础设施原生耦合**。

这一点决定了后面所有取舍——凡是 Claude Code 与同类产品**结构上不可能有**的能力，才是 aikota 的护城河；凡是它们都有的能力，aikota 只是补课。

---

## 1. 事实基线

| | aikota | openclaude |
|---|---|---|
| 语言 / 规模 | Go，**~40,000 行** | TypeScript，**169,763 行 / 3,191 文件** |
| 内置工具 | **~15** | **~54** |
| 许可 | **MIT（干净，无第三方专有代码）** | ⚠️ **派生于专有产品，见 §1.1** |
| 出身 | 从 pi 移植（MIT） | 从 Claude Code 派生 |

参照项目位置：`AImlyForge/another/agent/openclaude/`（同目录下另有 `pi/`、`hermes-agent/`、`grok-build/` 等）。

### 1.1 openclaude 的许可约束 ⚠️

**这是一个必须先摆到台面上的事实。** 其 `LICENSE` 首段原文：

```
This repository contains code derived from Anthropic's Claude Code CLI.

The original Claude Code source is proprietary software:
  Copyright (c) Anthropic PBC. All rights reserved.
  Subject to Anthropic's Commercial Terms of Service.

Modifications and additions by OpenClaude contributors are offered under
the MIT License where legally permissible
```

README §"Claude" 亦写明：*"MIT for OpenClaude contributors' modifications; **the derived Claude Code remains Anthropic's**."*

⇒ **它声明 MIT，但只覆盖贡献者新写的部分；底座是 Anthropic 的专有代码。**

对 aikota 的约束：

| 借鉴方式 | 判定 |
|---|---|
| **借鉴设计、交互范式、prompt 思路** | ✅ 安全——著作权不保护思想 |
| **复制代码** | ⛔ **禁止**——无法分辨哪一行属于 Anthropic |

aikota 当前是 MIT 干净链条（pigo ↔ pi 均为 MIT）。**不要让这条链被污染。**

---

## 2. 第一性原理：什么决定 agent 的能力上限

```
产出质量 ≈ f(模型能力, 上下文质量, 工具能力, 验证强度, 可恢复性)
```

关键不在因子清单，而在**边际收益曲线**：

| 因子 | 边际收益 | 谁控制 |
|---|---|---|
| 模型能力 | 外部给定，只能选型 | ❌ |
| **上下文质量** | **持续上升，不饱和** | ✅ 完全可控 |
| **工具能力** | **快速饱和**——编码 agent 的核心需求约 10 个工具即覆盖 | ✅ 可控 |
| **验证强度** | **持续上升，且被普遍低估** | ✅ 完全可控 |
| 可恢复性 | 决定"敢不敢放手自动跑" | ✅ 可控 |

**本路线的支点结论：**

> **工具数会饱和；上下文质量和验证强度不会。**

证据就在 openclaude 自身：它提供 54 个工具，但 `TungstenTool`、`SyntheticOutputTool`、`SnipTool` 之类对多数工作的边际价值≈0；而 `RepoMapTool`（改变**模型看到什么**）属于另一个数量级的杠杆。

---

## 3. 四个需要警惕的片面

### 3.1 "更强大" ≠ "更多工具"

从 15 个工具加到 54 个，**能力几乎不增长**，增加的是：维护面、测试面、安全面，以及**与上游 pigo 的分叉深度**。

aikota 是 fork——**diff 越厚，越难跟上上游**。这是隐性成本，且随时间复利。

### 3.2 借鉴源选低了一档，且带法律风险

openclaude 本身是 Claude Code 的派生物。因此"从 openclaude 借 Claude Code 的能力"是**二手借鉴**。

- **真正的源头是 Claude Code 本身**——本机就在用，行为可直接观察，无需经过一个专有代码的派生品
- 若确需从 openclaude 取经，**只取设计，不取代码**（§1.1）

### 3.3 忽略了 aikota 唯一的护城河

```
aikota 的差异化 = 与自有基础设施的原生耦合
   ├─ NATS 总线         → 在现有编排体系里可见、可编排
   ├─ FreeLLMAPI        → 路由与成本归自己控制
   └─ ai-guard / SPT    → 自有约束能在循环内强制
```

**这三样，Claude Code 与 openclaude 结构上不可能有。**

若补课补成一个"弱化版 Claude Code"，aikota 就失去了存在理由。

### 3.4 "主力"的真实约束是信任，不是功能

"主力"意味着会把真活交给它、会放手让它自动跑。那要求的是**工程属性**：

| 需要 | aikota 现状 |
|---|---|
| 可靠（崩溃可恢复、错误可诊断） | 🟡 双平台 CI 已补（`09c50d6`）；崩溃恢复未验证 |
| 可观测（做了什么、花了多少） | 🟡 有 telemetry 事件，未接入自有系统 |
| 可中断 / 可回退 | ✅ rewind（file_snapshot）+ session fork 已有 |
| 成本可控 | ❌ 无 |

**这些是"主力"的地基，优先级高于再添 40 个工具。**

---

## 4. 按杠杆排序的路线

> 排序依据是**杠杆**（每单位投入带来的能力增量），不是实现难度或功能热度。

### 第 0 层 · 前提（不做则上面全白搭）

让 aikota 具备"可以被信任地放手"的工程属性。

| 项 | 现状 | 说明 |
|---|---|---|
| 双平台 CI | ✅ 完成 | 见 `.github/workflows/ci.yml` |
| 崩溃恢复 | ⬜ | 进程被杀后会话/工作区是否可恢复 |
| 错误可诊断 | ⬜ | 失败时能否定位到"哪一步、为什么" |
| 成本可见 | ⬜ | token 用量已采集（telemetry），未暴露给使用者 |
| 延迟可测 | ⬜ | 启动与首 token 延迟 |

### 第 1 层 · 最高杠杆（aikota 与自有基础设施的接口）

> **归属原则（见 §5.5）：通用能力进产品，基础设施耦合留外面。**
> 本层的每一项都必须以**通用机制**的形式落在 aikota，而不是把某个具体基础设施写进去。

| 项 | 现状 | 挂载点 |
|---|---|---|
| **内建安全底线** | ✅ 完成 | `internal/safetygate`，装在 `run.NewConfig` 的 `BeforeToolCall` 基底——**CLI 与 SDK 两种模式同时覆盖**，`--approve` 不可绕过 |
| **通用事件流出口** | ✅ 已有 | 机器可读的事件流：CLI 侧 `--output-format stream-json`（每个 `AgentEvent` 一行 JSON），SDK 侧 `Session.StreamEvents`。**这就是 aikota 提供的事件接口** |
| **总线接入（NATS 等）** | ⬜ | **不在 aikota 内做**：由 AImlyForge 侧一个 bridge 消费上面的通用事件流，映射到 `aimly.*` 主题。aikota 不认识 NATS |
| **ai-guard 策略外挂** | ⬜ | 走 aikota 的 hook 层接 SPT 的策略检查（`check-scope` 等，依赖 SPT 目录结构）|
| ~~模型网关策略内嵌~~ | ❌ **不做** | 路由策略在 `tools/free-tier-aggregator`（Go 网关）里；aikota 只用 `--base-url` + `--protocol` 接它的标准端点，**零改动**。见 §4.1 |

### 4.1 模型网关：为什么不做内嵌（例证 §5.5）

`tools/free-tier-aggregator`（"Free Tier Proxy"）是你的 Go 路由网关，暴露**标准端点**：

| 端点 | 协议 | aikota 接法 |
|---|---|---|
| `/v1/chat/completions` | OpenAI 兼容 | `--base-url http://<网关>/v1 --protocol openai` |
| `/v1/messages` | Anthropic Messages | `--base-url http://<网关>/v1 --protocol anthropic` |
| `/v1/models` · `/health` | — | 用于发现与探活 |

aikota 的拼接方式（`baseURL` + `/chat/completions` 或 `/messages`）**与网关路径正好对齐**，所以两边都不需要改。

**免费优先 / 多 key 轮换 / 冷却 / 兜底链**这些策略属于**网关**（你的基础设施），不属于 aikota（通用产品）。这是 §5.5 判据的正例：把网关那行代码给一个不用它的用户，毫无意义。

> ⚠️ **端口冲突（实测）**：网关 README 写 `:8462`，但该端口当前被 `master-loop.exe` 占用。用之前需改端口或调整 master-loop。

**这一层是 aikota 与你的体系对接的地方。** 内建安全底线已落地；事件流出口已存在；其余待做。

### 第 2 层 · 高杠杆，**可从外部借鉴设计**（借设计，不借代码）

| 项 | 现状 | 借鉴对象 | 价值 |
|---|---|---|---|
| **RepoMap** | ✅ 完成 | openclaude `RepoMapTool`（借设计） | `internal/repomap` + `repo_map` 工具。**实现对 Go 是净简化**：openclaude 用 tree-sitter（C 库），aikota 用标准库 `go/parser`+`go/ast`——**零依赖、零 CGO**。算法保真：IDF 加权 + PageRank(α=0.85) + 焦点提升 + token 预算渲染。实测 aikota 自身：412 文件 / 103ms，排名首位是 `internal/agentcore/content.go`（所有包依赖的核心类型）|
| **LSP** | ⬜ | openclaude `LSPTool` | 从文本匹配升级到**语义理解**（真实定义/引用，而非正则）。需外部 LSP server → 按 §5.5 需先判定归属 |
| **验证闭环** | ⬜ | 无成熟参照（openclaude 亦弱） | 改动后自动跑测试/lint 并回喂结果——**"敢放手"的关键** |

### 第 3 层 · 锦上添花（按需，不建议一次全上）

MCP 客户端 · plan mode · worktree 隔离 · notebook 编辑 · 多 agent team

> 这一层功能最多、参照最全、**也最容易上瘾**。它是 3.1 说的"饱和区"——投入产出比最低。

---

## 5. 执行纪律

1. **薄 diff**：改动尽量落在 pigo 既有的可选钩子 seam 上；新代码进独立包。分叉越薄，越能跟上上游。
2. **借设计不借代码**：尤其不要从任何派生于专有产品的仓库取码（§1.1）。
3. **工具数不是目标**：新增工具前先问"它改变模型看到的东西，还是只增加一个动作"。后者多半属于第 3 层。
4. **先做只有自己能做的**：第 1 层是身份，第 3 层是补课。顺序反了，得到的是功能很多但自己都不放心交付的产品。
5. **通用能力进产品，基础设施耦合留外面** ⚠️

   aikota 是**通用 agent**。任何"某个具体基础设施"的知识——某条消息总线的客户端库、某个网关的认证方式、某套部署的路径——**都不进 aikota**。产品只提供**通用机制**，具体基础设施在外部对接。

   | 想做的事 | ❌ 错误做法 | ✅ 正确做法 |
   |---|---|---|
   | 让运行过程在编排层可见 | 在 aikota 里引入 NATS 客户端，硬编主题 | aikota 提供通用事件流（已有 `stream-json` / `StreamEvents`），AImlyForge 侧写 bridge 消费它 |
   | 让 aikota 用你的模型网关 | 把网关的选模/兜底策略编进 aikota | 先用 aikota 的通用能力（`--base-url` + 协议）；只有当"网关客户端"本身是通用能力时，才进 `internal/provider` |

   **判据**：把这行代码给一个**不用你基础设施**的用户，它还有意义吗？
   - 有 → 通用能力，可以进产品
   - 没有 → 你的基础设施耦合，留外面

   > 这条是实践中踩出来的：曾计划把 NATS 客户端直接加进 aikota（`go get nats.go` + 在事件发射点硬编发布）。依赖已撤。**症状**：需求描述里出现"**原生**"两个字时，先停一下问是哪一层的"原生"。

### 5.5.1 已判定案例（三次独立判断，同一结论）

| # | 东西 | 性质 | 曾被考虑的合并方式 | 判定 | 正确形态 |
|---|---|---|---|---|---|
| 1 | **ai-guard** | 你的 AI 约束工具（SPT）| 内置进 aikota 当闸门 | ❌ 不内置 | 走 aikota 的 hook 层外挂；aikota 自己只做**通用安全底线** |
| 2 | **NATS** | 你的消息总线 | `go get nats.go` + 事件发射点硬编发布 | ❌ 不硬编 | aikota 提供**通用事件流**（已有 `stream-json` / `StreamEvents`），AImlyForge 侧写 bridge 消费 |
| 3 | **free-tier-aggregator** | 你的模型路由网关 | 合并进 aikota 作为一个模块 | ❌ 不合并 | **标准协议对接，零代码**（`--base-url` + `--protocol`）|

**三例的共同结构**：

- 都是**只有你才有的基础设施**——别人没有 40+ 免费 key、没有 SPT 目录结构、没有你的总线
- 都对 aikota 有"要用上"的合理诉求
- 三次独立判断都指向同一结论：**aikota 只提供通用机制，耦合留在外面**

### 5.5.2 下次遇到"要不要把 X 合进 aikota"时的判定程序

```
1. 把 X 的那行代码给一个【不用你基础设施】的用户 —— 还有意义吗？
     没有 → 停止。它是你的基础设施，不进产品。
     有   → 进入第 2 步

2. X 和 aikota 的【部署形态 / 生命周期】一样吗？
     不一样（如：常驻服务 vs 一次性 CLI）→ 停止。不是模块关系。
     一样 → 进入第 3 步

3. X 和 aikota 的【演化节奏】一样吗？
     不一样（跟上游 vs 跟你的流程/配置）→ 停止。各自独立演进，用协议对接。
     一样 → 可以考虑合并。
```

> **⚠️ 特别警告**：aikota 是 pigo 的 fork。任何合并都会**加深与上游的分叉**（§5.1 薄 diff 纪律）。合并不是免费的——它按时间复利计息。

---

## 附录：证据来源

| 结论 | 依据 |
|---|---|
| openclaude 规模与工具数 | 实测：`find src -name '*.ts' -o -name '*.tsx'` → 3,191 文件 / 169,763 行；`ls src/tools/` |
| openclaude 许可约束 | 直接读取该仓库 `LICENSE` 与 `README.md` §"Claude" |
| aikota 工具与能力现状 | `internal/cli/run/run.go` 的 `BuiltinTools`；`internal/` 各包实测 |
| 第 1 层挂载点 | `internal/agenttool/tool_executor.go`（`BeforeToolCall`）、`internal/runtime`（事件发射）、`internal/provider`（路由） |
| ai-guard 的性质与协议 | `SPT/docs/architecture/modules/ai-guard.md`；子命令实测（`ai-guard --help`）；`scripts/hook.sh` 的 exit 0/2 语义注释 |
| 网关暴露的标准端点 | `tools/free-tier-aggregator/README.md` §API 端点；`docs/decisions/ADR-003-positioning.md` |
| 网关未运行 + 端口冲突 | 实测：`tasklist` 无 `free-tier-proxy`；`netstat` 显示 `:8462` 由 `master-loop.exe` 持有 |
| NATS 主题与信封约定 | `deploy/nats/NEURAL_BACKBONE.md` §4；`tools/_shared/natsutil` 的 `EnvelopeMeta` |

> 本文的"片面"判断基于上述实测，非对任何项目整体优劣的评价。
