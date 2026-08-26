package definition

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ErrNotFound 是三类定义查询未命中的哨兵错误
// （DEVELOPMENT.md §4.2：可编程判断用 errors.Is）。
var ErrNotFound = errors.New("definition not found")

// Registry 是定义侧三类组件的内存注册表（设计文档 §3.1–§3.3）：
//
//   - Node Type 按 name 寻址（全局恰三个种子）；
//   - Node Definition 按 name 寻址（契约唯一来源）；
//   - Node Executor 按 (definition name, version) 寻址，Latest 取最大版本。
//
// 注册一律显式调用（禁止 init() 隐式注册，DEVELOPMENT.md §3）；
// 重复注册视为编程错误。
type Registry struct {
	nodeTypes map[string]NodeTypeDefinition
	defs      map[string]NodeDefinition
	execs     map[string]map[string]NodeExecutorDefinition // node -> version -> executor
}

// NewRegistry 创建空注册表。种子数据经 LoadSeeds 填充。
func NewRegistry() *Registry {
	return &Registry{
		nodeTypes: map[string]NodeTypeDefinition{},
		defs:      map[string]NodeDefinition{},
		execs:     map[string]map[string]NodeExecutorDefinition{},
	}
}

// RegisterNodeType 注册一份 Node Type Definition，name 重复即报错。
func (r *Registry) RegisterNodeType(t NodeTypeDefinition) error {
	if _, ok := r.nodeTypes[t.Metadata.Name]; ok {
		return fmt.Errorf("register node type: %q already registered", t.Metadata.Name)
	}
	r.nodeTypes[t.Metadata.Name] = t
	return nil
}

// RegisterDefinition 注册一份 Node Definition，name 重复即报错。
// type 引用的 Node Type 必须已注册（§3.2 校验：type ∈ 已注册三值）。
func (r *Registry) RegisterDefinition(d NodeDefinition) error {
	if _, ok := r.defs[d.Metadata.Name]; ok {
		return fmt.Errorf("register node definition: %q already registered", d.Metadata.Name)
	}
	if _, ok := r.nodeTypes[string(d.Type)]; !ok {
		return fmt.Errorf("register node definition %q: unknown node type %q (registered: %s)",
			d.Metadata.Name, d.Type, joinQuote(r.NodeTypeNames()))
	}
	r.defs[d.Metadata.Name] = d
	return nil
}

// RegisterExecutor 注册一份 Node Executor Definition。
// node 引用的 Node Definition 必须已注册；(definition, version)
// 重复即报错（同一定义多版本并存，版本是唯一性维度）。
func (r *Registry) RegisterExecutor(e NodeExecutorDefinition) error {
	if _, ok := r.defs[e.Node]; !ok {
		return fmt.Errorf("register node executor %q: unknown node definition %q (registered: %s)",
			e.Metadata.Name, e.Node, joinQuote(r.DefinitionNames()))
	}
	versions, ok := r.execs[e.Node]
	if !ok {
		versions = map[string]NodeExecutorDefinition{}
		r.execs[e.Node] = versions
	}
	if _, ok := versions[e.Version]; ok {
		return fmt.Errorf("register node executor: (%s, %s) already registered",
			e.Node, e.Version)
	}
	versions[e.Version] = e
	return nil
}

// NodeType 按 name 返回 Node Type Definition。
func (r *Registry) NodeType(name string) (NodeTypeDefinition, error) {
	t, ok := r.nodeTypes[name]
	if !ok {
		return NodeTypeDefinition{}, fmt.Errorf("%w: node type %q (registered: %s)",
			ErrNotFound, name, joinQuote(r.NodeTypeNames()))
	}
	return t, nil
}

// NodeTypeNames 返回全部 Node Type 名（有序）。
func (r *Registry) NodeTypeNames() []string {
	names := make([]string, 0, len(r.nodeTypes))
	for name := range r.nodeTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Definition 按 name 返回 Node Definition。
func (r *Registry) Definition(name string) (NodeDefinition, error) {
	d, ok := r.defs[name]
	if !ok {
		return NodeDefinition{}, fmt.Errorf("%w: node definition %q (registered: %s)",
			ErrNotFound, name, joinQuote(r.DefinitionNames()))
	}
	return d, nil
}

// DefinitionNames 返回全部 Node Definition 名（有序）。
func (r *Registry) DefinitionNames() []string {
	names := make([]string, 0, len(r.defs))
	for name := range r.defs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Executor 按 (definition name, version) 返回 Node Executor Definition。
func (r *Registry) Executor(definition, version string) (NodeExecutorDefinition, error) {
	versions, ok := r.execs[definition]
	if !ok {
		return NodeExecutorDefinition{}, fmt.Errorf("%w: no executor for node definition %q (registered: %s)",
			ErrNotFound, definition, joinQuote(r.DefinitionNames()))
	}
	e, ok := versions[version]
	if !ok {
		return NodeExecutorDefinition{}, fmt.Errorf("%w: executor %q of node definition %q (versions: %s)",
			ErrNotFound, version, definition, joinQuote(r.ExecutorVersions(definition)))
	}
	return e, nil
}

// ExecutorVersions 返回某 Node Definition 的全部 executor 版本（有序）。
// definition 不存在时返回 nil。
func (r *Registry) ExecutorVersions(definition string) []string {
	versions, ok := r.execs[definition]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(versions))
	for v := range versions {
		names = append(names, v)
	}
	sort.Strings(names)
	return names
}

// Latest 返回某 Node Definition 的最新 executor 版本。
// 版本形如 v1、v2……（§3.3）：比较按 v 后的数字大小，数字相同按字典序，
// 避免字典序把 v10 排在 v2 之前。
func (r *Registry) Latest(definition string) (NodeExecutorDefinition, error) {
	versions := r.ExecutorVersions(definition)
	if len(versions) == 0 {
		if _, ok := r.defs[definition]; !ok {
			return NodeExecutorDefinition{}, fmt.Errorf("%w: node definition %q (registered: %s)",
				ErrNotFound, definition, joinQuote(r.DefinitionNames()))
		}
		return NodeExecutorDefinition{}, fmt.Errorf("%w: no executor for node definition %q",
			ErrNotFound, definition)
	}
	latest := versions[0]
	for _, v := range versions[1:] {
		if compareVersion(v, latest) > 0 {
			latest = v
		}
	}
	return r.execs[definition][latest], nil
}

// compareVersion 比较两个版本字符串：先比 v 前缀（按字典序），
// 再比数字部分（按数值）。返回 -1/0/1。
func compareVersion(a, b string) int {
	aPrefix, aNum, aOK := splitVersion(a)
	bPrefix, bNum, bOK := splitVersion(b)
	if aOK && bOK && aPrefix == bPrefix {
		switch {
		case aNum < bNum:
			return -1
		case aNum > bNum:
			return 1
		default:
			return strings.Compare(a, b)
		}
	}
	return strings.Compare(a, b)
}

// splitVersion 把 "v10" 拆为 ("v", 10, true)；无数字尾缀时 ok 为 false。
func splitVersion(v string) (prefix string, num int, ok bool) {
	i := strings.LastIndexFunc(v, func(r rune) bool { return r < '0' || r > '9' })
	prefix, digits := v[:i+1], v[i+1:]
	if digits == "" {
		return prefix, 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return prefix, 0, false
	}
	return prefix, n, true
}
