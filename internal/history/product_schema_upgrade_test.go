package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// productFixture holds one representative pre-upgrade database: every product
// identity and run the ticket requires to survive the sequential upgrade.
type productFixture struct {
	dbPath     string
	workflowID string
	revisionID string
	runID      string
	nodeRunID  string
	providerID string
	modelUUID  string
	artifactID string
}

// fixture times are fixed so an upgraded Draft, Revision and Run keep the exact
// stored timestamps instead of drifting with the wall clock.
var productFixtureTime = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

// draft bound by the fixture's agent node; the model UUID is materialized.
const fixtureDraftContent = `{"nodes":[{"definition":"human-chat","executor":"v1","id":"prompt"},{"definition":"llm-chat","executor":"v1","id":"answer","inputs":{"conversation":{"from":"prompt.conversation"}},"llm":{"modelUuid":"11111111-2222-4111-8111-111111111112"}}],"semanticSchemaVersion":"productWorkflow/v1"}`

// seedProductFixtureAt applies migrations up to (and including) fromVersion and
// seeds the product data that version's users could hold. Versions at or below
// ProductWorkflowSchemaVersion (5) only have product_workflow; Draft rows exist
// from version 6, LLM settings from 7, Run history from 8.
func seedProductFixtureAt(t testing.TB, fromVersion int) productFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "product.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:fromVersion] {
		if err := applyMigration(context.Background(), tx, migration); err != nil {
			t.Fatal(err)
		}
	}
	fixture := productFixture{
		dbPath:     dbPath,
		workflowID: "11111111-1111-4111-8111-111111111111",
		revisionID: "33333333-3333-4333-8333-333333333333",
		runID:      "44444444-4444-4444-8444-444444444444",
		nodeRunID:  "55555555-5555-4555-8555-555555555555",
		providerID: "66666666-6666-4666-8666-666666666666",
		modelUUID:  "11111111-2222-4111-8111-111111111112",
		artifactID: "77777777-7777-4777-8777-777777777777",
	}
	createdAt := productFixtureTime.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO product_workflow (id, display_name, created_at) VALUES (?, 'Fixture workflow', ?)`, fixture.workflowID, createdAt); err != nil {
		t.Fatal(err)
	}
	if fromVersion >= ProductWorkflowDraftSchemaVersion {
		if _, err := tx.Exec(`INSERT INTO product_workflow_draft (workflow_id, content_json, lock_version, updated_at) VALUES (?, ?, 3, ?)`, fixture.workflowID, fixtureDraftContent, createdAt); err != nil {
			t.Fatal(err)
		}
	}
	if fromVersion >= ProductLLMSettingsSchemaVersion {
		if _, err := tx.Exec(`INSERT INTO product_llm_provider (id, name, protocol, base_url, api_key_ref, is_explicit_default, created_at) VALUES (?, 'Fixture provider', 'openai-chat-completions', 'https://api.example/v1', 'keychain://fixture', 1, ?)`, fixture.providerID, createdAt); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO product_llm_model (id, provider_id, display_name, provider_model_id, is_explicit_default, created_at) VALUES (?, ?, 'Fixture model', 'fixture-model', 1, ?)`, fixture.modelUUID, fixture.providerID, createdAt); err != nil {
			t.Fatal(err)
		}
		// A second, soft-deleted pair proves deletion state survives upgrades:
		// deleted rows stay excluded from settings and default resolution.
		if _, err := tx.Exec(`INSERT INTO product_llm_provider (id, name, protocol, base_url, api_key_ref, created_at, deleted_at) VALUES (?, 'Deleted provider', 'openai-chat-completions', 'https://deleted.example/v1', 'keychain://deleted', ? , ?)`, "88888888-8888-4888-8888-888888888888", createdAt, productFixtureTime.Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO product_llm_model (id, provider_id, display_name, provider_model_id, created_at, deleted_at) VALUES (?, ?, 'Deleted model', 'deleted-model', ?, ?)`, "99999999-9999-4999-8999-999999999999", fixture.providerID, createdAt, productFixtureTime.Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	if fromVersion >= ProductWorkflowRunSchemaVersion {
		revisionContent, semanticHash, err := productworkflow.RevisionContent(json.RawMessage(fixtureDraftContent))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO product_workflow_revision (id, workflow_id, semantic_hash, content_json, created_at) VALUES (?, ?, ?, ?, ?)`, fixture.revisionID, fixture.workflowID, semanticHash, string(revisionContent), createdAt); err != nil {
			t.Fatal(err)
		}
		snapshot := fmt.Sprintf(`{"revisionId":%q,"executors":[{"nodeId":"prompt","definitionId":"human-chat","version":"v1"},{"nodeId":"answer","definitionId":"llm-chat","version":"v1"}],"llmSelection":[%s]}`, fixture.revisionID, fixtureSelectionJSON(fromVersion, fixture))
		if _, err := tx.Exec(`INSERT INTO product_workflow_run (id, workflow_id, revision_id, status, snapshot_json, started_at, finished_at) VALUES (?, ?, ?, 'succeeded', ?, ?, ?)`, fixture.runID, fixture.workflowID, fixture.revisionID, snapshot, createdAt, productFixtureTime.Add(time.Minute).Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO product_workflow_node_run (id, run_id, node_id, node_definition, node_executor, status, inputs_json, outputs_json, started_at, finished_at) VALUES (?, ?, 'prompt', 'human-chat', 'v1', 'succeeded', '{}', '{}', ?, ?)`, fixture.nodeRunID, fixture.runID, createdAt, createdAt); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO product_workflow_artifact (id, run_id, node_run_id, node_id, port, artifact_type, version, uri, created_at) VALUES (?, ?, ?, 'prompt', 'conversation', 'Conversation', '1', '1.json', ?)`, fixture.artifactID, fixture.runID, fixture.nodeRunID, createdAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// fixtureSelectionJSON returns the Run Snapshot LLM selection exactly as the
// given released version's writer persisted it: version 8 has no
// dialect/APIKeyRef, version 9 adds apiKeyRef and version 10 adds dialect.
// Hand-written JSON pins the historical shapes instead of reusing the current
// marshaling, which evolves with the schema.
func fixtureSelectionJSON(fromVersion int, fixture productFixture) string {
	apiKeyRef := ""
	dialect := ""
	if fromVersion >= ProductNodeRunDiagnosticsSchemaVersion {
		apiKeyRef = `"apiKeyRef":"keychain://fixture",`
	}
	if fromVersion >= ProductLLMProviderDialectSchemaVersion {
		dialect = `"dialect":"developer",`
	}
	return fmt.Sprintf(`{"nodeId":"answer","providerId":%q,"providerName":"Fixture provider","protocol":"openai-chat-completions","baseUrl":"https://api.example/v1",%s%s"modelUuid":%q,"providerModelId":"fixture-model","effectiveGeneration":{}}`, fixture.providerID, apiKeyRef, dialect, fixture.modelUUID)
}
