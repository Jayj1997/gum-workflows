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

func TestRaceBundleRunsPOSIXContract(t *testing.T) {
	tests := []struct {
		name, mode string
		verdict    scriptnode.Verdict
		races      int
	}{
		{name: "no race observed", mode: "passed", verdict: scriptnode.VerdictPassed},
		{name: "race observed", mode: "race", verdict: scriptnode.VerdictFailed, races: 1},
		{name: "ordinary test failure", mode: "test-failure", verdict: scriptnode.VerdictFailed},
		{name: "compile failure", mode: "compile-failure", verdict: scriptnode.VerdictFailed},
		{name: "no Go package", mode: "empty", verdict: scriptnode.VerdictNotApplicable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsPath := installFakeRaceToolchain(t)
			t.Setenv("FAKE_GO_MODE", tt.mode)
			workspace := t.TempDir()
			nodeRunDir := t.TempDir()
			store := artifact.NewMemStore()
			diagnostics := &node.RunDiagnostics{}
			check, err := (raceExecutor{}).Create(nil)
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
			result, ok := body.Data.(scriptnode.RaceResult)
			if !ok || result.Verdict != tt.verdict {
				t.Fatalf("result = %#v, want %s", body.Data, tt.verdict)
			}
			if result.Metrics.RacesDetected.Available && (result.Metrics.RacesDetected.Value == nil || *result.Metrics.RacesDetected.Value != tt.races) {
				t.Errorf("racesDetected = %+v, want %d", result.Metrics.RacesDetected, tt.races)
			}
			arguments, err := os.ReadFile(argsPath)
			if tt.mode != "empty" && (err != nil || !strings.Contains(string(arguments), "-race -count=1 -json ./...")) {
				t.Errorf("go test arguments = %q/%v, want race, cache disabled, JSON, and full scope", arguments, err)
			}
			stdout, _ := os.ReadFile(filepath.Join(nodeRunDir, "logs", "stdout.log"))
			if !strings.Contains(string(stdout), "running go race check") {
				t.Errorf("stdout log = %q", stdout)
			}
			if diagnostics.BundleDigest == "" || diagnostics.ResultAdapter != raceAdapterID || diagnostics.Toolchain == nil || diagnostics.Toolchain.Tool != "go test -race" || diagnostics.Toolchain.CCompiler != "fake-cc" {
				t.Errorf("diagnostics = %+v", diagnostics)
			}
			entries, _ := os.ReadDir(workspace)
			if len(entries) != 0 {
				t.Errorf("workspace contains Gum output: %v", entries)
			}
		})
	}
}

func TestRaceExecutorDiagnosesRaceRequirementsBeforeRun(t *testing.T) {
	tests := []struct {
		name, requirementMode, want string
	}{
		{name: "target platform differs from host", requirementMode: "host-mismatch", want: "must match host"},
		{name: "CGO disabled", requirementMode: "cgo-disabled", want: "CGO_ENABLED=1"},
		{name: "unsupported architecture", requirementMode: "unsupported-arch", want: "does not support"},
		{name: "missing C compiler", requirementMode: "missing-cc", want: "C compiler"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installFakeRaceToolchain(t)
			t.Setenv("FAKE_RACE_REQUIREMENT", tt.requirementMode)
			err := (raceExecutor{}).ValidateHostRequirements()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateHostRequirements() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRaceBundleRejectsInvalidGoJSON(t *testing.T) {
	installFakeRaceToolchain(t)
	t.Setenv("FAKE_GO_MODE", "invalid-json")
	workspace := t.TempDir()
	nodeRunDir := t.TempDir()
	check, err := (raceExecutor{}).Create(nil)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(nodeRunDir, "logs"), ToolOutputDir: filepath.Join(nodeRunDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{
		"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace},
	})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural || outputs != nil {
		t.Fatalf("Execute() = %v, %v, want Structural Error without result", outputs, err)
	}
	if _, statErr := os.Stat(filepath.Join(nodeRunDir, "tool-output")); !os.IsNotExist(statErr) {
		t.Fatalf("tool-output remains after failure: %v", statErr)
	}
}

func TestRaceNodeRequirementFailureIsStructural(t *testing.T) {
	installFakeRaceToolchain(t)
	t.Setenv("FAKE_RACE_REQUIREMENT", "cgo-disabled")
	workspace := t.TempDir()
	check, err := (raceExecutor{}).Create(nil)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(t.TempDir(), "logs"), ToolOutputDir: filepath.Join(t.TempDir(), "tool-output")},
	}, map[string]artifact.ArtifactRef{
		"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace},
	})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural || outputs != nil {
		t.Fatalf("Execute() = %v, %v, want Structural Error without result", outputs, err)
	}
}

