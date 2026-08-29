//go:build darwin || linux

package scriptnode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

func TestNodeExecuteCancellationTerminatesChildProcessGroup(t *testing.T) {
	bundle := testBundle(t, "toolOutputs: [{path: child.pid, required: true}]", `#!/bin/sh
(trap '' TERM; while :; do :; done) &
child=$!
printf '%s\n' "$child" > "$2/child.pid"
wait "$child"
`)
	adapterCalled := false
	check, err := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		adapterCalled = true
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	workspace := t.TempDir()
	runDir := t.TempDir()
	toolOutputDir := filepath.Join(runDir, "tool-output")
	done := make(chan error, 1)
	go func() {
		_, executeErr := check.Execute(node.ExecutionContext{
			Context: ctx, Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
			Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: toolOutputDir},
		}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
		done <- executeErr
	}()

	pid := waitForPIDFile(t, filepath.Join(toolOutputDir, "child.pid"))
	cancel()
	select {
	case executeErr := <-done:
		if executeErr == nil || node.ErrorKindOf(executeErr) != node.ErrorKindStructural || !errors.Is(executeErr, context.Canceled) {
			t.Fatalf("Execute(canceled) error = %v, want canceled Structural Error", executeErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Execute did not return after cancellation")
	}
	if adapterCalled {
		t.Fatal("result adapter called after cancellation")
	}
	deadline := time.Now().Add(3 * time.Second)
	for processExists(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processExists(pid) {
		t.Fatalf("child process %d remains after cancellation", pid)
	}
}

func TestNodeExecuteLogLimitTerminatesProcessWithoutResult(t *testing.T) {
	line := strings.Repeat("x", 1023) + "\\n"
	bundle := testBundle(t, "toolOutputs: []", "#!/bin/sh\nwhile :; do printf '"+line+"'; done\n")
	adapterCalled := false
	check, err := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		adapterCalled = true
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	runDir := t.TempDir()
	started := time.Now()
	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural || !strings.Contains(err.Error(), "log output exceeded") {
		t.Fatalf("Execute(unbounded logs) error = %v, want log limit Structural Error", err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatalf("log limit took %s to terminate process", time.Since(started))
	}
	if adapterCalled {
		t.Fatal("result adapter called after log limit")
	}
	stdout, readErr := os.ReadFile(filepath.Join(runDir, "logs", "stdout.log"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if int64(len(stdout)) > maxLogBytes {
		t.Fatalf("stdout size = %d, exceeds fixed limit %d", len(stdout), maxLogBytes)
	}
}

func TestNodeExecuteReportsFailurePathCleanupError(t *testing.T) {
	bundle := testBundleWithRequirements(t, "[sh, chmod]", "toolOutputs: []", `#!/bin/sh
chmod 0555 "${2%/*}"
exit 0
`)
	check, err := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		return artifact.Artifact{}, errors.New("adapter failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	runDir := t.TempDir()
	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if chmodErr := os.Chmod(runDir, 0o755); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if err == nil || !strings.Contains(err.Error(), "adapter failed") || !strings.Contains(err.Error(), "remove non-persistent tool-output after failure") {
		t.Fatalf("Execute(cleanup failure) error = %v, want adapter and cleanup errors", err)
	}
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr != nil {
				t.Fatalf("parse child pid %q: %v", data, parseErr)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child pid was not published at %s", path)
	return 0
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
