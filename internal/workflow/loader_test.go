package workflow

import (
	"strings"
	"testing"
)

func TestLoadValid(t *testing.T) {
	def, err := LoadFile("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("LoadFile() unexpected error: %v", err)
	}

	if def.APIVersion != APIVersionV1 {
		t.Errorf("APIVersion = %q, want %q", def.APIVersion, APIVersionV1)
	}
	if def.Kind != KindWorkflow {
		t.Errorf("Kind = %q, want %q", def.Kind, KindWorkflow)
	}
	if def.Metadata.Name != "fullstack-development" {
		t.Errorf("Metadata.Name = %q", def.Metadata.Name)
	}
	if def.Project.Repository != "./examples/order-system" {
		t.Errorf("Project.Repository = %q", def.Project.Repository)
	}
	if len(def.Nodes) != 5 {
		t.Fatalf("len(Nodes) = %d, want 5", len(def.Nodes))
	}

	backend := def.Nodes["backend"]
	if backend.Type != "coding-agent" {
		t.Errorf("backend.Type = %q, want %q", backend.Type, "coding-agent")
	}
	if len(backend.Inputs) != 2 {
		t.Errorf("len(backend.Inputs) = %d, want 2", len(backend.Inputs))
	}
	if got := backend.Inputs["analysis-output"].From; got != "requirement.analysis-output" {
		t.Errorf("backend.Inputs[analysis-output].From = %q", got)
	}

	// dependsOn 是可选字段：valid.yaml 没有声明，应为 nil。
	if backend.DependsOn != nil {
		t.Errorf("backend.DependsOn = %v, want nil (dependsOn is optional)", backend.DependsOn)
	}

	if err := def.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	_, err := LoadFile("testdata/unknown-field.yaml")
	if err == nil {
		t.Fatal("LoadFile() = nil error, want unknown-field rejection")
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Errorf("error %q should mention the unknown field %q", err, "retry")
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	_, err := LoadFile("testdata/malformed.yaml")
	if err == nil {
		t.Fatal("LoadFile() = nil error, want parse failure")
	}
}

func TestLoadRejectsEmptyFile(t *testing.T) {
	_, err := LoadFile("testdata/empty.yaml")
	if err == nil {
		t.Fatal("LoadFile() = nil error, want empty-file rejection")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error %q should mention empty", err)
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	_, err := LoadFile("testdata/nonexistent.yaml")
	if err == nil {
		t.Fatal("LoadFile() = nil error, want read failure")
	}
}
