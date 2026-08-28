# 13: history CLI 三级查询

**What to build:** 只读查询命令（设计文档 §11、吸收 run-history §8）：`workflow history`（最近 20 条列表：RUN ID/WORKFLOW/STATUS/STARTED/DURATION/NODES，非 Pending 计数子查询）、`workflow history <run-id>`（运行详情 + 各节点概要，含轮次数）、`workflow history <run-id> <node-id>`（节点详情：默认最新轮，列出全部轮次的输入/输出引用）。run-id 支持 ≥8 位 UUID 前缀（多义报错列候选）；DB 不存在或无记录输出空态不报错；不加过滤/分页参数。不内联 artifact 内容（按 URI 查看为主，内联预览属后续演进）。

**Blocked by:** 12（有数据可查）

**Status:** done

- [x] 三级查询输出形态按设计文档 §11（列表/详情/节点明细）
- [x] UUID 前缀解析：唯一命中/多义候选/无命中三种行为
- [x] 空态：无 DB、无记录、无该节点均不报错
- [x] 节点详情展示各轮次（round 递增、error_kind、advise 的 `#advise-retry` from 标记可读）
- [x] CLI 测试：库层直写种子运行记录（e2e 不跑含 human 的 run--spec Testing Decisions 已确认），CLI 只做读验证

## Comments

**2026-08-29（agent 实施记录）：** `internal/history` 新增最近 20 条列表、Run UUID 前缀解析、运行详情与 Node 全轮次查询；列表的 NODES 进度按每个 Node 的最新轮计算，避免旧轮状态污染。CLI 以只读方式打开既有 DB；DB 不存在、无记录、Run/Node 无命中均输出空态且不写 `.workflow`。

三级 CLI 输出覆盖完整 Run 元数据、Node 最新概要与逐轮摘要，以及每轮输入/输出 ArtifactRef（名称、from、Kind、URI、Version）；即时 advise 的 `#advise-retry` 标记原样展示，不内联 Artifact 内容。e2e 通过 history Store 直接写种子后只执行读命令，另覆盖唯一/歧义/无命中和最短前缀约束。双轴审查后补齐“运行详情逐轮摘要”、最新轮 NODES 聚合，并统一使用 `execution.Status` 类型。
