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

**2026-08-27（agent 实施记录）**：

交付内容：
1. `Validate` 签名改为 `([]Warning, error)`：环从错误降为提示（§6.7/§10 #10）--无 human 环 -> warning（含收敛保护兜底说明），含 human 环静默放行。新增 `Warning` 类型；CLI validate/run 共用 `printWarnings`（stderr 前缀 warning）。
2. executor 显式版本检查（§10 #2）：`spec.Executor != ""` 时走 `executors.Get` 精确命中（此前语义层只做 Latest，显式版本写错要到 run 的 instantiate 才炸）。
3. llm/target_model 检查（§10 #9）：非 agent 节点报错（逐字段定位）；agent 节点统一经 `llm.Config.Resolve` 默认链解析（含双空缺省解析）。`NewSemanticValidator` 增 `WithLLMConfig` / `WithWorkflowFile` Option；cmd 层经 `llm.LoadDefault` 注入，`ErrConfigNotFound` 时 nil 注入（无 agent 节点放行、有则聚合报错，错误列实际查找路径，取自新增 `llm.CandidatePaths()`）。
4. projects 恰一 + 路径存在且为目录（§10 #11）：相对路径相对 workflow 文件；无文件锚点（内存形态）时跳过路径检查。
5. human 非源节点 dependsOn 必填（§10 #8）：human 节点有 inputs 且无 dependsOn 时报错（入口规则 #7 仍归票 09）。
6. fixture：`invalid-cycle/` 语义更新为 `warning-cycle/`（data/control 两份）；新增 invalid-executor / invalid-llm（非 agent 节点、未知 provider、默认链未知 model、跨 provider model）/ invalid-projects（0 条、目录不存在）/ invalid-type/optional-port-mismatch / invalid-human/input-without-depends-on / valid/union-port（正向 union 兼容）/ valid/human-cycle（合法审批回环）；全部 fixture 的 projects.repository 收敛到共享 `testdata/examples/order-system`。
7. 同步文档：CLAUDE.md 约束 #7「无环」改「环仅提示」；DEVELOPMENT.md §6 fixture 目录表更新。
8. 测试迁移：`Validate` 双返回值波及 tests/dag 冒烟与 CLI 测试；tests/dag 的 `newRegistries` 一并注入 llm 配置与 nodeType 分类。

口径决定：
1. **agent 节点统一走默认链解析**（含两字段全空）：缺省解析也是运行前提，留到 run 才失败就晚了；此为 §10 #9「llm 引用解析」的自然延伸，非新增检查项。
2. **`llm.CandidatePaths()` 导出**：错误信息与 LoadDefault 共用查找路径知识，避免漂移（评审发现）。
3. **`invalid-cycle` 改名 `warning-cycle`**：fixture 目录名承载「这是错误还是提示」的语义。

测试：`go vet`、`gofmt -l`（空）、`go test ./...` 与 `-race` 全绿；CLI 手动验证 warning 展示、四类新增错误、无 llm.yaml 放行/拒绝、示例 run 不受影响。
