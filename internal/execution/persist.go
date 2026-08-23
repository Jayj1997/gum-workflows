package execution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// stateDirLayout 对应设计计划 §28 的 Execution State 目录结构：
//
//	<dir>/
//	├── state.json                       WorkflowExecution 级状态
//	└── nodes/
//	    └── <node-id>/state.json         NodeExecution 级状态
//
// Artifact 文件由 ArtifactStore 负责，不在本包布局内。

// PersistState 将 WorkflowExecution 状态写入 dir（state.json + nodes/<id>/state.json）。
// 目录已存在时覆盖写入（同一执行的最新快照）。
func PersistState(dir string, exec *WorkflowExecution) error {
	if exec == nil {
		return fmt.Errorf("persist state: execution must not be nil")
	}

	if err := os.MkdirAll(nodesDir(dir), 0o755); err != nil {
		return fmt.Errorf("persist state: create %s: %w", nodesDir(dir), err)
	}

	// WorkflowExecution 级：ID、Workflow 名、整体状态、各 Node 状态摘要。
	top := map[string]any{
		"id":        exec.ID,
		"workflow":  exec.Workflow,
		"status":    string(exec.Status),
		"nodeCount": len(exec.Nodes),
	}
	if err := writeJSON(filepath.Join(dir, "state.json"), top); err != nil {
		return fmt.Errorf("persist state %s: %w", exec.ID, err)
	}

	// NodeExecution 级：每个 Node 一个 state.json（快照，含定义身份）。
	for id, ne := range exec.Nodes {
		nodeDir := filepath.Join(nodesDir(dir), id)
		if err := os.MkdirAll(nodeDir, 0o755); err != nil {
			return fmt.Errorf("persist state: create %s: %w", nodeDir, err)
		}
		if err := writeJSON(filepath.Join(nodeDir, "state.json"), ne); err != nil {
			return fmt.Errorf("persist state %s/%s: %w", exec.ID, id, err)
		}
	}
	return nil
}

// LoadNodeState 从 dir/nodes/<nodeID>/state.json 读回一个 NodeExecution。
func LoadNodeState(dir, nodeID string) (*NodeExecution, error) {
	data, err := os.ReadFile(filepath.Join(nodesDir(dir), nodeID, "state.json"))
	if err != nil {
		return nil, fmt.Errorf("load node state %q: %w", nodeID, err)
	}
	var ne NodeExecution
	if err := json.Unmarshal(data, &ne); err != nil {
		return nil, fmt.Errorf("load node state %q: %w", nodeID, err)
	}
	return &ne, nil
}

func nodesDir(dir string) string { return filepath.Join(dir, "nodes") }

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
