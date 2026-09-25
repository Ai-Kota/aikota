# aikota 持久记忆大脑 · 集成设计文档

> **目标**：让 aikota 的记忆系统从「被动存储 + 粗暴截断」进化为「人类大脑式」的主动、分层、可逆、跨会话连续的认知层。
>
> **核心哲学**：记忆不是历史的副本，而是为了**延续意识**而存在的精简表达。

---

## 一、现状分析：aikota 已有的记忆基础设施

aikota 并非从零开始。以下组件已经存在并经过验证：

### 1.1 已实现的能力

| 模块 | 路径 | 能力 | 质量评估 |
|------|------|------|---------|
| **Memory Store** | `internal/memory/` | SQLite + FTS5 BM25，Markdown 文件落盘，4 作用域（global/projects/sessions/cc） | ✅ 生产可用 |
| **Memory Search Tool** | `internal/agenttool/memory_tool.go` | `memory_search` 工具，BM25 检索，`ReconcileFirst=true` 懒加载 | ✅ 工作正常 |
| **Dream Consolidator** | `internal/dream/consolidator.go` | LLM 驱动的合并/修剪/蒸馏，JSON 严格输出 + 保守 KEEP 策略 | ✅ 成熟 |
| **Dream Runner** | `internal/dream/runner.go` | 定时调度（interval 天），锁机制，路径作用域守卫，原子写入 | ✅ 生产可用 |
| **Dream Plan** | `internal/dream/plan.go` | 精确去重（SHA-256）+ 近似的 Jaccard 相似度（threshold=0.7）+ 死路径清理 | ✅ 优秀 |
| **Dream Distiller** | `internal/dream/distill.go` | 从 JSONL 会话转录中提取 durable facts，过滤 ephemeral 内容 | ✅ 优秀 |
| **Context Compaction** | `internal/compaction/compact.go` | LLM 摘要 + 文件操作提取 + 滚动更新（iterative update） | ✅ 工作正常 |
| **Token Estimation** | `internal/compaction/tokens.go` | 字符→token 启发式，Provider Usage 优先，`ShouldCompact()` 阈值检查 | ✅ 准确 |
| **Compaction Checkpoint** | `runtime/loop.go:418` | 摘要持久化为 `sessions/<id>/checkpoint.md`，rebuild 时注入 | ✅ 工作正常 |
| **Hook System** | `internal/hooks/` | 9 条生命周期 hook，PreToolUse/PostToolUse/Stop/SessionStart 等 | ✅ 可扩展 |
| **Compaction Settings** | `DefaultCompactionSettings` | `ReserveTokens=16384`, `KeepRecentTokens=20000` | ⚠️ 需要调优 |

### 1.2 关键差距：为什么现在还不是「大脑」

| 维度 | 现状 | 大脑应有的样子 | 差距 |
|------|------|-------------|------|
| **压缩策略** | 全段截断 + LLM 摘要（粗暴） | Live-zone 渐进压缩（精细） | 🔴 核心缺陷 |
| **可逆性** | 压缩后原文丢弃 | CCR 原文缓存，按需检索 | 🔴 信息永久丢失 |
| **检索方式** | 纯 BM25 关键词 | BM25 + 向量语义 + 图谱关系 | 🟡 缺失语义层 |
| **记忆分层** | 扁平 Markdown 文件 | Working/Episodic/Semantic/Procedural 四阶 | 🟡 缺少层次 |
| **SessionStart 注入** | 静态文件加载 | 基于 query 的动态 Hybrid Retrieval 注入 | 🟡 被动 vs 主动 |
| **跨会话连续性** | checkpoint 只含摘要 | 结构化知识 + 未完成任务 + 用户偏好 | 🟡 碎片化 |
| **实时性** | Dream 定时运行（天级） | PostToolUse 实时捕获 + 增量 Consolidation | 🟡 滞后 |
| **上下文感知** | 无 | 预测性注入（根据当前任务类型预加载相关记忆） | 🔴 缺失 |

