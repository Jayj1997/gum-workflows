# 14: Product schema upgrade fixtures

**What to build:** 让安装了早期 Product Tracer 或首闭环版本的用户安全升级到完整 P11 schema。升级保留 Workflow、Draft、Revision、Provider/Model、Run 和 Artifact，并在失败时保持旧库可恢复。

**Blocked by:** 11: 删除 Model 后的悬空 UUID 诊断; 13: 重启后的 Run 与 Artifact 查询

**Status:** complete

- [x] 每个已发布 product schema version 都有代表性旧库 fixture 和顺序 upgrade 测试。
- [x] 升级保留 Draft lock version、Revision hash、Model Slot UUID 和历史 Run Snapshot。
- [x] 删除状态、显式 default 和 created-time fallback 在升级后保持确定行为。
- [x] migration 重放幂等，不复制 Revision、Run、Node Run 或 Artifact 元数据。
- [x] migration 失败不发布半迁移 schema 或半可见历史。
- [x] workflow/v1 现有定义与 Run history 在 Product schema 升级后仍通过原有查询和测试。

## Comments

- 2026-09-03：实施完成。新增 `internal/history/product_schema_upgrade_test.go`（fixture 构造）与 `product_schema_upgrade_sequential_test.go`（顺序升级合同测试）：`seedProductFixtureAt` 按版本参数应用 `migrations[:N]` 并以直接 SQL 写入该版本真实可持有的数据（Workflow/Draft lock_version=3/显式 default 的 Provider+Model/软删除的第二组 Provider+Model/Revision+semantic_hash/Run Snapshot 按版本历史形状手写 JSON/Node Run/Artifact 元数据）；Run Snapshot 形状按写入版本区分（v8 无 apiKeyRef/dialect，v9 加 apiKeyRef，v10 加 dialect），用当前 `ResolvedLLMSelection` 解码验证前向兼容。
- 断言覆盖四个合同测试：`TestProductSchemaUpgradePreservesDataAndQueryability` 对 5–11 每个已发布版本做表驱动顺序升级，断言 Workflow 身份与时间戳、Draft lock version 与内容（v5 回填 initial Draft）、显式/有效 default、developer dialect 默认、软删除行不复活（`ResolveLLMModel` 拒绝已删除 UUID）、`DeleteLLMProvider` 后 default 不再解析、Revision semantic hash 与规范化内容、Run Snapshot 的 ProviderName/ModelUUID/ProviderModelID、Node Run 与 Artifact 元数据。`latestUserVersion` 防护测试在新增 schema version 时强制扩展版本表。`TestProductSchemaUpgradeReopenDoesNotDuplicateHistory` 证明升级后重开是零迁移重放，Revision/Run/Node Run/Artifact/Draft 行数不复制。`TestProductSchemaUpgradeFailureLeavesOldDatabaseRecoverable` 预先添加 step 10 的 dialect 列使升级在 step 9 成功后失败，断言 `user_version` 保持旧值、已执行 step 的 DDL 被回滚（diagnostics_json 列不存在）、Draft lock version 与 Run 行原样可读。`TestProductSchemaUpgradeKeepsWorkflowV1HistoryUsable` 在同一库混合 workflow/v1 定义与 Run history，验证 `ListRuns`/`GetRun`/`GetNodeRun` 原查询、`ImportDefinitions` 幂等 upsert 与 Product 查询互不污染。
- 验证：`go build ./...`、`go test ./...`、`go vet ./...`、`go test -race ./...` 全部通过（race 首次运行有一次历史性时序失败，重跑与 `-count=3` 稳定通过）。双轴 Standards/Spec review 后修复：删除死代码 `writeFixtureConversationArtifact`、以 `testing.TB` 取代自定义 TB 接口、快照 JSON 用单一 builder 消除三段重复、fixture 增加软删除 Provider/Model 与 artifactID 字段、失败原子性测试改为真正可失败的 poison（预加 dialect 列）并断言 step 9 回滚。
