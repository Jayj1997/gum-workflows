package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// CreateLLMProvider persists one active Provider. APIKeyRef is only an opaque Secret reference.
func (s *Store) CreateLLMProvider(ctx context.Context, provider productworkflow.LLMProvider) (productworkflow.LLMProvider, error) {
	provider.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO product_llm_provider (id, name, protocol, base_url, api_key_ref, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, provider.ID, provider.Name, provider.Protocol, provider.BaseURL, provider.APIKeyRef, provider.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return productworkflow.LLMProvider{}, fmt.Errorf("create LLM Provider: %w", err)
	}
	return s.getLLMProvider(ctx, provider.ID)
}

// UpdateLLMProvider changes mutable Provider metadata without changing identity or creation order.
func (s *Store) UpdateLLMProvider(ctx context.Context, provider productworkflow.LLMProvider) (productworkflow.LLMProvider, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE product_llm_provider SET name = ?, protocol = ?, base_url = ?, api_key_ref = ?
WHERE id = ? AND deleted_at IS NULL`, provider.Name, provider.Protocol, provider.BaseURL, provider.APIKeyRef, provider.ID)
	if err != nil {
		return productworkflow.LLMProvider{}, fmt.Errorf("update LLM Provider %s: %w", provider.ID, err)
	}
	if err := requireOneRow(result, "llm provider", provider.ID); err != nil {
		return productworkflow.LLMProvider{}, err
	}
	return s.getLLMProvider(ctx, provider.ID)
}

// DeleteLLMProvider soft-deletes a Provider and excludes all its Models from active settings.
func (s *Store) DeleteLLMProvider(ctx context.Context, providerID string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE product_llm_provider SET deleted_at = ?, is_explicit_default = 0
WHERE id = ? AND deleted_at IS NULL`, time.Now().UTC().Format(time.RFC3339Nano), providerID)
	if err != nil {
		return fmt.Errorf("delete LLM Provider %s: %w", providerID, err)
	}
	return requireOneRow(result, "llm provider", providerID)
}

// SetDefaultLLMProvider atomically replaces the explicit Provider default.
func (s *Store) SetDefaultLLMProvider(ctx context.Context, providerID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set default LLM Provider: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE product_llm_provider SET is_explicit_default = 0 WHERE is_explicit_default = 1 AND deleted_at IS NULL`); err != nil {
		return fmt.Errorf("clear default LLM Provider: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE product_llm_provider SET is_explicit_default = 1 WHERE id = ? AND deleted_at IS NULL`, providerID)
	if err != nil {
		return fmt.Errorf("set default LLM Provider %s: %w", providerID, err)
	}
	if err := requireOneRow(result, "llm provider", providerID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit default LLM Provider: %w", err)
	}
	return nil
}

