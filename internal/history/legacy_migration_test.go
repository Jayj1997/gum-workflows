package history_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
	legacyDatabaseBefore, err := os.ReadFile(source.Database())
	if err != nil {
		t.Fatal(err)
	}
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
	if run == nil || run.ID != fixture.runID || run.ExecutionID != fixture.runID || run.Workflow != "legacy-quality" {
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

	migratedArtifacts, err := artifact.NewFilesystemStore(destination.ArtifactsDir(fixture.runID))
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
	assertDatabaseID(t, destination.Database(), "node_executor", "version", "v1", "target-executor-id")
	assertDatabaseID(t, destination.Database(), "workflow", "name", "legacy-quality", "legacy-workflow-id")
	assertDatabaseID(t, destination.Database(), "node_instance", "node_id", "check", "legacy-instance-id")
	assertEquivalentHistory(t, ctx, source, destination, fixture)
	legacyDatabaseAfter, err := os.ReadFile(source.Database())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacyDatabaseBefore, legacyDatabaseAfter) {
		t.Fatal("migration modified the legacy database")
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
	artifacts, err := artifact.NewFilesystemStore(destination.ArtifactsDir(fixture.runID))
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
	if _, err := os.Stat(filepath.Join(destination.ArtifactsDir(fixture.runID), fixture.ref.URI)); !os.IsNotExist(err) {
		t.Fatalf("conflict published an Artifact: %v", err)
	}
}

func TestMigrateLegacyUsesRunIdentityAcrossProjects(t *testing.T) {
	ctx := context.Background()
	firstSource, destination := migrationPaths(t)
	first := seedLegacyFixture(t, ctx, firstSource)
	if err := history.MigrateLegacy(ctx, firstSource, destination); err != nil {
		t.Fatal(err)
	}

	secondRoot := t.TempDir()
	secondSource, err := runtimepath.New(
		filepath.Join(secondRoot, "other-project", ".workflow", "gum-workflows.db"),
		filepath.Join(secondRoot, "other-project", ".workflow", "executions"),
	)
	if err != nil {
		t.Fatal(err)
	}
	second := seedLegacyFixtureVariant(t, secondSource,
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"other project failed",
	)
	if first.executionID != second.executionID {
		t.Fatal("fixture must reproduce per-project legacy execution ID collision")
	}
	if err := history.MigrateLegacy(ctx, secondSource, destination); err != nil {
		t.Fatalf("migrate second Project with colliding legacy execution ID: %v", err)
	}

	for _, fixture := range []legacyFixture{first, second} {
		store, err := artifact.NewFilesystemStore(destination.ArtifactsDir(fixture.runID))
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.Get(fixture.ref)
		if err != nil {
			t.Fatal(err)
		}
		if got.Data != fixture.data {
			t.Fatalf("Run %s Artifact data = %v, want %q", fixture.runID, got.Data, fixture.data)
		}
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
	data        string
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
	_ = ctx
	schema, err := os.ReadFile(filepath.Join("testdata", "legacy_workflow.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Database()), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	executionID := "execution-000007"
	body, err := os.ReadFile(filepath.Join("testdata", "legacy_artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ArtifactsDir(executionID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.ArtifactsDir(executionID), "1.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := legacyFixture{
		runID: "11111111-1111-4111-8111-111111111111", nodeRunID: "22222222-2222-4222-8222-222222222222",
		executionID: executionID,
		ref:         artifact.ArtifactRef{ID: "quality-result", Kind: "QualityCheckResult", Version: "1", URI: "1.json"},
		data:        "legacy passed",
	}
	return fixture
}

func seedLegacyFixtureVariant(t *testing.T, paths runtimepath.Paths, runID, nodeRunID, data string) legacyFixture {
	t.Helper()
	fixture := seedLegacyFixture(t, context.Background(), paths)
	db, err := sql.Open("sqlite", paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE workflow_node_run_history SET id=?, run_id=?`, nodeRunID, runID); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE workflow_run_history SET id=?`, runID); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	bodyPath := filepath.Join(paths.ArtifactsDir(fixture.executionID), fixture.ref.URI)
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	value["Data"] = data
	body, err = json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bodyPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.runID, fixture.nodeRunID, fixture.data = runID, nodeRunID, data
	return fixture
}

func assertEquivalentHistory(t *testing.T, ctx context.Context, source, destination runtimepath.Paths, fixture legacyFixture) {
	t.Helper()
	legacy, err := history.OpenReadOnly(ctx, source.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	migrated, err := history.OpenReadOnly(ctx, destination.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()

	legacyList, err := legacy.ListRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	migratedList, err := migrated.ListRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacyList, migratedList) {
		t.Fatalf("history list changed:\nlegacy: %+v\nmigrated: %+v", legacyList, migratedList)
	}
	legacyRun, err := legacy.GetRun(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	migratedRun, err := migrated.GetRun(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	legacyRun.ExecutionID = migratedRun.ExecutionID // Local Data Root uses the stable Run identity.
	if !reflect.DeepEqual(legacyRun, migratedRun) {
		t.Fatalf("Run history changed:\nlegacy: %+v\nmigrated: %+v", legacyRun, migratedRun)
	}
	legacyNode, err := legacy.GetNodeRun(ctx, fixture.runID, "check")
	if err != nil {
		t.Fatal(err)
	}
	migratedNode, err := migrated.GetNodeRun(ctx, fixture.runID, "check")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacyNode, migratedNode) {
		t.Fatalf("Node Run history changed:\nlegacy: %+v\nmigrated: %+v", legacyNode, migratedNode)
	}
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
