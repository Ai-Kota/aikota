# 设计：aikota 会话实时接入神经中枢（bridge 方案）

> 状态：**草案 v0.2（方向 A），待评审**
> 日期：2026-09-16
> 目标：让 AImlyForge 侧（WorkBoard、编排器）实时接住 aikota 的全部运行会话，
> 体验对标 `claude -p --output-format stream-json` 的完整事件流。

---

## 0. 决策记录（先读）

v0.1 草案把 NATS 客户端直接加进 aikota（`go.mod` 引入 `nats.go` + 在事件发射点
硬编 `aimly.agent.*` 主题），**与 `capability-roadmap.md` §5.5 的判定冲突**，
§5.5.1 案例 #2 已明确"❌ 不硬编，aikota 不认识 NATS，由 AImlyForge 侧 bridge 消费"。

v0.2 按方向 A 改写：

- **aikota 侧**：保持零 NATS 依赖，只补**通用**事件面缺口（§3）——这部分任何用户都有用；
- **AImlyForge 侧**：新建 `tools/aikota-bridge/` 常驻进程（§4），消费 aikota 的
  stream-json 事件流，映射到 NATS `aimly.*` 主题——基础设施耦合全部留在本仓。

若未来论证 NATS 原生发布确实更有优势（aikota 成为神经中枢一等器官），
再改判 §5.5.1 案例 #2 并回写本文 §0。**改判前，本文 §3 的事件面补全是唯一要进 aikota 的产品代码。**

---

## 1. 现状（已核实的代码事实）

| 事实 | 位置 |
|---|---|
| 14 种内部 `AgentEvent`（agent_start/end、turn_start/end、message_start/update/end、tool_execution_*、compaction(_start)、telemetry、subagent_progress） | `internal/agentcore/event.go` |
| SDK `StreamEvents` 只公开 **7 种**：message_delta / message_end / thinking_delta / tool_execution_{start,update,end} / usage | `agent/events.go`（`eventMapper` 的 switch 就是全部映射面） |
| stream-json（无头）导出 10 种，**刻意排除 subagent_progress**（stderr-only by contract，避免污染 stdout 机器可读流） | `internal/runtime/headless.go:96-99,105-121` |
| 子 Agent 是独立进程，有独立 session_id，各自产出自己的事件流 | `internal/runtime/subagent.go` |
| 主题命名规范 `aimly.<域>.<操作>.<目标>`，`event` 广播类型 | `deploy/nats/NEURAL_BACKBONE.md §4` |

⇒ 缺口（roadmap §4 第 1 层已实测记录）：**SDK 缺 7 种（编排层最想要的：运行起止、轮次边界、成本汇总、子 Agent 活动）；stream-json 缺 1 种（subagent_progress）**。

---

## 2. 总体拓扑

```
aikota 进程（每个会话一个；子 Agent 各自也是进程）
  │ stream-json stdout（--output-format stream-json）
  ▼
aikota-bridge（AImlyForge 侧常驻进程，tools/aikota-bridge/）
  ├─ 子进程模式：spawn/attach 各 aikota 会话，pipe 住其 stdout
  ├─ 解析每行事件（与 CLI 同源同格式）
  └─ 发布到 NATS
       aimly.agent.aikota.event.<session_id>   （Core，实时，全量）
       可选 JetStream 持久化 stream（回放/审计，运维侧建）
```

要点：

- **aikota 一行 NATS 代码都不加**，`go.mod` 不引入 `nats.go`。
- 子 Agent 会话由 bridge 用同一机制接入（它是 `aikota --subagent-rpc` 子进程，
  天然有独立 stdout 流，subject 用子 session_id，与父会话并列）。
- 交互 TUI/REPL 会话**本期不自动接入**（无 stream-json 输出）；TUI 场景下
  bridge 可以后续消费 SDK（§3 补全后的 `StreamEvents`）或等 aikota 提供
  "交互会话可选导出 stream-json 到文件"的通用能力——那是 aikota 侧的独立需求，
  不与本方案耦合。

