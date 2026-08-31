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
  view.render(initialState);
  view.onOpenWorkspace(async () => {
    view.render({
      status: "loading",
      title: "Gum Workflows",
      message: "Calling the product application…",
    });
    try {
      const workspace = await client.openWorkspace();
      view.render({ status: "ready", ...workspace });
    } catch (error) {
      view.render({
        status: "error",
        title: "Gum Workflows",
        message: errorMessage(error),
      });
    }
  });
}
