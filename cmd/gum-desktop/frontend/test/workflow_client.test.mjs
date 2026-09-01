import assert from "node:assert/strict";
import test from "node:test";

import {
  createBrowserWorkflowClient,
  createDesktopWorkflowClient,
} from "../dist/workflow-client.js";
import { createProductDOMView } from "../dist/product-dom-view.js";
import { createProductShell, productStatusMessage } from "../dist/product-shell.js";
import { createBuiltinNodeRegistry, validateConfig } from "../dist/node-registry.js";
import { createWorkflowPreview } from "../dist/workflow-preview.js";
import { createBrowserLLMSettings } from "../dist/browser-llm-settings.js";
import { productRevisionKey } from "../dist/browser-run.js";

const expectedView = {
  title: "Gum Workflows",
  message: "Product application round-trip complete",
};
const expectedWorkflow = {
  id: "0198fb41-43d2-7e2b-a4cd-2bc5f7889ff9",
  displayName: "Release checklist",
  createdAt: "2026-08-31T09:00:00Z",
};
const expectedDraft = {
  workflowId: expectedWorkflow.id,
  content: { semanticSchemaVersion: "productWorkflow/v1", nodes: [] },
  lockVersion: 1,
  updatedAt: "2026-08-31T09:00:00Z",
};
const expectedRun = {
	id: "run-uuid", revisionId: "revision-uuid", status: "succeeded", draft: expectedDraft,
	nodeRuns: [
		{ id: "human-run", nodeId: "prompt", nodeDefinition: "human-chat", nodeExecutor: "v1", status: "succeeded" },
		{ id: "agent-run", nodeId: "answer", nodeDefinition: "llm-chat", nodeExecutor: "v1", status: "succeeded" },
	],
	artifacts: [{ id: "artifact-uuid", nodeId: "answer", port: "conversation", type: "Conversation", version: "2", uri: "2.json", messages: [{ role: "user", text: "Hello" }, { role: "assistant", text: "Fake response" }] }],
};
const expectedRevisions = [
	{ id: "revision-uuid", semanticHash: "abc12345", runCount: 1, createdAt: "2026-08-31T09:00:00Z" },
];
const expectedRevisionRuns = [
	{ id: "run-uuid", revisionId: "revision-uuid", status: "succeeded", startedAt: "2026-08-31T09:00:00Z", finishedAt: "2026-08-31T09:00:01Z" },
];
const expectedSettings = {
	providers: [{
		id: "provider-uuid", name: "Primary", protocol: "openai-chat-completions", baseUrl: "https://api.example/v1",
		apiKeyRef: "keychain://primary", explicitDefault: true, effectiveDefault: true, createdAt: "2026-08-31T09:00:00Z",
		models: [{ id: "model-uuid", providerId: "provider-uuid", displayName: "Fast", providerModelId: "model-fast", generationDefaults: { temperature: 0.2, maxOutputTokens: 1024 }, explicitDefault: true, effectiveDefault: true, createdAt: "2026-08-31T09:00:00Z" }],
	}],
	diagnostics: [],
};
const expectedCatalog = [
	{
		definition: {
			id: "llm-chat", displayName: "LLM chat", description: "Append one model response", kind: "agent",
			config: { fields: [
				{ name: "instructions", type: "markdown", required: false, hasDefault: false, sensitive: false, presentation: { label: "Instructions", help: "Model guidance", editor: "markdown" } },
				{ name: "temperature", type: "number", required: false, hasDefault: false, min: 0, max: 2, sensitive: false, presentation: { label: "Temperature", help: "Sampling", editor: "number" } },
			] },
		},
		executor: { definitionId: "llm-chat", version: "v1" },
	},
];

const clientContract = [
  [
    "browser mock",
    () =>
      createBrowserWorkflowClient({
        async openWorkspace() {
          return expectedView;
        },
        async createWorkflow(input) {
          return { ...expectedWorkflow, displayName: input.displayName };
        },
        async listWorkflows() {
          return [expectedWorkflow];
        },
		async getDraft() {
			return expectedDraft;
		},
		async updateDraft(input) {
			return { draft: { ...expectedDraft, ...input, lockVersion: 2 }, preview: { nodes: [], edges: [], groups: [], diagnostics: [] }, saved: true, conflict: false, refreshRequired: false };
		},
		async startRun() { return expectedRun; },
		async listRevisions() { return expectedRevisions; },
		async listRevisionRuns() { return expectedRevisionRuns; },
		async getRunHistory() { return expectedRun; },
		async listNodeCatalog() { return expectedCatalog; },
		async getLLMSettings() { return expectedSettings; },
		async createLLMProvider(input) { return { ...expectedSettings.providers[0], ...input }; },
		async updateLLMProvider(input) { return { ...expectedSettings.providers[0], ...input }; },
		async deleteLLMProvider() {}, async setDefaultLLMProvider() { return expectedSettings; },
		async createLLMModel(input) { return { ...expectedSettings.providers[0].models[0], ...input }; },
		async updateLLMModel(input) { return { ...expectedSettings.providers[0].models[0], ...input }; },
		async deleteLLMModel() {}, async setDefaultLLMModel() { return expectedSettings; },
      }),
  ],
  [
    "desktop adapter",
    () =>
      createDesktopWorkflowClient({
        async OpenWorkspace() {
          return expectedView;
        },
        async CreateWorkflow(input) {
          return { ...expectedWorkflow, displayName: input.displayName };
        },
        async ListWorkflows() {
          return [expectedWorkflow];
        },
		async GetDraft() {
			return expectedDraft;
		},
		async UpdateDraft(input) {
			return { draft: { ...expectedDraft, ...input, lockVersion: 2 }, preview: { nodes: [], edges: [], groups: [], diagnostics: [] }, saved: true, conflict: false, refreshRequired: false };
		},
		async StartRun() { return expectedRun; },
		async ListRevisions() { return expectedRevisions; },
		async ListRevisionRuns() { return expectedRevisionRuns; },
		async GetRunHistory() { return expectedRun; },
		async ListNodeCatalog() { return expectedCatalog; },
		async GetLLMSettings() { return expectedSettings; },
		async CreateLLMProvider(input) { return { ...expectedSettings.providers[0], ...input }; },
		async UpdateLLMProvider(input) { return { ...expectedSettings.providers[0], ...input }; },
		async DeleteLLMProvider() {}, async SetDefaultLLMProvider() { return expectedSettings; },
		async CreateLLMModel(input) { return { ...expectedSettings.providers[0].models[0], ...input }; },
		async UpdateLLMModel(input) { return { ...expectedSettings.providers[0].models[0], ...input }; },
		async DeleteLLMModel() {}, async SetDefaultLLMModel() { return expectedSettings; },
      }),
  ],
];

