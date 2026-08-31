function issue(field, code, message) {
	return { field, code, message };
}

export function validateConfig(schema, config) {
	const issues = [];
	const fields = new Map(schema.fields.map((field) => [field.name, field]));
	for (const field of schema.fields) {
		const value = config?.[field.name];
		if (value === undefined || value === null) {
			if (field.required && !field.hasDefault) issues.push(issue(field.name, "required", "field is required"));
			continue;
		}
		if (["string", "markdown"].includes(field.type) && typeof value !== "string") {
			issues.push(issue(field.name, "invalid-type", `must be ${field.type}`));
			continue;
		}
		if (field.type === "boolean" && typeof value !== "boolean") {
			issues.push(issue(field.name, "invalid-type", "must be boolean"));
			continue;
		}
		if (field.type === "enum") {
			if (typeof value !== "string") issues.push(issue(field.name, "invalid-type", "must be enum value"));
			else if (!(field.values ?? []).includes(value)) issues.push(issue(field.name, "invalid-enum", `must be one of ${field.values}`));
			continue;
		}
		if (["integer", "number"].includes(field.type)) {
			if (typeof value !== "number" || !Number.isFinite(value) || (field.type === "integer" && !Number.isInteger(value))) {
				issues.push(issue(field.name, "invalid-type", `must be ${field.type}`));
				continue;
			}
			if (field.min !== undefined && value < field.min) issues.push(issue(field.name, "below-minimum", `must be at least ${field.min}`));
			else if (field.max !== undefined && value > field.max) issues.push(issue(field.name, "above-maximum", `must be at most ${field.max}`));
		}
	}
	for (const name of Object.keys(config ?? {}).sort()) {
		if (!fields.has(name)) issues.push(issue(name, "unknown", "field is not declared by the Node Definition"));
	}
	return issues;
}

export function createNodeRegistry() {
	const definitions = new Map();
	const executors = new Map();
	return {
		registerDefinition(definition) {
			if (definitions.has(definition.id)) throw new Error(`Node Definition ${definition.id} is already registered`);
			definitions.set(definition.id, structuredClone(definition));
		},
		registerExecutor(executor) {
			if (!definitions.has(executor.definitionId)) throw new Error(`Node Definition ${executor.definitionId} is not registered`);
			executors.set(`${executor.definitionId}@${executor.version}`, structuredClone(executor));
		},
		catalog() {
			return [...definitions.values()].sort((left, right) => left.id.localeCompare(right.id)).flatMap((definition) => {
				const versions = [...executors.values()].filter((executor) => executor.definitionId === definition.id).sort((left, right) => left.version.localeCompare(right.version));
				return versions.length ? [{ definition: structuredClone(definition), executor: structuredClone(versions.at(-1)) }] : [];
			});
		},
		definition(id) { return definitions.get(id); },
	};
}

export function createBuiltinNodeRegistry() {
	const registry = createNodeRegistry();
	for (const definition of [
		{ id: "human-chat", displayName: "Human chat", description: "Collect a human message for a Conversation", kind: "human", config: { fields: [] } },
		{
			id: "llm-chat", displayName: "LLM chat", description: "Append one model response to a Conversation", kind: "agent",
			config: { fields: [
				{ name: "instructions", type: "markdown", required: false, hasDefault: false, sensitive: false, presentation: { label: "Instructions", help: "Guidance sent separately from the Conversation", editor: "markdown" } },
				{ name: "temperature", type: "number", required: false, hasDefault: false, min: 0, max: 2, sensitive: false, presentation: { label: "Temperature", help: "Sampling temperature from 0 to 2", editor: "number" } },
				{ name: "max_output_tokens", type: "integer", required: false, hasDefault: false, min: 1, max: 128000, sensitive: false, presentation: { label: "Max output tokens", help: "Maximum number of generated tokens", editor: "number" } },
			] },
		},
	]) {
		registry.registerDefinition(definition);
		registry.registerExecutor({ definitionId: definition.id, version: "v1" });
	}
	return registry;
}
