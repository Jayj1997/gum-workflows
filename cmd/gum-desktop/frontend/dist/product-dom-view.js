function fieldControl(document, field, value, onChange) {
	let control;
	if (field.type === "enum") {
		control = document.createElement("select");
		if (!field.required) {
			const empty = document.createElement("option");
			empty.value = "";
			empty.textContent = "Not set";
			control.append(empty);
		}
		for (const optionValue of field.values ?? []) {
			const option = document.createElement("option");
			option.value = optionValue;
			option.textContent = optionValue;
			control.append(option);
		}
	} else if (field.type === "markdown" || field.presentation.editor === "markdown") {
		control = document.createElement("textarea");
		control.rows = 5;
	} else {
		control = document.createElement("input");
		control.type = field.sensitive ? "password" : field.type === "boolean" ? "checkbox" : ["integer", "number"].includes(field.type) ? "number" : "text";
	}
	control.name = field.name;
	control.required = field.required;
	if (field.min !== undefined) control.min = String(field.min);
	if (field.max !== undefined) control.max = String(field.max);
	if (field.type === "integer") control.step = "1";
	if (field.type === "number") control.step = "any";
	if (field.type === "boolean") control.checked = value ?? false;
	else control.value = value ?? "";
	control.addEventListener("change", () => {
		if (field.type !== "boolean" && control.value === "" && !field.required) {
			onChange({ field: field.name, remove: true });
			return;
		}
		let next = control.value;
		if (field.type === "boolean") next = control.checked;
		if (field.type === "integer") next = Number.parseInt(control.value, 10);
		if (field.type === "number") next = Number.parseFloat(control.value);
		onChange({ field: field.name, value: next });
	});
	return control;
}

