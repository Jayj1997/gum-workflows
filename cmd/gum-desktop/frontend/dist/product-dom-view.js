export function createProductDOMView(elements, statusMessage) {
  const { title, message, status, button, form, nameInput, workflowList } = elements;
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
        const name = document.createElement("strong");
        const identity = document.createElement("code");
        name.textContent = workflow.displayName;
        identity.textContent = workflow.id;
        item.append(name, identity);
        return item;
      });
      workflowList.replaceChildren(...items);
    },
  };
}
