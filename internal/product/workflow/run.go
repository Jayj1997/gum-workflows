package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/chat"
)

// ChatMessage is one user-visible message in a Conversation Artifact. It is the
// canonical chat model; the product workflow package re-exports it so run
// persistence keeps one conversation identity across views and history.
type ChatMessage = chat.ChatMessage

// Conversation is the canonical body persisted by chat Nodes.
type Conversation = chat.Conversation

// NodeRunDiagnostics carries sanitized per-call observations into Node Run
// history. It must never contain API keys, headers or raw wire bodies.
type NodeRunDiagnostics struct {
	// ProviderRequestID, FinishReason and Usage record the completed real
	// model call for this Node Run.
	ProviderRequestID string          `json:"providerRequestId,omitempty"`
	FinishReason      string          `json:"finishReason,omitempty"`
	Usage             *Usage          `json:"usage,omitempty"`
	Error             *ExecutionError `json:"error,omitempty"`
}

// ExecutionError is the sanitized, persisted explanation for one failed or
// unknown Product execution outcome.
type ExecutionError struct {
	Kind       string `json:"kind"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	UserAction string `json:"userAction"`
}

// Usage re-exports the canonical token accounting for persistence.
type Usage = chat.Usage

// ResolvedLLMSelection freezes the model connection facts used by a Run. The
// API Key itself never appears: APIKeyRef names the Secret resolved through
// the Secret Adapter at call time, and no other field carries secrets.
type ResolvedLLMSelection struct {
	NodeID              string             `json:"nodeId"`
	ProviderID          string             `json:"providerId"`
	ProviderName        string             `json:"providerName"`
	Protocol            string             `json:"protocol"`
	Dialect             string             `json:"dialect"`
	BaseURL             string             `json:"baseUrl"`
	APIKeyRef           string             `json:"apiKeyRef"`
	ModelUUID           string             `json:"modelUuid"`
	ProviderModelID     string             `json:"providerModelId"`
	EffectiveGeneration GenerationDefaults `json:"effectiveGeneration"`
}

// ResolvedExecutor freezes the Definition and Executor version for one Node.
type ResolvedExecutor struct {
	NodeID       string `json:"nodeId"`
	DefinitionID string `json:"definitionId"`
	Version      string `json:"version"`
}

// RunSnapshot freezes one Revision and all resolved model selections.
type RunSnapshot struct {
	RevisionID   string                 `json:"revisionId"`
	Executors    []ResolvedExecutor     `json:"executors"`
	LLMSelection []ResolvedLLMSelection `json:"llmSelection"`
	Project      map[string]any         `json:"project,omitempty"`
}

// Revision is one immutable, normalized Product Workflow definition.
type Revision struct {
	ID           string
	WorkflowID   string
	SemanticHash string
	Content      json.RawMessage
	CreatedAt    time.Time
}

// Run is one independent execution of a Product Workflow Revision.
type Run struct {
	ID         string
	WorkflowID string
	RevisionID string
	Status     string
	Error      *ExecutionError
	Snapshot   RunSnapshot
	StartedAt  time.Time
	FinishedAt time.Time
}

// NodeRun is one persisted Node invocation of a Product Run.
type NodeRun struct {
	ID             string
	RunID          string
	NodeID         string
	NodeDefinition string
	NodeExecutor   string
	Status         string
	Inputs         map[string]artifact.ArtifactRef
	Outputs        map[string]artifact.ArtifactRef
	Diagnostics    NodeRunDiagnostics
	StartedAt      time.Time
	FinishedAt     time.Time
}

// RunArtifact indexes one filesystem-backed Artifact produced by a Product Run.
type RunArtifact struct {
	ID        string
	RunID     string
	NodeRunID string
	NodeID    string
	Port      string
	Type      string
	Version   string
	URI       string
	CreatedAt time.Time
}

// StartRunRequest is the complete atomic persistence request prepared by the Application.
type StartRunRequest struct {
	WorkflowID          string
	ExpectedLockVersion uint64
	DraftContent        json.RawMessage
	Revision            Revision
	Run                 Run
	NodeRuns            []NodeRun
	Artifacts           []RunArtifact
}

// StartRunResult returns the materialized Draft and persisted execution identities.
type StartRunResult struct {
	Draft    Draft
	Revision Revision
	Run      Run
}

// FinishRunRequest atomically publishes the terminal state and all completed
// Node Runs/Artifacts for an already persisted running Product Run.
type FinishRunRequest struct {
	Run       Run
	NodeRuns  []NodeRun
	Artifacts []RunArtifact
}

// RunRepository materializes a visible Draft into one running Product Run,
// then atomically publishes its terminal execution details.
type RunRepository interface {
	BeginProductWorkflowRun(ctx context.Context, request StartRunRequest) (StartRunResult, error)
	RecordProductWorkflowRunProgress(ctx context.Context, runID string, nodeRuns []NodeRun, artifacts []RunArtifact) error
	FinishProductWorkflowRun(ctx context.Context, request FinishRunRequest) error
}

// RunRecoveryRepository reconciles in-flight Product Runs when a new local
// Application process opens the workspace.
type RunRecoveryRepository interface {
	InterruptProductWorkflowRuns(ctx context.Context, interruptedAt time.Time) error
}

// RevisionContent returns canonical execution semantics and its SHA-256 identity.
func RevisionContent(content json.RawMessage) (json.RawMessage, string, error) {
	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, "", fmt.Errorf("decode revision content: %w", err)
	}
	delete(root, "displayName")
	delete(root, "description")
	delete(root, "view")
	if nodes, ok := root["nodes"].([]any); ok {
		for _, value := range nodes {
			node, ok := value.(map[string]any)
			if !ok {
				continue
			}
			delete(node, "displayName")
			delete(node, "description")
			delete(node, "presentation")
			if dependencies, ok := node["dependsOn"].([]any); ok {
				sort.Slice(dependencies, func(i, j int) bool {
					left, _ := dependencies[i].(string)
					right, _ := dependencies[j].(string)
					return left < right
				})
			}
		}
		sort.Slice(nodes, func(i, j int) bool {
			leftNode, _ := nodes[i].(map[string]any)
			rightNode, _ := nodes[j].(map[string]any)
			left, _ := leftNode["id"].(string)
			right, _ := rightNode["id"].(string)
			return left < right
		})
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return nil, "", fmt.Errorf("encode revision content: %w", err)
	}
	normalized, err := NormalizeDraftContent(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("normalize revision content: %w", err)
	}
	sum := sha256.Sum256(normalized)
	return normalized, hex.EncodeToString(sum[:]), nil
}
