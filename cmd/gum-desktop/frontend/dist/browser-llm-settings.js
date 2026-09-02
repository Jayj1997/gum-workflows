// Browser Mock settings mirror the Product Application behavior without SQLite.
export function createMemorySecretAdapter() {
	const values = new Map();
	return {
		store(name, value) {
			if (!value) throw new Error("Secret value must not be empty");
			const reference = `memory://gum-workflows/${encodeURIComponent(name)}`;
			values.set(reference, value);
			return reference;
		},
		resolve(reference) {
			if (!values.has(reference)) throw new Error("Secret reference not found");
			return values.get(reference);
		},
		delete(reference) {
			if (!values.delete(reference)) throw new Error("Secret reference not found");
		},
	};
}

function normalizeProviderDialect(value) {
	const dialect = value?.trim() || "developer";
	if (dialect !== "developer" && dialect !== "system") throw new Error("dialect must be developer or system");
	return dialect;
}

export function createBrowserLLMSettings(options = {}) {
	const newID = options.newID ?? (() => crypto.randomUUID());
	const now = options.now ?? (() => new Date().toISOString());
	const secrets = options.secrets ?? createMemorySecretAdapter();
	const providers = [];
	const drafts = options.drafts ?? new Map();
	const workflowNames = options.workflowNames ?? new Map();
	const byCreatedAtAndId = (left, right) => left.createdAt.localeCompare(right.createdAt) || left.id.localeCompare(right.id);

	function getSettings() {
		const activeProviders = providers.filter((provider) => !provider.deleted).sort(byCreatedAtAndId);
		const effectiveProvider = activeProviders.find((provider) => provider.explicitDefault) ?? activeProviders[0];
		return {
			providers: activeProviders.map((provider) => {
				const { deleted: _deletedProvider, apiKeyRef: _apiKeyRef, ...activeProvider } = provider;
				const models = provider.models.filter((model) => !model.deleted).sort(byCreatedAtAndId);
				const effectiveModel = models.find((model) => model.explicitDefault) ?? models[0];
				return {
					...activeProvider, hasApiKey: Boolean(provider.apiKeyRef),
					models: models.map((model) => {
						const { deleted: _deletedModel, ...activeModel } = model;
						return { ...activeModel, effectiveDefault: model === effectiveModel };
					}),
					effectiveDefault: provider === effectiveProvider,
				};
			}),
			diagnostics: !effectiveProvider
				? [{ code: "llm-provider-required", severity: "error", path: "llm.providers", message: "create an LLM Provider before selecting a model" }]
				: effectiveProvider.models.every((model) => model.deleted)
					? [{ code: "llm-model-required", severity: "error", path: `llm.providers.${effectiveProvider.id}.models`, message: "create a Model Slot for the effective default Provider" }]
					: [],
		};
	}

	function providerView(providerID) {
		return getSettings().providers.find((provider) => provider.id === providerID);
	}

	function modelView(providerID, modelID) {
		return providerView(providerID)?.models.find((model) => model.id === modelID);
	}

	// deletionImpact describes which workflows' drafts reference the given Model
	// UUIDs before a deletion is confirmed. It never mutates drafts; entries
	// read the same { content: { nodes } } shape updateDraft stores.
	function deletionImpact(modelUUIDs) {
		const affected = [];
		for (const draft of drafts.values()) {
			for (const node of draft?.content?.nodes ?? []) {
				if (!node.llm?.modelUuid || !modelUUIDs.has(node.llm.modelUuid)) continue;
				affected.push({ id: draft.workflowId, displayName: workflowNames.get(draft.workflowId) ?? draft.workflowId, nodeId: node.id ?? "", nodeDefinition: node.definition ?? "", modelUuid: node.llm.modelUuid });
			}
		}
		return affected.sort((left, right) => left.id.localeCompare(right.id) || (left.nodeId ?? "").localeCompare(right.nodeId ?? "") || left.nodeDefinition.localeCompare(right.nodeDefinition) || left.modelUuid.localeCompare(right.modelUuid));
	}

	return {
		getSettings,
		// rememberWorkflow teaches the settings preview the display name of a
		// workflow created after this settings instance was constructed.
		rememberWorkflow(workflow) { workflowNames.set(workflow.id, workflow.displayName); },
		// referenceFor returns the internal Secret reference of one Provider so
		// the fixture chat Adapter can resolve credentials like the real seam.
		referenceFor(providerID) {
			return providers.find((candidate) => candidate.id === providerID && !candidate.deleted)?.apiKeyRef;
		},
			createProvider(input) {
				const dialect = normalizeProviderDialect(input.dialect);
				const id = newID();
				const apiKeyRef = secrets.store(`llm-provider/${id}`, input.apiKey);
				const provider = { id, name: input.name, protocol: input.protocol, dialect, baseUrl: input.baseUrl, apiKeyRef, explicitDefault: false, effectiveDefault: false, createdAt: now(), models: [] };
			providers.push(provider);
			return structuredClone(providerView(provider.id));
		},
			updateProvider(input) {
				const provider = providers.find((candidate) => candidate.id === input.id && !candidate.deleted);
				const dialect = normalizeProviderDialect(input.dialect);
			if (input.apiKey) {
				const reference = secrets.store(`llm-provider/${provider.id}`, input.apiKey);
				if (reference !== provider.apiKeyRef) throw new Error("Secret Adapter changed the Provider reference");
			}
				Object.assign(provider, structuredClone({ name: input.name, protocol: input.protocol, dialect, baseUrl: input.baseUrl }));
			return structuredClone(providerView(provider.id));
		},
		deleteProvider(input) {
			if (!input.confirmed) throw new Error("Provider deletion requires confirmation");
			const provider = providers.find((candidate) => candidate.id === input.providerId && !candidate.deleted);
			secrets.delete(provider.apiKeyRef);
			provider.deleted = true;
		},
		setDefaultProvider(providerID) {
			for (const provider of providers) provider.explicitDefault = provider.id === providerID && !provider.deleted;
			return structuredClone(getSettings());
		},
		createModel(input) {
			const provider = providers.find((candidate) => candidate.id === input.providerId && !candidate.deleted);
			const model = { id: newID(), generationDefaults: {}, ...structuredClone(input), explicitDefault: false, effectiveDefault: false, createdAt: now() };
			provider.models.push(model);
			return structuredClone(modelView(provider.id, model.id));
		},
		updateModel(input) {
			const provider = providers.find((candidate) => candidate.id === input.providerId && !candidate.deleted);
			const model = provider.models.find((candidate) => candidate.id === input.id && !candidate.deleted);
			Object.assign(model, structuredClone({ displayName: input.displayName, providerModelId: input.providerModelId, generationDefaults: input.generationDefaults ?? {} }));
			return structuredClone(modelView(provider.id, model.id));
		},
		deleteModel(providerID, modelID) { providers.find((provider) => provider.id === providerID).models.find((model) => model.id === modelID).deleted = true; },
		// modelDeletionImpact previews the workflows referencing one Model Slot.
		// Like the Go Application it only accepts not-yet-deleted slots.
		modelDeletionImpact(providerID, modelID) {
			const model = providers.find((provider) => provider.id === providerID)?.models.find((candidate) => candidate.id === modelID && !candidate.deleted);
			if (!model) throw new Error(`llm model ${modelID}: not found`);
			return { workflows: deletionImpact(new Set([modelID])), modelSlots: [], diagnostics: [] };
		},
		// providerDeletionImpact previews every Model Slot and referencing
		// workflow for one Provider removal.
		providerDeletionImpact(providerID) {
			const provider = providers.find((candidate) => candidate.id === providerID && !candidate.deleted);
			if (!provider) throw new Error(`llm provider ${providerID}: not found`);
			const activeModels = provider.models.filter((model) => !model.deleted);
			const modelIDs = new Set(activeModels.map((model) => model.id));
			return {
				workflows: deletionImpact(modelIDs),
				modelSlots: activeModels.map((model) => ({ id: model.id, displayName: model.displayName, providerModelId: model.providerModelId })),
				diagnostics: [],
			};
		},
		setDefaultModel(providerID, modelID) {
			const provider = providers.find((candidate) => candidate.id === providerID && !candidate.deleted);
			for (const model of provider.models) model.explicitDefault = model.id === modelID && !model.deleted;
			return structuredClone(getSettings());
		},
	};
}
