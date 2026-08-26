package llm

import "strings"

// ValidationErrors 聚合一次校验发现的全部问题，
// 一次报出而不是在第一个错误处短路（见 docs/DEVELOPMENT.md §4.2）。
type ValidationErrors []error

// Error 将全部错误逐行拼接。
func (e ValidationErrors) Error() string {
	msgs := make([]string, len(e))
	for i, err := range e {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "\n")
}

// OrNil 在无错误时返回 nil，便于作为 error 返回值。
func (e ValidationErrors) OrNil() error {
	if len(e) == 0 {
		return nil
	}
	return e
}
