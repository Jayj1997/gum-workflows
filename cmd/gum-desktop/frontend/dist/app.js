import { createDesktopWorkflowClient } from "./workflow-client.js";
import { createProductDOMView } from "./product-dom-view.js";
import { createProductShell, productStatusMessage } from "./product-shell.js";

const desktopBinding = window.go?.main?.DesktopAdapter;
const client = createDesktopWorkflowClient(desktopBinding);
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

createProductShell(
  createProductDOMView(
		{ title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList, nodeCatalogList, nodeList, nodeEditor, nodeEditorStatus, nodeName, removeNodeButton, nodeConfigForm },
    productStatusMessage,
  ),
  client,
);
