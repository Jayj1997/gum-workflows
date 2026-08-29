---
status: accepted
---

# Use the Workflow Run UUID as the sole Run identity

The platform-core history designs retained a project-local `execution-*` directory ID beside the Workflow Run UUID for compatibility with the old `.workflow` layout. The later Code Quality Check Automation design moved all projects into one Local Data Root and requires stable global identities, so a Workflow Run now uses its existing UUID as both the SQLite primary key and the `runs/<run-id>/` directory name. This supersedes only the dual-ID decisions in the read-only platform-core and Run History plans; their remaining historical scope stays unchanged.

Legacy migration may read an old `execution_id` solely to locate source Artifacts, but it publishes them under the unchanged Run UUID. SQLite schema v3 removes `execution_id`, and new Run creation must allocate the UUID before any Run-owned file is written.
