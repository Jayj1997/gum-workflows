package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

func TestCoverageBundleRunsPOSIXContract(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		config    node.Config
		verdict   scriptnode.Verdict
		available bool
	}{
		{name: "threshold passed", mode: "passed", config: node.Config{"minimumStatementCoverage": 75}, verdict: scriptnode.VerdictPassed, available: true},
		{name: "test failure is a successful Node Run result", mode: "failed", verdict: scriptnode.VerdictFailed},
		{name: "no statements", mode: "no-statements", verdict: scriptnode.VerdictNotApplicable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsPath := installFakeCoverageGo(t)
			t.Setenv("FAKE_GO_MODE", tt.mode)
			workspace := t.TempDir()
			nodeRunDir := t.TempDir()
			store := artifact.NewMemStore()
			diagnostics := &node.RunDiagnostics{}
			check, err := (coverageExecutor{}).Create(tt.config)
			if err != nil {
				t.Fatal(err)
			}
			outputs, err := check.Execute(node.ExecutionContext{
				Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: store,
				Run:         node.RunContext{LogsDir: filepath.Join(nodeRunDir, "logs"), ToolOutputDir: filepath.Join(nodeRunDir, "tool-output")},
				Diagnostics: diagnostics,
			}, map[string]artifact.ArtifactRef{
				"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace},
			})
			if err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}
			body, err := store.Get(outputs["result"])
			if err != nil {
				t.Fatal(err)
			}
			result, ok := body.Data.(scriptnode.CoverageResult)
			if !ok || result.Verdict != tt.verdict || result.Metrics.StatementCoverage.Available != tt.available {
				t.Fatalf("result = %#v, want %s available=%t", body.Data, tt.verdict, tt.available)
			}
			arguments, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(arguments), "-count=1 -json -covermode=atomic") || !strings.Contains(string(arguments), "./...") {
				t.Errorf("go test arguments = %q, want cache disabled, JSON, atomic coverage, and full scope", arguments)
			}
			stdout, _ := os.ReadFile(filepath.Join(nodeRunDir, "logs", "stdout.log"))
			if !strings.Contains(string(stdout), "running go coverage check") {
				t.Errorf("stdout log = %q", stdout)
			}
			if diagnostics.BundleDigest == "" || diagnostics.ResultAdapter != coverageAdapterID || diagnostics.Toolchain == nil || diagnostics.Toolchain.Tool != "go test" {
				t.Errorf("diagnostics = %+v", diagnostics)
			}
			entries, _ := os.ReadDir(workspace)
			if len(entries) != 0 {
				t.Errorf("workspace contains Gum output: %v", entries)
			}
		})
	}
}

func TestCoverageBundleRejectsSuccessfulRunWithoutProfile(t *testing.T) {
	installFakeCoverageGo(t)
	t.Setenv("FAKE_GO_MODE", "missing-profile")
	workspace := t.TempDir()
	nodeRunDir := t.TempDir()
	store := artifact.NewMemStore()
	check, err := (coverageExecutor{}).Create(nil)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: store,
		Run: node.RunContext{LogsDir: filepath.Join(nodeRunDir, "logs"), ToolOutputDir: filepath.Join(nodeRunDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{
		"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace},
	})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural || outputs != nil {
		t.Fatalf("Execute() = outputs %v, error %v, want Structural Error without result", outputs, err)
	}
	if _, statErr := os.Stat(filepath.Join(nodeRunDir, "tool-output")); !os.IsNotExist(statErr) {
		t.Fatalf("tool-output remains after failure: %v", statErr)
	}
}

func TestCoverageBundleParsesRealGoCoverageProfile(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("coverage v1 supports Darwin and Linux")
	}
	t.Setenv("GOCACHE", t.TempDir())
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOTOOLCHAIN", "local")
	workspace := t.TempDir()
	writeBuiltinFixture(t, filepath.Join(workspace, "go.mod"), "module example.com/coveragefixture\n\ngo 1.25.0\n")
	writeBuiltinFixture(t, filepath.Join(workspace, "math.go"), "package fixture\n\nfunc Double(value int) int { return value * 2 }\n")
	writeBuiltinFixture(t, filepath.Join(workspace, "math_test.go"), "package fixture\n\nimport \"testing\"\n\nfunc TestDouble(t *testing.T) { if Double(2) != 4 { t.Fatal(\"wrong\") } }\n")
	nodeRunDir := t.TempDir()
	store := artifact.NewMemStore()
	check, err := (coverageExecutor{}).Create(nil)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: store,
		Run: node.RunContext{LogsDir: filepath.Join(nodeRunDir, "logs"), ToolOutputDir: filepath.Join(nodeRunDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{
		"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace},
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	body, err := store.Get(outputs["result"])
	if err != nil {
		t.Fatal(err)
	}
	result := body.Data.(scriptnode.CoverageResult)
	metric := result.Metrics.StatementCoverage
	if result.Verdict != scriptnode.VerdictPassed || metric.Value == nil || *metric.Value != 100 {
		t.Fatalf("real Go coverage result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(workspace, "coverage.out")); !os.IsNotExist(err) {
		t.Errorf("Project Workspace contains coverprofile: %v", err)
	}
}

func installFakeCoverageGo(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	argsPath := filepath.Join(t.TempDir(), "test-args.txt")
	script := `#!/bin/sh
case "$1" in
  version)
    printf 'go version go1.25.0 darwin/arm64\n'
    ;;
  env)
    printf 'go1.25.0\n/go\ndarwin\narm64\n1\n'
    ;;
  list)
    printf 'example.com/app\n'
    ;;
  test)
    printf '%s\n' "$*" > "$FAKE_GO_ARGS"
    profile=
    for argument in "$@"; do
      case "$argument" in
        -coverprofile=*) profile=${argument#-coverprofile=} ;;
      esac
    done
    case "$FAKE_GO_MODE" in
      passed)
        printf 'mode: atomic\napp.go:1.1,2.2 3 1\napp.go:3.1,4.2 1 0\n' > "$profile"
        printf '%s\n' '{"Action":"pass","Package":"example.com/app"}'
        ;;
      failed)
        printf '%s\n' '{"Action":"output","Package":"example.com/app","Output":"package example.com/app failed\\n"}'
		printf '%s\n' '{"Action":"fail","Package":"example.com/app"}'
        exit 1
        ;;
      no-statements)
        printf 'mode: atomic\n' > "$profile"
        printf '%s\n' '{"Action":"pass","Package":"example.com/app"}'
        ;;
      missing-profile)
        printf '%s\n' '{"Action":"pass","Package":"example.com/app"}'
        ;;
      *)
        printf 'unexpected fake mode: %s\n' "$FAKE_GO_MODE" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    printf 'unexpected fake go arguments: %s\n' "$*" >&2
    exit 2
    ;;
esac
`
	path := filepath.Join(bin, "go")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_GO_ARGS", argsPath)
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", bin, os.PathListSeparator, os.Getenv("PATH")))
	return argsPath
}