---

## 二、目标架构：aikota 大脑系统

### 2.1 核心设计原则

```
原则 1：记忆永不消失，只会沉睡
  → 所有压缩都是可逆的（CCR），原文本地缓存，模型按需检索

原则 2：分层存储，不同寿命
  → 工作记忆（小时级）→ 事件记忆（天级）→ 语义记忆（月级）→ 程序记忆（永久）

原则 3：主动注入，而非被动加载
  → SessionStart 不是读文件，而是基于当前意图的 Hybrid Retrieval

原则 4：渐进压缩，而非截断
  → Live-zone 策略：冻结前缀，只压缩新增，历史永不丢弃

原则 5：人类式遗忘
  → Ebbinghaus 衰减曲线，高频访问强化，长期未访问自动降级
```

### 2.2 架构总图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            aikota Agent Loop                                │
│                                                                             │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐              │
│  │ Session  │───▶│  Inner   │───▶│  Tool    │───▶│  Provider│              │
│  │  Start   │    │   Loop   │    │  Exec    │    │  Call    │              │
│  └──────────┘    └──────────┘    └──────────┘    └────┬─────┘              │
│       ▲                                              │                      │
│       │                   ┌──────────────────────────┘                      │
│       │                   │                                                  │
│       ▼                   ▼                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     BRAIN LAYER (新)                               │    │
│  │                                                                     │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌──────────┐  │    │
│  │  │ Working     │  │ Episodic    │  │ Semantic    │  │ Proced.  │  │    │
│  │  │ Memory      │  │ Memory      │  │ Memory      │  │ Memory   │  │    │
│  │  │ (小时级)     │  │ (天级)       │  │ (月级)       │  │ (永久)    │  │    │
│  │  │ 当前会话快照  │  │ 会话摘要链    │  │ 事实/决策库   │  │ 技能/模式 │  │    │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └────┬─────┘  │    │
│  │         │                │                │              │         │    │
│  │         └────────────────┴────────────────┴──────────────┘         │    │
│  │                           │                                         │    │
│  │               ┌───────────▼───────────┐                             │    │
│  │               │   Live-zone + CCR     │                             │    │
│  │               │   渐进压缩 + 可逆缓存    │                             │    │
│  │               └───────────┬───────────┘                             │    │
│  │                           │                                         │    │
│  │               ┌───────────▼───────────┐                             │    │
│  │               │  Hybrid Retrieval     │                             │    │
│  │               │  BM25 + Vector + Graph │                             │    │
│  │               └───────────────────────┘                             │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  现有层（保留并增强）：                                                        │
│  - memory.Store (SQLite + FTS5) ← 作为 Semantic 层持久化                     │
│  - dream.Runner ← 作为 Consolidation 调度器                                  │
│  - compaction.Compact ← 重写为 Live-zone + CCR                              │
│  - hooks.* ← 增加实时记忆捕获钩子                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 数据流：一次完整会话的生命周期

