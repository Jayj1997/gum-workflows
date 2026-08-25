# 06: 语义校验扩展与环降提示

**What to build:** 两层校验的语义层按设计文档 §10 清单升级（入口规则一条除外，随 T09 落地）：TypeExpr 兼容检查替换现有 Kind 精确相等；executor 显式版本存在；llm/target_model 仅 agent 类节点合法且能经 resolver 解析（llm.yaml 经注入路径加载）；projects 恰好 1 个且路径存在为目录；环检测从错误降为 warning（无 human 环警告可能死循环，含 human 环不提示）。错误聚合、定位到 Node ID 与字段的既有风格不变；同时修复 OptionalInputs Kind 漏检（由 TypeExpr 检查天然覆盖）。fixture 目录沿用 valid/invalid 模式新增各场景。

**Blocked by:** 01, 02, 03, 04, 05（类型语言、定义注册表、新 Schema、resolver 全部就位后才有可校验对象）

**Status:** ready-for-agent

- [ ] `node:` 引用存在、绑定端口在契约中声明、required 已绑定、from/输出存在（既有检查迁移到新字段名）
- [ ] 端口类型兼容（consumer ⊇ producer）替换 Kind 相等；optional 端口同样校验
- [ ] executor 显式版本存在于注册表；llm/target_model 仅 agent 合法 + resolver 解析（四象限错误均定位到节点与字段）
- [ ] projects 恰一 + 路径检查
- [ ] 环检测：无 human 环 -> warning（非 error）；含 human 环不提示；既有 invalid-cycle fixture 语义更新
- [ ] 新 fixture 全覆盖 + 既有语义测试迁移绿
