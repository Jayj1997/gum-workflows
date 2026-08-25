package definition

import "testing"

// parse 是兼容矩阵用例的简写：表达式非法即测试失败。
func parse(t *testing.T, expr string) TypeExpr {
	t.Helper()
	got, err := ParseTypeExpr(expr)
	if err != nil {
		t.Fatalf("ParseTypeExpr(%q): %v", expr, err)
	}
	return got
}

// TestCompatibleMatrix 覆盖设计文档 §4 匹配规则的兼容矩阵：
// consumer ⊇ producer（生产者的每个可能取值都落在消费者集合内）。
// 期望值是独立推导的规范结果，不重算实现逻辑。
func TestCompatibleMatrix(t *testing.T) {
	tests := []struct {
		name     string
		consumer string // 消费者（声明在 inputs）
		producer string // 生产者（声明在 outputs）
		want     bool
	}{
		// 原子/Kind：字面相等。
		{"string vs string", "string", "string", true},
		{"markdown vs markdown", "markdown", "markdown", true},
		{"int vs int", "int", "int", true},
		{"bool vs bool", "bool", "bool", true},
		{"float vs float", "float", "float", true},
		{"kind vs same kind", "SourceCode", "SourceCode", true},

		// 无隐式子类型：字面不等一律不兼容。
		{"markdown vs string", "string", "markdown", false},
		{"string vs markdown", "markdown", "string", false},
		{"int vs float", "float", "int", false},
		{"kind vs string", "string", "SourceCode", false},
		{"kind vs other kind", "OpenAPI", "SourceCode", false},

		// file 家族：裸 file 与 file:ext 互不隐式兼容。
		{"file vs file", "file", "file", true},
		{"file:jpg vs file:jpg", "file:jpg", "file:jpg", true},
		{"file:jpg vs file:png", "file:png", "file:jpg", false},
		{"file vs file:jpg (no widening)", "file", "file:jpg", false},
		{"file:jpg vs file (no narrowing)", "file:jpg", "file", false},

		// union：一侧展开后按成员包含。
		{"union contains atomic", "string|int", "string", true},
		{"union contains both atomics", "string|int", "int", true},
		{"atomic vs union producer", "string", "string|int", false},
		{"union vs union subset", "string|int|bool", "string|int", true},
		{"union vs union overlap", "string|int", "int|bool", false},
		{"union with kind member", "markdown|SourceCode", "SourceCode", true},
		{"union missing kind member", "markdown|OpenAPI", "SourceCode", false},
		{"union covers file variants", "file:jpg|file:png", "file:jpg", true},
		{"union bare file covers ext", "file", "file:jpg", false}, // 无隐式：union 成员字面相等

		// list：递归比较成员。
		{"list vs list same", "[string]", "[string]", true},
		{"list vs list different", "[string]", "[int]", false},
		{"list vs bare atomic", "string", "[string]", false},
		{"bare atomic vs list", "[string]", "string", false},
		{"list of kind", "[SourceCode]", "[SourceCode]", true},
		{"list vs list kind mismatch", "[OpenAPI]", "[SourceCode]", false},

		// 嵌套：[string|int] vs [string]。
		{"nested list union consumer", "[string|int]", "[string]", true},
		{"nested list union producer", "[string]", "[string|int]", false},
		{"nested list union both", "[string|int|bool]", "[string|int]", true},
		{"list of union vs list of union overlap", "[string|int]", "[int|bool]", false},
		{"list of file variants", "[file:jpg|file:png]", "[file:jpg]", true},

		// 深层嵌套。
		{"nested list of list", "[[string]]", "[[string]]", true},
		{"nested list of list mismatch", "[[string]]", "[[int]]", false},
		{"nested list of union vs nested", "[[string|int]]", "[[string]]", true},
		{"union of lists", "[string]|[int]", "[string]", true},
		{"union of lists producer", "string", "[string]|[int]", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumer := parse(t, tt.consumer)
			producer := parse(t, tt.producer)
			if got := Compatible(consumer, producer); got != tt.want {
				t.Errorf("Compatible(consumer=%s, producer=%s) = %v, want %v",
					tt.consumer, tt.producer, got, tt.want)
			}
		})
	}
}

// TestCompatibleCommutativeDocuments 语义方向性文档化：
// 兼容判断不对称，方向错用必须给出不同结果（consumer ⊇ producer）。
func TestCompatibleDirection(t *testing.T) {
	wide := parse(t, "string|int")
	narrow := parse(t, "string")

	if !Compatible(wide, narrow) {
		t.Error("Compatible(string|int, string) = false, want true (wide consumer accepts narrow producer)")
	}
	if Compatible(narrow, wide) {
		t.Error("Compatible(string, string|int) = true, want false (narrow consumer rejects wide producer)")
	}
}
