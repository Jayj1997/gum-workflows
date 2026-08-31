import { createBrowserWorkflowClient } from "./workflow-client.js";
import { createProductDOMView } from "./product-dom-view.js";
import { createProductShell, productStatusMessage } from "./product-shell.js";
import { createBuiltinNodeRegistry } from "./node-registry.js";
import { createWorkflowPreview } from "./workflow-preview.js";
import { createBrowserLLMSettings } from "./browser-llm-settings.js";

const workflows = [];
const drafts = new Map();
const nodeRegistry = createBuiltinNodeRegistry();
const llmSettings = createBrowserLLMSettings();

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
	async getLLMSettings() { return llmSettings.getSettings(); },
	async createLLMProvider(input) { return llmSettings.createProvider(input); },
	async updateLLMProvider(input) { return llmSettings.updateProvider(input); },
	async deleteLLMProvider(providerId) { llmSettings.deleteProvider(providerId); },
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
const providerAPIKeyRef = document.querySelector("#provider-api-key-ref");
const llmProviderList = document.querySelector("#llm-provider-list");
const llmDiagnosticList = document.querySelector("#llm-settings-diagnostics");

createProductShell(
  createProductDOMView(
		{ title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList, nodeCatalogList, nodeList, nodeEditor, nodeEditorStatus, nodeName, removeNodeButton, nodeConfigForm, nodeInputForm, nodeControlForm, previewCanvas, previewEdges, previewGroups, previewZoomIn, previewZoomOut, previewZoomReset, providerForm, providerName, providerProtocol, providerBaseURL, providerAPIKeyRef, llmProviderList, llmDiagnosticList },
    productStatusMessage,
  ),
  client,
);
