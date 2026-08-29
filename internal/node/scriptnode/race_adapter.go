package scriptnode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

type goTestEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

// AdaptRaceResult interprets one full-scope Go race invocation as a strict Quality Check Result.
func AdaptRaceResult(record ExecutionRecord) (RaceResult, error) {
	packages, err := readRequired(filepath.Join(record.ToolOutputDir, "packages.txt"))
	if err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}
	evidence, err := readRequired(filepath.Join(record.ToolOutputDir, "test.json"))
	if err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}
	exitStatus, err := readRequired(filepath.Join(record.ToolOutputDir, "test-exit.txt"))
	if err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}
	reportedExitCode, err := strconv.Atoi(strings.TrimSpace(string(exitStatus)))
	if err != nil || reportedExitCode != record.ExitCode {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: test-exit.txt does not match process exit code %d", record.ExitCode))
	}
	if record.ExitCode == 125 || record.ExitCode == 126 || record.ExitCode == 127 {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: race script infrastructure exited %d", record.ExitCode))
	}
	toolchain, err := readToolchain(record.ToolOutputDir)
	if err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}
	toolchain.Tool = "go test -race"
	compiler, err := readRequired(filepath.Join(record.ToolOutputDir, "cc.txt"))
	if err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}
	toolchain.CCompiler = strings.TrimSpace(string(compiler))
	events, err := decodeGoTestEvents(evidence)
	if err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}
	packageNames := nonEmptyLines(packages)
	if err := validateGoTestEventCompletion(packageNames, events, record.ExitCode); err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}

	raceCount := 0
	findings := make([]RaceFinding, 0)
	testRan := make(map[string]bool)
	racePackages := make(map[string]bool)
	lastOutput := make(map[string]string)
	for _, event := range events {
		if event.Action == "run" && event.Test != "" {
			testRan[event.Package] = true
		}
		if output := strings.TrimSpace(event.Output); output != "" {
			lastOutput[event.Package] = output
		}
		if strings.Contains(event.Output, "WARNING: DATA RACE") {
			raceCount++
			racePackages[event.Package] = true
			findings = append(findings, RaceFinding{
				Tool: "go test -race", Kind: RaceFindingObserved, Package: event.Package,
				Message: "data race observed during this invocation",
			})
		}
	}

	result := RaceResult{
		APIVersion: qualityCheckResultAPIVersion, Check: RaceCheck, Code: record.Code,
		EffectiveConfig: RaceEffectiveConfig{PackageScope: "./..."}, Toolchain: toolchain,
		Metrics:  RaceMetrics{RacesDetected: RaceMetric{Available: true, Value: &raceCount, Unit: "count"}},
		Findings: findings,
		Logs: LogReferences{
			Stdout: artifact.ArtifactRef{ID: "stdout", Kind: artifact.KindLog, URI: record.StdoutPath},
			Stderr: artifact.ArtifactRef{ID: "stderr", Kind: artifact.KindLog, URI: record.StderrPath},
		},
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt,
	}
	switch {
	case len(packageNames) == 0 && record.ExitCode == 0:
		result.Verdict = VerdictNotApplicable
		result.Metrics.RacesDetected = RaceMetric{Reason: "no Go packages"}
	case record.ExitCode != 0:
		result.Verdict = VerdictFailed
		if len(packageNames) == 0 {
			message, messageErr := raceFailureMessage(record, "")
			if messageErr != nil {
				return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", messageErr))
			}
			result.Findings = append(result.Findings, RaceFinding{Tool: "go test -race", Kind: RaceFindingPackageFailure, Message: message})
		} else {
			for _, packageName := range failedGoTestPackages(events) {
				if racePackages[packageName] {
					continue
				}
				kind := RaceFindingCompileFailure
				if testRan[packageName] {
					kind = RaceFindingTestFailure
				}
				message, messageErr := raceFailureMessage(record, lastOutput[packageName])
				if messageErr != nil {
					return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", messageErr))
				}
				result.Findings = append(result.Findings, RaceFinding{
					Tool: "go test -race", Kind: kind, Package: packageName, Message: message,
				})
			}
		}
	case raceCount > 0:
		result.Verdict = VerdictFailed
	default:
		result.Verdict = VerdictPassed
	}
	if err := result.Validate(); err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}
	return result, nil
}

func nonEmptyLines(data []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func validateGoTestEventCompletion(packages []string, events []goTestEvent, exitCode int) error {
	if len(packages) == 0 {
		return nil
	}
	terminal := make(map[string]string)
	for _, event := range events {
		if event.Test == "" && (event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
			terminal[event.Package] = event.Action
		}
	}
	if exitCode == 0 {
		for _, packageName := range packages {
			if action := terminal[packageName]; action != "pass" && action != "skip" {
				return fmt.Errorf("test.json lacks a successful terminal event for package %q", packageName)
			}
		}
		return nil
	}
	if len(failedGoTestPackages(events)) == 0 {
		return fmt.Errorf("test.json lacks a failed package terminal event")
	}
	return nil
}

func failedGoTestPackages(events []goTestEvent) []string {
	seen := make(map[string]bool)
	var packages []string
	for _, event := range events {
		if event.Test != "" || event.Action != "fail" || event.Package == "" || seen[event.Package] {
			continue
		}
		seen[event.Package] = true
		packages = append(packages, event.Package)
	}
	return packages
}

func decodeGoTestEvents(data []byte) ([]goTestEvent, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var events []goTestEvent
	for {
		var event goTestEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return events, nil
			}
			return nil, fmt.Errorf("decode test.json: %w", err)
		}
		events = append(events, event)
	}
}

func raceFailureMessage(record ExecutionRecord, lastOutput string) (string, error) {
	if strings.TrimSpace(lastOutput) != "" {
		return strings.TrimSpace(lastOutput), nil
	}
	stderr, err := os.ReadFile(record.StderrPath)
	if err != nil {
		return "", fmt.Errorf("read stderr log: %w", err)
	}
	if message := strings.TrimSpace(string(stderr)); message != "" {
		return message, nil
	}
	return fmt.Sprintf("go test -race failed with exit code %d", record.ExitCode), nil
}
