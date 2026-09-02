package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	WorkflowID          string        `json:"workflowId"`
	ExpectedLockVersion uint64        `json:"expectedLockVersion"`
	HumanInput          HumanRunInput `json:"humanInput"`
}

// HumanRunInput is the one submitted user turn consumed by the authored
// human-chat source Node in the P10 single-turn execution.
type HumanRunInput struct {
	NodeID string `json:"nodeId"`
	Text   string `json:"text"`
}

// RunExecutionError reports a failed or interrupted execution that has already
// been persisted and can be inspected by RunID.
type RunExecutionError struct {
	RunID   string
	Details *ExecutionErrorView
	Err     error
}

func (e *RunExecutionError) Error() string {
	if e.Details == nil {
		return fmt.Sprintf("run %s failed: %v", e.RunID, e.Err)
	}
	return fmt.Sprintf("run %s %s/%s: %s; %s", e.RunID, e.Details.Kind, e.Details.Code, e.Details.Message, e.Details.UserAction)
}

func (e *RunExecutionError) Unwrap() error { return e.Err }

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
	ID             string                          `json:"id"`
	NodeID         string                          `json:"nodeId"`
	NodeDefinition string                          `json:"nodeDefinition"`
	NodeExecutor   string                          `json:"nodeExecutor"`
	Status         string                          `json:"status"`
	Inputs         map[string]artifact.ArtifactRef `json:"inputs"`
	Outputs        map[string]artifact.ArtifactRef `json:"outputs"`
	Diagnostics    *NodeRunDiagnosticsView         `json:"diagnostics,omitempty"`
	StartedAt      time.Time                       `json:"startedAt"`
	FinishedAt     time.Time                       `json:"finishedAt"`
}

// NodeRunDiagnosticsView is the sanitized, typed telemetry exposed through
// Browser and Desktop WorkflowClient contracts.
type NodeRunDiagnosticsView struct {
	ProviderRequestID string              `json:"providerRequestId,omitempty"`
	FinishReason      string              `json:"finishReason,omitempty"`
	Usage             *chat.Usage         `json:"usage,omitempty"`
	Error             *ExecutionErrorView `json:"error,omitempty"`
}

// ExecutionErrorView is the safe error contract shared by current and
// historical Product Run views.
type ExecutionErrorView = productworkflow.ExecutionError

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
	Dialect         string   `json:"dialect"`
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
	ID         string              `json:"id"`
	RevisionID string              `json:"revisionId"`
	Status     string              `json:"status"`
	Error      *ExecutionErrorView `json:"error,omitempty"`
	StartedAt  time.Time           `json:"startedAt"`
	FinishedAt time.Time           `json:"finishedAt"`
	Draft      DraftView           `json:"draft"`
	Snapshot   RunSnapshotView     `json:"snapshot"`
	NodeRuns   []NodeRunView       `json:"nodeRuns"`
	Artifacts  []ArtifactView      `json:"artifacts"`
}

