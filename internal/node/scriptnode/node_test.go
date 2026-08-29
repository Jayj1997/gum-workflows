package scriptnode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

func TestNodeExecuteStreamsLogsAndAdaptsOnlyFormalToolOutputs(t *testing.T) {
	workspace := t.TempDir()
	nodeRunDir := t.TempDir()
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: test-check
executor: v1
entry: check.sh
platforms: [` + runtime.GOOS + `]
requirements: {executables: [sh]}
toolOutputs: [{path: result.txt, required: true}]
resultAdapter: test/v1
`)
	manifest, err := LoadManifest(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	bundle := Bundle{
		Manifest: manifest, ManifestBytes: manifestBytes,
		Files: map[string][]byte{"check.sh": []byte("#!/bin/sh\nprintf 'log-only\\n'\n" +
			"printf 'warning-only\\n' >&2\nprintf 'formal-result\\n' > \"$2/result.txt\"\n")},
	}
	bundle.ExpectedDigest = bundle.Digest()

	adapter := func(record ExecutionRecord) (artifact.Artifact, error) {
		data, err := os.ReadFile(filepath.Join(record.ToolOutputDir, "result.txt"))
		if err != nil {
			return artifact.Artifact{}, err
		}
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult, Data: strings.TrimSpace(string(data))}, nil
	}
	check, err := New(bundle, "test-check", "v1", "test/v1", adapter)
	if err != nil {
		t.Fatal(err)
	}
	store := artifact.NewMemStore()
	diagnostics := &node.RunDiagnostics{}
	ctx := node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: store,
		Run: node.RunContext{
			WorkflowRunID: "run-1", NodeRunID: "node-run-1",
			LogsDir: filepath.Join(nodeRunDir, "logs"), ToolOutputDir: filepath.Join(nodeRunDir, "tool-output"),
		},
		Diagnostics: diagnostics,
	}
	code := artifact.ArtifactRef{ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}
	outputs, err := check.Execute(ctx, map[string]artifact.ArtifactRef{"code": code})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	result, err := store.Get(outputs["result"])
	if err != nil || result.Data != "formal-result" {
		t.Fatalf("result = %+v/%v, want formal tool output only", result, err)
	}
	stdout, _ := os.ReadFile(filepath.Join(ctx.Run.LogsDir, "stdout.log"))
	stderr, _ := os.ReadFile(filepath.Join(ctx.Run.LogsDir, "stderr.log"))
	if string(stdout) != "log-only\n" || string(stderr) != "warning-only\n" {
		t.Errorf("logs = stdout %q stderr %q", stdout, stderr)
	}
	if diagnostics.BundleDigest != bundle.ExpectedDigest || diagnostics.CWD != workspace || len(diagnostics.Arguments) != 2 {
		t.Errorf("diagnostics = %+v", diagnostics)
	}
	if _, err := os.Stat(ctx.Run.ToolOutputDir); !os.IsNotExist(err) {
		t.Errorf("tool-output remains after successful adaptation: %v", err)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Errorf("workspace entries = %v/%v, want no Gum products", entries, err)
	}
}

func TestNodeExecuteRevalidatesImmutableBundleBeforeLaunch(t *testing.T) {
	bundle := testBundle(t, "toolOutputs: []", "#!/bin/sh\nexit 0\n")
	adapterCalled := false
	check, err := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		adapterCalled = true
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle.Files["check.sh"] = []byte("#!/bin/sh\nprintf 'tampered\\n'\n")

	workspace := t.TempDir()
	runDir := t.TempDir()
	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural || !strings.Contains(err.Error(), "bundle digest mismatch") {
		t.Fatalf("Execute(tampered bundle) error = %v, want digest Structural Error", err)
	}
	if adapterCalled {
		t.Fatal("result adapter called for tampered bundle")
	}
}

func TestNodeExecuteRejectsMaterializedBundleMutationAfterRun(t *testing.T) {
	bundle := testBundleWithRequirements(t, "[sh, chmod]", "toolOutputs: [{path: result.txt, required: true}]", `#!/bin/sh
chmod u+w "${0%/*}/helper.txt"
printf 'changed\n' > "${0%/*}/helper.txt"
printf 'formal-result\n' > "$2/result.txt"
`)
	bundle.Files["helper.txt"] = []byte("immutable helper\n")
	bundle.ExpectedDigest = bundle.Digest()
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
	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural || !strings.Contains(err.Error(), "materialized bundle file") {
		t.Fatalf("Execute(mutated materialized bundle) error = %v, want Structural Error", err)
	}
	if adapterCalled {
		t.Fatal("result adapter called after materialized bundle mutation")
	}
}

func TestNodeExecuteRejectsNodeRunPathResolvedInsideWorkspace(t *testing.T) {
	bundle := testBundle(t, "toolOutputs: []", "#!/bin/sh\nexit 0\n")
	check, err := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	stateLink := filepath.Join(t.TempDir(), "node-run")
	if err := os.Symlink(workspace, stateLink); err != nil {
		t.Fatal(err)
	}
	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(stateLink, "logs"), ToolOutputDir: filepath.Join(stateLink, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural || !strings.Contains(err.Error(), "outside Project Workspace") {
		t.Fatalf("Execute(linked Node Run path) error = %v, want path Structural Error", err)
	}
	entries, readErr := os.ReadDir(workspace)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("workspace entries = %v/%v, want no Gum products", entries, readErr)
	}
}

func TestNodeExecuteRejectsToolOutputSymlinkEscape(t *testing.T) {
	bundle := testBundleWithRequirements(t, "[sh, ln]", "toolOutputs: [{path: result.txt, required: true}]", "#!/bin/sh\nln -s \"$1/outside.txt\" \"$2/result.txt\"\n")
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
	toolOutputDir := filepath.Join(runDir, "tool-output")
	outside := filepath.Join(workspace, "outside.txt")
	if err := os.WriteFile(outside, []byte("not formal evidence"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: toolOutputDir},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Execute(symlink output) error = %v, want path Structural Error", err)
	}
	if adapterCalled {
		t.Fatal("result adapter called for escaped tool output")
	}
	if _, statErr := os.Stat(toolOutputDir); !os.IsNotExist(statErr) {
		t.Errorf("tool-output remains after rejected evidence: %v", statErr)
	}
}

func TestNodeExecuteRecordsOnlyNonSensitiveHostDiagnostics(t *testing.T) {
	bin := t.TempDir()
	goPath := filepath.Join(bin, "go")
	if err := os.WriteFile(goPath, []byte(`#!/bin/sh
case "$1" in
  version) printf 'go version go1.25.0 darwin/arm64\n' ;;
  env) printf 'go1.25.0\n/test/goroot\ndarwin\narm64\n1\n' ;;
  *) exit 2 ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SECRET_TOKEN", "must-not-enter-history")
	bundle := testBundleWithRequirements(t, "[sh, go]", "toolOutputs: []", "#!/bin/sh\nexit 0\n")
	check, err := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	runDir := t.TempDir()
	diagnostics := &node.RunDiagnostics{}
	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run:         node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
		Diagnostics: diagnostics,
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Host == nil || diagnostics.Host.GOOS != runtime.GOOS || diagnostics.Host.GOARCH != runtime.GOARCH {
		t.Errorf("host diagnostics = %+v", diagnostics.Host)
	}
	if diagnostics.Toolchain == nil || diagnostics.Executables["go"] != goPath || diagnostics.Toolchain.LauncherVersion != "go version go1.25.0 darwin/arm64" ||
		diagnostics.Toolchain.GOROOT != "/test/goroot" || diagnostics.Toolchain.CGOEnabled != "1" {
		t.Errorf("tool diagnostics = executables %+v toolchain %+v", diagnostics.Executables, diagnostics.Toolchain)
	}
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "SECRET_TOKEN") || strings.Contains(string(encoded), "must-not-enter-history") {
		t.Fatalf("diagnostics persisted sensitive environment: %s", encoded)
	}
}

