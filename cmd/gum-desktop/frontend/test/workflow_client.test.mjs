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

const clientContract = [
  [
    "browser mock",
    () =>
      createBrowserWorkflowClient({
        async openWorkspace() {
          return expectedView;
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
      }),
  ],
];

for (const [name, createClient] of clientContract) {
  test(`${name} follows the WorkflowClient contract`, async () => {
    assert.deepEqual(await createClient().openWorkspace(), expectedView);
  });
}

test("a user action crosses WorkflowClient and renders the visible result", async () => {
  const renderStates = [];
  let openWorkspace;
  const view = {
    onOpenWorkspace(handler) {
      openWorkspace = handler;
    },
    render(state) {
      renderStates.push(structuredClone(state));
    },
  };
  const client = createBrowserWorkflowClient({
    async openWorkspace() {
      return expectedView;
    },
  });

  createProductShell(view, client);
  await openWorkspace();

  assert.deepEqual(renderStates.at(-1), {
    status: "ready",
    title: "Gum Workflows",
    message: "Product application round-trip complete",
  });
});

test("application failures become visible without leaking adapter details", async () => {
  const renderStates = [];
  let openWorkspace;
  const view = {
    onOpenWorkspace(handler) {
      openWorkspace = handler;
    },
    render(state) {
      renderStates.push(structuredClone(state));
    },
  };
  const client = createBrowserWorkflowClient({
    async openWorkspace() {
      throw new Error("backend unavailable");
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
  const view = createProductDOMView(
    { title, message, status, button },
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
