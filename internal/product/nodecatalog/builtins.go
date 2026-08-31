package nodecatalog

// NewBuiltinRegistry returns the explicitly registered first product Catalog.
func NewBuiltinRegistry() (*Registry, error) {
	registry := NewRegistry()
	zero, two := 0.0, 2.0
	one, maxTokens := 1.0, 128000.0
	definitions := []Definition{
		{
			ID: "human-chat", DisplayName: "Human chat", Description: "Collect a human message for a Conversation", Kind: NodeHuman,
			Config: ConfigSchema{Fields: []ConfigField{}},
		},
		{
			ID: "llm-chat", DisplayName: "LLM chat", Description: "Append one model response to a Conversation", Kind: NodeAgent,
			Config: ConfigSchema{Fields: []ConfigField{
				{
					Name: "instructions", Type: FieldMarkdown,
					Presentation: PresentationHint{Label: "Instructions", Help: "Guidance sent separately from the Conversation", Editor: "markdown"},
				},
				{
					Name: "temperature", Type: FieldNumber, Min: &zero, Max: &two,
					Presentation: PresentationHint{Label: "Temperature", Help: "Sampling temperature from 0 to 2", Editor: "number"},
				},
				{
					Name: "max_output_tokens", Type: FieldInteger, Min: &one, Max: &maxTokens,
					Presentation: PresentationHint{Label: "Max output tokens", Help: "Maximum number of generated tokens", Editor: "number"},
				},
			}},
		},
	}
	for _, definition := range definitions {
		if err := registry.RegisterDefinition(definition); err != nil {
			return nil, err
		}
		if err := registry.RegisterExecutor(Executor{DefinitionID: definition.ID, Version: "v1"}); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
