package scriptnode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/node"
)

func TestCoverageAdapterComputesStatementCoverageAndThresholdVerdict(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		verdict   Verdict
	}{
		{name: "below threshold", threshold: 75.1, verdict: VerdictFailed},
		{name: "equal threshold", threshold: 75, verdict: VerdictPassed},
		{name: "above threshold", threshold: 70, verdict: VerdictPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := coverageExecutionFixture(t, 0, "mode: set\napp.go:1.1,2.2 3 1\napp.go:3.1,4.2 1 0\n", "")
			result, err := AdaptCoverageResult(record, tt.threshold)
			if err != nil {
				t.Fatalf("AdaptCoverageResult() unexpected error: %v", err)
			}
			metric := result.Metrics.StatementCoverage
			if result.Verdict != tt.verdict || !metric.Available || metric.Value == nil || *metric.Value != 75 {
				t.Fatalf("result = %+v, want %s with 75%% coverage", result, tt.verdict)
			}
			if result.EffectiveConfig.MinimumStatementCoverage != tt.threshold {
				t.Errorf("effective threshold = %v, want %v", result.EffectiveConfig.MinimumStatementCoverage, tt.threshold)
			}
			if err := result.Validate(); err != nil {
				t.Errorf("result contract: %v", err)
			}
		})
	}
}

func TestCoverageAdapterReportsNoStatementsAsNotApplicable(t *testing.T) {
	result, err := AdaptCoverageResult(coverageExecutionFixture(t, 0, "mode: set\n", ""), 80)
	if err != nil {
		t.Fatalf("AdaptCoverageResult() unexpected error: %v", err)
	}
	metric := result.Metrics.StatementCoverage
	if result.Verdict != VerdictNotApplicable || metric.Available || !strings.Contains(metric.Reason, "statement") {
		t.Fatalf("result = %+v, want not-applicable unavailable statement metric", result)
	}
}

func TestCoverageAdapterReportsTestFailureWithoutFabricatingZeroCoverage(t *testing.T) {
	record := coverageExecutionFixture(t, 1, "", "package example.com/app failed\n")
	result, err := AdaptCoverageResult(record, 80)
	if err != nil {
		t.Fatalf("AdaptCoverageResult() unexpected error: %v", err)
	}
	metric := result.Metrics.StatementCoverage
	if result.Verdict != VerdictFailed || metric.Available || metric.Value != nil || metric.Reason == "" {
		t.Fatalf("result = %+v, want failed with unavailable metric and reason", result)
	}
	if len(result.Findings) != 1 || !strings.Contains(result.Findings[0].Message, "failed") {
		t.Fatalf("findings = %+v, want test failure reason", result.Findings)
	}
}

func TestCoverageAdapterUsesValidProfileDespiteToolchainDiagnostic(t *testing.T) {
	record := coverageExecutionFixture(t, 0, "mode: atomic\napp.go:1.1,2.2 1 1\n", "go: no such tool \"covdata\"\n")
	result, err := AdaptCoverageResult(record, 100)
	if err != nil {
		t.Fatalf("AdaptCoverageResult() unexpected error: %v", err)
	}
	metric := result.Metrics.StatementCoverage
	if result.Verdict != VerdictPassed || metric.Value == nil || *metric.Value != 100 {
		t.Fatalf("result = %+v, want valid profile to determine verdict", result)
	}
}

func TestCoverageAdapterRejectsMissingOrDamagedSuccessfulProfile(t *testing.T) {
	tests := []struct {
		name, profile string
		removeProfile bool
	}{
		{name: "missing", removeProfile: true},
		{name: "missing mode", profile: "app.go:1.1,2.2 1 1\n"},
		{name: "damaged block", profile: "mode: set\napp.go:not-a-block\n"},
		{name: "negative statements", profile: "mode: set\napp.go:1.1,2.2 -1 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := coverageExecutionFixture(t, 0, tt.profile, "")
			if tt.removeProfile {
				if err := os.Remove(filepath.Join(record.ToolOutputDir, "coverage.out")); err != nil {
					t.Fatal(err)
				}
			}
			_, err := AdaptCoverageResult(record, 80)
			if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural {
				t.Fatalf("AdaptCoverageResult() error = %v, want Structural Error", err)
			}
		})
	}
}

func coverageExecutionFixture(t *testing.T, exitCode int, profile, stderr string) ExecutionRecord {
	t.Helper()
	record := staticExecutionFixture(t, "", "", stderr, exitCode)
	writeFixture(t, filepath.Join(record.ToolOutputDir, "coverage.out"), profile)
	writeFixture(t, filepath.Join(record.ToolOutputDir, "test.json"), "")
	writeFixture(t, filepath.Join(record.ToolOutputDir, "test-exit.txt"), fmt.Sprintf("%d\n", exitCode))
	return record
}
