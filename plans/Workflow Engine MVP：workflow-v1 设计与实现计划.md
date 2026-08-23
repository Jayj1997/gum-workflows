# Workflow Engine MVP：workflow/v1 设计与实现计划

## 1. 项目目标

构建一个基于 Go 的轻量级 Workflow Runtime。

核心目标：

> **使用 YAML 定义 Workflow，使用 Go 实现 Workflow Runtime，通过 Node 的 Input / Output Contract 自动形成 DAG，并按照 Artifact 数据依赖持续执行。**

MVP 不考虑 UI，不依赖 Temporal，不实现完整的分布式调度系统。

最终希望能够运行：

```text
Requirement
    ↓
Architecture
    ↓
Backend Coding Agent
    ↓
OpenAPI
    ↓
OpenAPI Generator
    ↓
Frontend Coding Agent
```

并且 Workflow 本身不需要因为 Node 的增删、顺序调整而修改 Go 代码。

---

# 2. 核心设计原则

## 2.1 Workflow 与 Node 解耦

Workflow 只负责：

- 声明有哪些 Node
- 配置 Node
- 建立数据连接
- 配置 Workflow 的运行环境

Node 负责：

- 定义 Input
- 定义 Output
- 实现具体执行逻辑

例如：

```text
Workflow
 ├── requirement-analysis
 ├── architecture-design
 ├── backend-implementation
 ├── openapi-generator
 └── frontend-implementation
```

Node 本身不属于任何固定 Workflow。

同一个 Node 可以被多个 Workflow 复用。

---

# 3. Workflow 不直接通过代码定义

Workflow 使用 YAML 定义。

例如：

```yaml
apiVersion: workflow/v1
kind: Workflow

metadata:
  name: fullstack-development

project:
  repository: ./examples/order-system
  branch: main

nodes:

  requirement:
    type: requirement-analysis

  architecture:
    type: architecture-design

    inputs:
      requirement:
        from: requirement.requirement

  backend:
    type: backend-implementation

    inputs:
      requirement:
        from: requirement.requirement

      architecture:
        from: architecture.architecture

  openapi:
    type: openapi-generator

    inputs:
      openapi:
        from: backend.openapi

  frontend:
    type: frontend-implementation

    inputs:
      requirement:
        from: requirement.requirement

      openapi:
        from: backend.openapi

      frontend-sdk:
        from: openapi.frontend-sdk
```

Go Runtime 不需要针对这个 Workflow 修改代码。

---

# 4. Node 是 Workflow 的基本执行单位

Node 定义：

```text
Node
├── Type
├── Input Contract
├── Output Contract
└── Executor
```

例如：

```text
backend-implementation

Input:
    RequirementSpec
    ArchitectureSpec

Output:
    SourceCode
    OpenAPI
```

而：

```text
openapi-generator

Input:
    OpenAPI

Output:
    FrontendSDK
```

---

# 5. Node 不要求必须存在 dependsOn

这是本项目一个非常重要的设计原则。

## 5.1 默认情况下，数据连接就是依赖

例如：

```text
UI Designer
    │
    │ UI Design
    ▼
UI Implementer
```

UI Designer：

```text
Output:
    FigmaDesign
```

UI Implementer：

```text
Input:
    FigmaDesign
```

那么 Workflow：

```yaml
nodes:

  designer:
    type: ui-designer

  implementer:
    type: ui-implementer

    inputs:
      design:
        from: designer.design
```

Runtime 自动知道：

```text
designer
    ↓
implementer
```

不需要：

```yaml
dependsOn:
  - designer
```

---

# 6. dependsOn 的真正含义

`dependsOn` 不表示普通的数据依赖。

它表示：

> **显式的执行顺序约束。**

例如：

```yaml
deploy:
  type: cd

  dependsOn:
    - approval
```

表示：

```text
approval 完成
      ↓
deploy 才允许执行
```

即使：

```text
deploy
```

并没有消费：

```text
approval
```

的任何 Artifact。

---

# 7. 因此 DAG 有两种 Edge

Workflow Runtime 内部将 Edge 分成：

## 7.1 Data Edge

```text
Node A.Output
      ↓
Node B.Input
```

例如：

