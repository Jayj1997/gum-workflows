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
import { createBrowserLLMSettings, createMemorySecretAdapter } from "../dist/browser-llm-settings.js";
import { productRevisionKey } from "../dist/browser-run.js";
import { createFixtureChatAdapter } from "../dist/browser-chat-fixture.js";
import { createBrowserApplication } from "../dist/browser.js";

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
		{ id: "agent-run", nodeId: "answer", nodeDefinition: "llm-chat", nodeExecutor: "v1", status: "succeeded", diagnostics: { providerRequestId: "chatcmpl-1", finishReason: "stop", usage: { inputTokens: 12, outputTokens: 7, totalTokens: 19 } } },
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
		id: "provider-uuid", name: "Primary", protocol: "openai-chat-completions", dialect: "developer", baseUrl: "https://api.example/v1",
		hasApiKey: true, explicitDefault: true, effectiveDefault: true, createdAt: "2026-08-31T09:00:00Z",
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
		async deleteLLMProvider() {},
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [{ id: "model-uuid", displayName: "Fast", providerModelId: "model-fast" }], diagnostics: [] }; },
		async setDefaultLLMProvider() { return expectedSettings; },
		async createLLMModel(input) { return { ...expectedSettings.providers[0].models[0], ...input }; },
		async updateLLMModel(input) { return { ...expectedSettings.providers[0].models[0], ...input }; },
		async deleteLLMModel() {},
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async setDefaultLLMModel() { return expectedSettings; },
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
		async DeleteLLMProvider() {},
		async ListProviderDeletionImpact() { return { workflows: [], modelSlots: [{ id: "model-uuid", displayName: "Fast", providerModelId: "model-fast" }], diagnostics: [] }; },
		async SetDefaultLLMProvider() { return expectedSettings; },
		async CreateLLMModel(input) { return { ...expectedSettings.providers[0].models[0], ...input }; },
		async UpdateLLMModel(input) { return { ...expectedSettings.providers[0].models[0], ...input }; },
		async DeleteLLMModel() {},
		async ListModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async SetDefaultLLMModel() { return expectedSettings; },
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
	await client.deleteLLMProvider({ providerId: "provider-uuid", confirmed: true });
	assert.deepEqual(await client.listProviderDeletionImpact("provider-uuid"), { workflows: [], modelSlots: [{ id: "model-uuid", displayName: "Fast", providerModelId: "model-fast" }], diagnostics: [] });
	assert.deepEqual(await client.setDefaultLLMProvider("provider-uuid"), expectedSettings);
	assert.equal((await client.createLLMModel({ providerId: "provider-uuid", displayName: "Fast" })).displayName, "Fast");
	assert.equal((await client.updateLLMModel({ id: "model-uuid", providerId: "provider-uuid", displayName: "Strong" })).displayName, "Strong");
	await client.deleteLLMModel("provider-uuid", "model-uuid");
	assert.deepEqual(await client.listModelDeletionImpact("provider-uuid", "model-uuid"), { workflows: [], modelSlots: [], diagnostics: [] });
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
			calls.push(["start", input.expectedLockVersion, input.humanInput]);
			return { ...structuredClone(expectedRun), draft: structuredClone(draft) };
		},
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
	});
	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	await startRun({ nodeId: "prompt", text: "Explain the application seam." });

	assert.deepEqual(calls, ["flush", ["start", 2, { nodeId: "prompt", text: "Explain the application seam." }]]);
	assert.equal(renderedRuns.at(-1).artifacts[0].messages[1].text, "Fake response");
});

