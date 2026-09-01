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
	requireMethod(application, "getDraft", "browser application");
	requireMethod(application, "updateDraft", "browser application");
	const listNodeCatalog = typeof application.listNodeCatalog === "function"
		? () => application.listNodeCatalog()
		: async () => [];
	const getLLMSettings = typeof application.getLLMSettings === "function"
		? () => application.getLLMSettings()
		: async () => ({ providers: [], diagnostics: [] });
	const startRun = typeof application.startRun === "function"
		? (input) => application.startRun(input)
		: async () => { throw new Error("browser application must provide startRun()"); };
	const listRevisions = typeof application.listRevisions === "function"
		? (workflowId) => application.listRevisions(workflowId)
		: async () => [];
	const listRevisionRuns = typeof application.listRevisionRuns === "function"
		? (revisionId) => application.listRevisionRuns(revisionId)
		: async () => [];
	const getRunHistory = typeof application.getRunHistory === "function"
		? (runId) => application.getRunHistory(runId)
		: async () => { throw new Error("browser application must provide getRunHistory()"); };
  return {
    openWorkspace: () => application.openWorkspace(),
    createWorkflow: (input) => application.createWorkflow(input),
    listWorkflows: () => application.listWorkflows(),
		getDraft: (workflowId) => application.getDraft(workflowId),
		updateDraft: (input) => application.updateDraft(input),
		startRun,
		listRevisions,
		listRevisionRuns,
		getRunHistory,
		listNodeCatalog,
		getLLMSettings,
		createLLMProvider: (input) => application.createLLMProvider(input),
		updateLLMProvider: (input) => application.updateLLMProvider(input),
		deleteLLMProvider: (input) => application.deleteLLMProvider(input),
		setDefaultLLMProvider: (providerId) => application.setDefaultLLMProvider(providerId),
		createLLMModel: (input) => application.createLLMModel(input),
		updateLLMModel: (input) => application.updateLLMModel(input),
		deleteLLMModel: (providerId, modelId) => application.deleteLLMModel(providerId, modelId),
		setDefaultLLMModel: (providerId, modelId) => application.setDefaultLLMModel(providerId, modelId),
  };
}

export function createDesktopWorkflowClient(desktopAdapter) {
  requireMethod(desktopAdapter, "OpenWorkspace", "desktop adapter");
  requireMethod(desktopAdapter, "CreateWorkflow", "desktop adapter");
  requireMethod(desktopAdapter, "ListWorkflows", "desktop adapter");
	requireMethod(desktopAdapter, "GetDraft", "desktop adapter");
	requireMethod(desktopAdapter, "UpdateDraft", "desktop adapter");
	requireMethod(desktopAdapter, "ListNodeCatalog", "desktop adapter");
	requireMethod(desktopAdapter, "StartRun", "desktop adapter");
	for (const method of ["GetLLMSettings", "CreateLLMProvider", "UpdateLLMProvider", "DeleteLLMProvider", "SetDefaultLLMProvider", "CreateLLMModel", "UpdateLLMModel", "DeleteLLMModel", "SetDefaultLLMModel", "ListRevisions", "ListRevisionRuns", "GetRunHistory"]) {
		requireMethod(desktopAdapter, method, "desktop adapter");
	}
  return {
    openWorkspace: () => desktopAdapter.OpenWorkspace(),
    createWorkflow: (input) => desktopAdapter.CreateWorkflow(input),
    listWorkflows: () => desktopAdapter.ListWorkflows(),
		getDraft: (workflowId) => desktopAdapter.GetDraft(workflowId),
		updateDraft: (input) => desktopAdapter.UpdateDraft(input),
		startRun: (input) => desktopAdapter.StartRun(input),
		listRevisions: (workflowId) => desktopAdapter.ListRevisions(workflowId),
		listRevisionRuns: (revisionId) => desktopAdapter.ListRevisionRuns(revisionId),
		getRunHistory: (runId) => desktopAdapter.GetRunHistory(runId),
		listNodeCatalog: () => desktopAdapter.ListNodeCatalog(),
		getLLMSettings: () => desktopAdapter.GetLLMSettings(),
		createLLMProvider: (input) => desktopAdapter.CreateLLMProvider(input),
		updateLLMProvider: (input) => desktopAdapter.UpdateLLMProvider(input),
		deleteLLMProvider: (input) => desktopAdapter.DeleteLLMProvider(input),
		setDefaultLLMProvider: (providerId) => desktopAdapter.SetDefaultLLMProvider(providerId),
		createLLMModel: (input) => desktopAdapter.CreateLLMModel(input),
		updateLLMModel: (input) => desktopAdapter.UpdateLLMModel(input),
		deleteLLMModel: (providerId, modelId) => desktopAdapter.DeleteLLMModel(providerId, modelId),
		setDefaultLLMModel: (providerId, modelId) => desktopAdapter.SetDefaultLLMModel(providerId, modelId),
  };
}
