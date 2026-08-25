# 04: llm.yaml 加载与默认解析链

**What to build:** 用户级 LLM 配置的加载与解析（设计文档 §3.4）：`llm/v1` 信封、providers->models 嵌套、XDG 路径寻址（`$XDG_CONFIG_HOME/gum-workflows/llm.yaml` -> `~/.config/gum-workflows/llm.yaml`）、apikey 的 `$VAR` 环境变量引用（加载时解析，变量缺失报错并指明变量名）、默认解析链四象限（llm 填/空 × target_model 填/空）。本包不含任何网络客户端。交付后：给定一个 Node Instance 的 llm/target_model 引用，resolver 输出 (provider, model) 或定位准确的错误。

默认链：llm 填+model 填 -> 校验归属；llm 填+model 空 -> 该 provider 默认 model；llm 空+model 填 -> 默认 provider 内找该名（找不到报错提示补 llm）；都空 -> 默认 provider 的默认 model。默认 provider = 显式 default 或第一个；provider 内默认 model 同理。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] 加载与校验：provider 名唯一、model 名 provider 内唯一、default 各至多一个、type ∈ {openai-compatible, anthropic}、url/apikey/models 必填、temperature 等生成参数挂 model 级（默认 0.2）
- [ ] `$VAR` 解析：环境变量缺失时错误含变量名；明文 apikey 也允许
- [ ] XDG 寻址：两路径按序查找；文件不存在时返回可区分的哨兵错误（无 agent 节点的 workflow 合法地不需要它）
- [ ] resolver 四象限全覆盖（表驱动）
- [ ] 单测经临时 XDG_CONFIG_HOME 注入，不触真实 $HOME
