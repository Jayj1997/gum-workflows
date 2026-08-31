package workflow

import (
	"context"
	"time"
)

// GenerationDefaults are optional model-level request defaults overridden by Node config.
type GenerationDefaults struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
}

// LLMProvider is one user-managed model service connection. APIKeyRef names a
// Secret held outside SQLite; it never contains the Secret value itself.
type LLMProvider struct {
	ID               string
	Name             string
	Protocol         string
	BaseURL          string
	APIKeyRef        string
	ExplicitDefault  bool
	EffectiveDefault bool
	CreatedAt        time.Time
}

// LLMModel is a stable Gum Model configuration slot below one Provider.
type LLMModel struct {
	ID                 string
	ProviderID         string
	DisplayName        string
	ProviderModelID    string
	GenerationDefaults GenerationDefaults
	ExplicitDefault    bool
	EffectiveDefault   bool
	CreatedAt          time.Time
}

// LLMSettings is the active Provider -> Models tree in stable creation order.
type LLMSettings struct {
	Providers []LLMProvider
	Models    map[string][]LLMModel
}

// ResolvedLLMModel is the effective default Provider and Model Slot.
type ResolvedLLMModel struct {
	Provider LLMProvider
	Model    LLMModel
}

// LLMSettingsRepository persists and resolves user-level product LLM settings.
type LLMSettingsRepository interface {
	CreateLLMProvider(ctx context.Context, provider LLMProvider) (LLMProvider, error)
	UpdateLLMProvider(ctx context.Context, provider LLMProvider) (LLMProvider, error)
	DeleteLLMProvider(ctx context.Context, providerID string) error
	SetDefaultLLMProvider(ctx context.Context, providerID string) error
	CreateLLMModel(ctx context.Context, model LLMModel) (LLMModel, error)
	UpdateLLMModel(ctx context.Context, model LLMModel) (LLMModel, error)
	DeleteLLMModel(ctx context.Context, providerID, modelID string) error
	SetDefaultLLMModel(ctx context.Context, providerID, modelID string) error
	GetLLMSettings(ctx context.Context) (LLMSettings, error)
	ResolveDefaultLLMModel(ctx context.Context) (ResolvedLLMModel, error)
}
