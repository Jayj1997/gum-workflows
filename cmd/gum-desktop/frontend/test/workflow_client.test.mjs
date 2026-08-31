import assert from "node:assert/strict";
import test from "node:test";

import {
  createBrowserWorkflowClient,
  createDesktopWorkflowClient,
} from "../dist/workflow-client.js";
import { createProductDOMView } from "../dist/product-dom-view.js";
import { createProductShell, productStatusMessage } from "../dist/product-shell.js";
import { createBuiltinNodeRegistry, validateConfig } from "../dist/node-registry.js";

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
		async listNodeCatalog() { return expectedCatalog; },
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
		async ListNodeCatalog() { return expectedCatalog; },
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
	assert.deepEqual(await client.listNodeCatalog(), expectedCatalog);
  });
}

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
