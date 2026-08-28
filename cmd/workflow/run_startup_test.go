package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
)

type finishInputGateway struct{}

func (finishInputGateway) RequestRound(context.Context, execution.RoundRequest) (execution.RoundResponse, error) {
	return execution.RoundResponse{Content: "build the order system", Finished: true}, nil
}

func prepareRunFixture(t *testing.T) string {
	t.Helper()
	validFixtureEnv(t)
	source, err := os.ReadFile(filepath.Join("..", "..", "examples", "minimal", "workflow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(dir, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func runFixtureUntilSettled(t *testing.T, dir string) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runWorkflow(ctx, "workflow.yaml", true, finishInputGateway{})
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-deadline.C:
			cancel()
			<-done
			return context.DeadlineExceeded
		case <-ticker.C:
			executions, _ := filepath.Glob(filepath.Join(dir, ".workflow", "executions", "execution-*"))
			if len(executions) == 0 || !allNodeStatesSucceeded(executions[len(executions)-1]) {
				continue
			}
			cancel()
			return <-done
		}
	}
}

func allNodeStatesSucceeded(executionDir string) bool {
	states, _ := filepath.Glob(filepath.Join(executionDir, "nodes", "*", "state.json"))
	if len(states) == 0 {
		return false
	}
	for _, state := range states {
		data, err := os.ReadFile(state)
		if err != nil || !strings.Contains(string(data), `"status": "Succeeded"`) {
			return false
		}
	}
	return true
}

func TestRunWorkflowPersistsRepeatedInteractiveExecutions(t *testing.T) {
	dir := prepareRunFixture(t)
	if err := runFixtureUntilSettled(t, dir); err != nil {
		t.Fatalf("first run: %v", err)
	}
	firstStatePath := filepath.Join(dir, ".workflow", "executions", "execution-000001", "state.json")
	firstState, err := os.ReadFile(firstStatePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := runFixtureUntilSettled(t, dir); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".workflow", "executions", "execution-000002", "nodes", "sdk", "state.json")); err != nil {
		t.Fatalf("second execution state missing: %v", err)
	}
	afterSecond, err := os.ReadFile(firstStatePath)
	if err != nil || string(afterSecond) != string(firstState) {
		t.Errorf("first execution state changed after second run: %v", err)
	}

	db, err := sql.Open("sqlite", history.DefaultDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var nodeInstances int
	if err := db.QueryRow(`SELECT count(*) FROM node_instance`).Scan(&nodeInstances); err != nil {
		t.Fatal(err)
	}
	if nodeInstances != 4 {
		t.Errorf("node_instance count = %d, want 4", nodeInstances)
	}
}

func TestRunWorkflowStopsWhenDefinitionDatabaseCannotOpen(t *testing.T) {
	dir := prepareRunFixture(t)
	if err := os.WriteFile(filepath.Join(dir, ".workflow"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runWorkflow(context.Background(), "workflow.yaml", true, finishInputGateway{})
	if err == nil || !strings.Contains(err.Error(), "history database") {
		t.Fatalf("runWorkflow() error = %v, want history database failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".workflow", "executions")); statErr == nil {
		t.Error("engine state exists after database startup failure")
	}
}

func TestRunWorkflowRejectsNewerDatabaseExecutor(t *testing.T) {
	prepareRunFixture(t)
	ctx := context.Background()
	def, _, executors, definitions, llmConfig, _, err := loadAndValidate(ctx, "workflow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pinAndImportDefinitions(ctx, def, executors, definitions, llmConfig); err != nil {
		t.Fatal(err)
	}
	store, err := history.Open(ctx, history.DefaultDBPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ImportDefinitions(ctx, nil, nil, []history.NodeExecRow{{
		Node: "coding-agent", Version: "v2", Name: "coding-agent-v2",
	}}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	def, _, executors, definitions, llmConfig, _, err = loadAndValidate(ctx, "workflow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pinAndImportDefinitions(ctx, def, executors, definitions, llmConfig); err == nil || !strings.Contains(err.Error(), `executor "v2"`) {
		t.Fatalf("pinAndImportDefinitions() error = %v, want unavailable v2", err)
	}
}

func TestRunWorkflowMigratesVersionZeroDatabase(t *testing.T) {
	prepareRunFixture(t)
	if err := os.Mkdir(".workflow", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(history.DefaultDBPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	def, _, executors, definitions, llmConfig, _, err := loadAndValidate(ctx, "workflow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pinAndImportDefinitions(ctx, def, executors, definitions, llmConfig); err != nil {
		t.Fatalf("pinAndImportDefinitions() migration error: %v", err)
	}
	store, err := history.OpenReadOnly(ctx, history.DefaultDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	version, err := store.UserVersion(ctx)
	if err != nil || version < history.DefinitionSchemaVersion {
		t.Errorf("database version = %d/%v, want at least %d", version, err, history.DefinitionSchemaVersion)
	}
}
