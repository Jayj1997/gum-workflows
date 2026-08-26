package validation

import (
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

const validYAML = `
apiVersion: workflow/v1
kind: workflow

metadata:
  name: minimal-development
  version: "1.0"

projects:
  - name: order-system
    repository: ./examples/order-system

nodes:
  coder:
    node: coding-agent
  sdk:
    node: openapi-generator
    inputs:
      openapi:
        from: coder.openapi
    dependsOn:
      - coder
`

func TestValidateSchemaAcceptsValid(t *testing.T) {
	if err := ValidateSchema("test.yaml", []byte(validYAML)); err != nil {
		t.Fatalf("ValidateSchema() unexpected error: %v", err)
	}
}

func TestValidateSchemaAcceptsNodeInstanceOptionalFields(t *testing.T) {
	// Node Instance 新形态：executor/llm/target_model/metadata 全部可选。
	yaml := `
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
	if err := ValidateSchema("test.yaml", []byte(yaml)); err != nil {
		t.Fatalf("ValidateSchema() unexpected error: %v", err)
	}
	if _, err := workflow.Load([]byte(yaml)); err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
}

func TestValidateSchemaRejectsStructuralErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "wrong apiVersion",
			yaml:    "apiVersion: workflow/v2\nkind: workflow\nmetadata:\n  name: x\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    node: t\n",
			wantErr: "apiVersion",
		},
		{
			name:    "wrong kind",
			yaml:    "apiVersion: workflow/v1\nkind: Job\nmetadata:\n  name: x\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    node: t\n",
			wantErr: "kind",
		},
		{
			name:    "uppercase kind rejected",
			yaml:    "apiVersion: workflow/v1\nkind: Workflow\nmetadata:\n  name: x\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    node: t\n",
			wantErr: "kind",
		},
		{
			name:    "missing metadata.name",
			yaml:    "apiVersion: workflow/v1\nkind: workflow\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    node: t\n",
			wantErr: "metadata",
		},
		{
			name:    "project entry missing repository",
			yaml:    "apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nprojects:\n  - name: p\nnodes:\n  a:\n    node: t\n",
			wantErr: "repository",
		},
		{
			name:    "projects not a list",
			yaml:    "apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nprojects: ./p\nnodes:\n  a:\n    node: t\n",
			wantErr: "projects",
		},
		{
			name:    "missing node reference",
			yaml:    "apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    inputs:\n      i:\n        from: b.o\n",
			wantErr: "node",
		},
		{
			name:    "node reference wrong type",
			yaml:    "apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    node: 123\n",
			wantErr: "node",
		},
		{
			name:    "input from wrong type",
			yaml:    "apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    node: t\n    inputs:\n      i:\n        from: 42\n",
			wantErr: "from",
		},
		{
			name:    "dependsOn wrong element type",
			yaml:    "apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    node: t\n    dependsOn:\n      - 42\n",
			wantErr: "dependsOn",
		},
		{
			name:    "executor wrong type",
			yaml:    "apiVersion: workflow/v1\nkind: workflow\nmetadata:\n  name: x\nprojects:\n  - name: p\n    repository: ./p\nnodes:\n  a:\n    node: t\n    executor: 3\n",
			wantErr: "executor",
		},
		{
			name:    "malformed yaml syntax",
			yaml:    "apiVersion: workflow/v1\n\tkind: workflow\n",
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSchema("test.yaml", []byte(tt.yaml))
			if err == nil {
				t.Fatalf("ValidateSchema() = nil error, want rejection")
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSchemaEmptyFile(t *testing.T) {
	if err := ValidateSchema("test.yaml", nil); err == nil {
		t.Fatal("ValidateSchema(empty) = nil error, want rejection")
	}
}

// CUE 与 Go Loader 应当一致：CUE 通过的定义必须能被 Loader 解析。
func TestCUEAndLoaderAgree(t *testing.T) {
	if err := ValidateSchema("test.yaml", []byte(validYAML)); err != nil {
		t.Fatalf("ValidateSchema() unexpected error: %v", err)
	}
	if _, err := workflow.Load([]byte(validYAML)); err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
}
