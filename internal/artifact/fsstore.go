package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
)

// FilesystemStore 是基于文件系统的 Artifact Store 实现（设计计划 §28-§29）。
// 目录布局：root/<n>.json（n 自增）；URI 为相对文件名。
// Engine / Node 通过 Store 接口使用，未来可替换为 S3/OSS 而不修改 Node。
type FilesystemStore struct {
	mu   sync.Mutex
	root string
	next int
}

// NewFilesystemStore 在 root 下创建存储目录（幂等）。
func NewFilesystemStore(root string) (*FilesystemStore, error) {
	if root == "" {
		return nil, fmt.Errorf("filesystem store: root must not be empty")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("filesystem store: create %s: %w", root, err)
	}
	// 扫描已有文件确定自增起点，避免覆盖。
	s, err := os.Open(root)
	if err != nil {
		return nil, fmt.Errorf("filesystem store: open %s: %w", root, err)
	}
	defer s.Close()
	names, err := s.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("filesystem store: read %s: %w", root, err)
	}
	next := 0
	for _, name := range names {
		var n int
		if _, err := fmt.Sscanf(name, "%d.json", &n); err == nil && n > next {
			next = n
		}
	}
	return &FilesystemStore{root: root, next: next}, nil
}

// filePattern 匹配自增文件名（1.json、2.json ...）。
var filePattern = regexp.MustCompile(`^[0-9]+\.json$`)

// Put 校验并保存 Artifact，返回指向相对路径 URI 的引用。
func (s *FilesystemStore) Put(a Artifact) (ArtifactRef, error) {
	if err := a.Validate(); err != nil {
		return ArtifactRef{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++
	name := fmt.Sprintf("%d.json", s.next)
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return ArtifactRef{}, fmt.Errorf("filesystem store: marshal %q: %w", a.ID, err)
	}
	if err := os.WriteFile(filepath.Join(s.root, name), data, 0o644); err != nil {
		return ArtifactRef{}, fmt.Errorf("filesystem store: write %q: %w", name, err)
	}
	return a.Ref(name), nil
}

// Get 按 URI（相对文件名）取回 Artifact。
func (s *FilesystemStore) Get(ref ArtifactRef) (Artifact, error) {
	name := filepath.Base(ref.URI)
	if !filePattern.MatchString(name) {
		return Artifact{}, fmt.Errorf("filesystem store: invalid URI %q", ref.URI)
	}
	data, err := os.ReadFile(filepath.Join(s.root, name))
	if err != nil {
		return Artifact{}, fmt.Errorf("filesystem store: get %q: %w", ref.URI, err)
	}
	var a Artifact
	if err := json.Unmarshal(data, &a); err != nil {
		return Artifact{}, fmt.Errorf("filesystem store: parse %q: %w", ref.URI, err)
	}
	return a, nil
}

// Exists 报告引用是否已存在。
func (s *FilesystemStore) Exists(ref ArtifactRef) bool {
	name := filepath.Base(ref.URI)
	if !filePattern.MatchString(name) {
		return false
	}
	_, err := os.Stat(filepath.Join(s.root, name))
	return err == nil
}

// UpdateVersion assigns a runtime output version without creating a duplicate artifact.
func (s *FilesystemStore) UpdateVersion(ref ArtifactRef, version string) (ArtifactRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := filepath.Base(ref.URI)
	if !filePattern.MatchString(name) {
		return ArtifactRef{}, fmt.Errorf("filesystem store: invalid URI %q", ref.URI)
	}
	path := filepath.Join(s.root, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ArtifactRef{}, fmt.Errorf("filesystem store: get %q: %w", ref.URI, err)
	}
	var a Artifact
	if err := json.Unmarshal(data, &a); err != nil {
		return ArtifactRef{}, fmt.Errorf("filesystem store: parse %q: %w", ref.URI, err)
	}
	a.Version = version
	data, err = json.MarshalIndent(a, "", "  ")
	if err != nil {
		return ArtifactRef{}, fmt.Errorf("filesystem store: marshal %q: %w", a.ID, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ArtifactRef{}, fmt.Errorf("filesystem store: write %q: %w", name, err)
	}
	return a.Ref(name), nil
}

// List 返回全部已存储 Artifact 的 URI（有序）。
func (s *FilesystemStore) List() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s2, err := os.Open(s.root)
	if err != nil {
		return nil, fmt.Errorf("filesystem store: open %s: %w", s.root, err)
	}
	defer s2.Close()
	names, err := s2.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("filesystem store: read %s: %w", s.root, err)
	}
	var uris []string
	for _, name := range names {
		if filePattern.MatchString(name) {
			uris = append(uris, name)
		}
	}
	sort.Strings(uris)
	return uris, nil
}
