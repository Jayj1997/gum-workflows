package scriptnode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/project"
)

func TestNodeExecuteStreamsLogsAndAdaptsOnlyFormalToolOutputs(t *testing.T) {
	workspace := t.TempDir()
	nodeRunDir := t.TempDir()
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: test-check
executor: v1
entry: check.sh
platforms: [` + runtimePlatform() + `]
requirements: {executables: [sh]}
toolOutputs: [{path: result.txt, required: true}]
resultAdapter: test/v1
`)
	manifest, err := LoadManifest(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	bundle := Bundle{
		Manifest: manifest, ManifestBytes: manifestBytes,
		Files: map[string][]byte{"check.sh": []byte("#!/bin/sh\nprintf 'log-only\\n'\n" +
			"printf 'warning-only\\n' >&2\nprintf 'formal-result\\n' > \"$2/result.txt\"\n")},
	}
	bundle.ExpectedDigest = bundle.Digest()

	adapter := func(record ExecutionRecord) (artifact.Artifact, error) {
		data, err := os.ReadFile(filepath.Join(record.ToolOutputDir, "result.txt"))
		if err != nil {
			return artifact.Artifact{}, err
		}
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult, Data: strings.TrimSpace(string(data))}, nil
	}
	check, err := New(bundle, "test-check", "v1", "test/v1", adapter)
	if err != nil {
		t.Fatal(err)
	}
	store := artifact.NewMemStore()
	diagnostics := &node.RunDiagnostics{}
	ctx := node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: store,
		Run: node.RunContext{
			WorkflowRunID: "run-1", NodeRunID: "node-run-1",
			LogsDir: filepath.Join(nodeRunDir, "logs"), ToolOutputDir: filepath.Join(nodeRunDir, "tool-output"),
		},
		Diagnostics: diagnostics,
	}
	code := artifact.ArtifactRef{ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}
	outputs, err := check.Execute(ctx, map[string]artifact.ArtifactRef{"code": code})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	result, err := store.Get(outputs["result"])
	if err != nil || result.Data != "formal-result" {
		t.Fatalf("result = %+v/%v, want formal tool output only", result, err)
	}
	stdout, _ := os.ReadFile(filepath.Join(ctx.Run.LogsDir, "stdout.log"))
	stderr, _ := os.ReadFile(filepath.Join(ctx.Run.LogsDir, "stderr.log"))
	if string(stdout) != "log-only\n" || string(stderr) != "warning-only\n" {
		t.Errorf("logs = stdout %q stderr %q", stdout, stderr)
	}
	if diagnostics.BundleDigest != bundle.ExpectedDigest || diagnostics.CWD != workspace || len(diagnostics.Arguments) != 2 {
		t.Errorf("diagnostics = %+v", diagnostics)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Errorf("workspace entries = %v/%v, want no Gum products", entries, err)
	}
}

func TestNodeExecuteTreatsMissingFormalOutputAsStructuralError(t *testing.T) {
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: test-check
executor: v1
entry: check.sh
platforms: [` + runtimePlatform() + `]
requirements: {executables: [sh]}
toolOutputs: [{path: result.txt, required: true}]
resultAdapter: test/v1
`)
	manifest, _ := LoadManifest(manifestBytes)
	bundle := Bundle{Manifest: manifest, ManifestBytes: manifestBytes, Files: map[string][]byte{"check.sh": []byte("#!/bin/sh\nexit 0\n")}}
	bundle.ExpectedDigest = bundle.Digest()
	check, err := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		return artifact.Artifact{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	workspace := t.TempDir()
	_, err = check.Execute(node.ExecutionContext{
		Context: context.Background(), Project: project.Context{Workspace: workspace}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: workspace}})
	if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural {
		t.Fatalf("Execute() error = %v, want Structural Error", err)
	}
}

func TestNodeExecuteHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: test-check
executor: v1
entry: check.sh
platforms: [` + runtimePlatform() + `]
requirements: {executables: [sh]}
toolOutputs: []
resultAdapter: test/v1
`)
	manifest, _ := LoadManifest(manifestBytes)
	bundle := Bundle{Manifest: manifest, ManifestBytes: manifestBytes, Files: map[string][]byte{"check.sh": []byte("#!/bin/sh\nsleep 10\n")}}
	bundle.ExpectedDigest = bundle.Digest()
	check, _ := New(bundle, "test-check", "v1", "test/v1", func(ExecutionRecord) (artifact.Artifact, error) {
		return artifact.Artifact{ID: "result", Kind: artifact.KindQualityCheckResult}, nil
	})
	runDir := t.TempDir()
	started := time.Now()
	_, err := check.Execute(node.ExecutionContext{
		Context: ctx, Project: project.Context{Workspace: t.TempDir()}, Store: artifact.NewMemStore(),
		Run: node.RunContext{LogsDir: filepath.Join(runDir, "logs"), ToolOutputDir: filepath.Join(runDir, "tool-output")},
	}, map[string]artifact.ArtifactRef{"code": {ID: "code", Kind: artifact.KindSourceCode, Version: "1", URI: "/workspace"}})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("Execute(canceled) = %v after %s", err, time.Since(started))
	}
}
