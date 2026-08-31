import { createDesktopWorkflowClient } from "./workflow-client.js";
import { createProductDOMView } from "./product-dom-view.js";
import { createProductShell, productStatusMessage } from "./product-shell.js";

const desktopBinding = window.go?.main?.DesktopAdapter;
const client = createDesktopWorkflowClient(desktopBinding);
const title = document.querySelector("#title");
const message = document.querySelector("#message");
const status = document.querySelector("#status");
const button = document.querySelector("#open-workspace");

createProductShell(
  createProductDOMView(
    { title, message, status, button },
    productStatusMessage,
  ),
  client,
);
