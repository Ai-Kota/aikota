# aikota 开发计划（2026-09-30）

> **状态**：living document。本文档是 aikota 的 **执行路线图**，回答一个问题：**下一步做什么、为什么、怎么做。**
>
> 关联文档：
> - 方向基线：`docs/capability-roadmap.md`（杠杆排序与归属原则）
> - 能力对标：`docs/harness-capability-matrix.md`（harness 六维度现状）
> - PRD 来源：`tasks/prd-*.md`（每个项目的原始需求）
>
> 更新方式：每落地一个 Phase，回到 §4 改对应行的"状态"与"完成日期"。**不新增章节，只更新状态。**

---

## 0. 当前阶段判定

### 0.1 版本状态

| 项目 | 值 |
|------|-----|
| **当前版本** | 0.4.3 (2026-07-31) |
| **开发阶段** | beta 晚期 → GA 前期过渡 |
| **代码规模** | ~40,000 行 Go / 38 个包 |
| **内置工具** | ~15 个 |
| **测试覆盖** | 核心路径 ~85%，部分边界 60%+ |
| **CI 状态** | ✅ 双平台通过（ubuntu-latest / windows-latest） |

### 0.2 已完成的核心能力

| 能力域 | 状态 | 关键提交 |
|--------|------|----------|
| Agent 核心循环 | ✅ | `0c1a4d5` (verify), `52683ac` (dream骨架) |
| 工具系统 | ✅ | `8012a14` (MCP), `0ce803b` (BrowserTool) |
| 交互式 TUI | ✅ | `a974d3f` (rename tui→interactive) |
| 子 Agent 编排 | ✅ | `8fa653d` (completion summary) |
| 远程控件 | ✅ | `409160e` (remote control) |
| 自更新 | ✅ | `560d326` (update semantics) |
| 沙箱/多租户/RLHF | ✅ | `d84bc4c` (P2 enterprise) |
| P0/P1 安全修复 | ✅ | `7342ead` (security audit) |
| Dream 骨架 | 🟡 | `52683ac` (runner skeleton), `nopConsolidator` 待替换 |
| /btw 侧线程 | 🟡 | `internal/cli/btw/` 已实现 |
| /status 命令 | 🟡 | `internal/cli/status/` 已实现，telemetry 累积需验证 |

### 0.3 待办状态（从 PRD 提取）

| PRD | 总项数 | 已完成 | 待完成 |
|-----|--------|--------|--------|
| `prd-dream-memory-consolidation.md` | 47 | 0 | 47 |
| `prd-harness-engineering.md` | 34 | 0 | 34 |
| `prd-prompt-templates.md` | 87 | 0 | 87 |
| `prd-pigo-hooks.md` | 89 | 0 | 89 |
| `prd-pigo-parity-features.md` | 98 | 0 | 98 |
| `prd-remote-control.md` | 42 | ~30 | ~12 |
| `prd-repl-multiline-input.md` | 37 | ~15 | ~22 |
| `prd-status-command.md` | 34 | ~20 | ~14 |
| `prd-goal-autonomous-mode.md` | 27 | 26 | 1 |
| `prd-agent-package-split.md` | ~6 | 6 | 0 ✅ |

> **注**：`prd-agent-package-split` 已全部分解为 US-001~006，对应提交 `95556ee`~`db5b978`，**已完成**。

---

## 1. 开发优先级（杠杆排序）

> 排序依据：`docs/capability-roadmap.md` §4 的杠杆原则——**每单位投入带来的能力增量**，不是实现难度。

### 第 0 层：主力 Agent 地基（不做则无法"放心放手"）

| # | 功能 | 现状 | 预期收益 | 难度 |
|---|------|------|----------|------|
| 0.1 | **崩溃恢复** | ⬜ 未验证 — 进程被杀后会话/工作区能否恢复 | 信任门槛 | 中 |
| 0.2 | **成本可见性** | ❌ 无 — token 用量已采集但用户不可见 | 信任门槛 | 低 |
| 0.3 | **错误诊断** | ⬜ 失败时无法定位"哪一步、为什么" | 可维护性 | 中 |
| 0.4 | **延迟可测** | ⬜ 启动延迟 + 首 token 延迟 | 可观测性 | 低 |

### 第 1 层：自有基础设施对接（护城河）

| # | 功能 | 现状 | 预期收益 | 难度 |
|---|------|------|----------|------|
| 1.1 | **ai-guard 策略外挂** | ⬜ 待启动 — hooks 框架已就绪 | 差异化能力 | 中 |
| 1.2 | **NATS bridge 设计落地** | ⬜ 设计稿 v0.2 已完成 | 事件流标准化 | 中 |
| 1.3 | **dynamic system-reminder** | 🟡 hooks 机制有，通用注入点无 | 任务保持力 | 中 |

