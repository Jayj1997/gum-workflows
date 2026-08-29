package history_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
)

func TestMigrateLegacyPreservesHistoryAndArtifacts(t *testing.T) {
	ctx := context.Background()
	source, destination := migrationPaths(t)
	fixture := seedLegacyFixture(t, ctx, source)
	seedEquivalentTargetDefinitions(t, ctx, destination)

	if err := history.MigrateLegacy(ctx, source, destination); err != nil {
		t.Fatalf("MigrateLegacy() unexpected error: %v", err)
	}

	store, err := history.OpenReadOnly(ctx, destination.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run, err := store.GetRun(ctx, fixture.runID[:8])
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.ID != fixture.runID || run.ExecutionID != fixture.executionID || run.Workflow != "legacy-quality" {
		t.Fatalf("GetRun() = %+v, want migrated legacy run", run)
	}
	detail, err := store.GetNodeRun(ctx, fixture.runID, "check")
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil || len(detail.Rounds) != 1 {
		t.Fatalf("GetNodeRun() = %+v, want one migrated round", detail)
	}
	round := detail.Rounds[0]
	if round.NodeRunID != fixture.nodeRunID || round.Outputs["result"] != fixture.ref {
		t.Fatalf("migrated round = %+v, want IDs and ArtifactRef preserved", round)
	}

	migratedArtifacts, err := artifact.NewFilesystemStore(destination.ArtifactsDir(fixture.executionID))
	if err != nil {
		t.Fatal(err)
	}
	got, err := migratedArtifacts.Get(fixture.ref)
	if err != nil {
		t.Fatalf("read migrated Artifact: %v", err)
	}
	if got.ID != "quality-result" || got.Data != "legacy passed" {
		t.Fatalf("migrated Artifact = %+v", got)
	}
	assertDatabaseID(t, destination.Database(), "node_type_definition", "name", "automation", "target-type-id")
	assertDatabaseID(t, destination.Database(), "node_definition", "name", "go-static-analysis", "target-definition-id")
	assertDatabaseID(t, destination.Database(), "workflow", "name", "legacy-quality", "legacy-workflow-id")
	if _, err := os.Stat(filepath.Join(filepath.Dir(source.Database()), "migration-marker")); !os.IsNotExist(err) {
		t.Fatalf("migration modified legacy directory: %v", err)
	}
	legacyArtifacts, err := artifact.NewFilesystemStore(source.ArtifactsDir(fixture.executionID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacyArtifacts.Get(fixture.ref); err != nil {
		t.Fatalf("migration removed or modified legacy Artifact: %v", err)
	}
}

func assertDatabaseID(t *testing.T, database, table, key, value, want string) {
	t.Helper()
	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var got string
	query := "SELECT id FROM " + table + " WHERE " + key + " = ?" // test-only allowlisted identifiers
	if err := db.QueryRow(query, value).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s identity = %q, want %q", table, got, want)
	}
}

func TestMigrateLegacyReplayIsIdempotent(t *testing.T) {
	ctx := context.Background()
	source, destination := migrationPaths(t)
	fixture := seedLegacyFixture(t, ctx, source)

	for attempt := 1; attempt <= 2; attempt++ {
		if err := history.MigrateLegacy(ctx, source, destination); err != nil {
			t.Fatalf("MigrateLegacy() attempt %d: %v", attempt, err)
		}
	}

	store, err := history.OpenReadOnly(ctx, destination.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runs, err := store.ListRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != fixture.runID {
		t.Fatalf("ListRuns() = %+v, want one preserved Run ID", runs)
	}
	detail, err := store.GetNodeRun(ctx, fixture.runID, "check")
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil || len(detail.Rounds) != 1 || detail.Rounds[0].NodeRunID != fixture.nodeRunID {
		t.Fatalf("GetNodeRun() = %+v, want one preserved Node Run ID", detail)
	}
	artifacts, err := artifact.NewFilesystemStore(destination.ArtifactsDir(fixture.executionID))
	if err != nil {
		t.Fatal(err)
	}
	files, err := artifacts.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != fixture.ref.URI {
		t.Fatalf("migrated Artifact files = %v, want one preserved body", files)
	}
}

func TestMigrateLegacyRejectsConflictingRunWithoutPublishingArtifacts(t *testing.T) {
	ctx := context.Background()
	source, destination := migrationPaths(t)
	fixture := seedLegacyFixture(t, ctx, source)
	target, err := history.Open(ctx, destination.Database())
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	if err := target.Record(ctx, &execution.WorkflowExecution{
		ID: fixture.executionID, RunID: fixture.runID, Workflow: "different-workflow",
		Status: execution.StatusStopped, StartedAt: started, FinishedAt: started.Add(time.Second),
		Nodes: map[string]*execution.NodeExecution{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}

	err = history.MigrateLegacy(ctx, source, destination)
	if err == nil {
		t.Fatal("MigrateLegacy() = nil error, want conflicting Run rejection")
	}

	target, err = history.OpenReadOnly(ctx, destination.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	run, err := target.GetRun(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.Workflow != "different-workflow" {
		t.Fatalf("conflicting destination Run was modified: %+v", run)
	}
	if _, err := os.Stat(filepath.Join(destination.ArtifactsDir(fixture.executionID), fixture.ref.URI)); !os.IsNotExist(err) {
		t.Fatalf("conflict published an Artifact: %v", err)
	}
}

func TestMigrateLegacyMissingArtifactRollsBackVisibleHistory(t *testing.T) {
	ctx := context.Background()
	source, destination := migrationPaths(t)
	fixture := seedLegacyFixture(t, ctx, source)
	if err := os.Remove(filepath.Join(source.ArtifactsDir(fixture.executionID), fixture.ref.URI)); err != nil {
		t.Fatal(err)
	}

	if err := history.MigrateLegacy(ctx, source, destination); err == nil {
		t.Fatal("MigrateLegacy() = nil error, want missing Artifact failure")
	}

	target, err := history.OpenReadOnly(ctx, destination.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	run, err := target.GetRun(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if run != nil {
		t.Fatalf("failed migration published partial Run: %+v", run)
	}
	detail, err := target.GetNodeRun(ctx, fixture.runID, "check")
	if err != nil {
		t.Fatal(err)
	}
	if detail != nil {
		t.Fatalf("failed migration published partial Node Run: %+v", detail)
	}
}

type legacyFixture struct {
	runID       string
	nodeRunID   string
	executionID string
	ref         artifact.ArtifactRef
}

func migrationPaths(t *testing.T) (runtimepath.Paths, runtimepath.Paths) {
	t.Helper()
	root := t.TempDir()
	source, err := runtimepath.New(
		filepath.Join(root, "project", ".workflow", "gum-workflows.db"),
		filepath.Join(root, "project", ".workflow", "executions"),
	)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := runtimepath.New(
		filepath.Join(root, "local-data", "product.db"),
		filepath.Join(root, "local-data", "runs"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return source, destination
}

func seedLegacyFixture(t *testing.T, ctx context.Context, paths runtimepath.Paths) legacyFixture {
	t.Helper()
	store, err := history.Open(ctx, paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.ImportDefinitions(ctx,
		[]history.NodeTypeDefRow{{ID: "legacy-type-id", Name: "automation"}},
		[]history.NodeDefRow{{
			ID: "legacy-definition-id", Name: "go-static-analysis", Type: "automation",
			Inputs:  map[string]history.InputPort{"code": {Type: "SourceCode"}},
			Outputs: map[string]history.OutputPort{"result": {Type: "QualityCheckResult"}},
		}},
		[]history.NodeExecRow{{
			ID: "legacy-executor-id", Node: "go-static-analysis", Version: "v1", Name: "Go Static Analysis",
		}},
	); err != nil {
		t.Fatal(err)
	}
	if err := store.ImportWorkflow(ctx, history.WorkflowRow{
		ID: "legacy-workflow-id", Name: "legacy-quality", Version: "v1",
		Projects: []history.ProjectRow{{Name: "project", Repository: "."}},
	}, []history.NodeInstanceRow{{
		ID: "legacy-instance-id", NodeID: "check",
		NodeDefinitionID: "legacy-definition-id", NodeExecutorID: "legacy-executor-id",
		Inputs: map[string]history.InputBinding{"code": {From: "project.code"}},
	}}); err != nil {
		t.Fatal(err)
	}

	executionID := "execution-000007"
	artifactStore, err := artifact.NewFilesystemStore(paths.ArtifactsDir(executionID))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := artifactStore.Put(artifact.Artifact{
		ID: "quality-result", Kind: "QualityCheckResult", Version: "1", Data: "legacy passed",
	})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	fixture := legacyFixture{
		runID: "11111111-1111-4111-8111-111111111111", nodeRunID: "22222222-2222-4222-8222-222222222222",
		executionID: executionID, ref: ref,
	}
	if err := store.Record(ctx, &execution.WorkflowExecution{
		ID: executionID, RunID: fixture.runID, Workflow: "legacy-quality", WorkflowVersion: "v1",
		WorkflowFile: "/legacy/project/workflow.yaml", Status: execution.StatusStopped,
		StartedAt: started, FinishedAt: started.Add(time.Minute), StoppedReason: "user requested stop",
		Nodes: map[string]*execution.NodeExecution{"check": {
			NodeID: "check", NodeDefinition: "go-static-analysis", NodeExecutor: "v1",
			Current: execution.NodeRun{
				RunID: fixture.nodeRunID, Round: 1, Status: execution.StatusSucceeded,
				Inputs: map[string]execution.InputSnapshot{
					"evidence": {From: "legacy.result", Ref: ref},
				},
				Outputs:   map[string]artifact.ArtifactRef{"result": ref},
				StartedAt: started, FinishedAt: started.Add(30 * time.Second),
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func seedEquivalentTargetDefinitions(t *testing.T, ctx context.Context, paths runtimepath.Paths) {
	t.Helper()
	store, err := history.Open(ctx, paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.ImportDefinitions(ctx,
		[]history.NodeTypeDefRow{{ID: "target-type-id", Name: "automation"}},
		[]history.NodeDefRow{{
			ID: "target-definition-id", Name: "go-static-analysis", Type: "automation",
			Inputs:  map[string]history.InputPort{"code": {Type: "SourceCode"}},
			Outputs: map[string]history.OutputPort{"result": {Type: "QualityCheckResult"}},
		}},
		[]history.NodeExecRow{{
			ID: "target-executor-id", Node: "go-static-analysis", Version: "v1", Name: "Go Static Analysis",
		}},
	); err != nil {
		t.Fatal(err)
	}
}
