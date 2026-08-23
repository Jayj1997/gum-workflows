// workflow/v1 的 CUE Schema：结构层校验（设计计划 §21-§22）。
// 只声明字段存在性与类型约束；语义校验（引用、类型匹配、环）由 Go Semantic Validator 完成。
// 与 internal/workflow/definition.go 的 Go Struct 必须同步修改（见 docs/DEVELOPMENT.md §5）。

apiVersion: "workflow/v1"
kind:       "Workflow"

metadata: {
	name:    string
	version?: string
}

project: {
	repository: string
	branch?:    string
}

nodes: {
	[string]: {
		type: string

		inputs?: {[string]: {
			from: string
		}}

		// dependsOn 是可选字段：仅表达 Control Edge（执行顺序约束）。
		dependsOn?: [...string]

		config?: {...}
	}
}
