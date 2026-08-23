package node

import (
	"fmt"
	"sort"
)

// Registry 维护 Node Type 到 Factory 的映射（设计计划 §12）。
// Runtime 加载 YAML 后按 type 从这里找到 Factory 完成实例化，
// Workflow 不需要知道 Node 的 Go 实现。
type Registry struct {
	factories map[string]Factory
}

// NewRegistry 创建空 Registry。注册通过显式的 Register 完成
// （禁止 init() 隐式注册，见 docs/DEVELOPMENT.md §3）。
func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

// Register 注册一个 Factory，Node Type 重复注册视为编程错误。
func (r *Registry) Register(f Factory) error {
	t := f.Type()
	if t == "" {
		return fmt.Errorf("register node: factory Type must not be empty")
	}
	if _, ok := r.factories[t]; ok {
		return fmt.Errorf("register node: type %q already registered", t)
	}
	r.factories[t] = f
	return nil
}

// Get 返回指定 Node Type 的 Factory。
func (r *Registry) Get(nodeType string) (Factory, bool) {
	f, ok := r.factories[nodeType]
	return f, ok
}

// Types 返回已注册的 Node Type 列表（有序）。
func (r *Registry) Types() []string {
	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}
