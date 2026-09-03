package product_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSecretCanaryNeverLeaksIntoAnySink runs one failing real-model Run with
// a known canary API key that the fixture provider echoes back inside its
// error message — the worst case for leakage — and then scans every sink the
// ticket names: the SQLite database, the run log, the returned error, the
// persisted Artifacts and the diagnostics bundle.
func TestSecretCanaryNeverLeaksIntoAnySink(t *testing.T) {
	ctx := context.Background()
	const canary = "gum-secret-canary-8291"
	application, paths, runID := generateFailedRunWithCanary(t, canary)

	// Database: the provider error body echoed the canary; the persisted
	// Structural Error must not carry it.
	database, err := os.ReadFile(paths.Database())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(database), canary) {
		t.Fatal("SQLite database contains the canary secret")
	}

	// Run log and full run directory (artifacts included).
	assertNoCanary(t, canary, paths.RunDir(runID), "run directory")

	// Returned error chain and the persisted Run view.
	detail, err := application.GetRunHistory(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), canary) {
		t.Fatal("run history view contains the canary secret")
	}

	// Diagnostics bundle written from the failed Run.
	bundle, err := application.GenerateDiagnosticsBundle(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	assertNoCanary(t, canary, bundle.Path, "diagnostics bundle")
	// And the bundle's own view carries no secret either.
	bundleEncoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bundleEncoded), canary) {
		t.Fatal("diagnostics bundle view contains the canary secret")
	}
}

// TestRunLogRecordsNodeRunIdentityPhaseLatencyAndRequestID verifies the run
// log contract: every line carries run and node-run identity, phases cover
// started/finished, latency is present, and the Provider request ID of the
// successful model call is recorded.
func TestRunLogRecordsNodeRunIdentityPhaseLatencyAndRequestID(t *testing.T) {
	ctx := context.Background()
	server, _ := startFixtureLLMServer(t)
	application, paths, _, workflow, lockVersion := openRealRunApplication(t, ctx, server)
	run, err := application.StartRun(ctx, singleTurnInput(workflow.ID, lockVersion))
	if err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(paths.LogsDir(run.ID), "run.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read run log: %v", err)
	}
	var lines []map[string]any
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("decode run log line %q: %v", line, err)
		}
		lines = append(lines, decoded)
	}
	if len(lines) < 5 {
		t.Fatalf("run log lines = %d, want node-run start/finish for both nodes plus run event:\n%s", len(lines), data)
	}

	var startedCount, finishedCount, latencyCount, requestIDCount, agentIdentity int
	for _, line := range lines {
		if line["runId"] != run.ID {
			t.Fatalf("log line missing run identity: %#v", line)
		}
		switch line["msg"] {
		case "node run started":
			startedCount++
		case "node run finished":
			finishedCount++
			if _, ok := line["latencyMs"]; ok {
				latencyCount++
			}
			if line["nodeRunId"] == run.NodeRuns[1].ID && line["nodeId"] == "answer" {
				agentIdentity++
				if line["providerRequestId"] == "chatcmpl-fixture-1" {
					requestIDCount++
				}
				if line["phase"] != "succeeded" {
					t.Fatalf("agent finish phase = %#v", line["phase"])
				}
			}
		case "run event":
			if line["event"] != "run-succeeded" {
				t.Fatalf("run event = %#v", line["event"])
			}
		}
	}
	if startedCount != 2 || finishedCount != 2 || latencyCount != 2 {
		t.Fatalf("log phases: started=%d finished=%d latency=%d", startedCount, finishedCount, latencyCount)
	}
	if agentIdentity != 1 || requestIDCount != 1 {
		t.Fatalf("agent identity lines = %d, request ID lines = %d", agentIdentity, requestIDCount)
	}
}

// TestFailedRunLogCarriesSanitizedError verifies the error path of the run
// log: identity, failed phase, latency and a redacted error message.
func TestFailedRunLogCarriesSanitizedError(t *testing.T) {
	const canary = "gum-secret-canary-log-5507"
	_, paths, runID := generateFailedRunWithCanary(t, canary)

	logPath := filepath.Join(paths.LogsDir(runID), "run.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"runId":"`+runID+`"`) {
		t.Fatalf("failed run log has no run identity:\n%s", text)
	}
	if !strings.Contains(text, "run-failed") || !strings.Contains(text, "latencyMs") {
		t.Fatalf("failed run log missing phase or latency:\n%s", text)
	}
	if !strings.Contains(text, "authentication") {
		t.Fatalf("failed run log missing the sanitized error classification:\n%s", text)
	}
	if strings.Contains(text, canary) {
		t.Fatalf("failed run log contains the canary secret:\n%s", text)
	}
}
