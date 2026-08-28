# 09: human-input 入口节点与多轮输入

**What to build:** 人工在环的第一半（设计文档 §7.1、§7.4）：HumanGateway 接口定义在 execution（RequestRound：input/approval/advise-retry 三类请求 + 展示上下文），fake 实现供测试注入；内置 human-input Node Definition（无 inputs、无 dependsOn、输出 `requirement: markdown`）与其 v1 执行器；入口规则校验（全 workflow 恰好一个源节点且其 definition type=human，错误级）；运行开始即等待第一轮输入，每轮产出新版本 requirement 级联全下游；空行结束本轮 + Continue/Finish 询问；Finish 后入口关闭，运行继续收敛剩余工作，静止后保持 Running 等待 Ctrl-C。CLI 侧 stdin 实现（薄壳）+ 非 TTY 守卫（stdin 非控制台且 workflow 含 human 节点 -> 启动即报错）。轮次输入/Finish 是真实人类事件，重置收敛计数（替换 T08 的模拟 hook）。

**Blocked by:** 06（语义校验就位以承接入口规则）, 08（迭代引擎的轮次/级联骨架）

**Status:** done

- [x] HumanGateway 接口与 RoundRequest/RoundResponse 形态（设计文档 §7.4）
- [x] human-input 定义 + 执行器 + 种子（契约进入 T02 的种子布局）
- [x] 入口规则校验：恰一源节点且为 human-input（错误级，fixture 覆盖：无源/多源/源非 human）
- [x] 多轮输入：第 2 轮需求级联全下游重跑（fake gateway 驱动断言）；Finish 后不再等待、运行继续收敛剩余
- [x] 非 TTY 守卫：管道 stdin 下含 human 节点的 run 启动即报错（e2e 正向断言错误信息）
- [x] stdin 实现单测：管道喂字节流断言提示文案与解析（不依赖 TTY）

## Comments

**2026-08-28（agent 实施记录）：** `internal/execution` 已定义统一的 `HumanGateway.RequestRound`、三类请求枚举、展示上下文与联合响应形态。`human-input/v1` 的 YAML 种子与 Go executor 已进入内置注册/一致性检查；Engine 对入口轮次经 gateway 阻塞获取需求，每轮生成新的 `requirement: markdown` Artifact 版本，等待该轮下游收敛后才请求下一轮。Continue 驱动下一轮，Finish 关闭入口；每次真实输入（含 Finish 决策）重置收敛保护计数。等待输入期间取消会把入口当前轮记 Failed，并把运行记 Stopped。

语义校验新增唯一源规则并覆盖无源、多源、源非 human 三类 fixture；既有 valid/warning fixture 与最小示例迁移到 human 入口。CLI 在定义导入、数据库和 execution 目录写入之前检查 stdin 是否为交互终端，非 TTY 含 human 节点立即给出明确错误；stdin adapter 的多行、空行结束、Continue/Finish、默认 Finish 与非法选择重试均由纯字节流单测覆盖。由于规格明确不引入 pty 依赖，真实交互与两次运行的状态/定义持久化由可注入 gateway 的 CLI workflow seam 覆盖，数据库不可用、新版 executor 拒绝及 version-zero 迁移回归也迁入该 seam；真实二进制 e2e 聚焦 validate 零副作用与非 TTY 零写入路径。