test("a failed StartRun refreshes persisted history and shows the actionable error", async () => {
	let selectWorkflow, startRun;
	let revisionCalls = 0;
	const rendered = [];
	const revisions = [];
	const view = {
		onOpenWorkspace() {}, onCreateWorkflow() {}, onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty() {}, onEditDraft() {}, onStartRun(handler) { startRun = handler; },
		render(state) { rendered.push(structuredClone(state)); }, renderDraft() {}, renderDraftLoading() {}, renderNodeEditor() {},
		renderRevisions(items) { revisions.push(structuredClone(items)); }, renderRevisionRuns() {}, renderHistoryRun() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; }, async createWorkflow() {}, async listWorkflows() { return []; },
		async getDraft() { return structuredClone(expectedDraft); }, async updateDraft() {},
		async startRun() { throw new Error("Run run-failed structural/authentication: key rejected; check the Provider API Key and start a new Run"); },
		async listRevisions() { revisionCalls += 1; return [{ ...expectedRevisions[0], runCount: 1 }]; },
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
	});
	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	await startRun({ nodeId: "prompt", text: "Hello" });

	assert.equal(revisionCalls, 2);
	assert.equal(rendered.at(-1).status, "error");
	assert.match(rendered.at(-1).message, /check the Provider API Key and start a new Run/);
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
	const laterProvider = settings.createProvider({ name: "Later UUID", protocol: "openai-chat-completions", dialect: "", baseUrl: "https://later.example/v1", apiKey: "later-secret" });
	const earlierProvider = settings.createProvider({ name: "Earlier UUID", protocol: "openai-chat-completions", baseUrl: "https://earlier.example/v1", apiKey: "earlier-secret" });
	assert.equal(laterProvider.dialect, "developer");
	assert.throws(() => settings.updateProvider({ ...laterProvider, dialect: "legacy-system" }), /dialect must be developer or system/);
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

test("Browser Mock injects Secret storage and never returns plaintext API Keys", () => {
	const secrets = createMemorySecretAdapter();
	const settings = createBrowserLLMSettings({
		newID: () => "provider-uuid",
		now: () => "2026-09-01T00:00:00Z",
		secrets,
	});
	const provider = settings.createProvider({
		name: "Primary", protocol: "openai-chat-completions", baseUrl: "https://api.example/v1", apiKey: "sk-browser-secret",
	});
	assert.equal(provider.hasApiKey, true);
	assert.equal(JSON.stringify(provider).includes("sk-browser-secret"), false);
	const reference = "memory://gum-workflows/llm-provider%2Fprovider-uuid";
	assert.equal(secrets.resolve(reference), "sk-browser-secret");
	assert.throws(() => settings.deleteProvider({ providerId: provider.id }), /confirmation/);
	settings.deleteProvider({ providerId: provider.id, confirmed: true });
	assert.throws(() => secrets.resolve(reference), /not found/);
});

