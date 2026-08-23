package validation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// SemanticValidator 是两层校验的第二层（设计计划 §23）：
// 在 CUE 结构校验通过后，结合 Node Registry 检查语义。
type SemanticValidator struct {
	nodes *node.Registry
	kinds *artifact.Registry
}

// NewSemanticValidator 创建语义校验器。
// nodes 提供已注册的 Node Type（及其 Input/Output Contract），kinds 提供已登记的 Artifact Kind。
func NewSemanticValidator(nodes *node.Registry, kinds *artifact.Registry) *SemanticValidator {
	return &SemanticValidator{nodes: nodes, kinds: kinds}
}

// Validate 校验整份定义，聚合返回全部问题（不在第一个错误处短路）。
func (v *SemanticValidator) Validate(def workflow.Definition) error {
	var errs ValidationErrors

	// 结构层检查（字段格式、dependsOn 局部一致性）。
	if err := def.Validate(); err != nil {
		errs = append(errs, err)
		return errs
	}

	// 实例化全部 Node 以获得 Input/Output Contract。
	instances := make(map[string]node.Node, len(def.Nodes))
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		f, ok := v.nodes.Get(spec.Type)
		if !ok {
			errs = append(errs, fmt.Errorf("node %q: unknown node type %q (registered: %s)",
				id, spec.Type, strings.Join(v.nodes.Types(), ", ")))
			continue
		}
		n, err := f.Create(node.Config(spec.Config))
		if err != nil {
			errs = append(errs, fmt.Errorf("node %q: create %q: %w", id, spec.Type, err))
			continue
		}
		instances[id] = n
	}
	if len(errs) > 0 {
		// Node 实例化失败时无法继续做 Contract 级检查。
		return errs
	}

	v.checkSchemaKinds(instances, &errs)
	v.checkInputs(def, instances, &errs)
	v.checkDependsOn(def, &errs)
	v.checkCycle(def, &errs)

	return errs.OrNil()
}

// checkSchemaKinds 检查各 Node Contract 引用的 Artifact Kind 是否已登记。
func (v *SemanticValidator) checkSchemaKinds(instances map[string]node.Node, errs *ValidationErrors) {
	for _, id := range sortedKeys(instances) {
		n := instances[id]
		for name, k := range n.InputSchema().Inputs {
			if !v.kinds.Has(k) {
				*errs = append(*errs, fmt.Errorf("node %q: input %q declares unregistered artifact kind %q", id, name, k))
			}
		}
		for name, k := range n.OutputSchema().Outputs {
			if !v.kinds.Has(k) {
				*errs = append(*errs, fmt.Errorf("node %q: output %q declares unregistered artifact kind %q", id, name, k))
			}
		}
	}
}

// checkInputs 检查数据连接：引用完整、Input/Output 存在、Kind 匹配、required Input 全部绑定。
func (v *SemanticValidator) checkInputs(def workflow.Definition, instances map[string]node.Node, errs *ValidationErrors) {
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		inputSchema := instances[id].InputSchema()

		// required Input 必须全部绑定，否则 Node 永远不会 Ready。
		for name := range inputSchema.Inputs {
			if _, bound := spec.Inputs[name]; !bound {
				*errs = append(*errs, fmt.Errorf("node %q: required input %q is not bound", id, name))
			}
		}

		for name, binding := range spec.Inputs {
			fromNode, fromOutput, err := workflow.ParseRef(binding.From)
			if err != nil {
				*errs = append(*errs, fmt.Errorf("node %q input %q: %w", id, name, err))
				continue
			}

			producer, ok := instances[fromNode]
			if !ok {
				*errs = append(*errs, fmt.Errorf("node %q input %q: references unknown node %q", id, name, fromNode))
				continue
			}

			outKind, ok := producer.OutputSchema().Outputs[fromOutput]
			if !ok {
				*errs = append(*errs, fmt.Errorf("node %q input %q: node %q has no output %q",
					id, name, fromNode, fromOutput))
				continue
			}

			inKind, declared := inputSchema.Inputs[name]
			if !declared {
				inKind, declared = inputSchema.OptionalInputs[name]
			}
			if !declared {
				*errs = append(*errs, fmt.Errorf("node %q: input %q is not declared in the InputSchema of type %q",
					id, name, spec.Type))
				continue
			}
			if inKind != outKind {
				*errs = append(*errs, fmt.Errorf("node %q input %q: artifact kind mismatch: %s.%s produces %q but %q is expected",
					id, name, fromNode, fromOutput, outKind, inKind))
			}
		}
	}
}

// checkDependsOn 检查 Control Edge 引用完整（dependsOn 可选，仅声明时检查）。
func (v *SemanticValidator) checkDependsOn(def workflow.Definition, errs *ValidationErrors) {
	for _, id := range sortedKeys(def.Nodes) {
		for _, dep := range def.Nodes[id].DependsOn {
			if _, ok := def.Nodes[dep]; !ok {
				*errs = append(*errs, fmt.Errorf("node %q: dependsOn unknown node %q", id, dep))
			}
		}
	}
}

// checkCycle 检查 Data Edge 与 Control Edge 合并后的环（设计计划 §36）。
func (v *SemanticValidator) checkCycle(def workflow.Definition, errs *ValidationErrors) {
	g, err := workflow.BuildGraph(def)
	if err != nil {
		*errs = append(*errs, err)
		return
	}
	if cycle := g.Cycle(); cycle != nil {
		*errs = append(*errs, fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> ")))
	}
}

// sortedKeys 返回 map key 的有序列表（保证校验错误顺序稳定）。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
