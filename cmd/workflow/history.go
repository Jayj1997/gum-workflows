package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/Jayj1997/gum-workflows/internal/definition"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/llm"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

func pinAndImportDefinitions(
	ctx context.Context,
	dbPath string,
	def workflow.Definition,
	executors *node.ExecutorRegistry,
	definitions *definition.Registry,
	llmConfig *llm.Config,
) (workflow.Definition, error) {
	store, err := history.Open(ctx, dbPath)
	if err != nil {
		return workflow.Definition{}, fmt.Errorf("open history database: %w", err)
	}
	defer store.Close()

	nodeTypes, nodeDefinitions, nodeExecutors, err := definitionRows(definitions)
	if err != nil {
		return workflow.Definition{}, fmt.Errorf("prepare seed definitions: %w", err)
	}
	if err := store.ImportDefinitions(ctx, nodeTypes, nodeDefinitions, nodeExecutors); err != nil {
		return workflow.Definition{}, fmt.Errorf("import seed definitions: %w", err)
	}

	workflowRow, instanceRows, pinned, err := workflowRows(ctx, store, def, executors, definitions, llmConfig)
	if err != nil {
		return workflow.Definition{}, fmt.Errorf("prepare workflow definitions: %w", err)
	}
	if err := store.ImportWorkflow(ctx, workflowRow, instanceRows); err != nil {
		return workflow.Definition{}, fmt.Errorf("import workflow definitions: %w", err)
	}
	return pinned, nil
}

func definitionRows(registry *definition.Registry) ([]history.NodeTypeDefRow, []history.NodeDefRow, []history.NodeExecRow, error) {
	nodeTypes := make([]history.NodeTypeDefRow, 0, len(registry.NodeTypeNames()))
	for _, name := range registry.NodeTypeNames() {
		item, err := registry.NodeType(name)
		if err != nil {
			return nil, nil, nil, err
		}
		nodeTypes = append(nodeTypes, history.NodeTypeDefRow{
			Name: name, Description: item.Metadata.Description, Requires: requirements(item.Requires),
		})
	}

	definitions := make([]history.NodeDefRow, 0, len(registry.DefinitionNames()))
	var executors []history.NodeExecRow
	for _, name := range registry.DefinitionNames() {
		item, err := registry.Definition(name)
		if err != nil {
			return nil, nil, nil, err
		}
		inputs := make(map[string]history.InputPort, len(item.Inputs))
		for portName, port := range item.Inputs {
			inputs[portName] = history.InputPort{
				Type: port.Type, Optional: port.Optional, Description: port.Description,
			}
		}
		outputs := make(map[string]history.OutputPort, len(item.Outputs))
		for portName, port := range item.Outputs {
			outputs[portName] = history.OutputPort{Type: port.Type, Description: port.Description}
		}
		definitions = append(definitions, history.NodeDefRow{
			Name: name, Description: item.Metadata.Description, Type: string(item.Type),
			Requires: requirements(item.Requires), Inputs: inputs, Outputs: outputs,
		})

		for _, version := range registry.ExecutorVersions(name) {
			executor, err := registry.Executor(name, version)
			if err != nil {
				return nil, nil, nil, err
			}
			executors = append(executors, history.NodeExecRow{
				Node: name, Version: version, Name: executor.Metadata.Name,
				Description: executor.Metadata.Description, Updates: executor.Updates,
			})
		}
	}
	return nodeTypes, definitions, executors, nil
}

