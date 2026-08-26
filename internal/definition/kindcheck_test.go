package definition

import (
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
)

func TestValidateKinds(t *testing.T) {
	def := NodeDefinition{
		Metadata: Metadata{Name: "test-node", Description: "d"},
		Type:     TypeAgent,
		Inputs: map[string]InputPort{
			"spec":        {Type: "ArchitectureSpec"},
			"optional-in": {Type: "FrontendSDK", Optional: true},
			"plain":       {Type: "markdown"},
		},
		Outputs: map[string]OutputPort{
			"code": {Type: "[SourceCode]"},
		},
	}

	t.Run("all registered", func(t *testing.T) {
		if err := def.ValidateKinds(artifact.NewRegistry()); err != nil {
			t.Fatalf("ValidateKinds() unexpected error: %v", err)
		}
	})

	t.Run("unregistered kind in optional input is caught", func(t *testing.T) {
		// 旧实现漏检 OptionalInputs（设计文档 §10 检查 #3 的修复点）。
		bad := def
		bad.Inputs = map[string]InputPort{
			"optional-in": {Type: "GhostSpec", Optional: true},
		}
		err := bad.ValidateKinds(artifact.NewRegistry())
		if err == nil {
			t.Fatal("ValidateKinds() = nil error, want optional-port kind rejection")
		}
		if !strings.Contains(err.Error(), "GhostSpec") || !strings.Contains(err.Error(), "optional-in") {
			t.Errorf("error %q should name the port and kind", err)
		}
	})

	t.Run("unregistered kind in nested list output is caught", func(t *testing.T) {
		bad := def
		bad.Outputs = map[string]OutputPort{
			"code": {Type: "[GhostSpec]"},
		}
		err := bad.ValidateKinds(artifact.NewRegistry())
		if err == nil {
			t.Fatal("ValidateKinds() = nil error, want nested-kind rejection")
		}
		if !strings.Contains(err.Error(), "GhostSpec") {
			t.Errorf("error %q should name the kind inside the list", err)
		}
	})
}
