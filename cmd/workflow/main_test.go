package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validFixtureEnv 满足语义校验的运行前提（票 06 起 validate 全链生效）：
// 临时 XDG_CONFIG_HOME 提供 llm.yaml（agent 节点的默认链解析），
// fixture 的相对 projects 路径已就位于 testdata/examples/。
// 不触真实 $HOME（DEVELOPMENT.md §6）。
func validFixtureEnv(t *testing.T) {
	t.Helper()

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir := filepath.Join(xdg, "gum-workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir xdg config: %v", err)
	}
	llmYAML := `
apiVersion: llm/v1
kind: llm
providers:
  - name: openai
    type: openai-compatible
    url: https://api.openai.com/v1
    apikey: plain-test-key
    default: true
    models:
      - name: gpt-4o
        default: true
      - name: gpt-4o-mini
`
	if err := os.WriteFile(filepath.Join(dir, "llm.yaml"), []byte(llmYAML), 0o644); err != nil {
		t.Fatalf("write llm.yaml: %v", err)
	}
}

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"validate"}, {"run", "x.yaml"}, {"validate", "a", "b"}} {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) = nil error, want usage error", args)
		}
	}
	if err := run([]string{"bogus", "x.yaml"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run(bogus) error = %v, want unknown command", err)
	}
}

func TestValidateCmd(t *testing.T) {
	validFixtureEnv(t)

	t.Run("valid workflow", func(t *testing.T) {
		err := validateCmd(filepath.Join("..", "..", "internal", "workflow", "testdata", "valid.yaml"))
		if err != nil {
			t.Fatalf("validateCmd() unexpected error: %v", err)
		}
	})

	t.Run("example workflow passes full pipeline", func(t *testing.T) {
		if err := validateCmd(filepath.Join("..", "..", "examples", "minimal", "workflow.yaml")); err != nil {
			t.Fatalf("validateCmd(example) unexpected error: %v", err)
		}
	})

	t.Run("semantic violation is rejected", func(t *testing.T) {
		// unknown-type 结构合法（CUE 通过），语义层必须拦截。
		path := filepath.Join("..", "..", "internal", "validation", "testdata", "invalid-node", "unknown-type.yaml")
		err := validateCmd(path)
		if err == nil {
			t.Fatal("validateCmd(unknown-type) = nil error, want semantic rejection")
		}
		if !strings.Contains(err.Error(), "unknown node definition") {
			t.Errorf("error %q should mention unknown node definition", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if err := validateCmd("nonexistent.yaml"); err == nil {
			t.Fatal("validateCmd(nonexistent) = nil error, want read failure")
		}
	})
}
