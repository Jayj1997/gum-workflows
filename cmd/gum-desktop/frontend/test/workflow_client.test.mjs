import assert from "node:assert/strict";
import test from "node:test";

import {
  createBrowserWorkflowClient,
  createDesktopWorkflowClient,
} from "../dist/workflow-client.js";
import { createProductDOMView } from "../dist/product-dom-view.js";
import { createProductShell, productStatusMessage } from "../dist/product-shell.js";

const expectedView = {
  title: "Gum Workflows",
  message: "Product application round-trip complete",
};
const expectedWorkflow = {
  id: "0198fb41-43d2-7e2b-a4cd-2bc5f7889ff9",
  displayName: "Release checklist",
  createdAt: "2026-08-31T09:00:00Z",
};

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
      }),
  ],
];

for (const [name, createClient] of clientContract) {
  test(`${name} follows the WorkflowClient contract`, async () => {
	const client = createClient();
	assert.deepEqual(await client.openWorkspace(), expectedView);
	assert.deepEqual(await client.createWorkflow({ displayName: "Release checklist" }), expectedWorkflow);
	assert.deepEqual(await client.listWorkflows(), [expectedWorkflow]);
  });
}

test("a user action crosses WorkflowClient and renders the visible result", async () => {
	const renderStates = [];
	const workflowStates = [];
	let openWorkspace;
	const view = {
		onOpenWorkspace(handler) {
			openWorkspace = handler;
		},
		onCreateWorkflow() {},
		render(state) {
			renderStates.push(structuredClone(state));
		},
		renderWorkflows(workflows) {
			workflowStates.push(structuredClone(workflows));
		},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() {
			return expectedView;
		},
		async createWorkflow() {},
		async listWorkflows() {
			return [expectedWorkflow];
		},
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
		render() {},
		renderWorkflows(items) {
			rendered.push(structuredClone(items));
		},
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
		render(state) {
			renderStates.push(structuredClone(state));
		},
		renderWorkflows() {},
	};
	const client = createBrowserWorkflowClient({
		async openWorkspace() {
			throw new Error("backend unavailable");
		},
		async createWorkflow() {},
		async listWorkflows() {
			return [];
		},
	});

  createProductShell(view, client);
  await openWorkspace();

  assert.deepEqual(renderStates.at(-1), {
    status: "error",
    title: "Gum Workflows",
    message: "backend unavailable",
  });
});

test("desktop and browser entries share one DOM view adapter", () => {
  const title = { textContent: "" };
  const message = { textContent: "" };
  const status = { textContent: "", dataset: {} };
	const button = { disabled: false, addEventListener() {} };
	const form = { addEventListener() {}, reset() {} };
	const nameInput = { value: "Release checklist" };
	const workflowList = { replaceChildren() {} };
	const view = createProductDOMView(
		{ title, message, status, button, form, nameInput, workflowList },
		productStatusMessage,
	);

  view.render({ status: "ready", title: "Gum Workflows", message: "ready" });

  assert.equal(title.textContent, "Gum Workflows");
  assert.equal(message.textContent, "ready");
  assert.equal(status.textContent, "Application round-trip complete");
  assert.equal(status.dataset.state, "ready");
  assert.equal(button.disabled, false);
});

test("the shell gives both clients the same user-facing status text", () => {
  assert.equal(productStatusMessage({ status: "idle" }), "Waiting for an action");
  assert.equal(productStatusMessage({ status: "loading" }), "Working…");
  assert.equal(productStatusMessage({ status: "ready" }), "Application round-trip complete");
  assert.equal(productStatusMessage({ status: "error" }), "Application error");
});
