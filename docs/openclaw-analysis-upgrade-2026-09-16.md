# openclaw 第一性原理拆解 · 对 aikota 的升级杠杆排序

> 生成：2026-09-16 · 基于 openclaw 归档（archive/openclaw）与 aikota 现状（capability-roadmap / harness-matrix）
> 原则：借设计不借代码；通用机制进产品，基础设施耦合留外面。

---

## 一、openclaw 架构真相（8 个第一性原理）

1. **Gateway 是本地控制平面**：session / tools / events / channels 的 owner；CLI、Control UI、TUI 都是它的客户端。
2. **信任边界：trusted gateway + untrusted execution**——策略确定性优先于能力最大化。
3. **能力分层：核心瘦，边缘厚**。核心每次调用都有"上下文税"（每个 tool/prompt/config 键都进每次模型请求）；可选能力放 plugin/skill/channel。
4. **单一 owner，完整 cutover**：一个责任只有一个 owner；改一处要迁掉所有旧路径（Intent→Owner→Cutover→Proof 四步）。
5. **稳定对话上下文**：不重建历史（保 prompt 前缀复用）；确定性注入；只有 compaction 可重写历史。
6. **契约沉淀规律**：recurring demand defines interfaces——多个独立需求撞同一种能力 → 落 contract 到 core/SDK，bundled 实现迁上去，其余做成 plugin。
7. **Doctor 迁移体系**：runtime 只读当前 schema；legacy 状态归 Doctor 归一化，不留兼容 reader。
8. **验证即产品**：每个 action 有可见结果；证明要走真实入口；安全是权衡而非最大化限制。

## 二、对照：aikota 已有什么（不重复造）

- 双层 agent loop + steering / follow-up 钩子；自动 compaction；工具结果统一字节预算；
- 安全闸门（safetygate，`--approve` 不可绕）；信任三态；repo_map（零依赖，净简化）；
- verify 工具（项目自发现 build/test + 验证提醒）；memory（FTS + MEMORY.md + checkpoint）；
- /dream 记忆固结（子进程隔离）；subagent 双隔离（goroutine / process JSON-RPC）；
- 结构化遥测（v1 schema + golden 测试）；hooks 9 生命周期；skills/prompts/plugins 包管理。

## 三、升级杠杆（按 ROI 排序）

### P0 · 聪明（上下文质量）

1. **system-reminder 动态注入**（harness 矩阵 P0，US-002）：复用 `GetSteeringMessages` / `PrepareNextTurn` 钩子按轮注入易失上下文（todo 状态 / 文件变更 / 预算告警）；压缩时安全丢弃。对标 CC 标志能力。
2. **记忆召回质量提升**：openclaw memory 是 plugin 槽（memory-core / lancedb / wiki 三选一）——可借鉴的只有两点：
   - 记忆的 **provenance 与失效生命周期**（root-memory-files / artifact-provenance 思路）：记忆引用失效时主动剔除，而非"应该是"；
   - dream 固结时**注入"当前态"而非"历史"**（九天当前态模式，与 openclaw 稳定上下文原则同源）。

### P1 · 务实（信任地基——"主力"的四个工程属性）

3. **崩溃可恢复**（roadmap 第 0 层 ⬜）：进程被杀后会话/工作区可恢复；参考 openclaw 的 run-registry liveness + 写入前先绑定 target 的纪律。
4. **错误可诊断**：失败时定位到"哪一步、为什么"（telemetry 已有，缺面向使用者的诊断出口）。
5. **成本可见**：token 用量已采集未暴露——加一行 usage 展示即可，ROI 极高。

### P2 · 聪明（验证与恢复）

6. **plan mode / 检查点语义**：openclaw 的"plan 是 checkpoint 不是 completion"工作约定；aikota 已有 rewind + fork，可加"计划先行、执行确认"的轻量模式。
7. **owner 纪律进代码规范**：aikota 内部包之间也应有"单一 owner、完整 cutover"的 AGENTS.md 约束（借鉴 AGENTS.md 分层写法，不抄内容）。

### 明确不做（openclaw 有但 aikota 结构上不该要的）

- Gateway 常驻控制平面 / channels / 多设备伴随应用——aikota 是 CLI，形态不同；
- 200+ provider 插件——已有 38 个 + 标准端点协议，饱和区；
- 核心内置 skill 扩张——skill 走包管理外部化（aikota 判据 §5.5 的实例）。

## 四、执行纪律（沿用 capability-roadmap）

- 薄 diff：落 pigo 既有 seam；新代码进独立包；
- 借设计不借代码；工具数不是目标；
- 判定程序：给不用你基础设施的用户还有意义吗？→ 没意义就留外面。
