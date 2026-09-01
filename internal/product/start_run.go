package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/chat"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
)

// StartRunInput starts the Draft version currently visible to the UI.
type StartRunInput struct {
	WorkflowID          string `json:"workflowId"`
	ExpectedLockVersion uint64 `json:"expectedLockVersion"`
}

// ChatMessageView is one message rendered from a Conversation Artifact.
type ChatMessageView struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// ArtifactView is one user-visible Product Run output.
type ArtifactView struct {
	ID       string            `json:"id"`
	NodeID   string            `json:"nodeId"`
	Port     string            `json:"port"`
	Type     string            `json:"type"`
	Version  string            `json:"version"`
	URI      string            `json:"uri"`
	Messages []ChatMessageView `json:"messages,omitempty"`
}

// NodeRunView is one user-visible Node Run result.
type NodeRunView struct {
	ID             string         `json:"id"`
	NodeID         string         `json:"nodeId"`
	NodeDefinition string         `json:"nodeDefinition"`
	NodeExecutor   string         `json:"nodeExecutor"`
	Status         string         `json:"status"`
	Diagnostics    map[string]any `json:"diagnostics,omitempty"`
}

// ExecutorSnapshotView is one fixed Node Executor in a Product Run.
type ExecutorSnapshotView struct {
	NodeID       string `json:"nodeId"`
	DefinitionID string `json:"definitionId"`
	Version      string `json:"version"`
}

