package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/llm"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// SemanticValidator 是两层校验的第二层（设计计划 §23）：
// 在 CUE 结构校验通过后，结合定义 Registry 与 Executor Registry 检查语义，
// 检查项按设计文档 §10 清单。
// 契约（inputs/outputs）唯一来源是 Node Definition YAML（设计文档 §6.9）。
//
// 两项可选注入（Option）：
//   - llm.yaml 解析结果：agent 节点的 llm/target_model 解析依据；
//     未注入且 workflow 含 agent 节点时报错（不含 agent 节点则合法地不需要它）。
//   - workflow 文件路径：projects 相对路径的解析基准；
//     未注入时跳过路径存在检查（内存形态的 Definition 无文件锚点，
//     run 的 Project Runtime 仍会复核路径）。
type SemanticValidator struct {
	executors    *node.ExecutorRegistry
	defs         *definition.Registry
	kinds        *artifact.Registry
	llmConfig    *llm.Config
	workflowFile string
}

// NewSemanticValidator 创建语义校验器。
// executors 提供已注册的 Go 实现，defs 提供契约（Node Definition YAML），
// kinds 提供已登记的 Artifact Kind。
func NewSemanticValidator(executors *node.ExecutorRegistry, defs *definition.Registry, kinds *artifact.Registry, opts ...Option) *SemanticValidator {
	v := &SemanticValidator{executors: executors, defs: defs, kinds: kinds}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Option 配置 SemanticValidator 的可选输入（沿 execution 包的 Option 模式）。
type Option func(*SemanticValidator)

// WithLLMConfig 注入用户级 llm.yaml 的解析结果。
// nil 表示未加载（workflow 含 agent 节点时 Validate 报错，见 checkLLM）。
func WithLLMConfig(c *llm.Config) Option {
	return func(v *SemanticValidator) { v.llmConfig = c }
}

// WithWorkflowFile 注入 workflow 文件路径（projects 相对路径的解析基准）。
// 空串表示跳过路径存在检查。
func WithWorkflowFile(path string) Option {
	return func(v *SemanticValidator) { v.workflowFile = path }
}

// Warning 是校验通过但值得提示的问题（设计文档 §6.7：静态环降为提示）。
// 与错误分开返回：warning 不阻断 run/validate。
type Warning struct {
	// NodeIDs 是提示涉及的节点（如环路径），可为空（workflow 级提示）。
	NodeIDs []string
	// Message 是提示正文（含定位与后果说明）。
	Message string
}

// Validate 校验整份定义，聚合返回全部问题（不在第一个错误处短路）。
// 环检测按设计文档 §10 检查 #10 降为 warning：不含 human 节点的环提示
// 可能死循环（运行期收敛保护兜底），含 human 的环是合法迭代路径不提示。
func (v *SemanticValidator) Validate(def workflow.Definition) ([]Warning, error) {
	var errs ValidationErrors
	var warns []Warning

	// 结构层检查（字段格式、dependsOn 局部一致性）。
	if err := def.Validate(); err != nil {
		errs = append(errs, err)
		return warns, errs
	}

	// 逐 Node 确认定义存在、Go 实现存在且 config 合法。
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		if _, err := v.defs.Definition(spec.Node); err != nil {
			errs = append(errs, fmt.Errorf("node %q: unknown node definition %q (registered: %s)",
				id, spec.Node, strings.Join(v.defs.DefinitionNames(), ", ")))
			continue
		}
		// executor 存在性：显式版本经 Get 精确命中，缺省经 Latest 解析
		// （设计文档 §10 检查 #2）。
		var f node.ExecutorFactory
		var err error
		if spec.Executor != "" {
			f, err = v.executors.Get(spec.Node, spec.Executor)
			if err != nil {
				errs = append(errs, fmt.Errorf("node %q executor: %w", id, err))
				continue
			}
		} else {
			f, err = v.executors.Latest(spec.Node)
			if err != nil {
				errs = append(errs, fmt.Errorf("node %q: %w", id, err))
				continue
			}
		}
		if _, err := f.Create(node.Config(spec.Config)); err != nil {
			errs = append(errs, fmt.Errorf("node %q: create %q: %w", id, spec.Node, err))
		}
		if host, ok := f.(node.HostRequirementValidator); ok {
			if err := host.ValidateHostRequirements(); err != nil {
				errs = append(errs, fmt.Errorf("node %q: executor host requirements: %w", id, err))
			}
		}
	}
	if len(errs) > 0 {
		// 实例化失败时无法继续做 Contract 级检查。
		return warns, errs
	}

	v.checkContractKinds(def, &errs)
	v.checkWorkflowContextNames(def, &errs)
	v.checkInputs(def, &errs)
	v.checkDependsOn(def, &errs)
	v.checkEntry(def, &errs)
	v.checkHumanControlEdge(def, &errs)
	v.checkLLM(def, &errs)
	v.checkProjects(def, &errs)
	warns = append(warns, v.checkAgentAdvise(def)...)
	warns = append(warns, v.checkCycle(def)...)

	return warns, errs.OrNil()
}

