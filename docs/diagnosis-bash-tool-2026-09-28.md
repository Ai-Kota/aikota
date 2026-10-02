# aikota bash 工具故障排查记录

**时间**：2026-09-28
**状态**：✅ 修复完成，待 TUI 验证
**关联项目**：E:\AImlyForge\tools\agent\aikota

---

## 症状

aikota TUI 中调用 `bash` 工具执行命令（如 `ls -la`、`find`）全部失败：
```
<3>WSL (11 - Relay) ERROR: CreateProcessCommon:818: execvpe(/bin/bash) failed: No such file or directory
bash: command exited with code 1
```

模型（auto:reasoning / anthropic）收到失败后无法恢复，输出 "note: empty response from the model"。

---

## 根本原因

**已安装的 aikota 二进制过旧，缺少 2026-09-16 提交的 WSL 修复。**

| 项目 | 时间 |
|------|------|
| `/d/Tools/aikota/aikota-bin.exe` 编译时间（旧） | 2026-09-15 11:26 |
| WSL 修复提交 `409160e`（probe shell candidates） | 2026-09-16 14:17 |
| WSL stderr 误判修复提交 `2a79868` | 2026-09-16 之后 |
| 当前源码 HEAD `560d326` | 2026-09-27 |

### 修复内容（`internal/agenttool/bash_tool.go`）

旧版行为：`resolveShell` 直接取 `exec.LookPath("bash")`，Windows 上 PATH 的 `bash` 可能是 `C:\Windows\System32\bash.exe`（WindowsApps 别名，路由到 WSL）。

新版行为：按顺序探测候选并用 `exit 0` 验证（5s 超时，检查 stderr 是否含 WSL 错误签名）：
1. Git Bash 安装树（`C:\Program Files\Git\usr\bin\bash.exe` 等）—— 不依赖 WSL
2. PATH 上的 `bash`
3. `wsl` launcher
4. `powershell` / `cmd`

### 本机环境状况

- **WSL 默认 distro 损坏**：`wsl.conf` 中有未知 key `automount.crossDistro`，导致 WSL 启动 `/bin/sh` 失败，但退出码为 0（误判陷阱）
- **Git Bash 健康**：`D:\Git\usr\bin\bash.exe` 可正常工作
- **修复后**：新代码应自动选中 Git Bash，跳过 WSL

---

## 诊断步骤

```bash
# 1. 确认 wsl 损坏
wsl -e /bin/bash -c "echo ok"
# 输出：<3>WSL (11 - Relay) ERROR: CreateProcessCommon:818: execvpe(D:/Git/usr/bin/bash) failed

# 2. 确认 Git Bash 正常
"D:/Git/usr/bin/bash.exe" -c "echo ok"
# 输出：ok

# 3. 检查路径上有哪些 bash
where.exe bash
# D:\Git\usr\bin\bash.exe
# C:\Windows\System32\bash.exe
# C:\Users\aimly\AppData\Local\Microsoft\WindowsApps\bash.exe

# 4. 确认源码包含修复
git -C "E:/AImlyForge/tools/agent/aikota" log --oneline -- internal/agenttool/bash_tool.go
# 409160e fix(agenttool): probe shell candidates, prefer Git Bash over broken WSL alias
```

---

## 修复执行

### 已完成的步骤

1. **确认源码 HEAD 包含修复** ✓
   - 本地 master 领先 origin/master 33 个 commit（未推送）
   - `go.mod` 有未提交的本地修改（添加 `ysmood/goob` 和 `ysmood/got` 依赖）

2. **从源码重建二进制** ✓
   ```bash
   cd E:/AImlyForge/tools/agent/aikota
   go build -o /d/Tools/aikota/aikota-bin.exe ./cmd/aikota
   ```
   - 构建成功（无错误）
   - 新二进制时间戳：2026-09-28 19:08
   - 文件大小：48,589,312 bytes（约 46.4 MB）

3. **验证新二进制功能** ✓
   - `--help` 输出正常，包含 `--shell` 参数说明
   - `--version` 返回：`aikota dev (commit none, built unknown)`
   - Git Bash 路径可正常启动二进制

### 待验证步骤

4. **TUI 内 bash 工具验证** ⏳
   - 启动 aikota TUI
   - 执行 `bash(ls -la)` 命令
   - 预期：正常返回目录列表，不再出现 WSL 错误

5. **shell probe 验证** ⏳
   - 确认 `resolveShell` 自动选择 `D:\Git\usr\bin\bash.exe`
   - 排除 WSL 损坏路径

---

## 验证结果

> ✅ 新二进制已构建验证通过，待 TUI 集成验证

| 验证项 | 状态 | 结果 |
|--------|------|------|
| 源码 HEAD 包含修复 | ✅ | `560d326` 包含 `409160e` 和 `2a79868` |
| 构建成功 | ✅ | `go build` 无错误，输出 `/d/Tools/aikota/aikota-bin.exe` |
| 新二进制时间戳 | ✅ | 2026-09-28 19:08 |
| `--shell` 参数存在 | ✅ | 确认存在，描述匹配代码逻辑 |
| `--version` 运行 | ✅ | 返回 `aikota dev (commit none, built unknown)` |
| `--help` 运行 | ✅ | 完整帮助信息正常输出 |
| Git Bash 功能测试 | ⏳ | 待 TUI 中验证 `bash(ls -la)` 等命令 |

**下一步**：启动 aikota TUI，执行 `bash(ls -la)` 确认不再报 WSL 错误。

---

## 相关文件

- `internal/agenttool/bash_tool.go` — shell 解析 + WSL 探测逻辑
- `internal/agenttool/wsl_probe_test.go` — 锁定本机构建环境的测试
- `D:\UserDotfiles\.config\aikota\config.toml` — 当前 aikota 配置
- `/d/Tools/aikota/aikota-bin.exe` — **已更新**（2026-09-28 19:08，含 WSL 修复）

---

## 后续建议

1. **推送本地 commits**：本地 master 领先 origin/master 33 个 commit，建议同步到远端
2. **修复 WSL distro**：清理 `wsl.conf` 中的 `automount.crossDistro` 无效 key
3. **发布新版本**：当 WSL 修复稳定后，考虑打 tag 并发布 Release 二进制