// StartRun validates and materializes the visible Draft, executes the single
// real `human-chat(source) -> llm-chat` turn through the chat Protocol Adapter,
// and persists its running progress before atomically finalizing its terminal
// history.
func (a *Application) StartRun(ctx context.Context, input StartRunInput) (RunView, error) {
	if strings.TrimSpace(input.WorkflowID) == "" {
		return RunView{}, fmt.Errorf("start Run: workflow ID must not be empty")
	}
	if input.ExpectedLockVersion == 0 {
		return RunView{}, fmt.Errorf("start Run: expected lock version must be positive")
	}
	if strings.TrimSpace(input.HumanInput.NodeID) == "" {
		return RunView{}, fmt.Errorf("start Run: human input Node ID must not be empty")
	}
	if strings.TrimSpace(input.HumanInput.Text) == "" {
		return RunView{}, fmt.Errorf("start Run: human input text must not be empty")
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
	preview := a.previewDraftWithModels(draft.Content, a.activeModelUUIDs(ctx))
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
		ID: runID, WorkflowID: input.WorkflowID, RevisionID: revision.ID, Status: "running",
		Snapshot: snapshot, StartedAt: now, FinishedAt: now,
	}
	var beginResult productworkflow.StartRunResult
	begun := false
	beginRun := func() error {
		result, beginErr := a.runRepo.BeginProductWorkflowRun(ctx, productworkflow.StartRunRequest{
			WorkflowID: input.WorkflowID, ExpectedLockVersion: input.ExpectedLockVersion, DraftContent: draftJSON,
			Revision: revision, Run: run,
		})
		if beginErr != nil {
			return beginErr
		}
		beginResult = result
		revision = result.Revision
		run = result.Run
		begun = true
		return nil
	}
	recordProgress := func(nodeRuns []productworkflow.NodeRun, artifacts []productworkflow.RunArtifact) error {
		return a.runRepo.RecordProductWorkflowRunProgress(ctx, runID, nodeRuns, artifacts)
	}
	nodeRuns, runArtifacts, artifactViews, err := a.executeSingleTurn(ctx, runID, materialized, selections, input.HumanInput, beginRun, recordProgress, now)
	if err != nil {
		if !begun {
			_ = os.RemoveAll(a.runPaths.RunDir(runID))
			return RunView{}, fmt.Errorf("start Run: %w", err)
		}
		run.Status = "failed"
		run.FinishedAt = time.Now().UTC()
		run.Error = productExecutionError(err)
		if len(nodeRuns) > 0 && nodeRuns[len(nodeRuns)-1].Diagnostics.Error != nil {
			run.Error = nodeRuns[len(nodeRuns)-1].Diagnostics.Error
		}
		if run.Error.Kind == "unknown-outcome" {
			run.Status = "interrupted"
		}
		finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancelFinish()
		if persistErr := a.runRepo.FinishProductWorkflowRun(finishCtx, productworkflow.FinishRunRequest{Run: run, NodeRuns: nodeRuns, Artifacts: runArtifacts}); persistErr != nil {
			return RunView{}, fmt.Errorf("start Run: %w; persist failed Run: %w", err, persistErr)
		}
		return RunView{}, &RunExecutionError{RunID: run.ID, Details: run.Error, Err: err}
	}
	run.Status = "succeeded"
	run.FinishedAt = time.Now().UTC()
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelFinish()
	if err := a.runRepo.FinishProductWorkflowRun(finishCtx, productworkflow.FinishRunRequest{Run: run, NodeRuns: nodeRuns, Artifacts: runArtifacts}); err != nil {
		return RunView{}, fmt.Errorf("start Run: finish persisted Run: %w", err)
	}
	materializedDraft, err := draftView(beginResult.Draft)
	if err != nil {
		return RunView{}, fmt.Errorf("start Run: %w", err)
	}
	// The materialized Draft has a resolvable UUID per agent Node by
	// construction; the Preview is re-checked against the live settings the
	// Run just resolved with.
	materializedPreview := a.previewDraftWithModels(materializedDraft.Content, a.activeModelUUIDs(ctx))
	materializedDraft.Preview = &materializedPreview
	views := make([]NodeRunView, 0, len(nodeRuns))
	for _, nodeRun := range nodeRuns {
		views = append(views, nodeRunView(nodeRun))
	}
	return RunView{ID: run.ID, RevisionID: revision.ID, Status: run.Status, Error: run.Error, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Draft: materializedDraft, Snapshot: runSnapshotView(run.Snapshot), NodeRuns: views, Artifacts: artifactViews}, nil
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
			Protocol: resolved.Provider.Protocol, Dialect: resolved.Provider.Dialect, BaseURL: resolved.Provider.BaseURL, APIKeyRef: resolved.Provider.APIKeyRef,
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
func (a *Application) executeSingleTurn(ctx context.Context, runID string, content map[string]any, selections []productworkflow.ResolvedLLMSelection, humanInput HumanRunInput, beginRun func() error, recordProgress func([]productworkflow.NodeRun, []productworkflow.RunArtifact) error, now time.Time) ([]productworkflow.NodeRun, []productworkflow.RunArtifact, []ArtifactView, error) {
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
	if humanInput.NodeID != sourceID {
		return nil, nil, nil, fmt.Errorf("single-turn executor requires human input for authored source Node %q", sourceID)
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
	agentID := stringValue(agent, "id")
	apiKey, err := a.secrets.Resolve(ctx, selection.APIKeyRef)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("llm-chat Node %q resolve Provider API Key: %w", agentID, err)
	}
	if err := beginRun(); err != nil {
		return nil, nil, nil, fmt.Errorf("persist running Run: %w", err)
	}
	store, err := artifact.NewFilesystemStore(a.runPaths.ArtifactsDir(runID))
	if err != nil {
		return nil, nil, nil, err
	}
	userText := humanInput.Text
	sourceConversation := chat.Conversation{Messages: []chat.ChatMessage{chat.UserTextMessage(userText)}}
	sourceArtifactID := uuid.NewString()
	sourceRef, err := store.Put(artifact.Artifact{ID: sourceArtifactID, Kind: artifact.KindConversation, Version: "1", Data: sourceConversation})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write user Conversation Artifact: %w", err)
	}

	humanRunID := uuid.NewString()
	agentRunID := uuid.NewString()
	humanRun := productworkflow.NodeRun{ID: humanRunID, RunID: runID, NodeID: sourceID, NodeDefinition: stringValue(human, "definition"), NodeExecutor: stringValue(human, "executor"), Status: "succeeded", Inputs: map[string]artifact.ArtifactRef{}, Outputs: map[string]artifact.ArtifactRef{"conversation": sourceRef}, StartedAt: now, FinishedAt: now}
	sourceItem := productworkflow.RunArtifact{ID: sourceArtifactID, RunID: runID, NodeRunID: humanRunID, NodeID: sourceID, Port: "conversation", Type: "Conversation", Version: "1", URI: sourceRef.URI, CreatedAt: now}
	agentStarted := time.Now().UTC()
	runningAgent := productworkflow.NodeRun{ID: agentRunID, RunID: runID, NodeID: agentID, NodeDefinition: stringValue(agent, "definition"), NodeExecutor: stringValue(agent, "executor"), Status: "running", Inputs: map[string]artifact.ArtifactRef{"conversation": sourceRef}, Outputs: map[string]artifact.ArtifactRef{}, StartedAt: agentStarted, FinishedAt: agentStarted}
	if err := recordProgress([]productworkflow.NodeRun{humanRun, runningAgent}, []productworkflow.RunArtifact{sourceItem}); err != nil {
		return []productworkflow.NodeRun{humanRun}, []productworkflow.RunArtifact{sourceItem}, []ArtifactView{artifactView(sourceItem, sourceConversation)}, fmt.Errorf("persist Run progress: %w", err)
	}
	result, err := a.chat.Generate(ctx, chat.Connection{
		Protocol: selection.Protocol, InstructionsRole: chat.InstructionsRole(selection.Dialect), BaseURL: selection.BaseURL,
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
		// successful, but the attempted Run and completed human source remain
		// traceable together with the failed agent Node Run.
		details := productExecutionError(err)
		status := "failed"
		if details.Kind == "unknown-outcome" {
			status = "unknown-outcome"
		}
		agentRun := productworkflow.NodeRun{ID: agentRunID, RunID: runID, NodeID: agentID, NodeDefinition: stringValue(agent, "definition"), NodeExecutor: stringValue(agent, "executor"), Status: status, Inputs: map[string]artifact.ArtifactRef{"conversation": sourceRef}, Outputs: map[string]artifact.ArtifactRef{}, Diagnostics: productworkflow.NodeRunDiagnostics{Error: details}, StartedAt: agentStarted, FinishedAt: agentFinished}
		return []productworkflow.NodeRun{humanRun, agentRun}, []productworkflow.RunArtifact{sourceItem}, []ArtifactView{artifactView(sourceItem, sourceConversation)}, fmt.Errorf("llm-chat Node %q model call: %w", agentID, err)
	}
	finalConversation := chat.Conversation{Messages: append(append([]chat.ChatMessage{}, sourceConversation.Messages...), result.Assistant)}
	finalArtifactID := uuid.NewString()
	finalRef, err := store.Put(artifact.Artifact{ID: finalArtifactID, Kind: artifact.KindConversation, Version: "2", Data: finalConversation})
	if err != nil {
		wrapped := fmt.Errorf("write assistant Conversation Artifact: %w", err)
		details := productExecutionError(wrapped)
		failedAgent := productworkflow.NodeRun{
			ID: agentRunID, RunID: runID, NodeID: agentID,
			NodeDefinition: stringValue(agent, "definition"), NodeExecutor: stringValue(agent, "executor"),
			Status: "failed", Inputs: map[string]artifact.ArtifactRef{"conversation": sourceRef}, Outputs: map[string]artifact.ArtifactRef{},
			Diagnostics: productworkflow.NodeRunDiagnostics{Error: details}, StartedAt: agentStarted, FinishedAt: agentFinished,
		}
		return []productworkflow.NodeRun{humanRun, failedAgent}, []productworkflow.RunArtifact{sourceItem}, []ArtifactView{artifactView(sourceItem, sourceConversation)}, wrapped
	}
	usage := chat.Usage(result.Usage)
	nodeRuns := []productworkflow.NodeRun{
		humanRun,
		{ID: agentRunID, RunID: runID, NodeID: agentID, NodeDefinition: stringValue(agent, "definition"), NodeExecutor: stringValue(agent, "executor"), Status: "succeeded", Inputs: map[string]artifact.ArtifactRef{"conversation": sourceRef}, Outputs: map[string]artifact.ArtifactRef{"conversation": finalRef}, Diagnostics: productworkflow.NodeRunDiagnostics{ProviderRequestID: result.ProviderRequestID, FinishReason: result.FinishReason, Usage: &usage}, StartedAt: agentStarted, FinishedAt: agentFinished},
	}
	items := []productworkflow.RunArtifact{
		sourceItem,
		{ID: finalArtifactID, RunID: runID, NodeRunID: agentRunID, NodeID: agentID, Port: "conversation", Type: "Conversation", Version: "2", URI: finalRef.URI, CreatedAt: agentFinished},
	}
	views := []ArtifactView{
		artifactView(items[0], sourceConversation), artifactView(items[1], finalConversation),
	}
	return nodeRuns, items, views, nil
}

func productExecutionError(err error) *productworkflow.ExecutionError {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &productworkflow.ExecutionError{Kind: "unknown-outcome", Code: "request-interrupted", Message: err.Error(), UserAction: "inspect Provider activity, then start a new Run only if it is safe"}
	}
	var openAIError *chat.OpenAIError
	if errors.As(err, &openAIError) {
		return &productworkflow.ExecutionError{Kind: "structural", Code: string(openAIError.Kind), Message: openAIError.Error(), UserAction: providerErrorAction(openAIError.Kind)}
	}
	return &productworkflow.ExecutionError{Kind: "structural", Code: "runtime", Message: err.Error(), UserAction: "review the Node configuration and start a new Run"}
}

