// workflow/v1 的 CUE Schema：结构层校验（设计计划 §21-§22）。
// 只声明字段存在性与类型约束；语义校验（引用、类型匹配、环）由 Go Semantic Validator 完成。
// 与 internal/workflow/definition.go 的 Go Struct 必须同步修改（见 docs/DEVELOPMENT.md §5）。

apiVersion: "workflow/v1"
kind:       "workflow"

metadata: {
	name:        string
	version?:    string
	description?: string
}

// projects 列表（设计文档 §3.5）：本期结构就位；
// 「恰好 1 个」的校验属语义层（票 06）。
projects: [...{
	name:       string
	repository: string
}]

nodes: {
	[string]: {
		node: string

		executor?: string
		llm?:      string
		target_model?: string

		metadata?: {...}

		inputs?: {[string]: {
			from: string
		}}

		// dependsOn 是可选字段：仅表达 Control Edge（执行顺序约束）。
		dependsOn?: [...string]

		config?: {...}
	}
}
