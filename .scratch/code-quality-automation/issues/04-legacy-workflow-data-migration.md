# 04: Legacy `.workflow` data migration

**What to build:** 为需要保留旧项目内运行历史的用户提供显式、幂等且失败安全的一次性迁移操作，迁移后统一从 Local Data Root 查询。

**Blocked by:** 02: Local Data Root cutover

**Status:** ready-for-human

- [x] 迁移只在用户显式发起时读取指定 Project 的 legacy 数据。
- [x] 定义、Workflow、Run、Node Run、Artifact 引用与仍需保留的 Artifact 本体依已定义身份迁入新事实来源。
- [x] 重复执行迁移不会产生重复记录或新 ID。
- [x] 迁移失败不留下部分可见的 Run/Node Run 导入结果。
- [x] 迁移不修改、删除或双写用户的 legacy 目录。
- [x] 迁移完成后，现有三级 history 查询返回与迁移前等价的历史语义。
- [x] 使用旧库/Artifact fixture 覆盖完整迁移、重放、冲突和回滚。
- [x] 新 Run 与迁移 Run 统一使用 Run UUID 作为 SQLite 主键和 Local Data Root 目录身份。
- [x] 删除 `execution-*` 序号分配、`execution_id` 双身份与无调用 legacy path helper。
