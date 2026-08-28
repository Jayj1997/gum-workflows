# 12: 运行历史落库（node-run 粒度）与 run 摘要

**What to build:** 运行侧两张表与写入路径（设计文档 §8.3–§8.4，吸收 run-history 设计并按 node-run 改写）：RunRecorder 接口（定义在 execution，全量快照 upsert，与 persist 同点调用，Record 失败仅记日志不使运行失败）；运行开始即为全部节点建 Pending 行；每轮一行（UNIQUE(run_id, node_id, round)，id 列首插后不更新）；inputs/outputs JSON 沿 run-history §5.3 形态（即时 advise 以 `#advise-retry` 标记）；运行行 status ∈ Running/Stopped/Failed + stopped_reason；node 行 error_kind；RunID UUID 首次分配回填。CLI run 摘要扩展：WaitingHuman/轮次数/error_kind 展示。

**Blocked by:** 07（库基建与导入）, 08（轮次/Stopped 语义）, 09（WaitingHuman 行建立时机）, 10（审批轮次数据）, 11（error_kind/advise-retry 标记）

**Status:** done

- [x] RunRecorder 接口 + Engine Option 注入（缺省不记录）；Record 与 persist 同点（snapshot 时机单测：开始/每轮变化/终态）
- [x] 两表 DDL 与 upsert 幂等（同 exec 多次不重复、UUID 稳定）；RunID 首插回填
- [x] 一轮一行：审批循环 e2e 后 review 节点多行、round 递增；RunID/round 往返一致
- [x] Record 出错运行照常完成（假 recorder 注错断言）
- [x] run 摘要输出 WaitingHuman/轮次/error_kind

## Comments

**2026-08-29（agent 实施记录）：** 新增 execution 消费方 `RunRecorder` 与 `WithRunRecorder`，普通快照沿用 Run context，取消后的终态快照使用 `context.WithoutCancel` 保证 Stopped 可落库；Record 与 state persist 同点，顺序为 Record 回填 UUID 后再写 state.json，记录失败仅写 warning。

SQLite schema v2 新增 `workflow_run_history` / `workflow_node_run_history`。运行开始为全部节点写 Pending round 0；首轮 Prepare 时以稳定 node-run UUID 原位推进为 Ready round 1，之后每轮一行，upsert 不更新 id。inputs/outputs 只保存 ArtifactRef，保留 `#advise-retry`，并记录 stopped_reason、error_kind、固定 executor/version 与 workflow version。

TDD 覆盖 Pending→Ready→Running/WaitingHuman→完成→终态快照、recorder 注错不中断运行、UUID/round 幂等往返、FK 级联、两轮审批 Engine→SQLite 集成，以及 run 摘要的 WaitingHuman/rounds/error_kind。双轴审查后修复 Ready 漏快照与 Pending round/ID 两侧不一致；Spec 复审无剩余行为缺口。文档总同步按票 14 的既定范围保留。
