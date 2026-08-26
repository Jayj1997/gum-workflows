package llm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig 把 yaml 内容写到 dir 下的 gum-workflows/llm.yaml，返回完整路径。
func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	sub := filepath.Join(dir, "gum-workflows")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(sub, "llm.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write llm.yaml: %v", err)
	}
	return path
}

// writeHomeConfig 写到 dir/.config/gum-workflows/llm.yaml（home 回退路径布局）。
func writeHomeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	sub := filepath.Join(dir, ".config", "gum-workflows")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(sub, "llm.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write llm.yaml: %v", err)
	}
	return path
}

// validYAML 是设计文档 §3.4 示例的 YAML 形态。
const validYAML = `
apiVersion: llm/v1
kind: llm
providers:
  - name: openai
    description: 主力模型
    type: openai-compatible
    url: https://api.openai.com/v1
    apikey: $TEST_OPENAI_KEY
    default: true
    models:
      - name: gpt-4o
        default: true
        temperature: 0.2
        max_tokens: 4096
      - name: gpt-4o-mini
  - name: anthropic
    type: anthropic
    url: https://api.anthropic.com
    apikey: $TEST_ANTHROPIC_KEY
    models:
      - name: claude-sonnet-5
`

func TestLoadResolvesEnvRefs(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "sk-openai-secret")
	t.Setenv("TEST_ANTHROPIC_KEY", "sk-anthropic-secret")

	c, err := Load([]byte(validYAML))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if got := c.Providers[0].APIKey; got != "sk-openai-secret" {
		t.Errorf("providers[0].APIKey = %q, want resolved env value", got)
	}
	if got := c.Providers[1].APIKey; got != "sk-anthropic-secret" {
		t.Errorf("providers[1].APIKey = %q, want resolved env value", got)
	}
}

func TestLoadAllowsPlaintextAPIKey(t *testing.T) {
	// 明文与 $VAR 都允许（设计文档 §3.4）。
	yaml := `
apiVersion: llm/v1
kind: llm
providers:
  - name: local
    type: openai-compatible
    url: http://localhost:8080/v1
    apikey: plaintext-key
    models:
      - name: llama
`
	c, err := Load([]byte(yaml))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if got := c.Providers[0].APIKey; got != "plaintext-key" {
		t.Errorf("APIKey = %q, want plaintext preserved", got)
	}
}

func TestLoadMissingEnvVarNamesTheVariable(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "x")
	// TEST_MISSING_VAR 未设置。
	yaml := strings.Replace(validYAML, "$TEST_ANTHROPIC_KEY", "$TEST_MISSING_VAR", 1)

	_, err := Load([]byte(yaml))
	if err == nil {
		t.Fatal("Load() = nil error, want missing env var rejection")
	}
	if !strings.Contains(err.Error(), "TEST_MISSING_VAR") {
		t.Fatalf("Load() error = %v, want it to name the missing variable", err)
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Fatalf("Load() error = %v, want it to locate the provider", err)
	}
}

func TestLoadSetButEmptyEnvVarResolves(t *testing.T) {
	// 变量「缺失」才报错；显式设为空串是已设置，按值解析（区分 ok 与空值）。
	// 该 provider 的 apikey 因此为空，会被必填校验拒绝--先验证报错来自
	// apikey 必填（后续校验），而不是「变量未设置」。
	t.Setenv("TEST_OPENAI_KEY", "x")
	t.Setenv("TEST_ANTHROPIC_KEY", "")

	_, err := Load([]byte(validYAML))
	if err == nil {
		t.Fatal("Load() = nil error, want rejection from apikey required check")
	}
	if strings.Contains(err.Error(), "which is not set") {
		t.Fatalf("Load() error = %v, want set-but-empty var treated as set (missing-var error would be wrong)", err)
	}
	if !strings.Contains(err.Error(), `provider "anthropic": apikey must not be empty`) {
		t.Fatalf("Load() error = %v, want apikey required error", err)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "x")
	t.Setenv("TEST_ANTHROPIC_KEY", "x")

	yaml := strings.Replace(validYAML, "    type: openai-compatible", "    type: openai-compatible\n    retry: 3", 1)
	_, err := Load([]byte(yaml))
	if err == nil {
		t.Fatal("Load() = nil error, want unknown-field rejection")
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Errorf("error %q should mention the unknown field", err)
	}
}

func TestLoadRejectsEmptyFile(t *testing.T) {
	_, err := Load([]byte(""))
	if err == nil {
		t.Fatal("Load() = nil error, want empty-file rejection")
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "x")
	t.Setenv("TEST_ANTHROPIC_KEY", "x")

	yaml := strings.Replace(validYAML, "type: openai-compatible", "type: azure", 1)
	_, err := Load([]byte(yaml))
	if err == nil {
		t.Fatal("Load() = nil error, want semantic rejection")
	}
}

func TestLoadFileSearchOrder(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "x")
	t.Setenv("TEST_ANTHROPIC_KEY", "x")

	xdg := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", home)

	t.Run("prefers XDG_CONFIG_HOME over home fallback", func(t *testing.T) {
		writeConfig(t, xdg, validYAML)
		// home 回退路径放一份不同的内容，若被选中则 name 会不同。
		writeHomeConfig(t, home, strings.Replace(validYAML, "name: openai", "name: openai-home", 1))

		c, err := LoadDefault()
		if err != nil {
			t.Fatalf("LoadDefault() unexpected error: %v", err)
		}
		if _, ok := c.Provider("openai-home"); ok {
			t.Fatal("LoadDefault() should prefer $XDG_CONFIG_HOME path")
		}
	})

	t.Run("falls back to HOME/.config", func(t *testing.T) {
		// 清掉 XDG 下的文件，只留 home。
		if err := os.RemoveAll(filepath.Join(xdg, "gum-workflows")); err != nil {
			t.Fatalf("clean xdg dir: %v", err)
		}
		writeHomeConfig(t, home, validYAML)

		c, err := LoadDefault()
		if err != nil {
			t.Fatalf("LoadDefault() unexpected error: %v", err)
		}
		if _, ok := c.Provider("openai"); !ok {
			t.Fatal("LoadDefault() should fall back to $HOME/.config/gum-workflows/llm.yaml")
		}
	})
}

func TestLoadDefaultNotFound(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	_, err := LoadDefault()
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("LoadDefault() error = %v, want ErrConfigNotFound", err)
	}
}

func TestLoadDefaultRejectsUnsetHomeAndXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	_, err := LoadDefault()
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("LoadDefault() error = %v, want ErrConfigNotFound when both unset", err)
	}
}