```yaml
inputs:
  openapi:
    from: backend.openapi
```

这是最主要的 Edge。

---

## 7.2 Control Edge

```text
Node A
      ↓
Node B
```

通过：

```yaml
dependsOn:
  - A
```

表达。

---

## 7.3 最终 DAG

Runtime 构建 DAG 时：

```text
Data Edge
+
Control Edge
=
Execution DAG
```

因此：

```text
UI Designer
      │
      │ FigmaDesign
      ▼
UI Implementer
```

自动形成 Data Edge。

而：

```text
Test
   ↓
Human Approval
   ↓
CD
```

可以通过 Control Edge 表达。

---

# 8. Node 的运行条件

一个 Node 可以执行，当且仅当：

```text
所有 required Input Artifact 已经存在
+
所有 Control Dependency 已经完成
```

即：

```text
Ready(Node)
=
InputsReady
AND
ControlDependenciesCompleted
```

如果没有 `dependsOn`：

```text
Ready(Node)
=
InputsReady
```

如果没有 Input：

```text
Ready(Node)
=
ControlDependenciesCompleted
```

如果两个都没有：

```text
Ready(Node)
=
true
```

这意味着一个 Node 可以是：

```text
Trigger / Source Node
```

例如：

```text
requirement-analysis
```

可以不需要任何输入，Workflow 启动后直接运行。

---

# 9. workflow/v1 YAML Schema

第一版只定义这些字段：

```yaml
apiVersion:
kind:

metadata:

project:

nodes:
  <node-id>:
    type:
    inputs:
    dependsOn:
    config:
```

暂时不要加入：

```text
retry
timeout
condition
parallelism
environment
secrets
hooks
```

这些放到后续版本。

---

# 10. workflow/v1 完整示例

```yaml
apiVersion: workflow/v1
kind: Workflow

metadata:
  name: fullstack-development
  version: "1.0"

project:
  repository: ./examples/order-system
  branch: main

nodes:

  requirement:
    type: requirement-analysis

  architecture:
    type: architecture-design

    inputs:
      requirement:
        from: requirement.requirement

  backend:
    type: backend-implementation

    inputs:
      requirement:
        from: requirement.requirement

      architecture:
        from: architecture.architecture

  openapi:
    type: openapi-generator

    inputs:
      openapi:
        from: backend.openapi

  frontend:
    type: frontend-implementation

    inputs:
      requirement:
        from: requirement.requirement

      openapi:
        from: backend.openapi

      frontend-sdk:
        from: openapi.frontend-sdk
```

这个 Workflow 不需要显式写：

```yaml
dependsOn:
```

因为数据关系已经完整表达了执行关系。

---

# 11. Node Type 与 Node ID 必须分离

例如：

```yaml
nodes:

  backend:
    type: coding-agent

  frontend:
    type: coding-agent
```

这里：

```text
backend
frontend
```

是 Node ID。

而：

```text
coding-agent
```

是 Node Type。

这样同一个 Node Type 可以被实例化多次。

例如：

```yaml
nodes:

  backend:
    type: coding-agent

  frontend:
    type: coding-agent

  test-agent:
    type: coding-agent
```

三者可以拥有不同的 Input / Config。

---

# 12. Node Registry

Go Runtime 内部维护 Node Registry：

```text
Node Registry

requirement-analysis
architecture-design
coding-agent
openapi-generator
human-approval
external-ci
external-test
external-cd
```

概念上：

```go
type NodeFactory interface {
    Type() string
    Create(config NodeConfig) (Node, error)
}
```

Runtime 加载 YAML：

```text
type: coding-agent
```

然后：

```text
Node Registry
      ↓
coding-agent Factory
      ↓
CodingAgent Node
```

Workflow 不需要知道 Node 的 Go 实现。

---

# 13. Artifact 是 Node 之间唯一的数据通信方式

定义：

```go
type Artifact struct {
    ID      string
    Kind    string
    Version string
    Data    any
}
```

实际实现建议进一步拆分：

```go
type ArtifactRef struct {
    ID      string
    Kind    string
    Version string
    URI     string
}
```

Workflow 中传递：

```text
ArtifactRef
```

而不是大型 Artifact 数据本身。

---

# 14. MVP Artifact 类型

