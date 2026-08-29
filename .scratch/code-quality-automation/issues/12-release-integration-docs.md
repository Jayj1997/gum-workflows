# 12: Release integration and truthful documentation

**What to build:** 完成 Local Data Root、legacy 迁移、In-place Project Workspace 和四个质量 Node 的最终集成验收，并将已实现行为、后续待办和平台限制如实反映到项目文档。

**Blocked by:** 04: Legacy `.workflow` data migration; 11: Concurrent checks and dogfood Workflow

**Status:** ready-for-agent

- [ ] 完整 Go 测试、Race 与 Vet 通过，新增脚本合同/fixture 在 Darwin/Linux 稳定。
- [ ] 真实 Host PATH smoke 成功产生 Static、Coverage、Race 和 Complexity Result，产物只在 Local Data Root。
- [ ] Legacy 迁移、重放幂等、新旧位置不双写与三级 history 查询回归通过。
- [ ] README/状态文档明确标识已实现四个 Go full-scope 检查、In-place Project Workspace 和 Local Data Root。
- [ ] 领域词汇、开发规范、Node Catalog/配置说明与产品计划使用同一术语且不夸大 Race/Vet/Coverage 能力。
- [ ] README 的唯一待办清单保留 Changed Scope、语言检测、条件/Skipped、Container、Windows/WSL 和用户自定义脚本；Fuzz 不建立承诺。
- [ ] Windows 不支持、无自动 timeout、无安全沙箱、不保存/恢复历史源码等范围边界在用户文档中清晰可见。
- [ ] 运行 dogfood 后用户项目不出现 `.workflow`、Coverage profile、Analyzer 产物或 Gum 日志。
