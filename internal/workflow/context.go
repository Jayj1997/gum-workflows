package workflow

const (
	// ProjectContextName is the built-in Workflow Context that exposes the
	// in-place Project Workspace to node input bindings.
	ProjectContextName = "project"
	// ProjectCodeOutput is the SourceCode output exported by the project Context.
	ProjectCodeOutput = "code"
)

var workflowContextOutputs = map[string]map[string]string{
	ProjectContextName: {
		ProjectCodeOutput: "SourceCode",
	},
}

// IsWorkflowContext reports whether name identifies a built-in Workflow
// Context rather than a Node Instance.
func IsWorkflowContext(name string) bool {
	_, ok := workflowContextOutputs[name]
	return ok
}

// WorkflowContextOutputType returns the Artifact contract type exported by a
// built-in Workflow Context output.
func WorkflowContextOutputType(contextName, outputName string) (string, bool) {
	outputs, ok := workflowContextOutputs[contextName]
	if !ok {
		return "", false
	}
	typeName, ok := outputs[outputName]
	return typeName, ok
}
