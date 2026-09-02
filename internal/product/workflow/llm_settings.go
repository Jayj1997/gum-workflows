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

// Provider instruction dialects select the OpenAI-compatible role used for
// Node instructions. Developer is the compatibility default for existing
// Providers.
const (
	ProviderDialectDeveloper = "developer"
	ProviderDialectSystem    = "system"
)

// LLMProvider is one user-managed model service connection. APIKeyRef names a
// Secret held outside SQLite; it never contains the Secret value itself.
type LLMProvider struct {
	ID               string
	Name             string
	Protocol         string
	Dialect          string
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

// WorkflowModelReference is one current Draft Node's selection of one Gum Model
// UUID. NodeDefinition lets the Application keep the reference semantics of
// agent Nodes without the store knowing the product Node Catalog; NodeID
// identifies the referencing Node Instance inside that Draft.
type WorkflowModelReference struct {
	WorkflowID     string
	NodeID         string
	NodeDefinition string
	ModelUUID      string
}

// Less orders references by Workflow, Node Instance, Definition and Model so
// both the store query and the deletion preview share one stable order.
func (r WorkflowModelReference) Less(other WorkflowModelReference) bool {
	if r.WorkflowID != other.WorkflowID {
		return r.WorkflowID < other.WorkflowID
	}
	if r.NodeID != other.NodeID {
		return r.NodeID < other.NodeID
	}
	if r.NodeDefinition != other.NodeDefinition {
		return r.NodeDefinition < other.NodeDefinition
	}
	return r.ModelUUID < other.ModelUUID
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
	ResolveLLMModel(ctx context.Context, modelID string) (ResolvedLLMModel, error)
}

// LLMUsageRepository reads which current Product Workflow Drafts select Gum
// Model UUIDs so Provider/Model deletion can preview affected Workflows.
type LLMUsageRepository interface {
	ListProductWorkflowDraftModelReferences(ctx context.Context) ([]WorkflowModelReference, error)
}
