export function createProductDOMView(elements, statusMessage) {
  const { title, message, status, button } = elements;
  return {
    onOpenWorkspace(handler) {
      button.addEventListener("click", handler);
    },
    render(state) {
      title.textContent = state.title;
      message.textContent = state.message;
      status.textContent = statusMessage(state);
      status.dataset.state = state.status;
      button.disabled = state.status === "loading";
    },
  };
}
