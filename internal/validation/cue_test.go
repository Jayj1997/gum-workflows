package validation

import (
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

const validYAML = `
apiVersion: workflow/v1
kind: Workflow

metadata:
  name: fullstack-development
  version: "1.0"

project:
  repository: ./examples/order-system
  branch: main

nodes:
  requirement:
    type: requirement-analysis
  backend:
    type: coding-agent
    inputs:
      requirement:
        from: requirement.requirement
    dependsOn:
      - requirement
`

func TestValidateSchemaAcceptsValid(t *testing.T) {
	if err := ValidateSchema("test.yaml", []byte(validYAML)); err != nil {
		t.Fatalf("ValidateSchema() unexpected error: %v", err)
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
			yaml:    "apiVersion: workflow/v2\nkind: Workflow\nmetadata:\n  name: x\nproject:\n  repository: ./p\nnodes:\n  a:\n    type: t\n",
			wantErr: "apiVersion",
		},
		{
			name:    "wrong kind",
			yaml:    "apiVersion: workflow/v1\nkind: Job\nmetadata:\n  name: x\nproject:\n  repository: ./p\nnodes:\n  a:\n    type: t\n",
			wantErr: "kind",
		},
		{
			name:    "missing metadata.name",
			yaml:    "apiVersion: workflow/v1\nkind: Workflow\nproject:\n  repository: ./p\nnodes:\n  a:\n    type: t\n",
			wantErr: "metadata",
		},
		{
			name:    "missing project.repository",
			yaml:    "apiVersion: workflow/v1\nkind: Workflow\nmetadata:\n  name: x\nproject:\n  branch: main\nnodes:\n  a:\n    type: t\n",
			wantErr: "project",
		},
		{
			name:    "missing node type",
			yaml:    "apiVersion: workflow/v1\nkind: Workflow\nmetadata:\n  name: x\nproject:\n  repository: ./p\nnodes:\n  a:\n    inputs:\n      i:\n        from: b.o\n",
			wantErr: "type",
		},
		{
			name:    "node type wrong type",
			yaml:    "apiVersion: workflow/v1\nkind: Workflow\nmetadata:\n  name: x\nproject:\n  repository: ./p\nnodes:\n  a:\n    type: 123\n",
			wantErr: "type",
		},
		{
			name:    "input from wrong type",
			yaml:    "apiVersion: workflow/v1\nkind: Workflow\nmetadata:\n  name: x\nproject:\n  repository: ./p\nnodes:\n  a:\n    type: t\n    inputs:\n      i:\n        from: 42\n",
			wantErr: "from",
		},
		{
			name:    "dependsOn wrong element type",
			yaml:    "apiVersion: workflow/v1\nkind: Workflow\nmetadata:\n  name: x\nproject:\n  repository: ./p\nnodes:\n  a:\n    type: t\n    dependsOn:\n      - 42\n",
			wantErr: "dependsOn",
		},
		{
			name:    "malformed yaml syntax",
			yaml:    "apiVersion: workflow/v1\n\tkind: Workflow\n",
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