func testBundle(t *testing.T, toolOutputs, script string) Bundle {
	return testBundleWithRequirements(t, "[sh]", toolOutputs, script)
}

func testBundleWithRequirements(t *testing.T, executables, toolOutputs, script string) Bundle {
	t.Helper()
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: test-check
executor: v1
entry: check.sh
platforms: [` + runtime.GOOS + `]
requirements: {executables: ` + executables + `}
` + toolOutputs + `
resultAdapter: test/v1
`)
	manifest, err := LoadManifest(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	bundle := Bundle{Manifest: manifest, ManifestBytes: manifestBytes, Files: map[string][]byte{"check.sh": []byte(script)}}
	bundle.ExpectedDigest = bundle.Digest()
	return bundle
}

func TestNodeExecuteTreatsMissingFormalOutputAsStructuralError(t *testing.T) {
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: test-check
executor: v1
entry: check.sh
platforms: [` + runtime.GOOS + `]
requirements: {executables: [sh]}
toolOutputs: [{path: result.txt, required: true}]
resultAdapter: test/v1
`)
	manifest, _ := LoadManifest(manifestBytes)
	bundle := Bundle{Manifest: manifest, ManifestBytes: manifestBytes, Files: map[string][]byte{"check.sh": []byte("#!/bin/sh\nexit 0\n")}}
	bundle.ExpectedDigest = bundle.Digest()
	check, err := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		return artifact.Artifact{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	workspace := t.TempDir()
	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural {
		t.Fatalf("Execute() error = %v, want Structural Error", err)
	}
}

func TestNodeExecuteRejectsCodeReferenceForAnotherWorkspace(t *testing.T) {
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: test-check
executor: v1
entry: check.sh
platforms: [` + runtime.GOOS + `]
requirements: {executables: [sh]}
toolOutputs: []
resultAdapter: test/v1
`)
	manifest, _ := LoadManifest(manifestBytes)
	bundle := Bundle{Manifest: manifest, ManifestBytes: manifestBytes, Files: map[string][]byte{"check.sh": []byte("#!/bin/sh\nexit 0\n")}}
	bundle.ExpectedDigest = bundle.Digest()
	check, _ := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult}, nil
	})
	runDir := t.TempDir()
	workspace := t.TempDir()
	_, err := check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: t.TempDir()}})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural {
		t.Fatalf("Execute(mismatched code ref) error = %v, want Structural Error", err)
	}
}

func TestNodeExecuteHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: test-check
executor: v1
entry: check.sh
platforms: [` + runtime.GOOS + `]
requirements: {executables: [sh]}
toolOutputs: []
resultAdapter: test/v1
`)
	manifest, _ := LoadManifest(manifestBytes)
	bundle := Bundle{Manifest: manifest, ManifestBytes: manifestBytes, Files: map[string][]byte{"check.sh": []byte("#!/bin/sh\nsleep 10\n")}}
	bundle.ExpectedDigest = bundle.Digest()
	check, _ := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult}, nil
	})
	runDir := t.TempDir()
	workspace := t.TempDir()
	started := time.Now()
	_, err := check.Execute(node.ExecutionContext{
		Context: ctx, Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("Execute(canceled) = %v after %s", err, time.Since(started))
	}
}
