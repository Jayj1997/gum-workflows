package main

import (
	"fmt"
	"os"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins/defs"
	"github.com/Jayj1997/gum-workflows/internal/validation"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// loadAndValidate 执行 validate 与 run 共用的校验管线（设计计划 §21）：
//
//	read -> CUE Schema -> Go Parser -> 语义校验（内置 Executor + 种子定义）
//
// 返回通过两层校验的 Definition、内容字节、已通过启动一致性检查的
// Executor Registry 与定义 Registry。契约（inputs/outputs）唯一来源是
// 种子 Node Definition YAML；Go 实现集与 YAML 声明集双向 diff，
// 任一不一致即报错（设计文档 §6.9）。
func loadAndValidate(path string) (workflow.Definition, []byte, *node.ExecutorRegistry, *definition.Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return workflow.Definition{}, nil, nil, nil, fmt.Errorf("read workflow file: %w", err)
	}

	if err := validation.ValidateSchema(path, data); err != nil {
		return workflow.Definition{}, nil, nil, nil, err
	}
	def, err := workflow.Load(data)
	if err != nil {
		return workflow.Definition{}, nil, nil, nil, err
	}

	defsRegistry, err := defs.NewRegistry()
	if err != nil {
		return workflow.Definition{}, nil, nil, nil, fmt.Errorf("load seed definitions: %w", err)
	}
	executors := node.NewExecutorRegistry()
	if err := builtins.RegisterAll(executors); err != nil {
		return workflow.Definition{}, nil, nil, nil, fmt.Errorf("register builtin executors: %w", err)
	}
	if err := builtins.CheckConsistency(executors, defsRegistry); err != nil {
		return workflow.Definition{}, nil, nil, nil, fmt.Errorf("startup consistency check failed:\n%w", err)
	}
	if err := validation.NewSemanticValidator(executors, defsRegistry, artifact.NewRegistry()).Validate(def); err != nil {
		return workflow.Definition{}, nil, nil, nil, fmt.Errorf("semantic validation failed:\n%w", err)
	}
	return def, data, executors, defsRegistry, nil
}
