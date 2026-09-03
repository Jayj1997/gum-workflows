// Package product defines the application boundary used by product-facing UI
// adapters. It is deliberately separate from the workflow/v1 CLI.
package product

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/chat"
	"github.com/Jayj1997/gum-workflows/internal/product/nodecatalog"
	productworkflow "github.com/Jayj1997/gum-workflows/internal/product/workflow"
	"github.com/Jayj1997/gum-workflows/internal/redaction"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
	"github.com/Jayj1997/gum-workflows/internal/secret"
)

// WorkspaceView is the product shell state returned to a UI adapter.
type WorkspaceView struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// WorkflowApplication is the product use-case boundary consumed by UI adapters.
type WorkflowApplication interface {
	OpenWorkspace(ctx context.Context) (WorkspaceView, error)
	CreateWorkflow(ctx context.Context, input CreateWorkflowInput) (WorkflowView, error)
	ListWorkflows(ctx context.Context) ([]WorkflowView, error)
	GetDraft(ctx context.Context, workflowID string) (DraftView, error)
	UpdateDraft(ctx context.Context, input UpdateDraftInput) (DraftUpdateView, error)
	ListNodeCatalog(ctx context.Context) ([]nodecatalog.Entry, error)
	GetLLMSettings(ctx context.Context) (LLMSettingsView, error)
	CreateLLMProvider(ctx context.Context, input CreateLLMProviderInput) (LLMProviderView, error)
	UpdateLLMProvider(ctx context.Context, input UpdateLLMProviderInput) (LLMProviderView, error)
	DeleteLLMProvider(ctx context.Context, input DeleteLLMProviderInput) error
	ListProviderDeletionImpact(ctx context.Context, providerID string) (AffectedWorkflowsView, error)
	SetDefaultLLMProvider(ctx context.Context, providerID string) (LLMSettingsView, error)
	CreateLLMModel(ctx context.Context, input CreateLLMModelInput) (LLMModelView, error)
	UpdateLLMModel(ctx context.Context, input UpdateLLMModelInput) (LLMModelView, error)
	DeleteLLMModel(ctx context.Context, providerID, modelID string) error
	ListModelDeletionImpact(ctx context.Context, providerID, modelID string) (AffectedWorkflowsView, error)
	SetDefaultLLMModel(ctx context.Context, providerID, modelID string) (LLMSettingsView, error)
	ResolveDefaultLLMModel(ctx context.Context) (ResolvedLLMModelView, error)
	StartRun(ctx context.Context, input StartRunInput) (RunView, error)
	ListRevisions(ctx context.Context, workflowID string) ([]RevisionView, error)
	ListRevisionRuns(ctx context.Context, revisionID string) ([]RunSummaryView, error)
	GetRunHistory(ctx context.Context, runID string) (RunView, error)
	GenerateDiagnosticsBundle(ctx context.Context, runID string) (DiagnosticsBundleView, error)
}

// DraftView is the current mutable Product Workflow definition returned to UI adapters.
type DraftView struct {
	WorkflowID  string           `json:"workflowId"`
	Content     map[string]any   `json:"content"`
	LockVersion uint64           `json:"lockVersion"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	Preview     *WorkflowPreview `json:"preview,omitempty"`
}

// UpdateDraftInput is an autosave request against the UI's current lock token.
type UpdateDraftInput struct {
	WorkflowID          string         `json:"workflowId"`
	ExpectedLockVersion uint64         `json:"expectedLockVersion"`
	Content             map[string]any `json:"content"`
}

// Diagnostic identifies one problem in an incomplete Product Workflow Draft.
type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// WorkflowPreview is the renderer-independent structure derived from a Draft.
type WorkflowPreview struct {
	Nodes       []PreviewNode  `json:"nodes"`
	Edges       []PreviewEdge  `json:"edges"`
	Groups      []PreviewGroup `json:"groups"`
	Diagnostics []Diagnostic   `json:"diagnostics"`
}

// PreviewNode is a renderer-independent Node projection.
type PreviewNode struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definitionId"`
	DisplayName  string `json:"displayName"`
	Kind         string `json:"kind,omitempty"`
}

