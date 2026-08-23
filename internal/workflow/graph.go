package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// EdgeType 区分 DAG 的两种边（设计计划 §7）：
// Data Edge 来自 inputs.from（隐式、主要），Control Edge 来自 dependsOn（显式、可选）。
type EdgeType string

const (
	DataEdge    EdgeType = "data"
	ControlEdge EdgeType = "control"
)

// Edge 是 Execution DAG 的一条边，From/To 均为 Node ID。
type Edge struct {
	From string
	To   string
	Type EdgeType
}

// Graph 是由 Definition 构建的 Execution DAG：
// Data Edge + Control Edge（设计计划 §7.3）。
type Graph struct {
	NodeIDs []string
	Edges   []Edge

	// succ/pred 为去重后的邻接表，供遍历使用。
	succ map[string]map[string]bool
	pred map[string]map[string]bool
}

// ParseRef 解析 "<node-id>.<output-name>" 形式的 Artifact 引用。
// Node ID 不含 "."（由 Definition.Validate 保证），因此按第一个 "." 切分。
func ParseRef(from string) (nodeID, output string, err error) {
	i := strings.Index(from, ".")
	if i <= 0 || i == len(from)-1 {
		return "", "", fmt.Errorf("invalid artifact reference %q: want %q", from, "<node-id>.<output-name>")
	}
	return from[:i], from[i+1:], nil
}

// BuildGraph 依据 inputs.from 构建 Data Edge、依据 dependsOn 构建 Control Edge
// （设计计划 §7.3：Data Edge + Control Edge = Execution DAG）。
//
// 契约：入参 def 是已经通过两层 Validator（CUE + Semantic）的 Workflow。
// 在此前提下所有 From / dependsOn 引用必然存在，本函数不会失败；
// 对畸形 From 仍返回错误（防御未校验的调用方）。
// dependsOn 是可选的：未声明的 Node 没有 Control Edge，数据连接本身就表达了执行关系。
func BuildGraph(def Definition) (Graph, error) {
	g := Graph{
		NodeIDs: make([]string, 0, len(def.Nodes)),
		succ:    map[string]map[string]bool{},
		pred:    map[string]map[string]bool{},
	}
	for id := range def.Nodes {
		g.NodeIDs = append(g.NodeIDs, id)
		g.succ[id] = map[string]bool{}
		g.pred[id] = map[string]bool{}
	}
	sort.Strings(g.NodeIDs)

	for _, to := range g.NodeIDs {
		spec := def.Nodes[to]
		for name, binding := range spec.Inputs {
			from, _, err := ParseRef(binding.From)
			if err != nil {
				return Graph{}, fmt.Errorf("node %q input %q: %w", to, name, err)
			}
			g.addEdge(from, to, DataEdge)
		}
		for _, from := range spec.DependsOn {
			g.addEdge(from, to, ControlEdge)
		}
	}
	return g, nil
}

func (g *Graph) addEdge(from, to string, edgeType EdgeType) {
	g.Edges = append(g.Edges, Edge{From: from, To: to, Type: edgeType})
	if g.succ[from] == nil {
		g.succ[from] = map[string]bool{}
	}
	if g.pred[to] == nil {
		g.pred[to] = map[string]bool{}
	}
	g.succ[from][to] = true
	g.pred[to][from] = true
}

// Roots 返回没有入边的 Node（源节点）。
// 无输入也无 dependsOn 的 Node（如 requirement-analysis）是合法的源节点。
func (g Graph) Roots() []string {
	var roots []string
	for _, id := range g.NodeIDs {
		if len(g.pred[id]) == 0 {
			roots = append(roots, id)
		}
	}
	return roots
}

// Successors 返回指定 Node 的直接后继（去重、有序）。
func (g Graph) Successors(nodeID string) []string {
	return sortedKeys(g.succ[nodeID])
}

// Predecessors 返回指定 Node 的直接前驱（去重、有序）。
func (g Graph) Predecessors(nodeID string) []string {
	return sortedKeys(g.pred[nodeID])
}

// Cycle 检测依赖环（Data Edge 与 Control Edge 合并检测，设计计划 §36），
// 返回环路径（首尾相同便于阅读），无环时返回 nil。
func (g Graph) Cycle() []string {
	const (
		white = iota // 未访问
		gray         // 在当前 DFS 路径上
		black        // 已完成
	)
	state := map[string]int{}
	var path []string

	var dfs func(n string) []string
	dfs = func(n string) []string {
		state[n] = gray
		path = append(path, n)
		for _, m := range sortedKeys(g.succ[n]) {
			switch state[m] {
			case gray:
				// 找到环：回溯 path 中从 m 开始到当前的节点。
				start := 0
				for i, p := range path {
					if p == m {
						start = i
						break
					}
				}
				cycle := append([]string{}, path[start:]...)
				return append(cycle, m)
			case white:
				if c := dfs(m); c != nil {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		state[n] = black
		return nil
	}

	for _, id := range g.NodeIDs {
		if state[id] == white {
			if c := dfs(id); c != nil {
				return c
			}
		}
	}
	return nil
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
