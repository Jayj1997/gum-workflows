// workflow 是 gum-workflows 的 CLI 入口。
//
// 命令（设计计划 §16，不接受业务参数）：
//
//	workflow validate <workflow-file>
//	workflow run <workflow-file>
package main

import (
	"fmt"
	"os"

	"github.com/Jayj1997/gum-workflows/internal/validation"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: workflow <validate|run> <workflow-file>")
	}
	switch args[0] {
	case "validate":
		return validateCmd(args[1])
	case "run":
		return runCmd(args[1])
	default:
		return fmt.Errorf("unknown command %q (available: validate, run)", args[0])
	}
}

// validateCmd 执行校验管线（设计计划 §21 两层校验），与 run 共用 loadAndValidate。
// warning 不阻断校验（环降为提示，票 06），以 warning 前缀打印到 stderr。
func validateCmd(path string) error {
	def, _, _, _, warnings, err := loadAndValidate(path)
	if err != nil {
		return err
	}

	printWarnings(warnings)
	fmt.Printf("%s: valid (workflow/v1)\n", def.Metadata.Name)
	return nil
}

// printWarnings 输出语义校验的提示级问题（validate 与 run 共用展示形态）。
func printWarnings(warnings []validation.Warning) {
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w.Message)
	}
}
