package product

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// LLMProviderView is a user-visible Provider and its Model Slots.
type LLMProviderView struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Protocol         string         `json:"protocol"`
	BaseURL          string         `json:"baseUrl"`
	APIKeyRef        string         `json:"apiKeyRef"`
	ExplicitDefault  bool           `json:"explicitDefault"`
	EffectiveDefault bool           `json:"effectiveDefault"`
	CreatedAt        time.Time      `json:"createdAt"`
	Models           []LLMModelView `json:"models"`
}

// LLMModelView is a stable Gum Model configuration slot.
type LLMModelView struct {
	ID                 string                             `json:"id"`
	ProviderID         string                             `json:"providerId"`
	DisplayName        string                             `json:"displayName"`
	ProviderModelID    string                             `json:"providerModelId"`
	GenerationDefaults productworkflow.GenerationDefaults `json:"generationDefaults"`
	ExplicitDefault    bool                               `json:"explicitDefault"`
	EffectiveDefault   bool                               `json:"effectiveDefault"`
	CreatedAt          time.Time                          `json:"createdAt"`
}

// LLMSettingsView is the active Provider -> Models settings tree.
type LLMSettingsView struct {
	Providers   []LLMProviderView `json:"providers"`
	Diagnostics []Diagnostic      `json:"diagnostics"`
}

// ResolvedLLMModelView is the effective default selection used by later StartRun preflight.
type ResolvedLLMModelView struct {
	Provider    LLMProviderView `json:"provider"`
	Model       LLMModelView    `json:"model"`
	Diagnostics []Diagnostic    `json:"diagnostics"`
}

// CreateLLMProviderInput creates a Provider with a generated stable Gum UUID.
type CreateLLMProviderInput struct {
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	BaseURL   string `json:"baseUrl"`
	APIKeyRef string `json:"apiKeyRef"`
}

// UpdateLLMProviderInput edits connection metadata without changing Provider identity.
type UpdateLLMProviderInput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	BaseURL   string `json:"baseUrl"`
	APIKeyRef string `json:"apiKeyRef"`
}

// CreateLLMModelInput creates a Model Slot with a generated stable Gum UUID.
type CreateLLMModelInput struct {
	ProviderID         string                             `json:"providerId"`
	DisplayName        string                             `json:"displayName"`
	ProviderModelID    string                             `json:"providerModelId"`
	GenerationDefaults productworkflow.GenerationDefaults `json:"generationDefaults"`
}

// UpdateLLMModelInput edits one Model Slot without changing its identity.
type UpdateLLMModelInput struct {
	ID                 string                             `json:"id"`
	ProviderID         string                             `json:"providerId"`
	DisplayName        string                             `json:"displayName"`
	ProviderModelID    string                             `json:"providerModelId"`
	GenerationDefaults productworkflow.GenerationDefaults `json:"generationDefaults"`
}

func (a *Application) requireLLMSettings() (productworkflow.LLMSettingsRepository, error) {
	if a.llmSettings == nil {
		return nil, fmt.Errorf("llm settings repository is not configured")
	}
	return a.llmSettings, nil
}

// GetLLMSettings returns active Providers and Models plus actionable empty-state diagnostics.
func (a *Application) GetLLMSettings(ctx context.Context) (LLMSettingsView, error) {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return LLMSettingsView{}, err
	}
	settings, err := repository.GetLLMSettings(ctx)
	if err != nil {
		return LLMSettingsView{}, fmt.Errorf("get LLM settings: %w", err)
	}
	return llmSettingsView(settings), nil
}

// CreateLLMProvider creates one Provider.
func (a *Application) CreateLLMProvider(ctx context.Context, input CreateLLMProviderInput) (LLMProviderView, error) {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return LLMProviderView{}, err
	}
	provider, err := providerFromInput("", input.Name, input.Protocol, input.BaseURL, input.APIKeyRef)
	if err != nil {
		return LLMProviderView{}, fmt.Errorf("create LLM Provider: %w", err)
	}
	provider.ID = uuid.NewString()
	created, err := repository.CreateLLMProvider(ctx, provider)
	if err != nil {
		return LLMProviderView{}, fmt.Errorf("create LLM Provider: %w", err)
	}
	return llmProviderView(created, nil), nil
}