for (const [name, createClient] of clientContract) {
  test(`${name} follows the WorkflowClient contract`, async () => {
	const client = createClient();
	assert.deepEqual(await client.openWorkspace(), expectedView);
	assert.deepEqual(await client.createWorkflow({ displayName: "Release checklist" }), expectedWorkflow);
	assert.deepEqual(await client.listWorkflows(), [expectedWorkflow]);
	assert.deepEqual(await client.getDraft(expectedWorkflow.id), expectedDraft);
	assert.equal((await client.updateDraft({ workflowId: expectedWorkflow.id, expectedLockVersion: 1, content: expectedDraft.content })).draft.lockVersion, 2);
	assert.deepEqual(await client.startRun({ workflowId: expectedWorkflow.id, expectedLockVersion: 1 }), expectedRun);
	assert.deepEqual(await client.listRevisions(expectedWorkflow.id), expectedRevisions);
	assert.deepEqual(await client.listRevisionRuns("revision-uuid"), expectedRevisionRuns);
	assert.deepEqual(await client.getRunHistory("run-uuid"), expectedRun);
	assert.deepEqual(await client.listNodeCatalog(), expectedCatalog);
	assert.deepEqual(await client.getLLMSettings(), expectedSettings);
	assert.equal((await client.createLLMProvider({ name: "Primary" })).name, "Primary");
	assert.equal((await client.updateLLMProvider({ id: "provider-uuid", name: "Renamed" })).name, "Renamed");
	await client.deleteLLMProvider("provider-uuid");
	assert.deepEqual(await client.setDefaultLLMProvider("provider-uuid"), expectedSettings);
	assert.equal((await client.createLLMModel({ providerId: "provider-uuid", displayName: "Fast" })).displayName, "Fast");
	assert.equal((await client.updateLLMModel({ id: "model-uuid", providerId: "provider-uuid", displayName: "Strong" })).displayName, "Strong");
	await client.deleteLLMModel("provider-uuid", "model-uuid");
	assert.deepEqual(await client.setDefaultLLMModel("provider-uuid", "model-uuid"), expectedSettings);
  });
}

test("Run flushes pending autosave and uses the latest Draft lock token", async () => {
	let selectWorkflow, startRun;
	let draft = structuredClone(expectedDraft);
	const calls = [];
	const renderedRuns = [];
	const view = {
		onOpenWorkspace() {}, onCreateWorkflow() {}, onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty() {}, onEditDraft() {}, onStartRun(handler) { startRun = handler; },
		async flushDraftEdit() {
			calls.push("flush");
			draft = { ...draft, lockVersion: 2 };
			return { draft };
		},
		render() {}, renderDraft() {}, renderDraftLoading() {}, renderNodeEditor() {},
		renderRun(run) { renderedRuns.push(structuredClone(run)); },
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; }, async createWorkflow() {}, async listWorkflows() { return []; },
		async getDraft() { return structuredClone(draft); }, async updateDraft() {},
		async startRun(input) {
			calls.push(["start", input.expectedLockVersion]);
			return { ...structuredClone(expectedRun), draft: structuredClone(draft) };
		},
	});
	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	await startRun();

	assert.deepEqual(calls, ["flush", ["start", 2]]);
	assert.equal(renderedRuns.at(-1).artifacts[0].messages[1].text, "Fake response");
});

test("History drill-down loads Revisions after a Run, then Runs and the Run detail", async () => {
	let selectWorkflow, startRun, selectRevision, selectRun, refreshRevisions;
	const renderedRevisions = [];
	const renderedRevisionRuns = [];
	const renderedHistoryRuns = [];
	const view = {
		onOpenWorkspace() {}, onCreateWorkflow() {}, onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty() {}, onEditDraft() {}, onStartRun(handler) { startRun = handler; },
		onRefreshRevisions(handler) { refreshRevisions = handler; },
		onSelectRevision(handler) { selectRevision = handler; },
		onSelectRun(handler) { selectRun = handler; },
		render() {}, renderDraft() {}, renderDraftLoading() {}, renderNodeEditor() {},
		renderRevisions(revisions) { renderedRevisions.push(structuredClone(revisions)); },
		renderRevisionRuns(runs) { renderedRevisionRuns.push(structuredClone(runs)); },
		renderHistoryRun(run) { renderedHistoryRuns.push(structuredClone(run)); },
	};
	const runResult = { ...structuredClone(expectedRun), draft: structuredClone(expectedDraft) };
	const revisionRunListCalls = [];
	const runHistoryCalls = [];
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; }, async createWorkflow() {}, async listWorkflows() { return []; },
		async getDraft() { return structuredClone(expectedDraft); }, async updateDraft() {},
		async startRun() { return structuredClone(runResult); },
		async listRevisions(workflowId) { return [{ ...expectedRevisions[0], id: expectedRun.revisionId, runCount: 2 }]; },
		async listRevisionRuns(revisionId) { revisionRunListCalls.push(revisionId); return structuredClone(expectedRevisionRuns); },
		async getRunHistory(runId) { runHistoryCalls.push(runId); return structuredClone(runResult); },
	});
	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	await startRun();
	refreshRevisions();
	await selectRevision("revision-uuid");
	await selectRun("run-uuid");

	// Revisions render on workflow selection and again after a successful Run.
	assert.deepEqual(renderedRevisions.at(-1), [{ ...expectedRevisions[0], id: expectedRun.revisionId, runCount: 2 }]);
	assert.deepEqual(revisionRunListCalls, ["revision-uuid"]);
	assert.deepEqual(renderedRevisionRuns.at(-1), expectedRevisionRuns);
	assert.deepEqual(runHistoryCalls, ["run-uuid"]);
	assert.equal(renderedHistoryRuns.at(-1).artifacts[0].messages[1].text, "Fake response");
});