```
【SessionStart】
  ├─ 1. 加载 Project Profile（项目结构、关键决策、当前目标）
  ├─ 2. Hybrid Retrieval 搜索相关记忆（BM25 + Vector + Graph）
  ├─ 3. Token Budget 控制注入量（默认 2000 tokens）
  ├─ 4. 注入到 system prompt 开头（"你已经完成过的工作："）
  └─ 5. 创建 Working Memory 快照

【PreToolUse】
  ├─ 1. 记录文件访问模式（read/write/edit）
  ├─ 2. 如果访问新文件 → 自动注入该文件的关键上下文
  └─ 3. 更新 Working Memory

【PostToolUse】
  ├─ 1. 捕获工具输出（原始 + 摘要）
  ├─ 2. 隐私过滤（API Key、Token 剥离）
  ├─ 3. Live-zone 压缩（仅压缩新增部分）
  ├─ 4. CCR 缓存原文（TTL 72h，可检索）
  └─ 5. 如果输出 > 阈值 → 触发增量 Consolidation

【PreCompact】（旧 compaction 触发前）
  ├─ 1. 重新注入最新记忆（防止截断后失忆）
  ├─ 2. 标记哪些 Working Memory 需要保留为 CCR
  └─ 3. 执行 Live-zone 压缩（而非全段截断）

【Stop / SessionEnd】
  ├─ 1. 会话总结（LLM 生成结构化摘要）
  ├─ 2. 知识提取（实体、决策、模式）
  ├─ 3. Procedural Memory 更新（如果检测到新模式）
  ├─ 4. Working → Episodic 晋升
  └─ 5. 触发 Dream Consolidation（如果距离上次 > interval）

【Dream Consolidation】（后台，每天/每周）
  ├─ 1. BuildPlan（去重、死路径、近似的）
  ├─ 2. LLM Merge/Prune（保守策略）
  ├─ 3. LLM Distill（从会话转录提取新记忆）
  ├─ 4. 4-Tier 晋升（Working → Episodic → Semantic）
  └─ 5. Ebbinghaus 衰减扫描
```

---

## 三、核心创新设计

### 3.1 Live-zone + CCR 渐进压缩（替换现有 compaction）

**现状问题**：
- 现有 `compaction.Compact()` 是全段截断：找到 cut point，把 `[0, cut)` 替换为一个 LLM 摘要
- 摘要丢失细节，且不可恢复
- provider KV-cache 前缀被破坏（因为摘要替换了前缀）

**新设计**：

```go
// internal/compaction/livezone.go (新增)
package compaction

// LiveZoneConfig 控制渐进压缩行为
type LiveZoneConfig struct {
    // FrozenPrefixTokens 冻结的前缀 token 数（保持 provider KV-cache 命中）
    FrozenPrefixTokens int
    // CompressRatio 目标压缩比（0.3 = 压缩到 30% 大小）
    CompressRatio float64
    // CCRTTL 原文缓存 TTL（小时）
    CCRTTL int
}

// LiveZoneResult 渐进压缩的结果
type LiveZoneResult struct {
    // FrozenPrefix 保持不变的对话前缀
    FrozenPrefix []agentcore.Message
    // CompressedTail 压缩后的新增内容
    CompressedTail []agentcore.Message
    // CCRIndices 原文缓存的索引（模型可通过 memory_retrieve 获取）
    CCRIndices []CCRIndex
    // TokensBefore/After 压缩前后的 token 数
    TokensBefore, TokensAfter int
}

// CCRIndex 原文缓存条目
type CCRIndex struct {
    ID        string // 唯一 ID
    Original  string // 压缩前的原文
    Compressed string // 压缩后的摘要
    TTL       int64  // 过期时间戳
    Tags      []string // 检索标签
}
```

**压缩流程**：
```
原始对话：[Prefix (10K tokens)] + [Tool Output A (5K tokens)] + [Tool Output B (8K tokens)] + [Recent Turn (2K tokens)]
                                    ↑ frozen prefix 保持原样，provider cache hit
                                    ↓
Live-zone 压缩后：
  [Prefix (10K tokens, 不变)]
  + [Tool Output A → CCR cache, 可检索]
  + [Tool Output B → CCR 摘要 (1.5K tokens)]
  + [Recent Turn (2K tokens, 未压缩)]
  
总 token: 10K + 1.5K + 2K = 13.5K (原 25K，节省 46%)
```

**关键优势**：
1. **Provider KV-cache 命中**：前缀不变，cache 效率不变
2. **原文可恢复**：模型通过 `memory_retrieve` 工具获取完整原文
3. **渐进式**：每次都只压缩新增部分，历史积累不被破坏
4. **零信息丢失**：CC 缓存在 TTL 内始终可检索

### 3.2 四阶记忆分层

