package artifact

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemStoreRoundtrip(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	s, err := NewFilesystemStore(root)
	if err != nil {
		t.Fatalf("NewFilesystemStore() unexpected error: %v", err)
	}

	a := Artifact{ID: "openapi", Kind: KindOpenAPI, Version: "1", Data: "spec: v1"}
	ref, err := s.Put(a)
	if err != nil {
		t.Fatalf("Put() unexpected error: %v", err)
	}
	if !s.Exists(ref) {
		t.Error("Exists(ref) = false after Put")
	}

	got, err := s.Get(ref)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.ID != a.ID || got.Kind != a.Kind || got.Data != a.Data {
		t.Fatalf("Get() = %+v, want %+v", got, a)
	}
}

func TestFilesystemStoreConcurrentPutUniqueURIs(t *testing.T) {
	s, err := NewFilesystemStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	refs := make(chan ArtifactRef, n)
	for i := 0; i < n; i++ {
		go func() {
			ref, err := s.Put(Artifact{ID: "source-code", Kind: KindSourceCode})
			if err != nil {
				t.Errorf("Put(): %v", err)
			}
			refs <- ref
		}()
	}
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		ref := <-refs
		if seen[ref.URI] {
			t.Fatalf("duplicate URI %q", ref.URI)
		}
		seen[ref.URI] = true
	}
	uris, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(uris) != n {
		t.Fatalf("List() = %d items, want %d", len(uris), n)
	}
}

func TestFilesystemStorePersistsAcrossInstances(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")

	s1, err := NewFilesystemStore(root)
	if err != nil {
		t.Fatal(err)
	}
	a := Artifact{ID: "requirement", Kind: KindRequirementSpec, Data: "用户故事"}
	ref, err := s1.Put(a)
	if err != nil {
		t.Fatal(err)
	}

	// 新实例指向同一 root：不覆盖已有文件，且能读回。
	s2, err := NewFilesystemStore(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get(ref)
	if err != nil {
		t.Fatalf("Get() via new instance: %v", err)
	}
	if got.Data != a.Data {
		t.Fatalf("Data = %v, want %v", got.Data, a.Data)
	}
	ref2, err := s2.Put(Artifact{ID: "requirement", Kind: KindRequirementSpec})
	if err != nil {
		t.Fatal(err)
	}
	if ref2.URI == ref.URI {
		t.Fatalf("new Put reused URI %q", ref.URI)
	}
}

func TestFilesystemStoreInvalidInput(t *testing.T) {
	s, err := NewFilesystemStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Put(Artifact{}); err == nil {
		t.Error("Put(empty) = nil error, want validation failure")
	}

	// 非法 URI（含路径分隔符）不得越出 root。
	bad := ArtifactRef{ID: "x", Kind: KindOpenAPI, URI: "../../etc/passwd"}
	if s.Exists(bad) {
		t.Error("Exists(traversal URI) = true, want false")
	}
	if _, err := s.Get(bad); err == nil {
		t.Error("Get(traversal URI) = nil error, want rejection")
	} else if !strings.Contains(err.Error(), "invalid URI") {
		t.Errorf("error %q should mention invalid URI", err)
	}
}
