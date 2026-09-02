package history

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// ListProductWorkflowDraftModelReferences returns every current Draft's
// selections of Gum Model UUIDs, one entry per (workflow, node, model). The
// store stays agnostic of the product Node Catalog: the Application decides
// which Definitions carry an LLM preference. Decoding happens in Go because
// the embedded SQLite build is not required to provide the JSON1 extension.
func (s *Store) ListProductWorkflowDraftModelReferences(ctx context.Context) ([]productworkflow.WorkflowModelReference, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT d.workflow_id, d.content_json
FROM product_workflow_draft d
ORDER BY d.workflow_id`)
	if err != nil {
		return nil, fmt.Errorf("list product workflow draft model references: %w", err)
	}
	defer rows.Close()
	type draftNode struct {
		ID         string         `json:"id"`
		Definition string         `json:"definition"`
		LLM        map[string]any `json:"llm"`
	}
	type draftContent struct {
		Nodes []draftNode `json:"nodes"`
	}
	references := make([]productworkflow.WorkflowModelReference, 0)
	for rows.Next() {
		var workflowID, contentJSON string
		if err := rows.Scan(&workflowID, &contentJSON); err != nil {
			return nil, fmt.Errorf("scan product workflow draft %s: %w", workflowID, err)
		}
		var content draftContent
		if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
			// Drafts may be semantically invalid; one unparsable row must not
			// hide the references of other Workflows.
			continue
		}
		for _, node := range content.Nodes {
			if node.LLM == nil {
				continue
			}
			modelUUID, _ := node.LLM["modelUuid"].(string)
			if modelUUID == "" {
				continue
			}
			references = append(references, productworkflow.WorkflowModelReference{
				WorkflowID: workflowID, NodeID: node.ID, NodeDefinition: node.Definition, ModelUUID: modelUUID,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product workflow draft model references: %w", err)
	}
	sort.Slice(references, func(i, j int) bool { return references[i].Less(references[j]) })
	return references, nil
}
