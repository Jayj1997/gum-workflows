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
	// RaceCheck identifies the go-race-check result shape.
	RaceCheck = "race"
)

// RaceResult is the strict qualityCheckResult/v1 payload for one race-detector invocation.
type RaceResult struct {
	APIVersion      string               `json:"apiVersion"`
	Check           string               `json:"check"`
	Verdict         Verdict              `json:"verdict"`
	Code            artifact.ArtifactRef `json:"code"`
	EffectiveConfig RaceEffectiveConfig  `json:"effectiveConfig"`
	Toolchain       Toolchain            `json:"toolchain"`
	Metrics         RaceMetrics          `json:"metrics"`
	Findings        []RaceFinding        `json:"findings"`
	Logs            LogReferences        `json:"logs"`
	StartedAt       time.Time            `json:"startedAt"`
	FinishedAt      time.Time            `json:"finishedAt"`
}

// RaceEffectiveConfig records the fixed, non-user-overridable package scope.
type RaceEffectiveConfig struct {
	PackageScope string `json:"packageScope"`
}

// RaceMetrics is the fixed metric set for go-race-check.
type RaceMetrics struct {
	RacesDetected RaceMetric `json:"racesDetected"`
}

// RaceMetric represents either an observed count or an unavailable reason.
type RaceMetric struct {
	Available bool   `json:"available"`
	Value     *int   `json:"value,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// RaceFinding records an observed race or a project test/build failure.
type RaceFinding struct {
	Tool    string `json:"tool"`
	Kind    string `json:"kind"`
	Package string `json:"package,omitempty"`
	Message string `json:"message"`
}

// Validate enforces the closed race-result contract and cross-field invariants.
func (r RaceResult) Validate() error {
	if r.APIVersion != qualityCheckResultAPIVersion {
		return fmt.Errorf("apiVersion must be %q", qualityCheckResultAPIVersion)
	}
	if r.Check != RaceCheck {
		return fmt.Errorf("check must be %q", RaceCheck)
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
	if r.EffectiveConfig.PackageScope != "./..." {
		return fmt.Errorf("effectiveConfig.packageScope must be %q", "./...")
	}
	if r.Toolchain.Tool != "go test -race" || strings.TrimSpace(r.Toolchain.LauncherVersion) == "" ||
		strings.TrimSpace(r.Toolchain.FinalVersion) == "" || strings.TrimSpace(r.Toolchain.GOROOT) == "" ||
		strings.TrimSpace(r.Toolchain.GOOS) == "" || strings.TrimSpace(r.Toolchain.GOARCH) == "" ||
		strings.TrimSpace(r.Toolchain.CGOEnabled) == "" || strings.TrimSpace(r.Toolchain.CCompiler) == "" {
		return fmt.Errorf("toolchain must contain go test -race and complete Go environment details")
	}
	metric := r.Metrics.RacesDetected
	if metric.Available {
		if metric.Value == nil || *metric.Value < 0 || metric.Unit != "count" || metric.Reason != "" {
			return fmt.Errorf("available racesDetected must contain a non-negative count and no reason")
		}
	} else if metric.Value != nil || metric.Unit != "" || strings.TrimSpace(metric.Reason) == "" {
		return fmt.Errorf("unavailable racesDetected must contain only a reason")
	}
	for i, finding := range r.Findings {
		if finding.Tool != "go test -race" || strings.TrimSpace(finding.Message) == "" {
			return fmt.Errorf("findings[%d] must identify go test -race and a message", i)
		}
		switch finding.Kind {
		case "race", "test-failure", "compile-failure", "package-failure":
		default:
			return fmt.Errorf("findings[%d].kind %q is invalid", i, finding.Kind)
		}
	}
	switch r.Verdict {
	case VerdictPassed:
		if !metric.Available || *metric.Value != 0 || len(r.Findings) != 0 {
			return fmt.Errorf("passed result must report zero observed races and no findings")
		}
	case VerdictFailed:
		if !metric.Available || len(r.Findings) == 0 {
			return fmt.Errorf("failed result must report an observed count and at least one finding")
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

// ToolchainDiagnostics exposes the result's non-sensitive toolchain facts to Node Run history.
func (r RaceResult) ToolchainDiagnostics() *node.ToolchainDiagnostics {
	return &node.ToolchainDiagnostics{
		Tool: r.Toolchain.Tool, LauncherVersion: r.Toolchain.LauncherVersion,
		FinalVersion: r.Toolchain.FinalVersion, GOROOT: r.Toolchain.GOROOT,
		GOOS: r.Toolchain.GOOS, GOARCH: r.Toolchain.GOARCH, CGOEnabled: r.Toolchain.CGOEnabled,
		CCompiler: r.Toolchain.CCompiler,
	}
}

// DecodeRaceResult strictly decodes and validates a qualityCheckResult/v1 payload.
func DecodeRaceResult(data []byte) (RaceResult, error) {
	var result RaceResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return RaceResult{}, fmt.Errorf("decode race result: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RaceResult{}, fmt.Errorf("decode race result: expected one JSON value")
	}
	if err := result.Validate(); err != nil {
		return RaceResult{}, fmt.Errorf("validate race result: %w", err)
	}
	return result, nil
}