func (v *SemanticValidator) checkWorkflowContextNames(def workflow.Definition, errs *ValidationErrors) {
	for _, id := range sortedKeys(def.Nodes) {
		if workflow.IsWorkflowContext(id) {
			*errs = append(*errs, fmt.Errorf("node %q: ID is reserved for workflow context bindings", id))
		}
	}
}

func (v *SemanticValidator) checkAgentAdvise(def workflow.Definition) []Warning {
	var warnings []Warning
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		d, err := v.defs.Definition(spec.Node)
		if err != nil || d.Type != definition.TypeAgent {
			continue
		}
		if _, declared := d.Inputs["advise"]; declared {
			continue
		}
		warnings = append(warnings, Warning{
			NodeIDs: []string{id},
			Message: fmt.Sprintf(
				"node %q: agent definition %q does not declare input %q; interaction errors cannot be retried with advise",
				id, spec.Node, "advise"),
		})
	}
	return warnings
}

// checkEntry enforces the single human source that starts every workflow.
// A source has neither data inputs nor control dependencies.
func (v *SemanticValidator) checkEntry(def workflow.Definition, errs *ValidationErrors) {
	var sources []string
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		if len(spec.Inputs) == 0 && len(spec.DependsOn) == 0 {
			sources = append(sources, id)
		}
	}
	if len(sources) != 1 {
		*errs = append(*errs, fmt.Errorf(
			"workflow entry: expected exactly 1 source node, got %d (%s)",
			len(sources), strings.Join(sources, ", ")))
		return
	}

	id := sources[0]
	d, err := v.defs.Definition(def.Nodes[id].Node)
	if err != nil {
		return // Unknown definitions are reported before contract checks.
	}
	if d.Type != definition.TypeHuman {
		*errs = append(*errs, fmt.Errorf(
			"workflow entry: source node %q must have type human: field node references definition %q with type %q",
			id, def.Nodes[id].Node, d.Type))
	}
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

		for _, name := range sortedKeys(spec.Inputs) {
			binding := spec.Inputs[name]
			fromNode, fromOutput, err := workflow.ParseRef(binding.From)
			if err != nil {
				*errs = append(*errs, fmt.Errorf("node %q input %q: %w", id, name, err))
				continue
			}

			inPort, declared := consumer.Inputs[name]
			if !declared {
				*errs = append(*errs, fmt.Errorf("node %q: input %q is not declared in the contract of definition %q",
					id, name, spec.Node))
				continue
			}

			if workflow.IsWorkflowContext(fromNode) {
				outType, ok := workflow.WorkflowContextOutputType(fromNode, fromOutput)
				if !ok {
					*errs = append(*errs, fmt.Errorf("node %q input %q: workflow context %q has no output %q",
						id, name, fromNode, fromOutput))
					continue
				}
				if !v.compatible(inPort.Type, outType) {
					*errs = append(*errs, fmt.Errorf("node %q input %q: artifact type mismatch: %s.%s produces %q but %q is expected",
						id, name, fromNode, fromOutput, outType, inPort.Type))
				}
				continue
			}

			producerSpec, ok := def.Nodes[fromNode]
			if !ok {
				*errs = append(*errs, fmt.Errorf("node %q input %q: references unknown node or workflow context %q", id, name, fromNode))
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

// checkHumanControlEdge 检查 human-approval 必须经 Control Edge 挂接被审节点。
// human-input 是唯一合法的 human 源节点，由入口规则单独约束。
func (v *SemanticValidator) checkHumanControlEdge(def workflow.Definition, errs *ValidationErrors) {
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		d, err := v.defs.Definition(spec.Node)
		if err != nil {
			continue // 定义未知已报出。
		}
		if d.Type != definition.TypeHuman || spec.Node != "human-approval" {
			continue
		}
		if len(spec.DependsOn) == 0 {
			*errs = append(*errs, fmt.Errorf(
				"node %q: human-approval must declare non-empty dependsOn",
				id))
		}
	}
}

// checkLLM 检查 llm/target_model 的合法性（设计文档 §10 检查 #9）：
// 仅 agent 类节点可填；agent 节点的引用经 resolver 解析（provider/model
// 存在且归属正确）。llm.yaml 是 agent 节点的运行前提：未注入即视为
// 未找到（含 agent 节点的 workflow 报错，纯 automation/human 不需要它）。
func (v *SemanticValidator) checkLLM(def workflow.Definition, errs *ValidationErrors) {
	hasAgent := false
	for _, id := range sortedKeys(def.Nodes) {
		spec := def.Nodes[id]
		d, err := v.defs.Definition(spec.Node)
		if err != nil {
			continue // 定义未知已报出。
		}

		isAgent := d.Type == definition.TypeAgent
		if !isAgent {
			// 逐字段报错：两个非法字段各自定位（聚合风格）。
			if spec.LLM != "" {
				*errs = append(*errs, fmt.Errorf(
					"node %q: llm is only valid on agent nodes (definition %q has type %q)",
					id, spec.Node, d.Type))
			}
			if spec.TargetModel != "" {
				*errs = append(*errs, fmt.Errorf(
					"node %q: target_model is only valid on agent nodes (definition %q has type %q)",
					id, spec.Node, d.Type))
			}
			continue
		}

		// agent 节点统一走默认链（含两字段都空的缺省解析：
		// 默认 provider 的默认 model 也必须在 llm.yaml 里可达，
		// 否则 run 时才失败就晚了）。
		hasAgent = true
		if v.llmConfig == nil {
			continue // 缺 llm.yaml 的错误在收齐全部 agent 节点后统一报出。
		}
		if _, err := v.llmConfig.Resolve(spec.LLM, spec.TargetModel); err != nil {
			*errs = append(*errs, fmt.Errorf("node %q: %w", id, err))
		}
	}

	if hasAgent && v.llmConfig == nil {
		*errs = append(*errs, fmt.Errorf(
			"workflow contains agent nodes but no llm.yaml was found (searched: %s); create one to run agent definitions",
			strings.Join(llm.CandidatePaths(), " -> ")))
	}
}

// checkProjects 检查 projects 数量与路径（设计文档 §10 检查 #11）：
// 本期恰好 1 个；相对路径相对 workflow 文件所在目录，路径必须存在且为目录。
// 未注入 workflow 文件路径时跳过路径检查（无文件锚点可依）。
func (v *SemanticValidator) checkProjects(def workflow.Definition, errs *ValidationErrors) {
	if len(def.Projects) != 1 {
		*errs = append(*errs, fmt.Errorf("projects: must contain exactly 1 entry, got %d", len(def.Projects)))
		return
	}
	if v.workflowFile == "" {
		return
	}

	p := def.Projects[0]
	repo := p.Repository
	if !filepath.IsAbs(repo) {
		absWorkflow, err := filepath.Abs(v.workflowFile)
		if err != nil {
			*errs = append(*errs, fmt.Errorf("projects[0] %q: resolve path: %w", p.Name, err))
			return
		}
		repo = filepath.Join(filepath.Dir(absWorkflow), repo)
	}
	info, err := os.Stat(repo)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("projects[0] %q: repository %q: %w", p.Name, repo, err))
		return
	}
	if !info.IsDir() {
		*errs = append(*errs, fmt.Errorf("projects[0] %q: repository %q is not a directory", p.Name, repo))
	}
}

// checkCycle 检查 Data Edge 与 Control Edge 合并后的环（设计文档 §6.7、
// §10 检查 #10：从错误降为提示）。环上含 human 类节点 -> 合法迭代路径，
// 不提示；不含 human -> warning「可能死循环（收敛保护兜底）」。
func (v *SemanticValidator) checkCycle(def workflow.Definition) []Warning {
	g, err := workflow.BuildGraph(def)
	if err != nil {
		// 畸形引用已在 checkInputs/checkDependsOn 报出；此处防御即可。
		return nil
	}
	cycle := g.Cycle()
	if cycle == nil {
		return nil
	}

	// 环路径首尾相同（Cycle 契约），判定节点集不含环内重复的首节点。
	nodes := cycle[:len(cycle)-1]
	for _, id := range nodes {
		d, err := v.defs.Definition(def.Nodes[id].Node)
		if err == nil && d.Type == definition.TypeHuman {
			return nil // 含 human 环：合法迭代路径，不提示。
		}
	}
	return []Warning{{
		NodeIDs: nodes,
		Message: fmt.Sprintf("dependency cycle without human nodes may loop forever (convergence guard will stop it): %s",
			strings.Join(cycle, " -> ")),
	}}
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
