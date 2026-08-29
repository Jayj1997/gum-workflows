package scriptnode

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

func TestCoverageResultValidationAcceptsCompletePassedResult(t *testing.T) {
	result := validCoverageResult()
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCoverageResult(data)
	if err != nil {
		t.Fatalf("DecodeCoverageResult() unexpected error: %v", err)
	}
	metric := decoded.Metrics.StatementCoverage
	if decoded.Verdict != VerdictPassed || metric.Value == nil || *metric.Value != 80 {
		t.Errorf("decoded result = %+v", decoded)
	}
}

func TestCoverageResultValidationRejectsContradictionsAndUnknownFields(t *testing.T) {
	tests := map[string]func(*CoverageResult){
		"unknown verdict":        func(result *CoverageResult) { result.Verdict = "green" },
		"wrong code kind":        func(result *CoverageResult) { result.Code.Kind = artifact.KindOpenAPI },
		"passed below threshold": func(result *CoverageResult) { *result.Metrics.StatementCoverage.Value = 79.9 },
		"available with reason":  func(result *CoverageResult) { result.Metrics.StatementCoverage.Reason = "contradiction" },
		"failed without finding": func(result *CoverageResult) {
			result.Verdict = VerdictFailed
			*result.Metrics.StatementCoverage.Value = 70
		},
		"invalid time": func(result *CoverageResult) { result.FinishedAt = result.StartedAt.Add(-time.Second) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := validCoverageResult()
			mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("Validate() = nil error, want rejection")
			}
		})
	}

	data, _ := json.Marshal(validCoverageResult())
	withUnknown := strings.Replace(string(data), `"verdict":"passed"`, `"verdict":"passed","extra":true`, 1)
	if _, err := DecodeCoverageResult([]byte(withUnknown)); err == nil {
		t.Fatal("DecodeCoverageResult(unknown field) = nil error")
	}
}

func validCoverageResult() CoverageResult {
	started := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	coverage := 80.0
	return CoverageResult{
		APIVersion: qualityCheckResultAPIVersion, Check: CoverageCheck, Verdict: VerdictPassed,
		Code:            artifact.ArtifactRef{ID: "project-code", Kind: artifact.KindSourceCode, Version: "1", URI: "/workspace/project"},
		EffectiveConfig: CoverageEffectiveConfig{PackageScope: "./...", MinimumStatementCoverage: 80},
		Toolchain: Toolchain{
			Tool: "go test", LauncherVersion: "go version go1.25.0 darwin/arm64", FinalVersion: "go1.25.0",
			GOROOT: "/go", GOOS: "darwin", GOARCH: "arm64", CGOEnabled: "1",
		},
		Metrics:  CoverageMetrics{StatementCoverage: CoverageMetric{Available: true, Value: &coverage, Unit: "percent"}},
		Findings: []CoverageFinding{},
		Logs: LogReferences{
			Stdout: artifact.ArtifactRef{ID: "stdout", Kind: artifact.KindLog, URI: "/data/stdout.log"},
			Stderr: artifact.ArtifactRef{ID: "stderr", Kind: artifact.KindLog, URI: "/data/stderr.log"},
		},
		StartedAt: started, FinishedAt: started.Add(time.Second),
	}
}
