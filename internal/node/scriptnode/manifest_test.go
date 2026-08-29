package scriptnode

import (
	"strings"
	"testing"
)

func TestLoadManifestAcceptsStrictAutomationScriptContract(t *testing.T) {
	manifest, err := LoadManifest([]byte(`apiVersion: automationScript/v1
kind: automationScript
node: go-static-analysis
executor: v1
entry: check.sh
platforms: [darwin, linux]
requirements:
  executables: [go]
toolOutputs:
  - path: vet.json
    required: true
resultAdapter: go-static-analysis/v1
`))
	if err != nil {
		t.Fatalf("LoadManifest() unexpected error: %v", err)
	}
	if manifest.Node != "go-static-analysis" || manifest.Executor != "v1" || manifest.Entry != "check.sh" {
		t.Errorf("manifest identity = %+v", manifest)
	}
	if len(manifest.Platforms) != 2 || len(manifest.Requirements.Executables) != 1 {
		t.Errorf("manifest requirements = %+v", manifest)
	}
}

func TestLoadManifestRejectsUnknownFieldsAndUnsafePaths(t *testing.T) {
	tests := map[string]string{
		"unknown field": `apiVersion: automationScript/v1
kind: automationScript
node: go-static-analysis
executor: v1
entry: check.sh
platforms: [linux]
requirements: {executables: [go]}
toolOutputs: [{path: vet.json, required: true}]
resultAdapter: go-static-analysis/v1
command: go vet
`,
		"unsafe entry": `apiVersion: automationScript/v1
kind: automationScript
node: go-static-analysis
executor: v1
entry: ../check.sh
platforms: [linux]
requirements: {executables: [go]}
toolOutputs: [{path: vet.json, required: true}]
resultAdapter: go-static-analysis/v1
`,
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadManifest([]byte(input)); err == nil {
				t.Fatal("LoadManifest() = nil error, want strict rejection")
			}
		})
	}
}

func TestBundleDigestAndIdentityCoverEveryImmutableAsset(t *testing.T) {
	manifestBytes := []byte(`apiVersion: automationScript/v1
kind: automationScript
node: go-static-analysis
executor: v1
entry: check.sh
platforms: [linux]
requirements: {executables: [go]}
toolOutputs: [{path: vet.json, required: true}]
resultAdapter: go-static-analysis/v1
`)
	manifest, err := LoadManifest(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	bundle := Bundle{Manifest: manifest, ManifestBytes: manifestBytes, Files: map[string][]byte{"check.sh": []byte("#!/bin/sh\n")}}
	bundle.ExpectedDigest = bundle.Digest()
	if err := bundle.Validate("go-static-analysis", "v1"); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	first := bundle.Digest()
	bundle.Files["check.sh"] = []byte("#!/bin/sh\necho changed\n")
	if changed := bundle.Digest(); first == changed {
		t.Fatalf("Digest() did not change after script edit: %s", first)
	}
	if err := bundle.Validate("go-static-analysis", "v1"); err == nil {
		t.Fatal("Validate(changed content) = nil error, want digest mismatch")
	}
	if !strings.HasPrefix(first, "sha256:") {
		t.Errorf("Digest() = %q, want sha256 prefix", first)
	}
	if err := bundle.Validate("other-node", "v1"); err == nil {
		t.Fatal("Validate(identity mismatch) = nil error")
	}
	bundle.Files["check.sh"] = []byte("#!/bin/sh\n")
	bundle.Manifest.ToolOutputs[0].Path = "changed.json"
	if err := bundle.Validate("go-static-analysis", "v1"); err == nil || !strings.Contains(err.Error(), "manifest bytes") {
		t.Fatalf("Validate(mutated parsed manifest) error = %v, want immutable manifest mismatch", err)
	}
}
