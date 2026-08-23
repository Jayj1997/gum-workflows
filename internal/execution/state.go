// Package execution 定义 Workflow 的运行时对象模型。
//
// 定义侧与运行侧严格区分（同名概念不得混用）：
//
//	Workflow（定义）          = workflow.Definition
//	WorkflowExecution（运行） = 一次 workflow run，如 execution-000001
//	Node（定义）              = workflow.NodeSpec（id + type + inputs + dependsOn）
//	NodeExecution（运行）     = 一个 Node 在某次 WorkflowExecution 中的运行实例
//
// 同一个 Workflow 可以运行多次（run #001、#002、#003），
// 每次运行产生一个独立的 WorkflowExecution，各自持有自己的 NodeExecution 集合。
// NodeExecution 是运行快照：记录实际使用的 Node Type、状态、产出与错误，
// 不回写定义。状态流转规则集中在本包；state.json 的持久化形态也以本包类型为基础。
package execution

import (
	"fmt"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

// Status 是 NodeExecution 与 WorkflowExecution 共用的状态（设计计划 §27）。
type Status string

// MVP 状态集合。
const (
	StatusPending   Status = "Pending"
	StatusReady     Status = "Ready"
	StatusRunning   Status = "Running"
	StatusSucceeded Status = "Succeeded"
	StatusFailed    Status = "Failed"
	StatusSkipped   Status = "Skipped"
)

// transitions 定义合法状态流转（设计计划 §27）：
//
//	Pending -> Ready | Skipped
//	Ready   -> Running | Skipped
//	Running -> Succeeded | Failed
//	Succeeded / Failed / Skipped 为终态
var transitions = map[Status][]Status{
	StatusPending:   {StatusReady, StatusSkipped},
	StatusReady:     {StatusRunning, StatusSkipped},
	StatusRunning:   {StatusSucceeded, StatusFailed},
	StatusSucceeded: {},
	StatusFailed:    {},
	StatusSkipped:   {},
}

// CanTransitionTo 报告 from -> next 是否为合法流转。
func CanTransitionTo(from, next Status) bool {
	for _, s := range transitions[from] {
		if s == next {
			return true
		}
	}
	return false
}

// Terminal 报告 s 是否为终态。
func Terminal(s Status) bool {
	return len(transitions[s]) == 0
}

// NodeExecution 是一个 Node 定义（workflow.NodeSpec）在某次
// WorkflowExecution 中的运行实例。
type NodeExecution struct {
	// NodeID 对应 Workflow 定义中的 Node ID（"有一个叫 backend 的节点"）。
	NodeID string
	// NodeType 记录本次运行实际实例化的 Node Type（运行快照，
	// 定义可能在两次运行之间变化）。
	NodeType string

	Status Status
	// Outputs 是本次运行实际产出的「输出名 -> ArtifactRef」映射，
	// 是后续 NodeExecution 解析 inputs.from 引用的依据。
	Outputs map[string]artifact.ArtifactRef
	Error   string
}

// TransitionTo 将本实例流转到 next，非法流转返回错误而不是静默接受。
func (n *NodeExecution) TransitionTo(next Status) error {
	if !CanTransitionTo(n.Status, next) {
		return fmt.Errorf("node execution %q: illegal transition %s -> %s", n.NodeID, n.Status, next)
	}
	n.Status = next
	return nil
}

// WorkflowExecution 是 Workflow Definition 的一次实际运行（计划中的 Execution）。
// 每次调用 Engine.Run 产生一个新实例，互不影响。
type WorkflowExecution struct {
	ID       string // 如 execution-000001
	Workflow string // workflow metadata.name（定义侧名称）
	Status   Status
	Nodes    map[string]*NodeExecution // key 为 Node ID
}

// Node 返回指定 Node 的运行实例（不存在时 nil）。
func (w *WorkflowExecution) Node(nodeID string) *NodeExecution {
	return w.Nodes[nodeID]
}
