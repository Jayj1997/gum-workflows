package validation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// SemanticValidator 是两层校验的第二层（设计计划 §23）：
// 在 CUE 结构校验通过后，结合定义 Registry 与 Executor Registry 检查语义。
// 契约（inputs/outputs）唯一来源是 Node Definition YAML（设计文档 §6.9）。
type SemanticValidator struct {
	executors *node.ExecutorRegistry
	defs      *definition.Registry
	kinds     *artifact.Registry
}

// NewSemanticValidator 创建语义校验器。
// executors 提供已注册的 Go 实现，defs 提供契约（Node Definition YAML），
// kinds 提供已登记的 Artifact Kind。
func NewSemanticValidator(executors *node.ExecutorRegistry, defs *definition.Registry, kinds *artifact.Registry) *SemanticValidator {
	return &SemanticValidator{executors: executors, defs: defs, kinds: kinds}
}

// Validate 校验整份定义，聚合返回全部问题（不在第一个错误处短路）。
func (v *SemanticValidator) Validate(def workflow.Definition) error {
	var errs ValidationErrors

	// 结构层检查（字段格式、dependsOn 局部一致性）。
	if err := def.Validate(); err != nil {
		errs = append(errs, err)
		return errs
	}

	// 逐 Node 确认定义存在、Go 实现存在且 config 合法。
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		if _, err := v.defs.Definition(spec.Node); err != nil {
			errs = append(errs, fmt.Errorf("node %q: unknown node definition %q (registered: %s)",
				id, spec.Node, strings.Join(v.defs.DefinitionNames(), ", ")))
			continue
		}
		f, err := v.executors.Latest(spec.Node)
		if err != nil {
			errs = append(errs, fmt.Errorf("node %q: no executor for %q", id, spec.Node))
			continue
		}
		if _, err := f.Create(node.Config(spec.Config)); err != nil {
			errs = append(errs, fmt.Errorf("node %q: create %q: %w", id, spec.Node, err))
		}
	}
	if len(errs) > 0 {
		// 实例化失败时无法继续做 Contract 级检查。
		return errs
	}

	v.checkContractKinds(def, &errs)
	v.checkInputs(def, &errs)
	v.checkDependsOn(def, &errs)
	v.checkCycle(def, &errs)

	return errs.OrNil()
}

// checkContractKinds 检查各 Node 契约引用的 Artifact Kind 是否已登记
// （含 optional 输入端口--修复旧实现漏检的问题）。
func (v *SemanticValidator) checkContractKinds(def workflow.Definition, errs *ValidationErrors) {
	for _, id := range sortedKeys(def.Nodes) {
		d, err := v.defs.Definition(def.Nodes[id].Node)
		if err != nil {
			continue // 已在前面的实例化检查报出。
		}
		if err := d.ValidateKinds(v.kinds); err != nil {
			*errs = append(*errs, fmt.Errorf("node %q (%s): %w", id, d.Metadata.Name, err))
		}
	}
}

// checkInputs 检查数据连接：引用完整、Input/Output 存在、类型兼容、
// required Input 全部绑定。
func (v *SemanticValidator) checkInputs(def workflow.Definition, errs *ValidationErrors) {
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		consumer, err := v.defs.Definition(spec.Node)
		if err != nil {
			continue
		}

		// required Input 必须全部绑定，否则 Node 永远不会 Ready。
		for _, name := range sortedKeys(consumer.Inputs) {
			if consumer.Inputs[name].Optional {
				continue
			}
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

			producerSpec, ok := def.Nodes[fromNode]
			if !ok {
				*errs = append(*errs, fmt.Errorf("node %q input %q: references unknown node %q", id, name, fromNode))
				continue
			}
			producer, err := v.defs.Definition(producerSpec.Node)
			if err != nil {
				continue // 生产者定义未知已报出。
			}

			outPort, ok := producer.Outputs[fromOutput]
			if !ok {
				*errs = append(*errs, fmt.Errorf("node %q input %q: node %q has no output %q",
					id, name, fromNode, fromOutput))
				continue
			}

			inPort, declared := consumer.Inputs[name]
			if !declared {
				*errs = append(*errs, fmt.Errorf("node %q: input %q is not declared in the contract of definition %q",
					id, name, spec.Node))
				continue
			}
			if !v.compatible(inPort.Type, outPort.Type) {
				*errs = append(*errs, fmt.Errorf("node %q input %q: artifact type mismatch: %s.%s produces %q but %q is expected",
					id, name, fromNode, fromOutput, outPort.Type, inPort.Type))
			}
		}
	}
}

// compatible 解析两个端口类型文本并判断兼容性
// （consumer ⊇ producer，设计文档 §4；无隐式子类型）。
// 语法非法已在定义层校验报出；此处解析失败按不兼容处理并跳过
// 双报（前序错误已定位）。
func (v *SemanticValidator) compatible(consumer, producer string) bool {
	c, err := definition.ParseTypeExpr(consumer)
	if err != nil {
		return true // 语法错误已由定义层校验报出。
	}
	p, err := definition.ParseTypeExpr(producer)
	if err != nil {
		return true
	}
	return definition.Compatible(c, p)
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