### 第 2 层：高杠杆能力补齐

| # | 功能 | 现状 | 预期收益 | 难度 |
|---|------|------|----------|------|
| 2.1 | **LSP Tool** | ⬜ 需外部 server，归属待判 | 语义理解升级 | 高 |
| 2.2 | **Provider 自定义 Header** | 🟡 resolve.go:225 标注 | 扩展性预留 | 低 |
| 2.3 | **Dream LLM Consolidator** | 🟡 nopConsolidator 占位 | 记忆质量提升 | 中高 |
| 2.4 | **/status 命令完善** | 🟡 代码已存在，telemetry 累积需验证 | 可观测性 | 低 |

### 第 3 层：锦上添花（按需，不建议一次全上）

| # | 功能 | 状态 | 说明 |
|---|------|------|------|
| 3.1 | MCP 客户端 | ✅ 已实现 | |
| 3.2 | BrowserTool 交互动作 | 🟡 click/fill 未实现 | |
| 3.3 | 多 Agent Team | ⬜ PRD 存在 | |
| 3.4 | Worktree 隔离 | ⬜ 研究 PRD 存在 | |
| 3.5 | Notebook 编辑 | ⬜ 未开始 | |
| 3.6 | /btw UI 打磨 | 🟡 后端完成 | |
| 3.7 | 中文 Provider 扩展 | 🟡 PRD 存在 | |

---

## 2. 当前紧急待办（本周）

| # | 事项 | 位置 | 优先级 |
|---|------|------|--------|
| 1 | bash 工具故障排查文档入库 | ✅ 已完成 (2026-10-02) | P1 |
| 2 | review go.mod / model.go 未提交变更 | ✅ 已完成 (2026-10-02) | P1 |
| 3 | LSP 归属判定决策 | ✅ 完成 (2026-10-01)，已实现 Phase 4.1 | P2 |

---

## 3. Phase 详细计划

### Phase 1 — 地基加固（预计 2-3 周）

> **目标**：让 aikota 达到"可放心交由自主运行"的工程标准

#### 1.1 成本可见性（预计 3 天）

**挂载点**：`internal/runtime/telemetry.go` → `internal/cli/status/`

**用户故事**：
- 作为使用者，我希望在 `/status` 中看到本次 run 的成本（input/output tokens + 估算费用），以便评估是否值得继续。

**验收标准**：
- [ ] `TelemetryHolder` 中记录 per-run 的 token 用量（已从 `usage` 事件中采集）
- [ ] `/status` 输出新增「成本」区块，显示：
  - 本次 run：input tokens, output tokens, 估算费用（按 provider 计价）
  - 累计（session 起）：同上
- [ ] 提供 `--cost-mode` 标志控制展示精度（exact / estimate / off）
- [ ] 单测覆盖：`go test ./internal/cli/status/...` 全绿

**实施步骤**：
1. 扩展 `TelemetryEvent` 增加 `CostEstimate` 字段
2. 在 `runtime/loop.go` 的 `afterTurn` 中调用计价函数
3. 在 `cli/status/status.go` 的 `printTelemetryStatus` 中渲染
4. 添加 golden test 验证输出格式

---

#### 1.2 错误诊断增强（预计 5 天）

**挂载点**：`internal/runtime/loop.go` + `internal/agenttool/tool_executor.go`

**用户故事**：
- 作为使用者，我希望在工具调用失败时看到结构化诊断（工具名、参数、错误类型、调用栈），以便快速定位问题。

**验收标准**：
- [ ] `ToolExecutionError` 事件包含结构化字段：`ToolName`, `Args`, `ErrorType`, `StackTrace`, `RetriesUsed`
- [ ] `/status` 新增「最近错误」区块，展示最近 5 次失败的摘要
- [ ] 提供 `--diag` 模式输出完整诊断信息到 stderr
- [ ] 单测覆盖：`go test ./internal/agenttool/... -run TestToolExecutionError`

**实施步骤**：
1. 在 `agenttool/tool_executor.go` 中捕获并结构化错误
2. 在 `runtime/loop.go` 的 `ExecuteToolCalls` 中注入诊断上下文
3. 扩展 `AgentEvent` 类型增加 `ToolError` 变体
4. 在 `cli/status/status.go` 中渲染错误历史

---

#### 1.3 崩溃恢复验证与加固（预计 5 天）

**挂载点**：`internal/agenttool/file_snapshot.go` + `internal/session/`

**用户故事**：
- 作为使用者，我希望进程被杀后重启时能恢复到上次状态（文件变更可回退、会话可续跑），以便放心让 agent 自动运行。

**验收标准**：
- [ ] 验证 `file_snapshot` rewind 在进程被杀场景下的完整性
- [ ] 启动时检测上次是否异常退出（检查 `<session>/crashed` 标记）
- [ ] 异常退出时提示用户是否恢复上次的文件变更
- [ ] 添加自动化测试：`TestCrashRecovery` 模拟 kill + 重启 + 验证状态

