package llm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrConfigNotFound 表示两个候选路径都没有 llm.yaml。
// 无 agent 节点的 workflow 合法地不需要它（设计文档 §10 检查 9），
// 调用方以 errors.Is 区分「文件不存在」与「文件存在但内容非法」。
var ErrConfigNotFound = errors.New("llm.yaml not found")

// Load 以严格模式解析 YAML 为 Config，并在加载时完成：
//
//  1. $VAR 环境变量引用解析（变量缺失报错并指明变量名）；
//  2. 语义校验（Config.Validate）。
func Load(data []byte) (Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var c Config
	if err := dec.Decode(&c); err != nil {
		if err == io.EOF {
			return Config{}, fmt.Errorf("parse YAML: empty llm.yaml file")
		}
		return Config{}, fmt.Errorf("parse YAML: %w", err)
	}

	if err := resolveAPIKeys(&c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate llm.yaml:\n%w", err)
	}
	return c, nil
}

// LoadFile 从指定路径加载 llm.yaml。
func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read llm.yaml: %w", err)
	}
	c, err := Load(data)
	if err != nil {
		return Config{}, fmt.Errorf("load %s: %w", path, err)
	}
	return c, nil
}

// LoadDefault 按设计文档 §3.4 的顺序查找用户级 llm.yaml：
//
//	$XDG_CONFIG_HOME/gum-workflows/llm.yaml -> ~/.config/gum-workflows/llm.yaml
//
// 两个路径都不存在时返回 ErrConfigNotFound（哨兵错误，见 DEVELOPMENT.md §4.2）。
func LoadDefault() (Config, error) {
	paths := candidatePaths()
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Config{}, fmt.Errorf("stat %s: %w", path, err)
		}
		return LoadFile(path)
	}
	return Config{}, fmt.Errorf("%w (searched: %s)",
		ErrConfigNotFound, strings.Join(paths, ", "))
}

// CandidatePaths 返回 llm.yaml 的查找路径（有序：XDG_CONFIG_HOME 优先，
// HOME 兜底）。供 LoadDefault 与语义校验的错误信息共用，避免查找顺序
// 的知识分散漂移。
func CandidatePaths() []string { return candidatePaths() }

// candidatePaths 返回查找顺序中的两个候选路径。
func candidatePaths() []string {
	paths := make([]string, 0, 2)
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "gum-workflows", "llm.yaml"))
	}
	if home := os.Getenv("HOME"); home != "" {
		paths = append(paths, filepath.Join(home, ".config", "gum-workflows", "llm.yaml"))
	}
	return paths
}

// resolveAPIKeys 把 provider 的 apikey 中 `$VAR` 形式解析为环境变量值。
// 明文 apikey 原样保留；`$VAR` 引用的变量缺失时报错并指明变量名（设计文档 §3.4）。
func resolveAPIKeys(c *Config) error {
	var errs ValidationErrors
	for i := range c.Providers {
		p := &c.Providers[i]
		if !strings.HasPrefix(p.APIKey, "$") {
			continue
		}
		varName := strings.TrimPrefix(p.APIKey, "$")
		value, ok := os.LookupEnv(varName)
		if !ok {
			errs = append(errs, fmt.Errorf(
				"provider %q: apikey references environment variable %q which is not set",
				p.Name, varName))
			continue
		}
		p.APIKey = value
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}