// CreateLLMModel persists one active Model Slot below an active Provider.
func (s *Store) CreateLLMModel(ctx context.Context, model productworkflow.LLMModel) (productworkflow.LLMModel, error) {
	model.CreatedAt = time.Now().UTC()
	defaultsJSON, err := json.Marshal(model.GenerationDefaults)
	if err != nil {
		return productworkflow.LLMModel{}, fmt.Errorf("encode LLM Model generation defaults: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO product_llm_model (id, provider_id, display_name, provider_model_id, generation_defaults_json, created_at)
SELECT ?, id, ?, ?, ?, ? FROM product_llm_provider WHERE id = ? AND deleted_at IS NULL`,
		model.ID, model.DisplayName, model.ProviderModelID, string(defaultsJSON), model.CreatedAt.Format(time.RFC3339Nano), model.ProviderID)
	if err != nil {
		return productworkflow.LLMModel{}, fmt.Errorf("create LLM Model: %w", err)
	}
	if err := requireOneRow(result, "llm provider", model.ProviderID); err != nil {
		return productworkflow.LLMModel{}, err
	}
	return s.getLLMModel(ctx, model.ProviderID, model.ID)
}

// UpdateLLMModel changes mutable Model Slot metadata without changing its Gum UUID.
func (s *Store) UpdateLLMModel(ctx context.Context, model productworkflow.LLMModel) (productworkflow.LLMModel, error) {
	defaultsJSON, err := json.Marshal(model.GenerationDefaults)
	if err != nil {
		return productworkflow.LLMModel{}, fmt.Errorf("encode LLM Model generation defaults: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE product_llm_model SET display_name = ?, provider_model_id = ?, generation_defaults_json = ?
WHERE id = ? AND provider_id = ? AND deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM product_llm_provider WHERE id = ? AND deleted_at IS NULL)`,
		model.DisplayName, model.ProviderModelID, string(defaultsJSON), model.ID, model.ProviderID, model.ProviderID)
	if err != nil {
		return productworkflow.LLMModel{}, fmt.Errorf("update LLM Model %s: %w", model.ID, err)
	}
	if err := requireOneRow(result, "llm model", model.ID); err != nil {
		return productworkflow.LLMModel{}, err
	}
	return s.getLLMModel(ctx, model.ProviderID, model.ID)
}

// DeleteLLMModel soft-deletes a Model Slot from active settings.
func (s *Store) DeleteLLMModel(ctx context.Context, providerID, modelID string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE product_llm_model SET deleted_at = ?, is_explicit_default = 0
WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`, time.Now().UTC().Format(time.RFC3339Nano), modelID, providerID)
	if err != nil {
		return fmt.Errorf("delete LLM Model %s: %w", modelID, err)
	}
	return requireOneRow(result, "llm model", modelID)
}

// SetDefaultLLMModel atomically replaces the explicit Model default within one Provider.
func (s *Store) SetDefaultLLMModel(ctx context.Context, providerID, modelID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set default LLM Model: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE product_llm_model SET is_explicit_default = 0 WHERE provider_id = ? AND is_explicit_default = 1 AND deleted_at IS NULL`, providerID); err != nil {
		return fmt.Errorf("clear default LLM Model: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE product_llm_model SET is_explicit_default = 1
WHERE id = ? AND provider_id = ? AND deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM product_llm_provider WHERE id = ? AND deleted_at IS NULL)`, modelID, providerID, providerID)
	if err != nil {
		return fmt.Errorf("set default LLM Model %s: %w", modelID, err)
	}
	if err := requireOneRow(result, "llm model", modelID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit default LLM Model: %w", err)
	}
	return nil
}

// GetLLMSettings returns active settings and marks both effective defaults.
func (s *Store) GetLLMSettings(ctx context.Context) (productworkflow.LLMSettings, error) {
	settings := productworkflow.LLMSettings{Providers: []productworkflow.LLMProvider{}, Models: map[string][]productworkflow.LLMModel{}}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, protocol, base_url, api_key_ref, is_explicit_default, created_at,
       CASE WHEN id = COALESCE(
         (SELECT id FROM product_llm_provider WHERE deleted_at IS NULL AND is_explicit_default = 1 LIMIT 1),
         (SELECT id FROM product_llm_provider WHERE deleted_at IS NULL ORDER BY created_at ASC, id ASC LIMIT 1)
       ) THEN 1 ELSE 0 END
FROM product_llm_provider WHERE deleted_at IS NULL ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return settings, fmt.Errorf("list LLM Providers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		provider, err := scanLLMProvider(rows)
		if err != nil {
			return settings, err
		}
		settings.Providers = append(settings.Providers, provider)
	}
	if err := rows.Err(); err != nil {
		return settings, fmt.Errorf("iterate LLM Providers: %w", err)
	}
	for _, provider := range settings.Providers {
		models, err := s.listLLMModels(ctx, provider.ID)
		if err != nil {
			return settings, err
		}
		settings.Models[provider.ID] = models
	}
	return settings, nil
}

// ResolveDefaultLLMModel resolves Provider then Model using explicit default or stable creation order.
func (s *Store) ResolveDefaultLLMModel(ctx context.Context) (productworkflow.ResolvedLLMModel, error) {
	settings, err := s.GetLLMSettings(ctx)
	if err != nil {
		return productworkflow.ResolvedLLMModel{}, err
	}
	var resolved productworkflow.ResolvedLLMModel
	for _, provider := range settings.Providers {
		if provider.EffectiveDefault {
			resolved.Provider = provider
			break
		}
	}
	if resolved.Provider.ID == "" {
		return resolved, fmt.Errorf("no LLM Provider is configured; create a Provider in settings")
	}
	for _, model := range settings.Models[resolved.Provider.ID] {
		if model.EffectiveDefault {
			resolved.Model = model
			break
		}
	}
	if resolved.Model.ID == "" {
		return resolved, fmt.Errorf("llm provider %q has no model slot; create a model in settings", resolved.Provider.Name)
	}
	return resolved, nil
}

// ResolveLLMModel resolves one active Gum Model UUID and its active Provider.
func (s *Store) ResolveLLMModel(ctx context.Context, modelID string) (productworkflow.ResolvedLLMModel, error) {
	var providerID string
	if err := s.db.QueryRowContext(ctx, `
SELECT provider_id FROM product_llm_model WHERE id = ? AND deleted_at IS NULL`, modelID).Scan(&providerID); err != nil {
		if err == sql.ErrNoRows {
			return productworkflow.ResolvedLLMModel{}, fmt.Errorf("llm model %s: not found", modelID)
		}
		return productworkflow.ResolvedLLMModel{}, fmt.Errorf("resolve LLM Model %s: %w", modelID, err)
	}
	provider, err := s.getLLMProvider(ctx, providerID)
	if err != nil {
		return productworkflow.ResolvedLLMModel{}, err
	}
	model, err := s.getLLMModel(ctx, providerID, modelID)
	if err != nil {
		return productworkflow.ResolvedLLMModel{}, err
	}
	return productworkflow.ResolvedLLMModel{Provider: provider, Model: model}, nil
}

func (s *Store) getLLMProvider(ctx context.Context, providerID string) (productworkflow.LLMProvider, error) {
	provider, err := scanLLMProvider(s.db.QueryRowContext(ctx, `
SELECT id, name, protocol, base_url, api_key_ref, is_explicit_default, created_at,
       CASE WHEN id = COALESCE(
         (SELECT id FROM product_llm_provider WHERE deleted_at IS NULL AND is_explicit_default = 1 LIMIT 1),
         (SELECT id FROM product_llm_provider WHERE deleted_at IS NULL ORDER BY created_at ASC, id ASC LIMIT 1)
       ) THEN 1 ELSE 0 END
FROM product_llm_provider WHERE id = ? AND deleted_at IS NULL`, providerID))
	if err == sql.ErrNoRows {
		return provider, fmt.Errorf("llm provider %s: not found", providerID)
	}
	return provider, err
}

func (s *Store) getLLMModel(ctx context.Context, providerID, modelID string) (productworkflow.LLMModel, error) {
	model, err := scanLLMModel(s.db.QueryRowContext(ctx, `
SELECT id, provider_id, display_name, provider_model_id, generation_defaults_json, is_explicit_default, created_at,
       CASE WHEN id = COALESCE(
         (SELECT id FROM product_llm_model WHERE provider_id = ? AND deleted_at IS NULL AND is_explicit_default = 1 LIMIT 1),
         (SELECT id FROM product_llm_model WHERE provider_id = ? AND deleted_at IS NULL ORDER BY created_at ASC, id ASC LIMIT 1)
       ) THEN 1 ELSE 0 END
FROM product_llm_model WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`, providerID, providerID, modelID, providerID))
	if err == sql.ErrNoRows {
		return model, fmt.Errorf("llm model %s: not found", modelID)
	}
	return model, err
}

func (s *Store) listLLMModels(ctx context.Context, providerID string) ([]productworkflow.LLMModel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, provider_id, display_name, provider_model_id, generation_defaults_json, is_explicit_default, created_at,
       CASE WHEN id = COALESCE(
         (SELECT id FROM product_llm_model WHERE provider_id = ? AND deleted_at IS NULL AND is_explicit_default = 1 LIMIT 1),
         (SELECT id FROM product_llm_model WHERE provider_id = ? AND deleted_at IS NULL ORDER BY created_at ASC, id ASC LIMIT 1)
       ) THEN 1 ELSE 0 END
FROM product_llm_model WHERE provider_id = ? AND deleted_at IS NULL ORDER BY created_at ASC, id ASC`, providerID, providerID, providerID)
	if err != nil {
		return nil, fmt.Errorf("list LLM Models for Provider %s: %w", providerID, err)
	}
	defer rows.Close()
	models := []productworkflow.LLMModel{}
	for rows.Next() {
		model, err := scanLLMModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate LLM Models for Provider %s: %w", providerID, err)
	}
	return models, nil
}

