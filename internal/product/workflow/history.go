package workflow

import "context"

// RunHistoryRepository reads persisted Product Workflow Revisions, Runs, Node Runs
// and Artifacts. It is the read counterpart to RunRepository's atomic publish seam
// and returns domain types; the Application maps them to UI views.
type RunHistoryRepository interface {
	// ListProductWorkflowRevisions returns every immutable Revision for one Product
	// Workflow in stable creation order. Each distinct semantic hash appears once
	// because the write path stores only one Revision per (workflow, hash).
	ListProductWorkflowRevisions(ctx context.Context, workflowID string) ([]Revision, error)
	// ListProductWorkflowRevisionRuns returns every Run that executed one Revision,
	// in stable started-at order.
	ListProductWorkflowRevisionRuns(ctx context.Context, revisionID string) ([]Run, error)
	// GetProductRun returns one Run together with its Node Runs and Artifact
	// references. A missing Run is represented by (zero, nil, nil, ErrRunNotFound).
	GetProductRun(ctx context.Context, runID string) (Run, []NodeRun, []RunArtifact, error)
}

// ErrRunNotFound is the sentinel for a Product Run that does not exist.
var ErrRunNotFound = runNotFoundError{}

type runNotFoundError struct{}

func (runNotFoundError) Error() string { return "product workflow run not found" }
