package builtins

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// CheckConsistency 做启动一致性检查（设计文档 §6.9）：
// Go 注册的每个 (definition, version) 必须有种子 Executor YAML 声明，
// 反之亦然；任一不一致即报错（错误列出缺失/多余项），
// 防止声明与实现漂移到运行期才暴露。
//
// 种子契约的 type 合法与 Kind 已注册属定义层校验（defs.Load 装载时
// 已做语法与结构检查；Kind 注册检查在语义校验接线，见票 06）。
func CheckConsistency(executors *node.ExecutorRegistry, defs *definition.Registry) error {
	// YAML 声明集：definition -> versions。
	declared := map[string]map[string]bool{}
	for _, def := range defs.DefinitionNames() {
		versions := map[string]bool{}
		for _, v := range defs.ExecutorVersions(def) {
			versions[v] = true
		}
		declared[def] = versions
	}

	// Go 注册集：definition -> versions。
	var missingYAML []string
	for _, def := range executors.Definitions() {
		versions, ok := declared[def]
		if !ok {
			for _, v := range executors.Versions(def) {
				missingYAML = append(missingYAML, fmt.Sprintf("(%s, %s)", def, v))
			}
			continue
		}
		for _, v := range executors.Versions(def) {
			factory, err := executors.Get(def, v)
			if err == nil {
				if validator, ok := factory.(node.ExecutorValidator); ok {
					if validateErr := validator.ValidateExecutor(); validateErr != nil {
						return fmt.Errorf("executor (%s, %s) packaged assets: %w", def, v, validateErr)
					}
				}
			}
			if !versions[v] {
				missingYAML = append(missingYAML, fmt.Sprintf("(%s, %s)", def, v))
			}
		}
	}

	// 反向：声明了没有 Go 实现的 executor。
	var missingGo []string
	for _, def := range sortedKeys(declared) {
		for _, v := range defs.ExecutorVersions(def) {
			if _, err := executors.Get(def, v); err != nil {
				missingGo = append(missingGo, fmt.Sprintf("(%s, %s)", def, v))
			}
		}
	}

	if len(missingYAML) > 0 || len(missingGo) > 0 {
		var parts []string
		if len(missingYAML) > 0 {
			parts = append(parts, fmt.Sprintf("Go executors without YAML declaration: %s",
				strings.Join(missingYAML, ", ")))
		}
		if len(missingGo) > 0 {
			parts = append(parts, fmt.Sprintf("executor YAML declarations without Go executor: %s",
				strings.Join(missingGo, ", ")))
		}
		return fmt.Errorf("executor/definition consistency check failed:\n%s",
			strings.Join(parts, "\n"))
	}
	return nil
}

// sortedKeys 返回 map key 的有序列表（保证错误信息顺序稳定）。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
