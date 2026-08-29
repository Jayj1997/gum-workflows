PRAGMA foreign_keys = ON;

CREATE TABLE node_type_definition (
  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '',
  requires_json TEXT NOT NULL DEFAULT '[]', created_at TEXT NOT NULL
);
CREATE TABLE node_definition (
  id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL REFERENCES node_type_definition(name), requires_json TEXT NOT NULL DEFAULT '[]',
  inputs_json TEXT NOT NULL DEFAULT '{}', outputs_json TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL
);
CREATE TABLE node_executor (
  id TEXT PRIMARY KEY, node_definition_id TEXT NOT NULL REFERENCES node_definition(id) ON DELETE CASCADE,
  version TEXT NOT NULL, name TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
  updates TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, UNIQUE (node_definition_id, version)
);
CREATE TABLE workflow (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, version TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
  projects_json TEXT NOT NULL DEFAULT '[]', created_at TEXT NOT NULL, UNIQUE (name, version)
);
CREATE TABLE node_instance (
  id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL REFERENCES workflow(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL, node_definition_id TEXT NOT NULL REFERENCES node_definition(id),
  node_executor_id TEXT NOT NULL REFERENCES node_executor(id), display_name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '', llm_provider TEXT NOT NULL DEFAULT '', llm_model TEXT NOT NULL DEFAULT '',
  inputs_json TEXT NOT NULL DEFAULT '{}', depends_on_json TEXT NOT NULL DEFAULT '[]',
  config_json TEXT NOT NULL DEFAULT '{}', UNIQUE (workflow_id, node_id)
);
CREATE TABLE workflow_run_history (
  id TEXT PRIMARY KEY, workflow_name TEXT NOT NULL, workflow_version TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL, workflow_file TEXT NOT NULL DEFAULT '', execution_id TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '', stopped_reason TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL, finished_at TEXT
);
CREATE INDEX idx_run_history_started_at ON workflow_run_history (started_at DESC);
CREATE INDEX idx_run_history_workflow ON workflow_run_history (workflow_name);
CREATE TABLE workflow_node_run_history (
  id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES workflow_run_history(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL, node_definition TEXT NOT NULL, node_executor TEXT NOT NULL, round INTEGER NOT NULL,
  status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '', error_kind TEXT NOT NULL DEFAULT '',
  inputs_json TEXT NOT NULL DEFAULT '{}', outputs_json TEXT NOT NULL DEFAULT '{}', started_at TEXT, finished_at TEXT,
  UNIQUE (run_id, node_id, round)
);
CREATE INDEX idx_node_run_history_run ON workflow_node_run_history (run_id);

INSERT INTO node_type_definition VALUES ('legacy-type-id','automation','','[]','2026-08-28T08:00:00Z');
INSERT INTO node_definition VALUES (
  'legacy-definition-id','go-static-analysis','','automation','[]',
  '{"code":{"type":"SourceCode"}}','{"result":{"type":"QualityCheckResult"}}','2026-08-28T08:00:00Z'
);
INSERT INTO node_executor VALUES (
  'legacy-executor-id','legacy-definition-id','v1','Go Static Analysis','','','2026-08-28T08:00:00Z'
);
INSERT INTO workflow VALUES (
  'legacy-workflow-id','legacy-quality','v1','','[{"name":"project","repository":"."}]','2026-08-28T08:00:00Z'
);
INSERT INTO node_instance VALUES (
  'legacy-instance-id','legacy-workflow-id','check','legacy-definition-id','legacy-executor-id','','','','',
  '{"code":{"from":"project.code"}}','[]','{}'
);
INSERT INTO workflow_run_history VALUES (
  '11111111-1111-4111-8111-111111111111','legacy-quality','v1','Stopped','/legacy/project/workflow.yaml',
  'execution-000007','','user requested stop','2026-08-28T08:00:00Z','2026-08-28T08:01:00Z'
);
INSERT INTO workflow_node_run_history VALUES (
  '22222222-2222-4222-8222-222222222222','11111111-1111-4111-8111-111111111111','check',
  'go-static-analysis','v1',1,'Succeeded','','',
  '{"evidence":{"from":"legacy.result","ref":{"ID":"quality-result","Kind":"QualityCheckResult","Version":"1","URI":"1.json"}}}',
  '{"result":{"ID":"quality-result","Kind":"QualityCheckResult","Version":"1","URI":"1.json"}}',
  '2026-08-28T08:00:00Z','2026-08-28T08:00:30Z'
);

PRAGMA user_version = 2;