---

## 3. aikota 侧改动（通用，进产品）

**目标：让 stream-json 成为全事件面，bridge 消费到的就是"完整会话"。**

| 改动 | 位置 | 规模 | 状态 |
|---|---|---|---|
| 把 `subagent_progress` 纳入 stream-json 导出（保留 stderr progress 行，两者并存） | `internal/runtime/headless.go` 的 OnEvent 链 | ~10 行 + fixture | ✅ 已落地（2026-09-16：stdout 机器通道 + stderr 人读通道并存；契约测试 `TestRunHeadlessSubAgentProgressChannels` + envelope 字段测试） |
| （可选）SDK 事件面补全：`agent_events.go` 新增 7 种公开事件类型 + mapper 映射 | `agent/events.go` / `agent/agent.go` | ~150 行 + 测试 |

两个改动都是纯通用能力：

- stream-json 的完整性 = 任何消费方（不只 bridge）的收益；
- SDK 事件面补全 = 任何嵌 aikota 的 Go 程序可拿到运行起止/成本/子 Agent 活动。

**契约注意**（roadmap 已记录的实测结论）：bridge 若只建在补全后的 SDK 上，
仍缺 7 种；只建在 stream-json 上，缺 `subagent_progress`——**先补事件面，bridge 才
有"完整会话"可发**。§3.1 的 stream-json 补全就是这一步；SDK 补全（§3.2 可选）
留给后续交互会话接入。

---

## 4. AImlyForge 侧：`tools/aikota-bridge`

独立 Go 模块（纳 `AImlyForge/go.work`），复用 `tools/_shared/natsutil`：

```
tools/aikota-bridge/
├── cmd/aikota-bridge/main.go    # 配置（哪些会话跑哪、NATS 连接参数）
├── internal/
│   ├── consumer/                # spawn aikota -p "…" -o stream-json，逐行读 stdout
│   ├── subject.go               # aimly.agent.aikota.event.<sid>
│   └── nats.go                  # natsutil.DefaultConnect / PublishEvent
└── README.md                    # 部署（常驻、故障重启）+ 订阅方示例
```

**事件 → NATS 映射**

| 项 | 取值 |
|---|------|
| subject | `aimly.agent.aikota.event.<session_id>`（Core 直发；可选 JetStream 持久化） |
| payload | **stream-json 行原文**（与本地 `--output-format stream-json` 同构，订阅方零适配） |
| session_id | 首行 `agent_start` 的 `sessionId` 字段（bridge 解析后拼 subject） |
| 多会话订阅 | `nats sub 'aimly.agent.aikota.event.>'` |
| 失败策略 | 发布失败 warn + 丢弃计数，bridge 崩溃可重启（会话本身由 aikota 的 JSONL 落盘兜底） |

**规模**：~250-350 行 + 测试（用嵌入式 NATS server 做契约测试）。

---

## 5. 与 v0.1 草案的差异（评审对照用）

| 点 | v0.1（已否决） | v0.2 |
|---|---|---|
| NATS 依赖 | 进 aikota `go.mod` | aikota 零依赖；进 bridge |
| 主题 | 默认值硬编 `aimly.agent.session.*` | 只存在于 bridge（AImlyForge 仓内，耦合合法） |
| 事件面 | 不补（直接全量发） | stream-json 补 `subagent_progress`（aikota 通用改动） |
| 交互会话 | 直接覆盖 | 本期不覆盖（§2 要点 3），后续独立立项 |

## 6. 待评审清单

- [ ] §3.1 stream-json 补 `subagent_progress` 的契约修订（stderr-only 注释需同步改）
- [ ] §3.2 SDK 事件面补全是否本期做（建议否，先 bridge 跑通）
- [ ] subject 命名 `aimly.agent.aikota.event.<sid>` 的中枢登记口径（§4）
- [ ] JetStream 持久化是否本期建 stream（建议否，Core 先行）
