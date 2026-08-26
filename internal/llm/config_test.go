package llm

import (
	"strings"
	"testing"
)

// t02 是测试共用的 temperature 0.2 声明值。
var t02 = 0.2

// validConfig 是设计文档 §3.4 的示例形态，作为全部测试的基底。
func validConfig() Config {
	return Config{
		APIVersion: APIVersionV1,
		Kind:       KindLLM,
		Providers: []Provider{
			{
				Name:    "openai",
				Type:    ProviderTypeOpenAICompatible,
				URL:     "https://api.openai.com/v1",
				APIKey:  "$OPENAI_API_KEY",
				Default: true,
				Models: []Model{
					{Name: "gpt-4o", Default: true, Temperature: &t02, MaxTokens: 4096},
					{Name: "gpt-4o-mini"},
				},
			},
			{
				Name:   "anthropic",
				Type:   ProviderTypeAnthropic,
				URL:    "https://api.anthropic.com",
				APIKey: "$ANTHROPIC_API_KEY",
				Models: []Model{{Name: "claude-sonnet-5"}},
			},
		},
	}
}

func TestValidateValid(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error:\n%v", err)
	}
}

func TestValidateStructuralEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "empty apiVersion",
			mutate:  func(c *Config) { c.APIVersion = "" },
			wantErr: `apiVersion: must not be empty`,
		},
		{
			name:    "empty kind",
			mutate:  func(c *Config) { c.Kind = "" },
			wantErr: `kind: must not be empty`,
		},
		{
			name:    "wrong apiVersion value",
			mutate:  func(c *Config) { c.APIVersion = "llm/v2" },
			wantErr: `apiVersion: "llm/v2" is not "llm/v1"`,
		},
		{
			name:    "wrong kind value",
			mutate:  func(c *Config) { c.Kind = "providers" },
			wantErr: `kind: "providers" is not "llm"`,
		},
		{
			name:    "no providers",
			mutate:  func(c *Config) { c.Providers = nil },
			wantErr: `providers: must contain at least one provider`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAggregatesAllProblems(t *testing.T) {
	c := validConfig()
	// 两处独立问题应一次报全（DEVELOPMENT.md §4.2 错误聚合）。
	c.Providers[0].URL = ""
	c.Providers[1].APIKey = ""

	err := c.Validate()
	if err == nil {
		t.Fatal("Validate() = nil error, want aggregated errors")
	}
	if got := len(err.(ValidationErrors)); got != 2 {
		t.Fatalf("Validate() returned %d errors, want 2:\n%v", got, err)
	}
}

func TestValidateProviderRules(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "duplicate provider name",
			mutate:  func(c *Config) { c.Providers[1].Name = "openai" },
			wantErr: `provider "openai": duplicate provider name`,
		},
		{
			name:    "empty provider name",
			mutate:  func(c *Config) { c.Providers[0].Name = "" },
			wantErr: `provider "": name must not be empty`,
		},
		{
			name:    "invalid provider type",
			mutate:  func(c *Config) { c.Providers[0].Type = ProviderType("azure") },
			wantErr: `provider "openai": type "azure" is not one of`,
		},
		{
			name:    "empty url",
			mutate:  func(c *Config) { c.Providers[0].URL = "" },
			wantErr: `provider "openai": url must not be empty`,
		},
		{
			name:    "empty apikey",
			mutate:  func(c *Config) { c.Providers[0].APIKey = "" },
			wantErr: `provider "openai": apikey must not be empty`,
		},
		{
			name:    "empty models",
			mutate:  func(c *Config) { c.Providers[0].Models = nil },
			wantErr: `provider "openai": models must contain at least one model`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMultipleDefaultProviders(t *testing.T) {
	c := validConfig()
	c.Providers[1].Default = true

	err := c.Validate()
	if err == nil {
		t.Fatal("Validate() = nil error, want rejection")
	}
	if !strings.Contains(err.Error(), `provider "anthropic": at most one provider may be default`) {
		t.Fatalf("Validate() error = %v, want multiple default provider rejection", err)
	}
}

func TestValidateModelRules(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "duplicate model name within provider",
			mutate:  func(c *Config) { c.Providers[0].Models[1].Name = "gpt-4o" },
			wantErr: `provider "openai" model "gpt-4o": duplicate model name`,
		},
		{
			name:    "same model name across providers is legal",
			mutate:  func(c *Config) { c.Providers[1].Models[0].Name = "gpt-4o" },
			wantErr: "",
		},
		{
			name:    "empty model name",
			mutate:  func(c *Config) { c.Providers[0].Models[0].Name = "" },
			wantErr: `provider "openai" model "": name must not be empty`,
		},
		{
			name:    "multiple default models in one provider",
			mutate:  func(c *Config) { c.Providers[0].Models[1].Default = true },
			wantErr: `provider "openai": at most one model may be default`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error:\n%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEffectiveTemperature(t *testing.T) {
	// temperature 缺省 0.2 经 EffectiveTemperature 生效；显式 0 是合法声明，不被改写。
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if got := c.Providers[1].Models[0].EffectiveTemperature(); got != DefaultTemperature {
		t.Errorf("EffectiveTemperature() of undeclared = %v, want default %v", got, DefaultTemperature)
	}

	zero := 0.0
	declared := Model{Name: "m", Temperature: &zero}
	if got := declared.EffectiveTemperature(); got != 0 {
		t.Errorf("EffectiveTemperature() of explicit 0 = %v, want 0 (not overwritten by default)", got)
	}
}
