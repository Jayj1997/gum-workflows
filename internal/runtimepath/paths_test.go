package runtimepath_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
)

func TestResolvePrefersEnvironmentOverrideToProductSetting(t *testing.T) {
	environmentRoot := filepath.Join(t.TempDir(), "environment-root")
	productRoot := filepath.Join(t.TempDir(), "product-setting-root")
	t.Setenv(runtimepath.DataRootEnv, environmentRoot)

	paths, err := runtimepath.Resolve(productRoot)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}

	assertPath(t, paths.Database(), filepath.Join(environmentRoot, "product.db"))
	assertPath(t, paths.RunsDir(), filepath.Join(environmentRoot, "runs"))
	if _, err := os.Stat(environmentRoot); !os.IsNotExist(err) {
		t.Fatalf("Resolve() touched the filesystem: %v", err)
	}
}

func TestResolveUsesProductSettingWithoutEnvironmentOverride(t *testing.T) {
	root := filepath.Join(t.TempDir(), "product-setting-root")
	t.Setenv(runtimepath.DataRootEnv, "")

	paths, err := runtimepath.Resolve(root)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}

	assertPath(t, paths.Database(), filepath.Join(root, "product.db"))
	assertPath(t, paths.RunDir("stable-run-id"), filepath.Join(root, "runs", "stable-run-id"))
}

func TestResolveRejectsRelativeDataRoot(t *testing.T) {
	t.Setenv(runtimepath.DataRootEnv, filepath.Join("project", ".gum-data"))

	if _, err := runtimepath.Resolve(""); err == nil {
		t.Fatal("Resolve() = nil error, want relative Local Data Root rejection")
	}
}

func TestStableIDsOwnRunAndNodeRunProducts(t *testing.T) {
	root := t.TempDir()
	t.Setenv(runtimepath.DataRootEnv, "")
	paths, err := runtimepath.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}

	runID := "11223344-1111-4111-8111-111111111111"
	nodeRunID := "55667788-1111-4111-8111-111111111111"
	assertPath(t, paths.ArtifactsDir(runID), filepath.Join(root, "runs", runID, "artifacts"))
	assertPath(t, paths.LogsDir(runID), filepath.Join(root, "runs", runID, "logs"))
	assertPath(t, paths.NodeRunDir(runID, nodeRunID), filepath.Join(root, "runs", runID, "node-runs", nodeRunID))
	assertPath(t, paths.NodeRunLogsDir(runID, nodeRunID), filepath.Join(root, "runs", runID, "node-runs", nodeRunID, "logs"))
	assertPath(t, paths.NodeRunToolOutputDir(runID, nodeRunID), filepath.Join(root, "runs", runID, "node-runs", nodeRunID, "tool-output"))
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
		name     string
		database string
		runs     string
	}{
		{name: "missing database", runs: "runs"},
		{name: "missing runs", database: "product.db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := runtimepath.New(tt.database, tt.runs); err == nil {
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
