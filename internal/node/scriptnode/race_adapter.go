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

	raceCount := 0
	findings := make([]RaceFinding, 0)
	anyTestRun := false
	lastOutput := ""
	lastPackage := ""
	for _, event := range events {
		if event.Action == "run" && event.Test != "" {
			anyTestRun = true
		}
		if output := strings.TrimSpace(event.Output); output != "" {
			lastOutput = output
			lastPackage = event.Package
		}
		if strings.Contains(event.Output, "WARNING: DATA RACE") {
			raceCount++
			findings = append(findings, RaceFinding{
				Tool: "go test -race", Kind: "race", Package: event.Package,
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
	case strings.TrimSpace(string(packages)) == "" && record.ExitCode == 0:
		result.Verdict = VerdictNotApplicable
		result.Metrics.RacesDetected = RaceMetric{Reason: "no Go packages"}
	case raceCount > 0:
		result.Verdict = VerdictFailed
	case record.ExitCode != 0:
		result.Verdict = VerdictFailed
		kind := "compile-failure"
		if strings.TrimSpace(string(packages)) == "" {
			kind = "package-failure"
		} else if anyTestRun {
			kind = "test-failure"
		}
		message, messageErr := raceFailureMessage(record, lastOutput)
		if messageErr != nil {
			return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", messageErr))
		}
		result.Findings = append(result.Findings, RaceFinding{
			Tool: "go test -race", Kind: kind, Package: lastPackage, Message: message,
		})
	default:
		result.Verdict = VerdictPassed
	}
	if err := result.Validate(); err != nil {
		return RaceResult{}, node.Structural(fmt.Errorf("race result adapter: %w", err))
	}
	return result, nil
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
