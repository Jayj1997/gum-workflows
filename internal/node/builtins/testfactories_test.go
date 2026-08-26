package builtins

import (
	"fmt"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// stubFactory 是测试用 ExecutorFactory：按给定 (definition, version) 注册。
type stubFactory struct {
	definition string
	version    string
}

func (f stubFactory) Definition() string { return f.definition }
func (f stubFactory) Version() string    { return f.version }
func (f stubFactory) Create(config node.Config) (node.Node, error) {
	return stubExecNode{}, nil
}

// orphanFactory 是测试用 ExecutorFactory：种子没有它的 YAML 声明。
type orphanFactory struct{}

func (orphanFactory) Definition() string { return "ghost-node" }
func (orphanFactory) Version() string    { return "v1" }
func (orphanFactory) Create(config node.Config) (node.Node, error) {
	return stubExecNode{}, nil
}

// stubExecNode 是测试用 Node（契约检查不在此层发生）。
type stubExecNode struct{}

func (stubExecNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	return nil, fmt.Errorf("stub")
}
