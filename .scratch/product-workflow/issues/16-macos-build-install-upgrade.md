# 16: macOS 构建、安装与升级闭环

**What to build:** 完成 P11 的真实 macOS 生命周期：应用可以重复构建、安装、启动和升级；Local Data Root、Keychain、schema migration、历史和诊断在安装态下保持一致。

**Blocked by:** 14: Product schema upgrade fixtures; 15: 日志脱敏与 Crash diagnostics

**Status:** ready-for-agent

- [ ] macOS 构建产物可在干净测试环境安装并启动。
- [ ] 首次启动创建正确的 Local Data Root 和 product database，不写用户项目目录。
- [ ] 安装态 Provider API Key 写入系统安全凭据存储，数据库只保存引用。
- [ ] 从上一 product schema 的安装态升级后，Workflow、Provider/Model 和 Run history 保持可用。
- [ ] 升级失败向用户展示可恢复错误，不覆盖原数据库。
- [ ] 最短真实 UI e2e 覆盖创建 Workflow、运行 OpenAI fixture、查看 Artifact、重启和历史查询。
- [ ] Windows 构建和对等行为不进入本票，但当前 Adapter/Application 边界不得依赖 macOS 领域语义。

