// Package llm 实现用户级 llm.yaml 的类型、加载与默认解析链
// （设计文档 §3.4，P2 里程碑）。
//
// 本包只含配置结构与解析器，不含任何网络客户端（真实 LLM 调用属
// 后续版本）。密钥以 $VAR 环境变量引用为主，解析后的值绝不落库。
package llm

import (
	"fmt"
	"sort"
)

// APIVersionV1 是 llm/v1 信封的 apiVersion 固定值。
const APIVersionV1 = "llm/v1"

// KindLLM 是 llm/v1 信封的 kind 固定值。
const KindLLM = "llm"

// DefaultTemperature 是 model 级 temperature 的缺省值（设计文档 §3.4）。
const DefaultTemperature = 0.2

// ProviderType 是 provider 的协议类型，二值之一。
type ProviderType string

// ProviderTypeOpenAICompatible 表示 OpenAI 兼容协议的 provider。
const ProviderTypeOpenAICompatible ProviderType = "openai-compatible"

// ProviderTypeAnthropic 表示 Anthropic 协议的 provider。
const ProviderTypeAnthropic ProviderType = "anthropic"

// Model 是 provider 下可选用的具体模型，携带可选生成参数。
// Temperature 为 nil 表示未声明（生效值缺省 0.2，见 DefaultTemperature）；
// 显式 temperature: 0 是合法声明，不以零值判缺省。
type Model struct {
	Name        string   `yaml:"name"`
	Default     bool     `yaml:"default"`
	Temperature *float64 `yaml:"temperature"`
	MaxTokens   int      `yaml:"max_tokens"`
}

// EffectiveTemperature 返回该 model 的生效 temperature：
// 显式声明值或缺省 0.2（设计文档 §3.4：temperature 挂 model 级，缺省 0.2）。
func (m Model) EffectiveTemperature() float64 {
	if m.Temperature == nil {
		return DefaultTemperature
	}
	return *m.Temperature
}

// Provider 是一个大模型服务接入点，下挂一组 Model。
type Provider struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Type        ProviderType `yaml:"type"`
	URL         string       `yaml:"url"`
	APIKey      string       `yaml:"apikey"`
	Default     bool         `yaml:"default"`
	Models      []Model      `yaml:"models"`
}

// Config 是 llm.yaml 的内存形态：providers -> models 嵌套（信封 llm/v1）。
type Config struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Providers  []Provider `yaml:"providers"`
}

// Validate 做语义层检查：
// provider 名唯一、model 名 provider 内唯一、default 各至多一个、
// type 二值之一、url/apikey/models 必填、信封值固定。错误聚合返回。
// 生成参数缺省值的生效规则见 Model.EffectiveTemperature（Validate 不改写数据）。
func (c *Config) Validate() error {
	var errs ValidationErrors

	if c.APIVersion == "" {
		errs = append(errs, fmt.Errorf("apiVersion: must not be empty"))
	} else if c.APIVersion != APIVersionV1 {
		errs = append(errs, fmt.Errorf("apiVersion: %q is not %q", c.APIVersion, APIVersionV1))
	}
	if c.Kind == "" {
		errs = append(errs, fmt.Errorf("kind: must not be empty"))
	} else if c.Kind != KindLLM {
		errs = append(errs, fmt.Errorf("kind: %q is not %q", c.Kind, KindLLM))
	}
	if len(c.Providers) == 0 {
		errs = append(errs, fmt.Errorf("providers: must contain at least one provider"))
		return errs.OrNil()
	}

	defaultProviders := 0
	providerNames := make(map[string]bool, len(c.Providers))
	for _, p := range c.Providers {
		if p.Name == "" {
			errs = append(errs, fmt.Errorf("provider %q: name must not be empty", p.Name))
			continue
		}
		if providerNames[p.Name] {
			errs = append(errs, fmt.Errorf("provider %q: duplicate provider name", p.Name))
		}
		providerNames[p.Name] = true

		if p.Type != ProviderTypeOpenAICompatible && p.Type != ProviderTypeAnthropic {
			errs = append(errs, fmt.Errorf("provider %q: type %q is not one of [%s, %s]",
				p.Name, p.Type, ProviderTypeOpenAICompatible, ProviderTypeAnthropic))
		}
		if p.URL == "" {
			errs = append(errs, fmt.Errorf("provider %q: url must not be empty", p.Name))
		}
		if p.APIKey == "" {
			errs = append(errs, fmt.Errorf("provider %q: apikey must not be empty", p.Name))
		}
		if len(p.Models) == 0 {
			errs = append(errs, fmt.Errorf("provider %q: models must contain at least one model", p.Name))
			continue
		}

		if p.Default {
			defaultProviders++
			if defaultProviders > 1 {
				errs = append(errs, fmt.Errorf("provider %q: at most one provider may be default", p.Name))
			}
		}

		defaultModels := 0
		modelNames := make(map[string]bool, len(p.Models))
		for _, m := range p.Models {
			if m.Name == "" {
				errs = append(errs, fmt.Errorf("provider %q model %q: name must not be empty", p.Name, m.Name))
				continue
			}
			if modelNames[m.Name] {
				errs = append(errs, fmt.Errorf("provider %q model %q: duplicate model name", p.Name, m.Name))
			}
			modelNames[m.Name] = true

			if m.Default {
				defaultModels++
				if defaultModels > 1 {
					errs = append(errs, fmt.Errorf("provider %q: at most one model may be default", p.Name))
				}
			}
		}
	}
	return errs.OrNil()
}

// Provider 按名返回 provider 及其是否找到。
func (c *Config) Provider(name string) (Provider, bool) {
	for _, p := range c.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// ProviderNames 返回全部 provider 名（有序），用于错误信息列出可选项。
func (c *Config) ProviderNames() []string {
	names := make([]string, 0, len(c.Providers))
	for _, p := range c.Providers {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}

// DefaultProvider 返回默认 provider：显式 default 优先，缺省取第一个
// （设计文档 §3.4：默认 provider = 显式 default 或第一个）。
func (c *Config) DefaultProvider() (Provider, bool) {
	if len(c.Providers) == 0 {
		return Provider{}, false
	}
	for _, p := range c.Providers {
		if p.Default {
			return p, true
		}
	}
	return c.Providers[0], true
}
