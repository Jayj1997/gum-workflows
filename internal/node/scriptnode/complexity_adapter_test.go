package scriptnode

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/node"
)

func TestComplexityAdapterAppliesThresholdAtFunctionBoundary(t *testing.T) {
	tests := []struct {
		name    string
		maximum int
		verdict Verdict
		over    int
	}{
		{name: "below", maximum: 4, verdict: VerdictFailed, over: 1},
		{name: "equal", maximum: 5, verdict: VerdictPassed},
		{name: "above", maximum: 6, verdict: VerdictPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := complexityExecutionFixture(t, complexityAnalyzerOutput{Functions: []complexityAnalyzerFunction{
				{File: "small.go", Line: 3, Name: "small", Complexity: 2},
				{File: "large.go", Line: 8, Name: "large", Complexity: 5},
			}, SyntaxErrors: []complexityAnalyzerError{}})
			result, err := AdaptComplexityResult(record, ComplexityPolicy{MaximumCyclomaticComplexity: tt.maximum, ExcludeGeneratedFiles: true})
			if err != nil {
				t.Fatalf("AdaptComplexityResult() unexpected error: %v", err)
			}
			if result.Verdict != tt.verdict || metricValue(result.Metrics.MaxCyclomaticComplexity) != 5 ||
				metricValue(result.Metrics.FunctionsAnalyzed) != 2 || metricValue(result.Metrics.FunctionsOverThreshold) != tt.over {
				t.Fatalf("result = %+v", result)
			}
			if tt.over == 1 && (len(result.Findings) != 1 || result.Findings[0].File != "large.go" || result.Findings[0].Line != 8) {
				t.Fatalf("findings = %+v, want located large function", result.Findings)
			}
		})
	}
}

func TestComplexityAdapterAppliesFileExclusionsIndependently(t *testing.T) {
	functions := []complexityAnalyzerFunction{
		{File: "app.go", Line: 1, Name: "app", Complexity: 2},
		{File: "app_test.go", Line: 1, Name: "test", Complexity: 30, Test: true},
		{File: "generated.go", Line: 2, Name: "generated", Complexity: 30, Generated: true},
		{File: "vendor/lib.go", Line: 1, Name: "vendor", Complexity: 30, Vendor: true},
	}
	tests := []struct {
		name   string
		policy ComplexityPolicy
		count  int
		over   int
	}{
		{name: "defaults exclude all policy files", policy: ComplexityPolicy{MaximumCyclomaticComplexity: 15, ExcludeGeneratedFiles: true}, count: 1},
		{name: "tests included", policy: ComplexityPolicy{MaximumCyclomaticComplexity: 15, IncludeTests: true, ExcludeGeneratedFiles: true}, count: 2, over: 1},
		{name: "generated included", policy: ComplexityPolicy{MaximumCyclomaticComplexity: 15}, count: 2, over: 1},
		{name: "vendor always excluded", policy: ComplexityPolicy{MaximumCyclomaticComplexity: 100, IncludeTests: true}, count: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AdaptComplexityResult(complexityExecutionFixture(t, complexityAnalyzerOutput{Functions: functions, SyntaxErrors: []complexityAnalyzerError{}}), tt.policy)
			if err != nil {
				t.Fatal(err)
			}
			if metricValue(result.Metrics.FunctionsAnalyzed) != tt.count || metricValue(result.Metrics.FunctionsOverThreshold) != tt.over {
				t.Fatalf("metrics = %+v", result.Metrics)
			}
		})
	}
}