// PreviewEdge is a typed Data Edge or untyped Control Edge.
type PreviewEdge struct {
	Kind         string `json:"kind"`
	SourceNodeID string `json:"sourceNodeId"`
	SourcePort   string `json:"sourcePort,omitempty"`
	TargetNodeID string `json:"targetNodeId"`
	TargetPort   string `json:"targetPort,omitempty"`
	ArtifactType string `json:"artifactType,omitempty"`
}

// PreviewGroup identifies one cyclic set without exposing layout-library data.
type PreviewGroup struct {
	NodeIDs []string `json:"nodeIds"`
}

// DraftUpdateView returns autosave state and the complete latest Draft projection.
type DraftUpdateView struct {
	Draft           DraftView       `json:"draft"`
	Preview         WorkflowPreview `json:"preview"`
	Saved           bool            `json:"saved"`
	Conflict        bool            `json:"conflict"`
	RefreshRequired bool            `json:"refreshRequired"`
}

// CreateWorkflowInput is the user-authored metadata for a new Product Workflow.
type CreateWorkflowInput struct {
	DisplayName string `json:"displayName"`
}

// WorkflowView is Product Workflow identity and display metadata returned to UI adapters.
type WorkflowView struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Application coordinates Product Workflow use cases for UI adapters.
type Application struct {
	repository   productworkflow.Repository
	llmSettings  productworkflow.LLMSettingsRepository
	catalog      *nodecatalog.Registry
	runPaths     runtimepath.Paths
	runRepo      productworkflow.RunRepository
	runHistory   productworkflow.RunHistoryRepository
	runRecovery  productworkflow.RunRecoveryRepository
	secrets      secret.Adapter
	chat         chat.Adapter
	redactor     *redaction.Redactor
	initializeMu sync.Mutex
	initialized  bool
}

// ApplicationOption configures optional product runtime dependencies.
type ApplicationOption func(*Application)

// WithRunPaths configures the Local Data Root layout used by Product Runs.
func WithRunPaths(paths runtimepath.Paths) ApplicationOption {
	return func(application *Application) { application.runPaths = paths }
}

// WithSecretAdapter configures credential storage for Product Provider use cases.
func WithSecretAdapter(adapter secret.Adapter) ApplicationOption {
	return func(application *Application) { application.secrets = adapter }
}

// WithChatAdapter configures the protocol Adapter used by real model calls.
func WithChatAdapter(adapter chat.Adapter) ApplicationOption {
	return func(application *Application) { application.chat = adapter }
}

// WithRedactor configures the shared Secret redaction seam used by run logs,
// errors and diagnostics bundles. A nil argument leaves the default in place.
func WithRedactor(redactor *redaction.Redactor) ApplicationOption {
	return func(application *Application) {
		if redactor != nil {
			application.redactor = redactor
		}
	}
}

// NewApplication creates the Product Application with injected persistence, the
// explicitly assembled product Node Definition/Executor registry, and the default
// OpenAI-compatible protocol Adapter for real model calls.
func NewApplication(repository productworkflow.Repository, catalog *nodecatalog.Registry, options ...ApplicationOption) *Application {
	settings, _ := repository.(productworkflow.LLMSettingsRepository)
	runs, _ := repository.(productworkflow.RunRepository)
	runHistory, _ := repository.(productworkflow.RunHistoryRepository)
	runRecovery, _ := repository.(productworkflow.RunRecoveryRepository)
	application := &Application{repository: repository, llmSettings: settings, runRepo: runs, runHistory: runHistory, runRecovery: runRecovery, catalog: catalog, chat: chat.NewOpenAIChatAdapter(nil), redactor: redaction.NewRedactor()}
	for _, option := range options {
		option(application)
	}
	return application
}

// ListNodeCatalog returns addable Nodes from the registered Definitions and Executors.
func (a *Application) ListNodeCatalog(context.Context) ([]nodecatalog.Entry, error) {
	if a.catalog == nil {
		return nil, fmt.Errorf("list node catalog: registry is not configured")
	}
	return a.catalog.Catalog(), nil
}