// LLMSelectionSnapshotView is one non-secret effective model selection.
type LLMSelectionSnapshotView struct {
	NodeID          string   `json:"nodeId"`
	ProviderID      string   `json:"providerId"`
	ProviderName    string   `json:"providerName"`
	Protocol        string   `json:"protocol"`
	BaseURL         string   `json:"baseUrl"`
	ModelUUID       string   `json:"modelUuid"`
	ProviderModelID string   `json:"providerModelId"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
}

// RunSnapshotView is the immutable execution selection returned to UI adapters.
type RunSnapshotView struct {
	Executors     []ExecutorSnapshotView     `json:"executors"`
	LLMSelections []LLMSelectionSnapshotView `json:"llmSelections"`
	Project       map[string]any             `json:"project,omitempty"`
}

// RunView is the completed Product Run result returned to UI adapters.
type RunView struct {
	ID         string          `json:"id"`
	RevisionID string          `json:"revisionId"`
	Status     string          `json:"status"`
	Draft      DraftView       `json:"draft"`
	Snapshot   RunSnapshotView `json:"snapshot"`
	NodeRuns   []NodeRunView   `json:"nodeRuns"`
	Artifacts  []ArtifactView  `json:"artifacts"`
}

// StartRun validates and materializes the visible Draft, executes the single
// real `human-chat(source) -> llm-chat` turn through the chat Protocol Adapter,
// and atomically publishes its persistent history.
func (a *Application) StartRun(ctx context.Context, input StartRunInput) (RunView, error) {
	if strings.TrimSpace(input.WorkflowID) == "" {
		return RunView{}, fmt.Errorf("start Run: workflow ID must not be empty")
	}
	if input.ExpectedLockVersion == 0 {
		return RunView{}, fmt.Errorf("start Run: expected lock version must be positive")
	}
	if a.runRepo == nil || a.runPaths.RunsDir() == "" {
		return RunView{}, fmt.Errorf("start Run: product Run persistence is not configured")
	}
	draft, err := a.GetDraft(ctx, input.WorkflowID)
	if err != nil {
		return RunView{}, fmt.Errorf("start Run: %w", err)
	}
	if draft.LockVersion != input.ExpectedLockVersion {
		return RunView{}, fmt.Errorf("start Run: draft lock version conflict: expected %d, current %d", input.ExpectedLockVersion, draft.LockVersion)
	}
	preview := a.previewDraft(draft.Content)
	if len(preview.Diagnostics) > 0 {
		return RunView{}, fmt.Errorf("start Run: draft has %d diagnostic(s); fix the highlighted fields", len(preview.Diagnostics))
	}
	materialized, err := cloneContent(draft.Content)
	if err != nil {
		return RunView{}, fmt.Errorf("start Run: %w", err)
	}
	selections, err := a.materializeLLMSelections(ctx, materialized)
	if err != nil {
		return RunView{}, fmt.Errorf("start Run: %w", err)
	}
	draftJSON, err := json.Marshal(materialized)
	if err != nil {
		return RunView{}, fmt.Errorf("start Run: encode materialized Draft: %w", err)
	}
	revisionContent, semanticHash, err := productworkflow.RevisionContent(draftJSON)
	if err != nil {
		return RunView{}, fmt.Errorf("start Run: %w", err)
	}

	now := time.Now().UTC()
	runID := uuid.NewString()
	revision := productworkflow.Revision{ID: uuid.NewString(), WorkflowID: input.WorkflowID, SemanticHash: semanticHash, Content: revisionContent, CreatedAt: now}
	snapshot := snapshotForDraft(revision.ID, materialized, selections)
	run := productworkflow.Run{
		ID: runID, WorkflowID: input.WorkflowID, RevisionID: revision.ID, Status: "succeeded",
		Snapshot: snapshot, StartedAt: now, FinishedAt: now,
	}
	nodeRuns, runArtifacts, artifactViews, err := a.executeSingleTurn(ctx, runID, materialized, selections, now)
	if err != nil {
		_ = os.RemoveAll(a.runPaths.RunDir(runID))
		return RunView{}, fmt.Errorf("start Run: %w", err)
	}
	result, err := a.runRepo.StartProductWorkflowRun(ctx, productworkflow.StartRunRequest{
		WorkflowID: input.WorkflowID, ExpectedLockVersion: input.ExpectedLockVersion, DraftContent: draftJSON,
		Revision: revision, Run: run, NodeRuns: nodeRuns, Artifacts: runArtifacts,
	})
	if err != nil {
		cleanupErr := os.RemoveAll(a.runPaths.RunDir(runID))
		if cleanupErr != nil {
			return RunView{}, fmt.Errorf("start Run: %w; clean unpublished Run: %v", err, cleanupErr)
		}
		return RunView{}, fmt.Errorf("start Run: %w", err)
	}
	materializedDraft, err := draftView(result.Draft)
	if err != nil {
		return RunView{}, fmt.Errorf("start Run: %w", err)
	}
	materializedPreview := a.previewDraft(materializedDraft.Content)
	materializedDraft.Preview = &materializedPreview
	views := make([]NodeRunView, 0, len(nodeRuns))
	for _, nodeRun := range nodeRuns {
		views = append(views, nodeRunView(nodeRun))
	}
	return RunView{ID: result.Run.ID, RevisionID: result.Revision.ID, Status: result.Run.Status, Draft: materializedDraft, Snapshot: runSnapshotView(result.Run.Snapshot), NodeRuns: views, Artifacts: artifactViews}, nil
}

func cloneContent(content map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("encode Draft: %w", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("decode Draft: %w", err)
	}
	return cloned, nil
}

func (a *Application) materializeLLMSelections(ctx context.Context, content map[string]any) ([]productworkflow.ResolvedLLMSelection, error) {
	if a.llmSettings == nil {
		return nil, fmt.Errorf("llm settings repository is not configured")
	}
	nodes, _ := content["nodes"].([]any)
	selections := make([]productworkflow.ResolvedLLMSelection, 0)
	for index, value := range nodes {
		node, _ := value.(map[string]any)
		definitionID, _ := node["definition"].(string)
		definition, found := a.catalog.Definition(definitionID)
		if !found || definition.Kind != nodecatalog.NodeAgent {
			continue
		}
		preference, _ := node["llm"].(map[string]any)
		if preference == nil {
			preference = map[string]any{}
			node["llm"] = preference
		}
		modelID, _ := preference["modelUuid"].(string)
		var resolved productworkflow.ResolvedLLMModel
		var err error
		if strings.TrimSpace(modelID) == "" {
			resolved, err = a.llmSettings.ResolveDefaultLLMModel(ctx)
			if err == nil {
				preference["modelUuid"] = resolved.Model.ID
			}
		} else {
			resolved, err = a.llmSettings.ResolveLLMModel(ctx, modelID)
		}
		if err != nil {
			return nil, fmt.Errorf("nodes[%d].llm.modelUuid: %w", index, err)
		}
		nodeID, _ := node["id"].(string)
		effective := resolved.Model.GenerationDefaults
		config, _ := node["config"].(map[string]any)
		if temperature, ok := config["temperature"].(float64); ok {
			effective.Temperature = &temperature
		}
		if tokens, ok := config["max_output_tokens"].(float64); ok {
			value := int(tokens)
			effective.MaxOutputTokens = &value
		}
		selections = append(selections, productworkflow.ResolvedLLMSelection{
			NodeID: nodeID, ProviderID: resolved.Provider.ID, ProviderName: resolved.Provider.Name,
			Protocol: resolved.Provider.Protocol, BaseURL: resolved.Provider.BaseURL, APIKeyRef: resolved.Provider.APIKeyRef,
			ModelUUID:       resolved.Model.ID,
			ProviderModelID: resolved.Model.ProviderModelID, EffectiveGeneration: effective,
		})
	}
	return selections, nil
}

// executeSingleTurn runs the authored `human-chat(source) -> llm-chat`
// Workflow: the human turn publishes the user Conversation Artifact, the agent
// turn makes one real OpenAI-compatible call and appends exactly one assistant
// message as the new Conversation version.
func (a *Application) executeSingleTurn(ctx context.Context, runID string, content map[string]any, selections []productworkflow.ResolvedLLMSelection, now time.Time) ([]productworkflow.NodeRun, []productworkflow.RunArtifact, []ArtifactView, error) {
	nodes, _ := content["nodes"].([]any)
	nodesByID := make(map[string]map[string]any, len(nodes))
	var agent map[string]any
	for _, value := range nodes {
		node, _ := value.(map[string]any)
		nodeID, _ := node["id"].(string)
		nodesByID[nodeID] = node
		definitionID, _ := node["definition"].(string)
		definition, _ := a.catalog.Definition(definitionID)
		if definition.Kind == nodecatalog.NodeAgent {
			if agent == nil {
				agent = node
			} else {
				return nil, nil, nil, fmt.Errorf("single-turn executor supports exactly one llm-chat Node")
			}
		}
	}
	if agent == nil {
		return nil, nil, nil, fmt.Errorf("single-turn executor requires one llm-chat Node")
	}
	inputs, _ := agent["inputs"].(map[string]any)
	conversationBinding, _ := inputs["conversation"].(map[string]any)
	from, _ := conversationBinding["from"].(string)
	sourceID, sourcePort, valid := parsePortReference(from)
	human := nodesByID[sourceID]
	if !valid || sourcePort != "conversation" || stringValue(human, "definition") != "human-chat" {
		return nil, nil, nil, fmt.Errorf("single-turn executor requires llm-chat.inputs.conversation from human-chat.conversation")
	}
	if a.chat == nil {
		return nil, nil, nil, fmt.Errorf("chat protocol adapter is not configured")
	}
	if a.secrets == nil {
		return nil, nil, nil, fmt.Errorf("secret adapter is not configured")
	}
	var selection productworkflow.ResolvedLLMSelection
	for _, candidate := range selections {
		if candidate.NodeID == stringValue(agent, "id") {
			selection = candidate
		}
	}
	if selection.ModelUUID == "" {
		return nil, nil, nil, fmt.Errorf("llm-chat Node %q has no resolved model selection", stringValue(agent, "id"))
	}
	apiKey, err := a.secrets.Resolve(ctx, selection.APIKeyRef)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve Provider API Key: %w", redactSecret(err))
	}
	store, err := artifact.NewFilesystemStore(a.runPaths.ArtifactsDir(runID))
	if err != nil {
		return nil, nil, nil, err
	}
	// The single-turn closure sends one fixed user turn; per-Run human text
	// input arrives with the Human Chat Entry upgrade.
	const userText = "Hello from the product UI."
	sourceConversation := chat.Conversation{Messages: []chat.ChatMessage{chat.UserTextMessage(userText)}}
	sourceArtifactID := uuid.NewString()
	sourceRef, err := store.Put(artifact.Artifact{ID: sourceArtifactID, Kind: artifact.KindConversation, Version: "1", Data: sourceConversation})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write user Conversation Artifact: %w", err)
	}

	agentID := stringValue(agent, "id")
	agentStarted := time.Now().UTC()
	result, err := a.chat.Generate(ctx, chat.Connection{
		Protocol: selection.Protocol, BaseURL: selection.BaseURL,
		ProviderModelID: selection.ProviderModelID, APIKey: apiKey,
	}, chat.GenerateRequest{
		Model:        selection.ProviderModelID,
		Instructions: agentInstructions(agent),
		Messages:     sourceConversation.Messages,
		Config: chat.GenerationConfig{
			Temperature:     selection.EffectiveGeneration.Temperature,
			MaxOutputTokens: selection.EffectiveGeneration.MaxOutputTokens,
		},
	})
	agentFinished := time.Now().UTC()
	if err != nil {
		// The provider call failed: this is a Structural Error, the Run is not
		// created and nothing user-visible is persisted.
		return nil, nil, nil, fmt.Errorf("llm-chat Node %q model call: %w", agentID, redactSecret(err))
	}
	finalConversation := chat.Conversation{Messages: append(append([]chat.ChatMessage{}, sourceConversation.Messages...), result.Assistant)}
	finalArtifactID := uuid.NewString()
	finalRef, err := store.Put(artifact.Artifact{ID: finalArtifactID, Kind: artifact.KindConversation, Version: "2", Data: finalConversation})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write assistant Conversation Artifact: %w", err)
	}
	humanRunID := uuid.NewString()
	agentRunID := uuid.NewString()
	usage := chat.Usage(result.Usage)
	nodeRuns := []productworkflow.NodeRun{
		{ID: humanRunID, RunID: runID, NodeID: sourceID, NodeDefinition: stringValue(human, "definition"), NodeExecutor: stringValue(human, "executor"), Status: "succeeded", Inputs: map[string]artifact.ArtifactRef{}, Outputs: map[string]artifact.ArtifactRef{"conversation": sourceRef}, StartedAt: now, FinishedAt: now},
		{ID: agentRunID, RunID: runID, NodeID: agentID, NodeDefinition: stringValue(agent, "definition"), NodeExecutor: stringValue(agent, "executor"), Status: "succeeded", Inputs: map[string]artifact.ArtifactRef{"conversation": sourceRef}, Outputs: map[string]artifact.ArtifactRef{"conversation": finalRef}, Diagnostics: productworkflow.NodeRunDiagnostics{ProviderRequestID: result.ProviderRequestID, FinishReason: result.FinishReason, Usage: &usage}, StartedAt: agentStarted, FinishedAt: agentFinished},
	}
	items := []productworkflow.RunArtifact{
		{ID: sourceArtifactID, RunID: runID, NodeRunID: humanRunID, NodeID: sourceID, Port: "conversation", Type: "Conversation", Version: "1", URI: sourceRef.URI, CreatedAt: now},
		{ID: finalArtifactID, RunID: runID, NodeRunID: agentRunID, NodeID: agentID, Port: "conversation", Type: "Conversation", Version: "2", URI: finalRef.URI, CreatedAt: agentFinished},
	}
	views := []ArtifactView{
		artifactView(items[0], sourceConversation), artifactView(items[1], finalConversation),
	}
	return nodeRuns, items, views, nil
}

// agentInstructions returns the Node config instructions text as canonical
// instruction parts. System guidance stays out of the Conversation.
func agentInstructions(agent map[string]any) []chat.ContentPart {
	config, _ := agent["config"].(map[string]any)
	instructions, _ := config["instructions"].(string)
	if strings.TrimSpace(instructions) == "" {
		return nil
	}
	return []chat.ContentPart{chat.TextPart(instructions)}
}

// redactSecret removes resolved secret values from an error. Typed protocol
// errors keep their identity (so errors.As keeps working); other errors are
// flattened only when the pattern actually matched something to redact.
func redactSecret(err error) error {
	if err == nil {
		return nil
	}
	var openAIError *chat.OpenAIError
	if errors.As(err, &openAIError) {
		if message := secretPattern.ReplaceAllString(openAIError.ProviderMessage, "[redacted]"); message != openAIError.ProviderMessage {
			return &chat.OpenAIError{Kind: openAIError.Kind, StatusCode: openAIError.StatusCode, ProviderMessage: message, Err: openAIError.Err}
		}
		return err
	}
	if message := secretPattern.ReplaceAllString(err.Error(), "[redacted]"); message != err.Error() {
		return fmt.Errorf("%s", message)
	}
	return err
}

// secretPattern matches common bearer-token shapes in error text.
var secretPattern = regexp.MustCompile(`sk-[A-Za-z0-9_\-]{4,}`)

func snapshotForDraft(revisionID string, content map[string]any, selections []productworkflow.ResolvedLLMSelection) productworkflow.RunSnapshot {
	snapshot := productworkflow.RunSnapshot{RevisionID: revisionID, Executors: []productworkflow.ResolvedExecutor{}, LLMSelection: selections}
	if project, ok := content["project"].(map[string]any); ok {
		snapshot.Project = project
	}
	if nodes, ok := content["nodes"].([]any); ok {
		for _, value := range nodes {
			node, _ := value.(map[string]any)
			snapshot.Executors = append(snapshot.Executors, productworkflow.ResolvedExecutor{NodeID: stringValue(node, "id"), DefinitionID: stringValue(node, "definition"), Version: stringValue(node, "executor")})
		}
	}
	return snapshot
}

func runSnapshotView(snapshot productworkflow.RunSnapshot) RunSnapshotView {
	view := RunSnapshotView{Executors: make([]ExecutorSnapshotView, 0, len(snapshot.Executors)), LLMSelections: make([]LLMSelectionSnapshotView, 0, len(snapshot.LLMSelection)), Project: snapshot.Project}
	for _, executor := range snapshot.Executors {
		view.Executors = append(view.Executors, ExecutorSnapshotView{NodeID: executor.NodeID, DefinitionID: executor.DefinitionID, Version: executor.Version})
	}
	for _, selection := range snapshot.LLMSelection {
		view.LLMSelections = append(view.LLMSelections, LLMSelectionSnapshotView{
			NodeID: selection.NodeID, ProviderID: selection.ProviderID, ProviderName: selection.ProviderName,
			Protocol: selection.Protocol, BaseURL: selection.BaseURL, ModelUUID: selection.ModelUUID, ProviderModelID: selection.ProviderModelID,
			Temperature: selection.EffectiveGeneration.Temperature, MaxOutputTokens: selection.EffectiveGeneration.MaxOutputTokens,
		})
	}
	return view
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

// nodeRunView maps one persisted Node Run to its user-visible projection,
// including sanitized call diagnostics for real model calls.
func nodeRunView(nodeRun productworkflow.NodeRun) NodeRunView {
	view := NodeRunView{
		ID: nodeRun.ID, NodeID: nodeRun.NodeID, NodeDefinition: nodeRun.NodeDefinition,
		NodeExecutor: nodeRun.NodeExecutor, Status: nodeRun.Status,
	}
	if nodeRun.Diagnostics.ProviderRequestID == "" && nodeRun.Diagnostics.FinishReason == "" && nodeRun.Diagnostics.Usage == nil {
		return view
	}
	diagnostics := map[string]any{}
	if nodeRun.Diagnostics.ProviderRequestID != "" {
		diagnostics["providerRequestId"] = nodeRun.Diagnostics.ProviderRequestID
	}
	if nodeRun.Diagnostics.FinishReason != "" {
		diagnostics["finishReason"] = nodeRun.Diagnostics.FinishReason
	}
	if nodeRun.Diagnostics.Usage != nil {
		diagnostics["usage"] = nodeRun.Diagnostics.Usage
	}
	view.Diagnostics = diagnostics
	return view
}

func artifactView(item productworkflow.RunArtifact, conversation chat.Conversation) ArtifactView {
	messages := make([]ChatMessageView, 0, len(conversation.Messages))
	for _, message := range conversation.Messages {
		messages = append(messages, ChatMessageView{Role: message.Role, Text: message.Text()})
	}
	return ArtifactView{ID: item.ID, NodeID: item.NodeID, Port: item.Port, Type: item.Type, Version: item.Version, URI: item.URI, Messages: messages}
}
