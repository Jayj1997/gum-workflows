package artifact

import (
	"fmt"
	"sync"
)

// MemStore 是纯内存的 Artifact Store 实现。
// 用于最小 Runtime（串行、内存版）；FilesystemArtifactStore 属于后续里程碑
// （设计计划开发顺序 ⑫），届时替换而不修改 Node。
//
// 同一 ID 的 Artifact 允许多次 Put（URI 唯一即可），引用按 URI 区分。
type MemStore struct {
	mu    sync.Mutex
	next  int
	byURI map[string]Artifact
}

// NewMemStore 创建空内存 Store。
func NewMemStore() *MemStore {
	return &MemStore{byURI: map[string]Artifact{}}
}

// Put 校验并保存 Artifact，返回指向唯一 URI 的引用。
func (s *MemStore) Put(a Artifact) (ArtifactRef, error) {
	if err := a.Validate(); err != nil {
		return ArtifactRef{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++
	uri := fmt.Sprintf("mem://%d", s.next)
	s.byURI[uri] = a
	return a.Ref(uri), nil
}

// Get 按 URI 取回 Artifact。
func (s *MemStore) Get(ref ArtifactRef) (Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	a, ok := s.byURI[ref.URI]
	if !ok {
		return Artifact{}, fmt.Errorf("artifact %q: not found at %q", ref.ID, ref.URI)
	}
	return a, nil
}

// Exists 报告引用是否已存在。
func (s *MemStore) Exists(ref ArtifactRef) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.byURI[ref.URI]
	return ok
}
