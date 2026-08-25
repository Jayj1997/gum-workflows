// Package definition 实现定义侧组件（设计文档 §3）：
// Node Type / Node Definition / Node Executor 的 YAML 声明形态、
// 端口类型语言 TypeExpr（§4）与内嵌种子加载。
//
// 本包是契约的唯一权威：inputs/outputs 端口类型自此以 YAML 声明，
// 不再来自 Go 代码（Schema 迁移由后续票完成）。
package definition

import "strings"

// TypeExpr 是端口类型表达式（设计文档 §4）的内存形态。
//
// 四种具体形态：
//   - Atomic（string/int/bool/float/markdown 及 file/file:ext）
//   - KindRef（语义 Artifact Kind，大写开头）
//   - List（[T]，多值端口，成员递归）
//   - Union（A|B|C，≥2 成员）
//
// ParseTypeExpr 的成功结果恒为非 nil。
type TypeExpr interface {
	// String 返回规范化文本（无空白、成员按声明序），可再解析为等价表达式。
	String() string
}

// AtomicName 是原子类型名的类型化枚举（DEVELOPMENT.md §4.3）。
type AtomicName string

// 原子类型名。file 特殊：可带 :ext 后缀（ext := [a-z0-9]+），其余恒裸名。
const (
	AtomicString   AtomicName = "string"
	AtomicInt      AtomicName = "int"
	AtomicBool     AtomicName = "bool"
	AtomicFloat    AtomicName = "float"
	AtomicMarkdown AtomicName = "markdown"
	AtomicFile     AtomicName = "file"
)

// Atomic 是原子类型。Name 为 file 时 Ext 为文件扩展名（jpg、pdf…），空表示裸 file。
type Atomic struct {
	Name AtomicName
	Ext  string
}

// KindRef 引用一个语义 Kind（artifact.Registry 登记，如 SourceCode）。
// 兼容判断与 Artifact 的 Kind 字段对齐：字面相等，无隐式子类型。
type KindRef struct {
	Name string
}

// List 是多值端口类型 [T]，成员可为任意 TypeExpr（含 union）。
type List struct {
	Elem TypeExpr
}

// Union 是复合类型 A|B|C，任一成员匹配即通过（≥2 成员）。
type Union struct {
	// Members 按声明序排列；Compatible 判断时顺序无关。
	Members []TypeExpr
}

// String 实现TypeExpr，返回规范化文本。
func (a Atomic) String() string {
	if a.Name == AtomicFile && a.Ext != "" {
		return string(a.Name) + ":" + a.Ext
	}
	return string(a.Name)
}

// String 实现TypeExpr，返回 Kind 名。
func (k KindRef) String() string { return k.Name }

// String 实现TypeExpr，返回 [成员] 形态。
func (l List) String() string { return "[" + l.Elem.String() + "]" }

// String 实现TypeExpr，返回 成员|成员 形态。
func (u Union) String() string {
	parts := make([]string, len(u.Members))
	for i, m := range u.Members {
		parts[i] = m.String()
	}
	return strings.Join(parts, "|")
}
