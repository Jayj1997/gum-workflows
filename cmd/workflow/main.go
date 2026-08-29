// workflow 是 gum-workflows 的 CLI 入口。
//
// 命令（设计计划 §16，不接受业务参数）：
//
//	workflow validate <workflow-file>
//	workflow run <workflow-file>
//	workflow history [<run-id> [<node-id>]]
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/validation"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	return runWithRuntimePaths(args, func() (runtimepath.Paths, error) {
		return runtimepath.Resolve("")
	})
}

func runWithRuntimePaths(args []string, resolve func() (runtimepath.Paths, error)) error {
	if len(args) > 0 && args[0] == "history" {
		if len(args) > 3 {
			return fmt.Errorf("usage: workflow history [<run-id> [<node-id>]]")
		}
		paths, err := resolveRuntimePaths(resolve)
		if err != nil {
			return err
		}
		return historyCmd(context.Background(), args[1:], paths, os.Stdout)
	}
	if len(args) != 2 {
		return fmt.Errorf("usage: workflow <validate|run> <workflow-file> | workflow history [<run-id> [<node-id>]]")
	}
	switch args[0] {
	case "validate":
		return validateCmd(args[1], resolve)
	case "run":
		paths, err := resolveRuntimePaths(resolve)
		if err != nil {
			return err
		}
		return runCmd(args[1], paths)
	default:
		return fmt.Errorf("unknown command %q (available: validate, run, history)", args[0])
	}
}

func resolveRuntimePaths(resolve func() (runtimepath.Paths, error)) (runtimepath.Paths, error) {
	if resolve == nil {
		return runtimepath.Paths{}, fmt.Errorf("resolve runtime paths: resolver must not be nil")
	}
	paths, err := resolve()
	if err != nil {
		return runtimepath.Paths{}, fmt.Errorf("resolve runtime paths: %w", err)
	}
	return paths, nil
}

// validateCmd 执行校验管线（设计计划 §21 两层校验），与 run 共用 loadAndValidate。
// warning 不阻断校验（环降为提示，票 06），以 warning 前缀打印到 stderr。
func validateCmd(path string, resolve func() (runtimepath.Paths, error)) error {
	ctx := context.Background()
	def, _, executors, _, _, warnings, err := loadAndValidate(ctx, path)
	if err != nil {
		return err
	}
	paths, err := resolveRuntimePaths(resolve)
	if err != nil {
		return fmt.Errorf("resolve runtime paths: %w", err)
	}
	if err := validateExistingDatabaseExecutors(ctx, paths.Database(), def, executors); err != nil {
		return fmt.Errorf("validate database executor resolution: %w", err)
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