```go
// internal/memory/tiers.go (新增)
package memory

// Tier 记忆层级
type Tier int

const (
    TierWorking    Tier = iota // 工作记忆：当前会话的原始观察
    TierEpisodic               // 事件记忆：会话摘要和关键节点
    TierSemantic               // 语义记忆：事实、决策、模式
    TierProcedural             // 程序记忆：工作流、技能、决策树
)

// MemoryEntry 统一记忆条目接口
type MemoryEntry struct {
    ID          string  `json:"id"`
    Tier        Tier    `json:"tier"`
    Scope       Scope   `json:"scope"`        // global/projects/sessions
    ScopeID     string  `json:"scope_id"`     // project hash / session id
    Type        Type    `json:"type"`         // user/feedback/project/reference/etc
    Body        string  `json:"body"`         // 当前层级的表示
    OriginalID  string  `json:"original_id"`  // 如果是晋升而来，指向源条目
    CreatedAt   int64   `json:"created_at"`
    UpdatedAt   int64   `json:"updated_at"`
    LastAccessed int64  `json:"last_accessed"`
    AccessCount int      `json:"access_count"`     // 访问次数（用于 Ebbinghaus 衰减）
    DecayScore  float64 `json:"decay_score"`      // 0-1，越低越可能被遗忘
    Tags        []string `json:"tags"`
    Embedding   []float32 `json:"embedding,omitempty"` // 向量嵌入（TierSemantic+）
}
```

**晋升规则**：
```
Working → Episodic:
  条件：会话结束（Stop hook）
  动作：LLM 生成结构化摘要，写入 episodes/<session_id>.md

Episodic → Semantic:
  条件：距离上次访问 > 7 天 且 访问次数 > 3
  动作：LLM 提取事实，写入 semantic/ 目录

Semantic → Procedural:
  条件：同一模式重复出现 ≥ 3 次
  动作：转化为 skill/prompt template，存入 procedural/
```

**Ebbinghaus 衰减**：
```go
// 简化版 Ebbinghaus 曲线
func decayScore(accessCount int, hoursSinceLastAccess float64) float64 {
    // 基础衰减：随时间指数下降
    timeDecay := math.Exp(-hoursSinceLastAccess / 72.0) // 3 天半衰期
    // 强化因子：访问次数越多，衰减越慢
    reinforcement := 1.0 + 0.1*float64(accessCount)
    return math.Min(1.0, timeDecay*reinforcement)
}

// 低于阈值时自动降级
func shouldDemote(entry *MemoryEntry, now time.Time) bool {
    return entry.DecayScore < 0.1 && 
           now.Sub(time.Unix(entry.LastAccessed, 0)).Hours() > 720 // 30 天
}
```

### 3.3 Hybrid Retrieval（BM25 + Vector + Graph）

**现状**：只有 BM25 FTS5

**新增**：

```go
// internal/memory/hybrid_search.go (新增)
package memory

// HybridSearchResult RRF 融合后的检索结果
type HybridSearchResult struct {
    Entries   []MemoryEntry
    Scores    map[string]float64 // entry_id → fused score
    Breakdown map[string]SearchBreakdown
}

type SearchBreakdown struct {
    BM25Score  float64
    VectorScore float64
    GraphScore float64
}

// HybridSearch 三路融合检索
func (s *Store) HybridSearch(query string, opts SearchOptions) (*HybridSearchResult, error) {
    // 1. BM25 检索
    bm25Results := s.bm25Search(query, opts)
    
    // 2. 向量检索（如果配置了 embedding provider）
    var vectorResults []SearchResult
    if s.embeddingsEnabled {
        queryVec := s.encode(query)
        vectorResults = s.vectorSearch(queryVec, opts)
    }
    
    // 3. 图谱检索（如果查询包含实体）
    var graphResults []SearchResult
    if entities := extractEntities(query); len(entities) > 0 {
        graphResults = s.graphTraversal(entities, opts)
    }
    
    // 4. RRF 融合
    return rrfFuse(bm25Results, vectorResults, graphResults, opts.Limit), nil
}
```