func scanLLMProvider(row rowScanner) (productworkflow.LLMProvider, error) {
	var provider productworkflow.LLMProvider
	var explicit, effective int
	var createdAt string
	if err := row.Scan(&provider.ID, &provider.Name, &provider.Protocol, &provider.BaseURL, &provider.APIKeyRef, &explicit, &createdAt, &effective); err != nil {
		return provider, err
	}
	provider.ExplicitDefault = explicit == 1
	provider.EffectiveDefault = effective == 1
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return provider, fmt.Errorf("parse LLM Provider %s created_at: %w", provider.ID, err)
	}
	provider.CreatedAt = parsed
	return provider, nil
}

func scanLLMModel(row rowScanner) (productworkflow.LLMModel, error) {
	var model productworkflow.LLMModel
	var explicit, effective int
	var createdAt string
	var defaultsJSON string
	if err := row.Scan(&model.ID, &model.ProviderID, &model.DisplayName, &model.ProviderModelID, &defaultsJSON, &explicit, &createdAt, &effective); err != nil {
		return model, err
	}
	if err := json.Unmarshal([]byte(defaultsJSON), &model.GenerationDefaults); err != nil {
		return model, fmt.Errorf("decode LLM Model %s generation defaults: %w", model.ID, err)
	}
	model.ExplicitDefault = explicit == 1
	model.EffectiveDefault = effective == 1
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return model, fmt.Errorf("parse LLM Model %s created_at: %w", model.ID, err)
	}
	model.CreatedAt = parsed
	return model, nil
}

func requireOneRow(result sql.Result, kind, id string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read %s %s rows: %w", kind, id, err)
	}
	if rows != 1 {
		return fmt.Errorf("%s %s: not found", kind, id)
	}
	return nil
}
