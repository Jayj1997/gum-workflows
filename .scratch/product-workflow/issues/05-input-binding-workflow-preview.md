# 05: Input binding 与只读 Workflow Preview

**What to build:** 让用户为 Node Input 选择上游 Output，并在只读自动布局 Preview 中理解实际 Data/Control Edge。Preview 在 Draft 未完成时仍显示整张图和聚合 Diagnostics。

**Blocked by:** 04: Node Catalog 与通用 Config Schema 表单

**Status:** complete

- [x] 用户可以把 `human-chat` 的 Conversation output 绑定到 `llm-chat` input，并形成 Data Edge。
- [x] Control Dependency 与 Data Edge 使用不同的创作控件和 Preview 表达。
- [x] 未绑定 Input、未知端口和类型不兼容均产生具体 Node/字段 Diagnostic。
- [x] Preview 在非法 Draft 下仍返回全部可识别 Node、Edge 和 Diagnostics。
- [x] 自动布局、缩放、折叠和最近选择不进入 Draft 语义哈希。
- [x] Preview ViewModel 不暴露具体渲染或布局库结构。

## Comments

- 2026-08-31：Application Preview、Browser Mock 与 Desktop 共用 UI 已完成。Catalog 公开 Conversation 端口合同；Input Binding 与 Control Dependency 分别创作并投影为 Data/Control Edge；非法 Draft 聚合缺失绑定、未知端口/来源、类型不兼容等具体 Diagnostic，同时保留可识别图与循环组。前端按依赖层自动布局，缩放、折叠和最近选择只存在于视图状态。
- 验证通过前端 21 项 WorkflowClient/DOM 合同测试、实际 Browser Mock 两节点连接检查、`go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...` 与 `git diff --check`。
- Standards/Spec 双轴审查完成并复核关闭全部硬性发现：未知目标 Definition 下仍保留可识别 Edge；Preview Node、Edge 与 Diagnostic 可选择或定位，Diagnostic 继续聚焦具体 Input/Config 字段；同层 Node 按稳定 ID 排序。保留 Go Application 与离线 Browser Mock 的双合同、双静态入口和当前 DOM view 聚合，作为无前端构建系统下的非阻断重复判断。
