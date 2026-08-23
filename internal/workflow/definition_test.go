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
		Project:    ProjectSpec{Repository: "./examples/order-system", Branch: "main"},
		Nodes: map[string]NodeSpec{
			"requirement": {Type: "requirement-analysis"},
			"backend": {
				Type: "coding-agent",
				Inputs: map[string]InputBinding{
					"requirement": {From: "requirement.requirement"},
				},
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
			name: "empty node type",
			mutate: func(d *Definition) {
				d.Nodes["requirement"] = NodeSpec{}
			},
			wantErr: `node "requirement": type`,
		},
		{
			name: "empty input from",
			mutate: func(d *Definition) {
				d.Nodes["backend"].Inputs["requirement"] = InputBinding{}
			},
			wantErr: `node "backend" input "requirement": from`,
		},
		{
			name: "malformed input from",
			mutate: func(d *Definition) {
				d.Nodes["backend"].Inputs["requirement"] = InputBinding{From: "no-dot-here"}
			},
			wantErr: `node "backend" input "requirement"`,
		},
		{
			name: "node id contains dot",
			mutate: func(d *Definition) {
				d.Nodes["back.end"] = NodeSpec{Type: "coding-agent"}
			},
			wantErr: `node "back.end": ID must not contain`,
		},
		{
			name: "self dependsOn",
			mutate: func(d *Definition) {
				spec := d.Nodes["requirement"]
				spec.DependsOn = []string{"requirement"}
				d.Nodes["requirement"] = spec
			},
			wantErr: `node "requirement": dependsOn must not include itself`,
		},
		{
			name: "duplicate dependsOn",
			mutate: func(d *Definition) {
				spec := d.Nodes["backend"]
				spec.DependsOn = []string{"requirement", "requirement"}
				d.Nodes["backend"] = spec
			},
			wantErr: `node "backend": duplicate dependsOn entry "requirement"`,
		},
		{
			name: "empty dependsOn entry",
			mutate: func(d *Definition) {
				spec := d.Nodes["backend"]
				spec.DependsOn = []string{""}
				d.Nodes["backend"] = spec
			},
			wantErr: `node "backend": dependsOn entries must not be empty`,
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