func TestRaceExecutorRejectsNodeScriptOrToolOverrides(t *testing.T) {
	for _, config := range []node.Config{{"script": "echo bypass"}, {"command": "go test"}, {"packageScope": "./pkg/..."}} {
		if _, err := (raceExecutor{}).Create(config); err == nil {
			t.Errorf("Create(%v) = nil error, want closed config rejection", config)
		}
	}
}

func installFakeRaceToolchain(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	argsPath := filepath.Join(t.TempDir(), "race-test-args.txt")
	goScript := `#!/bin/sh
case "$1" in
  version)
    printf 'go version go1.25.0 %s/%s\n' "$FAKE_GOOS" "$FAKE_GOARCH"
    ;;
  env)
    case "$2" in
      GOVERSION) printf 'go1.25.0\n/go\n%s\n%s\n1\n' "$FAKE_GOOS" "$FAKE_GOARCH" ;;
      GOOS)
        case "$FAKE_RACE_REQUIREMENT" in
          host-mismatch) printf 'unsupported-os\n%s\n1\nfake-cc\n' "$FAKE_GOARCH" ;;
          cgo-disabled) printf '%s\n%s\n0\nfake-cc\n' "$FAKE_GOOS" "$FAKE_GOARCH" ;;
          unsupported-arch) printf '%s\nwasm\n1\nfake-cc\n' "$FAKE_GOOS" ;;
          missing-cc) printf '%s\n%s\n1\nmissing-race-cc\n' "$FAKE_GOOS" "$FAKE_GOARCH" ;;
          *) printf '%s\n%s\n1\nfake-cc\n' "$FAKE_GOOS" "$FAKE_GOARCH" ;;
        esac
        ;;
      CC) printf 'fake-cc\n' ;;
      *) exit 2 ;;
    esac
    ;;
  list)
    if [ "$FAKE_GO_MODE" != empty ]; then printf 'example.com/app\n'; fi
    ;;
  test)
    printf '%s\n' "$*" > "$FAKE_GO_ARGS"
    case "$FAKE_GO_MODE" in
      passed) printf '%s\n' '{"Action":"run","Package":"example.com/app","Test":"TestSafe"}' '{"Action":"pass","Package":"example.com/app","Test":"TestSafe"}' '{"Action":"pass","Package":"example.com/app"}' ;;
      race) printf '%s\n' '{"Action":"run","Package":"example.com/app","Test":"TestRace"}' '{"Action":"output","Package":"example.com/app","Test":"TestRace","Output":"WARNING: DATA RACE\\n"}' '{"Action":"fail","Package":"example.com/app","Test":"TestRace"}' '{"Action":"fail","Package":"example.com/app"}'; exit 1 ;;
      test-failure) printf '%s\n' '{"Action":"run","Package":"example.com/app","Test":"TestBroken"}' '{"Action":"output","Package":"example.com/app","Test":"TestBroken","Output":"app_test.go:9: wrong value\\n"}' '{"Action":"fail","Package":"example.com/app","Test":"TestBroken"}' '{"Action":"fail","Package":"example.com/app"}'; exit 1 ;;
      compile-failure) printf '%s\n' '{"Action":"output","Package":"example.com/app","Output":"./app.go:4: undefined: missing\\n"}' '{"Action":"fail","Package":"example.com/app"}'; exit 1 ;;
      invalid-json) printf '{not-json}\n'; exit 1 ;;
      *) printf 'unexpected fake mode: %s\n' "$FAKE_GO_MODE" >&2; exit 2 ;;
    esac
    ;;
  *) printf 'unexpected fake go arguments: %s\n' "$*" >&2; exit 2 ;;
esac
`
	writeBuiltinFixture(t, filepath.Join(bin, "go"), goScript)
	if err := os.Chmod(filepath.Join(bin, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeBuiltinFixture(t, filepath.Join(bin, "fake-cc"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(bin, "fake-cc"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_GO_ARGS", argsPath)
	t.Setenv("FAKE_GOOS", runtime.GOOS)
	t.Setenv("FAKE_GOARCH", runtime.GOARCH)
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", bin, os.PathListSeparator, os.Getenv("PATH")))
	return argsPath
}