test("Browser Mock settings use UUID tie-breaks and truthful mutation defaults", () => {
	const ids = ["bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "cccccccc-cccc-4ccc-8ccc-cccccccccccc"];
	const settings = createBrowserLLMSettings({ newID: () => ids.shift(), now: () => "2026-09-01T00:00:00Z" });
	const laterProvider = settings.createProvider({ name: "Later UUID", protocol: "openai-chat-completions", baseUrl: "https://later.example/v1", apiKeyRef: "keychain://later" });
	const earlierProvider = settings.createProvider({ name: "Earlier UUID", protocol: "openai-chat-completions", baseUrl: "https://earlier.example/v1", apiKeyRef: "keychain://earlier" });
	assert.equal(laterProvider.effectiveDefault, true);
	assert.equal(earlierProvider.effectiveDefault, true);
	assert.equal(settings.updateProvider({ ...laterProvider, name: "Later renamed" }).effectiveDefault, false);
	const laterModel = settings.createModel({ providerId: earlierProvider.id, displayName: "Later UUID", providerModelId: "later", generationDefaults: {} });
	const earlierModel = settings.createModel({ providerId: earlierProvider.id, displayName: "Earlier UUID", providerModelId: "earlier", generationDefaults: {} });
	assert.equal(laterModel.effectiveDefault, true);
	assert.equal(earlierModel.effectiveDefault, true);
	assert.equal(settings.updateModel({ ...laterModel, displayName: "Later renamed" }).effectiveDefault, false);
	assert.equal(settings.getSettings().providers[0].id, earlierProvider.id);
	assert.equal(settings.getSettings().providers[0].models[0].id, earlierModel.id);
});

test("Browser Mock Revision identity ignores presentation and unordered storage", () => {
	const left = {
		semanticSchemaVersion: "productWorkflow/v1", displayName: "Left", view: { zoom: 1 },
		nodes: [
			{ id: "b", definition: "llm-chat", executor: "v1", displayName: "Writer", presentation: { x: 10 }, dependsOn: ["z", "a"], config: {} },
			{ id: "a", definition: "human-chat", executor: "v1", displayName: "Prompt", config: {} },
		],
	};
	const right = {
		nodes: [
			{ config: {}, displayName: "Renamed prompt", executor: "v1", definition: "human-chat", id: "a" },
			{ config: {}, dependsOn: ["a", "z"], presentation: { x: 999 }, displayName: "Renamed writer", executor: "v1", definition: "llm-chat", id: "b" },
		],
		view: { zoom: 2 }, displayName: "Right", semanticSchemaVersion: "productWorkflow/v1",
	};
	assert.equal(productRevisionKey(left), productRevisionKey(right));
});

test("the Browser Mock registry validates every Gum Config Schema field type", () => {
	assert.deepEqual(createBuiltinNodeRegistry().catalog().map((entry) => entry.definition.id), ["human-chat", "llm-chat"]);
	const schema = { fields: [
		{ name: "text", type: "string", required: true, hasDefault: false },
		{ name: "markdown", type: "markdown", required: false, hasDefault: false },
		{ name: "integer", type: "integer", required: false, hasDefault: false, min: 1, max: 2 },
		{ name: "number", type: "number", required: false, hasDefault: false, min: 0, max: 1 },
		{ name: "boolean", type: "boolean", required: false, hasDefault: false },
		{ name: "enum", type: "enum", required: false, hasDefault: false, values: ["one", "two"] },
	] };
	const issues = validateConfig(schema, { markdown: 42, integer: 1.5, number: 2, boolean: "yes", enum: "three", extra: true });
	assert.deepEqual(issues.map((item) => item.field), ["text", "markdown", "integer", "number", "boolean", "enum", "extra"]);
});

test("the Browser Mock derives the same typed Data and Control Preview", () => {
	const registry = createBuiltinNodeRegistry();
	const preview = createWorkflowPreview({
		semanticSchemaVersion: "productWorkflow/v1",
		nodes: [
			{ id: "prompt", definition: "human-chat", executor: "v1", displayName: "Prompt", config: {}, dependsOn: ["answer"] },
			{ id: "answer", definition: "llm-chat", executor: "v1", displayName: "Answer", config: {}, inputs: { conversation: { from: "prompt.conversation" } } },
		],
	}, registry);
	assert.deepEqual(preview.edges.map((edge) => edge.kind), ["data", "control"]);
	assert.deepEqual(preview.groups, [{ nodeIds: ["answer", "prompt"] }]);
	assert.deepEqual(preview.diagnostics, []);

	const incomplete = createWorkflowPreview({
		semanticSchemaVersion: "productWorkflow/v1",
		nodes: [
			{ id: "prompt", definition: "human-chat", config: {} },
			{ id: "future", definition: "future-node", config: {}, inputs: { conversation: { from: "prompt.conversation" } } },
		],
	}, registry);
	assert.equal(incomplete.edges.length, 1);
	assert.equal(incomplete.edges[0].targetNodeId, "future");
	assert.deepEqual(incomplete.diagnostics.map((item) => item.code), ["unknown-node-definition"]);
});

test("a user authors Node Instances and config through the registered Catalog", async () => {
	let openWorkspace;
	let selectWorkflow;
	let addNode;
	let selectNode;
	let renameNode;
	let editNodeConfig;
	let removeNode;
	const updates = [];
	const renderedEditors = [];
	let draft = structuredClone(expectedDraft);
	const view = {
		onOpenWorkspace(handler) { openWorkspace = handler; },
		onCreateWorkflow() {},
		onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty() {},
		onEditDraft() {},
		onAddNode(handler) { addNode = handler; },
		onSelectNode(handler) { selectNode = handler; },
		onRenameNode(handler) { renameNode = handler; },
		onEditNodeConfig(handler) { editNodeConfig = handler; },
		onRemoveNode(handler) { removeNode = handler; },
		render() {}, renderWorkflows() {}, renderDraft() {}, renderDraftLoading() {}, renderNodeCatalog() {},
		renderNodeEditor(state) { renderedEditors.push(structuredClone(state)); },
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; },
		async createWorkflow() { return expectedWorkflow; },
		async listWorkflows() { return [expectedWorkflow]; },
		async listNodeCatalog() { return expectedCatalog; },
		async getDraft() { return structuredClone(draft); },
		async updateDraft(input) {
			updates.push(structuredClone(input));
			draft = { ...draft, content: structuredClone(input.content), lockVersion: draft.lockVersion + 1 };
			return { draft: structuredClone(draft), preview: { nodes: draft.content.nodes, edges: [], groups: [], diagnostics: [] }, saved: true, conflict: false, refreshRequired: false };
		},
	});
	createProductShell(view, client, { createNodeId: () => "node-uuid" });
	await openWorkspace();
	await selectWorkflow(expectedWorkflow.id);
	await addNode("llm-chat");

	assert.deepEqual(updates.at(-1).content.nodes[0], {
		id: "node-uuid", definition: "llm-chat", executor: "v1", displayName: "LLM chat", config: {},
	});
	selectNode("node-uuid");
	assert.equal(renderedEditors.at(-1).fields[0].presentation.label, "Instructions");
	const rename = renameNode({ nodeId: "node-uuid", displayName: "Writer" });
	const configure = editNodeConfig({ nodeId: "node-uuid", field: "temperature", value: 0.8 });
	await Promise.all([rename, configure]);
	assert.equal(updates.at(-1).content.nodes[0].config.temperature, 0.8);
	assert.equal(updates.at(-1).content.nodes[0].displayName, "Writer");
	await removeNode("node-uuid");
	assert.deepEqual(updates.at(-1).content.nodes, []);
});

