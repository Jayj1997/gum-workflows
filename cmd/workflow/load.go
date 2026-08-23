package main

import (
	"fmt"
	"os"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/builtins"
	"github.com/Jayj1997/gum-workflows/internal/validation"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// loadAndValidate 执行 validate 与 run 共用的校验管线（设计计划 §21）：
//
//	read -> CUE Schema -> Go Parser -> 语义校验（内置 Registry）
//
// 返回通过两层校验的 Definition、内容字节与已注册内置 Node 的 Registry。
func loadAndValidate(path string) (workflow.Definition, []byte, *node.Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return workflow.Definition{}, nil, nil, fmt.Errorf("read workflow file: %w", err)
	}

	if err := validation.ValidateSchema(path, data); err != nil {
		return workflow.Definition{}, nil, nil, err
	}
	def, err := workflow.Load(data)
	if err != nil {
		return workflow.Definition{}, nil, nil, err
	}

	registry := node.NewRegistry()
	if err := builtins.RegisterAll(registry); err != nil {
		return workflow.Definition{}, nil, nil, fmt.Errorf("register builtin nodes: %w", err)
	}
	if err := validation.NewSemanticValidator(registry, artifact.NewRegistry()).Validate(def); err != nil {
		return workflow.Definition{}, nil, nil, fmt.Errorf("semantic validation failed:\n%w", err)
	}
	return def, data, registry, nil
}
