package scriptnode

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

func TestStaticResultValidationAcceptsCompletePassedResult(t *testing.T) {
	result := validStaticResult()
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStaticResult(data)
	if err != nil {
		t.Fatalf("DecodeStaticResult() unexpected error: %v", err)
	}
	if decoded.Verdict != VerdictPassed || decoded.Toolchain.Tool != "go vet" {
		t.Errorf("decoded result = %+v", decoded)
	}
}

func TestStaticResultValidationRejectsContradictionsAndUnknownFields(t *testing.T) {
	tests := map[string]func(*StaticResult){
		"unknown verdict": func(result *StaticResult) { result.Verdict = "green" },
		"wrong code kind": func(result *StaticResult) { result.Code.Kind = artifact.KindOpenAPI },
		"count mismatch": func(result *StaticResult) {
			result.Verdict = VerdictFailed
			result.FindingsCount = 2
			result.Findings = []StaticFinding{{Tool: "go vet", Message: "one"}}
		},
		"passed with findings": func(result *StaticResult) {
			result.FindingsCount = 1
			result.Findings = []StaticFinding{{Tool: "go vet", Message: "bad"}}
		},
		"invalid time": func(result *StaticResult) { result.FinishedAt = result.StartedAt.Add(-time.Second) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := validStaticResult()
			mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("Validate() = nil error, want rejection")
			}
		})
	}

	data, _ := json.Marshal(validStaticResult())
	withUnknown := strings.Replace(string(data), `"verdict":"passed"`, `"verdict":"passed","extra":true`, 1)
	if _, err := DecodeStaticResult([]byte(withUnknown)); err == nil {
		t.Fatal("DecodeStaticResult(unknown field) = nil error")
	}
}

func validStaticResult() StaticResult {
	started := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	return StaticResult{
		APIVersion: qualityCheckResultAPIVersion,
		Check:      StaticAnalysisCheck,
		Verdict:    VerdictPassed,
		Code: artifact.ArtifactRef{
			ID: "project-code", Kind: artifact.KindSourceCode, Version: "1", URI: "/workspace/project",
		},
		EffectiveConfig: StaticEffectiveConfig{PackageScope: "./..."},
		Toolchain: Toolchain{
			Tool: "go vet", LauncherVersion: "go version go1.25.0 darwin/arm64",
			FinalVersion: "go1.25.0", GOROOT: "/go", GOOS: "darwin", GOARCH: "arm64", CGOEnabled: "1",
		},
		FindingsCount: 0,
		Findings:      []StaticFinding{},
		Logs: LogReferences{
			Stdout: artifact.ArtifactRef{ID: "stdout", Kind: artifact.KindLog, URI: "/data/stdout.log"},
			Stderr: artifact.ArtifactRef{ID: "stderr", Kind: artifact.KindLog, URI: "/data/stderr.log"},
		},
		StartedAt: started, FinishedAt: started.Add(time.Second),
	}
}
