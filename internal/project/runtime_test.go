package project

import (
	"os"
	"path/filepath"
	"testing"
)

// 写一个临时项目目录供测试。
func writeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"README.md":             "# demo project\n",
		"package.json":          `{"name": "demo"}`,
		"src/app.ts":            "export {}\n",
		".claude/skills/x.md":   "skill\n",
		".agents/skills/y.md":   "skill\n",
		".git/config":           "[core]\n",
		".workflow/state.json":  "{}",
		"node_modules/pkg.json": "{}",
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestResolveRelativeRepository(t *testing.T) {
	// workflow 文件位于 wfdir/workflow.yaml，repository "./project" 应解析为
	// wfdir/project（相对于 workflow 文件，而非 CWD）。
	base := t.TempDir()
	wfdir := filepath.Join(base, "wfdir")
	projdir := filepath.Join(wfdir, "project")
	for _, dir := range []string{wfdir, projdir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	r := NewRuntime()
	ctx, err := r.Resolve(filepath.Join(wfdir, "workflow.yaml"), Spec{Name: "demo", Repository: "./project"})
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if ctx.Repository.Path != projdir {
		t.Errorf("Repository.Path = %q, want %q", ctx.Repository.Path, projdir)
	}
	if ctx.Workspace != projdir {
		t.Errorf("Workspace = %q, want in-place project %q", ctx.Workspace, projdir)
	}
}

func TestResolveAbsoluteRepository(t *testing.T) {
	repo := writeProject(t)
	r := NewRuntime()
	ctx, err := r.Resolve("", Spec{Repository: repo})
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if ctx.Repository.Path != repo {
		t.Errorf("Repository.Path = %q, want %q", ctx.Repository.Path, repo)
	}
	if ctx.Workspace != repo {
		t.Errorf("Workspace = %q, want in-place project %q", ctx.Workspace, repo)
	}
}

func TestResolveRejectsEmptyRepository(t *testing.T) {
	r := NewRuntime()
	if _, err := r.Resolve("wf.yaml", Spec{}); err == nil {
		t.Fatal("Resolve(empty) = nil error, want rejection")
	}
}

func TestResolveNormalizesInPlaceWorkspace(t *testing.T) {
	repo := writeProject(t)
	r := NewRuntime()

	ctx, err := r.Resolve("", Spec{Repository: filepath.Join(repo, "src", "..")})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Repository.Path != repo || ctx.Workspace != repo {
		t.Fatalf("resolved context = %#v, want repository and workspace %q", ctx, repo)
	}
}

func TestResolveRejectsMissingRepository(t *testing.T) {
	r := NewRuntime()

	if _, err := r.Resolve("", Spec{Repository: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("Resolve(missing repo) = nil error, want rejection")
	}

	// repository 指向文件而非目录。
	notADir := filepath.Join(t.TempDir(), "file-repo")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve("", Spec{Repository: notADir}); err == nil {
		t.Error("Resolve(file repo) = nil error, want rejection")
	}
}