test("a user manages Provider and Model Slots through the shared product shell", async () => {
	let openWorkspace, createProvider, updateProvider, deleteProvider, setDefaultProvider;
	let createModel, updateModel, deleteModel, setDefaultModel;
	const calls = [];
	const rendered = [];
	const view = {
		onOpenWorkspace(handler) { openWorkspace = handler; }, onCreateWorkflow() {}, onSelectWorkflow() {}, onDraftDirty() {}, onEditDraft() {},
		onCreateLLMProvider(handler) { createProvider = handler; }, onUpdateLLMProvider(handler) { updateProvider = handler; },
		onDeleteLLMProvider(handler) { deleteProvider = handler; }, onSetDefaultLLMProvider(handler) { setDefaultProvider = handler; },
		onCreateLLMModel(handler) { createModel = handler; }, onUpdateLLMModel(handler) { updateModel = handler; },
		onDeleteLLMModel(handler) { deleteModel = handler; }, onSetDefaultLLMModel(handler) { setDefaultModel = handler; },
		render() {}, renderWorkflows() {}, renderNodeCatalog() {}, renderLLMSettings(settings) { rendered.push(structuredClone(settings)); },
	};
	const settings = structuredClone(expectedSettings);
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; }, async listWorkflows() { return []; }, async listNodeCatalog() { return []; },
		async createWorkflow() {}, async getDraft() {}, async updateDraft() {}, async getLLMSettings() { calls.push("list"); return settings; },
		async createLLMProvider(input) { calls.push(["create-provider", input]); }, async updateLLMProvider(input) { calls.push(["update-provider", input]); },
		async deleteLLMProvider(id) { calls.push(["delete-provider", id]); }, async setDefaultLLMProvider(id) { calls.push(["default-provider", id]); },
		async createLLMModel(input) { calls.push(["create-model", input]); }, async updateLLMModel(input) { calls.push(["update-model", input]); },
		async deleteLLMModel(providerId, id) { calls.push(["delete-model", providerId, id]); },
		async setDefaultLLMModel(providerId, id) { calls.push(["default-model", providerId, id]); },
	});
	createProductShell(view, client);
	await openWorkspace();
	await createProvider({ name: "Primary", protocol: "openai-chat-completions", baseUrl: "https://api.example/v1", apiKeyRef: "keychain://primary" });
	await updateProvider({ ...settings.providers[0], name: "Renamed" });
	await setDefaultProvider("provider-uuid");
	await createModel({ providerId: "provider-uuid", displayName: "Fast", providerModelId: "model-fast" });
	await updateModel({ ...settings.providers[0].models[0], providerModelId: "model-fast-v2" });
	await setDefaultModel("provider-uuid", "model-uuid");
	await deleteModel("provider-uuid", "model-uuid");
	await deleteProvider("provider-uuid");

	assert.deepEqual(calls.filter((call) => Array.isArray(call)).map((call) => call[0]), [
		"create-provider", "update-provider", "default-provider", "create-model", "update-model", "default-model", "delete-model", "delete-provider",
	]);
	assert.equal(rendered.length, 9);
	assert.deepEqual(rendered.at(-1), settings);
});

test("input bindings and Control Dependencies use separate authoring actions", async () => {
	let openWorkspace;
	let selectWorkflow;
	let selectNode;
	let bindNodeInput;
	let editControlDependencies;
	const updates = [];
	const renderedEditors = [];
	const catalog = [
		{
			definition: { id: "human-chat", displayName: "Human chat", kind: "human", inputs: {}, outputs: { conversation: { type: "Conversation" } }, config: { fields: [] } },
			executor: { definitionId: "human-chat", version: "v1" },
		},
		{
			definition: { id: "llm-chat", displayName: "LLM chat", kind: "agent", inputs: { conversation: { type: "Conversation" } }, outputs: { conversation: { type: "Conversation" } }, config: { fields: [] } },
			executor: { definitionId: "llm-chat", version: "v1" },
		},
	];
	let draft = {
		...expectedDraft,
		content: { semanticSchemaVersion: "productWorkflow/v1", nodes: [
			{ id: "prompt", definition: "human-chat", executor: "v1", displayName: "Prompt", config: {} },
			{ id: "answer", definition: "llm-chat", executor: "v1", displayName: "Answer", config: {} },
		] },
	};
	const view = {
		onOpenWorkspace(handler) { openWorkspace = handler; }, onCreateWorkflow() {},
		onSelectWorkflow(handler) { selectWorkflow = handler; }, onDraftDirty() {}, onEditDraft() {},
		onAddNode() {}, onSelectNode(handler) { selectNode = handler; }, onRenameNode() {}, onEditNodeConfig() {}, onRemoveNode() {},
		onBindNodeInput(handler) { bindNodeInput = handler; },
		onEditControlDependencies(handler) { editControlDependencies = handler; },
		render() {}, renderWorkflows() {}, renderDraft() {}, renderDraftLoading() {}, renderNodeCatalog() {},
		renderNodeEditor(state) { renderedEditors.push(structuredClone(state)); },
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; }, async createWorkflow() { return expectedWorkflow; },
		async listWorkflows() { return [expectedWorkflow]; }, async listNodeCatalog() { return catalog; },
		async getDraft() { return structuredClone(draft); },
		async updateDraft(input) {
			updates.push(structuredClone(input));
			draft = { ...draft, content: structuredClone(input.content), lockVersion: draft.lockVersion + 1 };
			return { draft: structuredClone(draft), preview: { nodes: [], edges: [], groups: [], diagnostics: [] }, saved: true, conflict: false, refreshRequired: false };
		},
	});
	createProductShell(view, client);
	await openWorkspace();
	await selectWorkflow(expectedWorkflow.id);
	selectNode("answer");
	assert.deepEqual(renderedEditors.at(-1).inputSources, [{ reference: "prompt.conversation", type: "Conversation", displayName: "Prompt · conversation" }]);
	selectNode("answer", "nodes[1].inputs.conversation");
	assert.deepEqual(renderedEditors.at(-1).focus, { section: "inputs", field: "conversation" });

	await bindNodeInput({ nodeId: "answer", input: "conversation", from: "prompt.conversation" });
	await editControlDependencies({ nodeId: "answer", nodeIds: ["prompt"] });

	assert.deepEqual(updates.at(-2).content.nodes[1].inputs, { conversation: { from: "prompt.conversation" } });
	assert.deepEqual(updates.at(-1).content.nodes[1].dependsOn, ["prompt"]);
});

test("structured Node edits flush pending Draft text before mutating the latest content", async () => {
	let openWorkspace;
	let selectWorkflow;
	let editDraft;
	let addNode;
	let pendingDraft;
	let draft = structuredClone(expectedDraft);
	const view = {
		onOpenWorkspace(handler) { openWorkspace = handler; }, onCreateWorkflow() {},
		onSelectWorkflow(handler) { selectWorkflow = handler; }, onDraftDirty() {},
		onEditDraft(handler) { editDraft = handler; }, onAddNode(handler) { addNode = handler; },
		onSelectNode() {}, onRenameNode() {}, onEditNodeConfig() {}, onRemoveNode() {},
		async flushDraftEdit() { if (pendingDraft) await editDraft(pendingDraft); pendingDraft = undefined; },
		render() {}, renderWorkflows() {}, renderDraft() {}, renderDraftLoading() {}, renderNodeCatalog() {}, renderNodeEditor() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; }, async createWorkflow() { return expectedWorkflow; },
		async listWorkflows() { return [expectedWorkflow]; }, async listNodeCatalog() { return expectedCatalog; },
		async getDraft() { return structuredClone(draft); },
		async updateDraft(input) {
			draft = { ...draft, content: structuredClone(input.content), lockVersion: draft.lockVersion + 1 };
			return { draft: structuredClone(draft), preview: { nodes: draft.content.nodes, edges: [], groups: [], diagnostics: [] }, saved: true, conflict: false, refreshRequired: false };
		},
	});
	createProductShell(view, client, { createNodeId: () => "node-after-raw-edit" });
	await openWorkspace();
	await selectWorkflow(expectedWorkflow.id);
	pendingDraft = { workflowId: expectedWorkflow.id, revision: 1, content: { ...draft.content, project: { repository: "/workspace/example" } } };
	await addNode("llm-chat");
	assert.equal(draft.content.project.repository, "/workspace/example");
	assert.equal(draft.content.nodes[0].id, "node-after-raw-edit");
});

