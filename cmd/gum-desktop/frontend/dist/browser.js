import { createBrowserWorkflowClient } from "./workflow-client.js";
import { createProductDOMView } from "./product-dom-view.js";
import { createProductShell, productStatusMessage } from "./product-shell.js";
import { createBuiltinNodeRegistry } from "./node-registry.js";
import { createWorkflowPreview } from "./workflow-preview.js";
import { createBrowserLLMSettings } from "./browser-llm-settings.js";
import { productRevisionKey } from "./browser-run.js";

const workflows = [];
const drafts = new Map();
const nodeRegistry = createBuiltinNodeRegistry();
const llmSettings = createBrowserLLMSettings();
const revisions = new Map();
const runs = new Map();

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

const client = createBrowserWorkflowClient({
  async openWorkspace() {
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
		draft.preview = createWorkflowPreview(draft.content, nodeRegistry);
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
			preview: createWorkflowPreview(current.content, nodeRegistry),
			saved,
			conflict,
			refreshRequired: conflict,
		};
	},
	async startRun(input) {
		const current = drafts.get(input.workflowId);
		if (!current || current.lockVersion !== input.expectedLockVersion) throw new Error("Draft lock version conflict; refresh before running");
		const preview = createWorkflowPreview(current.content, nodeRegistry);
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
				baseUrl: provider.baseUrl, modelUuid: model.id, providerModelId: model.providerModelId,
				temperature: node.config?.temperature ?? model.generationDefaults?.temperature,
				maxOutputTokens: node.config?.max_output_tokens ?? model.generationDefaults?.maxOutputTokens,
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
		if (agents.length !== 1) throw new Error("Fake executor supports exactly one LLM chat Node");
		const agent = agents[0];
		const sourceId = agent.inputs?.conversation?.from?.split(".")[0];
		const human = materialized.nodes.find((node) => node.id === sourceId && node.definition === "human-chat");
		if (!human || agent.inputs.conversation.from !== `${human.id}.conversation`) throw new Error("Fake executor requires the authored human-chat Conversation Data Edge");
		const messages = [{ role: "user", text: "Hello from the P9 fake executor." }, { role: "assistant", text: "Fake model response." }];
		const draft = structuredClone(current);
		draft.preview = createWorkflowPreview(draft.content, nodeRegistry);
		const run = {
			id: runId, revisionId, status: "succeeded", draft,
			snapshot: {
				executors: materialized.nodes.map((node) => ({ nodeId: node.id, definitionId: node.definition, version: node.executor })),
				llmSelections: selections,
				...(materialized.project ? { project: structuredClone(materialized.project) } : {}),
			},
			nodeRuns: [human, agent].map((node) => ({ id: crypto.randomUUID(), nodeId: node.id, nodeDefinition: node.definition, nodeExecutor: node.executor, status: "succeeded" })),
			artifacts: [{ id: crypto.randomUUID(), nodeId: agent.id, port: "conversation", type: "Conversation", version: "2", uri: "2.json", messages }],
		};
		runs.set(run.id, {
			workflowId: input.workflowId, revisionId: run.revisionId, id: run.id, status: run.status,
			startedAt: new Date().toISOString(), finishedAt: new Date().toISOString(),
			// The Revision content and Run Snapshot that actually ran; getRunHistory
			// replays them even after the live Draft moves on.
			revisionContent: structuredClone(materialized), snapshot: structuredClone(run.snapshot),
			nodeRuns: structuredClone(run.nodeRuns), artifacts: structuredClone(run.artifacts),
		});
		return structuredClone(run);
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
		draft.preview = createWorkflowPreview(draft.content, nodeRegistry);
		return { id: run.id, revisionId: run.revisionId, status: run.status, draft, snapshot: structuredClone(run.snapshot), nodeRuns: structuredClone(run.nodeRuns), artifacts: structuredClone(run.artifacts) };
	},
	async getLLMSettings() { return llmSettings.getSettings(); },
	async createLLMProvider(input) { return llmSettings.createProvider(input); },
	async updateLLMProvider(input) { return llmSettings.updateProvider(input); },
	async deleteLLMProvider(input) { llmSettings.deleteProvider(input); },
	async setDefaultLLMProvider(providerId) { return llmSettings.setDefaultProvider(providerId); },
	async createLLMModel(input) { return llmSettings.createModel(input); },
	async updateLLMModel(input) { return llmSettings.updateModel(input); },
	async deleteLLMModel(providerId, modelId) { llmSettings.deleteModel(providerId, modelId); },
	async setDefaultLLMModel(providerId, modelId) { return llmSettings.setDefaultModel(providerId, modelId); },
});
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
const providerBaseURL = document.querySelector("#provider-base-url");
const providerAPIKey = document.querySelector("#provider-api-key");
const llmProviderList = document.querySelector("#llm-provider-list");
const llmDiagnosticList = document.querySelector("#llm-settings-diagnostics");
const runButton = document.querySelector("#start-run");
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
		{ title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList, nodeCatalogList, nodeList, nodeEditor, nodeEditorStatus, nodeName, removeNodeButton, nodeConfigForm, nodeInputForm, nodeControlForm, previewCanvas, previewEdges, previewGroups, previewZoomIn, previewZoomOut, previewZoomReset, providerForm, providerName, providerProtocol, providerBaseURL, providerAPIKey, llmProviderList, llmDiagnosticList, runButton, runStatus, nodeRunList, artifactList, historyRefreshButton, revisionList, revisionRunList, historyRunStatus, historyNodeRunList, historyArtifactList },
    productStatusMessage,
  ),
  client,
);
