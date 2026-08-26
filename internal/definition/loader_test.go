package definition

import (
	"strings"
	"testing"
)

// nodeTypeYAML 是设计文档 §3.1 示例的 YAML 形态。
const nodeTypeYAML = `
apiVersion: nodeTypeDefinition/v1
kind: nodeTypeDefinition
metadata:
  name: agent
  description: 以 AI 执行的节点。
requires: [llm]
`

func TestLoadNodeTypeAcceptsValid(t *testing.T) {
	nt, err := LoadNodeType([]byte(nodeTypeYAML))
	if err != nil {
		t.Fatalf("LoadNodeType() unexpected error: %v", err)
	}
	if nt.Metadata.Name != "agent" {
		t.Errorf("name = %q, want agent", nt.Metadata.Name)
	}
	if len(nt.Requires) != 1 || nt.Requires[0] != "llm" {
		t.Errorf("requires = %v, want [llm]", nt.Requires)
	}
}

func TestLoadNodeTypeRejectsUnknownField(t *testing.T) {
	yaml := strings.Replace(nodeTypeYAML, "requires: [llm]", "requires: [llm]\nextra: true", 1)
	_, err := LoadNodeType([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeType() = nil error, want unknown-field rejection")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Errorf("error %q should mention the unknown field", err)
	}
}

func TestLoadNodeTypeRejectsBadName(t *testing.T) {
	yaml := strings.Replace(nodeTypeYAML, "name: agent", "name: robot", 1)
	_, err := LoadNodeType([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeType() = nil error, want name ∈ three-value rejection")
	}
	if !strings.Contains(err.Error(), `robot`) || !strings.Contains(err.Error(), "agent") {
		t.Errorf("error %q should name the bad value and the legal set", err)
	}
}

func TestLoadNodeTypeRejectsBadRequires(t *testing.T) {
	yaml := strings.Replace(nodeTypeYAML, "requires: [llm]", "requires: [gpu]", 1)
	_, err := LoadNodeType([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeType() = nil error, want requires-value rejection")
	}
	if !strings.Contains(err.Error(), "gpu") {
		t.Errorf("error %q should name the illegal requires value", err)
	}
}

func TestLoadNodeTypeRejectsEmptyFile(t *testing.T) {
	if _, err := LoadNodeType([]byte("")); err == nil {
		t.Fatal("LoadNodeType() = nil error, want empty-file rejection")
	}
}

func TestLoadNodeTypeRejectsSecondDocument(t *testing.T) {
	yaml := nodeTypeYAML + "---\napiVersion: nodeTypeDefinition/v1\n"
	if _, err := LoadNodeType([]byte(yaml)); err == nil {
		t.Fatal("LoadNodeType() = nil error, want single-document rejection")
	}
}
