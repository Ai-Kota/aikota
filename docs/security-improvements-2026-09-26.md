# aikota 安全改进报告 (2026-09-26)

## P0 优先级（立即修复）

### 1. 插件签名验证

**文件**: `internal/plugin/signature.go`

**功能**:
- ECDSA P-256 签名验证
- 支持单公钥或多公钥模式
- 签名文件约定: `<plugin>.exe.sig`
- 默认关闭,通过配置启用

**配置示例**:
```toml
# config.toml
[plugin.signature]
enabled = true
public_key = "-----BEGIN PUBLIC KEY-----\n..."
```

**使用方式**:
```bash
# 为插件签名
openssl ecparam -genkey -name prime256v1 -out priv.pem
# 用私钥签名插件二进制
# 将公钥添加到aikota配置
```

### 2. 命令白名单机制

**文件**: `internal/agenttool/command_filter.go`

**功能**:
- 命令白名单 (AllowedCommands)
- 命令黑名单 (DeniedCommands)
- 正则模式匹配拒绝 (DenyPatterns)
- 环境变量配置

**环境变量**:
```bash
# 只允许特定命令
AIKOTA_ALLOW_COMMANDS=git,go,cat

# 拒绝特定命令
AIKOTA_DENY_COMMANDS=sudo,rm,dd

# 正则模式拒绝
AIKOTA_DENY_PATTERNS="rm\s+-rf|curl\s+.*\|\s*sh"
```

**测试覆盖**: 6个测试用例全部通过

---

## P1 优先级（本月完成）

### 3. 静态分析CI集成

**文件**: `.github/workflows/security.yml`

**集成工具**:
- `gosec`: Go安全漏洞扫描
- `golangci-lint`: 代码风格检查
- `go vet`: Go静态分析

**触发条件**:
- 所有push到master
- 所有pull request

### 4. 统一超时配置

**文件**: `internal/runtime/global_config.go`

**集中管理的超时**:
| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| BashDefaultTimeout | 2分钟 | Shell命令默认超时 |
| BashMaxTimeout | 10分钟 | Shell命令最大超时 |
| StreamTimeout | 5分钟 | LLM流式响应超时 |
| PluginInitTimeout | 10秒 | 插件初始化握手超时 |
| HookTimeout | 60秒 | Hook执行超时 |

**向后兼容**: 默认值与原有硬编码一致

---

## 测试状态

```
38/38 测试包通过
```

---

## 后续建议 (P2 - 下季度)

1. **沙箱执行环境**: Docker容器隔离
2. **多租户架构**: 组织级权限隔离
3. **RLHF反馈循环**: 学习用户偏好
