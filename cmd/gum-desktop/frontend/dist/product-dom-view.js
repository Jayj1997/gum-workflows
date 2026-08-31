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

function generationDefaults(temperature, maxOutputTokens) {
	const defaults = {};
	if (temperature !== "") defaults.temperature = Number.parseFloat(temperature);
	if (maxOutputTokens !== "") defaults.maxOutputTokens = Number.parseInt(maxOutputTokens, 10);
	return defaults;
}

function previewLayers(preview) {
	const componentByNode = new Map((preview?.nodes ?? []).map((node) => [node.id, node.id]));
	for (const [index, group] of (preview?.groups ?? []).entries()) {
		for (const nodeId of group.nodeIds) componentByNode.set(nodeId, `cycle:${index}`);
	}
	const components = [...new Set(componentByNode.values())].sort();
	const outgoing = new Map(components.map((component) => [component, new Set()]));
	const indegree = new Map(components.map((component) => [component, 0]));
	for (const edge of preview?.edges ?? []) {
		const source = componentByNode.get(edge.sourceNodeId);
		const target = componentByNode.get(edge.targetNodeId);
		if (!source || !target || source === target || outgoing.get(source).has(target)) continue;
		outgoing.get(source).add(target);
		indegree.set(target, indegree.get(target) + 1);
	}
	const layers = new Map(components.map((component) => [component, 0]));
	const ready = components.filter((component) => indegree.get(component) === 0);
	while (ready.length) {
		ready.sort();
		const source = ready.shift();
		for (const target of [...outgoing.get(source)].sort()) {
			layers.set(target, Math.max(layers.get(target), layers.get(source) + 1));
			indegree.set(target, indegree.get(target) - 1);
			if (indegree.get(target) === 0) ready.push(target);
		}
	}
	return new Map([...componentByNode].map(([nodeId, component]) => [nodeId, layers.get(component) ?? 0]));
}

