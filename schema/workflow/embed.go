// Package workflow 提供 workflow/v1 的 CUE Schema（go:embed），
// 供 validation 包在 Go Runtime 中内嵌校验（设计计划 §21）。
package workflow

import _ "embed"

// V1 是 workflow/v1 的 CUE Schema 内容。
//
//go:embed v1.cue
var V1 []byte
