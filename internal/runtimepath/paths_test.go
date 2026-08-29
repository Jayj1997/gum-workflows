package runtimepath_test

import (
	"path/filepath"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
)

func TestLegacyPathsPreservePlatformCoreLayout(t *testing.T) {
	paths := runtimepath.Legacy()

	assertPath(t, paths.Database(), filepath.Join(".workflow", "gum-workflows.db"))
	assertPath(t, paths.ExecutionsDir(), filepath.Join(".workflow", "executions"))
	assertPath(t, paths.ExecutionDir("execution-000007"), filepath.Join(".workflow", "executions", "execution-000007"))
	assertPath(t, paths.ArtifactsDir("execution-000007"), filepath.Join(".workflow", "executions", "execution-000007", "artifacts"))
	assertPath(t, paths.LogsDir("execution-000007"), filepath.Join(".workflow", "executions", "execution-000007", "logs"))
	assertPath(t, paths.TempDir("execution-000007"), filepath.Join(".workflow", "executions", "execution-000007", "tmp"))
}

func TestPathsCanBeInjectedWithoutTouchingFilesystem(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	paths, err := runtimepath.New(
		filepath.Join(root, "product.db"),
		filepath.Join(root, "runs"),
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	assertPath(t, paths.Database(), filepath.Join(root, "product.db"))
	assertPath(t, paths.ArtifactsDir("run-123"), filepath.Join(root, "runs", "run-123", "artifacts"))
	assertPath(t, paths.LogsDir("run-123"), filepath.Join(root, "runs", "run-123", "logs"))
	assertPath(t, paths.TempDir("run-123"), filepath.Join(root, "runs", "run-123", "tmp"))
}

func TestNewRejectsIncompleteLayout(t *testing.T) {
	tests := []struct {
		name       string
		database   string
		executions string
	}{
		{name: "missing database", executions: "runs"},
		{name: "missing executions", database: "product.db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := runtimepath.New(tt.database, tt.executions); err == nil {
				t.Fatal("New() = nil error, want incomplete layout rejection")
			}
		})
	}
}

func assertPath(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
