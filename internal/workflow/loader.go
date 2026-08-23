package workflow

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFile 读取并解析 workflow YAML 文件。
func LoadFile(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read workflow file: %w", err)
	}
	def, err := Load(data)
	if err != nil {
		return Definition{}, fmt.Errorf("load %s: %w", path, err)
	}
	return def, nil
}

// Load 以严格模式解析 YAML 为 Definition。
// 未知字段直接报错，防止 Schema 漂移（结构层校验由 CUE 负责，见 validation 包）。
func Load(data []byte) (Definition, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var def Definition
	if err := dec.Decode(&def); err != nil {
		if err == io.EOF {
			return Definition{}, fmt.Errorf("parse YAML: empty workflow file")
		}
		return Definition{}, fmt.Errorf("parse YAML: %w", err)
	}
	return def, nil
}
