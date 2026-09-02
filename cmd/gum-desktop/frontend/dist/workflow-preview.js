import { validateConfig } from "./node-registry.js";

function diagnostic(code, path, message) {
	return { code, severity: "error", path, message };
}

function cycleGroups(nodes, edges) {
	const ids = [...nodes.keys()].sort();
	const adjacency = new Map(ids.map((id) => [id, []]));
	const selfEdges = new Set();
	for (const edge of edges) {
		if (!adjacency.has(edge.sourceNodeId) || !adjacency.has(edge.targetNodeId)) continue;
		adjacency.get(edge.sourceNodeId).push(edge.targetNodeId);
		if (edge.sourceNodeId === edge.targetNodeId) selfEdges.add(edge.sourceNodeId);
	}
	for (const targets of adjacency.values()) targets.sort();
	let nextIndex = 0;
	const indices = new Map();
	const lowlinks = new Map();
	const stack = [];
	const onStack = new Set();
	const groups = [];
	function visit(nodeId) {
		indices.set(nodeId, nextIndex);
		lowlinks.set(nodeId, nextIndex);
		nextIndex += 1;
		stack.push(nodeId);
		onStack.add(nodeId);
		for (const target of adjacency.get(nodeId)) {
			if (!indices.has(target)) {
				visit(target);
				lowlinks.set(nodeId, Math.min(lowlinks.get(nodeId), lowlinks.get(target)));
			} else if (onStack.has(target)) {
				lowlinks.set(nodeId, Math.min(lowlinks.get(nodeId), indices.get(target)));
			}
		}
		if (lowlinks.get(nodeId) !== indices.get(nodeId)) return;
		const component = [];
		while (stack.length) {
			const member = stack.pop();
			onStack.delete(member);
			component.push(member);
			if (member === nodeId) break;
		}
		if (component.length > 1 || selfEdges.has(nodeId)) groups.push({ nodeIds: component.sort() });
	}
	for (const id of ids) if (!indices.has(id)) visit(id);
	return groups.sort((left, right) => left.nodeIds[0].localeCompare(right.nodeIds[0]));
}

