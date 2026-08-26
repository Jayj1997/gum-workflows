package llm

import (
	"fmt"
	"sort"
	"strings"
)

// Reference 是 Node Instance 的 llm/target_model 引用经解析链固定的结果：
// agent 节点实际使用的 provider 与 model（运行记录只记名字，设计文档 §8.3）。
type Reference struct {
	Provider string
	Model    string

	// ModelConfig 携带该 model 声明的生成参数（temperature 等，
	// 已补齐缺省值），供后续真实 Agent Adapter 消费。
	ModelConfig Model
}

// Resolve 实现默认解析链四象限（设计文档 §3.4）：
//
//	llm 填 + model 填 -> 该 provider 的该 model（必须归属它）
//	llm 填 + model 空 -> 该 provider 的默认 model
//	llm 空 + model 填 -> 默认 provider 中找该 model 名（找不到提示补 llm）
//	llm 空 + model 空 -> 默认 provider 的默认 model
//
// 错误信息定位到字段名（llm / target_model），列出可用选项。
func (c *Config) Resolve(llm, targetModel string) (Reference, error) {
	provider, err := c.resolveProvider(llm)
	if err != nil {
		return Reference{}, err
	}

	var model Model
	if targetModel == "" {
		model, err = defaultModel(provider)
		if err != nil {
			return Reference{}, err
		}
	} else {
		model, err = findModel(provider, targetModel)
		if err != nil {
			if llm == "" {
				// 默认链路径下找不到：模型可能属于其他 provider，提示补 llm。
				return Reference{}, fmt.Errorf(
					"target_model: default provider %q has no model %q (models: %s); add %q to select another provider",
					provider.Name, targetModel, modelNames(provider), "llm")
			}
			return Reference{}, err
		}
	}

	return Reference{
		Provider:    provider.Name,
		Model:       model.Name,
		ModelConfig: model,
	}, nil
}

// resolveProvider 选定 provider：显式 llm 优先，否则默认 provider。
func (c *Config) resolveProvider(llm string) (Provider, error) {
	if llm != "" {
		p, ok := c.Provider(llm)
		if !ok {
			return Provider{}, fmt.Errorf("llm: unknown provider %q (available: %s)",
				llm, strings.Join(c.ProviderNames(), ", "))
		}
		return p, nil
	}
	p, ok := c.DefaultProvider()
	if !ok {
		return Provider{}, fmt.Errorf("providers: must contain at least one provider")
	}
	return p, nil
}

// defaultModel 返回 provider 的默认 model：显式 default 优先，缺省取第一个。
func defaultModel(p Provider) (Model, error) {
	if len(p.Models) == 0 {
		return Model{}, fmt.Errorf("provider %q: models must contain at least one model", p.Name)
	}
	for _, m := range p.Models {
		if m.Default {
			return m, nil
		}
	}
	return p.Models[0], nil
}

// findModel 在 provider 内按名查找 model。
func findModel(p Provider, name string) (Model, error) {
	for _, m := range p.Models {
		if m.Name == name {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("target_model: provider %q has no model %q (models: %s)",
		p.Name, name, modelNames(p))
}

// modelNames 返回 provider 内全部 model 名（有序），用于错误信息列出可选项。
func modelNames(p Provider) string {
	names := make([]string, 0, len(p.Models))
	for _, m := range p.Models {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
