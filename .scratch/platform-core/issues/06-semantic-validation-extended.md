# 06: 语义校验扩展与环降提示

**What to build:** 两层校验的语义层按设计文档 §10 清单升级（入口规则一条除外，随 T09 落地）：TypeExpr 兼容检查替换现有 Kind 精确相等；executor 显式版本存在；llm/target_model 仅 agent 类节点合法且能经 resolver 解析（llm.yaml 经注入路径加载）；projects 恰好 1 个且路径存在为目录；环检测从错误降为 warning（无 human 环警告可能死循环，含 human 环不提示）。错误聚合、定位到 Node ID 与字段的既有风格不变；同时修复 OptionalInputs Kind 漏检（由 TypeExpr 检查天然覆盖）。fixture 目录沿用 valid/invalid 模式新增各场景。

**Blocked by:** 01, 02, 03, 04, 05（类型语言、定义注册表、新 Schema、resolver 全部就位后才有可校验对象）

**Status:** done

- [x] `node:` 引用存在、绑定端口在契约中声明、required 已绑定、from/输出存在（既有检查迁移到新字段名）
- [x] 端口类型兼容（consumer ⊇ producer）替换 Kind 相等；optional 端口同样校验
- [x] executor 显式版本存在于注册表；llm/target_model 仅 agent 合法 + resolver 解析（四象限错误均定位到节点与字段）
- [x] projects 恰一 + 路径检查
- [x] 环检测：无 human 环 -> warning（非 error）；含 human 环不提示；既有 invalid-cycle fixture 语义更新
- [x] 新 fixture 全覆盖 + 既有语义测试迁移绿

## Comments

**2026-08-27（agent 实施记录）：** 语义校验器按设计文档 §10 清单扩展（入口规则 #7 随 T09 落地，#8 human 节点 dependsOn 必填在本票一并补齐）。

交付内容：
1. `SemanticValidator` 形态演进：`Validate` 返回 `([]Warning, error)`，环降为 warning（非 error）。新增 `Option` 注入机制（沿 execution 包模式）：`WithLLMConfig`（agent 节点解析链）、`WithWorkflowFile`（projects 相对路径锚点）。
2. 新增检查：
   - **检查 #2** executor 显式版本经 `ExecutorRegistry.Get` 精确命中，缺省经 `Latest` 解析；错误定位 `node <id> executor:`。
   - **检查 #8** human 类节点带输入但无 dependsOn 报错（`checkHumanControlEdge`）。
   - **检查 #9** `checkLLM`：llm/target_model 仅 agent 合法（逐字段定位，两非法字段各自报出）；agent 节点统一走默认链解析（含两字段都空）；llm.yaml 缺失时 workflow 含 agent 节点则报错、纯 automation/human 放行。查询路径经 `llm.CandidatePaths()`（新增导出）共享，避免查找顺序知识漂移。
   - **检查 #11** `checkProjects`：恰好 1 个 + 相对路径相对 workflow 文件解析 + 路径存在且为目录；未注入文件锚点时跳过路径检查（数量检查仍生效）。
   - **检查 #10** `checkCycle`：无 human 环 -> warning（含收敛保护兜底说明）；含 human 环 -> 合法迭代路径不提示。
3. 端口类型兼容（§10 #4、#3）沿用既有 `definition.Compatible`（consumer ⊇ producer，无隐式子类型）；optional 端口经遍历 `spec.Inputs` 天然覆盖；Kind 注册检查（含 optional 输入）由 `ValidateKinds` 覆盖（已有，注释明确）。
4. CLI 接线：`loadAndValidate` 加载 llm.yaml（`LoadDefault`，`ErrConfigNotFound` 以 nil 注入）并注入校验器；warning 经 `printWarnings`（validate/run 共用）输出到 stderr，不阻断。

fixture 全覆盖（`testdata/` 沿用 valid/invalid/warning 模式；`warning-cycle/` 替代旧 `invalid-cycle/` 反映环降提示）：
- `warning-cycle/{data,control}-cycle.yaml`：无 human 环 -> warning（非 error）。
- `valid/human-cycle.yaml`：含 human 环（coder→review control 边 + review.advise→coder data 边）-> 合法，不提示。
- `valid/union-port.yaml`：TypeExpr 正向兼容（union `markdown|OpenAPI` 接受 OpenAPI 生产者）。
- `invalid-executor/unknown-version.yaml`、`invalid-llm/{non-agent-node,unknown-provider,unknown-model,cross-provider-model}.yaml`、`invalid-projects/{zero-entries,missing-dir}.yaml`、`invalid-human/input-without-depends-on.yaml`、`invalid-type/optional-port-mismatch.yaml`。
- 全部 fixture 的 `projects.repository` 统一指向共享 `testdata/examples/order-system`（真实目录）。

代码评审（Standards + Spec 双轴并行）修复：
- `checkProjects` 的 `%v` 改 `%w`（DEVELOPMENT.md §4.2）；导出 `Warning.Message` 补 doc comment（§4.1）。
- llm 查找路径知识收口到 `llm.CandidatePaths()`，消除 validation 内的硬编码重复。
- `loadAndValidate` 返回值扁平化（去掉过度设计的 `validateOutcome` 包装）；`printWarnings` 抽出，消除 validate/run 重复。
- 测试 `fakeFactory.NodeType()` 在 validation 与 tests/dag 两处同构（消除 nodeType 默认值逻辑重复）。
- 文档同步：CLAUDE.md 约束 #7「无环」改「环仅提示」；DEVELOPMENT.md §6 fixture 目录清单更新（`warning-cycle/` 等）。
- Spec 轴修复：原 `valid/human-cycle.yaml` 实为无环（测试空过）——重写为真实回环（coder↔review 双向边），豁免分支真正被覆盖；恢复误删的 `TestRunUsage`；补 §10 #8 检查与 fixture；补 TypeExpr 正向兼容 fixture 与可选端口不匹配 fixture；llm 四象限补齐 Q1（cross-provider）与 Q3（默认链提示补 llm）。

测试：`go vet ./...`、`go test ./...`（含 `-race`）全绿；`workflow validate` 在 examples/minimal 与 warning-cycle fixture 上手动验证通过。
