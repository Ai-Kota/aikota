# docs/ 索引

> `docs/` 混着四类东西：**手写文档**、**发布站点**、**生成物**、**上游归档**。
> 本页说清哪份是干什么的、不同角色该从哪份读起。**先读本页，别翻目录。**

---

## 按角色读

| 你是 | 先读 | 再读 |
|---|---|---|
| **用户**（用 aikota 干活） | 仓库根 [README](../README.md) | [沙箱与容器化](sandboxing.md)（需要隔离时）|
| **嵌入方**（用 SDK 写程序） | 根 README 的[作为 SDK 嵌入](../README.md#作为-sdk-嵌入) 与 [`agent/` 包文档](../agent/doc.go)（包级 doc 注释即 SDK 说明） | [providers.md](providers.md)（要接管模型连接时）|
| **贡献者**（改 aikota） | [开发笔记](development-notes.md) ← **在 Windows 上开发必读** | [能力路线](capability-roadmap.md)（了解方向与取舍）+ [开发计划](development-plan-2026-09-30.md)（执行路线图）+ [测试规划](test-plan-2026-09-30.md)（测试策略）|
| **编排/集成方**（把 aikota 接进别的系统） | [能力路线 §4 第 1 层](capability-roadmap.md) 的事件面说明 | [providers.md](providers.md) |

---

## 一、手写文档（7 份，随代码演进）

| 文档 | 讲什么 | 性质 |
|---|---|---|
| [**providers.md**](providers.md) | 模型连接的**内部机制**：38 个 provider 塌缩到 3 个驱动、端点拼接与 `/v1` 坑、认证顺序、传输重试、双失败模型 | 稳定 |
| [**development-notes.md**](development-notes.md) | **本地过、CI 挂**的那些坑（8.3 短名 / CRLF golden / exec 位 / `sh` 吃反斜杠），含判定方法与 token 纪律 | 活文档 |
| [**sandboxing.md**](sandboxing.md) | 操作系统级隔离：Docker 整进程 / micro-VM / 进程级策略沙箱三种模式，与 trust、工具策略的组合矩阵 | 稳定 |
| [**capability-roadmap.md**](capability-roadmap.md) | **方向基线**：aikota 往哪长才叫"更强大"而不是"更臃肿"。含分层路线、归属原则、"基础设施不进产品"的判定程序 | **living document** |
| [**harness-capability-matrix.md**](harness-capability-matrix.md) | harness 六个维度的现状与缺口（对标 pi 与 Claude Code） | **living document** |
| [**development-plan-2026-09-30.md**](development-plan-2026-09-30.md) | **执行路线图**：Phase 1-4 详细计划、优先级排序、验收标准 | **living document** |
| [**test-plan-2026-09-30.md**](test-plan-2026-09-30.md) | **测试策略**：覆盖率目标、测试分层、Golden 文件管理、CI 门禁 | **living document** |

> 三份 living document 的规矩是：**只更新状态列，不新增章节**。落地一项就回去改一格。

---

## 二、发布站点（改动即上线）

```
index.html      站点首页
web/            6 个页面：architecture / configuration / features / install
                          / quickstart / slash-commands
CNAME           站点域名（pigo.rpcx.io —— 仍指向旧名，见下）
favicon.svg     站点图标
install.sh      一键安装脚本（站点也提供下载）
```

**这些是公开可见的**，改动等于发布。改之前确认措辞（见 [开发笔记 §1](development-notes.md)：README 曾把"不带 `-p` 进 TUI"错写成"进 REPL"，网站反而是对的）。

> ⚠️ **`CNAME` 仍是 `pigo.rpcx.io`**，与产品名不符。改它要动 DNS，属未决事项。

---

## 三、生成物（不要手改）

| 文件 | 来源 |
|---|---|
| `aikota-agent-loop.html` · `.workflow.json` | Agent 循环流程图 |
| `aikota-runtime.html` · `.architecture.json` | 运行时架构图 |
| `agent-loop-flowchart.html` | Agent 循环流程图（其源） |
| `class_agentcore.html` · `class_agenttool.html` · `class_provider.html` | 类型关系图 |
| `sequence_runtime.html` | 时序图 |

**手改会在下次重新生成时被覆盖。** 要改内容就改生成源。

---

## 四、上游归档（**刻意不动**）

| 文件 | 数量 | 为什么不改 |
|---|---|---|
| `issue#*.html` | **164** | 上游 smallnest/pigo 的 GitHub issue 归档——**别人写的历史记录**。改名或改内容等于**篡改史料**，且对产品零价值 |
| `superpowers/` | — | 历史 spec 存档 |

---

## 五、维护约定

1. **新增手写文档** → 回来更新本页的表格（否则等于没加）
2. **living document** → 落地一项改一格，不新增章节
3. **改站点** → 记住它是公开的
4. **别把 token、密钥、内部地址写进 `docs/`** —— 这里大部分内容会被发布

---

> 根目录也有一份面向用户的 [README](../README.md)。**本页是文档的导航，那份是产品的门面**，两者受众不同，不重复。