export function createProductDOMView(elements, statusMessage) {
	const {
		title, message, status, button, form, nameInput, workflowList, draftEditor, draftStatus, diagnosticList,
		nodeCatalogList, nodeList, nodeEditor, nodeEditorStatus, nodeName, removeNodeButton, nodeConfigForm,
		nodeInputForm, nodeControlForm,
		previewCanvas, previewEdges, previewGroups, previewZoomIn, previewZoomOut, previewZoomReset,
			providerForm, providerName, providerProtocol, providerBaseURL, providerAPIKeyRef, llmProviderList, llmDiagnosticList,
			runButton, runStatus, nodeRunList, artifactList,
	} = elements;
	let selectWorkflow = () => {};
	let draftDirty = () => {};
	let addNode = () => {};
	let selectNode = () => {};
	let renameNode = () => {};
	let editNodeConfig = () => {};
	let bindNodeInput = () => {};
	let editControlDependencies = () => {};
	let removeNode = () => {};
	let editDraft = async () => {};
	let createLLMProvider = () => {};
	let updateLLMProvider = () => {};
	let deleteLLMProvider = () => {};
	let setDefaultLLMProvider = () => {};
	let createLLMModel = () => {};
	let updateLLMModel = () => {};
	let deleteLLMModel = () => {};
		let setDefaultLLMModel = () => {};
		let startRun = () => {};
	let pendingDraftEdit;
	let activeWorkflowId = "";
	let activeNodeId = "";
	let editRevision = 0;
	let autosaveTimer;
	let previewZoom = 1;
	let selectedPreviewEdge = "";
	const collapsedPreviewNodes = new Set();
	function applyPreviewZoom() {
		if (!previewCanvas) return;
		previewCanvas.style.transform = `scale(${Number(previewZoom.toFixed(1))})`;
		previewCanvas.style.transformOrigin = "top left";
	}
	previewZoomIn?.addEventListener("click", () => {
		previewZoom = Math.min(1.6, previewZoom + 0.1);
		applyPreviewZoom();
	});
	previewZoomOut?.addEventListener("click", () => {
		previewZoom = Math.max(0.6, previewZoom - 0.1);
		applyPreviewZoom();
	});
	previewZoomReset?.addEventListener("click", () => {
		previewZoom = 1;
		applyPreviewZoom();
	});
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
		onBindNodeInput(handler) { bindNodeInput = handler; },
		onEditControlDependencies(handler) { editControlDependencies = handler; },
		onRemoveNode(handler) {
			removeNode = handler;
			removeNodeButton?.addEventListener("click", () => removeNode(activeNodeId));
		},
		onCreateLLMProvider(handler) {
			createLLMProvider = handler;
			providerForm?.addEventListener("submit", async (event) => {
				event.preventDefault();
				await createLLMProvider({ name: providerName.value.trim(), protocol: providerProtocol.value, baseUrl: providerBaseURL.value.trim(), apiKeyRef: providerAPIKeyRef.value.trim() });
				providerForm.reset();
			});
		},
		onUpdateLLMProvider(handler) { updateLLMProvider = handler; },
		onDeleteLLMProvider(handler) { deleteLLMProvider = handler; },
		onSetDefaultLLMProvider(handler) { setDefaultLLMProvider = handler; },
		onCreateLLMModel(handler) { createLLMModel = handler; },
		onUpdateLLMModel(handler) { updateLLMModel = handler; },
		onDeleteLLMModel(handler) { deleteLLMModel = handler; },
			onSetDefaultLLMModel(handler) { setDefaultLLMModel = handler; },
			onStartRun(handler) {
				startRun = handler;
				runButton?.addEventListener("click", () => startRun());
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
		renderLLMSettings(settings) {
			if (!llmProviderList) return;
			const document = llmProviderList.ownerDocument;
			function textInput(value, label) {
				const input = document.createElement("input");
				input.value = value;
				input.setAttribute?.("aria-label", label);
				return input;
			}
			llmProviderList.replaceChildren(...settings.providers.map((provider) => {
				const card = document.createElement("article");
				card.className = "llm-provider-card";
				const heading = document.createElement("div");
				const name = textInput(provider.name, "Provider name");
				const protocol = document.createElement("select");
				const protocolOption = document.createElement("option");
				protocolOption.value = "openai-chat-completions";
				protocolOption.textContent = "OpenAI-compatible Chat Completions";
				protocol.append(protocolOption);
				protocol.value = provider.protocol;
				const baseURL = textInput(provider.baseUrl, "Base URL");
				const apiKeyRef = textInput(provider.apiKeyRef, "API Key reference");
				const save = document.createElement("button");
				save.type = "button"; save.textContent = "Save Provider";
				save.addEventListener("click", () => updateLLMProvider({ id: provider.id, name: name.value, protocol: protocol.value, baseUrl: baseURL.value, apiKeyRef: apiKeyRef.value }));
				const makeDefault = document.createElement("button");
				makeDefault.type = "button"; makeDefault.textContent = provider.effectiveDefault ? "Effective default" : "Make default";
				makeDefault.disabled = provider.explicitDefault;
				makeDefault.addEventListener("click", () => setDefaultLLMProvider(provider.id));
				const remove = document.createElement("button");
				remove.type = "button"; remove.className = "danger"; remove.textContent = "Delete Provider";
				remove.addEventListener("click", () => deleteLLMProvider(provider.id));
				heading.append(name, protocol, baseURL, apiKeyRef, save, makeDefault, remove);

				const models = document.createElement("div");
				models.className = "llm-model-list";
				for (const model of provider.models) {
					const row = document.createElement("div");
					const displayName = textInput(model.displayName, "Model display name");
					const providerModelID = textInput(model.providerModelId, "Provider Model ID");
					const temperature = textInput(model.generationDefaults?.temperature ?? "", "Default temperature");
					temperature.type = "number"; temperature.min = "0"; temperature.max = "2"; temperature.step = "any";
					const maxOutputTokens = textInput(model.generationDefaults?.maxOutputTokens ?? "", "Default max output tokens");
					maxOutputTokens.type = "number"; maxOutputTokens.min = "1"; maxOutputTokens.step = "1";
					const saveModel = document.createElement("button");
					saveModel.type = "button"; saveModel.textContent = "Save Model";
					saveModel.addEventListener("click", () => updateLLMModel({ id: model.id, providerId: provider.id, displayName: displayName.value, providerModelId: providerModelID.value, generationDefaults: generationDefaults(temperature.value, maxOutputTokens.value) }));
					const defaultModel = document.createElement("button");
					defaultModel.type = "button"; defaultModel.textContent = model.effectiveDefault ? "Effective default" : "Make default";
					defaultModel.disabled = model.explicitDefault;
					defaultModel.addEventListener("click", () => setDefaultLLMModel(provider.id, model.id));
					const removeModel = document.createElement("button");
					removeModel.type = "button"; removeModel.className = "danger"; removeModel.textContent = "Delete Model";
					removeModel.addEventListener("click", () => deleteLLMModel(provider.id, model.id));
					row.append(displayName, providerModelID, temperature, maxOutputTokens, saveModel, defaultModel, removeModel);
					models.append(row);
				}
				const addModel = document.createElement("form");
				const newName = textInput("", "New Model display name");
				const newProviderModelID = textInput("", "New Provider Model ID");
				const newTemperature = textInput("", "New default temperature");
				newTemperature.type = "number"; newTemperature.min = "0"; newTemperature.max = "2"; newTemperature.step = "any";
				const newMaxOutputTokens = textInput("", "New default max output tokens");
				newMaxOutputTokens.type = "number"; newMaxOutputTokens.min = "1"; newMaxOutputTokens.step = "1";
				const add = document.createElement("button");
				add.type = "submit"; add.textContent = "Add Model";
				addModel.addEventListener("submit", async (event) => {
					event.preventDefault();
					await createLLMModel({ providerId: provider.id, displayName: newName.value, providerModelId: newProviderModelID.value, generationDefaults: generationDefaults(newTemperature.value, newMaxOutputTokens.value) });
				});
				addModel.append(newName, newProviderModelID, newTemperature, newMaxOutputTokens, add);
				card.append(heading, models, addModel);
				return card;
			}));
			if (llmDiagnosticList) {
				llmDiagnosticList.replaceChildren(...settings.diagnostics.map((diagnostic) => {
					const item = document.createElement("li");
					item.textContent = diagnostic.message;
					return item;
				}));
			}
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
			renderRun(run) {
				if (runStatus) runStatus.textContent = `Run ${run.status} · revision ${run.revisionId}`;
				if (nodeRunList) {
					const document = nodeRunList.ownerDocument;
					nodeRunList.replaceChildren(...run.nodeRuns.map((nodeRun) => {
						const item = document.createElement("li");
						item.textContent = `${nodeRun.nodeId} · ${nodeRun.nodeDefinition}@${nodeRun.nodeExecutor} · ${nodeRun.status}`;
						return item;
					}));
				}
				if (artifactList) {
					const document = artifactList.ownerDocument;
					artifactList.replaceChildren(...run.artifacts.map((artifact) => {
						const item = document.createElement("li");
						const heading = document.createElement("strong");
						const messages = document.createElement("ol");
						heading.textContent = `${artifact.nodeId}.${artifact.port} · ${artifact.type} v${artifact.version}`;
						messages.append(...(artifact.messages ?? []).map((message) => {
							const row = document.createElement("li");
							row.textContent = `${message.role}: ${message.text}`;
							return row;
						}));
						item.append(heading, messages);
						return item;
					}));
				}
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
				const nodeIndex = /^nodes\[(\d+)\]/.exec(diagnostic.path)?.[1];
				const node = nodeIndex === undefined ? undefined : draft.content.nodes?.[Number(nodeIndex)];
				if (node?.id) {
					const control = document.createElement("button");
					control.type = "button";
					control.className = "diagnostic-link";
					control.textContent = `${diagnostic.path}: ${diagnostic.message}`;
					control.addEventListener("click", () => selectNode(node.id, diagnostic.path));
					item.append(control);
				} else {
					item.textContent = `${diagnostic.path}: ${diagnostic.message}`;
				}
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
			if (previewCanvas) {
				const layers = previewLayers(preview);
				const orderedNodes = [...(preview?.nodes ?? [])].sort((left, right) => {
					const layerDifference = (layers.get(left.id) ?? 0) - (layers.get(right.id) ?? 0);
					return layerDifference || left.id.localeCompare(right.id);
				});
				previewCanvas.replaceChildren(...orderedNodes.map((node) => {
					const card = document.createElement("details");
					const heading = document.createElement("summary");
					const detail = document.createElement("small");
					card.dataset.kind = node.kind || "unknown";
					card.style.gridColumn = String((layers.get(node.id) ?? 0) + 1);
					card.open = !collapsedPreviewNodes.has(node.id);
					heading.textContent = node.displayName || node.id;
					heading.addEventListener("click", () => selectNode(node.id));
					detail.textContent = `${node.definitionId} · ${node.id}`;
					card.append(heading, detail);
					card.addEventListener("toggle", () => {
						if (card.open) collapsedPreviewNodes.delete(node.id);
						else collapsedPreviewNodes.add(node.id);
					});
					return card;
				}));
				applyPreviewZoom();
			}
			if (previewEdges) {
				const edgeItems = (preview?.edges ?? []).map((edge) => {
					const item = document.createElement("li");
					const control = document.createElement("button");
					const edgeIdentity = [edge.kind, edge.sourceNodeId, edge.sourcePort, edge.targetNodeId, edge.targetPort].join(":");
					item.dataset.kind = edge.kind;
					item.dataset.selected = String(edgeIdentity === selectedPreviewEdge);
					item.className = `preview-edge preview-edge-${edge.kind}`;
					control.type = "button";
					control.textContent = edge.kind === "data"
						? `Data · ${edge.sourceNodeId}.${edge.sourcePort} → ${edge.targetNodeId}.${edge.targetPort} · ${edge.artifactType || "unknown type"}`
						: `Control · ${edge.sourceNodeId} → ${edge.targetNodeId}`;
					control.addEventListener("click", () => {
						selectedPreviewEdge = edgeIdentity;
						for (const candidate of edgeItems) candidate.dataset.selected = String(candidate === item);
					});
					item.append(control);
					return item;
				});
				previewEdges.replaceChildren(...edgeItems);
			}
			if (previewGroups) {
				previewGroups.replaceChildren(...(preview?.groups ?? []).map((group) => {
					const item = document.createElement("li");
					item.textContent = `Cycle · ${group.nodeIds.join(" ↔ ")}`;
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
				nodeInputForm?.replaceChildren();
				nodeControlForm?.replaceChildren();
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
				if (state.focus?.section === "config" && state.focus.field === field.name) control.focus?.();
				group.append(heading, control, help);
				return group;
			}));
			if (nodeInputForm) {
				nodeInputForm.replaceChildren(...Object.entries(state.inputs ?? {}).map(([name, contract]) => {
					const group = document.createElement("label");
					const heading = document.createElement("span");
					heading.textContent = `${name} · ${contract.type}${contract.optional ? " · optional" : ""}`;
					const select = document.createElement("select");
					const empty = document.createElement("option");
					empty.value = "";
					empty.textContent = "Not bound";
					select.append(empty);
					for (const source of state.inputSources ?? []) {
						const option = document.createElement("option");
						option.value = source.reference;
						option.textContent = `${source.displayName} · ${source.type}`;
						select.append(option);
					}
					select.value = state.node.inputs?.[name]?.from ?? "";
					select.addEventListener("change", () => bindNodeInput({ nodeId: activeNodeId, input: name, from: select.value }));
					if (state.focus?.section === "inputs" && state.focus.field === name) select.focus?.();
					group.append(heading, select);
					return group;
				}));
			}
			if (nodeControlForm) {
				const checkboxes = (state.controlNodes ?? []).map((candidate) => {
					const group = document.createElement("label");
					const checkbox = document.createElement("input");
					const text = document.createElement("span");
					checkbox.type = "checkbox";
					checkbox.value = candidate.id;
					checkbox.checked = (state.node.dependsOn ?? []).includes(candidate.id);
					text.textContent = candidate.displayName;
					group.append(checkbox, text);
					return { group, checkbox };
				});
				for (const { checkbox } of checkboxes) {
					checkbox.addEventListener("change", () => editControlDependencies({
						nodeId: activeNodeId,
						nodeIds: checkboxes.filter((item) => item.checkbox.checked).map((item) => item.checkbox.value),
					}));
				}
				nodeControlForm.replaceChildren(...checkboxes.map((item) => item.group));
			}
		},
	};
}
