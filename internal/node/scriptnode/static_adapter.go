package scriptnode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// StaticExecutionRecord is the complete evidence consumed by the static Result Adapter.
type StaticExecutionRecord struct {
	ExitCode      int
	ToolOutputDir string
	StdoutPath    string
	StderrPath    string
	Code          artifact.ArtifactRef
	StartedAt     time.Time
	FinishedAt    time.Time
}

type vetDiagnostic struct {
	Posn    string `json:"posn"`
	Message string `json:"message"`
}

// AdaptStaticResult interprets go vet evidence as a strict Quality Check Result.
func AdaptStaticResult(record StaticExecutionRecord) (StaticResult, error) {
	packages, err := readRequired(filepath.Join(record.ToolOutputDir, "packages.txt"))
	if err != nil {
		return StaticResult{}, node.Structural(fmt.Errorf("static result adapter: %w", err))
	}
	vetOutput, err := readRequired(filepath.Join(record.ToolOutputDir, "vet.json"))
	if err != nil {
		return StaticResult{}, node.Structural(fmt.Errorf("static result adapter: %w", err))
	}
	toolchain, err := readToolchain(record.ToolOutputDir)
	if err != nil {
		return StaticResult{}, node.Structural(fmt.Errorf("static result adapter: %w", err))
	}

	findings, err := decodeVetFindings(vetOutput)
	if err != nil {
		return StaticResult{}, node.Structural(fmt.Errorf("static result adapter: %w", err))
	}
	if record.ExitCode != 0 {
		stderr, readErr := os.ReadFile(record.StderrPath)
		if readErr != nil {
			return StaticResult{}, node.Structural(fmt.Errorf("static result adapter: read stderr log: %w", readErr))
		}
		findings = append(findings, packageFindings(string(stderr))...)
	}

	verdict := VerdictPassed
	switch {
	case strings.TrimSpace(string(packages)) == "" && record.ExitCode == 0:
		verdict = VerdictNotApplicable
	case len(findings) > 0:
		verdict = VerdictFailed
	case record.ExitCode != 0:
		return StaticResult{}, node.Structural(fmt.Errorf("static result adapter: go vet exited %d without a diagnostic", record.ExitCode))
	}

	result := StaticResult{
		APIVersion:      qualityCheckResultAPIVersion,
		Check:           StaticAnalysisCheck,
		Verdict:         verdict,
		Code:            record.Code,
		EffectiveConfig: StaticEffectiveConfig{PackageScope: "./..."},
		Toolchain:       toolchain,
		FindingsCount:   len(findings),
		Findings:        findings,
		Logs: LogReferences{
			Stdout: artifact.ArtifactRef{ID: "stdout", Kind: artifact.KindLog, URI: record.StdoutPath},
			Stderr: artifact.ArtifactRef{ID: "stderr", Kind: artifact.KindLog, URI: record.StderrPath},
		},
		StartedAt:  record.StartedAt,
		FinishedAt: record.FinishedAt,
	}
	if err := result.Validate(); err != nil {
		return StaticResult{}, node.Structural(fmt.Errorf("static result adapter: %w", err))
	}
	return result, nil
}

func readRequired(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tool output %s: %w", filepath.Base(path), err)
	}
	return data, nil
}

func readToolchain(dir string) (Toolchain, error) {
	version, err := readRequired(filepath.Join(dir, "go-version.txt"))
	if err != nil {
		return Toolchain{}, err
	}
	environment, err := readRequired(filepath.Join(dir, "go-env.txt"))
	if err != nil {
		return Toolchain{}, err
	}
	lines := strings.Split(strings.TrimSuffix(string(environment), "\n"), "\n")
	if len(lines) != 5 {
		return Toolchain{}, fmt.Errorf("go-env.txt has %d fields, want 5", len(lines))
	}
	return Toolchain{
		Tool: "go vet", LauncherVersion: strings.TrimSpace(string(version)), FinalVersion: lines[0],
		GOROOT: lines[1], GOOS: lines[2], GOARCH: lines[3], CGOEnabled: lines[4],
	}, nil
}

func decodeVetFindings(data []byte) ([]StaticFinding, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var findings []StaticFinding
	for {
		var report map[string]map[string][]vetDiagnostic
		if err := decoder.Decode(&report); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode vet.json: %w", err)
		}
		packages := make([]string, 0, len(report))
		for packageName := range report {
			packages = append(packages, packageName)
		}
		sort.Strings(packages)
		for _, packageName := range packages {
			analyzers := make([]string, 0, len(report[packageName]))
			for analyzer := range report[packageName] {
				analyzers = append(analyzers, analyzer)
			}
			sort.Strings(analyzers)
			for _, analyzer := range analyzers {
				for _, diagnostic := range report[packageName][analyzer] {
					if strings.TrimSpace(diagnostic.Message) == "" {
						return nil, fmt.Errorf("decode vet.json: diagnostic message must not be empty")
					}
					findings = append(findings, StaticFinding{
						Tool: "go vet", Package: packageName, Analyzer: analyzer,
						Position: diagnostic.Posn, Message: diagnostic.Message,
					})
				}
			}
		}
	}
	return findings, nil
}

func packageFindings(stderr string) []StaticFinding {
	var findings []StaticFinding
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "# ") {
			continue
		}
		findings = append(findings, StaticFinding{Tool: "go vet", Message: line})
	}
	return findings
}
