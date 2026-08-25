package definition

import (
	"reflect"
	"strings"
	"testing"
)

// mustParse 解析成功断言，失败即 Fatal。
func mustParse(t *testing.T, expr string) TypeExpr {
	t.Helper()
	got, err := ParseTypeExpr(expr)
	if err != nil {
		t.Fatalf("ParseTypeExpr(%q) unexpected error: %v", expr, err)
	}
	return got
}

func TestParseTypeExprForms(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want TypeExpr
	}{
		// 五种原子类型。
		{name: "string", expr: "string", want: Atomic{Name: AtomicString}},
		{name: "int", expr: "int", want: Atomic{Name: AtomicInt}},
		{name: "bool", expr: "bool", want: Atomic{Name: AtomicBool}},
		{name: "float", expr: "float", want: Atomic{Name: AtomicFloat}},
		{name: "markdown", expr: "markdown", want: Atomic{Name: AtomicMarkdown}},
		// file 家族：裸 file 与 file:ext（ext 为 [a-z0-9]+）。
		{name: "bare file", expr: "file", want: Atomic{Name: AtomicFile}},
		{name: "file jpg", expr: "file:jpg", want: Atomic{Name: AtomicFile, Ext: "jpg"}},
		{name: "file png", expr: "file:png", want: Atomic{Name: AtomicFile, Ext: "png"}},
		{name: "file pdf", expr: "file:pdf", want: Atomic{Name: AtomicFile, Ext: "pdf"}},
		{name: "file digits ext", expr: "file:mp3h264", want: Atomic{Name: AtomicFile, Ext: "mp3h264"}},
		// 语义 Kind：大写开头标识符。
		{name: "kind", expr: "SourceCode", want: KindRef{Name: "SourceCode"}},
		{name: "kind multiword", expr: "RequirementSpec", want: KindRef{Name: "RequirementSpec"}},
		// union：≥2 成员，跨原子/Kind/file:ext 混排。
		{
			name: "union two atomics",
			expr: "string|int",
			want: Union{Members: []TypeExpr{Atomic{Name: AtomicString}, Atomic{Name: AtomicInt}}},
		},
		{
			name: "union mixed members",
			expr: "string|file:jpg|file:png",
			want: Union{Members: []TypeExpr{
				Atomic{Name: AtomicString},
				Atomic{Name: AtomicFile, Ext: "jpg"},
				Atomic{Name: AtomicFile, Ext: "png"},
			}},
		},
		{
			name: "union with kind",
			expr: "markdown|SourceCode",
			want: Union{Members: []TypeExpr{Atomic{Name: AtomicMarkdown}, KindRef{Name: "SourceCode"}}},
		},
		// list：单成员与嵌套。
		{name: "list of string", expr: "[string]", want: List{Elem: Atomic{Name: AtomicString}}},
		{name: "list of kind", expr: "[SourceCode]", want: List{Elem: KindRef{Name: "SourceCode"}}},
		{
			name: "list of union",
			expr: "[string|int]",
			want: List{Elem: Union{Members: []TypeExpr{
				Atomic{Name: AtomicString}, Atomic{Name: AtomicInt},
			}}},
		},
		{
			name: "nested list",
			expr: "[[string]]",
			want: List{Elem: List{Elem: Atomic{Name: AtomicString}}},
		},
		{
			name: "union of lists",
			expr: "[string]|[SourceCode]",
			want: Union{Members: []TypeExpr{
				List{Elem: Atomic{Name: AtomicString}},
				List{Elem: KindRef{Name: "SourceCode"}},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustParse(t, tt.expr)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseTypeExpr(%q) = %#v, want %#v", tt.expr, got, tt.want)
			}
			// 规范化文本可再解析：String() 的输出必须是合法表达式，
			// 且二次解析结果与一次解析等价（往返不变）。
			round := mustParse(t, got.String())
			if !reflect.DeepEqual(round, got) {
				t.Errorf("ParseTypeExpr(String()) = %#v, want %#v", round, got)
			}
		})
	}
}

func TestParseTypeExprErrors(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr string // 错误信息必须包含的片段（定位 + 原因）
	}{
		{name: "empty", expr: "", wantErr: "position 0"},
		{name: "blank", expr: "   ", wantErr: "position 3"},
		// 工单示例 foo|：未知标识符先于悬空 | 报出（position 0 定位）；
		// 悬空成员的定位由 "string|" 形态单独覆盖。
		{name: "unknown identifier", expr: "foo", wantErr: "position 0-3"},
		{name: "missing union member", expr: "string|", wantErr: "position 7"},
		{name: "leading union bar", expr: "|string", wantErr: "position 0"},
		// 工单示例 [unclosed：unclosed 是未知标识符（先报出）；
		// 缺右括号的诊断由合法成员的 "[markdown" 形态覆盖。
		{name: "unknown identifier unclosed list", expr: "[unclosed", wantErr: "position 1-9"},
		{name: "unclosed list valid member", expr: "[markdown", wantErr: `expected "]" to close list`},
		{name: "unclosed nested list", expr: "[[string]", wantErr: "position 9"},
		{name: "close without open", expr: "string]", wantErr: "position 6"},
		{name: "double bar", expr: "string||int", wantErr: "position 7"},
		{name: "empty list", expr: "[]", wantErr: "position 1"},
		{name: "unknown identifier in union", expr: "string|foo", wantErr: "position 7-10"},
		{name: "unknown identifier in list", expr: "[foo]", wantErr: "position 1-4"},
		{name: "file with empty ext", expr: "file:", wantErr: "position 5"},
		{name: "file with uppercase ext", expr: "file:JPG", wantErr: "position 5"},
		{name: "file with dot ext", expr: "file:a.b", wantErr: "position 6"},
		{name: "kind not uppercase", expr: "sourceCode", wantErr: "position 0"},
		// 连字符不在 Kind 字符集内：Source 解析成功后 -Code 成为尾随输入。
		{name: "kind with hyphen", expr: "Source-Code", wantErr: "position 6: unexpected trailing input"},
		{name: "trailing junk after atom", expr: "string junk", wantErr: "position 7"},
		{name: "list unclosed after union", expr: "[string|int", wantErr: "position 11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTypeExpr(tt.expr)
			if err == nil {
				t.Fatalf("ParseTypeExpr(%q) = nil error, want rejection containing %q", tt.expr, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ParseTypeExpr(%q) error = %q, want containing %q", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestParseTypeExprWhitespace(t *testing.T) {
	// 表达式内空白宽容：声明者常在 | 两侧留空（YAML 内多行字符串）。
	got := mustParse(t, "string | int")
	want := Union{Members: []TypeExpr{Atomic{Name: AtomicString}, Atomic{Name: AtomicInt}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseTypeExpr(%q) = %#v, want %#v", "string | int", got, want)
	}
}