test("the Browser Mock default chat fixture completes a local model call", () => {
	const secrets = createMemorySecretAdapter();
	const apiKeyRef = secrets.store("llm-provider/provider-uuid", "browser-secret");
	const adapter = createFixtureChatAdapter({ secrets });
	const result = adapter.generate(
		{ dialect: "developer", baseUrl: "https://api.example/v1", providerModelId: "model-fast", apiKeyRef },
		{ instructions: [], messages: [{ role: "user", parts: [{ kind: "text", text: "Hello" }] }], config: {} },
	);
	assert.equal(result.assistant.parts[0].text, "Browser fixture response.");
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

async function configureBrowserTracer(client) {
	const workflow = await client.createWorkflow({ displayName: "Browser lifecycle" });
	const provider = await client.createLLMProvider({ name: "Primary", protocol: "openai-chat-completions", baseUrl: "https://api.example/v1", apiKey: "browser-secret" });
	await client.createLLMModel({ providerId: provider.id, displayName: "Fixture", providerModelId: "fixture-model", generationDefaults: {} });
	const draft = await client.getDraft(workflow.id);
	const updated = await client.updateDraft({
		workflowId: workflow.id, expectedLockVersion: draft.lockVersion,
		content: {
			semanticSchemaVersion: "productWorkflow/v1",
			nodes: [
				{ id: "prompt", definition: "human-chat", executor: "v1", config: {} },
				{ id: "answer", definition: "llm-chat", executor: "v1", config: {}, inputs: { conversation: { from: "prompt.conversation" } } },
			],
		},
	});
	return { workflow, lockVersion: updated.draft.lockVersion };
}

test("Browser WorkflowClient persists failed execution progress through its public history seam", async () => {
	const application = createBrowserApplication({ chatAdapter: { generate() { throw new Error("fixture capacity"); } } });
	const client = createBrowserWorkflowClient(application);
	await client.openWorkspace();
	const { workflow, lockVersion } = await configureBrowserTracer(client);
	await assert.rejects(
		client.startRun({ workflowId: workflow.id, expectedLockVersion: lockVersion, humanInput: { nodeId: "prompt", text: "Hello" } }),
		/structural\/provider.*fixture capacity/,
	);
	const revisions = await client.listRevisions(workflow.id);
	const runs = await client.listRevisionRuns(revisions[0].id);
	assert.equal(runs[0].status, "failed");
	const detail = await client.getRunHistory(runs[0].id);
	assert.equal(detail.nodeRuns[0].status, "succeeded");
	assert.equal(detail.nodeRuns[1].status, "failed");
	assert.equal(detail.nodeRuns[1].diagnostics.error.code, "provider");
	assert.equal(detail.artifacts.length, 1);
});

test("deleting a referenced Model dangles the UUID, blocks StartRun and keeps history visible", async () => {
	const application = createBrowserApplication();
	const client = createBrowserWorkflowClient(application);
	await client.openWorkspace();
	const { workflow, lockVersion } = await configureBrowserTracer(client);
	const settings = await client.getLLMSettings();
	const provider = settings.providers[0];
	const model = provider.models[0];

	// Bind the agent Node to the explicit Model UUID and run once.
	await client.updateDraft({
		workflowId: workflow.id, expectedLockVersion: lockVersion,
		content: {
			semanticSchemaVersion: "productWorkflow/v1",
			nodes: [
				{ id: "prompt", definition: "human-chat", executor: "v1", config: {} },
				{ id: "answer", definition: "llm-chat", executor: "v1", config: {}, inputs: { conversation: { from: "prompt.conversation" } }, llm: { modelUuid: model.id } },
			],
		},
	});
	const first = await client.startRun({ workflowId: workflow.id, expectedLockVersion: lockVersion + 1, humanInput: { nodeId: "prompt", text: "Hello" } });
	assert.equal(first.status, "succeeded");

	// The deletion preview reports the referencing workflow and its Node.
	const impact = await client.listModelDeletionImpact(provider.id, model.id);
	assert.deepEqual(impact.workflows, [{ id: workflow.id, displayName: "Browser lifecycle", nodeId: "answer", nodeDefinition: "llm-chat", modelUuid: model.id }]);
	assert.deepEqual(impact.modelSlots, []);

	// After deletion the Draft keeps the UUID, the Preview dangles and the
	// next StartRun is refused before any new Run appears.
	await client.deleteLLMModel(provider.id, model.id);
	const draft = await client.getDraft(workflow.id);
	assert.equal(draft.content.nodes[1].llm.modelUuid, model.id);
	const dangling = draft.preview.diagnostics.find((diagnostic) => diagnostic.code === "dangling-model-uuid");
	assert.ok(dangling, `preview diagnostics = ${JSON.stringify(draft.preview.diagnostics)}`);
	assert.equal(dangling.path, "nodes[1].llm.modelUuid");
	await assert.rejects(
		client.startRun({ workflowId: workflow.id, expectedLockVersion: draft.lockVersion, humanInput: { nodeId: "prompt", text: "Again" } }),
		/dangling|diagnostics|Model/i,
	);
	const revisions = await client.listRevisions(workflow.id);
	const runs = await client.listRevisionRuns(revisions[0].id);
	assert.equal(runs.length, 1);

	// The historical Run still resolves the deleted Slot's selection.
	const detail = await client.getRunHistory(first.id);
	assert.equal(detail.snapshot.llmSelections[0].providerName, provider.name);
	assert.equal(detail.snapshot.llmSelections[0].providerModelId, model.providerModelId);
	assert.equal(detail.snapshot.llmSelections[0].modelUuid, model.id);
	assert.equal(detail.draft.preview.diagnostics.filter((diagnostic) => diagnostic.code === "dangling-model-uuid").length, 0);
});

test("Browser WorkflowClient only performs recovery on its first workspace open", async () => {
	let enteredResolve;
	let releaseResolve;
	const entered = new Promise((resolve) => { enteredResolve = resolve; });
	const blocked = new Promise((resolve) => { releaseResolve = resolve; });
	const application = createBrowserApplication({ chatAdapter: { generate() { enteredResolve(); return blocked; } } });
	const client = createBrowserWorkflowClient(application);
	await client.openWorkspace();
	const { workflow, lockVersion } = await configureBrowserTracer(client);
	const pending = client.startRun({ workflowId: workflow.id, expectedLockVersion: lockVersion, humanInput: { nodeId: "prompt", text: "Hello" } });
	await entered;
	let revisions = await client.listRevisions(workflow.id);
	let runs = await client.listRevisionRuns(revisions[0].id);
	assert.equal(runs[0].status, "running");

	await client.openWorkspace();
	revisions = await client.listRevisions(workflow.id);
	runs = await client.listRevisionRuns(revisions[0].id);
	assert.equal(runs[0].status, "running");
	releaseResolve({ assistant: { role: "assistant", parts: [{ kind: "text", text: "Done" }] }, finishReason: "stop", usage: {}, providerRequestId: "chatcmpl-browser-blocked" });
	const completed = await pending;
	assert.equal(completed.status, "succeeded");
});

test("Browser WorkflowClient first open interrupts retained in-flight state and rejects a late result", async () => {
	let enteredResolve;
	let releaseResolve;
	const entered = new Promise((resolve) => { enteredResolve = resolve; });
	const blocked = new Promise((resolve) => { releaseResolve = resolve; });
	const application = createBrowserApplication({ chatAdapter: { generate() { enteredResolve(); return blocked; } } });
	const client = createBrowserWorkflowClient(application);
	const { workflow, lockVersion } = await configureBrowserTracer(client);
	const pending = client.startRun({ workflowId: workflow.id, expectedLockVersion: lockVersion, humanInput: { nodeId: "prompt", text: "Hello" } });
	await entered;
	await client.openWorkspace();
	const revisions = await client.listRevisions(workflow.id);
	const runs = await client.listRevisionRuns(revisions[0].id);
	assert.equal(runs[0].status, "interrupted");
	let detail = await client.getRunHistory(runs[0].id);
	assert.equal(detail.nodeRuns[0].status, "succeeded");
	assert.equal(detail.nodeRuns[1].status, "unknown-outcome");
	assert.equal(detail.artifacts.length, 1);

	releaseResolve({ assistant: { role: "assistant", parts: [{ kind: "text", text: "Late" }] }, finishReason: "stop", usage: {}, providerRequestId: "chatcmpl-browser-late" });
	await assert.rejects(pending, /cannot finalize because its status is interrupted/);
	detail = await client.getRunHistory(runs[0].id);
	assert.equal(detail.status, "interrupted");
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
	});
	createProductShell(view, client);
	await openWorkspace();
	await createProvider({ name: "Primary", protocol: "openai-chat-completions", baseUrl: "https://api.example/v1", apiKey: "primary-secret" });
	await updateProvider({ ...settings.providers[0], name: "Renamed" });
	await setDefaultProvider("provider-uuid");
	await createModel({ providerId: "provider-uuid", displayName: "Fast", providerModelId: "model-fast" });
	await updateModel({ ...settings.providers[0].models[0], providerModelId: "model-fast-v2" });
	await setDefaultModel("provider-uuid", "model-uuid");
	await deleteModel("provider-uuid", "model-uuid");
	await deleteProvider({ providerId: "provider-uuid", confirmed: true });

	assert.deepEqual(calls.filter((call) => Array.isArray(call)).map((call) => call[0]), [
		"create-provider", "update-provider", "default-provider", "create-model", "update-model", "default-model", "delete-model", "delete-provider",
	]);
	assert.equal(rendered.length, 9);
	assert.deepEqual(rendered.at(-1), settings);
});

