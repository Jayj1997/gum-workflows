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

// Paths is an immutable runtime filesystem layout. Constructing Paths only
// computes names; callers decide which command is allowed to create them.
type Paths struct {
	database string
	runsDir  string
}

// New creates an injectable runtime filesystem layout.
func New(database, runsDir string) (Paths, error) {
	if strings.TrimSpace(database) == "" {
		return Paths{}, fmt.Errorf("runtime paths: database must not be empty")
	}
	if strings.TrimSpace(runsDir) == "" {
		return Paths{}, fmt.Errorf("runtime paths: runs directory must not be empty")
	}
	return Paths{database: database, runsDir: runsDir}, nil
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

// Database returns the SQLite database path.
func (p Paths) Database() string { return p.database }

// RunsDir returns the directory containing every Workflow Run.
func (p Paths) RunsDir() string { return p.runsDir }

// RunDir returns the directory for one Workflow Run.
func (p Paths) RunDir(runID string) string {
	return filepath.Join(p.runsDir, runID)
}

// ArtifactsDir returns the Artifact Store directory for one Workflow Run.
func (p Paths) ArtifactsDir(runID string) string {
	return filepath.Join(p.RunDir(runID), "artifacts")
}

// WorkflowSnapshot returns the workflow definition snapshot path for one Workflow Run.
func (p Paths) WorkflowSnapshot(runID string) string {
	return filepath.Join(p.RunDir(runID), "workflow.yaml")
}

// LogsDir returns the log directory owned by one Workflow Run.
func (p Paths) LogsDir(runID string) string {
	return filepath.Join(p.RunDir(runID), "logs")
}

// NodeRunDir returns the directory owned by one stable Node Run ID.
func (p Paths) NodeRunDir(runID, nodeRunID string) string {
	return filepath.Join(p.RunDir(runID), "node-runs", nodeRunID)
}

// NodeRunLogsDir returns the log directory owned by one Node Run.
func (p Paths) NodeRunLogsDir(runID, nodeRunID string) string {
	return filepath.Join(p.NodeRunDir(runID, nodeRunID), "logs")
}

// NodeRunToolOutputDir returns the tool-output directory owned by one Node Run.
func (p Paths) NodeRunToolOutputDir(runID, nodeRunID string) string {
	return filepath.Join(p.NodeRunDir(runID, nodeRunID), "tool-output")
}

// TempDir returns the temporary-output directory owned by one Workflow Run.
func (p Paths) TempDir(runID string) string {
	return filepath.Join(p.RunDir(runID), "tmp")
}
