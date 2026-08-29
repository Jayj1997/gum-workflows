# 11: Concurrent checks and dogfood Workflow

**What to build:** 将四个质量 Node 组合成 gum-workflows 自身的开发用 dogfood Workflow，验证它们可在同一 In-place Project Workspace 上正常并发、各自留存 Result 且不向项目写入 Gum 产物。

**Blocked by:** 08: Go Coverage Check; 09: Go Race Check; 10: Go Cyclomatic Complexity Check

**Status:** ready-for-human

- [x] 开发用 dogfood Workflow 通过 `project.code` 将同一项目代码绑定到四个独立检查。
- [x] Workflow 只是本项目验证资产，不建设内置 Workflow 库或通用 Quality Gate。
- [x] 至少两个检查可同时进入 Running，无隐藏 Workspace Lease 或串行。
- [x] 四个 Node Run 共享同一 Project Workspace，但使用不同日志/tool-output 目录。
- [x] 每个节点产出符合 Schema 的独立 Quality Check Result，无下游消费者时仍可从历史查看。
- [x] 实际 Host PATH smoke 记录 Go 工具链与诊断，不将 exit code 当作唯一结果来源。
- [x] Gum 数据库、日志、Coverage profile、Analyzer 产物和 Result 全部位于配置的 Local Data Root。
- [x] 并发验收测试使用 channel/callback 控制时序，不使用 sleep。

## Comments

- 2026-08-30：实现 `examples/dogfood`，无环 Workflow 默认最多四路并发，含环 Workflow 保持单路迭代；验收通过 `Engine.Run`、Filesystem Store 与真实 History Store 的公共 seam，用 channel 闸门证明四个检查可同时 Running，并验证共享 Workspace、独立 Node Run 路径、严格 Result Schema 和项目零 Gum 产物。
- 2026-08-30：Host PATH dogfood Run `d8ed6412-9a31-4a00-94d8-7c2ca5cde39a` 的四个检查在同一秒启动并全部形成成功 Node Run：go vet passed；Race passed 且本次观察到 0 race；Coverage 79.584% 低于默认 80，业务 verdict=failed；Complexity 最大 61、20 个函数超阈值，业务 verdict=failed。Smoke 同时修复了测试对 `GUM_WORKFLOWS_DATA_ROOT` 的环境泄漏，以及 Coverage Adapter 对非零工具链诊断和合法零 statement block 的误判。
