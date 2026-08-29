package scriptnode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

const (
	qualityCheckResultAPIVersion = "qualityCheckResult/v1"
	// StaticAnalysisCheck identifies the go-static-analysis result shape.
	StaticAnalysisCheck = "static-analysis"
)

// Verdict is the business outcome of a completed quality check.
type Verdict string

const (
	// VerdictPassed means the completed check found no diagnostics.
	VerdictPassed Verdict = "passed"
	// VerdictFailed means the completed check found source or package diagnostics.
	VerdictFailed Verdict = "failed"
	// VerdictNotApplicable means the workspace contains no Go package to inspect.
	VerdictNotApplicable Verdict = "not-applicable"
)

// StaticResult is the strict qualityCheckResult/v1 payload for go vet.
type StaticResult struct {
	APIVersion      string                `json:"apiVersion"`
	Check           string                `json:"check"`
	Verdict         Verdict               `json:"verdict"`
	Code            artifact.ArtifactRef  `json:"code"`
	EffectiveConfig StaticEffectiveConfig `json:"effectiveConfig"`
	Toolchain       Toolchain             `json:"toolchain"`
	FindingsCount   int                   `json:"findingsCount"`
	Findings        []StaticFinding       `json:"findings"`
	Logs            LogReferences         `json:"logs"`
	StartedAt       time.Time             `json:"startedAt"`
	FinishedAt      time.Time             `json:"finishedAt"`
}

// StaticEffectiveConfig records the fixed, non-user-overridable static-analysis scope.
type StaticEffectiveConfig struct {
	PackageScope string `json:"packageScope"`
}

// Toolchain records the non-sensitive Go environment needed to explain a result.
type Toolchain struct {
	Tool            string `json:"tool"`
	LauncherVersion string `json:"launcherVersion"`
	FinalVersion    string `json:"finalVersion"`
	GOROOT          string `json:"goroot"`
	GOOS            string `json:"goos"`
	GOARCH          string `json:"goarch"`
	CGOEnabled      string `json:"cgoEnabled"`
}

// StaticFinding is one go vet or package-loading diagnostic.
type StaticFinding struct {
	Tool     string `json:"tool"`
	Package  string `json:"package,omitempty"`
	Analyzer string `json:"analyzer,omitempty"`
	Position string `json:"position,omitempty"`
	Message  string `json:"message"`
}

// LogReferences points at the streamed stdout and stderr files for a Node Run.
type LogReferences struct {
	Stdout artifact.ArtifactRef `json:"stdout"`
	Stderr artifact.ArtifactRef `json:"stderr"`
}

// Validate enforces the closed static-result contract and cross-field invariants.
func (r StaticResult) Validate() error {
	if r.APIVersion != qualityCheckResultAPIVersion {
		return fmt.Errorf("apiVersion must be %q", qualityCheckResultAPIVersion)
	}
	if r.Check != StaticAnalysisCheck {
		return fmt.Errorf("check must be %q", StaticAnalysisCheck)
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
	if r.Toolchain.Tool != "go vet" || strings.TrimSpace(r.Toolchain.LauncherVersion) == "" ||
		strings.TrimSpace(r.Toolchain.FinalVersion) == "" || strings.TrimSpace(r.Toolchain.GOROOT) == "" ||
		strings.TrimSpace(r.Toolchain.GOOS) == "" || strings.TrimSpace(r.Toolchain.GOARCH) == "" ||
		strings.TrimSpace(r.Toolchain.CGOEnabled) == "" {
		return fmt.Errorf("toolchain must contain go vet and complete Go environment details")
	}
	if r.FindingsCount < 0 || r.FindingsCount != len(r.Findings) {
		return fmt.Errorf("findingsCount %d does not match %d findings", r.FindingsCount, len(r.Findings))
	}
	for i, finding := range r.Findings {
		if finding.Tool != "go vet" || strings.TrimSpace(finding.Message) == "" {
			return fmt.Errorf("findings[%d] must identify go vet and a message", i)
		}
	}
	if r.Verdict == VerdictPassed && r.FindingsCount != 0 {
		return fmt.Errorf("passed result must not contain findings")
	}
	if r.Verdict == VerdictFailed && r.FindingsCount == 0 {
		return fmt.Errorf("failed result must contain findings")
	}
	if r.Verdict == VerdictNotApplicable && r.FindingsCount != 0 {
		return fmt.Errorf("not-applicable result must not contain findings")
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

// DecodeStaticResult strictly decodes and validates a qualityCheckResult/v1 payload.
func DecodeStaticResult(data []byte) (StaticResult, error) {
	var result StaticResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return StaticResult{}, fmt.Errorf("decode static result: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return StaticResult{}, fmt.Errorf("decode static result: expected one JSON value")
	}
	if err := result.Validate(); err != nil {
		return StaticResult{}, fmt.Errorf("validate static result: %w", err)
	}
	return result, nil
}
