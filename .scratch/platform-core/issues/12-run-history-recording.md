# 12: 运行历史落库（node-run 粒度）与 run 摘要

**What to build:** 运行侧两张表与写入路径（设计文档 §8.3–§8.4，吸收 run-history 设计并按 node-run 改写）：RunRecorder 接口（定义在 execution，全量快照 upsert，与 persist 同点调用，Record 失败仅记日志不使运行失败）；运行开始即为全部节点建 Pending 行；每轮一行（UNIQUE(run_id, node_id, round)，id 列首插后不更新）；inputs/outputs JSON 沿 run-history §5.3 形态（即时 advise 以 `#advise-retry` 标记）；运行行 status ∈ Running/Stopped/Failed + stopped_reason；node 行 error_kind；RunID UUID 首次分配回填。CLI run 摘要扩展：WaitingHuman/轮次数/error_kind 展示。

**Blocked by:** 07（库基建与导入）, 08（轮次/Stopped 语义）, 09（WaitingHuman 行建立时机）, 10（审批轮次数据）, 11（error_kind/advise-retry 标记）

**Status:** ready-for-agent

- [ ] RunRecorder 接口 + Engine Option 注入（缺省不记录）；Record 与 persist 同点（snapshot 时机单测：开始/每轮变化/终态）
- [ ] 两表 DDL 与 upsert 幂等（同 exec 多次不重复、UUID 稳定）；RunID 首插回填
- [ ] 一轮一行：审批循环 e2e 后 review 节点多行、round 递增；RunID/round 往返一致
- [ ] Record 出错运行照常完成（假 recorder 注错断言）
- [ ] run 摘要输出 WaitingHuman/轮次/error_kind