第一阶段只实现：

```text
RequirementSpec
ArchitectureSpec
OpenAPI
FrontendSDK
SourceCode
TestReport
ApprovalResult
```

其中：

```text
SourceCode
```

不应该把整个源码塞进 Artifact。

应该保存：

```yaml
kind: SourceCode

repository:
  path: ./backend

commit:
  sha: abc123
```

或者：

```yaml
workspace:
  path: /workspace/order-system/backend
```

---

# 15. ProjectContext

Project 是 Workflow Execution 的运行上下文。

Workflow YAML：

```yaml
project:
  repository: ./examples/order-system
  branch: main
```

Runtime 启动后：

```text
Workflow
    ↓
Project Resolver
    ↓
ProjectContext
```

ProjectContext：

```go
type ProjectContext struct {
    Repository Repository
    Branch     string
    Workspace  string
}
```

后续可以增加：

```text
Frontend
Backend
TechStack
ProjectMetadata
```

但 MVP 暂时不做过度设计。

---

# 16. workflow run 不接受额外业务参数

Workflow 运行方式：

```bash
workflow run workflow.yaml
```

所有 Workflow 所需信息：

```text
Workflow Definition
Project
Node Configuration
Agent Configuration
Automation Configuration
```

都写入 YAML 或 Workflow 自己引用的配置文件。

不设计：

```bash
workflow run workflow.yaml --project xxx
```

这样的额外业务参数。

CLI 第一版只负责：

```bash
workflow run <workflow-file>
```

---

# 17. Project Workspace

Runtime 负责创建 Workspace：

```text
Execution
    ↓
Workspace
    ↓
Project
```

例如：

```text
.workflow/
└── executions/
    └── execution-001/
        └── workspace/
            └── project/
```

Coding Agent Node 获取：

```text
ProjectContext.Workspace
```

然后在：

```text
/workspace/project
```

执行。

---

# 18. Coding Agent 不管理 Skills

这是本版本明确取消的设计。

Workflow 不定义：

```yaml
skills:
  - react
  - typescript
```

Workflow 只负责：

```text
Project
+
Workspace
+
Task
+
Input Artifact
```

Coding Agent 自己进入：

```text
project/
```

然后自行发现：

```text
.agents/skills/
.claude/skills/
```

以及其他 Agent 自己支持的项目约定。

因此：

```text
Workflow
    ↓
Coding Agent
    ↓
Project Workspace
    ↓
Agent 自己发现 Skills
```

Workflow 与 Skill 完全解耦。

---

# 19. Coding Agent Node

第一版：

```yaml
backend:
  type: coding-agent

  inputs:
    requirement:
      from: requirement.requirement

    architecture:
      from: architecture.architecture
```

Node 内部：

```text
CodingAgent Node
       ↓
获取 ProjectContext
       ↓
获取 Input Artifacts
       ↓
创建 Task
       ↓
启动 Agent
       ↓
Agent 进入 Project Workspace
       ↓
Agent 自己发现 Skills
       ↓
Agent 修改代码
       ↓
生成 Output Artifacts
```

---

# 20. Coding Agent Adapter

Runtime 不直接绑定某个 Coding Agent。

定义：

```go
type CodingAgent interface {
    Execute(
        ctx context.Context,
        task Task,
        project ProjectContext,
        inputs []ArtifactRef,
    ) ([]ArtifactRef, error)
}
```

第一阶段可以实现：

```text
MockCodingAgent
```

然后：

```text
RealCodingAgent
```

作为第二步。

这样可以先验证 Workflow Runtime，而不是一开始就被 Agent 集成问题拖住。

---

# 21. CUE 的职责

CUE 不负责 Workflow Runtime。

CUE 只负责：

> **验证 workflow.yaml 是否符合 workflow/v1 Schema。**

CUE 官方支持直接验证 YAML 文件，并且可以在 Go 中嵌入 CUE Schema 进行验证。

架构：

```text
workflow.yaml
      │
      ▼
CUE Schema
      │
      ▼
Configuration Validation
      │
      ▼
Go YAML Parser
      │
      ▼
Semantic Validation
      │
      ▼
Execution
```

---

# 22. CUE Schema 第一阶段负责什么

