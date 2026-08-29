# 05: Workflow Context Binding and `code` contract

**What to build:** 建立类型化 Workflow Context Binding 与 `code: SourceCode` 合同，使开发型 Workflow 可由 `backend.code` 触发检查，纯检查 Workflow 可由 `project.code` 直接引用 In-place Project Workspace。

**Blocked by:** 03: In-place Project Workspace

**Status:** ready-for-agent

- [ ] Workflow Context Binding 传递类型化 ArtifactRef，不使用字符串模板、OS 环境变量或源码内联。
- [ ] `project.code` 解析为指向 In-place Project Workspace 的 SourceCode ArtifactRef，不创建代码副本。
- [ ] Coding Agent 成功 Node Run 发布新版本 `code` Artifact；失败 Node Run 不发布。
- [ ] 新 `code` Version 使绑定的下游 Node 按现有 dirty/Ready 规则重新运行。
- [ ] Validation 聚合报告上下文名称不存在、类型不兼容与非法绑定。
- [ ] Run 历史保留每个 Node Run 实际消费的 code ArtifactRef，但不声称可据此恢复历史源码。
- [ ] 集成测试覆盖 `backend.code` 和 `project.code` 两条路径。

