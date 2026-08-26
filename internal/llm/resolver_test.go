package llm

import (
	"strings"
	"testing"
)

// resolverBase 是四象限测试的基底：openai 为显式默认 provider（gpt-4o 默认 model），
// anthropic 第二个（无 default 标记，其 claude-sonnet-5 也不是默认）。
func resolverBase(t *testing.T) Config {
	t.Helper()
	t.Setenv("TEST_OPENAI_KEY", "k1")
	t.Setenv("TEST_ANTHROPIC_KEY", "k2")
	c, err := Load([]byte(validYAML))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	return c
}

func TestResolveFourQuadrants(t *testing.T) {
	tests := []struct {
		name        string
		llm         string
		targetModel string
		wantErr     string
		want        Reference
	}{
		{
			name:        "llm set + model set -> that provider's that model",
			llm:         "anthropic",
			targetModel: "claude-sonnet-5",
			want:        Reference{Provider: "anthropic", Model: "claude-sonnet-5"},
		},
		{
			name: "llm set + model empty -> provider's default model",
			llm:  "anthropic",
			want: Reference{Provider: "anthropic", Model: "claude-sonnet-5"},
		},
		{
			name:        "llm empty + model set -> default provider's model by name",
			targetModel: "gpt-4o-mini",
			want:        Reference{Provider: "openai", Model: "gpt-4o-mini"},
		},
		{
			name: "llm empty + model empty -> default provider's default model",
			want: Reference{Provider: "openai", Model: "gpt-4o"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := resolverBase(t)
			got, err := c.Resolve(tt.llm, tt.targetModel)
			if err != nil {
				t.Fatalf("Resolve(%q, %q) unexpected error: %v", tt.llm, tt.targetModel, err)
			}
			if got.Provider != tt.want.Provider || got.Model != tt.want.Model {
				t.Errorf("Resolve(%q, %q) = {%s %s}, want {%s %s}",
					tt.llm, tt.targetModel, got.Provider, got.Model, tt.want.Provider, tt.want.Model)
			}
		})
	}
}

func TestResolveExplicitProviderModelDefaulting(t *testing.T) {
	// 无显式 default model 时取 provider 内第一个（设计文档 §3.4）。
	yaml := `
apiVersion: llm/v1
kind: llm
providers:
  - name: p
    type: openai-compatible
    url: http://localhost
    apikey: plain
    models:
      - name: first
      - name: second
`
	c, err := Load([]byte(yaml))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	got, err := c.Resolve("p", "")
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if got.Provider != "p" || got.Model != "first" {
		t.Errorf("Resolve() = {%s %s}, want {p first} (first model when none marked default)", got.Provider, got.Model)
	}
}

func TestResolveFirstProviderIsDefault(t *testing.T) {
	// 无显式 default provider 时取第一个。
	yaml := `
apiVersion: llm/v1
kind: llm
providers:
  - name: alpha
    type: openai-compatible
    url: http://alpha
    apikey: plain
    models:
      - name: a1
        default: true
  - name: beta
    type: anthropic
    url: http://beta
    apikey: plain
    models:
      - name: b1
`
	c, err := Load([]byte(yaml))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	got, err := c.Resolve("", "")
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if got.Provider != "alpha" || got.Model != "a1" {
		t.Errorf("Resolve() = {%s %s}, want {alpha a1} (first provider when none marked default)", got.Provider, got.Model)
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		name        string
		llm         string
		targetModel string
		wantErr     string
	}{
		{
			name:        "unknown provider",
			llm:         "azure",
			targetModel: "gpt-4o",
			wantErr:     `llm: unknown provider "azure" (available: anthropic, openai)`,
		},
		{
			name:        "model does not belong to named provider",
			llm:         "openai",
			targetModel: "claude-sonnet-5",
			wantErr:     `target_model: provider "openai" has no model "claude-sonnet-5" (models: gpt-4o, gpt-4o-mini)`,
		},
		{
			name:        "model not in default provider suggests llm field",
			targetModel: "claude-sonnet-5",
			wantErr:     `target_model: default provider "openai" has no model "claude-sonnet-5" (models: gpt-4o, gpt-4o-mini); add "llm" to select another provider`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := resolverBase(t)
			_, err := c.Resolve(tt.llm, tt.targetModel)
			if err == nil {
				t.Fatalf("Resolve(%q, %q) = nil error, want rejection containing %q", tt.llm, tt.targetModel, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Resolve() error =\n%v\nwant containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveReturnsEffectiveModelParams(t *testing.T) {
	c := resolverBase(t)

	// gpt-4o 声明了 temperature 0.2 与 max_tokens 4096。
	ref, err := c.Resolve("", "")
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if got := ref.ModelConfig.EffectiveTemperature(); got != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", got)
	}
	if ref.ModelConfig.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %v, want 4096", ref.ModelConfig.MaxTokens)
	}

	// gpt-4o-mini 未声明 temperature -> 生效缺省 0.2；MaxTokens 未声明为 0。
	ref, err = c.Resolve("", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if got := ref.ModelConfig.EffectiveTemperature(); got != DefaultTemperature {
		t.Errorf("Temperature = %v, want default %v", got, DefaultTemperature)
	}
	if ref.ModelConfig.MaxTokens != 0 {
		t.Errorf("MaxTokens = %v, want 0 (unset)", ref.ModelConfig.MaxTokens)
	}
}

func TestResolveModelNamesHelper(t *testing.T) {
	c := resolverBase(t)

	// 错误信息辅助方法：model 名按序排列，便于「应补什么」类提示。
	p, ok := c.Provider("openai")
	if !ok {
		t.Fatal("Provider(openai) not found")
	}
	got := modelNames(p)
	want := "gpt-4o, gpt-4o-mini"
	if got != want {
		t.Errorf("modelNames() = %q, want %q", got, want)
	}
}
