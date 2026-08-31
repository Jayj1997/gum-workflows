const initialState = {
  status: "idle",
  title: "Gum Workflows",
  message: "Open the local product workspace to verify the application seam.",
};

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

export function productStatusMessage(state) {
  switch (state.status) {
    case "ready":
      return "Application round-trip complete";
    case "loading":
      return "Working…";
    case "error":
      return "Application error";
    default:
      return "Waiting for an action";
  }
}

export function createProductShell(view, client) {
  async function refreshWorkflows() {
    view.renderWorkflows(await client.listWorkflows());
  }

  view.render(initialState);
  view.onOpenWorkspace(async () => {
    view.render({
      status: "loading",
      title: "Gum Workflows",
      message: "Calling the product application…",
    });
    try {
      const workspace = await client.openWorkspace();
      await refreshWorkflows();
      view.render({ status: "ready", ...workspace });
    } catch (error) {
      view.render({
        status: "error",
        title: "Gum Workflows",
        message: errorMessage(error),
      });
    }
  });
  view.onCreateWorkflow(async (displayName) => {
    view.render({
      status: "loading",
      title: "Gum Workflows",
      message: "Creating Product Workflow…",
    });
    try {
      await client.createWorkflow({ displayName });
      await refreshWorkflows();
      view.render({
        status: "ready",
        title: "Gum Workflows",
        message: `Created “${displayName}” in the local product database.`,
      });
    } catch (error) {
      view.render({
        status: "error",
        title: "Gum Workflows",
        message: errorMessage(error),
      });
    }
  });
}
