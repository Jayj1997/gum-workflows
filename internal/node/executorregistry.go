package node

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	defpkg "github.com/Jayj1997/gum-workflows/internal/definition"
)

// ErrExecutorNotFound 是 Executor 查询未命中的哨兵错误
// （DEVELOPMENT.md §4.2：可编程判断用 errors.Is）。
var ErrExecutorNotFound = errors.New("executor not found")

// ExecutorRegistry 按 (definition, version) 注册 ExecutorFactory
// （设计文档 §6.9）：同一定义多版本并存，Latest 供缺省解析。
// 契约（inputs/outputs）不在本注册表--唯一来源是 Node Definition YAML。
type ExecutorRegistry struct {
	factories map[string]map[string]ExecutorFactory // definition -> version -> factory
}

// NewExecutorRegistry 创建空注册表。注册通过显式的 Register 完成
// （禁止 init() 隐式注册，见 docs/DEVELOPMENT.md §3）。
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{factories: map[string]map[string]ExecutorFactory{}}
}

// Register 注册一个 ExecutorFactory，(definition, version) 重复视为编程错误。
func (r *ExecutorRegistry) Register(f ExecutorFactory) error {
	def, ver := f.Definition(), f.Version()
	if def == "" {
		return fmt.Errorf("register executor: factory Definition must not be empty")
	}
	if ver == "" {
		return fmt.Errorf("register executor %q: factory Version must not be empty", def)
	}
	versions, ok := r.factories[def]
	if !ok {
		versions = map[string]ExecutorFactory{}
		r.factories[def] = versions
	}
	if _, ok := versions[ver]; ok {
		return fmt.Errorf("register executor: (%s, %s) already registered", def, ver)
	}
	versions[ver] = f
	return nil
}

// Get 返回指定 (definition, version) 的 ExecutorFactory。
func (r *ExecutorRegistry) Get(definition, version string) (ExecutorFactory, error) {
	versions, ok := r.factories[definition]
	if !ok {
		return nil, fmt.Errorf("%w: no executor for node definition %q (registered: %s)",
			ErrExecutorNotFound, definition, joinQuoted(r.Definitions()))
	}
	f, ok := versions[version]
	if !ok {
		return nil, fmt.Errorf("%w: executor %q of node definition %q (versions: %s)",
			ErrExecutorNotFound, version, definition, joinQuoted(r.Versions(definition)))
	}
	return f, nil
}

// Latest 返回某 Node Definition 的最新 Executor 版本（缺省解析用）。
// 版本形如 v1、v2……（设计文档 §3.3）：比较按 v 后的数字大小，
// 避免字典序把 v10 排在 v2 之前。
func (r *ExecutorRegistry) Latest(definition string) (ExecutorFactory, error) {
	versions := r.Versions(definition)
	if len(versions) == 0 {
		return nil, fmt.Errorf("%w: no executor for node definition %q (registered: %s)",
			ErrExecutorNotFound, definition, joinQuoted(r.Definitions()))
	}
	latest := versions[0]
	for _, v := range versions[1:] {
		if defpkg.CompareVersions(v, latest) > 0 {
			latest = v
		}
	}
	return r.factories[definition][latest], nil
}

// joinQuoted 返回 ["a", "b"] 形态，供错误信息列出合法值集合。
func joinQuoted(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = `"` + v + `"`
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// Definitions 返回已注册的 Node Definition 名列表（有序）。
func (r *ExecutorRegistry) Definitions() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Versions 返回某 Node Definition 的全部 Executor 版本（有序）。
// definition 不存在时返回 nil。
func (r *ExecutorRegistry) Versions(definition string) []string {
	versions, ok := r.factories[definition]
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