**RRF 融合公式**：
```
score(doc) = Σ (1 / (k + rank_i(doc)))
其中 k=60, rank_i 是该 doc 在第 i 路检索中的排名
```

### 3.4 SessionStart 智能注入

**现状**：加载静态文件（CLAUDE.md、AGENTS.md 等）

**新设计**：

```go
// internal/runtime/memory_injection.go (新增)
package runtime

// injectMemoryContext 在 SessionStart 时注入相关记忆
func (a *Agent) injectMemoryContext(ctx context.Context, prompt string) error {
    // 1. 分析当前意图（轻量 LLM 调用或关键词提取）
    intent := a.extractIntent(prompt)
    
    // 2. Hybrid Retrieval 搜索相关记忆
    results, err := a.memoryStore.HybridSearch(intent.Query, memory.SearchOptions{
        Limit:         5,
        Tier:          []memory.Tier{memory.TierSemantic, memory.TierEpisodic},
        TokenBudget:   2000, // 控制注入量
    })
    if err != nil {
        return err
    }
    
    // 3. 构建注入上下文
    var injectParts []string
    
    // 3a. 项目画像（始终注入）
    if profile := a.loadProjectProfile(); profile != "" {
        injectParts = append(injectParts, "## Project Context\n"+profile)
    }
    
    // 3b. 相关记忆（按需注入）
    for _, r := range results.Entries {
        injectParts = append(injectParts, formatMemoryEntry(r))
    }
    
    // 3c. 未完成事项（如果有的话）
    if pending := a.loadPendingItems(); pending != "" {
        injectParts = append(injectParts, "## Pending Items\n"+pending)
    }
    
    // 4. 注入到 system prompt
    if len(injectParts) > 0 {
        a.injectedContext = strings.Join(injectParts, "\n\n")
    }
    
    return nil
}
```

---

## 四、与现有代码的集成点

### 4.1 修改 `internal/compaction/` — 替换为 Live-zone + CCR

**文件变更**：
- `compact.go` — 重写 `Compact()` 为 `LiveZoneCompact()`
- 新增 `livezone.go` — Live-zone 压缩逻辑
- 新增 `ccr.go` — CCR 原文缓存管理
- `tokens.go` — 增加 token 预算控制

**与现有代码的兼容**：
- `FindCutPoint()` 保留（用于确定冻结前缀长度）
- `GenerateSummary()` 保留（用于 CCR 摘要生成）
- `CompactionResult` 扩展为 `LiveZoneResult`

### 4.2 扩展 `internal/memory/` — 增加向量检索和四阶分层

**文件变更**：
- `store.go` — 增加 `EmbeddingsEnabled` 标志和向量索引
- `search.go` — 新增 `HybridSearch()` 方法
- 新增 `tiers.go` — 四阶记忆类型定义
- 新增 `hybrid_search.go` — RRF 融合逻辑
- 新增 `decay.go` — Ebbinghaus 衰减计算

**向后兼容**：
- `Search()` 方法保留（BM25 only），作为 fallback
- 所有现有测试通过 → 新增向量检索功能

### 4.3 新增 `internal/dream/consolidation_pipeline.go` — 四阶晋升

**新文件**：
```
internal/dream/
├── consolidation_pipeline.go  # 新增：四阶晋升管道
├── tier_manager.go            # 新增：层级管理
└── (现有文件保持不变)
```

**与现有 dream.Runner 的关系**：
- 现有 `dream.Runner.Run()` 继续负责 merge/prune/distill
- 新增 `ConsolidationPipeline` 在 Runner 之后运行，负责 tier 晋升
- 两者通过 `ConsolidateInput.Transcripts` 共享数据

### 4.4 扩展 `internal/hooks/` — 实时记忆捕获

