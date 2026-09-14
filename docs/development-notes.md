# 开发笔记：本地过、CI 挂的那些坑

> 面向**在 Windows 上开发 aikota 的人**（包括 AI）。
> 本文记录的是**环境差异**造成的失败——代码没错、测试也没错，但两者对"同一个东西"的表示不同。

---

## 1. 最重要的一条：本地绿 ≠ CI 绿

CI 跑两个平台（`.github/workflows/ci.yml`）：`ubuntu-latest` 与 `windows-latest`。

**在这台开发机上全绿，不代表 CI 会绿。** 已经踩过两次，两次都是**路径的字符串表示不同**：

| 差异来源 | 表现 | 谁遇到 |
|---|---|---|
| **Windows 8.3 短名** | 同一目录有长/短两种写法 | GitHub Windows runner |
| **macOS 符号链接** | `/tmp` 与 `/private/tmp` | macOS |

### 实例：8.3 短名（2026-09-14，`180c058`）

`cmd/aikota` 的 `TestCwdChdirRootsEnv` 在 Windows CI 上失败：

```
env.Cwd = "C:\Users\RUNNER~1\AppData\Local\Temp\TestCwd...\001"     ← 短名
want    = "C:\Users\runneradmin\AppData\Local\Temp\TestCwd...\001"  ← 长名
```

**同一个目录，两种写法。**

**为什么本地测不出**：

| | 开发机 | GitHub runner |
|---|---|---|
| 临时目录 | `D:\Temp\…` | `C:\Users\runneradmin\AppData\Local\Temp\…` |
| 有 8.3 别名吗 | **没有** | **有**（`runneradmin` → `RUNNER~1`）|
| `os.Getwd()` 返回 | 长名 | **短名** |
| 测试结果 | ✅ 过 | ❌ 挂 |

`t.TempDir()` 返回长名，`os.Getwd()` 返回短名——**两侧只解析了一侧**，于是字符串不等。

**修法**：比较前**两侧都归一化**。

```go
want, err := filepath.EvalSymlinks(dir)          // t.TempDir() 侧
got,  err := filepath.EvalSymlinks(env.Cwd)      // 系统给的侧
if got != want { … }
```

**已验证 `EvalSymlinks` 确实能解析 8.3 短名**（不是假设）：在开发机上 `C:\PROGRA~1` → `C:\Program Files`。

> **推广**：任何时候比较"系统给的路径"和"你自己拼的路径"，都该先归一化。候选手段：`filepath.EvalSymlinks`（解开符号链接 + 8.3）、`os.SameFile`（比较文件身份而非字符串）。

---

## 2. 判定方法：先分清是哪条腿挂的

```bash
# 看最近几次运行的结论
gh run list --limit 5

# 看某次运行里每条腿（job）的结果
gh run view <run-id>

# 看某条腿的完整日志
gh run view <run-id> --log
```

**没有 `gh` 时用 API**（需要 token，见 §3）：

```bash
TOKEN=$(cat <token-file>)
# 最近运行的结论
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://api.github.com/repos/Ai-Kota/aikota/actions/runs?per_page=5" \
  | python -c "import sys,json;[print(r['name'],r['status'],r.get('conclusion'),r['head_sha'][:8]) for r in json.load(sys.stdin)['workflow_runs']]"

# 某次运行里各腿的结果
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://api.github.com/repos/Ai-Kota/aikota/actions/runs/<run-id>/jobs" \
  | python -c "import sys,json;[print(j['name'],j.get('conclusion')) for j in json.load(sys.stdin)['jobs']]"

# 某条腿的日志（找出失败的包与用例）
curl -sL -H "Authorization: Bearer $TOKEN" \
  "https://api.github.com/repos/Ai-Kota/aikota/actions/jobs/<job-id>/logs" > ci.log
grep -E 'FAIL' ci.log
```

> ⚠️ **日志有 `时间戳 ` 前缀**，`grep '^\s*--- FAIL'` 这类锚定行首的正则**匹配不到**，要用 `grep -E 'FAIL'`。这一点我踩过，白查了一轮。

---

## 3. token 的使用纪律

读 Actions 结果需要 token（仓库私有）。**纪律**：

| 规则 | 原因 |
|---|---|
| **不要把 token 贴进对话** | 会进入会话记录（`D:\ClaudeConfig\.claude\projects\…\*.jsonl`），永久留存 |
| **写进文件，读时不打印** | `echo "<token>" > /e/tmp/gh-token.txt`，程序用 `$(tr -d '\r\n' < 文件)` 读 |
| **用完立刻删文件** | 并去 GitHub 吊销 token |
| **过期时间设短** | 一次性用途，7 天足够 |
| **最小权限** | 只需 `Administration: write`（改默认分支）+ `Actions: Read`（看 CI）|

---

## 4. 已踩过的清单（遇到新问题时先查这里）

| 日期 | 症状 | 根因 | 修法 |
|---|---|---|---|
| 2026-09-14 | Windows CI `TestCwdChdirRootsEnv` 挂，本地过 | 8.3 短名 vs 长名 | 两侧 `EvalSymlinks`（`180c058`）|
| 2026-09-14 | 三个 golden 测试在 Windows 挂 | 检出 CRLF，代码产出 LF | `.gitattributes` 锁 `*.golden text eol=lf`（`2ee5225`）|
| 2026-09-14 | 插件测试在 Windows 挂 | `mode&0o111` 在 Windows 恒为 0 | 改用扩展名判定（`6167ec7`）|
| 2026-09-14 | hooks 测试在 Windows 挂 | `sh -c` 吃掉 Windows 路径的反斜杠 | 路径 `filepath.ToSlash`（`9fdb02d`）|

---

## 5. 一条元教训

上面几条有一个共同点：**失败的都是"我以为已经验过"的地方**。

- "本地 37 包全过" —— 对 CI 不成立
- "工具表对照 `BuiltinTools` 一致" —— `BuiltinTools` 不是运行时全集（漏了 3 个 `schedule_*`）
- "`stopReason: error` 是上游偶发" —— 是多工具调用的必然失败

> **判据**：说"验过了"之前，先问**验的是不是同一个东西**。
> 源码级清单 ≠ 运行时清单；本地环境 ≠ CI 环境；一次成功 ≠ 稳定。
