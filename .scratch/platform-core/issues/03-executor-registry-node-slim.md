# 03: ExecutorRegistry 切换与 Node 接口瘦身（wide refactor）

**What to build:** 把「节点契约 + 寻址」从 Go 代码迁到定义层的一次广域重构，按 expand–contract 推进（spec Testing Decisions 已确认此为唯一 wide refactor）：新注册表按 (definition, version) 注册 Factory，`Latest(definition)` 供缺省解析；Node 接口瘦身（移除 Type/InputSchema/OutputSchema，契约唯一来源 = Node Definition YAML）；内置四节点迁移到新注册形态。每批迁移后既有测试保持绿。交付后 CLI 启动做 Go/YAML 双向一致性检查：Go 注册的每个 (definition, version) 必须有种子 YAML 声明，反之亦然；任一不一致启动即报错。

建议批次：先建新注册表与旧并存（expand），迁移 builtins + engine + validation（一批），删除旧 Registry 与 Node 接口旧方法（contract）。

**Blocked by:** 02（定义层与种子）

**Status:** ready-for-agent

- [ ] 新注册表：Register(factory) 按 (Definition(), Version()) 寻址，重复注册报错，Latest(definition) 返回最大版本
- [ ] Node 接口只剩 Execute（签名不变）；Factory 增 Definition()/Version()
- [ ] builtins 四节点全部迁移（Mock 行为不变；契约由种子 YAML 提供）
- [ ] engine/实例化改用新注册表；运行期输出契约检查按 YAML 契约执行
- [ ] 启动一致性检查：Go 注册集与种子声明集双向 diff，不一致报错（错误列出缺失/多余项）
- [ ] 旧 Registry 与 Schema 移除后 `go vet ./...`、`go test ./...`（含 -race）全绿
