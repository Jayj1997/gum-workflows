// Package artifact 定义 Node 之间唯一的数据通信方式。
//
// 运行时在 Node 之间传递的是 ArtifactRef（引用），
// 数据本体（Artifact）只在 Node 内部实际消费时才通过 Store 加载。
package artifact

import "fmt"

// Kind 标识 Artifact 的类型，是 Input/Output Contract 类型匹配的依据。
type Kind string

// MVP 阶段支持的 Artifact 类型（设计计划 §14）。
const (
	KindRequirementSpec    Kind = "RequirementSpec"
	KindArchitectureSpec   Kind = "ArchitectureSpec"
	KindOpenAPI            Kind = "OpenAPI"
	KindFrontendSDK        Kind = "FrontendSDK"
	KindSourceCode         Kind = "SourceCode"
	KindTestReport         Kind = "TestReport"
	KindApprovalResult     Kind = "ApprovalResult"
	KindQualityCheckResult Kind = "QualityCheckResult"
	KindLog                Kind = "Log"
)

// Artifact 是数据本体。大型数据（如源码）不应放进 Data，
// 而是保存引用信息（repo path / commit / workspace）。
type Artifact struct {
	ID      string
	Kind    Kind
	Version string
	Data    any
}

// ArtifactRef 是运行时传递的引用，不携带数据本体。
type ArtifactRef struct {
	ID      string
	Kind    Kind
	Version string
	URI     string
}

// Ref 为 Artifact 生成指向 uri 的引用。
func (a Artifact) Ref(uri string) ArtifactRef {
	return ArtifactRef{
		ID:      a.ID,
		Kind:    a.Kind,
		Version: a.Version,
		URI:     uri,
	}
}

// Validate 检查引用的基本不变式：ID、Kind、URI 必须非空。
func (r ArtifactRef) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("artifact ref: ID must not be empty")
	}
	if r.Kind == "" {
		return fmt.Errorf("artifact ref %q: Kind must not be empty", r.ID)
	}
	if r.URI == "" {
		return fmt.Errorf("artifact ref %q: URI must not be empty", r.ID)
	}
	return nil
}

// Validate 检查数据本体的基本不变式：ID、Kind 必须非空。
func (a Artifact) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("artifact: ID must not be empty")
	}
	if a.Kind == "" {
		return fmt.Errorf("artifact %q: Kind must not be empty", a.ID)
	}
	return nil
}

// Store 是 Artifact 的存取抽象（设计计划 §29）。
// MVP 提供 FilesystemArtifactStore 实现；未来可替换为 S3/OSS/Database 而不修改 Node。
type Store interface {
	Put(artifact Artifact) (ArtifactRef, error)
	Get(ref ArtifactRef) (Artifact, error)
	Exists(ref ArtifactRef) bool
}
