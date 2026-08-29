# 12: Release integration and truthful documentation

**What to build:** 完成 Local Data Root、legacy 迁移、In-place Project Workspace 和四个质量 Node 的最终集成验收，并将已实现行为、后续待办和平台限制如实反映到项目文档。

**Blocked by:** 04: Legacy `.workflow` data migration; 11: Concurrent checks and dogfood Workflow

**Status:** ready-for-human

- [x] 完整 Go 测试、Race 与 Vet 通过，新增脚本合同/fixture 在 Darwin/Linux 稳定。
- [x] 真实 Host PATH smoke 成功产生 Static、Coverage、Race 和 Complexity Result，产物只在 Local Data Root。
- [x] Legacy 迁移、重放幂等、新旧位置不双写与三级 history 查询回归通过。
- [x] README/状态文档明确标识已实现四个 Go full-scope 检查、In-place Project Workspace 和 Local Data Root。
- [x] 领域词汇、开发规范、Node Catalog/配置说明与产品计划使用同一术语且不夸大 Race/Vet/Coverage 能力。
- [x] README 的唯一待办清单保留 Changed Scope、语言检测、条件/Skipped、Container、Windows/WSL 和用户自定义脚本；Fuzz 不建立承诺。
- [x] Windows 不支持、无自动 timeout、无安全沙箱、不保存/恢复历史源码等范围边界在用户文档中清晰可见。
- [x] 运行 dogfood 后用户项目不出现 `.workflow`、Coverage profile、Analyzer 产物或 Gum 日志。

## Comments

- 2026-08-30：发布集成完成。README 集中记录 Darwin/Linux、Host 非沙箱、无自动 timeout、历史源码不可恢复以及 Vet/Coverage/Race 的诚实能力边界；产品化计划同步为已完成 Local Data Root 单一事实来源与显式 legacy 迁移。macOS PTY e2e 改为按公开提示逐步输入终端 carriage return，并在首个成功 coding-agent Node Run 后验证 Run → Stopped → history，连续运行稳定。
- 2026-08-30：真实 Host PATH dogfood Run `23eb87d4-96e9-43d6-9fbe-75e14b306045` 的四个检查均形成 Succeeded Node Run 和独立 `QualityCheckResult`：Static passed；Race passed，本次观察到 0 race；Coverage 77.255% 低于默认 80，业务 verdict=failed；Complexity 最大 60、21 个函数超阈值，业务 verdict=failed。数据库、Bundle、日志、tool-output 与 Result 全部位于隔离 Local Data Root `/private/tmp/gum-release-smoke.EKorRk`；仓库内 legacy `.workflow` 时间戳保持 2026-08-27，且未产生项目级 Coverage profile、Analyzer 产物或 Gum 日志。
- 2026-08-30：Darwin 本机 `go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...`，以及 dogfood、legacy 迁移、三级 history、Bundle/Adapter/Validation 聚焦回归全部通过。
- 2026-08-30：双轴 review 的 Standards 侧无发现；Spec 侧指出缺少 Linux 实证。使用官方 `golang:1.25.0-bookworm`、非 root 用户、init reaper 与只读仓库挂载运行 Linux 聚焦合同和全量 `go test ./...`、`go vet ./...`、`go test -race ./...`，全部通过；同时修正 dogfood fake Go 写死 Darwin GOOS/GOARCH 的 fixture 问题。等待人工验收。