test("a user action crosses WorkflowClient and renders the visible result", async () => {
	const renderStates = [];
	const workflowStates = [];
	let openWorkspace;
	const view = {
		onOpenWorkspace(handler) {
			openWorkspace = handler;
		},
		onCreateWorkflow() {},
		onSelectWorkflow() {},
		onDraftDirty() {},
		onEditDraft() {},
		render(state) {
			renderStates.push(structuredClone(state));
		},
		renderWorkflows(workflows) {
			workflowStates.push(structuredClone(workflows));
		},
		renderDraft() {},
		renderDraftLoading() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() {
			return expectedView;
		},
		async createWorkflow() {},
		async listWorkflows() {
			return [expectedWorkflow];
		},
		async getDraft() { return expectedDraft; },
		async updateDraft() {},
	});

  createProductShell(view, client);
  await openWorkspace();

	assert.deepEqual(renderStates.at(-1), {
    status: "ready",
    title: "Gum Workflows",
    message: "Product application round-trip complete",
	});
	assert.deepEqual(workflowStates.at(-1), [expectedWorkflow]);
});

test("user creates a Product Workflow and sees the refreshed list", async () => {
	const workflows = [];
	const rendered = [];
	let createWorkflow;
	const view = {
		onOpenWorkspace() {},
		onCreateWorkflow(handler) {
			createWorkflow = handler;
		},
		onSelectWorkflow() {},
		onDraftDirty() {},
		onEditDraft() {},
		render() {},
		renderWorkflows(items) {
			rendered.push(structuredClone(items));
		},
		renderDraft() {},
		renderDraftLoading() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() {
			return expectedView;
		},
		async createWorkflow(input) {
			const created = { ...expectedWorkflow, displayName: input.displayName };
			workflows.push(created);
			return created;
		},
		async listWorkflows() {
			return workflows;
		},
		async getDraft() { return expectedDraft; },
		async updateDraft() {},
	});

	createProductShell(view, client);
	await createWorkflow("Release checklist");

	assert.deepEqual(rendered.at(-1), [expectedWorkflow]);
});

test("application failures become visible without leaking adapter details", async () => {
  const renderStates = [];
  let openWorkspace;
	const view = {
		onOpenWorkspace(handler) {
			openWorkspace = handler;
		},
		onCreateWorkflow() {},
		onSelectWorkflow() {},
		onDraftDirty() {},
		onEditDraft() {},
		render(state) {
			renderStates.push(structuredClone(state));
		},
		renderWorkflows() {},
		renderDraft() {},
		renderDraftLoading() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() {
			throw new Error("backend unavailable");
		},
		async createWorkflow() {},
		async listWorkflows() {
			return [];
		},
		async getDraft() { return expectedDraft; },
		async updateDraft() {},
	});

  createProductShell(view, client);
  await openWorkspace();

  assert.deepEqual(renderStates.at(-1), {
    status: "error",
    title: "Gum Workflows",
    message: "backend unavailable",
  });
});

test("selecting a workflow and editing content autosaves with the current concurrency token", async () => {
	let selectWorkflow;
	let editDraft;
	const renderedDrafts = [];
	const updates = [];
	const view = {
		onOpenWorkspace() {},
		onCreateWorkflow() {},
		onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty() {},
		onEditDraft(handler) { editDraft = handler; },
		render() {},
		renderWorkflows() {},
		renderDraft(state) { renderedDrafts.push(structuredClone(state)); },
		renderDraftLoading() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; },
		async createWorkflow() { return expectedWorkflow; },
		async listWorkflows() { return [expectedWorkflow]; },
		async getDraft() { return expectedDraft; },
		async updateDraft(input) {
			updates.push(structuredClone(input));
			return {
				draft: { ...expectedDraft, content: input.content, lockVersion: 2 },
				preview: { nodes: [], edges: [], groups: [], diagnostics: [{ code: "workflow-needs-node", severity: "error", path: "nodes", message: "workflow must contain at least one node" }] },
				saved: true,
				conflict: false,
				refreshRequired: false,
			};
		},
	});

	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	await editDraft({ workflowId: expectedWorkflow.id, revision: 1, content: {} });

	assert.deepEqual(updates, [{ workflowId: expectedWorkflow.id, expectedLockVersion: 1, content: {} }]);
	assert.equal(renderedDrafts.at(-1).draft.lockVersion, 2);
	assert.equal(renderedDrafts.at(-1).preview.diagnostics.length, 1);
});

test("overlapping edits serialize autosaves with the latest returned lock token", async () => {
	let selectWorkflow;
	let editDraft;
	let draftDirty;
	let releaseFirstSave;
	const updates = [];
	const renderedDrafts = [];
	const view = {
		onOpenWorkspace() {},
		onCreateWorkflow() {},
		onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty(handler) { draftDirty = handler; },
		onEditDraft(handler) { editDraft = handler; },
		render() {},
		renderWorkflows() {},
		renderDraft(result) { renderedDrafts.push(structuredClone(result)); },
		renderDraftLoading() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; },
		async createWorkflow() { return expectedWorkflow; },
		async listWorkflows() { return [expectedWorkflow]; },
		async getDraft() { return expectedDraft; },
		async updateDraft(input) {
			updates.push(structuredClone(input));
			if (updates.length === 1) {
				await new Promise((resolve) => { releaseFirstSave = resolve; });
			}
			return {
				draft: { ...expectedDraft, content: input.content, lockVersion: input.expectedLockVersion + 1 },
				preview: { nodes: [], edges: [], groups: [], diagnostics: [] },
				saved: true,
				conflict: false,
				refreshRequired: false,
			};
		},
	});

	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	draftDirty({ workflowId: expectedWorkflow.id, revision: 1 });
	const firstSave = editDraft({ workflowId: expectedWorkflow.id, revision: 1, content: { nodes: [{ id: "first" }] } });
	await Promise.resolve();
	draftDirty({ workflowId: expectedWorkflow.id, revision: 2 });
	const secondSave = editDraft({ workflowId: expectedWorkflow.id, revision: 2, content: { nodes: [{ id: "second" }] } });
	await Promise.resolve();
	assert.equal(updates.length, 1);
	releaseFirstSave();
	await firstSave;
	await secondSave;

	assert.deepEqual(updates.map((update) => update.expectedLockVersion), [1, 2]);
	assert.deepEqual(updates.at(-1).content, { nodes: [{ id: "second" }] });
	assert.deepEqual(renderedDrafts.at(-1).draft.content, { nodes: [{ id: "second" }] });
	assert.equal(renderedDrafts.some((result) => result.saved && result.draft.content.nodes?.[0]?.id === "first"), false);
});

