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
func validateCmd(path string) error {
	def, _, _, err := loadAndValidate(path)
	if err != nil {
		return err
	}

	fmt.Printf("%s: valid (workflow/v1)\n", def.Metadata.Name)
	return nil
}
