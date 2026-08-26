package definition

import (
	"strings"
	"testing"
)

// nodeDefYAML 是设计文档 §3.2 示例的化简形态（契约全 optional 输入）。
const nodeDefYAML = `
apiVersion: nodeDefinition/v1
kind: nodeDefinition
metadata:
  name: coding-agent
  description: 在项目 Workspace 中执行编码任务的 agent。
type: agent
requires: [project]
inputs:
  analysis-output:
    type: markdown
    optional: true
outputs:
  source-code:
    type: SourceCode
`

func TestLoadNodeDefinitionAcceptsValid(t *testing.T) {
	d, err := LoadNodeDefinition([]byte(nodeDefYAML))
	if err != nil {
		t.Fatalf("LoadNodeDefinition() unexpected error: %v", err)
	}
	if d.Metadata.Name != "coding-agent" || d.Type != "agent" {
		t.Errorf("loaded = %+v, want coding-agent/agent", d)
	}
	if port := d.Inputs["analysis-output"]; !port.Optional {
		t.Errorf("input analysis-output optional = false, want true")
	}
}

func TestLoadNodeDefinitionRejectsUnknownField(t *testing.T) {
	yaml := strings.Replace(nodeDefYAML, "type: agent", "type: agent\nbogus: 1", 1)
	_, err := LoadNodeDefinition([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeDefinition() = nil error, want unknown-field rejection")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error %q should mention the unknown field", err)
	}
}

func TestLoadNodeDefinitionRejectsBadType(t *testing.T) {
	yaml := strings.Replace(nodeDefYAML, "type: agent", "type: robot", 1)
	_, err := LoadNodeDefinition([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeDefinition() = nil error, want type rejection")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error %q should locate the type field", err)
	}
}

func TestLoadNodeDefinitionRejectsBadTypeExpr(t *testing.T) {
	yaml := strings.Replace(nodeDefYAML, "type: markdown", "type: lowercase", 1)
	_, err := LoadNodeDefinition([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeDefinition() = nil error, want TypeExpr rejection")
	}
	// 错误必须定位到端口（§3.2 校验、M3 验收风格）。
	if !strings.Contains(err.Error(), "analysis-output") {
		t.Errorf("error %q should name the offending port", err)
	}
}

func TestLoadNodeDefinitionRejectsMissingDescription(t *testing.T) {
	yaml := strings.Replace(nodeDefYAML, "  description: 在项目 Workspace 中执行编码任务的 agent。\n", "", 1)
	_, err := LoadNodeDefinition([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeDefinition() = nil error, want description-required rejection")
	}
	if !strings.Contains(err.Error(), "metadata.description") {
		t.Errorf("error %q should locate metadata.description", err)
	}
}

func TestLoadNodeDefinitionRejectsOutputOptional(t *testing.T) {
	// outputs 端口无 optional 字段（§3.2）：严格模式下出现即未知字段。
	yaml := strings.Replace(nodeDefYAML, "    type: SourceCode", "    type: SourceCode\n    optional: true", 1)
	_, err := LoadNodeDefinition([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeDefinition() = nil error, want optional-on-output rejection")
	}
	if !strings.Contains(err.Error(), "optional") {
		t.Errorf("error %q should mention the illegal optional field", err)
	}
}
