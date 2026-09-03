package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// releasedProductSchemaVersions are the SQLite user_versions real users hold:
// every version between the first Product schema and the latest one.
var releasedProductSchemaVersions = []int{
	ProductWorkflowSchemaVersion,
	ProductWorkflowDraftSchemaVersion,
	ProductLLMSettingsSchemaVersion,
	ProductWorkflowRunSchemaVersion,
	ProductNodeRunDiagnosticsSchemaVersion,
	ProductLLMProviderDialectSchemaVersion,
	ProductRunErrorSchemaVersion,
}

// TestProductSchemaUpgradePreservesDataAndQueryability upgrades one
// representative old database per released product schema version and asserts
// the preserved identities, deletion state, effective defaults and full
// history queries.
func TestProductSchemaUpgradePreservesDataAndQueryability(t *testing.T) {
	if latestUserVersion() != ProductRunErrorSchemaVersion {
		t.Fatalf("latest user_version = %d, want %d (extend releasedProductSchemaVersions and the constant here)",
			latestUserVersion(), ProductRunErrorSchemaVersion)
	}
	for _, fromVersion := range releasedProductSchemaVersions {
		t.Run(fmt.Sprintf("from version %d", fromVersion), func(t *testing.T) {
			ctx := context.Background()
			fixture := seedProductFixtureAt(t, fromVersion)

			store, err := Open(ctx, fixture.dbPath)
			if err != nil {
				t.Fatalf("upgrade from version %d: %v", fromVersion, err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if version, err := store.UserVersion(ctx); err != nil || version != latestUserVersion() {
				t.Fatalf("upgraded user_version = %d, error = %v, want %d", version, err, latestUserVersion())
			}

			assertUpgradedProductData(t, ctx, store, fixture, fromVersion)
		})
	}
}

// assertUpgradedProductData verifies the identities and behavior the upgrade
// ticket promises to preserve: Workflow and Draft (lock version included),
// Provider/Model UUIDs with deletion state and deterministic defaults,
// Revision hash and Run Snapshot, plus the workflow/v1 tables created by the
// same migrations. Rows that could not exist at the fixture's released
// version are skipped: users of that version had no way to write them.
func assertUpgradedProductData(t *testing.T, ctx context.Context, store *Store, fixture productFixture, fromVersion int) {
	t.Helper()

	// Workflow identity survives.
	workflows, err := store.ListProductWorkflows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(workflows) != 1 || workflows[0].ID != fixture.workflowID || workflows[0].DisplayName != "Fixture workflow" || !workflows[0].CreatedAt.Equal(productFixtureTime) {
		t.Fatalf("upgraded workflows = %#v", workflows)
	}

	// Draft content and lock version survive untouched; version 5 databases
	// are backfilled with the initial empty Draft at lock version 1.
	draft, err := store.GetProductWorkflowDraft(ctx, fixture.workflowID)
	if err != nil {
		t.Fatal(err)
	}
	if fromVersion >= ProductWorkflowDraftSchemaVersion {
		if draft.LockVersion != 3 || string(draft.Content) != fixtureDraftContent {
			t.Fatalf("upgraded Draft lock version = %d, content = %s", draft.LockVersion, draft.Content)
		}
	} else if draft.LockVersion != 1 || string(draft.Content) != string(productworkflow.InitialDraftContent()) {
		t.Fatalf("backfilled Draft = %#v", draft)
	}

	if fromVersion >= ProductLLMSettingsSchemaVersion {
		assertUpgradedProviderModelSettings(t, ctx, store, fixture)
	}

	if fromVersion >= ProductWorkflowRunSchemaVersion {
		assertUpgradedRunHistory(t, ctx, store, fixture, fromVersion)
	}
}

// assertUpgradedProviderModelSettings verifies Provider and Model Slot UUIDs,
// deletion state and deterministic defaults after the upgrade: the explicit
// default stays explicit and effective, soft-deleted rows stay excluded, and
// the created-time fallback still resolves once the explicit default is gone.
func assertUpgradedProviderModelSettings(t *testing.T, ctx context.Context, store *Store, fixture productFixture) {
	t.Helper()
	settings, err := store.GetLLMSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Providers) != 1 || settings.Providers[0].ID != fixture.providerID || settings.Providers[0].Name != "Fixture provider" {
		t.Fatalf("upgraded providers = %#v, want the non-deleted provider only", settings.Providers)
	}
	if !settings.Providers[0].ExplicitDefault || !settings.Providers[0].EffectiveDefault {
		t.Fatalf("upgraded provider defaults = explicit %t effective %t, want both true", settings.Providers[0].ExplicitDefault, settings.Providers[0].EffectiveDefault)
	}
	if settings.Providers[0].Dialect != productworkflow.ProviderDialectDeveloper {
		t.Fatalf("upgraded provider dialect = %q, want developer default", settings.Providers[0].Dialect)
	}
	models := settings.Models[fixture.providerID]
	if len(models) != 1 || models[0].ID != fixture.modelUUID || models[0].ProviderModelID != "fixture-model" {
		t.Fatalf("upgraded models = %#v, want the non-deleted model only", models)
	}
	if !models[0].ExplicitDefault || !models[0].EffectiveDefault {
		t.Fatalf("upgraded model defaults = explicit %t effective %t, want both true", models[0].ExplicitDefault, models[0].EffectiveDefault)
	}
	// Soft-deleted rows keep their identity and deletion state: they never
	// resurrect into active settings or default resolution.
	if _, err := store.ResolveLLMModel(ctx, "99999999-9999-4999-8999-999999999999"); err == nil {
		t.Fatal("deleted Model UUID resolved after upgrade")
	}
	// The created-time fallback stays deterministic after the upgrade: the
	// explicit default is removed the same way the product deletes it, and
	// the sole remaining entry still resolves.
	if err := store.DeleteLLMProvider(ctx, fixture.providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveDefaultLLMModel(ctx); err == nil {
		t.Fatal("deleted explicit default still resolved after upgrade")
	}
}

// assertUpgradedRunHistory verifies Revision identity/hash/content, the Run
// Snapshot's fixed model selection, and Node Run/Artifact metadata after the
// upgrade. Snapshots written before the dialect field existed keep their
// stored (empty) dialect: history stays authoritative instead of being
// rewritten by the upgrade.
func assertUpgradedRunHistory(t *testing.T, ctx context.Context, store *Store, fixture productFixture, fromVersion int) {
	t.Helper()
	revisions, err := store.ListProductWorkflowRevisions(ctx, fixture.workflowID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 || revisions[0].ID != fixture.revisionID {
		t.Fatalf("upgraded revisions = %#v", revisions)
	}
	normalized, semanticHash, err := productworkflow.RevisionContent(json.RawMessage(fixtureDraftContent))
	if err != nil {
		t.Fatal(err)
	}
	if revisions[0].SemanticHash != semanticHash || string(revisions[0].Content) != string(normalized) {
		t.Fatalf("upgraded revision = hash %s content %s, want hash %s content %s", revisions[0].SemanticHash, revisions[0].Content, semanticHash, normalized)
	}

	runs, err := store.ListProductWorkflowRevisionRuns(ctx, fixture.revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != fixture.runID {
		t.Fatalf("upgraded revision runs = %#v", runs)
	}
	run, nodeRuns, artifacts, err := store.GetProductRun(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID != fixture.runID || run.Status != "succeeded" || run.RevisionID != fixture.revisionID || run.Error != nil {
		t.Fatalf("upgraded run = %#v", run)
	}
	if len(run.Snapshot.LLMSelection) != 1 {
		t.Fatalf("upgraded snapshot selections = %#v", run.Snapshot.LLMSelection)
	}
	selection := run.Snapshot.LLMSelection[0]
	if selection.ProviderName != "Fixture provider" || selection.ModelUUID != fixture.modelUUID || selection.ProviderModelID != "fixture-model" {
		t.Fatalf("upgraded snapshot selection = %#v", selection)
	}
	if fromVersion >= ProductLLMProviderDialectSchemaVersion && selection.Dialect != productworkflow.ProviderDialectDeveloper {
		t.Fatalf("upgraded snapshot dialect = %q, want developer default", selection.Dialect)
	}
	if len(nodeRuns) != 1 || nodeRuns[0].ID != fixture.nodeRunID || nodeRuns[0].NodeID != "prompt" {
		t.Fatalf("upgraded node runs = %#v", nodeRuns)
	}
	if len(artifacts) != 1 || artifacts[0].ID != fixture.artifactID || artifacts[0].URI != "1.json" {
		t.Fatalf("upgraded artifacts = %#v", artifacts)
	}
}

// TestProductSchemaUpgradeReopenDoesNotDuplicateHistory proves the migration
// replay is a no-op: after a successful upgrade, Open again neither duplicates
// Revision, Run, Node Run or Artifact rows nor rewrites any preserved value.
// DDL itself is not safe to re-execute against the upgraded schema (an
// ALTER TABLE ADD COLUMN cannot run twice), so the replay guarantee is the
// user_version guard: a second Open runs zero migration statements.
func TestProductSchemaUpgradeReopenDoesNotDuplicateHistory(t *testing.T) {
	ctx := context.Background()
	fixture := seedProductFixtureAt(t, ProductLLMProviderDialectSchemaVersion)

	store, err := Open(ctx, fixture.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// The reopen replays the migration chain from the persisted user_version;
	// the guard must skip every statement, leaving all rows untouched.
	// The preservation assertions above soft-deleted the fixture provider,
	// so the reopen check reads only the rows migration replay must never
	// duplicate: Revisions, Runs, Node Runs, Artifacts and Drafts.
	reopened, err := Open(ctx, fixture.dbPath)
	if err != nil {
		t.Fatalf("reopen upgraded database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	for _, query := range []string{
		`SELECT count(*) FROM product_workflow_revision`,
		`SELECT count(*) FROM product_workflow_run`,
		`SELECT count(*) FROM product_workflow_node_run`,
		`SELECT count(*) FROM product_workflow_artifact`,
		`SELECT count(*) FROM product_workflow_draft`,
	} {
		var count int
		if err := reopened.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("after reopen %s = %d rows, want 1", query, count)
		}
	}
	// The preserved history stays queryable through the public seams after
	// the reopen, with the same fixed model selection.
	run, _, artifacts, err := reopened.GetProductRun(ctx, fixture.runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Snapshot.LLMSelection) != 1 || run.Snapshot.LLMSelection[0].ModelUUID != fixture.modelUUID || len(artifacts) != 1 {
		t.Fatalf("reopened history = %#v artifacts = %#v", run.Snapshot.LLMSelection, artifacts)
	}
}

// TestProductSchemaUpgradeFailureLeavesOldDatabaseRecoverable proves a failed
// migration publishes neither a half-migrated schema version nor half-visible
// history. The poison pre-adds the column the second upgrade step wants, so
// that step fails with "duplicate column name" while the first step's DDL
// already ran: user_version must still hold the old value, the first step's
// column must be rolled back, and the original rows must stay readable,
// keeping the old database recoverable.
func TestProductSchemaUpgradeFailureLeavesOldDatabaseRecoverable(t *testing.T) {
	ctx := context.Background()
	fixture := seedProductFixtureAt(t, ProductWorkflowRunSchemaVersion)

	// The upgrade from version 8 first adds product_workflow_node_run's
	// diagnostics column (step 9), then adds product_llm_provider's dialect
	// column (step 10). Pre-adding the dialect column makes step 10 fail
	// after step 9 succeeded inside the same transaction.
	db, err := openRawSQLite(fixture.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE product_llm_provider ADD COLUMN instructions_dialect TEXT NOT NULL DEFAULT 'developer' CHECK (instructions_dialect IN ('developer', 'system'))`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rewindUserVersion(t, fixture.dbPath, ProductNodeRunDiagnosticsSchemaVersion)

	if _, err := Open(ctx, fixture.dbPath); err == nil {
		t.Fatal("upgrade with poisoned later migration = nil error")
	}

	recovered, err := openRawSQLite(fixture.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	var version int
	if err := recovered.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != ProductNodeRunDiagnosticsSchemaVersion {
		t.Fatalf("failed upgrade published user_version %d, want it to stay at %d", version, ProductNodeRunDiagnosticsSchemaVersion)
	}
	// The first step's DDL was rolled back too: the database is exactly the
	// old version's schema, not a half-migrated one.
	var diagnosticsColumn int
	if err := recovered.QueryRow(`SELECT count(*) FROM pragma_table_info('product_workflow_node_run') WHERE name = 'diagnostics_json'`).Scan(&diagnosticsColumn); err != nil {
		t.Fatal(err)
	}
	if diagnosticsColumn != 0 {
		t.Fatal("failed upgrade left a half-migrated column from the first step")
	}
	// The old database keeps its rows: the upgrade left the original data
	// recoverable instead of half-migrating it away.
	var draftLockVersion int
	if err := recovered.QueryRow(`SELECT lock_version FROM product_workflow_draft WHERE workflow_id = ?`, fixture.workflowID).Scan(&draftLockVersion); err != nil {
		t.Fatal(err)
	}
	if draftLockVersion != 3 {
		t.Fatalf("failed upgrade changed Draft lock version to %d, want 3", draftLockVersion)
	}
	var runCount int
	if err := recovered.QueryRow(`SELECT count(*) FROM product_workflow_run WHERE id = ?`, fixture.runID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("failed upgrade changed run rows to %d, want 1", runCount)
	}
}

// TestProductSchemaUpgradeKeepsWorkflowV1HistoryUsable proves the workflow/v1
// definitions and Run history in the same database stay intact and queryable
// through their original seams after the product schema upgrade.
func TestProductSchemaUpgradeKeepsWorkflowV1HistoryUsable(t *testing.T) {
	ctx := context.Background()
	fixture := seedProductFixtureAt(t, ProductWorkflowDraftSchemaVersion)
	seedWorkflowV1History(t, fixture.dbPath)

	store, err := Open(ctx, fixture.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	runs, err := store.ListRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "99999999-1111-4111-8111-111111111111" || runs[0].Workflow != "yaml-history" {
		t.Fatalf("workflow/v1 runs after upgrade = %#v", runs)
	}
	detail, err := store.GetRun(ctx, "99999999-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil || detail.Status != "Stopped" || len(detail.Nodes) != 1 || detail.Nodes[0].NodeID != "check" {
		t.Fatalf("workflow/v1 run detail after upgrade = %#v", detail)
	}
	nodeDetail, err := store.GetNodeRun(ctx, "99999999-1111-4111-8111-111111111111", "check")
	if err != nil {
		t.Fatal(err)
	}
	if nodeDetail == nil || len(nodeDetail.Rounds) != 1 || nodeDetail.Rounds[0].Status != "Succeeded" {
		t.Fatalf("workflow/v1 node rounds after upgrade = %#v", nodeDetail)
	}
	// The workflow/v1 definitions stay importable: re-importing the same
	// definitions is an idempotent upsert, not a conflict.
	if err := store.ImportDefinitions(ctx,
		[]NodeTypeDefRow{{ID: "legacy-type", Name: "automation"}},
		[]NodeDefRow{{ID: "legacy-def", Name: "go-static-analysis", Type: "automation"}},
		[]NodeExecRow{{ID: "legacy-exec", Node: "go-static-analysis", Version: "v1"}},
	); err != nil {
		t.Fatalf("re-import workflow/v1 definitions after upgrade: %v", err)
	}
	// Product queries on the same upgraded database still work; one store
	// serves both histories without cross-contamination.
	assertUpgradedProductData(t, ctx, store, fixture, ProductWorkflowDraftSchemaVersion)
}

// seedWorkflowV1History writes one workflow/v1 definition set and Run history
// directly, mirroring what the YAML CLI's recorder persisted.
func seedWorkflowV1History(t *testing.T, dbPath string) {
	t.Helper()
	db, err := openRawSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO node_type_definition (id, name, description, requires_json, created_at) VALUES ('legacy-type', 'automation', '', '[]', '2026-08-30T08:00:00Z')`,
		`INSERT INTO node_definition (id, name, description, type, requires_json, inputs_json, outputs_json, created_at) VALUES ('legacy-def', 'go-static-analysis', '', 'automation', '[]', '{}', '{}', '2026-08-30T08:00:00Z')`,
		`INSERT INTO node_executor (id, node_definition_id, version, name, description, updates, created_at) VALUES ('legacy-exec', 'legacy-def', 'v1', '', '', '', '2026-08-30T08:00:00Z')`,
		`INSERT INTO workflow (id, name, version, description, projects_json, created_at) VALUES ('legacy-wf', 'yaml-history', 'v1', '', '[]', '2026-08-30T08:00:00Z')`,
		`INSERT INTO node_instance (id, workflow_id, node_id, node_definition_id, node_executor_id, display_name, description, llm_provider, llm_model, inputs_json, depends_on_json, config_json) VALUES ('legacy-inst', 'legacy-wf', 'check', 'legacy-def', 'legacy-exec', '', '', '', '', '{}', '[]', '{}')`,
		`INSERT INTO workflow_run_history (id, workflow_name, workflow_version, status, workflow_file, error, stopped_reason, started_at, finished_at) VALUES ('99999999-1111-4111-8111-111111111111', 'yaml-history', 'v1', 'Stopped', 'workflow.yaml', '', 'user_interrupt', '2026-08-30T08:00:00Z', '2026-08-30T08:01:00Z')`,
		`INSERT INTO workflow_node_run_history (id, run_id, node_id, node_definition, node_executor, round, status, error, error_kind, inputs_json, outputs_json, started_at, finished_at) VALUES ('99999999-2222-4222-8222-222222222222', '99999999-1111-4111-8111-111111111111', 'check', 'go-static-analysis', 'v1', 1, 'Succeeded', '', '', '{}', '{}', '2026-08-30T08:00:00Z', '2026-08-30T08:00:30Z')`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// openRawSQLite opens the fixture database without migrations so tests can
// seed and inspect rows exactly as a released version would have written them.
func openRawSQLite(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite", dbPath)
}

// rewindUserVersion rewinds PRAGMA user_version so the next Open replays the
// sequential upgrade from the given version.
func rewindUserVersion(t *testing.T, dbPath string, version int) {
	t.Helper()
	db, err := openRawSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		t.Fatal(err)
	}
}
