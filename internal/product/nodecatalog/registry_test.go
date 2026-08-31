package nodecatalog_test

import (
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
)

func TestBuiltinRegistryPublishesHumanAndLLMChatCatalogEntries(t *testing.T) {
	registry, err := nodecatalog.NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("load built-in product Node Catalog: %v", err)
	}
	entries := registry.Catalog()
	if len(entries) != 2 {
		t.Fatalf("catalog entries = %#v, want two", entries)
	}
	if entries[0].Definition.ID != "human-chat" || entries[0].Executor.Version != "v1" {
		t.Fatalf("first catalog entry = %#v", entries[0])
	}
	llm := entries[1]
	if llm.Definition.ID != "llm-chat" || llm.Definition.Kind != nodecatalog.NodeAgent || llm.Executor.Version != "v1" {
		t.Fatalf("llm catalog entry = %#v", llm)
	}
	wantFields := []struct {
		name string
		kind nodecatalog.FieldType
	}{
		{"instructions", nodecatalog.FieldMarkdown},
		{"temperature", nodecatalog.FieldNumber},
		{"max_output_tokens", nodecatalog.FieldInteger},
	}
	for i, want := range wantFields {
		field := llm.Definition.Config.Fields[i]
		if field.Name != want.name || field.Type != want.kind {
			t.Fatalf("llm field %d = %#v, want %s %s", i, field, want.name, want.kind)
		}
		if field.Presentation.Label == "" || field.Presentation.Editor == "" {
			t.Fatalf("llm field %q lacks presentation hints: %#v", field.Name, field.Presentation)
		}
	}
}

func TestRegistryRequiresDefinitionBeforeExecutor(t *testing.T) {
	registry := nodecatalog.NewRegistry()
	err := registry.RegisterExecutor(nodecatalog.Executor{DefinitionID: "llm-chat", Version: "v1"})
	if err == nil {
		t.Fatal("register orphan executor = nil error")
	}
}