// OpenWorkspace reconciles in-flight Runs once for this Application process,
// then returns the product shell state.
func (a *Application) OpenWorkspace(ctx context.Context) (WorkspaceView, error) {
	a.initializeMu.Lock()
	defer a.initializeMu.Unlock()
	if !a.initialized && a.runRecovery != nil {
		if err := a.runRecovery.InterruptProductWorkflowRuns(ctx, time.Now().UTC()); err != nil {
			return WorkspaceView{}, fmt.Errorf("open workspace: interrupt unfinished Runs: %w", err)
		}
		a.initialized = true
	}
	return WorkspaceView{Title: "Gum Workflows", Message: "Product workspace ready"}, nil
}

// CreateWorkflow creates a SQLite Product Workflow through the repository seam.
func (a *Application) CreateWorkflow(ctx context.Context, input CreateWorkflowInput) (WorkflowView, error) {
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		return WorkflowView{}, fmt.Errorf("create workflow: display name must not be empty")
	}
	workflow, err := a.repository.CreateProductWorkflow(ctx, displayName)
	if err != nil {
		return WorkflowView{}, fmt.Errorf("create workflow: %w", err)
	}
	return workflowView(workflow), nil
}

// ListWorkflows lists SQLite Product Workflows in repository-defined stable order.
func (a *Application) ListWorkflows(ctx context.Context) ([]WorkflowView, error) {
	workflows, err := a.repository.ListProductWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	views := make([]WorkflowView, 0, len(workflows))
	for _, workflow := range workflows {
		views = append(views, workflowView(workflow))
	}
	return views, nil
}

// GetDraft returns the current mutable definition for a Product Workflow.
func (a *Application) GetDraft(ctx context.Context, workflowID string) (DraftView, error) {
	draft, err := a.repository.GetProductWorkflowDraft(ctx, workflowID)
	if err != nil {
		return DraftView{}, fmt.Errorf("get draft: %w", err)
	}
	view, err := draftView(draft)
	if err != nil {
		return DraftView{}, err
	}
	preview := a.previewDraft(ctx, view.Content)
	view.Preview = &preview
	return view, nil
}

// UpdateDraft autosaves semantic content and returns the latest Preview and Diagnostics.
func (a *Application) UpdateDraft(ctx context.Context, input UpdateDraftInput) (DraftUpdateView, error) {
	if strings.TrimSpace(input.WorkflowID) == "" {
		return DraftUpdateView{}, fmt.Errorf("update draft: workflow ID must not be empty")
	}
	if input.ExpectedLockVersion == 0 {
		return DraftUpdateView{}, fmt.Errorf("update draft: expected lock version must be positive")
	}
	content, err := json.Marshal(input.Content)
	if err != nil {
		return DraftUpdateView{}, fmt.Errorf("update draft: encode content: %w", err)
	}
	update, err := a.repository.UpdateProductWorkflowDraft(ctx, input.WorkflowID, input.ExpectedLockVersion, content)
	if err != nil {
		return DraftUpdateView{}, fmt.Errorf("update draft: %w", err)
	}
	view, err := draftView(update.Draft)
	if err != nil {
		return DraftUpdateView{}, fmt.Errorf("update draft: %w", err)
	}
	return DraftUpdateView{
		Draft:           view,
		Preview:         a.previewDraft(ctx, view.Content),
		Saved:           update.Saved,
		Conflict:        update.Conflict,
		RefreshRequired: update.Conflict,
	}, nil
}

func draftView(draft productworkflow.Draft) (DraftView, error) {
	var content map[string]any
	if err := json.Unmarshal(draft.Content, &content); err != nil {
		return DraftView{}, fmt.Errorf("decode draft content: %w", err)
	}
	return DraftView{WorkflowID: draft.WorkflowID, Content: content, LockVersion: draft.LockVersion, UpdatedAt: draft.UpdatedAt}, nil
}

func (a *Application) previewDraft(ctx context.Context, content map[string]any) WorkflowPreview {
	return a.previewDraftWithModels(content, a.activeModelUUIDs(ctx))
}

