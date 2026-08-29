# 14: examples 重写、e2e 重整与文档同步

**What to build:** 收尾票：按设计文档附录 A 重写 fullstack demo（human 入口 + 审批循环 + 多轮需求 + advise 回环的完整 YAML）；e2e 重整（validate 绿 + 非 TTY 守卫 + history 种子查询为骨架，完整运行行为已由主接缝测试覆盖）；四份文档同步：CLAUDE.md（硬性约束 #4 修订为迭代语义/#8 增补本设计范围、当前状态、常用命令加 history）、DEVELOPMENT.md（§1 持久化与依赖 modernc.org/sqlite、§2 目录结构 definition/llm/history、§3 依赖图、§5 Schema 字段集）、domain-model.md 按新架构重写、run-history 设计稿顶部标注「已被平台核心设计吸收取代」；设计文档 §13 里程碑表补记「运行历史落库从 P4 移至 P7 阶段（T12）」的修订行。可选加分项：macOS `script` 命令分配 pty 跑一次真交互 run（非主线验收）。

**Blocked by:** 11（错误二分法就位，demo 演示交互性错误可选）, 12（run 摘要形态定稿）, 13（history 查询与 demo 演示联动）

**Status:** resolved

- [x] examples/fullstack 重写为附录 A 形态；`workflow validate` 通过
- [x] e2e 骨架绿：validate / 非 TTY 守卫 / history 种子查询
- [x] CLAUDE.md / DEVELOPMENT.md / domain-model.md / run-history 稿四处修订完成（内容对照设计文档 §15 清单）
- [x] 设计文档 §13 补记里程碑偏离修订行（T12 推迟 + e2e 接缝决策）
- [x] `go vet ./...`、`go test ./...`（含 -race）全绿；新架构下全部测试无对旧词汇的引用残留

## Comments

- 2026-08-29：附录 A fullstack Demo、真实 CLI e2e 骨架与文档同步完成。移除测试专用 `gumworkflowe2e` 终端绕过；完整人工循环继续由 `Engine.Run` 的 fake HumanGateway / RunRecorder 接缝测试覆盖。
- 验证：`go vet ./...`、`GOCACHE=/private/tmp/gum-workflows-go-cache go test ./... -count=1`、`GOCACHE=/private/tmp/gum-workflows-go-cache-race go test -race ./... -count=1`、`git diff --check` 均通过。