**实施步骤**：
1. 在 `session.Store` 中增加 `IsCrashed()` 检测方法
2. 在 `interactive/model.go` 的 `Init()` 中检查崩溃状态
3. 添加恢复提示 UI（使用现有 `ui.Confirm`）
4. 编写端到端测试覆盖

---

#### 1.4 延迟可测（预计 2 天）

**挂载点**：`internal/runtime/loop.go`

**用户故事**：
- 作为使用者，我希望在 `/status` 中看到启动延迟和首 token 延迟，以便评估性能。

**验收标准**：
- [ ] 记录启动时间（binary load → first prompt）
- [ ] 记录首 token 延迟（TTFB）
- [ ] 在 `/status` 和 telemetry 中展示
- [ ] 单测覆盖：`go test ./internal/runtime/... -run TestLatencyMetrics`

---

### Phase 2 — 护城河构建（预计 2-3 周）

#### 2.1 dynamic system-reminder 机制（预计 5 天）

**挂载点**：`internal/runtime/reminder.go` → `internal/runtime/loop.go`

**用户故事**：
- 作为使用者，我希望 agent 每轮都能看到未完成任务提醒，以减少任务遗忘。

**验收标准**：
- [ ] `ReminderProvider` 接口定义完成
- [ ] 内置第一个 provider：**TODO 列表状态**（从 `todo_tool` 读取未完成项）
- [ ] 每轮 turn 前注入未完成任务提醒到上下文（作为 system-reminder）
- [ ] 提供配置开关：`config.toml` 中 `[reminder]` section
- [ ] 单测覆盖：`go test ./internal/runtime/... -run TestReminderInjection`

**实施步骤**：
1. 在 `runtime/reminder.go` 中定义 `ReminderProvider` 接口
2. 实现 `TodoListReminder` 作为参考 provider
3. 在 `runtime/loop.go` 的 `PrepareNextTurn` 中调用 reminder 注入
4. 添加 config 解析支持

---

#### 2.2 ai-guard hook 集成（预计 5 天）

**挂载点**：`internal/hooks/` + `internal/agenttool/tool_executor.go`

**用户故事**：
- 作为使用者，我希望 aikota 能调用 SPT 的 `check-scope` 策略，以便在执行危险操作前进行二次确认。

**验收标准**：
- [ ] `hooks.CheckScope` 钩子类型定义完成
- [ ] 集成 SPT 的 `check-scope` 逻辑（读 SPT 目录结构）
- [ ] 在 `tool_executor.go` 的 `BeforeToolCall` 中调用
- [ ] 提供 `--no-ai-guard` 标志跳过检查（用于测试）
- [ ] 单测覆盖：`go test ./internal/hooks/... -run TestCheckScope`

**实施步骤**：
1. 在 `hooks/hooks.go` 中增加 `CheckScopeHook` 类型
2. 在 `agenttool/tool_executor.go` 的 `executeTool` 中调用 hook
3. 添加 SPT 集成示例（docs/ 中）
4. 编写端到端测试

---

#### 2.3 Provider 自定义 Header 支持（预计 2 天）

**挂载点**：`internal/provider/resolve.go`

**用户故事**：
- 作为使用者，我希望能为 provider 配置自定义请求头，以便支持网关认证等场景。

**验收标准**：
- [ ] `ProviderSpec` 增加 `ExtraHeaders map[string]string` 字段
- [ ] 构造函数接受 `ExtraHeaders` 参数
- [ ] `config.toml` 支持 `extra_headers` 配置
- [ ] 单测覆盖：`go test ./internal/provider/... -run TestCustomHeaders`

**实施步骤**：
1. 修改 `provider/resolve.go` 中的构造函数签名
2. 在 `config.Config` 中增加解析支持
3. 在 `provider.New*` 函数中应用 headers
4. 添加文档说明

---

### Phase 3 — 核心能力深化（预计 3-4 周）

#### 3.1 Dream LLM Consolidator（✅ 已完成 2026-10-01）

**挂载点**：`internal/dream/runner.go`

**用户故事**：
- 作为使用者，我希望 `/dream` 能自动合并、去重、压缩记忆，让 aikota「越用越懂你」。

**验收标准**：
- [ ] 实现真实 `Consolidator` 替代 `nopConsolidator`
- [ ] 注入主会话模型 + dream 系统提示词
- [ ] 子进程崩溃隔离已就绪（复用 `--subagent-rpc`）
- [ ] 返回结构化变更报告（合并 N、去重 N、清理 N、剔除 N）
- [ ] 单测覆盖：`go test ./internal/dream/... -run TestRealConsolidator`