test("a conflict stops queued autosaves and renders the latest stored Draft", async () => {
	let selectWorkflow;
	let editDraft;
	let releaseConflict;
	const updates = [];
	const renderedDrafts = [];
	const latestDraft = { ...expectedDraft, content: { nodes: [{ id: "external" }] }, lockVersion: 2 };
	const view = {
		onOpenWorkspace() {},
		onCreateWorkflow() {},
		onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty() {},
		onEditDraft(handler) { editDraft = handler; },
		render() {},
		renderWorkflows() {},
		renderDraft(result) { renderedDrafts.push(structuredClone(result)); },
		renderDraftLoading() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; },
		async createWorkflow() { return expectedWorkflow; },
		async listWorkflows() { return [expectedWorkflow]; },
		async getDraft() { return expectedDraft; },
		async updateDraft(input) {
			updates.push(structuredClone(input));
			await new Promise((resolve) => { releaseConflict = resolve; });
			return {
				draft: latestDraft,
				preview: { nodes: [], edges: [], groups: [], diagnostics: [] },
				saved: false,
				conflict: true,
				refreshRequired: true,
			};
		},
	});

	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	const firstSave = editDraft({ workflowId: expectedWorkflow.id, revision: 1, content: { nodes: [{ id: "local-first" }] } });
	await Promise.resolve();
	const queuedSave = editDraft({ workflowId: expectedWorkflow.id, revision: 2, content: { nodes: [{ id: "local-second" }] } });
	releaseConflict();
	await firstSave;
	await queuedSave;

	assert.equal(updates.length, 1);
	assert.equal(renderedDrafts.at(-1).conflict, true);
	assert.deepEqual(renderedDrafts.at(-1).draft, latestDraft);
});

test("editing is suspended while another workflow Draft is loading", async () => {
	let selectWorkflow;
	let editDraft;
	let releaseSecondDraft;
	const updates = [];
	const loadingStates = [];
	const secondWorkflowId = "22222222-2222-4222-8222-222222222222";
	let draftLoads = 0;
	const view = {
		onOpenWorkspace() {},
		onCreateWorkflow() {},
		onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty() {},
		onEditDraft(handler) { editDraft = handler; },
		render() {},
		renderWorkflows() {},
		renderDraft() {},
		renderDraftLoading() { loadingStates.push(true); },
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; },
		async createWorkflow() { return expectedWorkflow; },
		async listWorkflows() { return [expectedWorkflow]; },
		async getDraft(workflowId) {
			draftLoads += 1;
			if (draftLoads === 2) await new Promise((resolve) => { releaseSecondDraft = resolve; });
			return { ...expectedDraft, workflowId };
		},
		async updateDraft(input) {
			updates.push(structuredClone(input));
			throw new Error("must not autosave while selection is loading");
		},
	});

	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	const loading = selectWorkflow(secondWorkflowId);
	await Promise.resolve();
	await editDraft({ workflowId: expectedWorkflow.id, revision: 1, content: { nodes: [{ id: "stale" }] } });
	assert.equal(updates.length, 0);
	assert.equal(loadingStates.length, 2);
	releaseSecondDraft();
	await loading;
});

test("desktop and browser entries share one DOM view adapter", () => {
  const title = { textContent: "" };
  const message = { textContent: "" };
  const status = { textContent: "", dataset: {} };
	const button = { disabled: false, addEventListener() {} };
	const form = { addEventListener() {}, reset() {} };
	const nameInput = { value: "Release checklist" };
	const workflowList = { replaceChildren() {} };
	const draftEditor = { disabled: true, value: "", addEventListener() {} };
	const draftStatus = { textContent: "" };
	const diagnosticList = { replaceChildren() {} };
	const view = createProductDOMView(
		{ title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList },
		productStatusMessage,
	);

	view.render({ status: "ready", title: "Gum Workflows", message: "ready" });
	view.renderDraft({ draft: expectedDraft });

  assert.equal(title.textContent, "Gum Workflows");
  assert.equal(message.textContent, "ready");
  assert.equal(status.textContent, "Application round-trip complete");
  assert.equal(status.dataset.state, "ready");
	assert.equal(button.disabled, false);
	assert.equal(draftStatus.textContent, "Draft loaded.");
	assert.equal(draftStatus.textContent.includes("token"), false);
});

test("the DOM Run action renders successful Node Runs and Conversation messages", async () => {
	const document = {
		createElement(tag) {
			return { tag, children: [], textContent: "", append(...children) { this.children.push(...children); } };
		},
	};
	const runButton = { listeners: {}, disabled: false, addEventListener(event, handler) { this.listeners[event] = handler; } };
	const runStatus = { textContent: "" };
	const artifactList = { ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } };
	const nodeRunList = { ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } };
	const view = createProductDOMView({
		title: {}, message: {}, status: { dataset: {} }, button: { addEventListener() {} }, form: { addEventListener() {} }, nameInput: {},
		workflowList: { replaceChildren() {} }, draftEditor: { addEventListener() {} }, draftStatus: {}, diagnosticList: { replaceChildren() {} },
		runButton, runStatus, artifactList, nodeRunList,
	}, productStatusMessage);
	let started = false;
	view.onStartRun(() => { started = true; });
	await runButton.listeners.click();
	view.renderRun(expectedRun);

	assert.equal(started, true);
	assert.equal(runStatus.textContent, "Run succeeded · revision revision-uuid");
	assert.deepEqual(nodeRunList.items.map((item) => item.textContent), ["prompt · human-chat@v1 · succeeded", "answer · llm-chat@v1 · succeeded"]);
	assert.equal(artifactList.items[0].children[1].children[1].textContent, "assistant: Fake response");
});

test("the DOM history panel renders Revisions, Revision Runs and a historical Run", async () => {
	const document = {
		createElement(tag) {
			return {
				tag, children: [], textContent: "", listeners: {},
				append(...children) { this.children.push(...children); },
				addEventListener(event, handler) { this.listeners[event] = handler; },
			};
		},
	};
	const revisionList = { ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } };
	const revisionRunList = { ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } };
	const historyRunStatus = { textContent: "" };
	const historyNodeRunList = { ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } };
	const historyArtifactList = { ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } };
	const view = createProductDOMView({
		title: {}, message: {}, status: { dataset: {} }, button: { addEventListener() {} }, form: { addEventListener() {} }, nameInput: {},
		workflowList: { replaceChildren() {} }, draftEditor: { addEventListener() {} }, draftStatus: {}, diagnosticList: { replaceChildren() {} },
		revisionList, revisionRunList, historyRunStatus, historyNodeRunList, historyArtifactList,
	}, productStatusMessage);
	let selectedRevision = "";
	let selectedRun = "";
	view.onSelectRevision((revisionId) => { selectedRevision = revisionId; });
	view.onSelectRun((runId) => { selectedRun = runId; });

	view.renderRevisions(expectedRevisions);
	view.renderRevisionRuns(expectedRevisionRuns);
	view.renderHistoryRun(expectedRun);

	assert.equal(revisionList.items[0].children[0].textContent, "Revision abc12345 · 1 run(s)");
	await revisionList.items[0].children[0].listeners.click();
	assert.equal(selectedRevision, "revision-uuid");
	assert.equal(revisionRunList.items[0].children[0].textContent, "Run succeeded · run-uuid");
	await revisionRunList.items[0].children[0].listeners.click();
	assert.equal(selectedRun, "run-uuid");
	assert.equal(historyRunStatus.textContent, "Run succeeded · revision revision-uuid");
	assert.deepEqual(historyNodeRunList.items.map((item) => item.textContent), ["prompt · human-chat@v1 · succeeded", "answer · llm-chat@v1 · succeeded"]);
	assert.equal(historyArtifactList.items[0].children[1].children[1].textContent, "assistant: Fake response");
});

