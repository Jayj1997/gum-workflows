// Package validation 实现 Workflow 的两层校验（设计计划 §24）：
//
//	workflow.yaml -> CUE Validation（结构）-> Go Semantic Validator（语义）-> Runtime
package validation

import (
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/encoding/yaml"

	workflowschema "github.com/Jayj1997/gum-workflows/schema/workflow"
)

// ValidateSchema 用 workflow/v1 CUE Schema 校验 YAML 原始内容（第一层：结构校验）。
// filename 仅用于错误信息定位。YAML 语法错误同样在此报出；
// 错误信息包含「字段路径 + 文件行号」（设计计划 M3 验收要求）。
func ValidateSchema(filename string, data []byte) error {
	ctx := cuecontext.New()

	schema := ctx.CompileBytes(workflowschema.V1)
	if schema.Err() != nil {
		// Schema 自身编译失败属于程序缺陷，不属于用户输入错误。
		return fmt.Errorf("internal error: compile workflow/v1 schema: %w", schema.Err())
	}

	// Extract 解析 YAML 并保留源码位置信息（含 filename）。
	f, err := yaml.Extract(filename, data)
	if err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	v := ctx.BuildFile(f)

	if err := schema.Unify(v).Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("schema validation failed:\n%s", errors.Details(err, nil))
	}
	return nil
}
