# gum-workflows code-quality dogfood

`workflow.yaml` 将当前仓库通过 `project.code` 绑定到四个内置 Go Code Quality Check。四个检查没有彼此之间的 Data/Control Edge；human-input 入口完成后，它们会在同一个 In-place Project Workspace 上按 CLI 的四路并发度运行。

```bash
go run ./cmd/workflow validate examples/dogfood/workflow.yaml
go run ./cmd/workflow run examples/dogfood/workflow.yaml
```

Run 启动后输入任意一行并以空行结束。四个检查都完成后，Workflow 会保持 Running；使用 Ctrl-C 将其记录为 Stopped，再通过 `workflow history` 查看四个独立的 `QualityCheckResult`、工具链诊断和日志引用。

Coverage 阈值使用内置默认值 80%。质量发现或阈值未达标会产生 `verdict=failed`，但仍是成功的 Node Run；只有工具、协议或产物不完整才会使 Workflow 结构性失败。数据库、Artifact、日志与 tool-output 使用配置的 Local Data Root，不写入本仓库。
