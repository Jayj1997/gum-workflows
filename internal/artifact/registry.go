package artifact

import (
	"fmt"
	"sort"
)

// Registry 维护已登记的 Artifact Kind（设计计划开发顺序 ⑦），
// 是语义校验中 Input/Output 类型匹配的依据之一。
type Registry struct {
	kinds map[Kind]bool
}

// NewRegistry 创建已登记全部 MVP 内置 Kind 的 Registry（设计计划 §14 的 7 种），
// 并登记 MVP 阶段的全部内置 Kind。
func NewRegistry() *Registry {
	r := &Registry{kinds: map[Kind]bool{}}
	for _, k := range []Kind{
		KindRequirementSpec,
		KindArchitectureSpec,
		KindOpenAPI,
		KindFrontendSDK,
		KindSourceCode,
		KindTestReport,
		KindApprovalResult,
		KindQualityCheckResult,
		KindLog,
	} {
		r.kinds[k] = true
	}
	return r
}

// Register 登记一个新的 Artifact Kind（供后续版本扩展 Node 时使用）。
func (r *Registry) Register(k Kind) error {
	if k == "" {
		return fmt.Errorf("register artifact kind: must not be empty")
	}
	if r.kinds[k] {
		return fmt.Errorf("register artifact kind: %q already registered", k)
	}
	r.kinds[k] = true
	return nil
}

// Has 报告 Kind 是否已登记。
func (r *Registry) Has(k Kind) bool {
	return r.kinds[k]
}

// Kinds 返回已登记的 Kind 列表（有序）。
func (r *Registry) Kinds() []string {
	kinds := make([]string, 0, len(r.kinds))
	for k := range r.kinds {
		kinds = append(kinds, string(k))
	}
	sort.Strings(kinds)
	return kinds
}