test("the Node editor renders distinct Input Binding and Control Dependency controls", async () => {
	const document = {
		createElement(tag) {
			return {
				tag, children: [], listeners: {}, value: "", checked: false,
				append(...children) { this.children.push(...children); },
				addEventListener(event, handler) { this.listeners[event] = handler; },
			};
		},
	};
	const container = () => ({ ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } });
	const inputForm = container();
	const controlForm = container();
	const bindings = [];
	const dependencies = [];
	const view = createProductDOMView({
		title: { textContent: "" }, message: { textContent: "" }, status: { textContent: "", dataset: {} },
		button: { addEventListener() {} }, form: { addEventListener() {} }, nameInput: {}, workflowList: container(),
		draftEditor: { addEventListener() {} }, draftStatus: {}, diagnosticList: container(),
		nodeEditor: { hidden: true }, nodeEditorStatus: {}, nodeName: { addEventListener() {} }, removeNodeButton: { addEventListener() {} },
		nodeConfigForm: container(), nodeInputForm: inputForm, nodeControlForm: controlForm,
	}, productStatusMessage);
	view.onBindNodeInput((binding) => bindings.push(binding));
	view.onEditControlDependencies((change) => dependencies.push(change));
	view.renderNodeEditor({
		node: { id: "answer", definition: "llm-chat", executor: "v1", displayName: "Answer", config: {}, inputs: {}, dependsOn: [] },
		fields: [], inputs: { conversation: { type: "Conversation" } },
		inputSources: [{ reference: "prompt.conversation", type: "Conversation", displayName: "Prompt · conversation" }],
		controlNodes: [{ id: "prompt", displayName: "Prompt" }],
	});

	const inputSelect = inputForm.items[0].children[1];
	inputSelect.value = "prompt.conversation";
	await inputSelect.listeners.change();
	const controlCheckbox = controlForm.items[0].children[0];
	controlCheckbox.checked = true;
	await controlCheckbox.listeners.change();

	assert.deepEqual(bindings, [{ nodeId: "answer", input: "conversation", from: "prompt.conversation" }]);
	assert.deepEqual(dependencies, [{ nodeId: "answer", nodeIds: ["prompt"] }]);
});

test("the DOM settings view renders editable Provider and Model controls", async () => {
	const document = {
		createElement(tag) {
			return {
				tag, children: [], listeners: {}, value: "", disabled: false,
				append(...children) { this.children.push(...children); },
				addEventListener(event, handler) { this.listeners[event] = handler; },
				setAttribute() {},
			};
		},
	};
	const container = () => ({ ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } });
	const providerList = container();
	const diagnosticList = container();
	const calls = [];
	const view = createProductDOMView({
		title: {}, message: {}, status: { dataset: {} }, button: { addEventListener() {} }, form: { addEventListener() {} }, nameInput: {},
		workflowList: container(), draftEditor: { addEventListener() {} }, draftStatus: {}, diagnosticList: container(),
		llmProviderList: providerList, llmDiagnosticList: diagnosticList,
	}, productStatusMessage);
	view.onUpdateLLMProvider((input) => calls.push(["provider", input]));
	view.onSetDefaultLLMProvider((id) => calls.push(["provider-default", id]));
	view.onDeleteLLMProvider((id) => calls.push(["provider-delete", id]));
	view.onCreateLLMModel((input) => calls.push(["model-create", input]));
	view.onUpdateLLMModel((input) => calls.push(["model", input]));
	view.onSetDefaultLLMModel((providerId, id) => calls.push(["model-default", providerId, id]));
	view.onDeleteLLMModel((providerId, id) => calls.push(["model-delete", providerId, id]));
	view.renderLLMSettings(expectedSettings);

	const card = providerList.items[0];
	const providerControls = card.children[0].children;
	providerControls[0].value = "Renamed";
	await providerControls[4].listeners.click();
	await providerControls[5].listeners.click();
	const modelControls = card.children[1].children[0].children;
	modelControls[1].value = "model-fast-v2";
	modelControls[2].value = "0.5";
	modelControls[3].value = "2048";
	await modelControls[4].listeners.click();
	const addModelForm = card.children[2];
	addModelForm.children[0].value = "Strong";
	addModelForm.children[1].value = "model-strong";
	addModelForm.children[2].value = "0.7";
	addModelForm.children[3].value = "4096";
	await addModelForm.listeners.submit({ preventDefault() {} });

	assert.deepEqual(calls.map((call) => call[0]), ["provider", "provider-default", "model", "model-create"]);
	assert.equal(calls[0][1].id, "provider-uuid");
	assert.equal(calls[2][1].providerModelId, "model-fast-v2");
	assert.deepEqual(calls[2][1].generationDefaults, { temperature: 0.5, maxOutputTokens: 2048 });
	assert.equal(diagnosticList.items.length, 0);
});

test("the read-only Preview separates Edge kinds and keeps view preferences outside autosave", () => {
	const document = {
		createElement(tag) {
			return {
				tag, children: [], listeners: {}, dataset: {}, style: {}, open: true,
				append(...children) { this.children.push(...children); },
				addEventListener(event, handler) { this.listeners[event] = handler; },
			};
		},
	};
	const container = () => ({ ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } });
	const previewCanvas = container();
	previewCanvas.style = {};
	const previewEdges = container();
	const previewGroups = container();
	const zoomIn = { listeners: {}, addEventListener(event, handler) { this.listeners[event] = handler; } };
	const zoomOut = { listeners: {}, addEventListener(event, handler) { this.listeners[event] = handler; } };
	const zoomReset = { listeners: {}, addEventListener(event, handler) { this.listeners[event] = handler; } };
	const submitted = [];
	const selectedNodes = [];
	const diagnosticList = container();
	const view = createProductDOMView({
		title: {}, message: {}, status: { dataset: {} }, button: { addEventListener() {} }, form: { addEventListener() {} }, nameInput: {},
		workflowList: container(), draftEditor: { addEventListener() {} }, draftStatus: {}, diagnosticList,
		previewCanvas, previewEdges, previewGroups, previewZoomIn: zoomIn, previewZoomOut: zoomOut, previewZoomReset: zoomReset,
	}, productStatusMessage);
	view.onEditDraft((edit) => submitted.push(edit));
	view.onSelectNode((nodeId, fieldPath) => selectedNodes.push([nodeId, fieldPath]));
	view.renderDraft({
		draft: { ...expectedDraft, content: { semanticSchemaVersion: "productWorkflow/v1", nodes: [
			{ id: "zeta" }, { id: "prompt" }, { id: "answer" }, { id: "alpha" }, { id: "review" },
		] } },
		preview: {
			nodes: [
				{ id: "zeta", definitionId: "human-chat", displayName: "Zeta", kind: "human" },
				{ id: "prompt", definitionId: "human-chat", displayName: "Prompt", kind: "human" },
				{ id: "answer", definitionId: "llm-chat", displayName: "Answer", kind: "agent" },
				{ id: "alpha", definitionId: "human-chat", displayName: "Alpha", kind: "human" },
				{ id: "review", definitionId: "human-chat", displayName: "Review", kind: "human" },
			],
			edges: [
				{ kind: "data", sourceNodeId: "prompt", sourcePort: "conversation", targetNodeId: "answer", targetPort: "conversation", artifactType: "Conversation" },
				{ kind: "control", sourceNodeId: "answer", targetNodeId: "review" },
			],
			groups: [], diagnostics: [{ code: "missing-input-binding", severity: "error", path: "nodes[2].inputs.conversation", message: "required input is not bound" }],
		},
	});

	assert.equal(previewCanvas.items.length, 5);
	assert.deepEqual(previewCanvas.items.map((item) => item.children[0].textContent), ["Alpha", "Prompt", "Zeta", "Answer", "Review"]);
	assert.deepEqual(previewCanvas.items.map((item) => item.style.gridColumn), ["1", "1", "1", "2", "3"]);
	assert.deepEqual(previewEdges.items.map((item) => item.dataset.kind), ["data", "control"]);
	diagnosticList.items[0].children[0].listeners.click();
	previewCanvas.items[3].children[0].listeners.click();
	assert.deepEqual(selectedNodes, [["answer", "nodes[2].inputs.conversation"], ["answer", undefined]]);
	previewEdges.items[1].children[0].listeners.click();
	assert.equal(previewEdges.items[1].dataset.selected, "true");
	zoomIn.listeners.click();
	assert.equal(previewCanvas.style.transform, "scale(1.1)");
	previewCanvas.items[0].open = false;
	previewCanvas.items[0].listeners.toggle();
	assert.deepEqual(submitted, []);
});