test("the Node editor renders a Model Slot selector for agent Nodes", async () => {
	const document = {
		createElement(tag) {
			return {
				tag, children: [], listeners: {}, value: "", checked: false, open: true,
				append(...children) { this.children.push(...children); },
				addEventListener(event, handler) { this.listeners[event] = handler; },
				setAttribute() {},
				focus() { this.focused = true; },
			};
		},
	};
	const container = () => ({ ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } });
	const configForm = container();
	const inputForm = container();
	const controlForm = container();
	const modelEdits = [];
	const view = createProductDOMView({
		title: { textContent: "" }, message: { textContent: "" }, status: { textContent: "", dataset: {} },
		button: { addEventListener() {} }, form: { addEventListener() {} }, nameInput: {}, workflowList: container(),
		draftEditor: { addEventListener() {} }, draftStatus: {}, diagnosticList: container(),
		nodeEditor: { hidden: true }, nodeEditorStatus: {}, nodeName: { addEventListener() {} }, removeNodeButton: { addEventListener() {} },
		nodeConfigForm: configForm, nodeInputForm: inputForm, nodeControlForm: controlForm,
	}, productStatusMessage);
	view.onEditNodeModel((edit) => modelEdits.push(structuredClone(edit)));
	view.renderNodeEditor({
		node: { id: "answer", definition: "llm-chat", executor: "v1", displayName: "Answer", config: {}, inputs: {}, llm: { modelUuid: "deleted-model-uuid" }, dependsOn: [] },
		fields: [], inputs: { conversation: { type: "Conversation" } },
		modelChoices: [
			{ value: "model-uuid", displayName: "Primary · Fast (model-fast)" },
		],
		inputSources: [], controlNodes: [], focus: { section: "llm", field: "modelUuid" },
	});

	// The selector lists the live Model Slots plus the dangling UUID so the
	// current selection stays visible until the user re-selects.
	const selector = configForm.items[0].children[1];
	assert.deepEqual(selector.children.map((option) => option.textContent), ["Use default at Run", "Primary · Fast (model-fast)", "Deleted model deleted-model-uuid"]);
	assert.equal(selector.value, "deleted-model-uuid");
	assert.equal(selector.focused, true);

	selector.value = "model-uuid";
	await selector.listeners.change();
	assert.deepEqual(modelEdits, [{ nodeId: "answer", modelUuid: "model-uuid" }]);

	// Human Nodes do not get a Model selector at all.
	view.renderNodeEditor({
		node: { id: "prompt", definition: "human-chat", executor: "v1", displayName: "Prompt", config: {}, inputs: {}, dependsOn: [] },
		fields: [], inputs: {}, modelChoices: [], inputSources: [], controlNodes: [],
	});
	assert.deepEqual(configForm.items, []);
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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
	const runInputLabel = { textContent: "" };
	const runInput = { value: "", dataset: {} };
	const artifactList = { ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } };
	const nodeRunList = { ownerDocument: document, items: [], replaceChildren(...items) { this.items = items; } };
	const view = createProductDOMView({
		title: {}, message: {}, status: { dataset: {} }, button: { addEventListener() {} }, form: { addEventListener() {} }, nameInput: {},
		workflowList: { replaceChildren() {} }, draftEditor: { addEventListener() {} }, draftStatus: {}, diagnosticList: { replaceChildren() {} },
		runButton, runStatus, runInputLabel, runInput, artifactList, nodeRunList,
	}, productStatusMessage);
	let submitted;
	view.onStartRun((humanInput) => { submitted = humanInput; });
	view.renderDraft({ draft: {
		...expectedDraft,
		content: { semanticSchemaVersion: "productWorkflow/v1", nodes: [
			{ id: "unused", definition: "human-chat", displayName: "Unused human" },
			{ id: "prompt", definition: "human-chat", displayName: "Question" },
			{ id: "answer", definition: "llm-chat", inputs: { conversation: { from: "prompt.conversation" } } },
		] },
	} });
	runInput.value = "  Explain the application seam.\n";
	await runButton.listeners.click();
	view.renderRun(expectedRun);

	assert.deepEqual(submitted, { nodeId: "prompt", text: "  Explain the application seam.\n" });
	assert.equal(runInputLabel.textContent, "Question input");
	assert.equal(runStatus.textContent, "Run succeeded · revision revision-uuid");
	assert.deepEqual(nodeRunList.items.map((item) => item.textContent), ["prompt · human-chat@v1 · node run human-run · succeeded", "answer · llm-chat@v1 · node run agent-run · succeeded"]);
	assert.equal(nodeRunList.items[1].children[0].textContent, "request chatcmpl-1 · finish stop · tokens 12 in / 7 out / 19 total");
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
	assert.deepEqual(historyNodeRunList.items.map((item) => item.textContent), ["prompt · human-chat@v1 · node run human-run · succeeded", "answer · llm-chat@v1 · node run agent-run · succeeded"]);
	assert.equal(historyNodeRunList.items[1].children[0].textContent, "request chatcmpl-1 · finish stop · tokens 12 in / 7 out / 19 total");
	assert.equal(historyArtifactList.items[0].children[1].children[1].textContent, "assistant: Fake response");

	const failed = structuredClone(expectedRun);
	failed.status = "failed";
	failed.error = { kind: "structural", code: "authentication", message: "openai-compatible request failed: authentication (status 401)", userAction: "check the Provider API Key and start a new Run" };
	failed.nodeRuns[1].status = "failed";
	failed.nodeRuns[1].startedAt = "2026-09-02T10:00:00Z";
	failed.nodeRuns[1].finishedAt = "2026-09-02T10:00:01Z";
	failed.nodeRuns[1].diagnostics = { error: failed.error };
	view.renderHistoryRun(failed);
	assert.match(historyRunStatus.textContent, /authentication.*check the Provider API Key/);
	assert.match(historyNodeRunList.items[1].textContent, /agent-run/);
	assert.equal(historyNodeRunList.items[1].children[0].textContent, "structural / authentication · openai-compatible request failed: authentication (status 401) · check the Provider API Key and start a new Run");
	assert.equal(historyNodeRunList.items[1].children[1].textContent, "2026-09-02T10:00:00Z → 2026-09-02T10:00:01Z");

	// A historical Run whose Model Slot was deleted still names the Provider
	// and Provider Model ID fixed in its Run Snapshot.
	const dangling = structuredClone(failed);
	dangling.error = undefined;
	dangling.status = "succeeded";
	dangling.snapshot = { executors: [], llmSelections: [{ nodeId: "answer", providerId: "provider-uuid", providerName: "Primary", protocol: "openai-chat-completions", dialect: "developer", baseUrl: "https://api.example/v1", modelUuid: "deleted-model-uuid", providerModelId: "model-fast" }] };
	view.renderHistoryRun(dangling);
	assert.match(historyRunStatus.textContent, /answer: Primary \(model-fast\)/);
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
	const confirmedMessages = [];
	const view = createProductDOMView({
		title: {}, message: {}, status: { dataset: {} }, button: { addEventListener() {} }, form: { addEventListener() {} }, nameInput: {},
		workflowList: container(), draftEditor: { addEventListener() {} }, draftStatus: {}, diagnosticList: container(),
		llmProviderList: providerList, llmDiagnosticList: diagnosticList,
	}, productStatusMessage, { confirmDelete: (message) => { confirmedMessages.push(message); return true; } });
	view.onUpdateLLMProvider((input) => calls.push(["provider", input]));
	view.onSetDefaultLLMProvider((id) => calls.push(["provider-default", id]));
	view.onListProviderDeletionImpact(() => ({ workflows: [{ id: "workflow-uuid", displayName: "Release checklist", nodeId: "answer", nodeDefinition: "llm-chat", modelUuid: "model-uuid" }], modelSlots: [{ id: "model-uuid", displayName: "Fast", providerModelId: "model-fast" }], diagnostics: [] }));
	view.onDeleteLLMProvider((id) => calls.push(["provider-delete", id]));
	view.onCreateLLMModel((input) => calls.push(["model-create", input]));
	view.onUpdateLLMModel((input) => calls.push(["model", input]));
	view.onListModelDeletionImpact(() => ({ workflows: [], modelSlots: [], diagnostics: [] }));
	view.onSetDefaultLLMModel((providerId, id) => calls.push(["model-default", providerId, id]));
	view.onDeleteLLMModel((providerId, id) => calls.push(["model-delete", providerId, id]));
	view.renderLLMSettings(expectedSettings);

	const card = providerList.items[0];
	const providerControls = card.children[0].children;
	providerControls[0].value = "Renamed";
	const dialect = providerControls.find((control) => control.value === "developer");
	assert.ok(dialect, "Provider settings should render its instructions dialect");
	dialect.value = "system";
	await providerControls.find((control) => control.textContent === "Save Provider").listeners.click();
	await providerControls.find((control) => control.textContent === "Effective default").listeners.click();
	await providerControls.find((control) => control.textContent === "Delete Provider").listeners.click();
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

	assert.deepEqual(calls.map((call) => call[0]), ["provider", "provider-default", "provider-delete", "model", "model-create"]);
	assert.equal(calls[0][1].id, "provider-uuid");
	assert.equal(calls[0][1].dialect, "system");
	const keyControl = providerControls.find((control) => control.type === "password");
	assert.equal(keyControl.value, "");
	assert.deepEqual(calls[2][1], { providerId: "provider-uuid", confirmed: true });
	// The Provider confirmation shows the affected Model Slots and Workflows
	// fetched through the deletion-impact seam before the destructive call.
	assert.match(confirmedMessages[0], /Delete Provider Primary and its saved API Key\?/);
	assert.match(confirmedMessages[0], /This removes 1 Model Slot\(s\):/);
	assert.match(confirmedMessages[0], /Fast \(model-fast\)/);
	assert.match(confirmedMessages[0], /Release checklist · llm-chat "answer"/);
	// The Model confirmation reports no referencing workflows when none exist.
	await modelControls[6].listeners.click();
	assert.equal(calls.at(-1)[0], "model-delete");
	assert.match(confirmedMessages[1], /Delete Model Fast from Provider Primary\?/);
	assert.match(confirmedMessages[1], /No current Workflow Draft references these Model\(s\)\./);
	assert.equal(calls[3][1].providerModelId, "model-fast-v2");
	assert.deepEqual(calls[3][1].generationDefaults, { temperature: 0.5, maxOutputTokens: 2048 });
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
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
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

