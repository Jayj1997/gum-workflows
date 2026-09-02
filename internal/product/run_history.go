package product

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// RevisionView is one immutable Product Workflow definition version returned to UI adapters.
type RevisionView struct {
	ID           string    `json:"id"`
	SemanticHash string    `json:"semanticHash"`
	RunCount     int       `json:"runCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// RunSummaryView is one Run row in the per-Revision history list.
type RunSummaryView struct {
	ID         string              `json:"id"`
	RevisionID string              `json:"revisionId"`
	Status     string              `json:"status"`
	Error      *ExecutionErrorView `json:"error,omitempty"`
	StartedAt  time.Time           `json:"startedAt"`
	FinishedAt time.Time           `json:"finishedAt"`
}

// ListRevisions returns every immutable Revision for one Product Workflow with its
// Run count, in stable creation order.
func (a *Application) ListRevisions(ctx context.Context, workflowID string) ([]RevisionView, error) {
	history, err := a.requireRunHistory()
	if err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}
	revisions, err := history.ListProductWorkflowRevisions(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}
	views := make([]RevisionView, 0, len(revisions))
	for _, revision := range revisions {
		runs, err := history.ListProductWorkflowRevisionRuns(ctx, revision.ID)
		if err != nil {
			return nil, fmt.Errorf("count revisions runs: %w", err)
		}
		views = append(views, RevisionView{ID: revision.ID, SemanticHash: revision.SemanticHash, RunCount: len(runs), CreatedAt: revision.CreatedAt})
	}
	return views, nil
}

// ListRevisionRuns returns every Run that executed one Revision, in stable order.
func (a *Application) ListRevisionRuns(ctx context.Context, revisionID string) ([]RunSummaryView, error) {
	history, err := a.requireRunHistory()
	if err != nil {
		return nil, fmt.Errorf("list revision runs: %w", err)
	}
	runs, err := history.ListProductWorkflowRevisionRuns(ctx, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list revision runs: %w", err)
	}
	views := make([]RunSummaryView, 0, len(runs))
	for _, run := range runs {
		views = append(views, RunSummaryView{ID: run.ID, RevisionID: run.RevisionID, Status: run.Status, Error: run.Error, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt})
	}
	return views, nil
}

// GetRunHistory returns one historical Run with its Node Runs, Snapshot and
// reconstructed Conversation messages. The Draft shows the Revision content that
// actually ran. A missing Run is represented by (zero, ErrRunNotFound-wrapped).
func (a *Application) GetRunHistory(ctx context.Context, runID string) (RunView, error) {
	history, err := a.requireRunHistory()
	if err != nil {
		return RunView{}, fmt.Errorf("get run history: %w", err)
	}
	run, nodeRuns, artifacts, err := history.GetProductRun(ctx, runID)
	if err != nil {
		return RunView{}, fmt.Errorf("get run history: %w", err)
	}
	draft := a.revisionDraftView(ctx, run)
	nodeRunViews := make([]NodeRunView, 0, len(nodeRuns))
	for _, nodeRun := range nodeRuns {
		nodeRunViews = append(nodeRunViews, nodeRunView(nodeRun))
	}
	artifactViews := a.reconstructArtifactViews(run.ID, artifacts)
	return RunView{ID: run.ID, RevisionID: run.RevisionID, Status: run.Status, Error: run.Error, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Draft: draft, Snapshot: runSnapshotView(run.Snapshot), NodeRuns: nodeRunViews, Artifacts: artifactViews}, nil
}

// revisionDraftView decodes the Revision content for a Run into a DraftView with a
// Preview, so the UI shows the definition version that ran. It is best-effort: a
// lookup or decode failure yields an empty Draft rather than failing the query.
func (a *Application) revisionDraftView(ctx context.Context, run productworkflow.Run) DraftView {
	history, err := a.requireRunHistory()
	if err != nil {
		return DraftView{}
	}
	revisions, err := history.ListProductWorkflowRevisions(ctx, run.WorkflowID)
	if err != nil {
		return DraftView{}
	}
	for _, revision := range revisions {
		if revision.ID != run.RevisionID {
			continue
		}
		view, err := draftView(productworkflow.Draft{WorkflowID: run.WorkflowID, Content: revision.Content})
		if err != nil {
			return DraftView{}
		}
		// Historical Revisions keep the Model UUID that ran; if the Slot was
		// deleted since, the Run Snapshot remains authoritative for the
		// resolved selection, so the historical Preview must not flag it.
		preview := a.previewDraftWithModels(view.Content, nil)
		view.Preview = &preview
		return view
	}
	return DraftView{}
}

// reconstructArtifactViews rebuilds user-visible Artifact views, decoding
// Conversation bodies from the filesystem Artifact Store. Missing or unreadable
// Artifacts degrade to an empty Messages list so one bad file never fails the
// whole history query.
func (a *Application) reconstructArtifactViews(runID string, artifacts []productworkflow.RunArtifact) []ArtifactView {
	store, storeErr := artifact.NewFilesystemStore(a.runPaths.ArtifactsDir(runID))
	views := make([]ArtifactView, 0, len(artifacts))
	for _, item := range artifacts {
		views = append(views, a.reconstructArtifactView(store, storeErr, item))
	}
	return views
}

func (a *Application) reconstructArtifactView(store *artifact.FilesystemStore, storeErr error, item productworkflow.RunArtifact) ArtifactView {
	if storeErr != nil || item.Type != string(artifact.KindConversation) {
		return artifactView(item, productworkflow.Conversation{})
	}
	got, err := store.Get(artifact.ArtifactRef{ID: item.ID, Kind: artifact.KindConversation, Version: item.Version, URI: item.URI})
	if err != nil {
		return artifactView(item, productworkflow.Conversation{})
	}
	data, err := json.Marshal(got.Data)
	if err != nil {
		return artifactView(item, productworkflow.Conversation{})
	}
	var conversation productworkflow.Conversation
	if err := json.Unmarshal(data, &conversation); err != nil {
		return artifactView(item, productworkflow.Conversation{})
	}
	return artifactView(item, conversation)
}

func (a *Application) requireRunHistory() (productworkflow.RunHistoryRepository, error) {
	if a.runHistory == nil {
		return nil, fmt.Errorf("product run history repository is not configured")
	}
	return a.runHistory, nil
}
