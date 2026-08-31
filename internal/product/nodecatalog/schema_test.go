package nodecatalog_test

import (
	"reflect"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
)

func TestConfigSchemaAppliesDefaultsAndValidatesEveryV1FieldType(t *testing.T) {
	minZero, maxTwo := 0.0, 2.0
	minOne, maxTokens := 1.0, 128000.0
	schema := nodecatalog.ConfigSchema{Fields: []nodecatalog.ConfigField{
		{Name: "title", Type: nodecatalog.FieldString, Required: true},
		{Name: "instructions", Type: nodecatalog.FieldMarkdown, Default: "Be concise", HasDefault: true},
		{Name: "attempts", Type: nodecatalog.FieldInteger, Min: &minOne, Max: &maxTokens},
		{Name: "temperature", Type: nodecatalog.FieldNumber, Min: &minZero, Max: &maxTwo},
		{Name: "enabled", Type: nodecatalog.FieldBoolean, Default: true, HasDefault: true},
		{Name: "tone", Type: nodecatalog.FieldEnum, Values: []string{"direct", "friendly"}},
		{Name: "secret", Type: nodecatalog.FieldString, Sensitive: true},
	}}

	got := schema.WithDefaults(map[string]any{"title": "Release", "attempts": 4, "temperature": 0.7, "tone": "direct"})
	want := map[string]any{
		"title": "Release", "instructions": "Be concise", "attempts": 4,
		"temperature": 0.7, "enabled": true, "tone": "direct",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config with defaults = %#v, want %#v", got, want)
	}
	if issues := schema.Validate(got); len(issues) != 0 {
		t.Fatalf("valid config issues = %#v", issues)
	}

	issues := schema.Validate(map[string]any{
		"attempts": 0.5, "temperature": 3, "enabled": "yes", "tone": "verbose", "extra": true,
	})
	wantFields := []string{"title", "attempts", "temperature", "enabled", "tone", "extra"}
	if len(issues) != len(wantFields) {
		t.Fatalf("issues = %#v, want fields %v", issues, wantFields)
	}
	for i, field := range wantFields {
		if issues[i].Field != field {
			t.Fatalf("issue %d field = %q, want %q", i, issues[i].Field, field)
		}
	}
}

func TestPresentationHintsDoNotChangeConfigValidation(t *testing.T) {
	contract := nodecatalog.ConfigField{Name: "instructions", Type: nodecatalog.FieldMarkdown, Required: true}
	plain := nodecatalog.ConfigSchema{Fields: []nodecatalog.ConfigField{contract}}
	presented := nodecatalog.ConfigSchema{Fields: []nodecatalog.ConfigField{{
		Name: contract.Name, Type: contract.Type, Required: contract.Required,
		Presentation: nodecatalog.PresentationHint{Label: "System guidance", Help: "Sent before the conversation", Editor: "large-markdown"},
	}}}

	if !reflect.DeepEqual(plain.Validate(map[string]any{}), presented.Validate(map[string]any{})) {
		t.Fatalf("presentation changed validation: plain=%#v presented=%#v", plain.Validate(map[string]any{}), presented.Validate(map[string]any{}))
	}
}