例如：

```text
apiVersion 必须存在
kind 必须是 Workflow
metadata.name 必须存在

project.repository 必须是 string

nodes 必须是 object

node.type 必须存在

inputs.from 必须是 string

dependsOn 必须是 list[string]
```

还可以限制：

```text
kind: "Workflow"
```

以及：

```text
apiVersion: "workflow/v1"
```

CUE 支持 required / optional fields 和类型约束，非常适合这一层配置验证。

---

# 23. Go Semantic Validator 负责什么

CUE 验证通过之后，还需要 Go 做第二层检查。

例如：

### Node Type 是否存在

```text
type: xxx
```

如果 Registry 没有：

```text
xxx
```

报错。

---

### Input Artifact 是否存在

```yaml
from: backend.openapi
```

但：

```text
backend
```

没有：

```text
openapi
```

报错。

---

### Input / Output 类型是否匹配

例如：

```text
Node A Output:
    FigmaDesign

Node B Input:
    OpenAPI
```

报错：

```text
FigmaDesign cannot be assigned to OpenAPI
```

---

### Artifact 是否唯一

防止：

```text
backend.openapi
```

产生歧义。

---

### DAG 是否存在控制环

例如：

```text
A dependsOn B
B dependsOn A
```

必须报错。

---

# 24. 因此 Workflow Validation 分成两层

```text
              workflow.yaml
                    │
                    ▼
              CUE Validation
                    │
             Schema Valid?
              /          \
            No            Yes
            │              │
          Error            ▼
                    Go Semantic Validator
                            │
                     ┌──────┼──────┐
                     ▼      ▼      ▼
                   Node   Artifact DAG
                  Check    Check   Check
```

最终：

```text
Valid Workflow
```

才允许进入 Runtime。

---

# 25. Runtime Execution Model

Runtime 启动：

```text
workflow run workflow.yaml
```

执行：

```text
① Load YAML
② CUE Validate
③ Parse Workflow
④ Resolve Node Types
⑤ Validate Node Inputs/Outputs
⑥ Build Data Edges
⑦ Build Control Edges
⑧ Validate DAG
⑨ Create Execution
⑩ Initialize Project
⑪ Execute Ready Nodes
⑫ Store Artifacts
⑬ Re-evaluate Ready Nodes
⑭ Continue
⑮ Workflow Complete
```

---

# 26. Ready Node 算法

Runtime 不应该简单：

```text
for node in yaml order
```

而应该：

```text
while workflow not completed:

    for node in nodes:

        if node is completed:
            continue

        if inputs are ready
           AND control dependencies are completed:

            execute(node)
```

实际实现使用：

```text
Ready Queue
+
Dependency Counter
```

避免每次全量扫描。

---

# 27. Node Execution 状态

MVP 至少定义：

```text
Pending
Ready
Running
Succeeded
Failed
Skipped
```

状态：

```text
Pending
   ↓
Ready
   ↓
Running
   ├──→ Succeeded
   │
   └──→ Failed
```

暂时不实现复杂 Retry。

---

# 28. Execution State

第一版使用文件系统：

```text
.workflow/
└── executions/
    └── <execution-id>/
        ├── workflow.yaml
        ├── state.json
        ├── nodes/
        │   ├── requirement/
        │   │   └── state.json
        │   ├── architecture/
        │   │   └── state.json
        │   └── backend/
        │       └── state.json
        │
        └── artifacts/
            ├── requirement/
            ├── architecture/
            ├── backend/
            └── openapi/
```

---

# 29. Artifact Store

定义：

```go
type ArtifactStore interface {
    Put(artifact Artifact) (ArtifactRef, error)

    Get(ref ArtifactRef) (Artifact, error)

    Exists(ref ArtifactRef) bool
}
```

MVP 实现：

```text
FilesystemArtifactStore
```

未来可以替换：

```text
MinIO
S3
OSS
Database
```

而不修改 Node。

---

# 30. Node Executor

定义统一接口：

```go
type Node interface {
    Type() string

    InputSchema() Schema

    OutputSchema() Schema

    Execute(
        ctx ExecutionContext,
        inputs map[string]ArtifactRef,
    ) ([]ArtifactRef, error)
}
```

