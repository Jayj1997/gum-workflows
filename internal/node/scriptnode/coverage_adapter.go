package scriptnode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// AdaptCoverageResult interprets go test evidence as a strict Quality Check Result.
func AdaptCoverageResult(record ExecutionRecord, threshold float64) (CoverageResult, error) {
	exitStatus, err := readRequired(filepath.Join(record.ToolOutputDir, "test-exit.txt"))
	if err != nil {
		return CoverageResult{}, node.Structural(fmt.Errorf("coverage result adapter: %w", err))
	}
	reportedExitCode, err := strconv.Atoi(strings.TrimSpace(string(exitStatus)))
	if err != nil || reportedExitCode != record.ExitCode {
		return CoverageResult{}, node.Structural(fmt.Errorf("coverage result adapter: test-exit.txt does not match process exit code %d", record.ExitCode))
	}
	toolchain, err := readToolchain(record.ToolOutputDir)
	if err != nil {
		return CoverageResult{}, node.Structural(fmt.Errorf("coverage result adapter: %w", err))
	}
	toolchain.Tool = "go test"
	result := CoverageResult{
		APIVersion: qualityCheckResultAPIVersion, Check: CoverageCheck, Code: record.Code,
		EffectiveConfig: CoverageEffectiveConfig{PackageScope: "./...", MinimumStatementCoverage: threshold},
		Toolchain:       toolchain,
		Logs: LogReferences{
			Stdout: artifact.ArtifactRef{ID: "stdout", Kind: artifact.KindLog, URI: record.StdoutPath},
			Stderr: artifact.ArtifactRef{ID: "stderr", Kind: artifact.KindLog, URI: record.StderrPath},
		},
		Findings: []CoverageFinding{}, StartedAt: record.StartedAt, FinishedAt: record.FinishedAt,
	}

	testFailed := false
	if record.ExitCode != 0 {
		testFailed, err = coverageHasFailedEvent(record)
		if err != nil {
			return CoverageResult{}, node.Structural(fmt.Errorf("coverage result adapter: %w", err))
		}
	}
	if testFailed {
		reason, reasonErr := coverageFailureReason(record)
		if reasonErr != nil {
			return CoverageResult{}, node.Structural(fmt.Errorf("coverage result adapter: %w", reasonErr))
		}
		result.Verdict = VerdictFailed
		result.Metrics.StatementCoverage = CoverageMetric{Reason: reason}
		result.Findings = []CoverageFinding{{Tool: "go test", Message: reason}}
	} else {
		profile, readErr := readRequired(filepath.Join(record.ToolOutputDir, "coverage.out"))
		if readErr != nil {
			return CoverageResult{}, node.Structural(fmt.Errorf("coverage result adapter: %w", readErr))
		}
		percentage, statements, parseErr := parseCoverageProfile(profile)
		if parseErr != nil {
			return CoverageResult{}, node.Structural(fmt.Errorf("coverage result adapter: %w", parseErr))
		}
		if statements == 0 {
			result.Verdict = VerdictNotApplicable
			result.Metrics.StatementCoverage = CoverageMetric{Reason: "no instrumentable statements"}
		} else {
			result.Metrics.StatementCoverage = CoverageMetric{Available: true, Value: &percentage, Unit: "percent"}
			if percentage < threshold {
				result.Verdict = VerdictFailed
				result.Findings = []CoverageFinding{{Tool: "go test", Message: fmt.Sprintf("statement coverage %.1f%% is below minimum %.1f%%", percentage, threshold)}}
			} else {
				result.Verdict = VerdictPassed
			}
		}
	}
	if err := result.Validate(); err != nil {
		return CoverageResult{}, node.Structural(fmt.Errorf("coverage result adapter: %w", err))
	}
	return result, nil
}

func coverageHasFailedEvent(record ExecutionRecord) (bool, error) {
	events, err := readRequired(filepath.Join(record.ToolOutputDir, "test.json"))
	if err != nil {
		return false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(events))
	for {
		var event struct {
			Action  string `json:"Action"`
			Package string `json:"Package"`
		}
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, fmt.Errorf("decode test.json: %w", err)
		}
		if event.Action == "fail" && event.Package != "" {
			return true, nil
		}
	}
}

func parseCoverageProfile(data []byte) (percentage float64, statements int64, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	if !scanner.Scan() {
		return 0, 0, fmt.Errorf("coverage.out is empty")
	}
	header := strings.Fields(scanner.Text())
	if len(header) != 2 || header[0] != "mode:" || (header[1] != "set" && header[1] != "count" && header[1] != "atomic") {
		return 0, 0, fmt.Errorf("coverage.out has an invalid mode header")
	}
	var covered int64
	line := 1
	for scanner.Scan() {
		line++
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || !validCoverageRange(fields[0]) {
			return 0, 0, fmt.Errorf("coverage.out line %d is not a complete profile block", line)
		}
		blockStatements, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil || blockStatements < 0 {
			return 0, 0, fmt.Errorf("coverage.out line %d has invalid statement count", line)
		}
		count, parseErr := strconv.ParseInt(fields[2], 10, 64)
		if parseErr != nil || count < 0 {
			return 0, 0, fmt.Errorf("coverage.out line %d has invalid execution count", line)
		}
		if statements > math.MaxInt64-blockStatements {
			return 0, 0, fmt.Errorf("coverage.out statement count overflows")
		}
		statements += blockStatements
		if count > 0 {
			covered += blockStatements
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("read coverage.out: %w", err)
	}
	if statements == 0 {
		return 0, 0, nil
	}
	return float64(covered) * 100 / float64(statements), statements, nil
}

func validCoverageRange(value string) bool {
	separator := strings.LastIndexByte(value, ':')
	if separator <= 0 || separator == len(value)-1 {
		return false
	}
	parts := strings.FieldsFunc(value[separator+1:], func(r rune) bool { return r == '.' || r == ',' })
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		number, err := strconv.ParseInt(part, 10, 64)
		if err != nil || number <= 0 {
			return false
		}
	}
	return true
}

func coverageFailureReason(record ExecutionRecord) (string, error) {
	events, err := readRequired(filepath.Join(record.ToolOutputDir, "test.json"))
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(events))
	var outputs []string
	for {
		var event struct {
			Output string `json:"Output"`
		}
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("decode test.json: %w", err)
		}
		if output := strings.TrimSpace(event.Output); output != "" {
			outputs = append(outputs, output)
		}
	}
	if len(outputs) > 0 {
		return outputs[len(outputs)-1], nil
	}
	stderr, err := os.ReadFile(record.StderrPath)
	if err != nil {
		return "", fmt.Errorf("read stderr log: %w", err)
	}
	if reason := strings.TrimSpace(string(stderr)); reason != "" {
		return reason, nil
	}
	return fmt.Sprintf("go test failed with exit code %d", record.ExitCode), nil
}
