# 05: workflow/v1 新 Schema 与临时示例迁移

**What to build:** workflow.yaml 的 Node Instance 新形态（设计文档 §3.6–§3.7）：`nodes.<id>.type` 改名 `node`，新增可选 `executor`/`llm`/`target_model`/`metadata`；`project`（单数）改 `projects`（列表，本票只做结构，恰一个的校验在 T06）；kind 值小写化。CUE schema、Go struct、loader 三者同步（DEVELOPMENT.md §5 规则）。旧 fullstack 示例退役（新契约下 requirement-analysis 有必填输入，旧示例必然失败且 human-input 源要到 T09 才存在--已与维护者确认此处置），替换为最小 human-free 临时示例（coding-agent 全 optional 输入当源 -> openapi-generator），CLI validate/run e2e 在临时示例上保持绿。

**Blocked by:** 03（新注册表；`node:` 字段引用 definition name）

**Status:** done

- [x] NodeSpec：node（必填）/executor/llm/target_model/metadata（可选）+ 既有 inputs/dependsOn/config 语义不变
- [x] `projects` 列表结构（name/repository，相对路径相对 workflow 文件）；`branch` 字段删除
- [x] CUE 与 Go struct 同步；loader 严格模式继续拒绝未知字段；kind 小写
- [x] 旧 examples/fullstack 退役（移入 git 历史；README 留一行说明待 T14 重写）
- [x] 临时示例可 `workflow validate` 通过、`workflow run` 跑通（无 human 节点）
- [x] 既有 e2e/测试改到临时示例后全绿

## Comments

**2026-08-26（agent 实施记录）**：Schema 三件套（`schema/workflow/v1.cue`、`internal/workflow/definition.go`、严格模式 loader）同步演进；全仓字段改名迁移（validation/execution/project/cmd/tests 四层测试与 fixture 全部换到新字段名）。

交付内容：
1. `NodeSpec`：`node`（必填，引用 Node Definition name）+ `executor`/`llm`/`target_model`/`metadata`（可选，全结构层就位；llm/target_model 仅 agent 合法的校验属 T06）；`Project`（单数 struct）改 `Projects []ProjectSpec`（name/repository，无 branch）；`KindWorkflow` 值小写化 `"workflow"`。`Definition.Validate` 增 projects 条目非空检查（数量恰一与路径存在属 T06）。
2. Engine 接入显式 executor 版本：`instantiate` 在 `spec.Executor != ""` 时走 `executors.Get(def, version)`，否则 `Latest`（设计文档 §3.6 版本固定语义）；`projectContext` / `cmd run` 改读 `projects[0]`。`project.Context`/`Spec` 删除 `Branch` 字段。
3. 种子回正：`requirement-analysis` 恢复 §12 定稿契约的必填输入 `requirement: markdown`（03 曾为保绿临时源节点化）；`MockCodingAgent` 与测试 mock agent 改为无条件产出 source-code + openapi（不再依赖 ArchitectureSpec 输入触发），支撑最小链 coding-agent 源节点形态。
4. examples/fullstack 删除（git 历史可寻），新增 `examples/minimal`（coder -> sdk，含 project 副本）；e2e（`tests/e2e/minimal_test.go`）与 CLI 测试全部改到临时示例。README/CLAUDE.md/DEVELOPMENT.md/domain-model.md 同步（含 fullstack 退役说明一行）。

与票面字样的口径决定：
1. **种子按 §12 定稿回正**（requirement-analysis 必填 requirement: markdown）：票面「旧示例必然失败」的前提即新契约生效，故本票同时回正种子并退役旧示例，不留 03 的临时源节点化偏差。
2. **`.gitignore` 的 `!examples/fullstack/project/.claude/**` 例外行改指 `examples/minimal`**：该例外是仓库既有约定（示例项目可带 skills 目录）的路径跟随，非新增行为。
3. **e2e 断言收窄到 3 类 Artifact**（SourceCode/OpenAPI/FrontendSDK）：最小链只有两个节点；§42 的 6 类 Artifact 验收随 T14 完整 demo 恢复。

测试：`go vet ./...`、`go test ./...`（含 `-race`）全绿；`workflow validate` 与 `workflow run` 在 examples/minimal 上手动验证通过。
