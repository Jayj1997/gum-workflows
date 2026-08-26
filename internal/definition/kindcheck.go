package definition

import (
	"fmt"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

// ValidateKinds 检查 Node Definition 契约中引用的全部语义 Kind 是否
// 已在 artifact Registry 登记（设计文档 §3.2、§10 检查 #3）。
//
// inputs 与 outputs 一视同仁，optional 输入同样检查--修复旧实现漏检
// OptionalInputs 的问题（该检查自 Go Schema 迁至 YAML 契约后归此处）。
// 错误聚合返回，逐端口定位。
func (d NodeDefinition) ValidateKinds(kinds *artifact.Registry) error {
	var errs ValidationErrors

	check := func(direction, name, expr string) {
		e, err := ParseTypeExpr(expr)
		if err != nil {
			// 语法非法已由 Validate 报出；此处跳过，避免同一问题双报。
			return
		}
		for _, kind := range Kinds(e) {
			if !kinds.Has(artifact.Kind(kind)) {
				errs = append(errs, fmt.Errorf(
					"node definition %q %s %q: references unregistered artifact kind %q",
					d.Metadata.Name, direction, name, kind))
			}
		}
	}
	for _, name := range sortedKeys(d.Inputs) {
		check("input", name, d.Inputs[name].Type)
	}
	for _, name := range sortedKeys(d.Outputs) {
		check("output", name, d.Outputs[name].Type)
	}
	return errs.OrNil()
}
