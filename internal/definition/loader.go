package definition

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// LoadNodeType 解析单份 Node Type Definition YAML（严格模式）并做语义校验。
func LoadNodeType(data []byte) (NodeTypeDefinition, error) {
	var t NodeTypeDefinition
	if err := decodeStrict(data, &t); err != nil {
		return NodeTypeDefinition{}, err
	}
	if err := t.Validate(); err != nil {
		return NodeTypeDefinition{}, fmt.Errorf("validate node type definition:\n%w", err)
	}
	return t, nil
}

// LoadNodeDefinition 解析单份 Node Definition YAML（严格模式）并做语义校验。
func LoadNodeDefinition(data []byte) (NodeDefinition, error) {
	var d NodeDefinition
	if err := decodeStrict(data, &d); err != nil {
		return NodeDefinition{}, err
	}
	if err := d.Validate(); err != nil {
		return NodeDefinition{}, fmt.Errorf("validate node definition:\n%w", err)
	}
	return d, nil
}

// LoadNodeExecutor 解析单份 Node Executor Definition YAML（严格模式）
// 并做语义校验（跨声明的集合级检查由 Registry 承接）。
func LoadNodeExecutor(data []byte) (NodeExecutorDefinition, error) {
	var e NodeExecutorDefinition
	if err := decodeStrict(data, &e); err != nil {
		return NodeExecutorDefinition{}, err
	}
	if err := e.Validate(); err != nil {
		return NodeExecutorDefinition{}, fmt.Errorf("validate node executor definition:\n%w", err)
	}
	return e, nil
}

// decodeStrict 以 KnownFields(true) 解析单文档 YAML，未知字段直接报错，
// 防 Schema 漂移（docs/DEVELOPMENT.md §5）。
func decodeStrict(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		if err == io.EOF {
			return fmt.Errorf("parse YAML: empty definition file")
		}
		return fmt.Errorf("parse YAML: %w", err)
	}
	// 单文档声明：尾随第二份文档视为非法（多文件合并由加载方逐份处理）。
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("parse YAML: expected a single document")
	}
	return nil
}
