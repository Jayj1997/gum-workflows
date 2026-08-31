export function createProductDOMView(elements, statusMessage) {
	const { title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList } = elements;
	let selectWorkflow = () => {};
	let draftDirty = () => {};
	let activeWorkflowId = "";
	let editRevision = 0;
	let autosaveTimer;
  return {
    onOpenWorkspace(handler) {
      button.addEventListener("click", handler);
    },
    onCreateWorkflow(handler) {
      form.addEventListener("submit", async (event) => {
        event.preventDefault();
        const displayName = nameInput.value.trim();
        if (!displayName) return;
        await handler(displayName);
        form.reset();
      });
    },
		onSelectWorkflow(handler) {
			selectWorkflow = handler;
		},
		onDraftDirty(handler) {
			draftDirty = handler;
		},
		onEditDraft(handler) {
			draftEditor.addEventListener("input", () => {
				clearTimeout(autosaveTimer);
				const workflowId = activeWorkflowId;
				const revision = ++editRevision;
				const content = draftEditor.value;
				draftDirty({ workflowId, revision });
				autosaveTimer = setTimeout(async () => {
					try {
						await handler({ workflowId, revision, content: JSON.parse(content) });
					} catch (error) {
						draftStatus.textContent = `Draft JSON is not ready to save: ${error.message}`;
					}
				}, 250);
			});
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
      const items = workflows.map((workflow) => {
        const item = document.createElement("li");
				const name = document.createElement("button");
        const identity = document.createElement("code");
				name.type = "button";
        name.textContent = workflow.displayName;
				name.addEventListener("click", () => selectWorkflow(workflow.id));
        identity.textContent = workflow.id;
        item.append(name, identity);
        return item;
      });
      workflowList.replaceChildren(...items);
    },
		renderDraftLoading() {
			clearTimeout(autosaveTimer);
			activeWorkflowId = "";
			editRevision = 0;
			draftEditor.disabled = true;
			draftStatus.textContent = "Loading Draft…";
			diagnosticList.replaceChildren();
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
			draftStatus.textContent = result.conflict
				? "A newer Draft was loaded. Refresh this workflow before editing again."
				: result.saved
					? "Autosaved."
					: "Draft loaded.";
			const diagnostics = preview?.diagnostics ?? [];
			const document = diagnosticList.ownerDocument;
			diagnosticList.replaceChildren(...diagnostics.map((diagnostic) => {
				const item = document.createElement("li");
				item.textContent = `${diagnostic.path}: ${diagnostic.message}`;
				return item;
			}));
		},
  };
}
