package execution

import (
	"fmt"
	"os"
	"regexp"
)

// executionIDPattern 匹配 execution 目录名（execution-000001）。
var executionIDPattern = regexp.MustCompile(`^execution-([0-9]{6})$`)

// NextExecutionID 扫描 baseDir 下已有的 execution-* 目录，
// 返回下一个可用 ID（最大序号 + 1）。目录分配是 Execution ID 的
// 唯一来源：CLI 与 Engine 必须共用本函数，避免双轨编号相互覆盖
// （state 目录、workspace、artifacts 都以此 ID 定位，计划 §28）。
func NextExecutionID(baseDir string) (string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "execution-000001", nil
		}
		return "", fmt.Errorf("next execution id: read %s: %w", baseDir, err)
	}

	max := 0
	for _, e := range entries {
		m := executionIDPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return fmt.Sprintf("execution-%06d", max+1), nil
}
