package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// ListProductWorkflowRevisions returns every immutable Revision for one Product
// Workflow in stable creation order (oldest first).
func (s *Store) ListProductWorkflowRevisions(ctx context.Context, workflowID string) ([]productworkflow.Revision, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, semantic_hash, content_json, created_at
  FROM product_workflow_revision
 WHERE workflow_id = ?
 ORDER BY created_at ASC, id ASC`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list product workflow revisions %s: %w", workflowID, err)
	}
	defer rows.Close()
	revisions := make([]productworkflow.Revision, 0)
	for rows.Next() {
		var revision productworkflow.Revision
		var contentJSON, createdAt string
		if err := rows.Scan(&revision.ID, &revision.SemanticHash, &contentJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan product workflow revision %s: %w", workflowID, err)
		}
		revision.WorkflowID = workflowID
		revision.Content = []byte(contentJSON)
		revision.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse product workflow revision %s created_at: %w", revision.ID, err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product workflow revisions %s: %w", workflowID, err)
	}
	return revisions, nil
}

// ListProductWorkflowRevisionRuns returns every Run that executed one Revision,
// in stable started-at order.
func (s *Store) ListProductWorkflowRevisionRuns(ctx context.Context, revisionID string) ([]productworkflow.Run, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, workflow_id, revision_id, status, snapshot_json, started_at, finished_at
  FROM product_workflow_run
 WHERE revision_id = ?
 ORDER BY started_at ASC, id ASC`, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list product workflow revision runs %s: %w", revisionID, err)
	}
	defer rows.Close()
	runs := make([]productworkflow.Run, 0)
	for rows.Next() {
		run, err := scanProductWorkflowRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product workflow run for revision %s: %w", revisionID, err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product workflow revision runs %s: %w", revisionID, err)
	}
	return runs, nil
}

// scanProductWorkflowRun decodes one product_workflow_run row, including its
// snapshot JSON and stored timestamps.
func scanProductWorkflowRun(row rowScanner) (productworkflow.Run, error) {
	var run productworkflow.Run
	var snapshotJSON, started, finished string
	if err := row.Scan(&run.ID, &run.WorkflowID, &run.RevisionID, &run.Status, &snapshotJSON, &started, &finished); err != nil {
		return productworkflow.Run{}, err
	}
	if err := json.Unmarshal([]byte(snapshotJSON), &run.Snapshot); err != nil {
		return productworkflow.Run{}, fmt.Errorf("decode product workflow run %s snapshot: %w", run.ID, err)
	}
	var err error
	if run.StartedAt, err = parseStoredTime(started); err != nil {
		return productworkflow.Run{}, fmt.Errorf("parse product workflow run %s started_at: %w", run.ID, err)
	}
	if run.FinishedAt, err = parseStoredTime(finished); err != nil {
		return productworkflow.Run{}, fmt.Errorf("parse product workflow run %s finished_at: %w", run.ID, err)
	}
	return run, nil
}

// GetProductRun returns one Run together with its Node Runs and Artifact references.
// A missing Run is represented by (zero, nil, nil, ErrRunNotFound).
func (s *Store) GetProductRun(ctx context.Context, runID string) (productworkflow.Run, []productworkflow.NodeRun, []productworkflow.RunArtifact, error) {
	run, err := scanProductWorkflowRun(s.db.QueryRowContext(ctx, `
SELECT id, workflow_id, revision_id, status, snapshot_json, started_at, finished_at
  FROM product_workflow_run
 WHERE id = ?`, runID))
	if err == sql.ErrNoRows {
		return productworkflow.Run{}, nil, nil, productworkflow.ErrRunNotFound
	}
	if err != nil {
		return productworkflow.Run{}, nil, nil, fmt.Errorf("get product workflow run %s: %w", runID, err)
	}

	nodeRuns, err := s.listProductNodeRuns(ctx, runID)
	if err != nil {
		return productworkflow.Run{}, nil, nil, err
	}
	artifacts, err := s.listProductRunArtifacts(ctx, runID)
	if err != nil {
		return productworkflow.Run{}, nil, nil, err
	}
	return run, nodeRuns, artifacts, nil
}

func (s *Store) listProductNodeRuns(ctx context.Context, runID string) ([]productworkflow.NodeRun, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, run_id, node_id, node_definition, node_executor, status, inputs_json, outputs_json, started_at, finished_at
  FROM product_workflow_node_run
 WHERE run_id = ?
 ORDER BY started_at ASC, id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("list product node runs %s: %w", runID, err)
	}
	defer rows.Close()
	nodeRuns := make([]productworkflow.NodeRun, 0)
	for rows.Next() {
		var nodeRun productworkflow.NodeRun
		var inputsJSON, outputsJSON, started, finished string
		if err := rows.Scan(&nodeRun.ID, &nodeRun.RunID, &nodeRun.NodeID, &nodeRun.NodeDefinition, &nodeRun.NodeExecutor, &nodeRun.Status, &inputsJSON, &outputsJSON, &started, &finished); err != nil {
			return nil, fmt.Errorf("scan product node run for run %s: %w", runID, err)
		}
		if err := json.Unmarshal([]byte(inputsJSON), &nodeRun.Inputs); err != nil {
			return nil, fmt.Errorf("decode product node run %s inputs: %w", nodeRun.ID, err)
		}
		if err := json.Unmarshal([]byte(outputsJSON), &nodeRun.Outputs); err != nil {
			return nil, fmt.Errorf("decode product node run %s outputs: %w", nodeRun.ID, err)
		}
		if nodeRun.StartedAt, err = parseStoredTime(started); err != nil {
			return nil, fmt.Errorf("parse product node run %s started_at: %w", nodeRun.ID, err)
		}
		if nodeRun.FinishedAt, err = parseStoredTime(finished); err != nil {
			return nil, fmt.Errorf("parse product node run %s finished_at: %w", nodeRun.ID, err)
		}
		nodeRuns = append(nodeRuns, nodeRun)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product node runs %s: %w", runID, err)
	}
	return nodeRuns, nil
}

func (s *Store) listProductRunArtifacts(ctx context.Context, runID string) ([]productworkflow.RunArtifact, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, run_id, node_run_id, node_id, port, artifact_type, version, uri, created_at
  FROM product_workflow_artifact
 WHERE run_id = ?
 ORDER BY created_at ASC, id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("list product artifacts %s: %w", runID, err)
	}
	defer rows.Close()
	artifacts := make([]productworkflow.RunArtifact, 0)
	for rows.Next() {
		var item productworkflow.RunArtifact
		var createdAt string
		if err := rows.Scan(&item.ID, &item.RunID, &item.NodeRunID, &item.NodeID, &item.Port, &item.Type, &item.Version, &item.URI, &createdAt); err != nil {
			return nil, fmt.Errorf("scan product artifact for run %s: %w", runID, err)
		}
		if item.CreatedAt, err = parseStoredTime(createdAt); err != nil {
			return nil, fmt.Errorf("parse product artifact %s created_at: %w", item.ID, err)
		}
		artifacts = append(artifacts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product artifacts %s: %w", runID, err)
	}
	return artifacts, nil
}

// Compile-time assertion that Store satisfies the read seam.
var _ productworkflow.RunHistoryRepository = (*Store)(nil)
