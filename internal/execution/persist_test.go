package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

// TestPersistStateLayout 验证计划 §28 的目录布局：
// state.json + nodes/<node-id>/state.json。
func TestPersistStateLayout(t *testing.T) {
	// 直接构造一个确定状态的执行对象验证布局。
	sample := &WorkflowExecution{
		ID:       "execution-000001",
		Workflow: "fullstack-development",
		Status:   StatusFailed,
		Nodes: map[string]*NodeExecution{
			"requirement": {
				NodeID:   "requirement",
				NodeType: "requirement-analysis",
				Status:   StatusSucceeded,
				Outputs: map[string]artifact.ArtifactRef{
					"requirement": {ID: "requirement", Kind: artifact.KindRequirementSpec, Version: "1", URI: "1.json"},
				},
			},
			"architecture": {
				NodeID:   "architecture",
				NodeType: "architecture-design",
				Status:   StatusFailed,
				Error:    "mock failure",
			},
			"backend": {
				NodeID:   "backend",
				NodeType: "coding-agent",
				Status:   StatusPending,
			},
		},
	}
	execDir := filepath.Join(t.TempDir(), "executions", sample.ID)
	if err := PersistState(execDir, sample); err != nil {
		t.Fatalf("PersistState() unexpected error: %v", err)
	}

	// 布局断言。
	for _, path := range []string{
		"state.json",
		"nodes/requirement/state.json",
		"nodes/architecture/state.json",
		"nodes/backend/state.json",
	} {
		if _, err := os.Stat(filepath.Join(execDir, path)); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}

	// 顶层 state.json 内容。
	var top map[string]any
	data, err := os.ReadFile(filepath.Join(execDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("parse state.json: %v", err)
	}
	if top["id"] != sample.ID || top["status"] != "Failed" || top["workflow"] != sample.Workflow {
		t.Errorf("state.json = %v", top)
	}

	// NodeExecution 快照可读回，定义身份完整保留。
	ne, err := LoadNodeState(execDir, "requirement")
	if err != nil {
		t.Fatalf("LoadNodeState(): %v", err)
	}
	if ne.NodeID != "requirement" || ne.NodeType != "requirement-analysis" {
		t.Errorf("loaded identity = %q/%q", ne.NodeID, ne.NodeType)
	}
	if ne.Status != StatusSucceeded {
		t.Errorf("loaded status = %s", ne.Status)
	}
	if ref := ne.Outputs["requirement"]; ref.URI != "1.json" || ref.Kind != artifact.KindRequirementSpec {
		t.Errorf("loaded output ref = %+v", ref)
	}

	failed, err := LoadNodeState(execDir, "architecture")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != StatusFailed || failed.Error != "mock failure" {
		t.Errorf("loaded failed node = %+v", failed)
	}
}

// TestEnginePersistsWithStateDir 验证 WithStateDir 集成：
// 失败执行与成功执行都会留下快照。
func TestEnginePersistsWithStateDir(t *testing.T) {
	root := t.TempDir()

	t.Run("failed run persists", func(t *testing.T) {
		e, _ := newChainEngine(t, func(nodeType string, f *mockFactory) {
			if nodeType == "coding-agent" {
				f.fail = true
			}
		})
		exec, err := e.Run(context.Background(), chainDef())
		if err == nil {
			t.Fatal("Run() = nil error, want failure")
		}
		// 未配置 stateDir 时不写盘：目录不存在。
		if _, err := os.Stat(filepath.Join(root, exec.ID)); !os.IsNotExist(err) {
			t.Errorf("state written without WithStateDir")
		}
	})

	t.Run("successful run persists", func(t *testing.T) {
		dr, er := newTestRegistries(t, chainFactories(nil)...)
		e := NewEngine(er, dr, artifact.NewMemStore(), nil, WithStateDir(root))
		exec, err := e.Run(context.Background(), chainDef())
		if err != nil {
			t.Fatalf("Run() unexpected error: %v", err)
		}

		execDir := filepath.Join(root, exec.ID)
		ne, err := LoadNodeState(execDir, "sdk")
		if err != nil {
			t.Fatalf("LoadNodeState(sdk): %v", err)
		}
		if ne.Status != StatusSucceeded {
			t.Errorf("sdk status = %s, want Succeeded", ne.Status)
		}
		if len(ne.Outputs) != 1 {
			t.Errorf("sdk outputs = %d, want 1", len(ne.Outputs))
		}
	})
}
