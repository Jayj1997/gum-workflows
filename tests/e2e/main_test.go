package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// binPath 是编译好的 CLI 二进制（TestMain 一次性构建）。
var binPath string
var interactiveBinPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "gum-workflows-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binPath = filepath.Join(tmp, "workflow")
	interactiveBinPath = filepath.Join(tmp, "workflow-interactive")

	cmd := exec.Command("go", "build", "-o", binPath, "github.com/Jayj1997/gum-workflows/cmd/workflow")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build CLI: %v\n%s", err, out)
		os.Exit(1)
	}
	interactiveCmd := exec.Command("go", "build", "-tags=gumworkflowe2e", "-o", interactiveBinPath, "github.com/Jayj1997/gum-workflows/cmd/workflow")
	if out, err := interactiveCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build interactive CLI: %v\n%s", err, out)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func execInteractiveCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command(interactiveBinPath, args...)
	cmd.Dir = dir
	return cmd
}

// execCommand 构造在 dir 下运行 CLI 的命令。
func execCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	return cmd
}
