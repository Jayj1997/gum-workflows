package product

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// bundleSchemaVersion names the diagnostics bundle manifest structure.
const bundleSchemaVersion = "productDiagnosticsBundle/v1"

// DiagnosticsBundleView describes one explicitly generated crash bundle.
// It reports what the bundle contains and where it lives so the UI can show
// the boundary before the user shares anything.
type DiagnosticsBundleView struct {
	RunID         string                  `json:"runId"`
	WorkflowID    string                  `json:"workflowId"`
	Path          string                  `json:"path"`
	SchemaVersion string                  `json:"schemaVersion"`
	AppVersion    string                  `json:"appVersion"`
	GeneratedAt   time.Time               `json:"generatedAt"`
	Contents      []DiagnosticsBundleItem `json:"contents"`
}

// DiagnosticsBundleItem names one file the bundle contains.
type DiagnosticsBundleItem struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// bundleNodeRunSummary is the per-Node-Run identity, phase, latency and
// sanitized error recorded in the bundle manifest.
type bundleNodeRunSummary struct {
	NodeRunID         string                  `json:"nodeRunId"`
	NodeID            string                  `json:"nodeId"`
	NodeDefinition    string                  `json:"nodeDefinition"`
	Status            string                  `json:"status"`
	StartedAt         time.Time               `json:"startedAt"`
	FinishedAt        time.Time               `json:"finishedAt"`
	LatencyMs         int64                   `json:"latencyMs"`
	ProviderRequestID string                  `json:"providerRequestId,omitempty"`
	Inputs            map[string]string       `json:"inputs"`
	Outputs           map[string]string       `json:"outputs"`
	Diagnostics       *NodeRunDiagnosticsView `json:"diagnostics,omitempty"`
}

// bundleRunSummary is the Run-level summary recorded in the bundle manifest.
type bundleRunSummary struct {
	ID         string                          `json:"id"`
	WorkflowID string                          `json:"workflowId"`
	RevisionID string                          `json:"revisionId"`
	Status     string                          `json:"status"`
	StartedAt  time.Time                       `json:"startedAt"`
	FinishedAt time.Time                       `json:"finishedAt"`
	Error      *productworkflow.ExecutionError `json:"error,omitempty"`
	NodeRuns   []bundleNodeRunSummary          `json:"nodeRuns"`
}

// bundleManifest is the JSON document at the root of every crash bundle.
type bundleManifest struct {
	SchemaVersion     string                  `json:"schemaVersion"`
	AppVersion        string                  `json:"appVersion"`
	ProductSchemaHint string                  `json:"productSchemaVersion"`
	GeneratedAt       time.Time               `json:"generatedAt"`
	Run               bundleRunSummary        `json:"run"`
	Includes          []DiagnosticsBundleItem `json:"includes"`
	// Excluded documents the boundary the bundle deliberately keeps: no
	// Prompt, Conversation body or other Artifact content is included.
	Excluded []string `json:"excluded"`
}

// GenerateDiagnosticsBundle writes an explicit crash diagnostics bundle for
// one Run below its Local Data Root directory and returns its user-visible
// description. The bundle contains the manifest with schema versions, app
// version, Run and Node Run summaries plus the sanitized run log; it never
// contains Prompt, Conversation or any other Artifact body.
func (a *Application) GenerateDiagnosticsBundle(ctx context.Context, runID string) (DiagnosticsBundleView, error) {
	if a.runHistory == nil {
		return DiagnosticsBundleView{}, fmt.Errorf("generate diagnostics bundle: product run history repository is not configured")
	}
	run, nodeRuns, _, err := a.runHistory.GetProductRun(ctx, runID)
	if err != nil {
		return DiagnosticsBundleView{}, fmt.Errorf("generate diagnostics bundle: %w", err)
	}

	summary := bundleRunSummary{
		ID: run.ID, WorkflowID: run.WorkflowID, RevisionID: run.RevisionID,
		Status: run.Status, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		Error: run.Error, NodeRuns: make([]bundleNodeRunSummary, 0, len(nodeRuns)),
	}
	for _, nodeRun := range nodeRuns {
		inputs := make(map[string]string, len(nodeRun.Inputs))
		for name, ref := range nodeRun.Inputs {
			inputs[name] = artifactRefText(ref)
		}
		outputs := make(map[string]string, len(nodeRun.Outputs))
		for name, ref := range nodeRun.Outputs {
			outputs[name] = artifactRefText(ref)
		}
		summary.NodeRuns = append(summary.NodeRuns, bundleNodeRunSummary{
			NodeRunID: nodeRun.ID, NodeID: nodeRun.NodeID, NodeDefinition: nodeRun.NodeDefinition,
			Status: nodeRun.Status, StartedAt: nodeRun.StartedAt, FinishedAt: nodeRun.FinishedAt,
			LatencyMs:         nodeRun.FinishedAt.Sub(nodeRun.StartedAt).Milliseconds(),
			ProviderRequestID: nodeRun.Diagnostics.ProviderRequestID,
			Inputs:            inputs, Outputs: outputs,
			Diagnostics: nodeRunView(nodeRun).Diagnostics,
		})
	}

	items := []DiagnosticsBundleItem{
		{Name: "manifest.json", Kind: "manifest", Description: "schema versions, app version, Run and Node Run summaries"},
		{Name: "run.log", Kind: "run-log", Description: "sanitized structured run log"},
	}
	manifest := bundleManifest{
		SchemaVersion:     bundleSchemaVersion,
		AppVersion:        appVersion,
		ProductSchemaHint: strconv.Itoa(ProductSchemaVersion),
		GeneratedAt:       time.Now().UTC(),
		Run:               summary,
		Includes:          items,
		Excluded: []string{
			"prompts", "conversation bodies", "artifact content",
			"api keys and secret references", "raw provider requests and responses",
		},
	}

	bundleDir := filepath.Join(a.runPaths.RunDir(run.ID), "diagnostics")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return DiagnosticsBundleView{}, fmt.Errorf("generate diagnostics bundle: create directory: %w", err)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return DiagnosticsBundleView{}, fmt.Errorf("generate diagnostics bundle: encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), []byte(a.redactor.Redact(string(manifestData))), 0o600); err != nil {
		return DiagnosticsBundleView{}, fmt.Errorf("generate diagnostics bundle: write manifest: %w", err)
	}
	// The run log is a safe reference by construction; a missing log (e.g. a
	// Run whose process died before logging) is not a bundle failure.
	if logPath := a.runLogPath(run.ID); logPath != "" {
		if _, statErr := os.Stat(logPath); statErr == nil {
			if err := writeRunLogToBundle(bundleDir, logPath, a.redactor); err != nil {
				return DiagnosticsBundleView{}, fmt.Errorf("generate diagnostics bundle: %w", err)
			}
		}
	}

	view := DiagnosticsBundleView{
		RunID: run.ID, WorkflowID: run.WorkflowID, Path: bundleDir,
		SchemaVersion: bundleSchemaVersion, AppVersion: appVersion,
		GeneratedAt: manifest.GeneratedAt, Contents: items,
	}
	return view, nil
}

// artifactRefText renders one ArtifactRef as a safe identity string. Refs
// carry ID/Kind/Version/URI only; the URI is a Local Data Root-relative file
// name, never Artifact content.
func artifactRefText(ref artifact.ArtifactRef) string {
	return fmt.Sprintf("%s:%s:%s", ref.Kind, ref.ID, ref.Version)
}
