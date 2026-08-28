package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
)

// RunSummary is one row in the recent-run history list.
type RunSummary struct {
	ID              string
	Workflow        string
	WorkflowVersion string
	Status          execution.Status
	StartedAt       time.Time
	FinishedAt      time.Time
	NodesCompleted  int
	NodesTotal      int
}

// RunDetail is one workflow run and the latest summary for each of its nodes.
type RunDetail struct {
	RunSummary
	WorkflowFile  string
	ExecutionID   string
	Error         string
	StoppedReason string
	Nodes         []NodeSummary
}

// NodeSummary describes the latest round of one node in a workflow run.
type NodeSummary struct {
	NodeID         string
	NodeDefinition string
	NodeExecutor   string
	Status         execution.Status
	Error          string
	ErrorKind      string
	StartedAt      time.Time
	FinishedAt     time.Time
	Rounds         int
	Inputs         int
	Outputs        int
	RoundDetails   []NodeRoundSummary
}

// NodeRoundSummary is the compact history of one node execution round.
type NodeRoundSummary struct {
	Round      int
	Status     execution.Status
	Error      string
	ErrorKind  string
	StartedAt  time.Time
	FinishedAt time.Time
	Inputs     int
	Outputs    int
}

// NodeDetail contains every recorded round for one node in a workflow run.
type NodeDetail struct {
	RunID          string
	NodeID         string
	NodeDefinition string
	NodeExecutor   string
	Rounds         []NodeRound
}

// NodeRound is one recorded execution attempt with artifact references only.
type NodeRound struct {
	RunID      string
	Round      int
	Status     execution.Status
	Error      string
	ErrorKind  string
	Inputs     map[string]execution.InputSnapshot
	Outputs    map[string]artifact.ArtifactRef
	StartedAt  time.Time
	FinishedAt time.Time
}

