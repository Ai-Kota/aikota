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

### 第 1 层 · 最高杠杆，且**只有 aikota 能做**（护城河）

| 项 | 现状 | 挂载点 |
|---|---|---|
| **内建安全底线** | ✅ 完成 | `internal/safetygate`，装在 `run.NewConfig` 的 `BeforeToolCall` 基底——**CLI 与 SDK 两种模式同时覆盖**，`--approve` 不可绕过 |
| **ai-guard 策略外挂** | ⬜ | 走 hook 层接 SPT 的策略检查（`check-scope` 等，依赖 SPT 目录结构）|
| **NATS 原生事件出口** | ⬜ **0** | `internal/runtime` 事件发射点的装饰器 |
| **FreeLLMAPI 策略内嵌** | ⬜ **0** | `internal/provider` 层——路由与成本策略 |

**这一层是 aikota 存在的理由。** 内建安全底线已落地（`--approve` 不再等于裸奔）；其余三项是 0，而不是"待优化"。

### 第 2 层 · 高杠杆，**可从外部借鉴设计**（借设计，不借代码）

| 项 | 现状 | 借鉴对象 | 价值 |
|---|---|---|---|
| **RepoMap** | ⬜ | openclaude `RepoMapTool`：*"Build a structural map of the repository showing **ranked files and their key signatures** (functions, classes, types, interfaces)"* | **上下文质量的最大单点提升**——让模型先看到骨架，而非靠 grep 摸黑 |
| **LSP** | ⬜ | openclaude `LSPTool` | 从文本匹配升级到**语义理解**（真实定义/引用，而非正则） |
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

---

## 附录：证据来源

| 结论 | 依据 |
|---|---|
| openclaude 规模与工具数 | 实测：`find src -name '*.ts' -o -name '*.tsx'` → 3,191 文件 / 169,763 行；`ls src/tools/` |
| openclaude 许可约束 | 直接读取该仓库 `LICENSE` 与 `README.md` §"Claude" |
| aikota 工具与能力现状 | `internal/cli/run/run.go` 的 `BuiltinTools`；`internal/` 各包实测 |
| 第 1 层挂载点 | `internal/agenttool/tool_executor.go`（`BeforeToolCall`）、`internal/runtime`（事件发射）、`internal/provider`（路由） |

> 本文的"片面"判断基于上述实测，非对任何项目整体优劣的评价。
