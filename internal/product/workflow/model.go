// Package workflow defines the SQLite-backed Product Workflow identity model.
// It is separate from the YAML workflow/v1 definition package.
package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Workflow is the stable identity and display metadata of a Product Workflow.
type Workflow struct {
	ID          string
	DisplayName string
	CreatedAt   time.Time
}

// Draft is the single mutable semantic definition of a Product Workflow.
type Draft struct {
	WorkflowID  string
	Content     json.RawMessage
	LockVersion uint64
	UpdatedAt   time.Time
}

// DraftUpdate reports whether an autosave wrote content or observed a conflict.
type DraftUpdate struct {
	Draft    Draft
	Saved    bool
	Conflict bool
}

// InitialDraftContent returns the semantic starting point for a new Draft.
func InitialDraftContent() json.RawMessage {
	return json.RawMessage(`{"nodes":[],"semanticSchemaVersion":"productWorkflow/v1"}`)
}

// NormalizeDraftContent canonicalizes JSON so storage order and whitespace do not
// create false semantic changes.
func NormalizeDraftContent(content json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode draft content: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode draft content: multiple JSON values")
		}
		return nil, fmt.Errorf("decode draft content: %w", err)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode draft content: %w", err)
	}
	return normalized, nil
}

// Repository persists Product Workflows independently from workflow/v1 imports.
type Repository interface {
	CreateProductWorkflow(ctx context.Context, displayName string) (Workflow, error)
	ListProductWorkflows(ctx context.Context) ([]Workflow, error)
	GetProductWorkflowDraft(ctx context.Context, workflowID string) (Draft, error)
	UpdateProductWorkflowDraft(ctx context.Context, workflowID string, expectedLockVersion uint64, content json.RawMessage) (DraftUpdate, error)
}