// ListRuns returns the 20 most recently started workflow runs.
func (s *Store) ListRuns(ctx context.Context) ([]RunSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.id, r.workflow_name, r.workflow_version, r.status, r.started_at, r.finished_at,
       (SELECT count(*)
          FROM workflow_node_run_history n
         WHERE n.run_id = r.id
           AND n.round = (SELECT max(latest.round)
                            FROM workflow_node_run_history latest
                           WHERE latest.run_id = n.run_id AND latest.node_id = n.node_id)
           AND n.status <> ?) AS nodes_completed,
       (SELECT count(DISTINCT n.node_id)
          FROM workflow_node_run_history n
         WHERE n.run_id = r.id) AS nodes_total
  FROM workflow_run_history r
 ORDER BY r.started_at DESC, r.id DESC
	LIMIT 20`, string(execution.StatusPending))
	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}
	defer rows.Close()

	runs := make([]RunSummary, 0, 20)
	for rows.Next() {
		var run RunSummary
		var status string
		var started string
		var finished sql.NullString
		if err := rows.Scan(
			&run.ID, &run.Workflow, &run.WorkflowVersion, &status, &started, &finished,
			&run.NodesCompleted, &run.NodesTotal,
		); err != nil {
			return nil, fmt.Errorf("scan workflow run: %w", err)
		}
		run.Status = execution.Status(status)
		var err error
		run.StartedAt, err = parseStoredTime(started)
		if err != nil {
			return nil, fmt.Errorf("parse workflow run %s started_at: %w", run.ID, err)
		}
		if finished.Valid {
			run.FinishedAt, err = parseStoredTime(finished.String)
			if err != nil {
				return nil, fmt.Errorf("parse workflow run %s finished_at: %w", run.ID, err)
			}
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow runs: %w", err)
	}
	return runs, nil
}

// GetRun resolves a full run ID or UUID prefix and returns its run details.
// A missing run is represented by (nil, nil).
func (s *Store) GetRun(ctx context.Context, idOrPrefix string) (*RunDetail, error) {
	runID, found, err := s.resolveRunID(ctx, idOrPrefix)
	if err != nil || !found {
		return nil, err
	}

	var run RunDetail
	var status string
	var started string
	var finished sql.NullString
	err = s.db.QueryRowContext(ctx, `
SELECT id, workflow_name, workflow_version, status, workflow_file, execution_id,
       error, stopped_reason, started_at, finished_at
  FROM workflow_run_history
 WHERE id = ?`, runID).Scan(
		&run.ID, &run.Workflow, &run.WorkflowVersion, &status, &run.WorkflowFile,
		&run.ExecutionID, &run.Error, &run.StoppedReason, &started, &finished,
	)
	if err != nil {
		return nil, fmt.Errorf("get workflow run %s: %w", runID, err)
	}
	run.Status = execution.Status(status)
	run.StartedAt, err = parseStoredTime(started)
	if err != nil {
		return nil, fmt.Errorf("parse workflow run %s started_at: %w", runID, err)
	}
	if finished.Valid {
		run.FinishedAt, err = parseStoredTime(finished.String)
		if err != nil {
			return nil, fmt.Errorf("parse workflow run %s finished_at: %w", runID, err)
		}
	}

	run.Nodes, err = s.listLatestNodes(ctx, runID)
	if err != nil {
		return nil, err
	}
	for _, node := range run.Nodes {
		if node.Status != execution.StatusPending {
			run.NodesCompleted++
		}
	}
	run.NodesTotal = len(run.Nodes)
	return &run, nil
}

// GetNodeRun resolves a run ID or prefix and returns all rounds for nodeID.
// A missing run or node is represented by (nil, nil).
func (s *Store) GetNodeRun(ctx context.Context, idOrPrefix, nodeID string) (*NodeDetail, error) {
	runID, found, err := s.resolveRunID(ctx, idOrPrefix)
	if err != nil || !found {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, node_definition, node_executor, round, status, error, error_kind,
       inputs_json, outputs_json, started_at, finished_at
  FROM workflow_node_run_history
 WHERE run_id = ? AND node_id = ?
 ORDER BY round`, runID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("get run %s node %q: %w", runID, nodeID, err)
	}
	defer rows.Close()

	detail := &NodeDetail{RunID: runID, NodeID: nodeID}
	for rows.Next() {
		var round NodeRound
		var status string
		var inputsJSON, outputsJSON string
		var started, finished sql.NullString
		if err := rows.Scan(
			&round.RunID, &detail.NodeDefinition, &detail.NodeExecutor, &round.Round,
			&status, &round.Error, &round.ErrorKind, &inputsJSON, &outputsJSON, &started, &finished,
		); err != nil {
			return nil, fmt.Errorf("scan run %s node %q round: %w", runID, nodeID, err)
		}
		round.Status = execution.Status(status)
		if err := json.Unmarshal([]byte(inputsJSON), &round.Inputs); err != nil {
			return nil, fmt.Errorf("decode run %s node %q round %d inputs: %w", runID, nodeID, round.Round, err)
		}
		if err := json.Unmarshal([]byte(outputsJSON), &round.Outputs); err != nil {
			return nil, fmt.Errorf("decode run %s node %q round %d outputs: %w", runID, nodeID, round.Round, err)
		}
		if started.Valid {
			round.StartedAt, err = parseStoredTime(started.String)
			if err != nil {
				return nil, fmt.Errorf("parse run %s node %q round %d started_at: %w", runID, nodeID, round.Round, err)
			}
		}
		if finished.Valid {
			round.FinishedAt, err = parseStoredTime(finished.String)
			if err != nil {
				return nil, fmt.Errorf("parse run %s node %q round %d finished_at: %w", runID, nodeID, round.Round, err)
			}
		}
		detail.Rounds = append(detail.Rounds, round)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run %s node %q rounds: %w", runID, nodeID, err)
	}
	if len(detail.Rounds) == 0 {
		return nil, nil
	}
	return detail, nil
}

