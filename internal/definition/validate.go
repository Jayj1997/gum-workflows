package definition

import (
	"fmt"
	"strings"
)

// ValidationErrors 聚合一次校验发现的全部问题，
// 一次报出而不是在第一个错误处短路（见 docs/DEVELOPMENT.md §4.2）。
type ValidationErrors []error

// Error 将全部错误逐行拼接。
func (e ValidationErrors) Error() string {
	msgs := make([]string, len(e))
	for i, err := range e {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "\n")
}

// OrNil 在无错误时返回 nil，便于作为 error 返回值。
func (e ValidationErrors) OrNil() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

// Validate 检查 Node Type Definition 声明（设计文档 §3.1）：
// 信封值固定、metadata 必填、name ∈ 三值、requires 合法值。
// 「全局恰三个」由 Registry 的种子加载保证，单份声明不检查数量。
func (t NodeTypeDefinition) Validate() error {
	var errs ValidationErrors

	errs = append(errs, validateEnvelope(t.APIVersion, NodeTypeAPIVersionV1, t.Kind, NodeTypeDefinitionKind)...)
	if t.Metadata.Name == "" {
		errs = append(errs, fmt.Errorf("metadata.name: must not be empty"))
	} else if !isNodeTypeName(t.Metadata.Name) {
		errs = append(errs, fmt.Errorf("metadata.name: %q is not one of %s",
			t.Metadata.Name, joinQuote(nodeTypeNames())))
	}
	if t.Metadata.Description == "" {
		errs = append(errs, fmt.Errorf("metadata.description: must not be empty"))
	}
	errs = append(errs, validateRequires(t.Metadata.Name, t.Requires)...)

	return errs.OrNil()
}

// Validate 检查 Node Definition 声明（设计文档 §3.2）：
// 信封值固定、metadata 必填、type 引用三值之一、requires 合法值、
// 各端口 TypeExpr 语法合法。Kind 注册检查需要 artifact.Registry，
// 由 ValidateKinds 独立承接（解析与校验分离，见 ParseTypeExpr 文档）。
func (d NodeDefinition) Validate() error {
	var errs ValidationErrors

	errs = append(errs, validateEnvelope(d.APIVersion, NodeDefinitionAPIVersionV1, d.Kind, NodeDefinitionKind)...)
	if d.Metadata.Name == "" {
		errs = append(errs, fmt.Errorf("metadata.name: must not be empty"))
	}
	if d.Metadata.Description == "" {
		errs = append(errs, fmt.Errorf("metadata.description: must not be empty"))
	}
	if d.Type == "" {
		errs = append(errs, fmt.Errorf("node definition %q: type must not be empty", d.Metadata.Name))
	} else if !isNodeTypeName(string(d.Type)) {
		errs = append(errs, fmt.Errorf("node definition %q: type %q is not one of %s",
			d.Metadata.Name, d.Type, joinQuote(nodeTypeNames())))
	}
	errs = append(errs, validateRequires(d.Metadata.Name, d.Requires)...)

	for _, name := range sortedKeys(d.Inputs) {
		port := d.Inputs[name]
		if port.Type == "" {
			errs = append(errs, fmt.Errorf("node definition %q input %q: type must not be empty",
				d.Metadata.Name, name))
			continue
		}
		if _, err := ParseTypeExpr(port.Type); err != nil {
			errs = append(errs, fmt.Errorf("node definition %q input %q: %w",
				d.Metadata.Name, name, err))
		}
	}
	for _, name := range sortedKeys(d.Outputs) {
		port := d.Outputs[name]
		if port.Type == "" {
			errs = append(errs, fmt.Errorf("node definition %q output %q: type must not be empty",
				d.Metadata.Name, name))
			continue
		}
		if _, err := ParseTypeExpr(port.Type); err != nil {
			errs = append(errs, fmt.Errorf("node definition %q output %q: %w",
				d.Metadata.Name, name, err))
		}
	}

	return errs.OrNil()
}

// Validate 检查 Node Executor Definition 声明（设计文档 §3.3）：
// 信封值固定、node（所属 Node Definition 名）与 version 必填。
// metadata 为展示用选填；「version 同定义内唯一」与「node 存在」
// 是跨声明的集合级检查，由 Registry 的加载函数承接。
func (e NodeExecutorDefinition) Validate() error {
	var errs ValidationErrors

	errs = append(errs, validateEnvelope(e.APIVersion, NodeExecutorAPIVersionV1, e.Kind, NodeExecutorKind)...)
	if e.Node == "" {
		errs = append(errs, fmt.Errorf("executor %q: node must not be empty", e.Metadata.Name))
	}
	if e.Version == "" {
		errs = append(errs, fmt.Errorf("executor %q: version must not be empty", e.Metadata.Name))
	}

	return errs.OrNil()
}

// validateRequires 检查 requires 列表的合法值（v1：llm、project）
// 与重复条目，owner 用于错误定位（Node Type 或 Node Definition 的 name）。
func validateRequires(owner string, requires []Requirement) ValidationErrors {
	var errs ValidationErrors
	seen := make(map[Requirement]bool, len(requires))
	for _, req := range requires {
		if req != RequireLLM && req != RequireProject {
			errs = append(errs, fmt.Errorf("%s: requires entry %q is not one of [%s, %s]",
				owner, req, RequireLLM, RequireProject))
		}
		if seen[req] {
			errs = append(errs, fmt.Errorf("%s: duplicate requires entry %q", owner, req))
		}
		seen[req] = true
	}
	return errs
}

// validateEnvelope 检查三类声明共用的信封字段：
// apiVersion/kind 非空且等于固定值（设计文档 §2 组件总览表）。
func validateEnvelope(apiVersion, wantAPIVersion, kind, wantKind string) ValidationErrors {
	var errs ValidationErrors
	if apiVersion == "" {
		errs = append(errs, fmt.Errorf("apiVersion: must not be empty"))
	} else if apiVersion != wantAPIVersion {
		errs = append(errs, fmt.Errorf("apiVersion: %q is not %q", apiVersion, wantAPIVersion))
	}
	if kind == "" {
		errs = append(errs, fmt.Errorf("kind: must not be empty"))
	} else if kind != wantKind {
		errs = append(errs, fmt.Errorf("kind: %q is not %q", kind, wantKind))
	}
	return errs
}
