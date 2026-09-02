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
	let activeWorkflowId = "";
	let editSequence = 0;
	let selectionSequence = 0;
	let saveQueue = Promise.resolve();
	let refreshRequired = false;
  async function refreshWorkflows() {
    view.renderWorkflows(await client.listWorkflows());
  }
	async function refreshLLMSettings() {
		if (!view.renderLLMSettings || !client.getLLMSettings) return;
		view.renderLLMSettings(await client.getLLMSettings());
	}
	async function refreshRevisions() {
		if (!activeWorkflowId || !view.renderRevisions || !client.listRevisions) return;
		try {
			view.renderRevisions(await client.listRevisions(activeWorkflowId));
		} catch (error) {
			view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
		}
	}
	async function changeLLMSettings(action) {
		try {
			await action();
			await refreshLLMSettings();
		} catch (error) {
			view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
		}
	}
	const createNodeId = options.createNodeId ?? (() => crypto.randomUUID());
	function editorFocus(fieldPath) {
		const match = /^nodes\[\d+\]\.(inputs|config)\.([^.\[]+)/.exec(fieldPath ?? "");
		return match ? { section: match[1], field: match[2] } : undefined;
	}
	function renderSelectedNode(fieldPath) {
		if (!view.renderNodeEditor) return;
		const node = currentDraft?.content?.nodes?.find((candidate) => candidate.id === selectedNodeId);
		const entry = nodeCatalog.find((candidate) => candidate.definition.id === node?.definition);
		if (!node || !entry) {
			view.renderNodeEditor(undefined);
			return;
		}
		const otherNodes = (currentDraft.content.nodes ?? []).filter((candidate) => candidate.id !== node.id);
		const inputSources = otherNodes.flatMap((candidate) => {
			const source = nodeCatalog.find((item) => item.definition.id === candidate.definition)?.definition;
			return Object.entries(source?.outputs ?? {}).map(([port, contract]) => ({
				reference: `${candidate.id}.${port}`,
				type: contract.type,
				displayName: `${candidate.displayName || candidate.id} · ${port}`,
			}));
		});
		view.renderNodeEditor({
			node,
			fields: entry.definition.config.fields,
			inputs: entry.definition.inputs ?? {},
			inputSources,
			controlNodes: otherNodes.map((candidate) => ({ id: candidate.id, displayName: candidate.displayName || candidate.id })),
			focus: editorFocus(fieldPath),
		});
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
			const [workspace, catalog] = await Promise.all([client.openWorkspace(), client.listNodeCatalog(), refreshLLMSettings()]);
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
	view.onCreateLLMProvider?.((input) => changeLLMSettings(() => client.createLLMProvider(input)));
	view.onUpdateLLMProvider?.((input) => changeLLMSettings(() => client.updateLLMProvider(input)));
	view.onDeleteLLMProvider?.((input) => changeLLMSettings(() => client.deleteLLMProvider(input)));
	view.onSetDefaultLLMProvider?.((providerId) => changeLLMSettings(() => client.setDefaultLLMProvider(providerId)));
	view.onCreateLLMModel?.((input) => changeLLMSettings(() => client.createLLMModel(input)));
	view.onUpdateLLMModel?.((input) => changeLLMSettings(() => client.updateLLMModel(input)));
	view.onDeleteLLMModel?.((providerId, modelId) => changeLLMSettings(() => client.deleteLLMModel(providerId, modelId)));
	view.onSetDefaultLLMModel?.((providerId, modelId) => changeLLMSettings(() => client.setDefaultLLMModel(providerId, modelId)));
	view.onSelectWorkflow(async (workflowId) => {
		const selection = ++selectionSequence;
		editSequence = 0;
		refreshRequired = false;
		saveQueue = Promise.resolve();
		currentDraft = undefined;
		selectedNodeId = "";
		activeWorkflowId = workflowId;
		view.renderDraftLoading();
		view.renderRevisions?.([]);
		view.renderRevisionRuns?.([]);
		try {
			const draft = await client.getDraft(workflowId);
			if (selection !== selectionSequence) return;
			if (draft.workflowId !== workflowId) throw new Error("draft response does not match selected workflow");
			currentDraft = draft;
			view.renderDraft({ draft: currentDraft, preview: currentDraft.preview });
			renderSelectedNode();
			await refreshRevisions();
		} catch (error) {
			view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
		}
	});
	view.onDraftDirty(({ workflowId, revision }) => {
		if (currentDraft?.workflowId === workflowId) editSequence = Math.max(editSequence, revision);
	});
	view.onEditDraft(queueDraftEdit);
	view.onStartRun?.(async (humanInput) => {
		if (!currentDraft || refreshRequired) return;
		try {
			const flushed = await view.flushDraftEdit?.();
			if (flushed?.draft) currentDraft = flushed.draft;
			await saveQueue;
			if (!currentDraft || refreshRequired) return;
			const result = await client.startRun({
				workflowId: currentDraft.workflowId,
				expectedLockVersion: currentDraft.lockVersion,
				humanInput,
			});
			currentDraft = result.draft;
			view.renderDraft?.({ draft: currentDraft, preview: currentDraft.preview });
			view.renderRun?.(result);
			await refreshRevisions();
		} catch (error) {
			await refreshRevisions();
			view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
		}
	});
	view.onRefreshRevisions?.(refreshRevisions);
	view.onSelectRevision?.(async (revisionId) => {
		if (!view.renderRevisionRuns || !client.listRevisionRuns) return;
		try {
			view.renderRevisionRuns(await client.listRevisionRuns(revisionId));
		} catch (error) {
			view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
		}
	});
	view.onSelectRun?.(async (runId) => {
		if (!view.renderHistoryRun || !client.getRunHistory) return;
		try {
			view.renderHistoryRun(await client.getRunHistory(runId));
		} catch (error) {
			view.render({ status: "error", title: "Gum Workflows", message: errorMessage(error) });
		}
	});
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
	view.onSelectNode?.((nodeId, fieldPath) => {
		selectedNodeId = nodeId;
		renderSelectedNode(fieldPath);
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
	view.onBindNodeInput?.(async ({ nodeId, input, from }) => {
		await editNodes((nodes) => {
			const node = nodes.find((candidate) => candidate.id === nodeId);
			if (!node) return;
			if (!node.inputs || typeof node.inputs !== "object" || Array.isArray(node.inputs)) node.inputs = {};
			if (from) node.inputs[input] = { from };
			else delete node.inputs[input];
		});
	});
	view.onEditControlDependencies?.(async ({ nodeId, nodeIds }) => {
		await editNodes((nodes) => {
			const node = nodes.find((candidate) => candidate.id === nodeId);
			if (node) node.dependsOn = [...new Set(nodeIds)].sort();
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
