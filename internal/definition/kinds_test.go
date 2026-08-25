package definition

import (
	"reflect"
	"testing"
)

// TestKindsExtractsKindRefs 验证语义 Kind 提取：
// 定义层校验（Kind 已注册检查）的输入数据来源。
func TestKindsExtractsKindRefs(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want []string
	}{
		{name: "no kinds", expr: "string", want: nil},
		{name: "no kinds in list", expr: "[string|int]", want: nil},
		{name: "single kind", expr: "SourceCode", want: []string{"SourceCode"}},
		{name: "kind in union", expr: "markdown|SourceCode", want: []string{"SourceCode"}},
		{name: "kind in list", expr: "[OpenAPI]", want: []string{"OpenAPI"}},
		{
			name: "kinds nested and deduped",
			expr: "[SourceCode|OpenAPI]|[SourceCode]",
			want: []string{"SourceCode", "OpenAPI"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := mustParse(t, tt.expr)
			if got := Kinds(e); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Kinds(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}