**实施步骤**：
1. 定义 `LLMConsolidator` 结构体，注入 `provider.Provider`
2. 在 `dream/runner.go` 中实现 `Consolidate` 方法
3. 构建系统提示词（参见 `dream/prompt.go`）
4. 在 `dream/runner.go` 的 `NewRunner` 中接受注入
5. 添加端到端测试（mock provider）

---

#### 3.2 /status 命令完善（预计 3 天）

**挂载点**：`internal/cli/status/status.go`

**验收标准**：
- [ ] 验证 telemetry 跨 run 累积逻辑
- [ ] 展示完整信息：模型、provider、context 使用率、成本、技能/插件数量、信任状态
- [ ] 添加「上一个 run」vs「累计」双视图
- [ ] 单测覆盖：`go test ./internal/cli/status/... -run TestStatusOutput`

---

#### 3.3 Dream 配置与自动触发（预计 3 天）

**挂载点**：`internal/dream/config.go` + `internal/cli/repl/dream_startup.go`

**验收标准**：
- [ ] `config.toml` 支持 `[dream]` section（enabled、interval_days）
- [ ] 会话启动时自动检测是否到期 → 后台静默触发
- [ ] 状态文件持久化到 `<memoryRoot>/global/dream/state.json`
- [ ] 单测覆盖：`go test ./internal/dream/... -run TestAutoTrigger`

---

### Phase 4 — 差异化能力（长期，2-4 周/项）

| # | 功能 | 预估工时 | 依赖 |
|---|------|----------|------|
| 4.1 | LSP Tool | 2 周 | 归属判定完成 |
| 4.2 | Crash Recovery 端到端验证 | 持续 | Phase 1.3 完成 |
| 4.3 | 多 Agent Team | 3 周 | subagent 编排完善 |
| 4.4 | Worktree 隔离 | 2 周 | GitTool 增强 |

---

## 4. 状态追踪表

| Phase | 项目 | 状态 | 开始日期 | 完成日期 | PRD |
|-------|------|------|----------|----------|-----|
| **Phase 1** | 1.1 成本可见性 | ✅ 完成 | 2026-09-30 | 2026-09-30 | - |
| | 1.2 错误诊断增强 | ✅ 完成 | 2026-09-30 | 2026-09-30 | - |
| | 1.3 崩溃恢复验证 | ✅ 完成 | 2026-09-30 | 2026-09-30 | - |
| | 1.4 延迟可测 | ✅ 完成 | 2026-09-30 | 2026-09-30 | - |
| **Phase 2** | 2.1 system-reminder | ✅ 完成 | 2026-10-01 | 2026-10-01 | `prd-harness-engineering.md` |
| | 2.2 ai-guard hook | ✅ 完成 | 2026-10-01 | 2026-10-01 | - |
| | 2.3 Provider headers | ✅ 完成 | 2026-10-01 | 2026-10-01 | `internal/provider/resolve.go:225` |
| **Phase 3** | 3.1 Dream Consolidator | ✅ 完成 | 2026-10-01 | 2026-10-01 | `prd-dream-memory-consolidation.md` |
| | 3.2 /status 完善 | ✅ 完成 | 2026-10-01 | 2026-10-01 | - |
| | 3.3 Dream 配置 | ✅ 完成 | 2026-10-01 | 2026-10-01 | `prd-dream-memory-consolidation.md` |
| **Phase 4** | 4.1 LSP Tool | ✅ 完成 | 2026-10-01 | 2026-10-01 | `tasks/spec-lsp-tool.md` |
| | 4.2 Crash Recovery 验证 | ✅ 完成 | 2026-10-02 | 2026-10-02 | - |

**图例**：⬜ 待开始 / 🟡 进行中 / ✅ 已完成

---

## 5. 执行纪律

1. **薄 diff**：改动尽量落在 pigo 既有的可选钩子 seam 上；新代码进独立包。分叉越薄，越能跟上上游。
2. **借设计不借代码**：尤其不要从任何派生于专有产品的仓库取码。
3. **工具数不是目标**：新增工具前先问"它改变模型看到的东西，还是只增加一个动作"。
4. **先做只有自己能做的**：第 1 层是身份，第 3 层是补课。顺序反了，得到的是功能很多但自己都不放心交付的产品。
5. **通用能力进产品，基础设施耦合留外面** ⚠️

---

## 6. 关联文档

| 文档 | 说明 |
|------|------|
| `docs/capability-roadmap.md` | 方向基线与杠杆排序 |
| `docs/harness-capability-matrix.md` | harness 六维度现状 |
| `docs/development-notes.md` | Windows 开发坑位 |
| `tasks/prd-*.md` | 原始 PRD 需求 |
| `tasks/spec-*.md` | 已落地的 SPEC 设计 |

---

*最后更新：2026-10-01*
