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
	let currentDraft;
	let editSequence = 0;
	let selectionSequence = 0;
	let saveQueue = Promise.resolve();
	let refreshRequired = false;
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
	view.onSelectWorkflow(async (workflowId) => {
		const selection = ++selectionSequence;
		editSequence = 0;
		refreshRequired = false;
		saveQueue = Promise.resolve();
		currentDraft = undefined;
		view.renderDraftLoading();
		try {
			const draft = await client.getDraft(workflowId);
			if (selection !== selectionSequence) return;
			if (draft.workflowId !== workflowId) throw new Error("draft response does not match selected workflow");
			currentDraft = draft;
			view.renderDraft({ draft: currentDraft });
		} catch (error) {
			view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
		}
	});
	view.onDraftDirty(({ workflowId, revision }) => {
		if (currentDraft?.workflowId === workflowId) editSequence = Math.max(editSequence, revision);
	});
	view.onEditDraft((editRequest) => {
		if (!currentDraft) return;
		if (editRequest.workflowId !== currentDraft.workflowId) return;
		editSequence = Math.max(editSequence, editRequest.revision);
		const edit = editRequest.revision;
		const selection = selectionSequence;
		const workflowId = editRequest.workflowId;
		const queued = saveQueue.then(async () => {
			if (selection !== selectionSequence || refreshRequired) return;
			try {
				const result = await client.updateDraft({
					workflowId,
					expectedLockVersion: currentDraft.lockVersion,
					content: editRequest.content,
				});
				if (selection !== selectionSequence || currentDraft?.workflowId !== workflowId) return;
				currentDraft = result.draft;
				refreshRequired = result.refreshRequired;
				if (result.conflict || edit === editSequence) view.renderDraft(result);
				return result;
			} catch (error) {
				view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
			}
		});
		saveQueue = queued.then(() => undefined, () => undefined);
		return queued;
	});
}