这是整个 MVP 最重要的 Interface 之一。

---

# 31. 三类 Node

MVP 不需要把 Node 设计得非常复杂，只定义三个执行类型：

```text
Agent Node
Automation Node
Human Node
```

例如：

```text
coding-agent
```

属于 Agent Node。

```text
openapi-generator
```

属于 Automation Node。

```text
human-approval
```

属于 Human Node。

但对于 DAG Runtime 来说：

```text
它们全部都是 Node。
```

Runtime 不需要关心 Node 内部怎么实现。

---

# 32. 第一批 MVP Node

建议只实现：

### 1. requirement-analysis

```text
Input:
    无

Output:
    RequirementSpec
```

第一版可以使用 Mock。

---

### 2. architecture-design

```text
Input:
    RequirementSpec

Output:
    ArchitectureSpec
```

第一版可以使用 Mock。

---

### 3. coding-agent

```text
Input:
    任意定义好的 Task Artifact

Output:
    SourceCode
    OpenAPI
```

第一版可以使用 MockCodingAgent。

---

### 4. openapi-generator

```text
Input:
    OpenAPI

Output:
    FrontendSDK
```

调用真实 OpenAPI Generator。

---

### 5. frontend-coding-agent

可以暂时复用：

```text
coding-agent
```

通过 YAML 配置 Task。

---

# 33. MVP 第一阶段不要真的接 AI

这是非常重要的实现策略。

先实现：

```text
MockRequirementNode
MockArchitectureNode
MockCodingAgent
```

让它们返回：

```text
RequirementSpec
ArchitectureSpec
SourceCode
OpenAPI
```

先把：

```text
YAML
 ↓
DAG
 ↓
Artifact
 ↓
Node
 ↓
Artifact
 ↓
Next Node
```

跑通。

然后再接真实 Agent。

---

# 34. MVP 里程碑

## Milestone 1：Core Model

实现：

```text
Workflow
Node
Artifact
ArtifactRef
ProjectContext
Execution
```

验收：

```text
Go Unit Tests Pass
```

---

## Milestone 2：YAML Loader

实现：

```text
workflow.yaml
      ↓
Go Struct
```

验收：

```bash
workflow validate workflow.yaml
```

可以成功解析。

---

## Milestone 3：CUE Validation

增加：

```text
workflow/v1.cue
```

实现：

```bash
workflow validate workflow.yaml
```

执行：

```text
YAML Syntax
+
CUE Schema
+
Go Semantic Validation
```

验收：

错误 Workflow 能明确指出：

```text
哪个字段
哪个 Node
什么错误
```

CUE 本身可以通过 `cue vet` 校验 YAML，也可以通过 Go API 嵌入 Runtime，因此 MVP 可以先采用 CLI 验证思路验证 Schema，再在 Go Runtime 中集成。

---

# 35. Milestone 4：DAG Builder

实现：

```text
Data Edge
Control Edge
```

输入：

```yaml
inputs:
  xxx:
    from: node.output
```

转换：

```text
Node.Output
       ↓
Node.Input
```

最终生成：

```go
type Edge struct {
    From NodeID
    To   NodeID
    Type EdgeType
}
```

其中：

```go
type EdgeType string

const (
    DataEdge    EdgeType = "data"
    ControlEdge EdgeType = "control"
)
```

---

# 36. Milestone 5：DAG Validator

必须支持：

```text
Node 不存在
Output 不存在
Input 不存在
Artifact 类型不匹配
Control Dependency 不存在
Control Dependency 成环
Data Dependency 成环
```

验收：

准备一组：

```text
valid/
invalid-node/
invalid-output/
invalid-type/
invalid-cycle/
```

全部自动测试。

---

# 37. Milestone 6：Execution Engine

实现：

```text
Ready Queue
Node State
Artifact Store
Execution State
```

首先只支持：

```text
单进程
串行执行
```

不要急着并行。

---

# 38. Milestone 7：并行 DAG

当串行版本稳定后：

```text
A
├── B
├── C
└── D
```

如果：

```text
B
C
D
```

之间没有依赖，则同时执行。

Go 的：

```text
goroutine
channel
sync.WaitGroup
```

足够完成 MVP。