// activeModelUUIDs returns the set of non-deleted Gum Model UUIDs the Preview
// accepts as resolvable LLM preferences. A nil map means the settings seam is
// unavailable, in which case dangling preference diagnostics are skipped
// rather than fabricating false errors.
func (a *Application) activeModelUUIDs(ctx context.Context) map[string]struct{} {
	if a.llmSettings == nil {
		return nil
	}
	settings, err := a.llmSettings.GetLLMSettings(ctx)
	if err != nil {
		return nil
	}
	models := make(map[string]struct{})
	for _, provider := range settings.Providers {
		for _, model := range settings.Models[provider.ID] {
			models[model.ID] = struct{}{}
		}
	}
	return models
}

// previewDraftWithModels derives the Preview and aggregates every diagnostic,
// including dangling Gum Model UUID preferences on agent Nodes.
func (a *Application) previewDraftWithModels(content map[string]any, modelUUIDs map[string]struct{}) WorkflowPreview {
	preview := WorkflowPreview{Nodes: []PreviewNode{}, Edges: []PreviewEdge{}, Groups: []PreviewGroup{}, Diagnostics: []Diagnostic{}}
	if content["semanticSchemaVersion"] != "productWorkflow/v1" {
		preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
			Code: "invalid-semantic-schema-version", Severity: "error", Path: "semanticSchemaVersion",
			Message: "semantic schema version must be productWorkflow/v1",
		})
	}
	nodes, ok := content["nodes"].([]any)
	if !ok {
		preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
			Code: "workflow-needs-node", Severity: "error", Path: "nodes",
			Message: "workflow nodes must be a non-empty list",
		})
		return preview
	}
	if len(nodes) == 0 {
		preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
			Code: "workflow-needs-node", Severity: "error", Path: "nodes",
			Message: "workflow must contain at least one node",
		})
	}
	type nodeRecord struct {
		index      int
		content    map[string]any
		definition nodecatalog.Definition
		known      bool
	}
	records := make([]nodeRecord, 0, len(nodes))
	nodesByID := make(map[string]nodeRecord, len(nodes))
	for index, value := range nodes {
		node, ok := value.(map[string]any)
		if !ok {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "invalid-node", Severity: "error", Path: fmt.Sprintf("nodes[%d]", index), Message: "node must be an object",
			})
			continue
		}
		definitionID, _ := node["definition"].(string)
		nodeID, _ := node["id"].(string)
		displayName, _ := node["displayName"].(string)
		previewNode := PreviewNode{ID: nodeID, DefinitionID: definitionID, DisplayName: displayName}
		definition, found := a.catalog.Definition(definitionID)
		if !found {
			preview.Nodes = append(preview.Nodes, previewNode)
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "unknown-node-definition", Severity: "error", Path: fmt.Sprintf("nodes[%d].definition", index),
				Message: fmt.Sprintf("node definition %q is not in the Catalog", definitionID),
			})
		} else {
			previewNode.Kind = string(definition.Kind)
			preview.Nodes = append(preview.Nodes, previewNode)
		}
		record := nodeRecord{index: index, content: node, definition: definition, known: found}
		records = append(records, record)
		if strings.TrimSpace(nodeID) == "" {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "invalid-node-id", Severity: "error", Path: fmt.Sprintf("nodes[%d].id", index), Message: "node ID must not be empty",
			})
		} else if _, duplicate := nodesByID[nodeID]; duplicate {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "duplicate-node-id", Severity: "error", Path: fmt.Sprintf("nodes[%d].id", index), Message: fmt.Sprintf("node ID %q is already used", nodeID),
			})
		} else {
			nodesByID[nodeID] = record
		}
		if !found {
			continue
		}
		config, ok := node["config"].(map[string]any)
		if !ok {
			if node["config"] == nil {
				config = map[string]any{}
			} else {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "invalid-node-config", Severity: "error", Path: fmt.Sprintf("nodes[%d].config", index), Message: "config must be an object",
				})
				config = map[string]any{}
			}
		}
		for _, issue := range definition.Config.Validate(config) {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "invalid-node-config-" + issue.Code, Severity: "error",
				Path: fmt.Sprintf("nodes[%d].config.%s", index, issue.Field), Message: issue.Message,
			})
		}
		if definition.Kind != nodecatalog.NodeAgent {
			continue
		}
		// An agent Node's LLM preference must point at a live Model Slot.
		// Deleted or unknown UUIDs never fall back; the field-level diagnostic
		// keeps the Node form and Preview red until the user re-selects.
		if modelUUIDs == nil {
			continue
		}
		preference, valid := node["llm"].(map[string]any)
		if !valid {
			if node["llm"] != nil {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "invalid-llm-preference", Severity: "error",
					Path: fmt.Sprintf("nodes[%d].llm", index), Message: "llm preference must be an object",
				})
			}
			continue
		}
		modelUUID, _ := preference["modelUuid"].(string)
		if strings.TrimSpace(modelUUID) == "" {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "missing-model-uuid", Severity: "error",
				Path: fmt.Sprintf("nodes[%d].llm.modelUuid", index), Message: "select a Model for this agent Node",
			})
			continue
		}
		if _, resolvable := modelUUIDs[modelUUID]; !resolvable {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "dangling-model-uuid", Severity: "error",
				Path:    fmt.Sprintf("nodes[%d].llm.modelUuid", index),
				Message: fmt.Sprintf("Model %q is deleted or no longer exists; select another Model before running", modelUUID),
			})
		}
	}
	for _, target := range records {
		targetID, _ := target.content["id"].(string)
		inputs := map[string]any{}
		if value, exists := target.content["inputs"]; exists && value != nil {
			var valid bool
			inputs, valid = value.(map[string]any)
			if !valid {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "invalid-inputs", Severity: "error", Path: fmt.Sprintf("nodes[%d].inputs", target.index), Message: "inputs must be an object",
				})
				inputs = map[string]any{}
			}
		}
		inputNames := sortedKeys(inputs)
		if target.known {
			for inputName, port := range target.definition.Inputs {
				if _, bound := inputs[inputName]; !bound && !port.Optional {
					preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
						Code: "missing-input-binding", Severity: "error", Path: fmt.Sprintf("nodes[%d].inputs.%s", target.index, inputName),
						Message: fmt.Sprintf("required input %q is not bound", inputName),
					})
				}
			}
		}
		for _, inputName := range inputNames {
			bindingValue := inputs[inputName]
			inputPort, inputFound := target.definition.Inputs[inputName]
			if target.known && !inputFound {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "unknown-input-port", Severity: "error", Path: fmt.Sprintf("nodes[%d].inputs.%s", target.index, inputName),
					Message: fmt.Sprintf("input port %q is not declared by Node Definition %q", inputName, target.definition.ID),
				})
			}
			binding, valid := bindingValue.(map[string]any)
			if !valid {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "invalid-input-binding", Severity: "error", Path: fmt.Sprintf("nodes[%d].inputs.%s", target.index, inputName), Message: "input binding must be an object",
				})
				continue
			}
			from, _ := binding["from"].(string)
			sourceID, outputName, valid := parsePortReference(from)
			if !valid {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "invalid-input-binding", Severity: "error", Path: fmt.Sprintf("nodes[%d].inputs.%s.from", target.index, inputName), Message: "binding must use <node-id>.<output-port>",
				})
				continue
			}
			source, exists := nodesByID[sourceID]
			if !exists {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "unknown-input-source", Severity: "error", Path: fmt.Sprintf("nodes[%d].inputs.%s.from", target.index, inputName), Message: fmt.Sprintf("source node %q does not exist", sourceID),
				})
				continue
			}
			outputPort, outputFound := source.definition.Outputs[outputName]
			artifactType := ""
			if outputFound {
				artifactType = outputPort.Type
			}
			preview.Edges = append(preview.Edges, PreviewEdge{
				Kind: "data", SourceNodeID: sourceID, SourcePort: outputName,
				TargetNodeID: targetID, TargetPort: inputName, ArtifactType: artifactType,
			})
			if !source.known || !outputFound {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "unknown-output-port", Severity: "error", Path: fmt.Sprintf("nodes[%d].inputs.%s.from", target.index, inputName),
					Message: fmt.Sprintf("output port %q is not declared by source Node %q", outputName, sourceID),
				})
				continue
			}
			if inputFound && inputPort.Type != outputPort.Type {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "incompatible-input-type", Severity: "error", Path: fmt.Sprintf("nodes[%d].inputs.%s", target.index, inputName),
					Message: fmt.Sprintf("input type %s is incompatible with %s.%s type %s", inputPort.Type, sourceID, outputName, outputPort.Type),
				})
			}
		}
	}
	for _, target := range records {
		targetID, _ := target.content["id"].(string)
		value, exists := target.content["dependsOn"]
		if !exists || value == nil {
			continue
		}
		dependencies, valid := stringList(value)
		if !valid {
			preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
				Code: "invalid-control-dependencies", Severity: "error", Path: fmt.Sprintf("nodes[%d].dependsOn", target.index), Message: "control dependencies must be a list of Node IDs",
			})
			continue
		}
		for dependencyIndex, sourceID := range dependencies {
			if _, found := nodesByID[sourceID]; !found {
				preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
					Code: "unknown-control-dependency", Severity: "error", Path: fmt.Sprintf("nodes[%d].dependsOn[%d]", target.index, dependencyIndex), Message: fmt.Sprintf("control dependency node %q does not exist", sourceID),
				})
				continue
			}
			preview.Edges = append(preview.Edges, PreviewEdge{Kind: "control", SourceNodeID: sourceID, TargetNodeID: targetID})
		}
	}
	nodeIDs := make(map[string]struct{}, len(nodesByID))
	for nodeID := range nodesByID {
		nodeIDs[nodeID] = struct{}{}
	}
	preview.Groups = cycleGroups(nodeIDs, preview.Edges)
	return preview
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parsePortReference(reference string) (string, string, bool) {
	source, output, found := strings.Cut(reference, ".")
	return source, output, found && source != "" && output != "" && !strings.Contains(output, ".")
}

