package scriptnode

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

func TestStaticAdapterBuildsPassedFailedAndNotApplicableResults(t *testing.T) {
	tests := []struct {
		name     string
		packages string
		vet      string
		stderr   string
		exitCode int
		verdict  Verdict
		findings int
	}{
		{name: "passed", packages: "example.com/app\n", vet: `{}` + "\n", verdict: VerdictPassed},
		{
			name: "vet diagnostic", packages: "example.com/app\n", exitCode: 1,
			vet:     `{"example.com/app":{"printf":[{"posn":"app.go:9:2","message":"wrong printf format"}]}}` + "\n",
			verdict: VerdictFailed, findings: 1,
		},
		{
			name: "package diagnostic", packages: "example.com/app\n", exitCode: 1, vet: "",
			stderr: "app.go:4:2: undefined: missing\n", verdict: VerdictFailed, findings: 1,
		},
		{name: "no package", packages: "", vet: "", verdict: VerdictNotApplicable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := staticExecutionFixture(t, tt.packages, tt.vet, tt.stderr, tt.exitCode)
			result, err := AdaptStaticResult(record)
			if err != nil {
				t.Fatalf("AdaptStaticResult() unexpected error: %v", err)
			}
			if result.Verdict != tt.verdict || result.FindingsCount != tt.findings {
				t.Errorf("result = %+v, want verdict %s and %d findings", result, tt.verdict, tt.findings)
			}
			if err := result.Validate(); err != nil {
				t.Errorf("result contract: %v", err)
			}
		})
	}
}

func TestStaticAdapterRejectsDamagedOrIncompleteToolOutput(t *testing.T) {
	tests := []struct {
		name, packages, vet, stderr string
		exitCode                    int
	}{
		{name: "invalid vet JSON", packages: "example.com/app\n", vet: `{not-json}`, exitCode: 1},
		{name: "non-JSON successful artifact", packages: "example.com/app\n", vet: `truncated`, exitCode: 0},
		{name: "unexplained tool failure", packages: "example.com/app\n", vet: "", exitCode: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AdaptStaticResult(staticExecutionFixture(t, tt.packages, tt.vet, tt.stderr, tt.exitCode))
			if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural {
				t.Fatalf("AdaptStaticResult() error = %v, want Structural Error", err)
			}
		})
	}
}

func staticExecutionFixture(t *testing.T, packages, vet, stderr string, exitCode int) ExecutionRecord {
	t.Helper()
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, "packages.txt"), packages)
	writeFixture(t, filepath.Join(dir, "vet.json"), vet)
	writeFixture(t, filepath.Join(dir, "go-version.txt"), "go version go1.25.0 darwin/arm64\n")
	writeFixture(t, filepath.Join(dir, "go-env.txt"), "go1.25.0\n/go\ndarwin\narm64\n1\n")
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	writeFixture(t, stdoutPath, "script note\n")
	writeFixture(t, stderrPath, stderr)
	started := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	return ExecutionRecord{
		ExitCode: exitCode, ToolOutputDir: dir, StdoutPath: stdoutPath, StderrPath: stderrPath,
		Code:      artifact.ArtifactRef{ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: "/workspace"},
		StartedAt: started, FinishedAt: started.Add(time.Second),
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