---

# 39. Milestone 8：Project Runtime

支持：

```bash
workflow run workflow.yaml
```

Workflow YAML：

```yaml
project:
  repository: ./examples/order-system
```

Runtime：

```text
Load Project
      ↓
Create Workspace
      ↓
ProjectContext
      ↓
Node
```

---

# 40. Milestone 9：OpenAPI Automation

实现：

```text
OpenAPI Artifact
      ↓
OpenAPI Generator
      ↓
FrontendSDK Artifact
```

这是第一个真实 Automation Node。

---

# 41. Milestone 10：Coding Agent Adapter

定义：

```go
type CodingAgent interface {
    Execute(
        context.Context,
        Task,
        ProjectContext,
        []ArtifactRef,
    ) ([]ArtifactRef, error)
}
```

先实现：

```text
MockCodingAgent
```

然后再实现真实 Agent Adapter。

Workflow 不应该知道具体 Agent 的内部实现。

---

# 42. Milestone 11：完整 Fullstack Demo

最终 Workflow：

```yaml
apiVersion: workflow/v1
kind: Workflow

metadata:
  name: fullstack-development

project:
  repository: ./examples/order-system

nodes:

  requirement:
    type: requirement-analysis

  architecture:
    type: architecture-design

    inputs:
      requirement:
        from: requirement.requirement

  backend:
    type: coding-agent

    inputs:
      requirement:
        from: requirement.requirement

      architecture:
        from: architecture.architecture

  openapi:
    type: openapi-generator

    inputs:
      openapi:
        from: backend.openapi

  frontend:
    type: coding-agent

    inputs:
      requirement:
        from: requirement.requirement

      openapi:
        from: backend.openapi

      frontend-sdk:
        from: openapi.frontend-sdk
```

运行：

```bash
workflow run ./workflows/fullstack.yaml
```

最终：

```text
Workflow SUCCESS

Artifacts:

RequirementSpec
ArchitectureSpec
BackendSourceCode
OpenAPI
FrontendSDK
FrontendSourceCode
```

---

# 43. MVP 最终目录结构

建议：

```text
workflow-engine/
│
├── cmd/
│   └── workflow/
│       └── main.go
│
├── internal/
│   │
│   ├── workflow/
│   │   ├── definition.go
│   │   ├── loader.go
│   │   ├── validator.go
│   │   └── graph.go
│   │
│   ├── node/
│   │   ├── node.go
│   │   ├── registry.go
│   │   └── builtins/
│   │
│   ├── artifact/
│   │   ├── artifact.go
│   │   ├── registry.go
│   │   └── store.go
│   │
│   ├── execution/
│   │   ├── engine.go
│   │   ├── state.go
│   │   └── scheduler.go
│   │
│   ├── project/
│   │   ├── project.go
│   │   └── workspace.go
│   │
│   ├── agent/
│   │   ├── agent.go
│   │   ├── mock.go
│   │   └── adapter.go
│   │
│   └── validation/
│       ├── cue.go
│       └── semantic.go
│
├── schema/
│   └── workflow/
│       └── v1.cue
│
├── examples/
│   └── fullstack/
│       ├── workflow.yaml
│       └── project/
│
└── tests/
    ├── workflow/
    ├── dag/
    ├── artifact/
    └── execution/
```

---

# 44. MVP 开发顺序

严格按照以下顺序：

```text
① 定义 Core Model
       ↓
② 定义 workflow/v1 YAML
       ↓
③ 编写 CUE Schema
       ↓
④ YAML Loader
       ↓
⑤ CUE Validator
       ↓
⑥ Node Registry
       ↓
⑦ Artifact Registry
       ↓
⑧ Data Edge Builder
       ↓
⑨ Control Edge Builder
       ↓
⑩ DAG Validator
       ↓
⑪ Execution Engine
       ↓
⑫ Artifact Store
       ↓
⑬ Project Context
       ↓
⑭ Mock Nodes
       ↓
⑮ OpenAPI Generator Node
       ↓
⑯ Mock Coding Agent
       ↓
⑰ Real Coding Agent Adapter
       ↓
⑱ Fullstack Demo
```

---

# 45. MVP 暂时明确不做

