# 17: Human Chat Entry execution seam

**What to build:** 为 Product Workflow 增加真正的 Human Chat Entry 运行语义：它能在没有必需输入时自举，也能消费上一版 Conversation feedback，并为每次人工提交建立新的 WaitingHuman/Running/Succeeded Node Run。

**Blocked by:** 16: macOS 构建、安装与升级闭环

**Status:** ready-for-agent

- [ ] Product Validator 允许恰好一个没有必需输入、可自举的 Human Chat Entry。
- [ ] workflow/v1 的“唯一完全无 inputs human-input”规则和现有 fixtures 保持不变。
- [ ] Human Executor 能接收 optional Conversation feedback，并把它呈现给 Human Gateway。
- [ ] feedback 到达后创建新 Node Run 并进入 WaitingHuman，但不会自动产出 Artifact。
- [ ] 每次人工提交使当前 Human Node Run 继续并产生一个新的 Conversation 版本。
- [ ] 同一 Human Entry 仍保持 Node 单并发，取消和 Structural Error 遵守既有 Run 语义。
- [ ] 公共 Engine seam 测试覆盖自举、feedback、round identity、取消和 Convergence Guard，不断言私有队列。