export function createWorkflowPreview(content, registry, options = {}) {
	const preview = { nodes: [], edges: [], groups: [], diagnostics: [] };
	// modelUUIDs mirrors the Application's live Model Slot set: a missing or
	// nil-like value skips dangling-preference diagnostics (history views must
	// not flag Revisions whose Slots were deleted after the Run).
	const modelUUIDs = options.modelUUIDs;
	const hasModelUUIDs = modelUUIDs instanceof Set;
	if (content?.semanticSchemaVersion !== "productWorkflow/v1") {
		preview.diagnostics.push(diagnostic("invalid-semantic-schema-version", "semanticSchemaVersion", "semantic schema version must be productWorkflow/v1"));
	}
	if (!Array.isArray(content?.nodes)) {
		preview.diagnostics.push(diagnostic("workflow-needs-node", "nodes", "workflow nodes must be a non-empty list"));
		return preview;
	}
	if (content.nodes.length === 0) preview.diagnostics.push(diagnostic("workflow-needs-node", "nodes", "workflow must contain at least one node"));
	const records = [];
	const nodesById = new Map();
	for (const [index, node] of content.nodes.entries()) {
		if (!node || typeof node !== "object" || Array.isArray(node)) {
			preview.diagnostics.push(diagnostic("invalid-node", `nodes[${index}]`, "node must be an object"));
			continue;
		}
		const definition = registry.definition(node.definition);
		preview.nodes.push({ id: node.id ?? "", definitionId: node.definition ?? "", displayName: node.displayName ?? "", ...(definition ? { kind: definition.kind } : {}) });
		if (!definition) preview.diagnostics.push(diagnostic("unknown-node-definition", `nodes[${index}].definition`, `node definition ${JSON.stringify(node.definition)} is not in the Catalog`));
		const record = { index, node, definition };
		records.push(record);
		if (!node.id) preview.diagnostics.push(diagnostic("invalid-node-id", `nodes[${index}].id`, "node ID must not be empty"));
		else if (nodesById.has(node.id)) preview.diagnostics.push(diagnostic("duplicate-node-id", `nodes[${index}].id`, `node ID ${JSON.stringify(node.id)} is already used`));
		else nodesById.set(node.id, record);
		if (!definition) continue;
		const config = node.config === undefined || node.config === null ? {} : node.config;
		if (!config || typeof config !== "object" || Array.isArray(config)) {
			preview.diagnostics.push(diagnostic("invalid-node-config", `nodes[${index}].config`, "config must be an object"));
		} else {
			for (const issue of validateConfig(definition.config, config)) {
				preview.diagnostics.push(diagnostic(`invalid-node-config-${issue.code}`, `nodes[${index}].config.${issue.field}`, issue.message));
			}
		}
		if (definition.kind !== "agent" || !hasModelUUIDs) continue;
		if (!node.llm || typeof node.llm !== "object" || Array.isArray(node.llm)) {
			if (node.llm !== undefined && node.llm !== null) {
				preview.diagnostics.push(diagnostic("invalid-llm-preference", `nodes[${index}].llm`, "llm preference must be an object"));
			}
			continue;
		}
		const modelUUID = node.llm.modelUuid;
		if (typeof modelUUID !== "string" || modelUUID.trim() === "") {
			preview.diagnostics.push(diagnostic("missing-model-uuid", `nodes[${index}].llm.modelUuid`, "select a Model for this agent Node"));
		} else if (!modelUUIDs.has(modelUUID)) {
			preview.diagnostics.push(diagnostic("dangling-model-uuid", `nodes[${index}].llm.modelUuid`, `Model ${JSON.stringify(modelUUID)} is deleted or no longer exists; select another Model before running`));
		}
	}
	for (const { index, node, definition } of records) {
		let inputs = node.inputs ?? {};
		if (!inputs || typeof inputs !== "object" || Array.isArray(inputs)) {
			preview.diagnostics.push(diagnostic("invalid-inputs", `nodes[${index}].inputs`, "inputs must be an object"));
			inputs = {};
		}
		for (const [name, port] of Object.entries(definition?.inputs ?? {})) {
			if (!(name in inputs) && !port.optional) preview.diagnostics.push(diagnostic("missing-input-binding", `nodes[${index}].inputs.${name}`, `required input ${JSON.stringify(name)} is not bound`));
		}
		for (const name of Object.keys(inputs).sort()) {
			const inputPort = definition?.inputs?.[name];
			if (definition && !inputPort) preview.diagnostics.push(diagnostic("unknown-input-port", `nodes[${index}].inputs.${name}`, `input port ${JSON.stringify(name)} is not declared by Node Definition ${JSON.stringify(definition.id)}`));
			const from = inputs[name]?.from;
			const parts = typeof from === "string" ? from.split(".") : [];
			if (parts.length !== 2 || parts.some((part) => !part)) {
				preview.diagnostics.push(diagnostic("invalid-input-binding", `nodes[${index}].inputs.${name}.from`, "binding must use <node-id>.<output-port>"));
				continue;
			}
			const [sourceNodeId, sourcePort] = parts;
			const source = nodesById.get(sourceNodeId);
			if (!source) {
				preview.diagnostics.push(diagnostic("unknown-input-source", `nodes[${index}].inputs.${name}.from`, `source node ${JSON.stringify(sourceNodeId)} does not exist`));
				continue;
			}
			const outputPort = source.definition?.outputs?.[sourcePort];
			preview.edges.push({ kind: "data", sourceNodeId, sourcePort, targetNodeId: node.id, targetPort: name, ...(outputPort ? { artifactType: outputPort.type } : {}) });
			if (!outputPort) preview.diagnostics.push(diagnostic("unknown-output-port", `nodes[${index}].inputs.${name}.from`, `output port ${JSON.stringify(sourcePort)} is not declared by source Node ${JSON.stringify(sourceNodeId)}`));
			else if (inputPort && inputPort.type !== outputPort.type) preview.diagnostics.push(diagnostic("incompatible-input-type", `nodes[${index}].inputs.${name}`, `input type ${inputPort.type} is incompatible with ${sourceNodeId}.${sourcePort} type ${outputPort.type}`));
		}
	}
	for (const { index, node } of records) {
		if (node.dependsOn === undefined || node.dependsOn === null) continue;
		if (!Array.isArray(node.dependsOn) || node.dependsOn.some((id) => typeof id !== "string" || !id)) {
			preview.diagnostics.push(diagnostic("invalid-control-dependencies", `nodes[${index}].dependsOn`, "control dependencies must be a list of Node IDs"));
			continue;
		}
		for (const [dependencyIndex, sourceNodeId] of node.dependsOn.entries()) {
			if (!nodesById.has(sourceNodeId)) preview.diagnostics.push(diagnostic("unknown-control-dependency", `nodes[${index}].dependsOn[${dependencyIndex}]`, `control dependency node ${JSON.stringify(sourceNodeId)} does not exist`));
			else preview.edges.push({ kind: "control", sourceNodeId, targetNodeId: node.id });
		}
	}
	preview.groups = cycleGroups(nodesById, preview.edges);
	return preview;
}
