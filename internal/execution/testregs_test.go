package execution

import (
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// testContract 是测试用 ExecutorFactory 的可选契约声明：
// 实现它的 factory 其端口契约会被注册进内存 definition.Registry，
// 引擎与语义校验即从 YAML 同构的内存契约读取。
type testContract interface {
	Contract() (inputs map[string]definition.InputPort, outputs map[string]definition.OutputPort)
}

// newTestRegistries 依据 factories 构造内存 definition.Registry 与
// ExecutorRegistry（引擎测试不依赖内置节点集）。definition 名取
// factory.Definition()，契约取 testContract（未实现则视为无端口）。
func newTestRegistries(t *testing.T, factories ...node.ExecutorFactory) (*definition.Registry, *node.ExecutorRegistry) {
	t.Helper()

	dr := definition.NewRegistry()
	for _, nt := range []definition.NodeType{
		definition.TypeAgent, definition.TypeAutomation, definition.TypeHuman,
	} {
		if err := dr.RegisterNodeType(definition.NodeTypeDefinition{
			APIVersion: definition.NodeTypeAPIVersionV1,
			Kind:       definition.NodeTypeDefinitionKind,
			Metadata:   definition.Metadata{Name: string(nt), Description: "test"},
		}); err != nil {
			t.Fatalf("register node type %s: %v", nt, err)
		}
	}

	er := node.NewExecutorRegistry()
	for _, f := range factories {
		d := definition.NodeDefinition{
			APIVersion: definition.NodeDefinitionAPIVersionV1,
			Kind:       definition.NodeDefinitionKind,
			Metadata:   definition.Metadata{Name: f.Definition(), Description: "test"},
			Type:       definition.TypeAgent,
		}
		if c, ok := f.(testContract); ok {
			d.Inputs, d.Outputs = c.Contract()
		}
		if err := dr.RegisterDefinition(d); err != nil {
			t.Fatalf("register definition %q: %v", f.Definition(), err)
		}
		if err := er.Register(f); err != nil {
			t.Fatalf("register executor %q: %v", f.Definition(), err)
		}
	}
	return dr, er
}
