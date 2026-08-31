# 08: Revision reuse 与 Run history UI

**What to build:** 让用户理解“定义版本”和“执行次数”的区别：相同语义内容重复运行复用 Revision，但每次创建新 Run，并能在 UI 中查看 Revision、Run、Node Run 和 Artifact 历史。

**Blocked by:** 07: Fake StartRun、Revision 与 Conversation Artifact

**Status:** ready-for-agent

- [ ] 相同规范化语义哈希的 Draft 重复 StartRun 复用同一 Revision。
- [ ] 每次成功 StartRun 都创建新的 Run UUID，并分别记录 Node Run 和 Artifact。
- [ ] 修改执行语义或首次物化 Model UUID 会创建新的 Revision。
- [ ] 展示文案、Presentation Hint 和 UI view preference 变化不创建 Revision。
- [ ] UI 可以从 Workflow 查看 Revision 列表、每个 Revision 的 Runs 和每次 Run 的 Node/Artifact 摘要。
- [ ] 应用重启后 fake tracer 的历史仍可查询。

