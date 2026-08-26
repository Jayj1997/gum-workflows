package workflow

import (
	"strings"
	"testing"
)

// validDefinition 返回一份结构合法的最小定义，各用例在其上做局部破坏。
func validDefinition() Definition {
	return Definition{
		APIVersion: APIVersionV1,
		Kind:       KindWorkflow,
		Metadata:   Metadata{Name: "fullstack-development", Version: "1.0"},
		Projects:   []ProjectSpec{{Name: "order-system", Repository: "./examples/order-system"}},
		Nodes: map[string]NodeSpec{
			"coder": {Node: "coding-agent"},
			"sdk": {
				Node:   "openapi-generator",
				Inputs: map[string]InputBinding{"openapi": {From: "coder.openapi"}},
			},
		},
	}
}

func TestDefinitionValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(d *Definition)
		wantErr string // 为空表示期望通过
	}{
		{
			name:   "valid definition",
			mutate: func(d *Definition) {},
		},
		{
			name: "valid with all optional node fields",
			mutate: func(d *Definition) {
				coder := d.Nodes["coder"]
				coder.Executor = "v1"
				coder.LLM = "openai"
				coder.TargetModel = "gpt-4o"
				coder.Metadata = InstanceMetadata{Name: "实现", Description: "编码节点"}
				d.Nodes["coder"] = coder
			},
		},
		{
			name:    "empty apiVersion",
			mutate:  func(d *Definition) { d.APIVersion = "" },
			wantErr: "apiVersion",
		},
		{
			name:    "empty kind",
			mutate:  func(d *Definition) { d.Kind = "" },
			wantErr: "kind",
		},
		{
			name:    "empty metadata name",
			mutate:  func(d *Definition) { d.Metadata.Name = "" },
			wantErr: "metadata.name",
		},
		{
			name:    "no nodes",
			mutate:  func(d *Definition) { d.Nodes = nil },
			wantErr: "nodes",
		},
		{
			name: "empty node reference",
			mutate: func(d *Definition) {
				d.Nodes["coder"] = NodeSpec{}
			},
			wantErr: `node "coder": node`,
		},
		{
			name: "empty input from",
			mutate: func(d *Definition) {
				d.Nodes["sdk"].Inputs["openapi"] = InputBinding{}
			},
			wantErr: `node "sdk" input "openapi": from`,
		},
		{
			name: "malformed input from",
			mutate: func(d *Definition) {
				d.Nodes["sdk"].Inputs["openapi"] = InputBinding{From: "no-dot-here"}
			},
			wantErr: `node "sdk" input "openapi"`,
		},
		{
			name: "node id contains dot",
			mutate: func(d *Definition) {
				d.Nodes["co.der"] = NodeSpec{Node: "coding-agent"}
			},
			wantErr: `node "co.der": ID must not contain`,
		},
		{
			name: "project name empty",
			mutate: func(d *Definition) {
				d.Projects = []ProjectSpec{{Repository: "./p"}}
			},
			wantErr: `projects[0]: name`,
		},
		{
			name: "project repository empty",
			mutate: func(d *Definition) {
				d.Projects = []ProjectSpec{{Name: "demo", Repository: "  "}}
			},
			wantErr: `projects[0] "demo": repository`,
		},
		{
			name: "self dependsOn",
			mutate: func(d *Definition) {
				spec := d.Nodes["coder"]
				spec.DependsOn = []string{"coder"}
				d.Nodes["coder"] = spec
			},
			wantErr: `node "coder": dependsOn must not include itself`,
		},
		{
			name: "duplicate dependsOn",
			mutate: func(d *Definition) {
				spec := d.Nodes["sdk"]
				spec.DependsOn = []string{"coder", "coder"}
				d.Nodes["sdk"] = spec
			},
			wantErr: `node "sdk": duplicate dependsOn entry "coder"`,
		},
		{
			name: "empty dependsOn entry",
			mutate: func(d *Definition) {
				spec := d.Nodes["sdk"]
				spec.DependsOn = []string{""}
				d.Nodes["sdk"] = spec
			},
			wantErr: `node "sdk": dependsOn entries must not be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validDefinition()
			tt.mutate(&d)

			err := d.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("Validate() unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("Validate() = nil, want error containing %q", tt.wantErr)
			case tt.wantErr != "" && err != nil:
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Validate() error = %q, want containing %q", err, tt.wantErr)
				}
			}
		})
	}
}