// UpdateLLMProvider edits a Provider while preserving its Gum UUID.
func (a *Application) UpdateLLMProvider(ctx context.Context, input UpdateLLMProviderInput) (LLMProviderView, error) {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return LLMProviderView{}, err
	}
	provider, err := providerFromInput(input.ID, input.Name, input.Protocol, input.BaseURL, input.APIKeyRef)
	if err != nil {
		return LLMProviderView{}, fmt.Errorf("update LLM Provider: %w", err)
	}
	updated, err := repository.UpdateLLMProvider(ctx, provider)
	if err != nil {
		return LLMProviderView{}, fmt.Errorf("update LLM Provider: %w", err)
	}
	return llmProviderView(updated, nil), nil
}

// DeleteLLMProvider soft-deletes a Provider from future default resolution.
func (a *Application) DeleteLLMProvider(ctx context.Context, providerID string) error {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return err
	}
	if err := repository.DeleteLLMProvider(ctx, strings.TrimSpace(providerID)); err != nil {
		return fmt.Errorf("delete LLM Provider: %w", err)
	}
	return nil
}

// SetDefaultLLMProvider selects the sole explicit Provider default.
func (a *Application) SetDefaultLLMProvider(ctx context.Context, providerID string) (LLMSettingsView, error) {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return LLMSettingsView{}, err
	}
	if err := repository.SetDefaultLLMProvider(ctx, strings.TrimSpace(providerID)); err != nil {
		return LLMSettingsView{}, fmt.Errorf("set default LLM Provider: %w", err)
	}
	return a.GetLLMSettings(ctx)
}

// CreateLLMModel creates one stable Model Slot below a Provider.
func (a *Application) CreateLLMModel(ctx context.Context, input CreateLLMModelInput) (LLMModelView, error) {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return LLMModelView{}, err
	}
	model, err := modelFromInput("", input.ProviderID, input.DisplayName, input.ProviderModelID, input.GenerationDefaults)
	if err != nil {
		return LLMModelView{}, fmt.Errorf("create LLM Model: %w", err)
	}
	model.ID = uuid.NewString()
	created, err := repository.CreateLLMModel(ctx, model)
	if err != nil {
		return LLMModelView{}, fmt.Errorf("create LLM Model: %w", err)
	}
	return llmModelView(created), nil
}

// UpdateLLMModel edits a Model Slot while preserving its Gum Model UUID.
func (a *Application) UpdateLLMModel(ctx context.Context, input UpdateLLMModelInput) (LLMModelView, error) {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return LLMModelView{}, err
	}
	model, err := modelFromInput(input.ID, input.ProviderID, input.DisplayName, input.ProviderModelID, input.GenerationDefaults)
	if err != nil {
		return LLMModelView{}, fmt.Errorf("update LLM Model: %w", err)
	}
	updated, err := repository.UpdateLLMModel(ctx, model)
	if err != nil {
		return LLMModelView{}, fmt.Errorf("update LLM Model: %w", err)
	}
	return llmModelView(updated), nil
}

// DeleteLLMModel soft-deletes a Model Slot from future default resolution.
func (a *Application) DeleteLLMModel(ctx context.Context, providerID, modelID string) error {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return err
	}
	if err := repository.DeleteLLMModel(ctx, strings.TrimSpace(providerID), strings.TrimSpace(modelID)); err != nil {
		return fmt.Errorf("delete LLM Model: %w", err)
	}
	return nil
}

// SetDefaultLLMModel selects the sole explicit Model default within one Provider.
func (a *Application) SetDefaultLLMModel(ctx context.Context, providerID, modelID string) (LLMSettingsView, error) {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return LLMSettingsView{}, err
	}
	if err := repository.SetDefaultLLMModel(ctx, strings.TrimSpace(providerID), strings.TrimSpace(modelID)); err != nil {
		return LLMSettingsView{}, fmt.Errorf("set default LLM Model: %w", err)
	}
	return a.GetLLMSettings(ctx)
}

// ResolveDefaultLLMModel applies the shared two-level explicit-or-created-order rule.
func (a *Application) ResolveDefaultLLMModel(ctx context.Context) (ResolvedLLMModelView, error) {
	repository, err := a.requireLLMSettings()
	if err != nil {
		return ResolvedLLMModelView{}, err
	}
	settings, err := repository.GetLLMSettings(ctx)
	if err != nil {
		return ResolvedLLMModelView{}, fmt.Errorf("resolve default LLM Model: %w", err)
	}
	settingsView := llmSettingsView(settings)
	if len(settingsView.Diagnostics) > 0 {
		return ResolvedLLMModelView{Diagnostics: settingsView.Diagnostics}, nil
	}
	resolved, err := repository.ResolveDefaultLLMModel(ctx)
	if err != nil {
		return ResolvedLLMModelView{}, fmt.Errorf("resolve default LLM Model: %w", err)
	}
	return ResolvedLLMModelView{Provider: llmProviderView(resolved.Provider, nil), Model: llmModelView(resolved.Model), Diagnostics: []Diagnostic{}}, nil
}

