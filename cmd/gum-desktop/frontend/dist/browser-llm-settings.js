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

export function createBrowserLLMSettings(options = {}) {
	const newID = options.newID ?? (() => crypto.randomUUID());
	const now = options.now ?? (() => new Date().toISOString());
	const secrets = options.secrets ?? createMemorySecretAdapter();
	const providers = [];
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

	return {
		getSettings,
		// referenceFor returns the internal Secret reference of one Provider so
		// the fixture chat Adapter can resolve credentials like the real seam.
		referenceFor(providerID) {
			return providers.find((candidate) => candidate.id === providerID && !candidate.deleted)?.apiKeyRef;
		},
		createProvider(input) {
			const id = newID();
			const apiKeyRef = secrets.store(`llm-provider/${id}`, input.apiKey);
			const provider = { id, name: input.name, protocol: input.protocol, baseUrl: input.baseUrl, apiKeyRef, explicitDefault: false, effectiveDefault: false, createdAt: now(), models: [] };
			providers.push(provider);
			return structuredClone(providerView(provider.id));
		},
		updateProvider(input) {
			const provider = providers.find((candidate) => candidate.id === input.id && !candidate.deleted);
			if (input.apiKey) {
				const reference = secrets.store(`llm-provider/${provider.id}`, input.apiKey);
				if (reference !== provider.apiKeyRef) throw new Error("Secret Adapter changed the Provider reference");
			}
			Object.assign(provider, structuredClone({ name: input.name, protocol: input.protocol, baseUrl: input.baseUrl }));
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
		setDefaultModel(providerID, modelID) {
			const provider = providers.find((candidate) => candidate.id === providerID && !candidate.deleted);
			for (const model of provider.models) model.explicitDefault = model.id === modelID && !model.deleted;
			return structuredClone(getSettings());
		},
	};
}