func (s *Store) resolveRunID(ctx context.Context, prefix string) (string, bool, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if len(prefix) < 8 {
		return "", false, fmt.Errorf("run id prefix must be at least 8 characters")
	}
	if len(prefix) > 36 || strings.IndexFunc(prefix, func(r rune) bool {
		return !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || r == '-')
	}) >= 0 {
		return "", false, fmt.Errorf("run id prefix %q is not a UUID prefix", prefix)
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id FROM workflow_run_history
 WHERE substr(id, 1, ?) = ?
 ORDER BY id`, len(prefix), prefix)
	if err != nil {
		return "", false, fmt.Errorf("resolve run id prefix %q: %w", prefix, err)
	}
	defer rows.Close()
	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", false, fmt.Errorf("scan run id candidate: %w", err)
		}
		candidates = append(candidates, id)
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("iterate run id candidates: %w", err)
	}
	switch len(candidates) {
	case 0:
		return "", false, nil
	case 1:
		return candidates[0], true, nil
	default:
		return "", false, fmt.Errorf("run id prefix %q is ambiguous; candidates: %s", prefix, strings.Join(candidates, ", "))
	}
}

func (s *Store) listLatestNodes(ctx context.Context, runID string) ([]NodeSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT node_id, node_definition, node_executor, round, status, error, error_kind,
       inputs_json, outputs_json, started_at, finished_at
  FROM workflow_node_run_history
 WHERE run_id = ?
 ORDER BY node_id, round`, runID)
	if err != nil {
		return nil, fmt.Errorf("list nodes for run %s: %w", runID, err)
	}
	defer rows.Close()

	var nodes []NodeSummary
	for rows.Next() {
		var nodeID, nodeDefinition, nodeExecutor, status string
		var round NodeRoundSummary
		var inputsJSON, outputsJSON string
		var started, finished sql.NullString
		if err := rows.Scan(
			&nodeID, &nodeDefinition, &nodeExecutor, &round.Round, &status,
			&round.Error, &round.ErrorKind, &inputsJSON, &outputsJSON, &started, &finished,
		); err != nil {
			return nil, fmt.Errorf("scan node for run %s: %w", runID, err)
		}
		round.Status = execution.Status(status)
		var inputs, outputs map[string]json.RawMessage
		if err := json.Unmarshal([]byte(inputsJSON), &inputs); err != nil {
			return nil, fmt.Errorf("decode run %s node %q round %d inputs: %w", runID, nodeID, round.Round, err)
		}
		if err := json.Unmarshal([]byte(outputsJSON), &outputs); err != nil {
			return nil, fmt.Errorf("decode run %s node %q round %d outputs: %w", runID, nodeID, round.Round, err)
		}
		round.Inputs, round.Outputs = len(inputs), len(outputs)
		if started.Valid {
			round.StartedAt, err = parseStoredTime(started.String)
			if err != nil {
				return nil, fmt.Errorf("parse run %s node %q round %d started_at: %w", runID, nodeID, round.Round, err)
			}
		}
		if finished.Valid {
			round.FinishedAt, err = parseStoredTime(finished.String)
			if err != nil {
				return nil, fmt.Errorf("parse run %s node %q round %d finished_at: %w", runID, nodeID, round.Round, err)
			}
		}
		if len(nodes) == 0 || nodes[len(nodes)-1].NodeID != nodeID {
			nodes = append(nodes, NodeSummary{
				NodeID: nodeID, NodeDefinition: nodeDefinition, NodeExecutor: nodeExecutor,
			})
		}
		node := &nodes[len(nodes)-1]
		node.Status, node.Error, node.ErrorKind = round.Status, round.Error, round.ErrorKind
		node.StartedAt, node.FinishedAt = round.StartedAt, round.FinishedAt
		node.Inputs, node.Outputs = round.Inputs, round.Outputs
		node.RoundDetails = append(node.RoundDetails, round)
		node.Rounds = len(node.RoundDetails)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nodes for run %s: %w", runID, err)
	}
	return nodes, nil
}

func parseStoredTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