func stringList(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...), true
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, false
			}
			result = append(result, text)
		}
		return result, true
	default:
		return nil, false
	}
}

func cycleGroups(nodes map[string]struct{}, edges []PreviewEdge) []PreviewGroup {
	adjacency := make(map[string][]string, len(nodes))
	selfEdge := make(map[string]bool)
	for id := range nodes {
		adjacency[id] = []string{}
	}
	for _, edge := range edges {
		if _, sourceExists := nodes[edge.SourceNodeID]; !sourceExists {
			continue
		}
		if _, targetExists := nodes[edge.TargetNodeID]; !targetExists {
			continue
		}
		adjacency[edge.SourceNodeID] = append(adjacency[edge.SourceNodeID], edge.TargetNodeID)
		if edge.SourceNodeID == edge.TargetNodeID {
			selfEdge[edge.SourceNodeID] = true
		}
	}
	for id := range adjacency {
		sort.Strings(adjacency[id])
	}
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	index := 0
	indices := map[string]int{}
	lowlinks := map[string]int{}
	onStack := map[string]bool{}
	stack := make([]string, 0, len(nodes))
	groups := make([]PreviewGroup, 0)
	var visit func(string)
	visit = func(nodeID string) {
		indices[nodeID] = index
		lowlinks[nodeID] = index
		index++
		stack = append(stack, nodeID)
		onStack[nodeID] = true
		for _, next := range adjacency[nodeID] {
			if _, seen := indices[next]; !seen {
				visit(next)
				lowlinks[nodeID] = min(lowlinks[nodeID], lowlinks[next])
			} else if onStack[next] {
				lowlinks[nodeID] = min(lowlinks[nodeID], indices[next])
			}
		}
		if lowlinks[nodeID] != indices[nodeID] {
			return
		}
		component := []string{}
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, last)
			if last == nodeID {
				break
			}
		}
		if len(component) > 1 || selfEdge[nodeID] {
			sort.Strings(component)
			groups = append(groups, PreviewGroup{NodeIDs: component})
		}
	}
	for _, id := range ids {
		if _, seen := indices[id]; !seen {
			visit(id)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].NodeIDs[0] < groups[j].NodeIDs[0] })
	return groups
}

func workflowView(workflow productworkflow.Workflow) WorkflowView {
	return WorkflowView{
		ID:          workflow.ID,
		DisplayName: workflow.DisplayName,
		CreatedAt:   workflow.CreatedAt,
	}
}
