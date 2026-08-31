function requireMethod(target, method, adapterName) {
  if (!target || typeof target[method] !== "function") {
    throw new TypeError(`${adapterName} must provide ${method}()`);
  }
}

// Both adapters return the same small object instead of exposing transport or
// desktop framework details to the product shell.
export function createBrowserWorkflowClient(application) {
  requireMethod(application, "openWorkspace", "browser application");
  requireMethod(application, "createWorkflow", "browser application");
  requireMethod(application, "listWorkflows", "browser application");
  return {
    openWorkspace: () => application.openWorkspace(),
    createWorkflow: (input) => application.createWorkflow(input),
    listWorkflows: () => application.listWorkflows(),
  };
}

export function createDesktopWorkflowClient(desktopAdapter) {
  requireMethod(desktopAdapter, "OpenWorkspace", "desktop adapter");
  requireMethod(desktopAdapter, "CreateWorkflow", "desktop adapter");
  requireMethod(desktopAdapter, "ListWorkflows", "desktop adapter");
  return {
    openWorkspace: () => desktopAdapter.OpenWorkspace(),
    createWorkflow: (input) => desktopAdapter.CreateWorkflow(input),
    listWorkflows: () => desktopAdapter.ListWorkflows(),
  };
}
