package scriptnode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

const (
	// CoverageCheck identifies the go-coverage-check result shape.
	CoverageCheck = "coverage"
)

// CoverageResult is the strict qualityCheckResult/v1 payload for statement coverage.
type CoverageResult struct {
	APIVersion      string                  `json:"apiVersion"`
	Check           string                  `json:"check"`
	Verdict         Verdict                 `json:"verdict"`
	Code            artifact.ArtifactRef    `json:"code"`
	EffectiveConfig CoverageEffectiveConfig `json:"effectiveConfig"`
	Toolchain       Toolchain               `json:"toolchain"`
	Metrics         CoverageMetrics         `json:"metrics"`
	Findings        []CoverageFinding       `json:"findings"`
	Logs            LogReferences           `json:"logs"`
	StartedAt       time.Time               `json:"startedAt"`
	FinishedAt      time.Time               `json:"finishedAt"`
}

// CoverageEffectiveConfig records the fixed scope and effective threshold.
type CoverageEffectiveConfig struct {
	PackageScope             string  `json:"packageScope"`
	MinimumStatementCoverage float64 `json:"minimumStatementCoverage"`
}

// CoverageMetrics is the fixed metric set for go-coverage-check.
type CoverageMetrics struct {
	StatementCoverage CoverageMetric `json:"statementCoverage"`
}

// CoverageMetric represents either a measured percentage or an unavailable reason.
type CoverageMetric struct {
	Available bool     `json:"available"`
	Value     *float64 `json:"value,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

// CoverageFinding explains a failed coverage check.
type CoverageFinding struct {
	Tool    string `json:"tool"`
	Message string `json:"message"`
}

// Validate enforces the closed coverage-result contract and cross-field invariants.
func (r CoverageResult) Validate() error {
	if r.APIVersion != qualityCheckResultAPIVersion {
		return fmt.Errorf("apiVersion must be %q", qualityCheckResultAPIVersion)
	}
	if r.Check != CoverageCheck {
		return fmt.Errorf("check must be %q", CoverageCheck)
	}
	switch r.Verdict {
	case VerdictPassed, VerdictFailed, VerdictNotApplicable:
	default:
		return fmt.Errorf("verdict %q is invalid", r.Verdict)
	}
	if err := r.Code.Validate(); err != nil {
		return fmt.Errorf("code: %w", err)
	}
	if r.Code.Kind != artifact.KindSourceCode || r.Code.Version == "" {
		return fmt.Errorf("code must be a versioned SourceCode reference")
	}
	threshold := r.EffectiveConfig.MinimumStatementCoverage
	if r.EffectiveConfig.PackageScope != "./..." || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 100 {
		return fmt.Errorf("effectiveConfig must contain packageScope ./... and a threshold between 0 and 100")
	}
	if r.Toolchain.Tool != "go test" || strings.TrimSpace(r.Toolchain.LauncherVersion) == "" ||
		strings.TrimSpace(r.Toolchain.FinalVersion) == "" || strings.TrimSpace(r.Toolchain.GOROOT) == "" ||
		strings.TrimSpace(r.Toolchain.GOOS) == "" || strings.TrimSpace(r.Toolchain.GOARCH) == "" ||
		strings.TrimSpace(r.Toolchain.CGOEnabled) == "" {
		return fmt.Errorf("toolchain must contain go test and complete Go environment details")
	}
	metric := r.Metrics.StatementCoverage
	if metric.Available {
		if metric.Value == nil || metric.Unit != "percent" || metric.Reason != "" ||
			math.IsNaN(*metric.Value) || math.IsInf(*metric.Value, 0) ||
			*metric.Value < 0 || *metric.Value > 100 {
			return fmt.Errorf("available statementCoverage must contain a percentage value and no reason")
		}
	} else if metric.Value != nil || metric.Unit != "" || strings.TrimSpace(metric.Reason) == "" {
		return fmt.Errorf("unavailable statementCoverage must contain only a reason")
	}
	for i, finding := range r.Findings {
		if finding.Tool != "go test" || strings.TrimSpace(finding.Message) == "" {
			return fmt.Errorf("findings[%d] must identify go test and a message", i)
		}
	}
	switch r.Verdict {
	case VerdictPassed:
		if !metric.Available || *metric.Value < threshold || len(r.Findings) != 0 {
			return fmt.Errorf("passed result must contain coverage at or above threshold and no findings")
		}
	case VerdictFailed:
		if len(r.Findings) == 0 {
			return fmt.Errorf("failed result must contain a finding")
		}
		if metric.Available && *metric.Value >= threshold {
			return fmt.Errorf("failed measured result must be below threshold")
		}
	case VerdictNotApplicable:
		if metric.Available || len(r.Findings) != 0 {
			return fmt.Errorf("not-applicable result must contain an unavailable metric and no findings")
		}
	}
	for name, ref := range map[string]artifact.ArtifactRef{"stdout": r.Logs.Stdout, "stderr": r.Logs.Stderr} {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("logs.%s: %w", name, err)
		}
		if ref.Kind != artifact.KindLog {
			return fmt.Errorf("logs.%s must be a Log reference", name)
		}
	}
	if r.StartedAt.IsZero() || r.FinishedAt.IsZero() || r.FinishedAt.Before(r.StartedAt) {
		return fmt.Errorf("startedAt and finishedAt must describe an ordered interval")
	}
	return nil
}

// ToolchainDiagnostics exposes the coverage toolchain facts to Node Run history.
func (r CoverageResult) ToolchainDiagnostics() *node.ToolchainDiagnostics {
	return &node.ToolchainDiagnostics{
		Tool: r.Toolchain.Tool, LauncherVersion: r.Toolchain.LauncherVersion,
		FinalVersion: r.Toolchain.FinalVersion, GOROOT: r.Toolchain.GOROOT,
		GOOS: r.Toolchain.GOOS, GOARCH: r.Toolchain.GOARCH, CGOEnabled: r.Toolchain.CGOEnabled,
	}
}

// DecodeCoverageResult strictly decodes and validates a qualityCheckResult/v1 payload.
func DecodeCoverageResult(data []byte) (CoverageResult, error) {
	var result CoverageResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return CoverageResult{}, fmt.Errorf("decode coverage result: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return CoverageResult{}, fmt.Errorf("decode coverage result: expected one JSON value")
	}
	if err := result.Validate(); err != nil {
		return CoverageResult{}, fmt.Errorf("validate coverage result: %w", err)
	}
	return result, nil
}
