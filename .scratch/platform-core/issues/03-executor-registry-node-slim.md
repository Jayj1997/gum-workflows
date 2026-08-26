# 03: ExecutorRegistry 切换与 Node 接口瘦身（wide refactor）

**What to build:** 把「节点契约 + 寻址」从 Go 代码迁到定义层的一次广域重构，按 expand–contract 推进（spec Testing Decisions 已确认此为唯一 wide refactor）：新注册表按 (definition, version) 注册 Factory，`Latest(definition)` 供缺省解析；Node 接口瘦身（移除 Type/InputSchema/OutputSchema，契约唯一来源 = Node Definition YAML）；内置四节点迁移到新注册形态。每批迁移后既有测试保持绿。交付后 CLI 启动做 Go/YAML 双向一致性检查：Go 注册的每个 (definition, version) 必须有种子 YAML 声明，反之亦然；任一不一致启动即报错。

建议批次：先建新注册表与旧并存（expand），迁移 builtins + engine + validation（一批），删除旧 Registry 与 Node 接口旧方法（contract）。

**Blocked by:** 02（定义层与种子）

**Status:** done

- [x] 新注册表：Register(factory) 按 (Definition(), Version()) 寻址，重复注册报错，Latest(definition) 返回最大版本
- [x] Node 接口只剩 Execute（签名不变）；Factory 增 Definition()/Version()
- [x] builtins 四节点全部迁移（Mock 行为不变；契约由种子 YAML 提供）
- [x] engine/实例化改用新注册表；运行期输出契约检查按 YAML 契约执行
- [x] 启动一致性检查：Go 注册集与种子声明集双向 diff，不一致报错（错误列出缺失/多余项）
- [x] 旧 Registry 与 Schema 移除后 `go vet ./...`、`go test ./...`（含 -race）全绿

## Comments

**2026-08-26（agent 实施记录）**：已交付于 commit `a4883c2`（main）。expand–contract 实际推进：因旧/新注册表签名不兼容且票面要求同批迁移 builtins+engine+validation，expand 阶段只落了 `ExecutorRegistry`+`ExecutorFactory`（与旧并存、独立测试），随后一批完成迁移与删旧。

交付内容：
1. `internal/node/executorregistry.go`：按 (definition, version) 注册，重复报错；`Latest` 数字比较（v10>v9，复用 `definition.CompareVersions`，去重了两处实现）；哨兵错误 `ErrExecutorNotFound`。
2. `internal/node/node.go`：Node 接口只余 `Execute`；删除 `Schema`、旧 `Factory`、`Registry`（registry.go 整文件删除）。
3. builtins 四节点迁移为 `xxxExecutor`；契约不再在 Go 声明。
4. engine：`NewEngine(executors, defs, store, logger, ...)`；实例化走 `Latest`（显式 executor 字段待票 05 的 Schema 变更接入 `Get(def, version)`）；运行期输出检查 `declaredOutputs` 按种子 YAML 解析 TypeExpr，经 `definition.MatchesKind`（兼容 KindRef 与原子类型同名携带）。
5. 语义校验器 `NewSemanticValidator(executors, defs, kinds)`：契约从 definition.Registry 读取；类型兼容（consumer ⊇ producer）替换 Kind 相等；OptionalInputs 漏检随之修复（spec Further Notes 末条）。
6. `builtins.CheckConsistency(executors, defs)` 双向 diff，错误逐项列出（"Go executors without YAML declaration: (x, v1)" / "executor YAML declarations without Go executor: (y, v1)"），CLI `loadAndValidate` 在注册后即调用（validate 与 run 共用管线）。

与票面/种子既状的口径决定：
1. **requirement-analysis 种子改为源节点（inputs: {}）**：02 落的种子带必填输入 `requirement: markdown`，但 workflow/v1 现有 Schema 无入口节点（human-input 属 T09），旧 fullstack 示例把 requirement-analysis 当源节点用——不改种子则 CLI validate/run 直接全红。§12 定稿表只写输入类型不写数量，源节点化与「无输入无依赖的 Node 是合法 Trigger」约束一致。examples/字段重写是票 05/14 的事，本票只做最小迁移保绿。
2. **原子类型输出以同名 Kind 携带**：`rationality: int` 在 Store 中 Kind="int"（`MatchesKind` 对 Atomic 同名匹配）。设计文档 §4 说原子类型 Data 存 Go 标量但未定 Kind 表示，此为最小可用解释。
3. **引擎测试不依赖内置节点集**：`testregs_test.go` 用内存 definition.Registry + 实现 `Contract()` 的 mock factory（与种子 YAML 同构），保持 Engine.Run 主接缝测试独立于内置实现。
4. 显式 `executor:` 版本固定属票 05（NodeSpec 加字段后接入），本票缺省解析即 Latest。

测试：`go vet ./...`、`go test ./...`、`go test -race ./...` 全绿；fullstack demo 跑通（产出 markdown/int/ArchitectureSpec/SourceCode/OpenAPI/FrontendSDK）。
