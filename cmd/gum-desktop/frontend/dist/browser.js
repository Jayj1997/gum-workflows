import { createBrowserWorkflowClient } from "./workflow-client.js";
import { createProductDOMView } from "./product-dom-view.js";
import { createProductShell, productStatusMessage } from "./product-shell.js";
import { createBuiltinNodeRegistry } from "./node-registry.js";
import { createWorkflowPreview } from "./workflow-preview.js";
import { createBrowserLLMSettings, createMemorySecretAdapter } from "./browser-llm-settings.js";
import { failBrowserRun, interruptBrowserRuns, productRevisionKey } from "./browser-run.js";
import { createFixtureChatAdapter } from "./browser-chat-fixture.js";

function normalize(value) {
	if (Array.isArray(value)) return value.map(normalize);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.keys(value).sort().map((key) => [key, normalize(value[key])]));
	}
	return value;
}

function sameContent(left, right) {
	return JSON.stringify(normalize(left)) === JSON.stringify(normalize(right));
}

// createBrowserApplication exposes the same use-case seam as Desktop while
// keeping the Browser implementation deliberately in-memory and injectable.
export function createBrowserApplication(options = {}) {
	const workflows = [];
	const drafts = new Map();
	const nodeRegistry = createBuiltinNodeRegistry();
	const secrets = options.secrets ?? createMemorySecretAdapter();
	const llmSettings = createBrowserLLMSettings({
		secrets,
		// Deletion previews read the same in-memory Drafts this mock serves,
		// mirroring the SQLite store's cross-workflow reference query.
		drafts,
		workflowNames: new Map(workflows.map((workflow) => [workflow.id, workflow.displayName])),
	});
	// Keep the settings preview in sync with workflows created later.
	const rememberWorkflow = (workflow) => llmSettings.rememberWorkflow?.(workflow);
	// liveModelUUIDs mirrors the Application's activeModelUUIDs: the Preview
	// flags dangling Model preferences using the same live settings seam.
	function liveModelUUIDs() {
		const uuids = new Set();
		for (const provider of llmSettings.getSettings().providers) {
			for (const model of provider.models) uuids.add(model.id);
		}
		return uuids;
	}
	const revisions = new Map();
	const runs = new Map();
	const chatAdapter = options.chatAdapter ?? createFixtureChatAdapter({ secrets });
	let initialized = false;

	return {
  async openWorkspace() {
		if (!initialized) {
			interruptBrowserRuns(runs);
			initialized = true;
		}
    return {
      title: "Gum Workflows",
      message: "Product application round-trip complete",
    };
  },
  async createWorkflow({ displayName }) {
    const workflow = {
      id: crypto.randomUUID(),
      displayName,
      createdAt: new Date().toISOString(),
    };
    workflows.push(workflow);
		rememberWorkflow(workflow);
		drafts.set(workflow.id, {
			workflowId: workflow.id,
			content: { semanticSchemaVersion: "productWorkflow/v1", nodes: [] },
			lockVersion: 1,
			updatedAt: workflow.createdAt,
		});
    return workflow;
  },
  async listWorkflows() {
    return structuredClone(workflows);
  },
	async listNodeCatalog() { return nodeRegistry.catalog(); },
	async getDraft(workflowId) {
		const draft = structuredClone(drafts.get(workflowId));
		draft.preview = createWorkflowPreview(draft.content, nodeRegistry, { modelUUIDs: liveModelUUIDs() });
		return draft;
	},
	async updateDraft(input) {
		const current = drafts.get(input.workflowId);
		let saved = false;
		let conflict = false;
		if (!sameContent(current.content, input.content) && current.lockVersion !== input.expectedLockVersion) {
			conflict = true;
		} else if (!sameContent(current.content, input.content)) {
			current.content = structuredClone(input.content);
			current.lockVersion += 1;
			current.updatedAt = new Date().toISOString();
			saved = true;
		}
		return {
			draft: structuredClone(current),
			preview: createWorkflowPreview(current.content, nodeRegistry, { modelUUIDs: liveModelUUIDs() }),
			saved,
			conflict,
			refreshRequired: conflict,
		};
	},
	async startRun(input) {
		const current = drafts.get(input.workflowId);
		if (!current || current.lockVersion !== input.expectedLockVersion) throw new Error("Draft lock version conflict; refresh before running");
		const preview = createWorkflowPreview(current.content, nodeRegistry, { modelUUIDs: liveModelUUIDs() });
		if (preview.diagnostics.length > 0) throw new Error("Draft has diagnostics; fix the highlighted fields before running");
		const settings = llmSettings.getSettings();
		const defaultProvider = settings.providers.find((provider) => provider.effectiveDefault);
		const defaultModel = defaultProvider?.models.find((model) => model.effectiveDefault);
		const materialized = structuredClone(current.content);
		const selections = [];
		for (const node of materialized.nodes) {
			if (node.definition !== "llm-chat") continue;
			const selected = node.llm?.modelUuid;
			const provider = selected
				? settings.providers.find((candidate) => candidate.models.some((model) => model.id === selected))
				: defaultProvider;
			const model = selected ? provider?.models.find((candidate) => candidate.id === selected) : defaultModel;
			if (!provider || !model) throw new Error(`Node ${node.id} has no available Model Slot`);
			node.llm = { modelUuid: model.id };
			selections.push({
				nodeId: node.id, providerId: provider.id, providerName: provider.name, protocol: provider.protocol,
				baseUrl: provider.baseUrl, dialect: provider.dialect, modelUuid: model.id, providerModelId: model.providerModelId,
				temperature: node.config?.temperature ?? model.generationDefaults?.temperature,
				maxOutputTokens: node.config?.max_output_tokens ?? model.generationDefaults?.maxOutputTokens,
				apiKeyRef: llmSettings.referenceFor(provider.id),
			});
		}
		if (!sameContent(current.content, materialized)) {
			current.content = materialized;
			current.lockVersion += 1;
			current.updatedAt = new Date().toISOString();
		}
		const key = productRevisionKey(materialized);
		const revisionId = revisions.get(key) ?? crypto.randomUUID();
		revisions.set(key, revisionId);
		const runId = crypto.randomUUID();
		const agents = materialized.nodes.filter((node) => node.definition === "llm-chat");
		if (agents.length !== 1) throw new Error("Single-turn executor supports exactly one LLM chat Node");
		const agent = agents[0];
		const sourceId = agent.inputs?.conversation?.from?.split(".")[0];
		const human = materialized.nodes.find((node) => node.id === sourceId && node.definition === "human-chat");
		if (!human || agent.inputs.conversation.from !== `${human.id}.conversation`) throw new Error("Single-turn executor requires the authored human-chat Conversation Data Edge");
		if (input.humanInput?.nodeId !== human.id || !input.humanInput.text?.trim()) throw new Error(`Single-turn executor requires submitted text for ${human.id}`);
		const selection = selections.find((candidate) => candidate.nodeId === agent.id);
		const startedAt = new Date().toISOString();
		const humanRunId = crypto.randomUUID();
		const agentRunId = crypto.randomUUID();
		const sourceArtifactId = crypto.randomUUID();
		const sourceRef = { id: sourceArtifactId, kind: "Conversation", version: "1", uri: "1.json" };
		const sourceMessages = [{ role: "user", text: input.humanInput.text }];
		const draft = structuredClone(current);
		// The materialized Draft re-checks against live settings: its UUID was
		// just resolved, so a dangling diagnostic here would contradict the
		// successful materialization above.
		draft.preview = createWorkflowPreview(draft.content, nodeRegistry, { modelUUIDs: liveModelUUIDs() });
		const snapshot = {
				executors: materialized.nodes.map((node) => ({ nodeId: node.id, definitionId: node.definition, version: node.executor })),
				llmSelections: selections.map(({ apiKeyRef: _apiKeyRef, ...selectionView }) => selectionView),
				...(materialized.project ? { project: structuredClone(materialized.project) } : {}),
		};
		const running = {
			workflowId: input.workflowId, revisionId, id: runId, status: "running", startedAt, finishedAt: startedAt,
			// The Revision content and Run Snapshot that actually ran; getRunHistory
			// replays them even after the live Draft moves on.
			revisionContent: structuredClone(materialized), snapshot: structuredClone(snapshot),
			nodeRuns: [
				{ id: humanRunId, nodeId: human.id, nodeDefinition: human.definition, nodeExecutor: human.executor, status: "succeeded", inputs: {}, outputs: { conversation: sourceRef }, startedAt, finishedAt: startedAt },
				{ id: agentRunId, nodeId: agent.id, nodeDefinition: agent.definition, nodeExecutor: agent.executor, status: "running", inputs: { conversation: sourceRef }, outputs: {}, startedAt, finishedAt: startedAt },
			],
			artifacts: [{ id: sourceArtifactId, nodeId: human.id, port: "conversation", type: "Conversation", version: "1", uri: "1.json", messages: sourceMessages }],
		};
		runs.set(runId, running);

		let result;
		try {
			// Await also supports asynchronous fixture adapters, making the
			// in-flight Running state observable through the Browser seam.
			result = await chatAdapter.generate(
				{ protocol: selection.protocol, dialect: selection.dialect, baseUrl: selection.baseUrl, providerModelId: selection.providerModelId, apiKeyRef: selection.apiKeyRef },
				{
					model: selection.providerModelId,
					instructions: agent.config?.instructions ? [{ kind: "text", text: agent.config.instructions }] : [],
					messages: [{ role: "user", parts: [{ kind: "text", text: input.humanInput.text }] }],
					config: {
						temperature: selection.temperature ?? undefined,
						maxOutputTokens: selection.maxOutputTokens ?? undefined,
					},
				},
			);
		} catch (error) {
			const apiKey = secrets.resolve(selection.apiKeyRef);
			const message = String(error?.message ?? error).split(apiKey).join("[REDACTED]");
			const details = { kind: "structural", code: "provider", message, userAction: "review the Provider settings and start a new Run" };
			failBrowserRun(running, details);
			throw new Error(`run ${runId} ${details.kind}/${details.code}: ${details.message}; ${details.userAction}`, { cause: error });
		}

		const finishedAt = new Date().toISOString();
		if (running.status !== "running") {
			throw new Error(`run ${runId} cannot finalize because its status is ${running.status}`);
		}
		const messages = [...sourceMessages, { role: "assistant", text: result.assistant.parts.map((part) => part.text).join("\n") }];
		const finalArtifactId = crypto.randomUUID();
		const finalRef = { id: finalArtifactId, kind: "Conversation", version: "2", uri: "2.json" };
		running.status = "succeeded";
		running.finishedAt = finishedAt;
		running.nodeRuns[1] = {
			...running.nodeRuns[1], status: "succeeded", outputs: { conversation: finalRef }, finishedAt,
			diagnostics: { providerRequestId: result.providerRequestId, finishReason: result.finishReason, usage: result.usage },
		};
		running.artifacts.push({ id: finalArtifactId, nodeId: agent.id, port: "conversation", type: "Conversation", version: "2", uri: "2.json", messages });
		return structuredClone({
			id: runId, revisionId, status: running.status, startedAt, finishedAt, draft, snapshot,
			nodeRuns: running.nodeRuns, artifacts: running.artifacts,
		});
	},
	async listRevisions(workflowId) {
		const counts = new Map();
		for (const run of runs.values()) {
			if (run.workflowId !== workflowId) continue;
			counts.set(run.revisionId, { count: (counts.get(run.revisionId)?.count ?? 0) + 1, createdAt: run.startedAt });
		}
		const seen = new Map();
		for (const [key, revisionId] of revisions) {
			const info = counts.get(revisionId);
			if (!info || seen.has(revisionId)) continue;
			seen.set(revisionId, { id: revisionId, semanticHash: key, runCount: info.count, createdAt: info.createdAt });
		}
		return [...seen.values()];
	},
	async listRevisionRuns(revisionId) {
		return [...runs.values()].filter((run) => run.revisionId === revisionId).map((run) => ({
			id: run.id, revisionId: run.revisionId, status: run.status, startedAt: run.startedAt, finishedAt: run.finishedAt,
		}));
	},
	async getRunHistory(runId) {
		const run = runs.get(runId);
		if (!run) throw new Error(`Run ${runId} not found`);
		const draft = {
			workflowId: run.workflowId,
			content: structuredClone(run.revisionContent),
			lockVersion: 0,
			updatedAt: run.startedAt,
		};
		// Historical Revisions keep the Model UUID that ran; the Run Snapshot
		// stays authoritative for the resolved selection, so the historical
		// Preview deliberately skips live-settings checks.
		draft.preview = createWorkflowPreview(draft.content, nodeRegistry);
		return {
			id: run.id, revisionId: run.revisionId, status: run.status,
			...(run.error ? { error: structuredClone(run.error) } : {}),
			startedAt: run.startedAt, finishedAt: run.finishedAt,
			draft, snapshot: structuredClone(run.snapshot), nodeRuns: structuredClone(run.nodeRuns), artifacts: structuredClone(run.artifacts),
		};
	},
	async getLLMSettings() { return llmSettings.getSettings(); },
	async createLLMProvider(input) { return llmSettings.createProvider(input); },
	async updateLLMProvider(input) { return llmSettings.updateProvider(input); },
	async deleteLLMProvider(input) { llmSettings.deleteProvider(input); },
	async listProviderDeletionImpact(providerId) { return llmSettings.providerDeletionImpact(providerId); },
	async setDefaultLLMProvider(providerId) { return llmSettings.setDefaultProvider(providerId); },
	async createLLMModel(input) { return llmSettings.createModel(input); },
	async updateLLMModel(input) { return llmSettings.updateModel(input); },
	async deleteLLMModel(providerId, modelId) { llmSettings.deleteModel(providerId, modelId); },
	async listModelDeletionImpact(providerId, modelId) { return llmSettings.modelDeletionImpact(providerId, modelId); },
	async setDefaultLLMModel(providerId, modelId) { return llmSettings.setDefaultModel(providerId, modelId); },
	};
}

