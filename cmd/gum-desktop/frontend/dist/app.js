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
		{ title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList, nodeCatalogList, nodeList, nodeEditor, nodeEditorStatus, nodeName, removeNodeButton, nodeConfigForm, nodeInputForm, nodeControlForm, previewCanvas, previewEdges, previewGroups, previewZoomIn, previewZoomOut, previewZoomReset, providerForm, providerName, providerProtocol, providerBaseURL, providerAPIKeyRef, llmProviderList, llmDiagnosticList, runButton, runStatus, nodeRunList, artifactList, historyRefreshButton, revisionList, revisionRunList, historyRunStatus, historyNodeRunList, historyArtifactList },
    productStatusMessage,
  ),
  client,
);
