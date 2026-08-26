// tests/workflow 放跨包集成测试（见 docs/DEVELOPMENT.md §6）。
// 本文件验证「已通过 Validator 的 Workflow -> DAG Builder」管线：
// CUE Schema -> YAML Loader -> 结构校验 -> BuildGraph。
// 该 fixture 的语义校验（含 Node Registry）在 internal/validation 的
// testdata/valid/fullstack.yaml 用例中覆盖。
package workflow_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/validation"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

func loadValidatedWorkflow(t *testing.T) workflow.Definition {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "workflow", "testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	if err := validation.ValidateSchema("valid.yaml", data); err != nil {
		t.Fatalf("ValidateSchema() unexpected error: %v", err)
	}
	def, err := workflow.Load(data)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	return def
}

func TestValidatePipelineOnValidWorkflow(t *testing.T) {
	def := loadValidatedWorkflow(t)

	// 数据依赖应能完整构建 DAG：coder 是唯一源节点。
	g, err := workflow.BuildGraph(def)
	if err != nil {
		t.Fatalf("BuildGraph() unexpected error: %v", err)
	}
	if roots := g.Roots(); len(roots) != 1 || roots[0] != "coder" {
		t.Errorf("Roots() = %v, want [coder]", roots)
	}
	if cycle := g.Cycle(); cycle != nil {
		t.Errorf("Cycle() = %v, want nil", cycle)
	}
}

// TestDAGBuilderMinimalChain 是 DAG Builder（M4）的管线级验收：
// 输入为已通过 Validator 的 Workflow，输出为 Execution DAG。
// 依据设计计划 §10：无需任何 dependsOn，数据关系完整表达执行关系。
func TestDAGBuilderMinimalChain(t *testing.T) {
	def := loadValidatedWorkflow(t)

	g, err := workflow.BuildGraph(def)
	if err != nil {
		t.Fatalf("BuildGraph() unexpected error: %v", err)
	}

	// 期望的完整边集（字典序）：全部为 Data Edge，无一条 Control Edge（计划 §10 的核心主张）。
	want := []workflow.Edge{
		{From: "coder", To: "sdk", Type: workflow.DataEdge},
	}

	got := append([]workflow.Edge(nil), g.Edges...)
	sort.Slice(got, func(i, j int) bool {
		if got[i].From != got[j].From {
			return got[i].From < got[j].From
		}
		return got[i].To < got[j].To
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Edges() =\n%v\nwant\n%v", got, want)
	}

	for _, e := range g.Edges {
		if e.Type == workflow.ControlEdge {
			t.Errorf("unexpected control edge %+v in data-only workflow", e)
		}
	}

	if roots := g.Roots(); !reflect.DeepEqual(roots, []string{"coder"}) {
		t.Errorf("Roots() = %v, want [coder]", roots)
	}
	if cycle := g.Cycle(); cycle != nil {
		t.Errorf("Cycle() = %v, want nil", cycle)
	}

	t.Logf("Execution DAG (%d nodes, %d edges):", len(g.NodeIDs), len(g.Edges))
	for _, e := range got {
		t.Logf("  %s --%s--> %s", e.From, e.Type, e.To)
	}
	for _, id := range g.NodeIDs {
		t.Logf("  %-8s preds=%v succs=%v", id, g.Predecessors(id), g.Successors(id))
	}
}

// TestDAGBuilderControlEdge 验证 Control Edge 场景（计划 §6/§7.2）：
// approval 与 deploy 之间没有数据传递，但存在明确执行顺序。
func TestDAGBuilderControlEdge(t *testing.T) {
	def := workflow.Definition{
		APIVersion: workflow.APIVersionV1,
		Kind:       workflow.KindWorkflow,
		Metadata:   workflow.Metadata{Name: "deploy"},
		Nodes: map[string]workflow.NodeSpec{
			"test":     {Node: "external-test"},
			"approval": {Node: "human-approval"},
			"deploy":   {Node: "external-cd", DependsOn: []string{"approval"}},
		},
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}

	g, err := workflow.BuildGraph(def)
	if err != nil {
		t.Fatalf("BuildGraph() unexpected error: %v", err)
	}

	want := []workflow.Edge{{From: "approval", To: "deploy", Type: workflow.ControlEdge}}
	if !reflect.DeepEqual(g.Edges, want) {
		t.Fatalf("Edges() = %v, want %v", g.Edges, want)
	}

	// approval 与 test 无入边是源节点；deploy 有来自 approval 的 Control Edge 入边，
	// 不是源节点--它必须等 approval 完成后才允许执行（计划 §6）。
	if roots := g.Roots(); !reflect.DeepEqual(roots, []string{"approval", "test"}) {
		t.Errorf("Roots() = %v, want [approval test]", roots)
	}
	if succ := g.Successors("approval"); !reflect.DeepEqual(succ, []string{"deploy"}) {
		t.Errorf("Successors(approval) = %v, want [deploy]", succ)
	}
}