test("the DOM editor debounces input and submits the latest captured content", async () => {
	let inputDraft;
	const dirtied = [];
	const submitted = [];
	const title = { textContent: "" };
	const message = { textContent: "" };
	const status = { textContent: "", dataset: {} };
	const button = { disabled: false, addEventListener() {} };
	const form = { addEventListener() {}, reset() {} };
	const nameInput = { value: "" };
	const workflowList = { replaceChildren() {} };
	const draftEditor = {
		disabled: false,
		value: "",
		addEventListener(event, handler) {
			if (event === "input") inputDraft = handler;
		},
	};
	const draftStatus = { textContent: "" };
	const diagnosticList = { replaceChildren() {} };
	const view = createProductDOMView(
		{ title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList },
		productStatusMessage,
	);
	view.renderDraft({ draft: expectedDraft });
	view.onDraftDirty((edit) => { dirtied.push(structuredClone(edit)); });
	view.onEditDraft(async (edit) => { submitted.push(structuredClone(edit)); });

	draftEditor.value = '{"nodes":[{"id":"first"}]}';
	inputDraft();
	draftEditor.value = '{"nodes":[{"id":"latest"}]}';
	inputDraft();
	assert.deepEqual(dirtied, [
		{ workflowId: expectedWorkflow.id, revision: 1 },
		{ workflowId: expectedWorkflow.id, revision: 2 },
	]);
	await view.flushDraftEdit();

	assert.deepEqual(submitted, [{ workflowId: expectedWorkflow.id, revision: 2, content: { nodes: [{ id: "latest" }] } }]);
});

test("selecting another workflow cancels the previous workflow's pending debounce", async () => {
	let inputDraft;
	const submitted = [];
	const draftEditor = {
		disabled: false,
		value: "",
		addEventListener(event, handler) { if (event === "input") inputDraft = handler; },
	};
	const view = createProductDOMView({
		title: { textContent: "" }, message: { textContent: "" }, status: { textContent: "", dataset: {} },
		button: { disabled: false, addEventListener() {} }, form: { addEventListener() {}, reset() {} },
		nameInput: { value: "" }, workflowList: { replaceChildren() {} }, draftEditor,
		draftStatus: { textContent: "" }, diagnosticList: { replaceChildren() {} },
	}, productStatusMessage);
	view.onDraftDirty(() => {});
	view.onEditDraft(async (edit) => { submitted.push(structuredClone(edit)); });
	view.renderDraft({ draft: expectedDraft });
	draftEditor.value = '{"nodes":[{"id":"workflow-a"}]}';
	inputDraft();
	view.renderDraft({
		draft: { ...expectedDraft, workflowId: "workflow-b" },
	});
	await new Promise((resolve) => setTimeout(resolve, 300));

	assert.deepEqual(submitted, []);
});

test("the DOM and shell preserve newer text while an earlier autosave is in flight", async () => {
	let openWorkspace;
	let inputDraft;
	let releaseFirstSave;
	const updates = [];
	const document = {
		createElement(tag) {
			return {
				tag,
				listeners: {},
				addEventListener(event, handler) { this.listeners[event] = handler; },
				append(...children) { this.children = children; },
			};
		},
	};
	const workflowList = {
		ownerDocument: document,
		replaceChildren(...items) { this.items = items; },
	};
	const draftEditor = {
		disabled: true,
		value: "",
		addEventListener(event, handler) { if (event === "input") inputDraft = handler; },
	};
	const elements = {
		title: { textContent: "" },
		message: { textContent: "" },
		status: { textContent: "", dataset: {} },
		button: { disabled: false, addEventListener(event, handler) { if (event === "click") openWorkspace = handler; } },
		form: { addEventListener() {}, reset() {} },
		nameInput: { value: "" },
		workflowList,
		draftEditor,
		draftStatus: { textContent: "" },
		diagnosticList: { ownerDocument: document, replaceChildren() {} },
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; },
		async createWorkflow() { return expectedWorkflow; },
		async listWorkflows() { return [expectedWorkflow]; },
		async getDraft() { return expectedDraft; },
		async updateDraft(input) {
			updates.push(structuredClone(input));
			if (updates.length === 1) await new Promise((resolve) => { releaseFirstSave = resolve; });
			return {
				draft: { ...expectedDraft, content: input.content, lockVersion: input.expectedLockVersion + 1 },
				preview: { nodes: [], edges: [], groups: [], diagnostics: [] },
				saved: true,
				conflict: false,
				refreshRequired: false,
			};
		},
	});
	createProductShell(createProductDOMView(elements, productStatusMessage), client);
	await openWorkspace();
	await workflowList.items[0].children[0].listeners.click();

	const firstText = '{"semanticSchemaVersion":"productWorkflow/v1","nodes":[{"id":"first"}]}';
	const latestText = '{"semanticSchemaVersion":"productWorkflow/v1","nodes":[{"id":"latest"}]}';
	draftEditor.value = firstText;
	inputDraft();
	await new Promise((resolve) => setTimeout(resolve, 300));
	draftEditor.value = latestText;
	inputDraft();
	releaseFirstSave();
	await Promise.resolve();
	await Promise.resolve();
	assert.equal(draftEditor.value, latestText);
	await new Promise((resolve) => setTimeout(resolve, 300));
	await Promise.resolve();

	assert.deepEqual(updates.map((update) => update.expectedLockVersion), [1, 2]);
	assert.equal(updates.at(-1).content.nodes[0].id, "latest");
});

test("the shell gives both clients the same user-facing status text", () => {
  assert.equal(productStatusMessage({ status: "idle" }), "Waiting for an action");
  assert.equal(productStatusMessage({ status: "loading" }), "Working…");
  assert.equal(productStatusMessage({ status: "ready" }), "Application round-trip complete");
  assert.equal(productStatusMessage({ status: "error" }), "Application error");
});
