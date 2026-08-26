# 01: TypeExpr 端口类型语言

**What to build:** Node Definition 契约端口的类型表达式体系（设计文档 §4）：原子类型（string/int/bool/float/markdown/file:ext）、语义 Kind、union（`|`）、list（`[T]`）的解析器，以及「消费者类型集合 ⊇ 生产者」的兼容判断函数。这是后续所有契约校验的地基：定义层加载、语义校验的类型兼容检查、engine 运行期输出检查都消费它。本票交付后，`definition.ParseTypeExpr` 与 `definition.Compatible` 可用且表驱动测试全绿。

**Blocked by:** None (can start immediately)

**Status:** done

- [x] 解析器覆盖：五种原子类型、`file:ext`（ext 为 [a-z0-9]+）、已注册 Kind、union（≥2 成员）、list（嵌套含 union 成员）
- [x] 语法错误信息定位到表达式位置（如 `foo|` 缺成员、`[unclosed`）
- [x] 兼容判断：原子/Kind 字面相等；list 递归；union 展开后成员包含（consumer ⊇ producer）
- [x] 无隐式子类型：`markdown` 不兼容 `string`、`file:jpg` 不兼容 `file`（要宽必须显式 union）
- [x] `file` 本期仅语言位（可解析、可比较），无真实产出/消费
- [x] 表驱动测试：全部语法形态 + 兼容矩阵（含嵌套 `[string|int]` vs `[string]`）

## Comments

**2026-08-25（agent 实施记录）**：已交付于 commits `9debaf8` + `60c66e2`（main）。`internal/definition` 新包：`ParseTypeExpr`（递归下降，错误携带表达式内 position）、`Compatible`（consumer ⊇ producer，无隐式子类型）、`Kinds`（Kind 引用提取，供语义层注册校验用）。85+ 表驱动子测试全绿，`go vet`/`gofmt` 干净。

两点与票面字样的偏差，均为有意的口径决定：
1. 「已注册 Kind」——解析层只识别大写驼峰形态，注册校验属语义层（设计文档 §10 检查 #3、§4 分工），经 `Kinds()` 提供；落地在 02/06。
2. 票面示例 `[unclosed` 实际报「未知标识符 unclosed」（位置 1-9），缺右括号的诊断由新增用例 `[markdown` 覆盖（`expected "]" to close list`）。
