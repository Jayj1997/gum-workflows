import { createBrowserWorkflowClient } from "./workflow-client.js";
import { createProductDOMView } from "./product-dom-view.js";
import { createProductShell, productStatusMessage } from "./product-shell.js";

const client = createBrowserWorkflowClient({
  async openWorkspace() {
    return {
      title: "Gum Workflows",
      message: "Product application round-trip complete",
    };
  },
});
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
