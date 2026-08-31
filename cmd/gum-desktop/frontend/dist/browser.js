import { createBrowserWorkflowClient } from "./workflow-client.js";
import { createProductDOMView } from "./product-dom-view.js";
import { createProductShell, productStatusMessage } from "./product-shell.js";

const workflows = [];
const drafts = new Map();

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

function diagnosticsFor(content) {
	const diagnostics = [];
	if (content.semanticSchemaVersion !== "productWorkflow/v1") {
		diagnostics.push({ code: "invalid-semantic-schema-version", severity: "error", path: "semanticSchemaVersion", message: "semantic schema version must be productWorkflow/v1" });
	}
	if (!Array.isArray(content.nodes) || content.nodes.length === 0) {
		diagnostics.push({ code: "workflow-needs-node", severity: "error", path: "nodes", message: "workflow must contain at least one node" });
	}
	return diagnostics;
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
	async getDraft(workflowId) {
		return structuredClone(drafts.get(workflowId));
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
			preview: {
				nodes: [], edges: [], groups: [],
				diagnostics: diagnosticsFor(current.content),
			},
			saved,
			conflict,
			refreshRequired: conflict,
		};
	},
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

createProductShell(
  createProductDOMView(
		{ title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList },
    productStatusMessage,
  ),
  client,
);
