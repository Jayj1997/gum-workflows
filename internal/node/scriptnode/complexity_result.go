package scriptnode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

const (
	// ComplexityCheck identifies the go-complexity-check result shape.
	ComplexityCheck            = "complexity"
	complexityTool             = "go ast"
	complexityThresholdFinding = "complexity-threshold"
	complexitySyntaxFinding    = "syntax-error"
)

// ComplexityResult is the strict qualityCheckResult/v1 payload for cyclomatic complexity.
type ComplexityResult struct {
	APIVersion      string                    `json:"apiVersion"`
	Check           string                    `json:"check"`
	Verdict         Verdict                   `json:"verdict"`
	Code            artifact.ArtifactRef      `json:"code"`
	EffectiveConfig ComplexityEffectiveConfig `json:"effectiveConfig"`
	Toolchain       Toolchain                 `json:"toolchain"`
	Metrics         ComplexityMetrics         `json:"metrics"`
	Findings        []ComplexityFinding       `json:"findings"`
	Logs            LogReferences             `json:"logs"`
	StartedAt       time.Time                 `json:"startedAt"`
	FinishedAt      time.Time                 `json:"finishedAt"`
}

// ComplexityEffectiveConfig records the effective analyzer selection and threshold policy.
type ComplexityEffectiveConfig struct {
	PackageScope                string `json:"packageScope"`
	MaximumCyclomaticComplexity int    `json:"maximumCyclomaticComplexity"`
	IncludeTests                bool   `json:"includeTests"`
	ExcludeGeneratedFiles       bool   `json:"excludeGeneratedFiles"`
	ExcludeVendor               bool   `json:"excludeVendor"`
}

// ComplexityMetrics is the fixed metric set for go-complexity-check.
type ComplexityMetrics struct {
	MaxCyclomaticComplexity ComplexityMetric `json:"maxCyclomaticComplexity"`
	FunctionsAnalyzed       ComplexityMetric `json:"functionsAnalyzed"`
	FunctionsOverThreshold  ComplexityMetric `json:"functionsOverThreshold"`
}

// ComplexityMetric represents either a measured integer or an unavailable reason.
type ComplexityMetric struct {
	Available bool   `json:"available"`
	Value     *int   `json:"value,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ComplexityFinding locates either an over-threshold function or a Go syntax error.
type ComplexityFinding struct {
	Tool       string `json:"tool"`
	Kind       string `json:"kind"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Function   string `json:"function,omitempty"`
	Complexity *int   `json:"complexity,omitempty"`
	Message    string `json:"message"`
}

