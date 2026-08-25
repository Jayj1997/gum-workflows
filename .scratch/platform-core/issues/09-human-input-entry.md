# 09: human-input 入口节点与多轮输入

**What to build:** 人工在环的第一半（设计文档 §7.1、§7.4）：HumanGateway 接口定义在 execution（RequestRound：input/approval/advise-retry 三类请求 + 展示上下文），fake 实现供测试注入；内置 human-input Node Definition（无 inputs、无 dependsOn、输出 `requirement: markdown`）与其 v1 执行器；入口规则校验（全 workflow 恰好一个源节点且其 definition type=human，错误级）；运行开始即等待第一轮输入，每轮产出新版本 requirement 级联全下游；空行结束本轮 + Continue/Finish 询问；Finish 后入口关闭，运行继续收敛剩余工作，静止后保持 Running 等待 Ctrl-C。CLI 侧 stdin 实现（薄壳）+ 非 TTY 守卫（stdin 非控制台且 workflow 含 human 节点 -> 启动即报错）。轮次输入/Finish 是真实人类事件，重置收敛计数（替换 T08 的模拟 hook）。

**Blocked by:** 06（语义校验就位以承接入口规则）, 08（迭代引擎的轮次/级联骨架）

**Status:** ready-for-agent

- [ ] HumanGateway 接口与 RoundRequest/RoundResponse 形态（设计文档 §7.4）
- [ ] human-input 定义 + 执行器 + 种子（契约进入 T02 的种子布局）
- [ ] 入口规则校验：恰一源节点且为 human-input（错误级，fixture 覆盖：无源/多源/源非 human）
- [ ] 多轮输入：第 2 轮需求级联全下游重跑（fake gateway 驱动断言）；Finish 后不再等待、运行继续收敛剩余
- [ ] 非 TTY 守卫：管道 stdin 下含 human 节点的 run 启动即报错（e2e 正向断言错误信息）
- [ ] stdin 实现单测：管道喂字节流断言提示文案与解析（不依赖 TTY）
