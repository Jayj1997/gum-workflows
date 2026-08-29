// Package runtimepath owns the filesystem layout used by a workflow run.
package runtimepath

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LegacyDatabase is the platform-core SQLite path relative to the working directory.
const LegacyDatabase = ".workflow/gum-workflows.db"

// Paths is an immutable runtime filesystem layout. Constructing Paths only
// computes names; callers decide which command is allowed to create them.
type Paths struct {
	database      string
	executionsDir string
}

// New creates an injectable runtime filesystem layout.
func New(database, executionsDir string) (Paths, error) {
	if strings.TrimSpace(database) == "" {
		return Paths{}, fmt.Errorf("runtime paths: database must not be empty")
	}
	if strings.TrimSpace(executionsDir) == "" {
		return Paths{}, fmt.Errorf("runtime paths: executions directory must not be empty")
	}
	return Paths{database: database, executionsDir: executionsDir}, nil
}

// Legacy returns the platform-core layout relative to the process working directory.
func Legacy() Paths {
	return Paths{
		database:      LegacyDatabase,
		executionsDir: filepath.Join(".workflow", "executions"),
	}
}

// Database returns the SQLite database path.
func (p Paths) Database() string { return p.database }

// ExecutionsDir returns the directory containing every execution.
func (p Paths) ExecutionsDir() string { return p.executionsDir }

// ExecutionDir returns the directory for one execution.
func (p Paths) ExecutionDir(executionID string) string {
	return filepath.Join(p.executionsDir, executionID)
}

// ArtifactsDir returns the Artifact Store directory for one execution.
func (p Paths) ArtifactsDir(executionID string) string {
	return filepath.Join(p.ExecutionDir(executionID), "artifacts")
}

// WorkflowSnapshot returns the workflow definition snapshot path for one execution.
func (p Paths) WorkflowSnapshot(executionID string) string {
	return filepath.Join(p.ExecutionDir(executionID), "workflow.yaml")
}

// LogsDir returns the log directory owned by one execution.
func (p Paths) LogsDir(executionID string) string {
	return filepath.Join(p.ExecutionDir(executionID), "logs")
}

// TempDir returns the temporary-output directory owned by one execution.
func (p Paths) TempDir(executionID string) string {
	return filepath.Join(p.ExecutionDir(executionID), "tmp")
}
