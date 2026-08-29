package history

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
)

// MigrateLegacy copies one explicitly selected platform-core history store into
// the Local Data Root. Existing equivalent identities are reused, conflicts
// fail without publishing database rows, and the legacy store remains read-only.
func MigrateLegacy(ctx context.Context, source, destination runtimepath.Paths) error {
	if source.Database() == destination.Database() || source.ExecutionsDir() == destination.ExecutionsDir() {
		return fmt.Errorf("migrate legacy history: source and destination must be distinct")
	}
	sourceStore, err := OpenReadOnly(ctx, source.Database())
	if err != nil {
		return fmt.Errorf("migrate legacy history: open source: %w", err)
	}
	defer sourceStore.Close()
	version, err := sourceStore.UserVersion(ctx)
	if err != nil {
		return fmt.Errorf("migrate legacy history: inspect source schema: %w", err)
	}
	if version < RunHistorySchemaVersion {
		return fmt.Errorf("migrate legacy history: source schema version %d has no run history", version)
	}
	snapshot, err := loadLegacySnapshot(ctx, sourceStore.db)
	if err != nil {
		return fmt.Errorf("migrate legacy history: read source: %w", err)
	}

	target, err := Open(ctx, destination.Database())
	if err != nil {
		return fmt.Errorf("migrate legacy history: open destination: %w", err)
	}
	defer target.Close()
	tx, err := beginLegacyImport(ctx, target.db, snapshot)
	if err != nil {
		return fmt.Errorf("migrate legacy history: prepare destination: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	staged, cleanup, err := stageLegacyArtifacts(source, destination, snapshot)
	if err != nil {
		return fmt.Errorf("migrate legacy history: stage artifacts: %w", err)
	}
	defer cleanup()
	rollbackArtifacts, err := publishLegacyArtifacts(staged)
	if err != nil {
		return fmt.Errorf("migrate legacy history: publish artifacts: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(fmt.Errorf("migrate legacy history: commit: %w", err), rollbackArtifacts())
	}
	return nil
}

type legacySnapshot struct {
	nodeTypes     []legacyNodeType
	nodeDefs      []legacyNodeDef
	nodeExecutors []legacyNodeExecutor
	workflows     []legacyWorkflow
	instances     []legacyNodeInstance
	runs          []legacyRun
	nodeRuns      []legacyNodeRun
}

type legacyNodeType struct{ id, name, description, requires, createdAt string }
type legacyNodeDef struct{ id, name, description, nodeType, requires, inputs, outputs, createdAt string }
type legacyNodeExecutor struct{ id, definitionID, version, name, description, updates, createdAt string }
type legacyWorkflow struct{ id, name, version, description, projects, createdAt string }
type legacyNodeInstance struct {
	id, workflowID, nodeID, definitionID, executorID string
	displayName, description, provider, model        string
	inputs, dependsOn, config                        string
}
type legacyRun struct {
	id, workflowName, workflowVersion, status, workflowFile string
	executionID, runError, stoppedReason, startedAt         string
	finishedAt                                              sql.NullString
}
type legacyNodeRun struct {
	id, runID, nodeID, definition, executor, status string
	round                                           int
	runError, errorKind, inputs, outputs            string
	startedAt, finishedAt                           sql.NullString
}

func loadLegacySnapshot(ctx context.Context, db *sql.DB) (legacySnapshot, error) {
	var snapshot legacySnapshot
	loads := []func() error{
		func() error {
			return loadRows(ctx, db, `SELECT id,name,description,requires_json,created_at FROM node_type_definition`, func(rows *sql.Rows) error {
				var row legacyNodeType
				if err := rows.Scan(&row.id, &row.name, &row.description, &row.requires, &row.createdAt); err != nil {
					return err
				}
				snapshot.nodeTypes = append(snapshot.nodeTypes, row)
				return nil
			})
		},
		func() error {
			return loadRows(ctx, db, `SELECT id,name,description,type,requires_json,inputs_json,outputs_json,created_at FROM node_definition`, func(rows *sql.Rows) error {
				var row legacyNodeDef
				if err := rows.Scan(&row.id, &row.name, &row.description, &row.nodeType, &row.requires, &row.inputs, &row.outputs, &row.createdAt); err != nil {
					return err
				}
				snapshot.nodeDefs = append(snapshot.nodeDefs, row)
				return nil
			})
		},
		func() error {
			return loadRows(ctx, db, `SELECT id,node_definition_id,version,name,description,updates,created_at FROM node_executor`, func(rows *sql.Rows) error {
				var row legacyNodeExecutor
				if err := rows.Scan(&row.id, &row.definitionID, &row.version, &row.name, &row.description, &row.updates, &row.createdAt); err != nil {
					return err
				}
				snapshot.nodeExecutors = append(snapshot.nodeExecutors, row)
				return nil
			})
		},
		func() error {
			return loadRows(ctx, db, `SELECT id,name,version,description,projects_json,created_at FROM workflow`, func(rows *sql.Rows) error {
				var row legacyWorkflow
				if err := rows.Scan(&row.id, &row.name, &row.version, &row.description, &row.projects, &row.createdAt); err != nil {
					return err
				}
				snapshot.workflows = append(snapshot.workflows, row)
				return nil
			})
		},
		func() error {
			return loadRows(ctx, db, `SELECT id,workflow_id,node_id,node_definition_id,node_executor_id,display_name,description,llm_provider,llm_model,inputs_json,depends_on_json,config_json FROM node_instance`, func(rows *sql.Rows) error {
				var row legacyNodeInstance
				if err := rows.Scan(&row.id, &row.workflowID, &row.nodeID, &row.definitionID, &row.executorID, &row.displayName, &row.description, &row.provider, &row.model, &row.inputs, &row.dependsOn, &row.config); err != nil {
					return err
				}
				snapshot.instances = append(snapshot.instances, row)
				return nil
			})
		},
		func() error {
			return loadRows(ctx, db, `SELECT id,workflow_name,workflow_version,status,workflow_file,execution_id,error,stopped_reason,started_at,finished_at FROM workflow_run_history`, func(rows *sql.Rows) error {
				var row legacyRun
				if err := rows.Scan(&row.id, &row.workflowName, &row.workflowVersion, &row.status, &row.workflowFile, &row.executionID, &row.runError, &row.stoppedReason, &row.startedAt, &row.finishedAt); err != nil {
					return err
				}
				snapshot.runs = append(snapshot.runs, row)
				return nil
			})
		},
		func() error {
			return loadRows(ctx, db, `SELECT id,run_id,node_id,node_definition,node_executor,round,status,error,error_kind,inputs_json,outputs_json,started_at,finished_at FROM workflow_node_run_history`, func(rows *sql.Rows) error {
				var row legacyNodeRun
				if err := rows.Scan(&row.id, &row.runID, &row.nodeID, &row.definition, &row.executor, &row.round, &row.status, &row.runError, &row.errorKind, &row.inputs, &row.outputs, &row.startedAt, &row.finishedAt); err != nil {
					return err
				}
				snapshot.nodeRuns = append(snapshot.nodeRuns, row)
				return nil
			})
		},
	}
	for _, load := range loads {
		if err := load(); err != nil {
			return legacySnapshot{}, err
		}
	}
	return snapshot, nil
}

func loadRows(ctx context.Context, db *sql.DB, query string, scan func(*sql.Rows) error) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func beginLegacyImport(ctx context.Context, db *sql.DB, snapshot legacySnapshot) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	if err := applyLegacySnapshot(ctx, tx, snapshot); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func applyLegacySnapshot(ctx context.Context, tx *sql.Tx, snapshot legacySnapshot) error {
	typeIDs, defIDs, executorIDs, workflowIDs := map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	for _, row := range snapshot.nodeTypes {
		id, err := ensureNodeType(ctx, tx, row)
		if err != nil {
			return err
		}
		typeIDs[row.id] = id
	}
	for _, row := range snapshot.nodeDefs {
		if _, ok := findTypeID(snapshot.nodeTypes, typeIDs, row.nodeType); !ok {
			return fmt.Errorf("node definition %q references unknown type %q", row.name, row.nodeType)
		}
		id, err := ensureNodeDefinition(ctx, tx, row)
		if err != nil {
			return err
		}
		defIDs[row.id] = id
	}
	for _, row := range snapshot.nodeExecutors {
		mapped, ok := defIDs[row.definitionID]
		if !ok {
			return fmt.Errorf("node executor %q references unknown definition %q", row.id, row.definitionID)
		}
		id, err := ensureNodeExecutor(ctx, tx, row, mapped)
		if err != nil {
			return err
		}
		executorIDs[row.id] = id
	}
	for _, row := range snapshot.workflows {
		id, err := ensureWorkflow(ctx, tx, row)
		if err != nil {
			return err
		}
		workflowIDs[row.id] = id
	}
	for _, row := range snapshot.instances {
		workflowID, wok := workflowIDs[row.workflowID]
		definitionID, dok := defIDs[row.definitionID]
		executorID, eok := executorIDs[row.executorID]
		if !wok || !dok || !eok {
			return fmt.Errorf("node instance %q has unresolved identities", row.nodeID)
		}
		if err := ensureNodeInstance(ctx, tx, row, workflowID, definitionID, executorID); err != nil {
			return err
		}
	}
	for _, row := range snapshot.runs {
		if err := ensureRun(ctx, tx, row); err != nil {
			return err
		}
	}
	for _, row := range snapshot.nodeRuns {
		if err := ensureNodeRun(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}

func findTypeID(rows []legacyNodeType, ids map[string]string, name string) (string, bool) {
	for _, row := range rows {
		if row.name == name {
			id, ok := ids[row.id]
			return id, ok
		}
	}
	return "", false
}

func ensureNodeType(ctx context.Context, tx *sql.Tx, row legacyNodeType) (string, error) {
	var id, description, requires string
	err := tx.QueryRowContext(ctx, `SELECT id,description,requires_json FROM node_type_definition WHERE name=?`, row.name).Scan(&id, &description, &requires)
	if err == nil {
		if description != row.description || !jsonEqual(requires, row.requires) {
			return "", conflict("node type", row.name)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO node_type_definition(id,name,description,requires_json,created_at) VALUES(?,?,?,?,?)`, row.id, row.name, row.description, row.requires, row.createdAt)
	if err != nil {
		return "", conflictCause("node type", row.name, err)
	}
	return row.id, nil
}

func ensureNodeDefinition(ctx context.Context, tx *sql.Tx, row legacyNodeDef) (string, error) {
	var id, description, nodeType, requires, inputs, outputs string
	err := tx.QueryRowContext(ctx, `SELECT id,description,type,requires_json,inputs_json,outputs_json FROM node_definition WHERE name=?`, row.name).Scan(&id, &description, &nodeType, &requires, &inputs, &outputs)
	if err == nil {
		if description != row.description || nodeType != row.nodeType || !jsonEqual(requires, row.requires) || !jsonEqual(inputs, row.inputs) || !jsonEqual(outputs, row.outputs) {
			return "", conflict("node definition", row.name)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO node_definition(id,name,description,type,requires_json,inputs_json,outputs_json,created_at) VALUES(?,?,?,?,?,?,?,?)`, row.id, row.name, row.description, row.nodeType, row.requires, row.inputs, row.outputs, row.createdAt)
	if err != nil {
		return "", conflictCause("node definition", row.name, err)
	}
	return row.id, nil
}

func ensureNodeExecutor(ctx context.Context, tx *sql.Tx, row legacyNodeExecutor, definitionID string) (string, error) {
	var id, name, description, updates string
	err := tx.QueryRowContext(ctx, `SELECT id,name,description,updates FROM node_executor WHERE node_definition_id=? AND version=?`, definitionID, row.version).Scan(&id, &name, &description, &updates)
	identity := definitionID + "@" + row.version
	if err == nil {
		if name != row.name || description != row.description || updates != row.updates {
			return "", conflict("node executor", identity)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO node_executor(id,node_definition_id,version,name,description,updates,created_at) VALUES(?,?,?,?,?,?,?)`, row.id, definitionID, row.version, row.name, row.description, row.updates, row.createdAt)
	if err != nil {
		return "", conflictCause("node executor", identity, err)
	}
	return row.id, nil
}

func ensureWorkflow(ctx context.Context, tx *sql.Tx, row legacyWorkflow) (string, error) {
	var id, description, projects string
	err := tx.QueryRowContext(ctx, `SELECT id,description,projects_json FROM workflow WHERE name=? AND version=?`, row.name, row.version).Scan(&id, &description, &projects)
	identity := row.name + "@" + row.version
	if err == nil {
		if description != row.description || !jsonEqual(projects, row.projects) {
			return "", conflict("workflow", identity)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow(id,name,version,description,projects_json,created_at) VALUES(?,?,?,?,?,?)`, row.id, row.name, row.version, row.description, row.projects, row.createdAt)
	if err != nil {
		return "", conflictCause("workflow", identity, err)
	}
	return row.id, nil
}

func ensureNodeInstance(ctx context.Context, tx *sql.Tx, row legacyNodeInstance, workflowID, definitionID, executorID string) error {
	var id, gotDefinition, gotExecutor, displayName, description, provider, model, inputs, dependsOn, config string
	err := tx.QueryRowContext(ctx, `SELECT id,node_definition_id,node_executor_id,display_name,description,llm_provider,llm_model,inputs_json,depends_on_json,config_json FROM node_instance WHERE workflow_id=? AND node_id=?`, workflowID, row.nodeID).Scan(&id, &gotDefinition, &gotExecutor, &displayName, &description, &provider, &model, &inputs, &dependsOn, &config)
	if err == nil {
		if gotDefinition != definitionID || gotExecutor != executorID || displayName != row.displayName || description != row.description || provider != row.provider || model != row.model || !jsonEqual(inputs, row.inputs) || !jsonEqual(dependsOn, row.dependsOn) || !jsonEqual(config, row.config) {
			return conflict("node instance", row.nodeID)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO node_instance(id,workflow_id,node_id,node_definition_id,node_executor_id,display_name,description,llm_provider,llm_model,inputs_json,depends_on_json,config_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, row.id, workflowID, row.nodeID, definitionID, executorID, row.displayName, row.description, row.provider, row.model, row.inputs, row.dependsOn, row.config)
	if err != nil {
		return conflictCause("node instance", row.nodeID, err)
	}
	return nil
}

func ensureRun(ctx context.Context, tx *sql.Tx, row legacyRun) error {
	row.executionID = row.id
	var got legacyRun
	err := tx.QueryRowContext(ctx, `SELECT id,workflow_name,workflow_version,status,workflow_file,execution_id,error,stopped_reason,started_at,finished_at FROM workflow_run_history WHERE id=?`, row.id).Scan(&got.id, &got.workflowName, &got.workflowVersion, &got.status, &got.workflowFile, &got.executionID, &got.runError, &got.stoppedReason, &got.startedAt, &got.finishedAt)
	if err == nil {
		if got != row {
			return conflict("workflow run", row.id)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_run_history(id,workflow_name,workflow_version,status,workflow_file,execution_id,error,stopped_reason,started_at,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, row.id, row.workflowName, row.workflowVersion, row.status, row.workflowFile, row.executionID, row.runError, row.stoppedReason, row.startedAt, row.finishedAt)
	if err != nil {
		return conflictCause("workflow run", row.id, err)
	}
	return nil
}

func ensureNodeRun(ctx context.Context, tx *sql.Tx, row legacyNodeRun) error {
	var got legacyNodeRun
	err := tx.QueryRowContext(ctx, `SELECT id,run_id,node_id,node_definition,node_executor,round,status,error,error_kind,inputs_json,outputs_json,started_at,finished_at FROM workflow_node_run_history WHERE id=?`, row.id).Scan(&got.id, &got.runID, &got.nodeID, &got.definition, &got.executor, &got.round, &got.status, &got.runError, &got.errorKind, &got.inputs, &got.outputs, &got.startedAt, &got.finishedAt)
	if err == nil {
		if got.id != row.id || got.runID != row.runID || got.nodeID != row.nodeID || got.definition != row.definition || got.executor != row.executor || got.round != row.round || got.status != row.status || got.runError != row.runError || got.errorKind != row.errorKind || !jsonEqual(got.inputs, row.inputs) || !jsonEqual(got.outputs, row.outputs) || got.startedAt != row.startedAt || got.finishedAt != row.finishedAt {
			return conflict("node run", row.id)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_node_run_history(id,run_id,node_id,node_definition,node_executor,round,status,error,error_kind,inputs_json,outputs_json,started_at,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, row.id, row.runID, row.nodeID, row.definition, row.executor, row.round, row.status, row.runError, row.errorKind, row.inputs, row.outputs, row.startedAt, row.finishedAt)
	if err != nil {
		return conflictCause("node run", row.id, err)
	}
	return nil
}

func conflict(kind, identity string) error {
	return fmt.Errorf("%s %q conflicts with destination", kind, identity)
}
func conflictCause(kind, identity string, cause error) error {
	return fmt.Errorf("%s %q conflicts with destination: %w", kind, identity, cause)
}

func jsonEqual(left, right string) bool {
	var a, b any
	return json.Unmarshal([]byte(left), &a) == nil && json.Unmarshal([]byte(right), &b) == nil && reflect.DeepEqual(a, b)
}

type stagedArtifact struct{ sourcePath, stagedPath, destinationPath string }

type legacyExecutionPath struct {
	source, destination string
}

type artifactGroup struct {
	stagedRun, destinationRun string
	items                     []stagedArtifact
	exists                    bool
}

var legacyArtifactName = regexp.MustCompile(`^[0-9]+\.json$`)

func stageLegacyArtifacts(source, destination runtimepath.Paths, snapshot legacySnapshot) ([]stagedArtifact, func(), error) {
	if err := os.MkdirAll(filepath.Dir(destination.ExecutionsDir()), 0o755); err != nil {
		return nil, func() {}, err
	}
	stageRoot, err := os.MkdirTemp(filepath.Dir(destination.ExecutionsDir()), ".legacy-migration-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(stageRoot) }
	runExecutions := make(map[string]legacyExecutionPath, len(snapshot.runs))
	for _, run := range snapshot.runs {
		runExecutions[run.id] = legacyExecutionPath{source: run.executionID, destination: run.id}
	}
	paths := map[string]stagedArtifact{}
	for _, nodeRun := range snapshot.nodeRuns {
		executionPath, ok := runExecutions[nodeRun.runID]
		if !ok {
			cleanup()
			return nil, func() {}, fmt.Errorf("node run %q references unknown run %q", nodeRun.id, nodeRun.runID)
		}
		refs, err := artifactRefs(nodeRun.inputs, nodeRun.outputs)
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("node run %q artifact refs: %w", nodeRun.id, err)
		}
		for _, ref := range refs {
			if !legacyArtifactName.MatchString(ref.URI) {
				continue
			}
			sourcePath := filepath.Join(source.ArtifactsDir(executionPath.source), ref.URI)
			destinationPath := filepath.Join(destination.ArtifactsDir(executionPath.destination), ref.URI)
			if existing, ok := paths[destinationPath]; ok {
				if existing.sourcePath != sourcePath {
					cleanup()
					return nil, func() {}, conflict("artifact", destinationPath)
				}
				continue
			}
			if err := stageFile(sourcePath, filepath.Join(stageRoot, executionPath.destination, "artifacts", ref.URI)); err != nil {
				cleanup()
				return nil, func() {}, err
			}
			paths[destinationPath] = stagedArtifact{
				sourcePath: sourcePath, stagedPath: filepath.Join(stageRoot, executionPath.destination, "artifacts", ref.URI),
				destinationPath: destinationPath,
			}
		}
	}
	result := make([]stagedArtifact, 0, len(paths))
	for _, item := range paths {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].destinationPath < result[j].destinationPath })
	return result, cleanup, nil
}

func artifactRefs(inputsJSON, outputsJSON string) ([]artifact.ArtifactRef, error) {
	var inputs map[string]struct {
		Ref artifact.ArtifactRef `json:"ref"`
	}
	if err := json.Unmarshal([]byte(inputsJSON), &inputs); err != nil {
		return nil, err
	}
	var outputs map[string]artifact.ArtifactRef
	if err := json.Unmarshal([]byte(outputsJSON), &outputs); err != nil {
		return nil, err
	}
	refs := make([]artifact.ArtifactRef, 0, len(inputs)+len(outputs))
	for _, input := range inputs {
		refs = append(refs, input.Ref)
	}
	for _, output := range outputs {
		refs = append(refs, output)
	}
	return refs, nil
}

func stageFile(sourcePath, stagedPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("read artifact %s: %w", sourcePath, err)
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o755); err != nil {
		return err
	}
	target, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func publishLegacyArtifacts(items []stagedArtifact) (func() error, error) {
	grouped := map[string]*artifactGroup{}
	for _, item := range items {
		destinationRun := filepath.Dir(filepath.Dir(item.destinationPath))
		group := grouped[destinationRun]
		if group == nil {
			group = &artifactGroup{
				stagedRun: filepath.Dir(filepath.Dir(item.stagedPath)), destinationRun: destinationRun,
			}
			grouped[destinationRun] = group
		}
		group.items = append(group.items, item)
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := grouped[key]
		info, err := os.Stat(group.destinationRun)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, conflict("run artifacts", group.destinationRun)
		}
		group.exists = true
		for _, item := range group.items {
			if err := requireEqualFiles(item.stagedPath, item.destinationPath); err != nil {
				return nil, err
			}
		}
	}

	published := make([]*artifactGroup, 0, len(grouped))
	for _, key := range keys {
		group := grouped[key]
		if group.exists {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(group.destinationRun), 0o755); err != nil {
			return nil, errors.Join(err, restorePublishedArtifacts(published))
		}
		if err := os.Rename(group.stagedRun, group.destinationRun); err != nil {
			return nil, errors.Join(err, restorePublishedArtifacts(published))
		}
		published = append(published, group)
	}
	return func() error { return restorePublishedArtifacts(published) }, nil
}

func requireEqualFiles(left, right string) error {
	leftData, err := os.ReadFile(left)
	if err != nil {
		return err
	}
	rightData, err := os.ReadFile(right)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return conflict("artifact", right)
		}
		return err
	}
	if !bytes.Equal(leftData, rightData) {
		return conflict("artifact", right)
	}
	return nil
}

func restorePublishedArtifacts(groups []*artifactGroup) error {
	var rollbackErrors []error
	for i := len(groups) - 1; i >= 0; i-- {
		group := groups[i]
		if err := os.Rename(group.destinationRun, group.stagedRun); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore %s: %w", group.destinationRun, err))
		}
	}
	return errors.Join(rollbackErrors...)
}
