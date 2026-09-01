# 09: macOS Keychain Secret Adapter

**What to build:** 让 macOS 用户安全保存 Provider API Key，同时保持 Product Application 和测试不依赖具体凭据后端。桌面使用系统安全凭据存储，Browser Mock 和测试使用注入 Adapter。

**Blocked by:** 06: LLM Provider / Model 设置

**Status:** complete

- [x] Desktop UI 保存 API Key 后，SQLite 只包含不可逆推出明文的 Secret 引用。
- [x] 读取和更新 Provider 时不会把明文 Key 返回给普通 ViewModel、日志或 Diagnostics。
- [x] 删除 Provider 可以在用户确认后删除对应安全凭据。
- [x] 测试 Adapter 支持环境变量或内存注入，不访问真实用户 Keychain。
- [x] 安全凭据存储不可用时明确失败，不静默降级为 SQLite 明文。
- [x] Secret Adapter 通过 Product Application 注入，不由领域方法内部创建。

## Comments

- 2026-09-01：新增 `internal/secret.Adapter`，Product Application 通过 `WithSecretAdapter` 注入。Desktop 组装 macOS Keychain 实现，Browser Mock 与 Go 测试使用 Memory Adapter；Provider ViewModel 只返回 `hasApiKey`。
- Desktop Provider 表单改为密码输入；创建与轮换由 Application 保存到外部凭据存储后把稳定引用交给 SQLite。空 Key 更新保留原凭据，删除必须携带 UI 确认并同步删除 Keychain 项。
- Keychain Adapter 通过 macOS Security.framework 的 `SecItemAdd` / `SecItemUpdate` / `SecItemCopyMatching` / `SecItemDelete` 直接管理 generic password，不经过命令参数；读写删除错误只返回脱敏上下文。测试使用注入 backend，不访问真实用户 Keychain。
- 聚焦验收覆盖 SQLite/ViewModel 无明文、创建/轮换/保留/确认删除、凭据不可用明确失败、Desktop Adapter 转发、Browser Mock 注入与 Keychain 原生边界。