func providerErrorAction(kind chat.OpenAIErrorKind) string {
	switch kind {
	case chat.ErrAuth:
		return "check the Provider API Key and start a new Run"
	case chat.ErrRateLimit:
		return "wait for the Provider limit to reset, then start a new Run"
	case chat.ErrNetwork:
		return "check the Provider Base URL and network, then start a new Run"
	default:
		return "review the Provider response or choose another Model, then start a new Run"
	}
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
			Protocol: selection.Protocol, Dialect: selection.Dialect, BaseURL: selection.BaseURL, ModelUUID: selection.ModelUUID, ProviderModelID: selection.ProviderModelID,
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
		NodeExecutor: nodeRun.NodeExecutor, Status: nodeRun.Status, Inputs: nodeRun.Inputs, Outputs: nodeRun.Outputs, StartedAt: nodeRun.StartedAt, FinishedAt: nodeRun.FinishedAt,
	}
	if nodeRun.Diagnostics.ProviderRequestID == "" && nodeRun.Diagnostics.FinishReason == "" && nodeRun.Diagnostics.Usage == nil && nodeRun.Diagnostics.Error == nil {
		return view
	}
	view.Diagnostics = &NodeRunDiagnosticsView{
		ProviderRequestID: nodeRun.Diagnostics.ProviderRequestID,
		FinishReason:      nodeRun.Diagnostics.FinishReason,
		Usage:             nodeRun.Diagnostics.Usage,
		Error:             nodeRun.Diagnostics.Error,
	}
	return view
}

func artifactView(item productworkflow.RunArtifact, conversation chat.Conversation) ArtifactView {
	messages := make([]ChatMessageView, 0, len(conversation.Messages))
	for _, message := range conversation.Messages {
		messages = append(messages, ChatMessageView{Role: message.Role, Text: message.Text()})
	}
	return ArtifactView{ID: item.ID, NodeID: item.NodeID, Port: item.Port, Type: item.Type, Version: item.Version, URI: item.URI, Messages: messages}
}