const client = createBrowserWorkflowClient(createBrowserApplication());

if (typeof document !== "undefined") {
const title = document.querySelector("#title");
const message = document.querySelector("#message");
const status = document.querySelector("#status");
const button = document.querySelector("#open-workspace");
const form = document.querySelector("#create-workflow");
const nameInput = document.querySelector("#workflow-name");
const workflowList = document.querySelector("#workflow-list");
const draftEditor = document.querySelector("#draft-content");
const draftStatus = document.querySelector("#draft-status");
const diagnosticList = document.querySelector("#draft-diagnostics");
const nodeCatalogList = document.querySelector("#node-catalog");
const nodeList = document.querySelector("#node-list");
const nodeEditor = document.querySelector("#node-editor");
const nodeEditorStatus = document.querySelector("#node-editor-status");
const nodeName = document.querySelector("#node-name");
const removeNodeButton = document.querySelector("#remove-node");
const nodeConfigForm = document.querySelector("#node-config-form");
const nodeInputForm = document.querySelector("#node-input-form");
const nodeControlForm = document.querySelector("#node-control-form");
const previewCanvas = document.querySelector("#preview-canvas");
const previewEdges = document.querySelector("#preview-edges");
const previewGroups = document.querySelector("#preview-groups");
const previewZoomIn = document.querySelector("#preview-zoom-in");
const previewZoomOut = document.querySelector("#preview-zoom-out");
const previewZoomReset = document.querySelector("#preview-zoom-reset");
const providerForm = document.querySelector("#create-provider");
const providerName = document.querySelector("#provider-name");
const providerProtocol = document.querySelector("#provider-protocol");
const providerDialect = document.querySelector("#provider-dialect");
const providerBaseURL = document.querySelector("#provider-base-url");
const providerAPIKey = document.querySelector("#provider-api-key");
const llmProviderList = document.querySelector("#llm-provider-list");
const llmDiagnosticList = document.querySelector("#llm-settings-diagnostics");
const runButton = document.querySelector("#start-run");
const runInputLabel = document.querySelector("#run-input-label");
const runInput = document.querySelector("#run-input");
const runStatus = document.querySelector("#run-status");
const nodeRunList = document.querySelector("#node-run-list");
const artifactList = document.querySelector("#artifact-list");
const historyRefreshButton = document.querySelector("#history-refresh");
const revisionList = document.querySelector("#revision-list");
const revisionRunList = document.querySelector("#revision-run-list");
const historyRunStatus = document.querySelector("#history-run-status");
const historyNodeRunList = document.querySelector("#history-node-run-list");
const historyArtifactList = document.querySelector("#history-artifact-list");

createProductShell(
  createProductDOMView(
		{ title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList, nodeCatalogList, nodeList, nodeEditor, nodeEditorStatus, nodeName, removeNodeButton, nodeConfigForm, nodeInputForm, nodeControlForm, previewCanvas, previewEdges, previewGroups, previewZoomIn, previewZoomOut, previewZoomReset, providerForm, providerName, providerProtocol, providerDialect, providerBaseURL, providerAPIKey, llmProviderList, llmDiagnosticList, runButton, runInputLabel, runInput, runStatus, nodeRunList, artifactList, historyRefreshButton, revisionList, revisionRunList, historyRunStatus, historyNodeRunList, historyArtifactList },
    productStatusMessage,
  ),
  client,
);
}