test("the Browser Mock startRun drives one real fixture model call and persists diagnostics", async () => {
	let selectWorkflow;
	let startRun;
	let renderedRun;
	let draft = structuredClone(expectedDraft);
	const secrets = createMemorySecretAdapter();
	const settings = createBrowserLLMSettings({ newID: () => "provider-uuid", now: () => "2026-09-01T00:00:00Z", secrets });
	settings.createProvider({ name: "Primary", protocol: "openai-chat-completions", dialect: "system", baseUrl: "https://api.example/v1", apiKey: "sk-browser-run-secret" });
	settings.createModel({ providerId: "provider-uuid", displayName: "Fast", providerModelId: "model-fast", generationDefaults: {} });
	draft = {
		...draft,
		content: {
			semanticSchemaVersion: "productWorkflow/v1",
			nodes: [
				{ id: "prompt", definition: "human-chat", executor: "v1", displayName: "Prompt", config: {} },
				{ id: "answer", definition: "llm-chat", executor: "v1", displayName: "Answer", config: { instructions: "Answer tersely." }, inputs: { conversation: { from: "prompt.conversation" } } },
			],
		},
	};
	const requests = [];
	// The shared browser fixture adapter resolves the real Secret reference,
	// proving the call carries the stored credential and nothing else.
	const chatWithSecrets = createFixtureChatAdapter({
		secrets,
		requests,
		responses: [{ assistantText: "Real fixture response.", finishReason: "stop", usage: { inputTokens: 12, outputTokens: 7, totalTokens: 19 }, providerRequestId: "chatcmpl-fixture-1" }],
	});
	const runResult = { ...structuredClone(expectedRun), draft: structuredClone(draft) };
	const view = {
		onOpenWorkspace() {}, onCreateWorkflow() {}, onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty() {}, onEditDraft() {}, onStartRun(handler) { startRun = handler; },
		render() {}, renderDraft() {}, renderDraftLoading() {}, renderNodeEditor() {},
		renderRun(run) { renderedRun = run; }, renderRevisions() {}, renderRevisionRuns() {}, renderHistoryRun() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() { return expectedView; }, async createWorkflow() {}, async listWorkflows() { return []; },
		async getDraft() { return structuredClone(draft); }, async updateDraft() {},
		async getLLMSettings() { return settings.getSettings(); },
		async createLLMProvider(input) { return settings.createProvider(input); },
		async createLLMModel(input) { return settings.createModel(input); },
		async startRun(input) {
			const materialized = structuredClone(draft.content);
			const provider = settings.getSettings().providers[0];
			const model = provider.models[0];
			for (const node of materialized.nodes) {
				if (node.definition === "llm-chat") node.llm = { modelUuid: model.id };
			}
			const agent = materialized.nodes.find((node) => node.definition === "llm-chat");
			const result = chatWithSecrets.generate(
				{ protocol: provider.protocol, dialect: provider.dialect, baseUrl: provider.baseUrl, providerModelId: model.providerModelId, apiKeyRef: settings.referenceFor(provider.id) },
				{
					model: model.providerModelId,
					instructions: [{ kind: "text", text: agent.config.instructions }],
					messages: [{ role: "user", parts: [{ kind: "text", text: input.humanInput.text }] }],
					config: {},
				},
			);
			return {
				...structuredClone(runResult),
				draft: { ...structuredClone(draft), content: materialized },
				snapshot: { executors: [], llmSelections: [{ nodeId: "answer", providerId: provider.id, providerName: provider.name, protocol: provider.protocol, dialect: provider.dialect, baseUrl: provider.baseUrl, modelUuid: model.id, providerModelId: model.providerModelId }] },
				nodeRuns: [
					{ id: "human-run", nodeId: "prompt", nodeDefinition: "human-chat", nodeExecutor: "v1", status: "succeeded" },
					{ id: "agent-run", nodeId: "answer", nodeDefinition: "llm-chat", nodeExecutor: "v1", status: "succeeded", diagnostics: { providerRequestId: result.providerRequestId, finishReason: result.finishReason, usage: result.usage } },
				],
				artifacts: [{ id: "artifact-uuid", nodeId: "answer", port: "conversation", type: "Conversation", version: "2", uri: "2.json", messages: [{ role: "user", text: input.humanInput.text }, { role: "assistant", text: result.assistant.parts[0].text }] }],
			};
		},
		async listModelDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
		async listProviderDeletionImpact() { return { workflows: [], modelSlots: [], diagnostics: [] }; },
	});
	createProductShell(view, client);
	await selectWorkflow(expectedWorkflow.id);
	await startRun({ nodeId: "prompt", text: "  Browser submitted text.\n" });
	const run = renderedRun;

	// The fixture saw exactly one authenticated canonical call.
	assert.deepEqual(requests, [{
		authorization: "Bearer sk-browser-run-secret",
		baseUrl: "https://api.example/v1",
		model: "model-fast",
		instructionsRole: "system",
		instructions: "Answer tersely.",
		messages: [{ role: "user", text: "  Browser submitted text.\n" }],
		config: {},
	}]);
	assert.equal(run.nodeRuns[1].diagnostics.providerRequestId, "chatcmpl-fixture-1");
	assert.equal(run.nodeRuns[1].diagnostics.finishReason, "stop");
	assert.deepEqual(run.nodeRuns[1].diagnostics.usage, { inputTokens: 12, outputTokens: 7, totalTokens: 19 });
	assert.equal(run.artifacts[0].messages[1].text, "Real fixture response.");
	assert.equal(run.snapshot.llmSelections[0].dialect, "system");
	assert.equal(JSON.stringify(run).includes("sk-browser-run-secret"), false);
});

test("the shell gives both clients the same user-facing status text", () => {
  assert.equal(productStatusMessage({ status: "idle" }), "Waiting for an action");
  assert.equal(productStatusMessage({ status: "loading" }), "Working…");
  assert.equal(productStatusMessage({ status: "ready" }), "Application round-trip complete");
  assert.equal(productStatusMessage({ status: "error" }), "Application error");
});
