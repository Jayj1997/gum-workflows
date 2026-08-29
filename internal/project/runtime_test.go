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

	ctx, err := Resolve(filepath.Join(wfdir, "workflow.yaml"), Spec{Name: "demo", Repository: "./project"})
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
	ctx, err := Resolve("", Spec{Repository: repo})
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
	if _, err := Resolve("wf.yaml", Spec{}); err == nil {
		t.Fatal("Resolve(empty) = nil error, want rejection")
	}
}

func TestResolveNormalizesInPlaceWorkspace(t *testing.T) {
	repo := writeProject(t)
	ctx, err := Resolve("", Spec{Repository: filepath.Join(repo, "src", "..")})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Repository.Path != repo || ctx.Workspace != repo {
		t.Fatalf("resolved context = %#v, want repository and workspace %q", ctx, repo)
	}
}

func TestResolveRejectsInvalidRepository(t *testing.T) {
	tests := []struct {
		name       string
		repository func(*testing.T) string
	}{
		{
			name: "missing path",
			repository: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
		},
		{
			name: "file instead of directory",
			repository: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file-repo")
				if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Resolve("", Spec{Repository: tt.repository(t)}); err == nil {
				t.Error("Resolve() = nil error, want rejection")
			}
		})
	}
}
