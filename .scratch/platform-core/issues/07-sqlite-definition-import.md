# 07: SQLite 统一库：迁移、定义侧导入、validate 零副作用

**What to build:** 项目本地 SQLite 统一库的基建与定义侧写入（设计文档 §8.1–§8.3）：`.workflow/gum-workflows.db`、modernc.org/sqlite 驱动、WAL/busy_timeout/user_version 顺序迁移；run 启动时隐式导入内嵌种子与本次 workflow（定义侧五表，UUID 主键、幂等 upsert、workflow 按 (name, version) 覆盖、node_instance 记解析后固定的 executor id 与 llm 名字字符串）。llm.yaml 继续不落库。validate 保持纯只读。运行历史落库**不在本票**（轮次/Stopped/error_kind 语义 T08–T11 才存在，已与维护者确认推迟到 T12；设计文档里程碑表的该行由 T14 修订）。

**Blocked by:** 03（定义注册表与种子导入源）, 04（llm 解析结果写入 node_instance 行）, 05（新 Schema 的 workflow/projects/node instance 结构）

**Status:** ready-for-agent

- [ ] history 包：Open（建库+迁移幂等，重复 Open 不重放）、导入 API
- [ ] 定义侧五表 DDL 按设计文档 §8.3；FK + 级联
- [ ] run 启动导入：种子 + workflow + projects + node instances（executor 解析固定为 id；llm/model 为名字字符串）；重复 run 幂等（同键覆盖、UUID 稳定）
- [ ] e2e：run 后 sqlite3 查五表内容正确；validate 不产生 DB 文件
- [ ] 导入失败不影响引擎既有行为（导入在 run 主流程内，失败即运行启动失败并明确报错）
