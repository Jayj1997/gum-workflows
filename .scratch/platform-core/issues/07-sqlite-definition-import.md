# 07: SQLite 统一库：迁移、定义侧导入、validate 零副作用

**What to build:** 项目本地 SQLite 统一库的基建与定义侧写入（设计文档 §8.1–§8.3）：`.workflow/gum-workflows.db`、modernc.org/sqlite 驱动、WAL/busy_timeout/user_version 顺序迁移；run 启动时隐式导入内嵌种子与本次 workflow（定义侧五表，UUID 主键、幂等 upsert、workflow 按 (name, version) 覆盖、node_instance 记解析后固定的 executor id 与 llm 名字字符串）。llm.yaml 继续不落库。validate 保持纯只读。运行历史落库**不在本票**（轮次/Stopped/error_kind 语义 T08–T11 才存在，已与维护者确认推迟到 T12；设计文档里程碑表的该行由 T14 修订）。

**Blocked by:** 03（定义注册表与种子导入源）, 04（llm 解析结果写入 node_instance 行）, 05（新 Schema 的 workflow/projects/node instance 结构）

**Status:** done

- [x] history 包：Open（建库+迁移幂等，重复 Open 不重放）、导入 API
- [x] 定义侧五表 DDL 按设计文档 §8.3；FK + 级联
- [x] run 启动导入：种子 + workflow + projects + node instances（executor 解析固定为 id；llm/model 为名字字符串）；重复 run 幂等（同键覆盖、UUID 稳定）
- [x] e2e：run 后 sqlite3 查五表内容正确；validate 不产生 DB 文件
- [x] 导入失败不影响引擎既有行为（导入在 run 主流程内，失败即运行启动失败并明确报错）

## Comments

**2026-08-28（agent 实施记录）：** 新增 `internal/history`，使用纯 Go `modernc.org/sqlite` 打开 `.workflow/gum-workflows.db`，设置 WAL / busy_timeout / foreign_keys，并以 `PRAGMA user_version` 顺序事务迁移定义侧五表。标准库 `database/sql` 不包含 SQLite 驱动，故按批准设计引入 modernc；定义侧主键要求 UUID，故把既有间接依赖 `google/uuid` 提升为直接依赖。定义与 workflow 导入按自然键覆盖式 upsert，保留既有 UUID；workflow 覆盖时清理已移除的 node instance。

`workflow run` 在创建 execution/workspace 前导入内嵌种子与本次 workflow，并把缺省 executor 固定为明确版本及对应数据库 UUID；agent 节点只记录解析后的 provider/model 名，不写入 llm.yaml 的 URL 或 API key。`workflow validate` 仅在数据库已存在时只读检查 executor 解析，不建库、不迁移、不写库。项目 Go 基线按维护者要求升级为 1.25.0。

测试覆盖 Open/迁移幂等、五表与 FK 级联、JSON 往返和空值形态、覆盖删除与 UUID 稳定；CLI e2e 使用系统 `sqlite3` 检查五表、解析结果、密钥不落库、重复 run 稳定、validate 不建库，以及数据库无法打开时在引擎启动前明确失败。`go vet ./...` 与 `go test -race ./...` 全绿。

代码评审（Standards + Spec 双轴并行）修复：`Open` 接收调用方 context；抽取稳定 UUID 分配逻辑；并发首次 Open 以 immediate transaction 串行迁移并在锁内读取 `user_version`；缺省 executor 同时比较数据库留存版本与二进制版本，数据库存在时 validate 以 query-only 方式执行同样检查；validate 遇到尚无定义表的 v0 库时按无留存版本处理且不迁移；run 严格在迁移与种子导入后解析（含既有 v0 DB 回归）；e2e 扩展为逐表核对定义、契约、执行器、project 与 node instance 内容。平台级持久化文档的集中同步仍按既定 T14 范围执行。
