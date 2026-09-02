# 11: 删除 Model 后的悬空 UUID 诊断

**What to build:** 让用户安全删除 Provider/Model，同时不改写任何 Workflow。删除前展示受影响 Workflow；删除后相关 Node 保留原 Gum Model UUID、表单与 Preview 飘红，并在用户重新选择前阻止 StartRun。

**Blocked by:** 10: 真实 OpenAI-compatible 单轮闭环

**Status:** complete

- [x] 删除 Model 前 UI 展示引用该 Gum Model UUID 的当前 Workflow/Draft 数量与身份。
- [x] 删除 Provider 前 UI 展示其全部 Model Slot 及受影响 Workflow。
- [x] 用户确认删除后，不修改任何 Draft、Revision 或历史 Run。
- [x] 悬空 Model UUID 在 Node 表单和 Preview 中产生具体字段 Diagnostic。
- [x] 悬空 UUID 不 fallback 到 default，StartRun 在创建 Run 前失败。
- [x] 用户选择新 Model UUID 后 Draft 正常保存，并在下次 Run 形成新的 Revision。
- [x] 历史 Run Snapshot 仍展示已删除 Slot 当时的 Provider 名称和 Provider Model ID。

## Comments

- 2026-09-02：实施完成。新增 `LLMUsageRepository.ListProductWorkflowDraftModelReferences` 读 seam（Go 解码 Draft JSON，不依赖 SQLite JSON1 扩展），Application 提供 `ListModelDeletionImpact` / `ListProviderDeletionImpact` 用例：按未删除 Model UUID 过滤当前 Draft 引用，返回受影响 Workflow 身份（ID/显示名/Node ID/Node Definition）与 Provider 将移除的 Model Slot 清单（`modelSlots`：ID/显示名/Provider Model ID），查询本身零写入。Desktop Adapter 与两种 WorkflowClient 暴露同名方法（缺失时 `requireMethod` 显式失败，不静默伪造空影响）；DOM Delete 按钮先取 impact，把“移除哪些 Model Slot / 哪些 Draft Node 将悬空（含 workflow 与 Node 名单）”并入 confirm 文案再执行删除。
- Preview 在 agent Node 上新增三类字段级 Diagnostic：`dangling-model-uuid`（引用已删除/不存在 Slot）、`missing-model-uuid`（显式空偏好）与 `invalid-llm-preference`（llm 非 object），路径形如 `nodes[i].llm.modelUuid`；StartRun 复用同一活设置集合（现在带调用方 ctx）做 preflight，Draft 存在任一 Diagnostic 时在创建 Run/Revision 前失败且无任何写入。历史 Run 的 Revision Preview 与 Run Snapshot 保持权威：历史视图不针对当前设置重查悬空，`GetRunHistory` 仍返回当时固定的 ProviderName/ProviderModelID/ModelUUID。
- Node 表单补上 Model 选择器：agent Node 的编辑器渲染 "Model Slot" 下拉（live Provider -> Models，空值即 "Use default at Run"，悬空 UUID 显示为 "Deleted model ..." 并保持选中），`editorFocus` 支持 `nodes[i].llm.modelUuid` 定位；用户可直接在表单重新选择，选择变化走 `onEditNodeModel` autosave（清空等价于删除偏好，交回 default 物化）。
- Browser Mock 复刻同一行为：`createWorkflowPreview` 接受可选 `modelUUIDs` Set（缺省跳过悬空检查，供 history 使用），`startRun` preflight 与 live Draft Preview 均传入当前未删除 Model 集合；`modelDeletionImpact`/`providerDeletionImpact` 读取共享 in-memory Drafts 并同样返回 `modelSlots` 清单与 `nodeId`。验证：38 项前端合同测试、`go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...` 全部通过。
- 双轴 code review 后补齐：测试证明受阻 StartRun 零 Revision 残留、重新选择后下次 Run 形成新 immutable Revision；引用去重改按 (Workflow, NodeID, Model) 使同 Draft 多 agent Node 各计一条；Browser mock 的 modelDeletionImpact 与 Go 一致拒绝已删除 Slot。

