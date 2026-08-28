package execution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// stateDirLayout follows the iterative execution layout:
//
//	<dir>/
//	├── state.json                       WorkflowExecution 级状态
//	└── nodes/
//	    └── <node-id>/
//	        ├── state.json               Current + history summary
//	        └── runs/<round>.json        Full round detail
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

	// WorkflowExecution level metadata does not duplicate node round bodies.
	top := map[string]any{
		"id":             exec.ID,
		"run_id":         exec.RunID,
		"workflow":       exec.Workflow,
		"workflow_file":  exec.WorkflowFile,
		"status":         string(exec.Status),
		"stopped_reason": exec.StoppedReason,
		"error":          exec.Error,
		"started_at":     exec.StartedAt,
		"finished_at":    exec.FinishedAt,
		"node_count":     len(exec.Nodes),
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
		nodeState := struct {
			NodeID   string    `json:"node_id"`
			NodeType string    `json:"node_type"`
			Current  NodeRun   `json:"current"`
			History  []NodeRun `json:"history,omitempty"`
		}{
			NodeID: ne.NodeID, NodeType: ne.NodeType, Current: ne.Current,
			History: summarizeRuns(ne.History),
		}
		if err := writeJSON(filepath.Join(nodeDir, "state.json"), nodeState); err != nil {
			return fmt.Errorf("persist state %s/%s: %w", exec.ID, id, err)
		}
		runsDir := filepath.Join(nodeDir, "runs")
		if err := os.MkdirAll(runsDir, 0o755); err != nil {
			return fmt.Errorf("persist state: create %s: %w", runsDir, err)
		}
		for _, run := range append(append([]NodeRun{}, ne.History...), ne.Current) {
			if run.Round == 0 {
				continue
			}
			if err := writeJSON(filepath.Join(runsDir, fmt.Sprintf("%d.json", run.Round)), run); err != nil {
				return fmt.Errorf("persist state %s/%s round %d: %w", exec.ID, id, run.Round, err)
			}
		}
	}
	return nil
}

func summarizeRuns(runs []NodeRun) []NodeRun {
	summaries := make([]NodeRun, len(runs))
	for i, run := range runs {
		summaries[i] = NodeRun{
			RunID: run.RunID, Round: run.Round, Status: run.Status,
			Error: run.Error, ErrorKind: run.ErrorKind,
			StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		}
	}
	return summaries
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
