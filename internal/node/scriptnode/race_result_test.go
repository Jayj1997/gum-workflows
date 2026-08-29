package scriptnode

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

func TestRaceResultValidationAcceptsCompletePassedResult(t *testing.T) {
	result := validRaceResult()
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRaceResult(data)
	if err != nil {
		t.Fatalf("DecodeRaceResult() unexpected error: %v", err)
	}
	metric := decoded.Metrics.RacesDetected
	if decoded.Verdict != VerdictPassed || metric.Value == nil || *metric.Value != 0 {
		t.Errorf("decoded result = %+v", decoded)
	}
}

func TestRaceResultValidationRejectsContradictionsAndUnknownFields(t *testing.T) {
	tests := map[string]func(*RaceResult){
		"unknown verdict":       func(result *RaceResult) { result.Verdict = "green" },
		"wrong code kind":       func(result *RaceResult) { result.Code.Kind = artifact.KindOpenAPI },
		"passed with races":     func(result *RaceResult) { *result.Metrics.RacesDetected.Value = 1 },
		"available with reason": func(result *RaceResult) { result.Metrics.RacesDetected.Reason = "contradiction" },
		"failed without finding": func(result *RaceResult) {
			result.Verdict = VerdictFailed
		},
		"not applicable with value": func(result *RaceResult) {
			result.Verdict = VerdictNotApplicable
			result.Metrics.RacesDetected = RaceMetric{Available: false, Reason: "no Go packages"}
			result.Findings = nil
			value := 0
			result.Metrics.RacesDetected.Value = &value
		},
		"invalid time": func(result *RaceResult) { result.FinishedAt = result.StartedAt.Add(-time.Second) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := validRaceResult()
			mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("Validate() = nil error, want rejection")
			}
		})
	}

	data, _ := json.Marshal(validRaceResult())
	withUnknown := strings.Replace(string(data), `"verdict":"passed"`, `"verdict":"passed","extra":true`, 1)
	if _, err := DecodeRaceResult([]byte(withUnknown)); err == nil {
		t.Fatal("DecodeRaceResult(unknown field) = nil error")
	}
}

func validRaceResult() RaceResult {
	started := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	races := 0
	return RaceResult{
		APIVersion: qualityCheckResultAPIVersion, Check: RaceCheck, Verdict: VerdictPassed,
		Code:            artifact.ArtifactRef{ID: "project-code", Kind: artifact.KindSourceCode, Version: "1", URI: "/workspace/project"},
		EffectiveConfig: RaceEffectiveConfig{PackageScope: "./..."},
		Toolchain: Toolchain{
			Tool: "go test -race", LauncherVersion: "go version go1.25.0 darwin/arm64", FinalVersion: "go1.25.0",
			GOROOT: "/go", GOOS: "darwin", GOARCH: "arm64", CGOEnabled: "1", CCompiler: "clang",
		},
		Metrics:  RaceMetrics{RacesDetected: RaceMetric{Available: true, Value: &races, Unit: "count"}},
		Findings: []RaceFinding{},
		Logs: LogReferences{
			Stdout: artifact.ArtifactRef{ID: "stdout", Kind: artifact.KindLog, URI: "/data/stdout.log"},
			Stderr: artifact.ArtifactRef{ID: "stderr", Kind: artifact.KindLog, URI: "/data/stderr.log"},
		},
		StartedAt: started, FinishedAt: started.Add(time.Second),
	}
}
