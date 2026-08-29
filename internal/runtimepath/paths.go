// Package runtimepath owns the filesystem layout used by a workflow run.
package runtimepath

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DataRootEnv overrides the user-level Local Data Root for CLI processes.
const DataRootEnv = "GUM_WORKFLOWS_DATA_ROOT"

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

// Resolve returns the product filesystem layout without creating it. A dedicated
// environment override wins over productSetting; an empty setting uses the
// operating system's conventional per-user application data directory.
func Resolve(productSetting string) (Paths, error) {
	root := strings.TrimSpace(os.Getenv(DataRootEnv))
	if root == "" {
		root = strings.TrimSpace(productSetting)
	}
	if root == "" {
		var err error
		root, err = defaultDataRoot()
		if err != nil {
			return Paths{}, err
		}
	}
	if !filepath.IsAbs(root) {
		return Paths{}, fmt.Errorf("resolve local data root: path must be absolute")
	}
	return New(filepath.Join(root, "product.db"), filepath.Join(root, "runs"))
}

func defaultDataRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve local data root: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "gum-workflows"), nil
	case "windows":
		if root := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); root != "" {
			return filepath.Join(root, "gum-workflows"), nil
		}
		return filepath.Join(home, "AppData", "Local", "gum-workflows"), nil
	default:
		if root := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); root != "" {
			return filepath.Join(root, "gum-workflows"), nil
		}
		return filepath.Join(home, ".local", "share", "gum-workflows"), nil
	}
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

// NodeRunDir returns the directory owned by one stable Node Run ID.
func (p Paths) NodeRunDir(executionID, nodeRunID string) string {
	return filepath.Join(p.ExecutionDir(executionID), "node-runs", nodeRunID)
}

// NodeRunLogsDir returns the log directory owned by one Node Run.
func (p Paths) NodeRunLogsDir(executionID, nodeRunID string) string {
	return filepath.Join(p.NodeRunDir(executionID, nodeRunID), "logs")
}

// NodeRunToolOutputDir returns the tool-output directory owned by one Node Run.
func (p Paths) NodeRunToolOutputDir(executionID, nodeRunID string) string {
	return filepath.Join(p.NodeRunDir(executionID, nodeRunID), "tool-output")
}

// TempDir returns the temporary-output directory owned by one execution.
func (p Paths) TempDir(executionID string) string {
	return filepath.Join(p.ExecutionDir(executionID), "tmp")
}
