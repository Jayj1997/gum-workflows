// Package definition 实现定义侧组件（设计文档 §3）：
// Node Type / Node Definition / Node Executor 的 YAML 声明形态、
// 端口类型语言 TypeExpr（§4）与内嵌种子加载。
//
// 本包是契约的唯一权威：inputs/outputs 端口类型自此以 YAML 声明，
// 不再来自 Go 代码（Schema 迁移由后续票完成）。
package definition

import "strings"

// 原子类型的合法名（设计文档 §4）。
// file 特殊：可带 :ext 后缀（[a-z0-9]+），其余四者恒裸名。
const (
	AtomicString   = "string"
	AtomicInt      = "int"
	AtomicBool     = "bool"
	AtomicFloat    = "float"
	AtomicMarkdown = "markdown"
	AtomicFile     = "file"
)

// TypeExpr 是端口类型表达式（设计文档 §4）的内存形态。
//
// 五种具体形态：
//   - Atomic（string/int/bool/float/markdown 及 file/file:ext）
//   - KindRef（已注册 Artifact Kind，大写开头）
//   - List（[T]，多值端口，成员递归）
//   - Union（A|B|C，≥2 成员）
//
// 空值 nil 不合法，ParseTypeExpr 的成功结果恒为非 nil。
type TypeExpr interface {
	// String 返回规范化的表达式文本（往返解析不变）。
	String() string
}

// Atomic 是原子类型。Name 为 string/int/bool/float 之一时 Ext 恒空；
// Name 为 file 时 Ext 为文件扩展名（jpg、pdf…），空表示裸 file。
type Atomic struct {
	Name string
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
		return a.Name + ":" + a.Ext
	}
	return a.Name
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
