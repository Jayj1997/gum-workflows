# 03: In-place Project Workspace

**What to build:** 让 Project Definition 指向的用户项目目录直接成为 Agent 与 Automation 的 Project Workspace，使 Agent 修改实时生效，并彻底移除每 Run 项目复制。

**Blocked by:** 02: Local Data Root cutover

**Status:** ready-for-human

- [x] Project Context 中的 Workspace 解析为用户项目的规范化绝对路径。
- [x] `run` 不再将项目复制到 Local Data Root 或其他 Run 私有目录。
- [x] Agent Node 在 Workspace 中的修改立即出现在用户项目目录。
- [x] Automation Node 获得与 Agent 相同的 Workspace 路径。
- [x] Gum 自身的数据库、Artifact、日志与工具产物仍只写 Local Data Root。
- [x] Runtime 不创建代码 Snapshot/Revision，不执行 Git 提交、回滚或恢复。
- [x] 端到端测试证明项目不被复制，Agent 修改实时可见。
