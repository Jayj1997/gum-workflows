---
status: accepted
---

# Materialize the model slot before creating a Workflow Revision

An Agent Node stores a stable Gum Model UUID that identifies a user-managed model configuration slot, not an immutable underlying provider model. When the preference is empty, StartRun validates the UI's expected Draft lock version, resolves the current Provider/Model defaults, writes the UUID into the mutable Draft, and only then creates or reuses an immutable Revision; changing the slot's connection or Provider Model ID affects future Runs while each Run Snapshot preserves the values actually used, and deleting the slot blocks StartRun until the user chooses another model.
