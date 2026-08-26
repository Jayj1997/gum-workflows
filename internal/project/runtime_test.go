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

	r := NewRuntime("")
	ctx, err := r.Resolve(filepath.Join(wfdir, "workflow.yaml"), Spec{Name: "demo", Repository: "./project"})
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if ctx.Repository.Path != projdir {
		t.Errorf("Repository.Path = %q, want %q", ctx.Repository.Path, projdir)
	}
}

func TestResolveAbsoluteRepository(t *testing.T) {
	repo := writeProject(t)
	r := NewRuntime("")
	ctx, err := r.Resolve("", Spec{Repository: repo})
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if ctx.Repository.Path != repo {
		t.Errorf("Repository.Path = %q, want %q", ctx.Repository.Path, repo)
	}
}

func TestResolveRejectsEmptyRepository(t *testing.T) {
	r := NewRuntime("")
	if _, err := r.Resolve("wf.yaml", Spec{}); err == nil {
		t.Fatal("Resolve(empty) = nil error, want rejection")
	}
}

func TestCreateWorkspace(t *testing.T) {
	repo := writeProject(t)
	execRoot := filepath.Join(t.TempDir(), "executions")

	r := NewRuntime(execRoot)
	ctx, err := r.Resolve("", Spec{Repository: repo})
	if err != nil {
		t.Fatal(err)
	}

	wsCtx, err := r.CreateWorkspace(ctx, "execution-000001")
	if err != nil {
		t.Fatalf("CreateWorkspace() unexpected error: %v", err)
	}

	want := filepath.Join(execRoot, "execution-000001", "workspace", "project")
	if wsCtx.Workspace != want {
		t.Fatalf("Workspace = %q, want %q", wsCtx.Workspace, want)
	}

	// 项目文件被复制。
	for _, rel := range []string{"README.md", "package.json", "src/app.ts", ".claude/skills/x.md", ".agents/skills/y.md"} {
		if _, err := os.Stat(filepath.Join(want, rel)); err != nil {
			t.Errorf("missing %s in workspace: %v", rel, err)
		}
	}
	// .git 与 .workflow 不复制。
	for _, rel := range []string{".git/config", ".workflow/state.json"} {
		if _, err := os.Stat(filepath.Join(want, rel)); !os.IsNotExist(err) {
			t.Errorf("%s should not be copied into workspace", rel)
		}
	}
	// 源仓库不受影响（在原位）。
	if _, err := os.Stat(filepath.Join(repo, ".git", "config")); err != nil {
		t.Errorf("source .git damaged: %v", err)
	}
}

func TestCreateWorkspaceIndependentPerExecution(t *testing.T) {
	repo := writeProject(t)
	execRoot := filepath.Join(t.TempDir(), "executions")
	r := NewRuntime(execRoot)

	ctx, _ := r.Resolve("", Spec{Repository: repo})
	ws1, err := r.CreateWorkspace(ctx, "execution-000001")
	if err != nil {
		t.Fatal(err)
	}
	// 在第一个 Workspace 里写入变更。
	if err := os.WriteFile(filepath.Join(ws1.Workspace, "NEW.md"), []byte("change"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws2, err := r.CreateWorkspace(ctx, "execution-000002")
	if err != nil {
		t.Fatal(err)
	}
	// 第二个 Workspace 不含第一个的变更；源仓库也不含。
	if _, err := os.Stat(filepath.Join(ws2.Workspace, "NEW.md")); !os.IsNotExist(err) {
		t.Error("workspace #2 contains changes from execution #001")
	}
	if _, err := os.Stat(filepath.Join(repo, "NEW.md")); !os.IsNotExist(err) {
		t.Error("source repository polluted by execution #001")
	}
}

func TestCreateWorkspaceRejectsMissingRepo(t *testing.T) {
	r := NewRuntime(filepath.Join(t.TempDir(), "executions"))

	if _, err := r.CreateWorkspace(Context{}, "e1"); err == nil {
		t.Error("CreateWorkspace(empty ctx) = nil error, want rejection")
	}
	if _, err := r.CreateWorkspace(Context{Repository: Repository{Path: "/nonexistent/repo"}}, "e1"); err == nil {
		t.Error("CreateWorkspace(missing repo) = nil error, want rejection")
	}

	// repository 指向文件而非目录。
	notADir := filepath.Join(t.TempDir(), "file-repo")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateWorkspace(Context{Repository: Repository{Path: notADir}}, "e1"); err == nil {
		t.Error("CreateWorkspace(file repo) = nil error, want rejection")
	}
}
