# 09: macOS Keychain Secret Adapter

**What to build:** 让 macOS 用户安全保存 Provider API Key，同时保持 Product Application 和测试不依赖具体凭据后端。桌面使用系统安全凭据存储，Browser Mock 和测试使用注入 Adapter。

**Blocked by:** 06: LLM Provider / Model 设置

**Status:** ready-for-agent

- [ ] Desktop UI 保存 API Key 后，SQLite 只包含不可逆推出明文的 Secret 引用。
- [ ] 读取和更新 Provider 时不会把明文 Key 返回给普通 ViewModel、日志或 Diagnostics。
- [ ] 删除 Provider 可以在用户确认后删除对应安全凭据。
- [ ] 测试 Adapter 支持环境变量或内存注入，不访问真实用户 Keychain。
- [ ] 安全凭据存储不可用时明确失败，不静默降级为 SQLite 明文。
- [ ] Secret Adapter 通过 Product Application 注入，不由领域方法内部创建。

