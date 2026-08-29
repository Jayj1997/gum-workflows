# 11: Concurrent checks and dogfood Workflow

**What to build:** 将四个质量 Node 组合成 gum-workflows 自身的开发用 dogfood Workflow，验证它们可在同一 In-place Project Workspace 上正常并发、各自留存 Result 且不向项目写入 Gum 产物。

**Blocked by:** 08: Go Coverage Check; 09: Go Race Check; 10: Go Cyclomatic Complexity Check

**Status:** ready-for-agent

- [ ] 开发用 dogfood Workflow 通过 `project.code` 将同一项目代码绑定到四个独立检查。
- [ ] Workflow 只是本项目验证资产，不建设内置 Workflow 库或通用 Quality Gate。
- [ ] 至少两个检查可同时进入 Running，无隐藏 Workspace Lease 或串行。
- [ ] 四个 Node Run 共享同一 Project Workspace，但使用不同日志/tool-output 目录。
- [ ] 每个节点产出符合 Schema 的独立 Quality Check Result，无下游消费者时仍可从历史查看。
- [ ] 实际 Host PATH smoke 记录 Go 工具链与诊断，不将 exit code 当作唯一结果来源。
- [ ] Gum 数据库、日志、Coverage profile、Analyzer 产物和 Result 全部位于配置的 Local Data Root。
- [ ] 并发验收测试使用 channel/callback 控制时序，不使用 sleep。

