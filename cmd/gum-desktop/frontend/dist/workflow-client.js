function requireMethod(target, method, adapterName) {
  if (!target || typeof target[method] !== "function") {
    throw new TypeError(`${adapterName} must provide ${method}()`);
  }
}

// Both adapters return the same small object instead of exposing transport or
// desktop framework details to the product shell.
export function createBrowserWorkflowClient(application) {
  requireMethod(application, "openWorkspace", "browser application");
  return {
    openWorkspace: () => application.openWorkspace(),
  };
}

export function createDesktopWorkflowClient(desktopAdapter) {
  requireMethod(desktopAdapter, "OpenWorkspace", "desktop adapter");
  return {
    openWorkspace: () => desktopAdapter.OpenWorkspace(),
  };
}