```text
❌ UI
❌ Web Dashboard
❌ Temporal
❌ Redis
❌ Kafka
❌ Database
❌ 分布式调度
❌ 多租户
❌ Skill Registry
❌ Workflow Marketplace
❌ Agent Marketplace
❌ 复杂 Retry
❌ Workflow Condition
❌ Workflow Version Migration
❌ Secret Management
```

这些都不是 MVP 的核心问题。

---

# 46. MVP 最终验收标准

只需要验证以下场景。

## Case 1：线性 DAG

```text
A → B → C
```

可以正确执行。

---

## Case 2：并行 DAG

```text
      ┌→ B ─┐
A ────┤     ├→ D
      └→ C ─┘
```

B/C 可以并行，D 等待两者。

---

## Case 3：Data Dependency

```text
A.output
   ↓
B.input
```

B 自动等待 A。

不需要：

```yaml
dependsOn:
  - A
```

---

## Case 4：Control Dependency

```yaml
B:
  type: xxx
  dependsOn:
    - A
```

即使 B 不消费 A 的任何 Artifact：

```text
A
↓
B
```

也必须保证 B 等 A 完成。

---

## Case 5：Input Type Error

```text
A.output = FigmaDesign

B.input = OpenAPI
```

Workflow Validate 阶段直接失败。

---

## Case 6：Project

```bash
workflow run workflow.yaml
```

Workflow 能正确初始化：

```text
ProjectContext
Workspace
Repository
```

---

## Case 7：真实 Automation

```text
OpenAPI
   ↓
OpenAPI Generator
   ↓
Frontend SDK
```

成功生成文件。

---

## Case 8：真实 Coding Agent

```text
Project
   ↓
Coding Agent
   ↓
Workspace
   ↓
修改代码
```

并且 Agent 可以自行读取：

```text
.agents/skills
.claude/skills
```

Workflow 不参与 Skill 管理。

---

# 47. 最终架构原则

整个 MVP 最终应该保持下面这个关系：

```text
                     workflow.yaml
                          │
                          ▼
                    Workflow Engine
                          │
             ┌────────────┼────────────┐
             │            │            │
             ▼            ▼            ▼
           Node         Node         Node
             │            │            │
             ▼            ▼            ▼
          Input        Input        Input
             │            │            │
             └────────────┼────────────┘
                          │
                       Artifact
                          │
                          ▼
                     Next Node
```

而 Project 是 Runtime Context：

```text
                    Workflow
                       │
                       ▼
                 ProjectContext
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
       Backend      Frontend      Docs
       Workspace    Workspace      ...
```

Coding Agent：

```text
             Coding Agent
                   │
                   ▼
             Project Workspace
                   │
                   ├── .agents/skills
                   ├── .claude/skills
                   └── source code
```

Workflow 不知道 Agent 到底使用什么 Skill。

---

# 48. 最核心的最终抽象

如果整个项目最后只能留下几个概念，我建议就是：

```text
Workflow
Node
Artifact
Project
Execution
```

其中：

```text
Workflow
    = Node 的组合

Node
    = Input → Execute → Output

Artifact
    = Node 之间的数据

Project
    = Workflow 的运行环境

Execution
    = Workflow 的一次实际运行
```

然后：

```text
Data Dependency
    = Artifact Input/Output

Control Dependency
    = dependsOn

DAG
    = Data Edge + Control Edge
```

这套模型已经足够支撑你第一版的整个软件开发 Workflow。

**最关键的一点是：不要把 `dependsOn` 当成 Workflow 的主要连接机制。**

你的直觉是对的。对于：

```text
UI Designer
    ↓ Figma
UI Implementer
```

真正重要的是：

```text
UI Designer
Output: Figma

UI Implementer
Input: Figma
```

Runtime 根据这个关系自然得到：

```text
Designer → Implementer
```

而 `dependsOn` 留给：

```text
Human Approval
    ↓
CD
```

这种**没有数据传递，但存在明确执行顺序**的场景。

这样设计以后，你的 Workflow DSL 会非常轻：**Node 声明能力，YAML 声明组合，Artifact 声明数据，Runtime 负责调度。**

这就是我建议现在正式进入编码阶段时所采用的 `workflow/v1` 基础。