**新增 Hook 类型**：
```go
// internal/hooks/types.go (扩展)
const (
    HookPostToolUseRealtime HookType = "post_tool_use_realtime"  // 新增：实时捕获
    HookSessionMemoryReady  HookType = "session_memory_ready"    // 新增：会话记忆就绪
)
```

**Hook 行为**：
- `PostToolUseRealtime`：捕获工具输出，触发 Live-zone 压缩，更新 Working Memory
- `SessionMemoryReady`：在 SessionStart 后触发，确认记忆注入完成

### 4.5 新增 `internal/agenttool/memory_retrieve_tool.go` — CCR 检索工具

**新工具**：
```go
// memory_retrieve 从 CCR 缓存中获取原文
type MemoryRetrieveTool struct {
    Store *memory.Store
}

func (t *MemoryRetrieveTool) Name() string { return "memory_retrieve" }
func (t *MemoryRetrieveTool) Description() string {
    return "Retrieve the original (uncompressed) text of a previously captured memory entry. " +
        "Use this when the compressed summary is insufficient and you need the full context."
}
```

---

## 五、实施路线图

### Phase 1：Live-zone + CCR（2 周）

**目标**：替换现有 compaction 为渐进压缩

| 步骤 | 文件 | 工作内容 |
|------|------|---------|
| 1.1 | `compaction/livezone.go` | 实现 `LiveZoneCompact()` |
| 1.2 | `compaction/ccr.go` | 实现 CCR 缓存（SQLite + TTL） |
| 1.3 | `runtime/loop.go` | 替换 `runCompaction()` 调用 |
| 1.4 | `agenttool/memory_retrieve_tool.go` | 新增 CCR 检索工具 |
| 1.5 | 测试 | 保留现有 compaction 测试，新增 live-zone 测试 |

**验收标准**：
- [ ] 长会话（>50K tokens）不再触发截断，而是渐进压缩
- [ ] 模型可通过 `memory_retrieve` 获取压缩前的原文
- [ ] Provider KV-cache 命中率不变（前缀冻结）

### Phase 2：向量检索 + Hybrid Search（2 周）

**目标**：增加语义检索能力

| 步骤 | 文件 | 工作内容 |
|------|------|---------|
| 2.1 | `memory/store.go` | 增加向量索引（HNSW 或简陋的 brute-force） |
| 2.2 | `memory/hybrid_search.go` | 实现 RRF 融合 |
| 2.3 | 配置 | `EMBEDDING_PROVIDER=local` 启用本地 embedding |
| 2.4 | 测试 | LongMemEval-S 基准测试 |

**验收标准**：
- [ ] `memory_search` 支持语义检索（可选开关）
- [ ] R@5 > 90%（LongMemEval-S）

### Phase 3：四阶记忆分层（2 周）

**目标**：实现 Working → Episodic → Semantic → Procedural 晋升

| 步骤 | 文件 | 工作内容 |
|------|------|---------|
| 3.1 | `memory/tiers.go` | 定义 Tier 类型和晋升规则 |
| 3.2 | `dream/consolidation_pipeline.go` | 实现晋升管道 |
| 3.3 | `dream/tier_manager.go` | 层级管理器 |
| 3.4 | `dream/runner.go` | 集成晋升管道到 Runner |
| 3.5 | 存储迁移 | 现有 Markdown 文件映射到对应 Tier |

**验收标准**：
- [ ] 会话结束后 Working Memory 自动晋升为 Episodic
- [ ] 7 天后 Episodic 自动晋升为 Semantic
- [ ] 重复模式自动提炼为 Procedural（skill/template）

### Phase 4：SessionStart 智能注入（1 周）

**目标**：基于意图的动态记忆注入

| 步骤 | 文件 | 工作内容 |
|------|------|---------|
| 4.1 | `runtime/memory_injection.go` | 实现 `injectMemoryContext()` |
| 4.2 | `hooks/session_start.go` | 扩展 SessionStart hook |
| 4.3 | `runtime/prompt.go` | 集成注入上下文到 system prompt |
| 4.4 | CLI | `--inject-memory` 开关 |

