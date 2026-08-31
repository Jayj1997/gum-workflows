package nodecatalog

import (
	"fmt"
	"sort"
	"strings"
)

// NodeKind is the execution subject advertised by a product Node Definition.
type NodeKind string

// Product Node kinds needed by the first Catalog.
const (
	// NodeAgent identifies a Node whose execution needs model reasoning.
	NodeAgent NodeKind = "agent"
	// NodeHuman identifies a Node advanced by a human event.
	NodeHuman NodeKind = "human"
)

// Definition is the product Node business contract shown in the Catalog.
type Definition struct {
	ID          string          `json:"id"`
	DisplayName string          `json:"displayName"`
	Description string          `json:"description"`
	Kind        NodeKind        `json:"kind"`
	Inputs      map[string]Port `json:"inputs"`
	Outputs     map[string]Port `json:"outputs"`
	Config      ConfigSchema    `json:"config"`
}

// Port is one typed Artifact input or output in a product Node contract.
type Port struct {
	Type     string `json:"type"`
	Optional bool   `json:"optional,omitempty"`
}

// Executor identifies one executable version of a product Node Definition.
type Executor struct {
	DefinitionID string `json:"definitionId"`
	Version      string `json:"version"`
}

// Entry is one addable Definition paired with its selected latest Executor.
type Entry struct {
	Definition Definition `json:"definition"`
	Executor   Executor   `json:"executor"`
}

// Registry stores product Node Definitions and Executor versions explicitly.
type Registry struct {
	definitions map[string]Definition
	executors   map[string]map[string]Executor
}

// NewRegistry creates an empty product Node registry.
func NewRegistry() *Registry {
	return &Registry{definitions: map[string]Definition{}, executors: map[string]map[string]Executor{}}
}

// RegisterDefinition registers one product Node Definition.
func (r *Registry) RegisterDefinition(definition Definition) error {
	if strings.TrimSpace(definition.ID) == "" {
		return fmt.Errorf("register product node definition: ID must not be empty")
	}
	if _, exists := r.definitions[definition.ID]; exists {
		return fmt.Errorf("register product node definition: %q already registered", definition.ID)
	}
	r.definitions[definition.ID] = definition
	return nil
}

// RegisterExecutor registers one version after its Definition is present.
func (r *Registry) RegisterExecutor(executor Executor) error {
	if _, exists := r.definitions[executor.DefinitionID]; !exists {
		return fmt.Errorf("register product node executor: unknown definition %q", executor.DefinitionID)
	}
	if strings.TrimSpace(executor.Version) == "" {
		return fmt.Errorf("register product node executor %q: version must not be empty", executor.DefinitionID)
	}
	versions := r.executors[executor.DefinitionID]
	if versions == nil {
		versions = map[string]Executor{}
		r.executors[executor.DefinitionID] = versions
	}
	if _, exists := versions[executor.Version]; exists {
		return fmt.Errorf("register product node executor: (%s, %s) already registered", executor.DefinitionID, executor.Version)
	}
	versions[executor.Version] = executor
	return nil
}

// Definition returns a registered Definition by stable identity.
func (r *Registry) Definition(id string) (Definition, bool) {
	definition, ok := r.definitions[id]
	return definition, ok
}

// Executor reports whether a Definition version is registered.
func (r *Registry) Executor(definitionID, version string) (Executor, bool) {
	executor, ok := r.executors[definitionID][version]
	return executor, ok
}

// Catalog returns addable Definitions in stable identity order.
func (r *Registry) Catalog() []Entry {
	ids := make([]string, 0, len(r.definitions))
	for id := range r.definitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	entries := make([]Entry, 0, len(ids))
	for _, id := range ids {
		versions := make([]string, 0, len(r.executors[id]))
		for version := range r.executors[id] {
			versions = append(versions, version)
		}
		if len(versions) == 0 {
			continue
		}
		sort.Strings(versions)
		latest := versions[len(versions)-1]
		entries = append(entries, Entry{Definition: r.definitions[id], Executor: r.executors[id][latest]})
	}
	return entries
}
