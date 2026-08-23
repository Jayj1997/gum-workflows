package workflow

import (
	"reflect"
	"testing"
)

// graphDefinition 返回与设计计划 §10 类似的 fullstack 定义（结构供构图测试使用）。
func graphDefinition() Definition {
	return Definition{
		APIVersion: APIVersionV1,
		Kind:       KindWorkflow,
		Metadata:   Metadata{Name: "g"},
		Nodes: map[string]NodeSpec{
			"requirement": {Type: "requirement-analysis"},
			"architecture": {
				Type:   "architecture-design",
				Inputs: map[string]InputBinding{"requirement": {From: "requirement.requirement"}},
			},
			"backend": {
				Type: "coding-agent",
				Inputs: map[string]InputBinding{
					"requirement":  {From: "requirement.requirement"},
					"architecture": {From: "architecture.architecture"},
				},
			},
			"openapi": {
				Type:   "openapi-generator",
				Inputs: map[string]InputBinding{"openapi": {From: "backend.openapi"}},
			},
			"frontend": {
				Type: "coding-agent",
				Inputs: map[string]InputBinding{
					"requirement":  {From: "requirement.requirement"},
					"openapi":      {From: "backend.openapi"},
					"frontend-sdk": {From: "openapi.frontend-sdk"},
				},
			},
		},
	}
}

func TestBuildGraphDataEdges(t *testing.T) {
	g, err := BuildGraph(graphDefinition())
	if err != nil {
		t.Fatalf("BuildGraph() unexpected error: %v", err)
	}

	want := map[Edge]int{
		{From: "requirement", To: "architecture", Type: DataEdge}: 1,
		{From: "requirement", To: "backend", Type: DataEdge}:      1,
		{From: "requirement", To: "frontend", Type: DataEdge}:     1,
		{From: "architecture", To: "backend", Type: DataEdge}:     1,
		{From: "backend", To: "openapi", Type: DataEdge}:          1,
		{From: "backend", To: "frontend", Type: DataEdge}:         1,
		{From: "openapi", To: "frontend", Type: DataEdge}:         1,
	}
	if len(g.Edges) != len(want) {
		t.Fatalf("len(Edges) = %d, want %d: %+v", len(g.Edges), len(want), g.Edges)
	}
	for _, e := range g.Edges {
		want[e]--
		if want[e] < 0 {
			t.Fatalf("unexpected edge %+v", e)
		}
	}
	for e, n := range want {
		if n != 0 {
			t.Fatalf("missing edge %+v", e)
		}
	}
}

func TestBuildGraphControlEdge(t *testing.T) {
	def := Definition{
		APIVersion: APIVersionV1,
		Kind:       KindWorkflow,
		Metadata:   Metadata{Name: "g"},
		Nodes: map[string]NodeSpec{
			"approval": {Type: "human-approval"},
			"deploy":   {Type: "cd", DependsOn: []string{"approval"}},
		},
	}
	g, err := BuildGraph(def)
	if err != nil {
		t.Fatalf("BuildGraph() unexpected error: %v", err)
	}

	if len(g.Edges) != 1 {
		t.Fatalf("len(Edges) = %d, want 1", len(g.Edges))
	}
	if got, want := g.Edges[0], (Edge{From: "approval", To: "deploy", Type: ControlEdge}); got != want {
		t.Fatalf("edge = %+v, want %+v", got, want)
	}

	if roots := g.Roots(); !reflect.DeepEqual(roots, []string{"approval"}) {
		t.Errorf("Roots() = %v, want [approval]", roots)
	}
	if succ := g.Successors("approval"); !reflect.DeepEqual(succ, []string{"deploy"}) {
		t.Errorf("Successors(approval) = %v, want [deploy]", succ)
	}
	if pred := g.Predecessors("deploy"); !reflect.DeepEqual(pred, []string{"approval"}) {
		t.Errorf("Predecessors(deploy) = %v, want [approval]", pred)
	}
}

func TestBuildGraphNoDependsOnIsNormal(t *testing.T) {
	// dependsOn 可选：纯数据依赖的定义中不应产生任何 Control Edge。
	g, err := BuildGraph(graphDefinition())
	if err != nil {
		t.Fatalf("BuildGraph() unexpected error: %v", err)
	}
	for _, e := range g.Edges {
		if e.Type == ControlEdge {
			t.Fatalf("unexpected control edge %+v in data-only workflow", e)
		}
	}
	if roots := g.Roots(); !reflect.DeepEqual(roots, []string{"requirement"}) {
		t.Errorf("Roots() = %v, want [requirement]", roots)
	}
}

func TestBuildGraphRejectsMalformedFrom(t *testing.T) {
	def := Definition{
		APIVersion: APIVersionV1,
		Kind:       KindWorkflow,
		Metadata:   Metadata{Name: "g"},
		Nodes: map[string]NodeSpec{
			"a": {Type: "x"},
			"b": {Type: "y", Inputs: map[string]InputBinding{"i": {From: "noseparator"}}},
		},
	}
	if _, err := BuildGraph(def); err == nil {
		t.Fatal("BuildGraph() = nil error, want malformed-from rejection")
	}
}

func TestCycle(t *testing.T) {
	tests := []struct {
		name string
		def  Definition
	}{
		{
			name: "acyclic fullstack",
			def:  graphDefinition(),
		},
		{
			name: "data dependency cycle",
			def: Definition{
				Metadata: Metadata{Name: "g"},
				Nodes: map[string]NodeSpec{
					"a": {Type: "x", Inputs: map[string]InputBinding{"i": {From: "b.out"}}},
					"b": {Type: "y", Inputs: map[string]InputBinding{"i": {From: "a.out"}}},
				},
			},
		},
		{
			name: "control dependency cycle",
			def: Definition{
				Metadata: Metadata{Name: "g"},
				Nodes: map[string]NodeSpec{
					"a": {Type: "x", DependsOn: []string{"b"}},
					"b": {Type: "y", DependsOn: []string{"a"}},
				},
			},
		},
		{
			name: "mixed cycle",
			def: Definition{
				Metadata: Metadata{Name: "g"},
				Nodes: map[string]NodeSpec{
					"a": {Type: "x", Inputs: map[string]InputBinding{"i": {From: "b.out"}}},
					"b": {Type: "y", DependsOn: []string{"a"}},
				},
			},
		},
		{
			name: "self data cycle",
			def: Definition{
				Metadata: Metadata{Name: "g"},
				Nodes: map[string]NodeSpec{
					"a": {Type: "x", Inputs: map[string]InputBinding{"i": {From: "a.out"}}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := BuildGraph(tt.def)
			if err != nil {
				t.Fatalf("BuildGraph() unexpected error: %v", err)
			}
			cycle := g.Cycle()
			if tt.name == "acyclic fullstack" {
				if cycle != nil {
					t.Fatalf("Cycle() = %v, want nil", cycle)
				}
				return
			}
			if cycle == nil {
				t.Fatal("Cycle() = nil, want cycle")
			}
			// 环路径首尾必须相同，且首节点在路径中出现两次。
			if cycle[0] != cycle[len(cycle)-1] {
				t.Fatalf("cycle %v must start and end with the same node", cycle)
			}
		})
	}
}