**验收标准**：
- [ ] SessionStart 时自动注入相关记忆（token budget 控制）
- [ ] 新会话无需重新解释项目结构

### Phase 5：实时捕获 + Ebbinghaus 衰减（1 周）

**目标**：PostToolUse 实时捕获 + 自动遗忘

| 步骤 | 文件 | 工作内容 |
|------|------|---------|
| 5.1 | `hooks/post_tool_use_realtime.go` | 新增实时捕获 hook |
| 5.2 | `memory/decay.go` | 实现 Ebbinghaus 衰减 |
| 5.3 | `dream/sweep.go` | 新增衰减扫描 cron |
| 5.4 | 配置 | `DECAY_ENABLED=true` 开关 |

**验收标准**：
- [ ] 工具输出实时捕获到 Working Memory
- [ ] 30 天未访问的记忆自动标记为"待遗忘"
- [ ] 高频访问的记忆衰减减缓

---

## 六、配置设计

### 6.1 新增环境变量

```env
# ~/.aikota/.env

# Live-zone + CCR
AIKOTA_LIVEZONE_ENABLED=true           # 默认 true
AIKOTA_LIVEZONE_FROZEN_TOKENS=10000    # 冻结前缀 token 数
AIKOTA_LIVEZONE_COMPRESS_RATIO=0.3     # 目标压缩比
AIKOTA_CCR_TTL_HOURS=72                # 原文缓存 TTL

# 向量检索
AIKOTA_EMBEDDING_PROVIDER=local        # local / openai / gemini / none
AIKOTA_EMBEDDING_MODEL=all-MiniLM-L6-v2
AIKOTA_HYBRID_SEARCH_ENABLED=true      # 默认 true

# 四阶记忆
AIKOTA_MEMORY_TIERS_ENABLED=true       # 默认 true
AIKOTA_TIERS_WORKING_TTL_HOURS=1       # Working 记忆 TTL
AIKOTA_TIERS_EPISODIC_TTL_DAYS=7       # Episodic 记忆 TTL
AIKOTA_TIERS_SEMANTIC_TTL_DAYS=30      # Semantic 记忆 TTL
AIKOTA_TIERS_PROMOTION_ENABLED=true    # 自动晋升

# 衰减
AIKOTA_DECAY_ENABLED=true              # 默认 true
AIKOTA_DECAY_HALF_LIFE_HOURS=72        # Ebbinghaus 半衰期

# 注入
AIKOTA_INJECT_MEMORY_ENABLED=true      # SessionStart 注入
AIKOTA_INJECT_TOKEN_BUDGET=2000        # 注入 token 预算
```

### 6.2 与现有配置的兼容

| 现有配置 | 新配置 | 行为 |
|---------|--------|------|
| `memory.enabled` | `AIKOTA_LIVEZONE_ENABLED` + `AIKOTA_MEMORY_TIERS_ENABLED` | 向后兼容，默认开启 |
| `compaction.*` | `AIKOTA_LIVEZONE_*` + `AIKOTA_CCR_*` | 自动迁移旧配置 |
| `dream.interval` | 不变 | 保持不变 |

---

## 七、风险与缓解

| 风险 | 影响 | 缓解策略 |
|------|------|---------|
| Live-zone 压缩丢失关键细节 | 🔴 高 | CCR 原文缓存 + 按需检索，零信息丢失 |
| 向量检索增加启动延迟 | 🟡 中 | 首次请求下载模型，后续本地推理；BM25 fallback |
| 四阶晋升过度激进 | 🟡 中 | 保守策略：晋升需多重确认，用户可手动回退 |
| Ebbinghaus 遗忘过早 | 🟡 中 | 半衰期可调（默认 72h），高频访问强化 |
| 与现有 dream.Runner 冲突 | 🟡 中 | 分阶段集成，Phase 1-2 独立，Phase 3 集成 |
| CCR 缓存占用磁盘 | 🟢 低 | TTL 自动清理，压缩比控制总量 |

