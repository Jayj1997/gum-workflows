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
	if len(def.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(def.Projects))
	}
	if def.Projects[0].Name != "order-system" || def.Projects[0].Repository != "./examples/order-system" {
		t.Errorf("Projects[0] = %+v", def.Projects[0])
	}
	if len(def.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(def.Nodes))
	}

	coder := def.Nodes["coder"]
	if coder.Node != "coding-agent" {
		t.Errorf("coder.Node = %q, want %q", coder.Node, "coding-agent")
	}
	if coder.Executor != "" || coder.LLM != "" || coder.TargetModel != "" {
		t.Errorf("coder optional fields = %q/%q/%q, want empty defaults", coder.Executor, coder.LLM, coder.TargetModel)
	}
	if len(coder.Config) != 1 || coder.Config["task"] == nil {
		t.Errorf("coder.Config = %v, want task entry", coder.Config)
	}

	sdk := def.Nodes["sdk"]
	if sdk.Node != "openapi-generator" {
		t.Errorf("sdk.Node = %q, want %q", sdk.Node, "openapi-generator")
	}
	if got := sdk.Inputs["openapi"].From; got != "coder.openapi" {
		t.Errorf("sdk.Inputs[openapi].From = %q, got %q", got, "coder.openapi")
	}

	// dependsOn 是可选字段：valid.yaml 没有声明，应为 nil。
	if sdk.DependsOn != nil {
		t.Errorf("sdk.DependsOn = %v, want nil (dependsOn is optional)", sdk.DependsOn)
	}

	if err := def.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

// TestLoadNodeInstanceOptionalFields 验证 Node Instance 新形态的
// 可选字段（executor/llm/target_model/metadata）经严格模式往返。
func TestLoadNodeInstanceOptionalFields(t *testing.T) {
	const yaml = `
apiVersion: workflow/v1
kind: workflow
metadata:
  name: optional-fields
projects:
  - name: demo
    repository: ./project
nodes:
  backend:
    node: coding-agent
    executor: v1
    llm: openai
    target_model: gpt-4o
    metadata:
      name: 后端实现
      description: 依据需求与架构产出后端源码
`
	def, err := Load([]byte(yaml))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	spec := def.Nodes["backend"]
	if spec.Node != "coding-agent" || spec.Executor != "v1" {
		t.Errorf("node/executor = %q/%q", spec.Node, spec.Executor)
	}
	if spec.LLM != "openai" || spec.TargetModel != "gpt-4o" {
		t.Errorf("llm/target_model = %q/%q", spec.LLM, spec.TargetModel)
	}
	if spec.Metadata.Name != "后端实现" || spec.Metadata.Description != "依据需求与架构产出后端源码" {
		t.Errorf("metadata = %+v", spec.Metadata)
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

func TestLoadRejectsOldTypeField(t *testing.T) {
	// 旧形态（type:）必须被严格模式拒绝：字段已改名 node。
	_, err := Load([]byte("apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nnodes:\n  a:\n    type: coding-agent\n"))
	if err == nil {
		t.Fatal("Load(old type field) = nil error, want rejection")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error %q should mention the removed field %q", err, "type")
	}
}

func TestLoadRejectsOldProjectSingular(t *testing.T) {
	// 旧形态（project 单数）必须被严格模式拒绝：字段已改 projects 列表。
	_, err := Load([]byte("apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nproject:\n  repository: ./p\nnodes:\n  a:\n    node: coding-agent\n"))
	if err == nil {
		t.Fatal("Load(old project field) = nil error, want rejection")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Errorf("error %q should mention the removed field %q", err, "project")
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