func providerFromInput(id, name, protocol, baseURL, apiKeyRef string) (productworkflow.LLMProvider, error) {
	provider := productworkflow.LLMProvider{ID: strings.TrimSpace(id), Name: strings.TrimSpace(name), Protocol: strings.TrimSpace(protocol), BaseURL: strings.TrimSpace(baseURL), APIKeyRef: strings.TrimSpace(apiKeyRef)}
	if provider.Name == "" {
		return provider, fmt.Errorf("name must not be empty")
	}
	if provider.Protocol == "" {
		return provider, fmt.Errorf("protocol must not be empty")
	}
	parsed, err := url.ParseRequestURI(provider.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return provider, fmt.Errorf("base URL must be an absolute URL")
	}
	if provider.APIKeyRef == "" {
		return provider, fmt.Errorf("api key reference must not be empty")
	}
	secretRef, err := url.Parse(provider.APIKeyRef)
	if err != nil || secretRef.Scheme == "" || (secretRef.Host == "" && secretRef.Opaque == "" && secretRef.Path == "") {
		return provider, fmt.Errorf("api key reference must be a Secret reference URI")
	}
	return provider, nil
}

func modelFromInput(id, providerID, displayName, providerModelID string, defaults productworkflow.GenerationDefaults) (productworkflow.LLMModel, error) {
	model := productworkflow.LLMModel{ID: strings.TrimSpace(id), ProviderID: strings.TrimSpace(providerID), DisplayName: strings.TrimSpace(displayName), ProviderModelID: strings.TrimSpace(providerModelID), GenerationDefaults: defaults}
	if model.ProviderID == "" {
		return model, fmt.Errorf("provider ID must not be empty")
	}
	if model.DisplayName == "" {
		return model, fmt.Errorf("display name must not be empty")
	}
	if model.ProviderModelID == "" {
		return model, fmt.Errorf("provider model ID must not be empty")
	}
	if defaults.Temperature != nil && (*defaults.Temperature < 0 || *defaults.Temperature > 2) {
		return model, fmt.Errorf("generation default temperature must be between 0 and 2")
	}
	if defaults.MaxOutputTokens != nil && *defaults.MaxOutputTokens < 1 {
		return model, fmt.Errorf("generation default max output tokens must be positive")
	}
	return model, nil
}

func llmSettingsView(settings productworkflow.LLMSettings) LLMSettingsView {
	view := LLMSettingsView{Providers: make([]LLMProviderView, 0, len(settings.Providers)), Diagnostics: []Diagnostic{}}
	for _, provider := range settings.Providers {
		view.Providers = append(view.Providers, llmProviderView(provider, settings.Models[provider.ID]))
	}
	if len(view.Providers) == 0 {
		view.Diagnostics = append(view.Diagnostics, Diagnostic{Code: "llm-provider-required", Severity: "error", Path: "llm.providers", Message: "create an LLM Provider before selecting a model"})
	} else {
		for _, provider := range view.Providers {
			if provider.EffectiveDefault && len(provider.Models) == 0 {
				view.Diagnostics = append(view.Diagnostics, Diagnostic{Code: "llm-model-required", Severity: "error", Path: "llm.providers." + provider.ID + ".models", Message: "create a Model Slot for the effective default Provider"})
			}
		}
	}
	return view
}

func llmProviderView(provider productworkflow.LLMProvider, models []productworkflow.LLMModel) LLMProviderView {
	view := LLMProviderView{ID: provider.ID, Name: provider.Name, Protocol: provider.Protocol, BaseURL: provider.BaseURL, APIKeyRef: provider.APIKeyRef, ExplicitDefault: provider.ExplicitDefault, EffectiveDefault: provider.EffectiveDefault, CreatedAt: provider.CreatedAt, Models: make([]LLMModelView, 0, len(models))}
	for _, model := range models {
		view.Models = append(view.Models, llmModelView(model))
	}
	return view
}

func llmModelView(model productworkflow.LLMModel) LLMModelView {
	return LLMModelView{ID: model.ID, ProviderID: model.ProviderID, DisplayName: model.DisplayName, ProviderModelID: model.ProviderModelID, GenerationDefaults: model.GenerationDefaults, ExplicitDefault: model.ExplicitDefault, EffectiveDefault: model.EffectiveDefault, CreatedAt: model.CreatedAt}
}