func TestComplexityAdapterReportsNoFunctionsAndSyntaxErrors(t *testing.T) {
	t.Run("no functions", func(t *testing.T) {
		result, err := AdaptComplexityResult(complexityExecutionFixture(t, complexityAnalyzerOutput{Functions: []complexityAnalyzerFunction{}, SyntaxErrors: []complexityAnalyzerError{}}), ComplexityPolicy{MaximumCyclomaticComplexity: 15, ExcludeGeneratedFiles: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Verdict != VerdictNotApplicable || result.Metrics.MaxCyclomaticComplexity.Available || metricValue(result.Metrics.FunctionsAnalyzed) != 0 {
			t.Fatalf("result = %+v", result)
		}
	})
	t.Run("syntax error", func(t *testing.T) {
		result, err := AdaptComplexityResult(complexityExecutionFixture(t, complexityAnalyzerOutput{Functions: []complexityAnalyzerFunction{}, SyntaxErrors: []complexityAnalyzerError{{File: "broken.go", Line: 4, Message: "expected }"}}}), ComplexityPolicy{MaximumCyclomaticComplexity: 15, ExcludeGeneratedFiles: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Verdict != VerdictFailed || len(result.Findings) != 1 || result.Findings[0].Kind != "syntax-error" {
			t.Fatalf("result = %+v", result)
		}
	})
}

func TestComplexityAdapterRejectsDamagedAnalyzerEvidence(t *testing.T) {
	tests := []struct {
		name, body string
		exit       int
	}{
		{name: "invalid JSON", body: "not-json", exit: 0},
		{name: "wrong version", body: `{"apiVersion":"wrong","functions":[],"syntaxErrors":[]}`, exit: 0},
		{name: "analyzer failure", body: `{"apiVersion":"goComplexityAnalyzer/v1","functions":[],"syntaxErrors":[]}`, exit: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := staticExecutionFixture(t, "", "", "", tt.exit)
			writeFixture(t, filepath.Join(record.ToolOutputDir, "complexity.json"), tt.body)
			writeFixture(t, filepath.Join(record.ToolOutputDir, "analyzer-exit.txt"), string(rune('0'+tt.exit))+"\n")
			_, err := AdaptComplexityResult(record, ComplexityPolicy{MaximumCyclomaticComplexity: 15, ExcludeGeneratedFiles: true})
			if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural {
				t.Fatalf("error = %v, want Structural Error", err)
			}
		})
	}
}

func complexityExecutionFixture(t *testing.T, output complexityAnalyzerOutput) ExecutionRecord {
	t.Helper()
	record := staticExecutionFixture(t, "", "", "", 0)
	output.APIVersion = "goComplexityAnalyzer/v1"
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(record.ToolOutputDir, "complexity.json"), string(data))
	writeFixture(t, filepath.Join(record.ToolOutputDir, "analyzer-exit.txt"), "0\n")
	return record
}

func metricValue(metric ComplexityMetric) int {
	if metric.Value == nil {
		return -1
	}
	return *metric.Value
}

func TestComplexityResultDecoderRejectsUnknownFields(t *testing.T) {
	result, err := AdaptComplexityResult(complexityExecutionFixture(t, complexityAnalyzerOutput{Functions: []complexityAnalyzerFunction{{File: "app.go", Line: 1, Name: "app", Complexity: 1}}, SyntaxErrors: []complexityAnalyzerError{}}), ComplexityPolicy{MaximumCyclomaticComplexity: 15, ExcludeGeneratedFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result)
	data = []byte(strings.Replace(string(data), `"verdict":"passed"`, `"verdict":"passed","extra":true`, 1))
	if _, err := DecodeComplexityResult(data); err == nil {
		t.Fatal("DecodeComplexityResult() = nil error, want unknown field rejection")
	}
}

func TestComplexityResultValidationRejectsContradictions(t *testing.T) {
	base, err := AdaptComplexityResult(complexityExecutionFixture(t, complexityAnalyzerOutput{Functions: []complexityAnalyzerFunction{{File: "app.go", Line: 1, Name: "app", Complexity: 1}}, SyntaxErrors: []complexityAnalyzerError{}}), ComplexityPolicy{MaximumCyclomaticComplexity: 15, ExcludeGeneratedFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*ComplexityResult){
		"passed with no functions": func(result *ComplexityResult) {
			zero := 0
			result.Metrics.FunctionsAnalyzed.Value = &zero
			result.Metrics.MaxCyclomaticComplexity = ComplexityMetric{Reason: "none"}
		},
		"over count exceeds analyzed": func(result *ComplexityResult) { two := 2; result.Metrics.FunctionsOverThreshold.Value = &two },
		"vendor not excluded":         func(result *ComplexityResult) { result.EffectiveConfig.ExcludeVendor = false },
		"invalid finding location": func(result *ComplexityResult) {
			result.Verdict = VerdictFailed
			result.Findings = []ComplexityFinding{{Tool: "go ast", Kind: "syntax-error", File: "../outside.go", Line: 1, Message: "bad"}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := base
			mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("Validate() = nil error, want rejection")
			}
		})
	}
}