func workflowRows(
	ctx context.Context,
	store *history.Store,
	def workflow.Definition,
	executors *node.ExecutorRegistry,
	definitions *definition.Registry,
	llmConfig *llm.Config,
) (history.WorkflowRow, []history.NodeInstanceRow, workflow.Definition, error) {
	projects := make([]history.ProjectRow, 0, len(def.Projects))
	for _, project := range def.Projects {
		projects = append(projects, history.ProjectRow{Name: project.Name, Repository: project.Repository})
	}
	workflowRow := history.WorkflowRow{
		Name: def.Metadata.Name, Version: def.Metadata.Version,
		Description: def.Metadata.Description, Projects: projects,
	}

	ids := make([]string, 0, len(def.Nodes))
	for id := range def.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	instances := make([]history.NodeInstanceRow, 0, len(ids))
	for _, id := range ids {
		spec := def.Nodes[id]
		factory, err := resolveExecutor(ctx, store, executors, spec)
		if err != nil {
			return history.WorkflowRow{}, nil, workflow.Definition{}, fmt.Errorf("node %q executor: %w", id, err)
		}
		spec.Executor = factory.Version()
		def.Nodes[id] = spec

		definitionID, err := store.DefinitionID(ctx, spec.Node)
		if err != nil {
			return history.WorkflowRow{}, nil, workflow.Definition{}, fmt.Errorf("node %q definition: %w", id, err)
		}
		executorID, err := store.ExecutorID(ctx, spec.Node, spec.Executor)
		if err != nil {
			return history.WorkflowRow{}, nil, workflow.Definition{}, fmt.Errorf("node %q executor: %w", id, err)
		}

		provider, model := "", ""
		nodeDefinition, err := definitions.Definition(spec.Node)
		if err != nil {
			return history.WorkflowRow{}, nil, workflow.Definition{}, fmt.Errorf("node %q definition: %w", id, err)
		}
		if nodeDefinition.Type == definition.TypeAgent {
			if llmConfig == nil {
				return history.WorkflowRow{}, nil, workflow.Definition{}, fmt.Errorf("node %q: llm config is required", id)
			}
			ref, err := llmConfig.Resolve(spec.LLM, spec.TargetModel)
			if err != nil {
				return history.WorkflowRow{}, nil, workflow.Definition{}, fmt.Errorf("node %q: %w", id, err)
			}
			provider, model = ref.Provider, ref.Model
		}

		inputs := make(map[string]history.InputBinding, len(spec.Inputs))
		for name, binding := range spec.Inputs {
			inputs[name] = history.InputBinding{From: binding.From}
		}
		instances = append(instances, history.NodeInstanceRow{
			NodeID: id, NodeDefinitionID: definitionID, NodeExecutorID: executorID,
			DisplayName: spec.Metadata.Name, Description: spec.Metadata.Description,
			LLMProvider: provider, LLMModel: model, Inputs: inputs,
			DependsOn: append([]string(nil), spec.DependsOn...), Config: spec.Config,
		})
	}
	return workflowRow, instances, def, nil
}

func resolveExecutor(ctx context.Context, store *history.Store, registry *node.ExecutorRegistry, spec workflow.NodeSpec) (node.ExecutorFactory, error) {
	if spec.Executor != "" {
		return registry.Get(spec.Node, spec.Executor)
	}
	latest, err := registry.Latest(spec.Node)
	if err != nil {
		return nil, err
	}
	version := latest.Version()
	databaseVersions, err := store.ExecutorVersions(ctx, spec.Node)
	if err != nil {
		return nil, err
	}
	for _, candidate := range databaseVersions {
		if definition.CompareVersions(candidate, version) > 0 {
			version = candidate
		}
	}
	return registry.Get(spec.Node, version)
}

func validateExistingDatabaseExecutors(ctx context.Context, dbPath string, def workflow.Definition, registry *node.ExecutorRegistry) error {
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat history database: %w", err)
	}
	store, err := history.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open history database read-only: %w", err)
	}
	defer store.Close()
	version, err := store.UserVersion(ctx)
	if err != nil {
		return fmt.Errorf("read history database schema version: %w", err)
	}
	if version < history.DefinitionSchemaVersion {
		return nil
	}

	ids := make([]string, 0, len(def.Nodes))
	for id := range def.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		spec := def.Nodes[id]
		if spec.Executor != "" {
			continue
		}
		if _, err := resolveExecutor(ctx, store, registry, spec); err != nil {
			return fmt.Errorf("node %q executor: %w", id, err)
		}
	}
	return nil
}

func requirements(items []definition.Requirement) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = string(item)
	}
	return result
}
