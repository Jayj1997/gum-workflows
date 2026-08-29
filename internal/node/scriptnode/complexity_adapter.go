package scriptnode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

const complexityAnalyzerAPIVersion = "goComplexityAnalyzer/v1"

// ComplexityPolicy is the validated per-instance threshold and source selection policy.
type ComplexityPolicy struct {
	MaximumCyclomaticComplexity int
	IncludeTests                bool
	ExcludeGeneratedFiles       bool
}

type complexityAnalyzerOutput struct {
	APIVersion   string                       `json:"apiVersion"`
	Functions    []complexityAnalyzerFunction `json:"functions"`
	SyntaxErrors []complexityAnalyzerError    `json:"syntaxErrors"`
}

type complexityAnalyzerFunction struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Name       string `json:"name"`
	Complexity int    `json:"complexity"`
	Test       bool   `json:"test"`
	Generated  bool   `json:"generated"`
	Vendor     bool   `json:"vendor"`
}

type complexityAnalyzerError struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Message   string `json:"message"`
	Test      bool   `json:"test"`
	Generated bool   `json:"generated"`
	Vendor    bool   `json:"vendor"`
}

// AdaptComplexityResult validates analyzer evidence and applies the effective policy.
func AdaptComplexityResult(record ExecutionRecord, policy ComplexityPolicy) (ComplexityResult, error) {
	if policy.MaximumCyclomaticComplexity < 1 {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: maximum cyclomatic complexity must be positive"))
	}
	exitData, err := readRequired(filepath.Join(record.ToolOutputDir, "analyzer-exit.txt"))
	if err != nil {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: %w", err))
	}
	reportedExit, err := strconv.Atoi(strings.TrimSpace(string(exitData)))
	if err != nil || reportedExit != record.ExitCode || record.ExitCode != 0 {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: analyzer did not complete successfully with matching exit evidence"))
	}
	data, err := readRequired(filepath.Join(record.ToolOutputDir, "complexity.json"))
	if err != nil {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: %w", err))
	}
	var output complexityAnalyzerOutput
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: decode complexity.json: %w", err))
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: complexity.json contains more than one JSON value"))
	}
	if output.APIVersion != complexityAnalyzerAPIVersion || output.Functions == nil || output.SyntaxErrors == nil {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: complexity.json has an invalid contract"))
	}
	toolchain, err := readToolchain(record.ToolOutputDir)
	if err != nil {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: %w", err))
	}
	toolchain.Tool = complexityTool
	result := ComplexityResult{
		APIVersion: qualityCheckResultAPIVersion, Check: ComplexityCheck, Code: record.Code,
		EffectiveConfig: ComplexityEffectiveConfig{PackageScope: "./...", MaximumCyclomaticComplexity: policy.MaximumCyclomaticComplexity, IncludeTests: policy.IncludeTests, ExcludeGeneratedFiles: policy.ExcludeGeneratedFiles, ExcludeVendor: true},
		Toolchain:       toolchain, Findings: []ComplexityFinding{},
		Logs:      LogReferences{Stdout: artifact.ArtifactRef{ID: "stdout", Kind: artifact.KindLog, URI: record.StdoutPath}, Stderr: artifact.ArtifactRef{ID: "stderr", Kind: artifact.KindLog, URI: record.StderrPath}},
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt,
	}
	selected := make([]complexityAnalyzerFunction, 0, len(output.Functions))
	for _, function := range output.Functions {
		if invalidComplexityLocation(function.File, function.Line) || strings.TrimSpace(function.Name) == "" || function.Complexity < 1 {
			return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: complexity.json contains an invalid function"))
		}
		if function.Vendor || (!policy.IncludeTests && function.Test) || (policy.ExcludeGeneratedFiles && function.Generated) {
			continue
		}
		selected = append(selected, function)
	}
	for _, syntaxError := range output.SyntaxErrors {
		if invalidComplexityLocation(syntaxError.File, syntaxError.Line) || strings.TrimSpace(syntaxError.Message) == "" {
			return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: complexity.json contains an invalid syntax error"))
		}
		if syntaxError.Vendor || (!policy.IncludeTests && syntaxError.Test) || (policy.ExcludeGeneratedFiles && syntaxError.Generated) {
			continue
		}
		result.Findings = append(result.Findings, ComplexityFinding{Tool: complexityTool, Kind: complexitySyntaxFinding, File: syntaxError.File, Line: syntaxError.Line, Message: syntaxError.Message})
	}
	maxComplexity, over := 0, 0
	for _, function := range selected {
		if function.Complexity > maxComplexity {
			maxComplexity = function.Complexity
		}
		if function.Complexity > policy.MaximumCyclomaticComplexity {
			over++
			value := function.Complexity
			result.Findings = append(result.Findings, ComplexityFinding{Tool: complexityTool, Kind: complexityThresholdFinding, File: function.File, Line: function.Line, Function: function.Name, Complexity: &value, Message: fmt.Sprintf("function %s has cyclomatic complexity %d, exceeding maximum %d", function.Name, function.Complexity, policy.MaximumCyclomaticComplexity)})
		}
	}
	count := len(selected)
	result.Metrics.FunctionsAnalyzed = availableComplexityMetric(count)
	result.Metrics.FunctionsOverThreshold = availableComplexityMetric(over)
	if count == 0 {
		result.Metrics.MaxCyclomaticComplexity = ComplexityMetric{Reason: "no analyzable functions"}
	} else {
		result.Metrics.MaxCyclomaticComplexity = availableComplexityMetric(maxComplexity)
	}
	switch {
	case len(result.Findings) > 0:
		result.Verdict = VerdictFailed
	case count == 0:
		result.Verdict = VerdictNotApplicable
	default:
		result.Verdict = VerdictPassed
	}
	if err := result.Validate(); err != nil {
		return ComplexityResult{}, node.Structural(fmt.Errorf("complexity result adapter: %w", err))
	}
	return result, nil
}

func invalidComplexityLocation(file string, line int) bool {
	return strings.TrimSpace(file) == "" || filepath.IsAbs(file) || strings.HasPrefix(filepath.Clean(file), "..") || line < 1
}

func availableComplexityMetric(value int) ComplexityMetric {
	return ComplexityMetric{Available: true, Value: &value, Unit: "count"}
}