// Validate enforces the closed complexity-result contract and cross-field invariants.
func (r ComplexityResult) Validate() error {
	if r.APIVersion != qualityCheckResultAPIVersion || r.Check != ComplexityCheck {
		return fmt.Errorf("result must use %s and check %q", qualityCheckResultAPIVersion, ComplexityCheck)
	}
	switch r.Verdict {
	case VerdictPassed, VerdictFailed, VerdictNotApplicable:
	default:
		return fmt.Errorf("verdict %q is invalid", r.Verdict)
	}
	if err := r.Code.Validate(); err != nil || r.Code.Kind != artifact.KindSourceCode || r.Code.Version == "" {
		return fmt.Errorf("code must be a valid versioned SourceCode reference")
	}
	if r.EffectiveConfig.PackageScope != "./..." || r.EffectiveConfig.MaximumCyclomaticComplexity < 1 || !r.EffectiveConfig.ExcludeVendor {
		return fmt.Errorf("effectiveConfig must contain packageScope ./..., a positive maximum, and excludeVendor true")
	}
	if r.Toolchain.Tool != complexityTool || strings.TrimSpace(r.Toolchain.LauncherVersion) == "" ||
		strings.TrimSpace(r.Toolchain.FinalVersion) == "" || strings.TrimSpace(r.Toolchain.GOROOT) == "" ||
		strings.TrimSpace(r.Toolchain.GOOS) == "" || strings.TrimSpace(r.Toolchain.GOARCH) == "" ||
		strings.TrimSpace(r.Toolchain.CGOEnabled) == "" {
		return fmt.Errorf("toolchain must contain go ast and complete Go environment details")
	}
	metrics := []struct {
		name   string
		metric ComplexityMetric
	}{
		{"maxCyclomaticComplexity", r.Metrics.MaxCyclomaticComplexity},
		{"functionsAnalyzed", r.Metrics.FunctionsAnalyzed},
		{"functionsOverThreshold", r.Metrics.FunctionsOverThreshold},
	}
	for _, item := range metrics {
		if item.metric.Available {
			if item.metric.Value == nil || *item.metric.Value < 0 || item.metric.Unit != "count" || item.metric.Reason != "" {
				return fmt.Errorf("available %s must contain a non-negative count and no reason", item.name)
			}
		} else if item.metric.Value != nil || item.metric.Unit != "" || strings.TrimSpace(item.metric.Reason) == "" {
			return fmt.Errorf("unavailable %s must contain only a reason", item.name)
		}
	}
	maxMetric := r.Metrics.MaxCyclomaticComplexity
	analyzedMetric := r.Metrics.FunctionsAnalyzed
	overMetric := r.Metrics.FunctionsOverThreshold
	if !analyzedMetric.Available || !overMetric.Available || *overMetric.Value > *analyzedMetric.Value {
		return fmt.Errorf("function count metrics must be available and ordered")
	}
	if *analyzedMetric.Value == 0 {
		if maxMetric.Available {
			return fmt.Errorf("maxCyclomaticComplexity must be unavailable when no functions were analyzed")
		}
	} else if !maxMetric.Available || *maxMetric.Value < 1 {
		return fmt.Errorf("maxCyclomaticComplexity must be available and positive when functions were analyzed")
	}
	for i, finding := range r.Findings {
		if finding.Tool != complexityTool || (finding.Kind != complexityThresholdFinding && finding.Kind != complexitySyntaxFinding) ||
			invalidComplexityLocation(finding.File, finding.Line) || strings.TrimSpace(finding.Message) == "" {
			return fmt.Errorf("findings[%d] must be a located go ast complexity or syntax finding", i)
		}
		if finding.Kind == complexityThresholdFinding && (finding.Function == "" || finding.Complexity == nil || *finding.Complexity <= r.EffectiveConfig.MaximumCyclomaticComplexity) {
			return fmt.Errorf("findings[%d] must identify an over-threshold function", i)
		}
	}
	switch r.Verdict {
	case VerdictPassed:
		if len(r.Findings) != 0 || *analyzedMetric.Value == 0 || *overMetric.Value != 0 || *maxMetric.Value > r.EffectiveConfig.MaximumCyclomaticComplexity {
			return fmt.Errorf("passed result must report analyzed functions at or below the maximum and no findings")
		}
	case VerdictFailed:
		if len(r.Findings) == 0 {
			return fmt.Errorf("failed result must contain findings")
		}
	case VerdictNotApplicable:
		if len(r.Findings) != 0 || *analyzedMetric.Value != 0 || *overMetric.Value != 0 {
			return fmt.Errorf("not-applicable result must report zero analyzed functions and no findings")
		}
	}
	for name, ref := range map[string]artifact.ArtifactRef{"stdout": r.Logs.Stdout, "stderr": r.Logs.Stderr} {
		if err := ref.Validate(); err != nil || ref.Kind != artifact.KindLog {
			return fmt.Errorf("logs.%s must be a valid Log reference", name)
		}
	}
	if r.StartedAt.IsZero() || r.FinishedAt.IsZero() || r.FinishedAt.Before(r.StartedAt) {
		return fmt.Errorf("startedAt and finishedAt must describe an ordered interval")
	}
	return nil
}

// ToolchainDiagnostics exposes the complexity toolchain facts to Node Run history.
func (r ComplexityResult) ToolchainDiagnostics() *node.ToolchainDiagnostics {
	return &node.ToolchainDiagnostics{
		Tool: r.Toolchain.Tool, LauncherVersion: r.Toolchain.LauncherVersion,
		FinalVersion: r.Toolchain.FinalVersion, GOROOT: r.Toolchain.GOROOT,
		GOOS: r.Toolchain.GOOS, GOARCH: r.Toolchain.GOARCH, CGOEnabled: r.Toolchain.CGOEnabled,
	}
}

// DecodeComplexityResult strictly decodes and validates a qualityCheckResult/v1 payload.
func DecodeComplexityResult(data []byte) (ComplexityResult, error) {
	var result ComplexityResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return ComplexityResult{}, fmt.Errorf("decode complexity result: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ComplexityResult{}, fmt.Errorf("decode complexity result: expected one JSON value")
	}
	if err := result.Validate(); err != nil {
		return ComplexityResult{}, fmt.Errorf("validate complexity result: %w", err)
	}
	return result, nil
}
