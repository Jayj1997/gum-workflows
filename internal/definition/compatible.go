package definition

// Compatible 报告端口类型兼容性（设计文档 §4 匹配规则）：
//
//	兼容 = 生产者类型的每个可能取值都落在消费者类型集合内
//	（consumer ⊇ producer）
//
// 具体规则：
//   - 原子/Kind：字面相等（file:jpg 与 file:png、file 均互不相等）。
//   - List：两侧都是 List 且成员递归兼容。
//   - Union：consumer 展开后逐成员包含 producer 展开后的每个成员
//     （producer 的每个成员都必须被 consumer 的某个成员包含）。
//
// 无隐式子类型：markdown 不兼容 string、file:jpg 不兼容 file；
// 要宽必须显式写 union。方向不对称：Compatible(A, B) 为真不代表
// Compatible(B, A) 为真。
func Compatible(consumer, producer TypeExpr) bool {
	return contains(consumer, producer)
}

// contains 报告类型集合 superset 是否覆盖 subset 的全部可能取值。
// union 视为其成员集合，其余类型视为单元素集合。
func contains(superset, subset TypeExpr) bool {
	// producer 侧展开：每个成员都必须被 consumer 覆盖。
	if u, ok := subset.(Union); ok {
		for _, m := range u.Members {
			if !contains(superset, m) {
				return false
			}
		}
		return true
	}
	// consumer 侧展开：任一成员覆盖 producer 即可。
	if u, ok := superset.(Union); ok {
		for _, m := range u.Members {
			if contains(m, subset) {
				return true
			}
		}
		return false
	}
	// List：仅 List 可覆盖 List，成员递归。
	if cl, ok := superset.(List); ok {
		pl, ok := subset.(List)
		return ok && contains(cl.Elem, pl.Elem)
	}
	if _, ok := subset.(List); ok {
		return false // 非 List 不覆盖 List。
	}
	// 叶子：字面相等（含 file:ext；Kind 与原子均按名比较）。
	return equal(superset, subset)
}

// equal 报告两个叶子类型（Atomic/KindRef）是否字面相等。
func equal(a, b TypeExpr) bool {
	switch av := a.(type) {
	case Atomic:
		bv, ok := b.(Atomic)
		return ok && av.Name == bv.Name && av.Ext == bv.Ext
	case KindRef:
		bv, ok := b.(KindRef)
		return ok && av.Name == bv.Name
	}
	return false
}

// MatchesKind 报告运行期产出值（以 Artifact Kind 名表示）是否落在
// 声明类型 expr 的取值集合内（设计文档 §4 运行期输出契约检查）。
// 语义 Kind 按字面匹配（含 union/list 成员递归）；原子类型
// （string/int/bool/float/markdown/file）在运行期以同名 Kind 携带
// 标量数据，同样按字面匹配。
func MatchesKind(expr TypeExpr, kind string) bool {
	return contains(expr, KindRef{Name: kind}) || contains(expr, Atomic{Name: AtomicName(kind)})
}