export function createProductDOMView(elements, statusMessage) {
	const {
		title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList,
		nodeCatalogList, nodeList, nodeEditor, nodeEditorStatus, nodeName, removeNodeButton, nodeConfigForm,
	} = elements;
	let selectWorkflow = () => {};
	let draftDirty = () => {};
	let addNode = () => {};
	let selectNode = () => {};
	let renameNode = () => {};
	let editNodeConfig = () => {};
	let removeNode = () => {};
	let editDraft = async () => {};
	let pendingDraftEdit;
	let activeWorkflowId = "";
	let activeNodeId = "";
	let editRevision = 0;
	let autosaveTimer;
	async function flushDraftEdit() {
		clearTimeout(autosaveTimer);
		if (!pendingDraftEdit) return;
		const request = pendingDraftEdit;
		pendingDraftEdit = undefined;
		return editDraft(request);
	}
	return {
		onOpenWorkspace(handler) { button.addEventListener("click", handler); },
		onCreateWorkflow(handler) {
			form.addEventListener("submit", async (event) => {
				event.preventDefault();
				const displayName = nameInput.value.trim();
				if (!displayName) return;
				await handler(displayName);
				form.reset();
			});
		},
		onSelectWorkflow(handler) { selectWorkflow = handler; },
		onDraftDirty(handler) { draftDirty = handler; },
		onEditDraft(handler) {
			editDraft = handler;
			draftEditor.addEventListener("input", () => {
				clearTimeout(autosaveTimer);
				const workflowId = activeWorkflowId;
				const revision = ++editRevision;
				pendingDraftEdit = { workflowId, revision, content: draftEditor.value };
				draftDirty({ workflowId, revision });
				autosaveTimer = setTimeout(async () => {
					try {
						if (pendingDraftEdit) pendingDraftEdit.content = JSON.parse(pendingDraftEdit.content);
						await flushDraftEdit();
					} catch (error) {
						draftStatus.textContent = `Draft JSON is not ready to save: ${error.message}`;
					}
				}, 250);
			});
		},
		async flushDraftEdit() {
			if (pendingDraftEdit && typeof pendingDraftEdit.content === "string") pendingDraftEdit.content = JSON.parse(pendingDraftEdit.content);
			return flushDraftEdit();
		},
		onAddNode(handler) { addNode = handler; },
		onSelectNode(handler) { selectNode = handler; },
		onRenameNode(handler) {
			renameNode = handler;
			nodeName?.addEventListener("change", () => renameNode({ nodeId: activeNodeId, displayName: nodeName.value }));
		},
		onEditNodeConfig(handler) { editNodeConfig = handler; },
		onRemoveNode(handler) {
			removeNode = handler;
			removeNodeButton?.addEventListener("click", () => removeNode(activeNodeId));
		},
		render(state) {
			title.textContent = state.title;
			message.textContent = state.message;
			status.textContent = statusMessage(state);
			status.dataset.state = state.status;
			button.disabled = state.status === "loading";
		},
		renderWorkflows(workflows) {
			const document = workflowList.ownerDocument;
			workflowList.replaceChildren(...workflows.map((workflow) => {
				const item = document.createElement("li");
				const name = document.createElement("button");
				const identity = document.createElement("code");
				name.type = "button";
				name.textContent = workflow.displayName;
				name.addEventListener("click", () => selectWorkflow(workflow.id));
				identity.textContent = workflow.id;
				item.append(name, identity);
				return item;
			}));
		},
		renderNodeCatalog(entries) {
			if (!nodeCatalogList) return;
			const document = nodeCatalogList.ownerDocument;
			nodeCatalogList.replaceChildren(...entries.map((entry) => {
				const item = document.createElement("li");
				const control = document.createElement("button");
				const detail = document.createElement("span");
				control.type = "button";
				control.textContent = `Add ${entry.definition.displayName}`;
				control.addEventListener("click", () => addNode(entry.definition.id));
				detail.textContent = entry.definition.description;
				item.append(control, detail);
				return item;
			}));
		},
		renderDraftLoading() {
			clearTimeout(autosaveTimer);
			activeWorkflowId = "";
			activeNodeId = "";
			pendingDraftEdit = undefined;
			editRevision = 0;
			draftEditor.disabled = true;
			draftStatus.textContent = "Loading Draft…";
			diagnosticList.replaceChildren();
			nodeList?.replaceChildren();
		},
		renderDraft(result) {
			const { draft, preview } = result;
			if (result.saved === undefined && result.conflict === undefined) {
				clearTimeout(autosaveTimer);
				activeWorkflowId = draft.workflowId;
				editRevision = 0;
			}
			draftEditor.disabled = Boolean(result.conflict);
			draftEditor.value = JSON.stringify(draft.content, null, 2);
			draftStatus.textContent = result.conflict ? "A newer Draft was loaded. Refresh this workflow before editing again." : result.saved ? "Autosaved." : "Draft loaded.";
			const diagnostics = preview?.diagnostics ?? [];
			const document = diagnosticList.ownerDocument;
			diagnosticList.replaceChildren(...diagnostics.map((diagnostic) => {
				const item = document.createElement("li");
				item.textContent = `${diagnostic.path}: ${diagnostic.message}`;
				return item;
			}));
			if (nodeList) {
				nodeList.replaceChildren(...(draft.content.nodes ?? []).map((node) => {
					const item = document.createElement("li");
					const control = document.createElement("button");
					control.type = "button";
					control.textContent = node.displayName || node.id;
					control.addEventListener("click", () => selectNode(node.id));
					item.append(control);
					return item;
				}));
			}
		},
		renderNodeEditor(state) {
			if (!nodeEditor) return;
			activeNodeId = state?.node.id ?? "";
			nodeEditor.hidden = !state;
			if (!state) {
				nodeEditorStatus.textContent = "Select a Node Instance to configure it.";
				nodeConfigForm.replaceChildren();
				return;
			}
			nodeEditorStatus.textContent = `${state.node.definition} · ${state.node.executor} · ${state.node.id}`;
			nodeName.value = state.node.displayName;
			const document = nodeConfigForm.ownerDocument;
			nodeConfigForm.replaceChildren(...state.fields.map((field) => {
				const group = document.createElement("label");
				const heading = document.createElement("span");
				heading.textContent = field.presentation.label || field.name;
				const help = document.createElement("small");
				help.textContent = field.presentation.help || "";
				const control = fieldControl(document, field, state.node.config?.[field.name], (change) => editNodeConfig({ nodeId: activeNodeId, ...change }));
				group.append(heading, control, help);
				return group;
			}));
		},
	};
}
