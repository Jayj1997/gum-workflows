package definition

import "fmt"

// ParseTypeExpr 解析端口类型表达式（设计文档 §4 语法）：
//
//	typeExpr := union | single
//	union    := single ("|" single)+
//	single   := atomic | kind | list
//	list     := "[" typeExpr "]"
//	atomic   := string|int|bool|float|markdown|file|file:ext
//	kind     := 语义 Artifact Kind（大写开头的驼峰标识符）
//
// 「已注册 Kind」检查不在本函数：解析层只识别标识符形态，注册表校验
// 属语义层（设计文档 §10 检查 #3），经 Kinds() 取引用名完成--解析与
// 校验分离，使本函数零依赖。
//
// 错误信息携带表达式内偏移（position N），便于上游定位到具体端口。
func ParseTypeExpr(expr string) (TypeExpr, error) {
	p := &parser{src: expr}
	t, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(p.src) {
		return nil, p.errorf("unexpected trailing input")
	}
	return t, nil
}

// parser 是 TypeExpr 的递归下降解析器。pos 始终指向下一个未消费字符。
type parser struct {
	src string
	pos int
}

// parseExpr 解析顶层：single ("|" single)*，成员 ≥2 时折叠为 Union。
func (p *parser) parseExpr() (TypeExpr, error) {
	first, err := p.parseSingle()
	if err != nil {
		return nil, err
	}
	members := []TypeExpr{first}
	for {
		save := p.pos
		p.skipSpace()
		if !p.consume('|') {
			p.pos = save
			break
		}
		p.skipSpace()
		next, err := p.parseSingle()
		if err != nil {
			return nil, err
		}
		members = append(members, next)
	}
	if len(members) == 1 {
		return first, nil
	}
	return Union{Members: members}, nil
}

// parseSingle 解析非 union 的单个成员：list 或原子/Kind。
func (p *parser) parseSingle() (TypeExpr, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return nil, p.errorf("expected a type, found end of expression")
	}
	if p.consume('[') {
		elem, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if !p.consume(']') {
			return nil, p.errorf("expected %q to close list", "]")
		}
		return List{Elem: elem}, nil
	}
	return p.parseAtomOrKind()
}

// parseAtomOrKind 解析一个标识符形态的成员：原子类型、file:ext 或 Kind 引用。
// 标识符字符集与 Kind 一致（字母/数字）；原子类型名不含连字符。
func (p *parser) parseAtomOrKind() (TypeExpr, error) {
	start := p.pos
	for p.pos < len(p.src) && isKindChar(p.src[p.pos]) {
		p.pos++
	}
	name := p.src[start:p.pos]
	if name == "" {
		return nil, p.errorf("expected a type, found %q", p.peek())
	}

	// file 特判：允许可选 :ext 后缀（ext := [a-z0-9]+）。
	if name == string(AtomicFile) {
		if p.pos < len(p.src) && p.src[p.pos] == ':' {
			p.pos++ // 消费 ':'
			extStart := p.pos
			for p.pos < len(p.src) && isExtChar(p.src[p.pos]) {
				p.pos++
			}
			ext := p.src[extStart:p.pos]
			if ext == "" {
				return nil, fmt.Errorf("type expression %q: position %d: file extension after %q must be [a-z0-9]+",
					p.src, p.pos, ":")
			}
			// 后缀后必须是分隔符（'|' ']' 或结尾），否则是非法尾随字符。
			if p.pos < len(p.src) && !isDelim(p.src[p.pos]) {
				return nil, p.errorf("invalid file extension character %q (want [a-z0-9]+)", string(p.src[p.pos]))
			}
			return Atomic{Name: AtomicFile, Ext: ext}, nil
		}
		return Atomic{Name: AtomicFile}, nil
	}

	if isAtomic(name) {
		return Atomic{Name: AtomicName(name)}, nil
	}

	// 非原子：大写开头即 Kind 引用，否则非法。
	// Kind 不检查注册表（解析与校验分离，见 ParseTypeExpr 文档）。
	if isKindIdent(name) {
		return KindRef{Name: name}, nil
	}
	// 未知标识符：报出成员的起止区间，便于上游定位（如 "[foo]" 内的 foo）。
	return nil, fmt.Errorf("type expression %q: position %d-%d: unknown type %q (atomic: string|int|bool|float|markdown|file[:ext], or an uppercase Artifact Kind)",
		p.src, start, p.pos, name)
}

// consume 在当前字符为 c 时前进一步并返回 true。
func (p *parser) consume(c byte) bool {
	if p.pos < len(p.src) && p.src[p.pos] == c {
		p.pos++
		return true
	}
	return false
}

// skipSpace 跳过空白（表达式内宽容，规范文本不带空白）。
func (p *parser) skipSpace() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
		p.pos++
	}
}

// peek 返回当前字符（越界时为空串），仅用于错误信息。
func (p *parser) peek() string {
	if p.pos >= len(p.src) {
		return ""
	}
	return string(p.src[p.pos])
}

// errorf 生成携带表达式内位置的错误（§4.2 定位要求在表达式粒度）。
func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("type expression %q: position %d: %s", p.src, p.pos, fmt.Sprintf(format, args...))
}

// isAtomic 报告 name 是否为原子类型名（file 已在前置分支处理）。
func isAtomic(name string) bool {
	switch AtomicName(name) {
	case AtomicString, AtomicInt, AtomicBool, AtomicFloat, AtomicMarkdown:
		return true
	}
	return false
}

// isKindIdent 报告 name 是否为合法 Kind 标识符：大写字母开头，
// 后续为字母/数字（与 artifact.Kind 的驼峰命名一致，如 SourceCode；
// 连字符不合法，避免 "Source-Code" 这类拼写被静默接受）。
func isKindIdent(name string) bool {
	if name == "" || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isKindChar(name[i]) {
			return false
		}
	}
	return true
}

// isKindChar 是 Kind 标识符的合法字符（字母/数字，不含连字符）。
func isKindChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// isExtChar 是 file 扩展名的合法字符（[a-z0-9]）。
func isExtChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

// isDelim 报告 c 是否为成员边界字符（之后允许出现的位置）。
func isDelim(c byte) bool {
	return c == '|' || c == ']' || c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// Kinds 遍历表达式中引用的全部语义 Kind 名（去重、按出现序）。
// 定义层校验据此检查 Kind 注册，避免解析器依赖 artifact.Registry。
func Kinds(e TypeExpr) []string {
	var names []string
	var walk func(TypeExpr)
	seen := map[string]bool{}
	walk = func(t TypeExpr) {
		switch v := t.(type) {
		case KindRef:
			if !seen[v.Name] {
				seen[v.Name] = true
				names = append(names, v.Name)
			}
		case List:
			walk(v.Elem)
		case Union:
			for _, m := range v.Members {
				walk(m)
			}
		}
	}
	walk(e)
	return names
}
