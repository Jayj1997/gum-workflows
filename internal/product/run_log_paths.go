package product

import (
	"os"
	"path/filepath"

	"github.com/Jayj1997/gum-workflows/internal/redaction"
)

// runLogPath returns the sanitized run log file location for one Run.
func (a *Application) runLogPath(runID string) string {
	if a.runPaths.RunsDir() == "" {
		return ""
	}
	return filepath.Join(a.runPaths.LogsDir(runID), "run.log")
}

// writeRunLogToBundle copies the already-sanitized run log into the bundle.
func writeRunLogToBundle(bundleDir, sourceLogPath string, redactor *redaction.Redactor) error {
	file, err := os.Create(filepath.Join(bundleDir, "run.log"))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return writeLogCopy(file, sourceLogPath, redactor)
}