---

## 八、成功指标

| 指标 | 目标 | 测量方式 |
|------|------|---------|
| **上下文命中率** | > 95% 会话不因超限中断 | 监控 `context_overflow` 事件 |
| **跨会话记忆恢复率** | > 90% 关键决策可恢复 | LongMemEval-S 基准 |
| **Token 节省** | > 50% 输入 token（长会话） | `aikota perf` 工具 |
| **检索延迟 p50** | < 50ms（BM25）< 200ms（Hybrid） | 压测 `memory_search` |
| **压缩延迟 p50** | < 2ms @ 100K tokens | `aikota perf` |
| **记忆准确率 R@5** | > 95% | coding-agent-life-v1 基准 |
| **遗忘准确率** | > 80% 被遗忘记忆确实已过时 | 人工抽样审计 |

---

## 九、与 agentmemory / Headroom 的关系

| 维度 | agentmemory | Headroom | aikota 大脑系统 |
|------|-------------|----------|----------------|
| **记忆持久化** | ✅ | ❌ | ✅ 继承 + 增强（四阶分层） |
| **上下文压缩** | ❌ 轻量 | ✅ 核心能力 | ✅ 继承 Live-zone + CCR |
| **跨代理共享** | ✅ | ✅ SharedContext | ✅ 单代理，跨会话共享 |
| **Live-zone** | ❌ | ✅ | ✅ 核心创新 |
| **CCR** | ❌ | ✅ | ✅ 核心创新 |
| **四阶整合** | ✅ | ❌ | ✅ 继承 + 集成 |
| **Hybrid Retrieval** | ✅ | ❌ | ✅ 继承 + 扩展 |
| **Hook 采集** | ✅ 12 条 | ❌ | ✅ 继承 + 新增实时捕获 |
| **向量检索** | ✅ | ❌ | ✅ 新增 |
| **Ebbinghaus 衰减** | ✅ | ❌ | ✅ 新增 |
| **SessionStart 注入** | ✅ | ❌ | ✅ 增强（基于意图） |

**结论**：aikota 大脑系统 = agentmemory 的记忆架构 + Headroom 的压缩技术 + aikota 原有的 compaction/dream 基础设施。

---

## 十、附录：关键代码改动清单

### 新增文件（12 个）
```
internal/compaction/livezone.go       # Live-zone 压缩核心
internal/compaction/ccr.go            # CCR 原文缓存
internal/memory/tiers.go              # 四阶记忆类型定义
internal/memory/hybrid_search.go      # RRF 融合检索
internal/memory/decay.go              # Ebbinghaus 衰减
internal/dream/consolidation_pipeline.go  # 四阶晋升管道
internal/dream/tier_manager.go        # 层级管理器
internal/dream/sweep.go               # 衰减扫描 cron
internal/hooks/post_tool_use_realtime.go  # 实时捕获 hook
internal/agenttool/memory_retrieve_tool.go # CCR 检索工具
internal/runtime/memory_injection.go  # SessionStart 注入
internal/cli/memstatus/memory_tiers.go    # 记忆层级状态命令
```

### 修改文件（8 个）
```
internal/compaction/compact.go        # 替换为 Live-zone 模式
internal/compaction/tokens.go         # 增加 token 预算控制
internal/memory/store.go              # 增加向量索引支持
internal/memory/search.go             # 增加 HybridSearch 方法
internal/dream/runner.go              # 集成晋升管道
internal/hooks/protocol.go            # 新增 hook 类型
internal/runtime/loop.go              # 替换 compaction 调用
internal/runtime/prompt.go            # 集成记忆注入
```

---

*文档版本：v1.0 · 2026-09-22*
*基于 aikota 源码分析 + agentmemory + Headroom 设计综合*
