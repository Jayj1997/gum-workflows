package definition

import (
	"strings"
	"testing"
)

// executorYAML 是设计文档 §3.3 示例的化简形态。
const executorYAML = `
apiVersion: nodeExecutor/v1
kind: nodeExecutor
metadata:
  name: coding-agent-v1
  description: coding-agent 首个执行器（Mock 实现）。
node: coding-agent
version: v1
updates: 首个版本：Mock 编码 agent。
`

func TestLoadNodeExecutorAcceptsValid(t *testing.T) {
	e, err := LoadNodeExecutor([]byte(executorYAML))
	if err != nil {
		t.Fatalf("LoadNodeExecutor() unexpected error: %v", err)
	}
	if e.Node != "coding-agent" || e.Version != "v1" {
		t.Errorf("loaded = %+v, want coding-agent/v1", e)
	}
}

func TestLoadNodeExecutorRejectsUnknownField(t *testing.T) {
	yaml := strings.Replace(executorYAML, "version: v1", "version: v1\nruntime: go", 1)
	_, err := LoadNodeExecutor([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeExecutor() = nil error, want unknown-field rejection")
	}
	if !strings.Contains(err.Error(), "runtime") {
		t.Errorf("error %q should mention the unknown field", err)
	}
}

func TestLoadNodeExecutorRejectsMissingNode(t *testing.T) {
	yaml := strings.Replace(executorYAML, "node: coding-agent\n", "", 1)
	_, err := LoadNodeExecutor([]byte(yaml))
	if err == nil {
		t.Fatal("LoadNodeExecutor() = nil error, want node-required rejection")
	}
	if !strings.Contains(err.Error(), "node") {
		t.Errorf("error %q should locate the node field", err)
	}
}

func TestLoadNodeExecutorMetadataOptional(t *testing.T) {
	// metadata 是展示用选填（§3.3），省略应合法。
	yaml := strings.Replace(executorYAML, "metadata:\n  name: coding-agent-v1\n  description: coding-agent 首个执行器（Mock 实现）。\n", "", 1)
	e, err := LoadNodeExecutor([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadNodeExecutor() unexpected error: %v", err)
	}
	if e.Metadata.Name != "" {
		t.Errorf("name = %q, want empty (optional metadata)", e.Metadata.Name)
	}
}
