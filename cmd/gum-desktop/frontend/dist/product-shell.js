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

export function createProductShell(view, client, options = {}) {
	let currentDraft;
	let nodeCatalog = [];
	let selectedNodeId = "";
	let editSequence = 0;
	let selectionSequence = 0;
	let saveQueue = Promise.resolve();
	let refreshRequired = false;
  async function refreshWorkflows() {
    view.renderWorkflows(await client.listWorkflows());
  }
	const createNodeId = options.createNodeId ?? (() => crypto.randomUUID());
	function renderSelectedNode() {
		if (!view.renderNodeEditor) return;
		const node = currentDraft?.content?.nodes?.find((candidate) => candidate.id === selectedNodeId);
		const entry = nodeCatalog.find((candidate) => candidate.definition.id === node?.definition);
		view.renderNodeEditor(node && entry ? { node, fields: entry.definition.config.fields } : undefined);
	}
	async function queueDraftEdit(editRequest) {
		if (!currentDraft) return;
		if (editRequest.workflowId !== currentDraft.workflowId) return;
		editSequence = Math.max(editSequence, editRequest.revision);
		const edit = editRequest.revision;
		const selection = selectionSequence;
		const workflowId = editRequest.workflowId;
		const queued = saveQueue.then(async () => {
			if (selection !== selectionSequence || refreshRequired) return;
			try {
				const content = typeof editRequest.content === "function"
					? editRequest.content(structuredClone(currentDraft.content))
					: editRequest.content;
				const result = await client.updateDraft({
					workflowId,
					expectedLockVersion: currentDraft.lockVersion,
					content,
				});
				if (selection !== selectionSequence || currentDraft?.workflowId !== workflowId) return;
				currentDraft = result.draft;
				refreshRequired = result.refreshRequired;
				if (result.conflict || edit === editSequence) {
					view.renderDraft(result);
					renderSelectedNode();
				}
				return result;
			} catch (error) {
				view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
			}
		});
		saveQueue = queued.then(() => undefined, () => undefined);
		return queued;
	}
	async function editNodes(mutator) {
		if (!currentDraft || refreshRequired) return;
		await view.flushDraftEdit?.();
		if (!currentDraft || refreshRequired) return;
		const revision = ++editSequence;
		return queueDraftEdit({
			workflowId: currentDraft.workflowId,
			revision,
			content(content) {
				if (!Array.isArray(content.nodes)) content.nodes = [];
				mutator(content.nodes);
				return content;
			},
		});
	}

  view.render(initialState);
  view.onOpenWorkspace(async () => {
    view.render({
      status: "loading",
      title: "Gum Workflows",
      message: "Calling the product application…",
    });
    try {
			const [workspace, catalog] = await Promise.all([client.openWorkspace(), client.listNodeCatalog()]);
			nodeCatalog = catalog;
			view.renderNodeCatalog?.(nodeCatalog);
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
		selectedNodeId = "";
		view.renderDraftLoading();
		try {
			const draft = await client.getDraft(workflowId);
			if (selection !== selectionSequence) return;
			if (draft.workflowId !== workflowId) throw new Error("draft response does not match selected workflow");
			currentDraft = draft;
			view.renderDraft({ draft: currentDraft, preview: currentDraft.preview });
			renderSelectedNode();
		} catch (error) {
			view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
		}
	});
	view.onDraftDirty(({ workflowId, revision }) => {
		if (currentDraft?.workflowId === workflowId) editSequence = Math.max(editSequence, revision);
	});
	view.onEditDraft(queueDraftEdit);
	view.onAddNode?.(async (definitionId) => {
		const entry = nodeCatalog.find((candidate) => candidate.definition.id === definitionId);
		if (!entry) throw new Error(`Node Definition ${definitionId} is not in the Catalog`);
		const nodeId = createNodeId();
		const config = {};
		for (const field of entry.definition.config.fields) {
			if (field.hasDefault) config[field.name] = structuredClone(field.default);
		}
		selectedNodeId = nodeId;
		await editNodes((nodes) => nodes.push({
			id: nodeId,
			definition: entry.definition.id,
			executor: entry.executor.version,
			displayName: entry.definition.displayName,
			config,
		}));
	});
	view.onSelectNode?.((nodeId) => {
		selectedNodeId = nodeId;
		renderSelectedNode();
	});
	view.onRenameNode?.(async ({ nodeId, displayName }) => {
		await editNodes((nodes) => {
			const node = nodes.find((candidate) => candidate.id === nodeId);
			if (node && displayName.trim()) node.displayName = displayName.trim();
		});
	});
	view.onEditNodeConfig?.(async ({ nodeId, field, value, remove }) => {
		await editNodes((nodes) => {
			const node = nodes.find((candidate) => candidate.id === nodeId);
			if (!node) return;
			if (!node.config || typeof node.config !== "object") node.config = {};
			if (remove) delete node.config[field];
			else node.config[field] = value;
		});
	});
	view.onRemoveNode?.(async (nodeId) => {
		await editNodes((nodes) => {
			const index = nodes.findIndex((candidate) => candidate.id === nodeId);
			if (index >= 0) nodes.splice(index, 1);
		});
		if (selectedNodeId === nodeId) selectedNodeId = "";
		renderSelectedNode();
	});
}
