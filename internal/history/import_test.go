package history

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func seedRows() (nodeTypes []NodeTypeDefRow, defs []NodeDefRow, execs []NodeExecRow,
	wf WorkflowRow, instances []NodeInstanceRow) {
	nodeTypes = []NodeTypeDefRow{
		{Name: "automation", Description: "automation node type", Requires: []string{"project"}},
	}
	defs = []NodeDefRow{
		{
			Name: "coding-agent", Description: "coder", Type: "automation",
			Requires: []string{"project"},
			Inputs: map[string]InputPort{
				"openapi": {Type: "OpenAPI", Optional: true, Description: "api doc"},
			},
			Outputs: map[string]OutputPort{
				"source-code": {Type: "SourceCode", Description: "src"},
			},
		},
		{
			Name: "openapi-generator", Description: "sdk gen", Type: "automation",
			Inputs:  map[string]InputPort{"openapi": {Type: "OpenAPI"}},
			Outputs: map[string]OutputPort{"sdk": {Type: "FrontendSDK"}},
		},
	}
	// executors 由测试在导入 definitions 后补 node_definition_id。
	wf = WorkflowRow{
		Name: "minimal-development", Version: "1.0",
		Description: "demo", Projects: []ProjectRow{{Name: "order-system", Repository: "./project"}},
	}
	instances = []NodeInstanceRow{
		{
			NodeID: "coder", DisplayName: "编码", Config: map[string]any{"task": "实现"},
			DependsOn: []string{}, Inputs: map[string]InputBinding{},
		},
		{
			NodeID: "sdk", DisplayName: "SDK", DependsOn: []string{},
			Inputs: map[string]InputBinding{"openapi": {From: "coder.openapi"}},
		},
	}
	return
}

func TestImportDefinitionsRoundTrip(t *testing.T) {
	s, _ := openTest(t)
	nt, defs, _, _, _ := seedRows()
	if err := s.ImportDefinitions(ctxWithNow(), nt, defs, nil); err != nil {
		t.Fatalf("ImportDefinitions: %v", err)
	}

	// node_type_definition
	var desc, reqJSON string
	if err := s.db.QueryRow(
		`SELECT description, requires_json FROM node_type_definition WHERE name='automation'`).Scan(&desc, &reqJSON); err != nil {
		t.Fatalf("query node type: %v", err)
	}
	if desc != "automation node type" {
		t.Errorf("node type description = %q", desc)
	}
	var reqs []string
	if err := json.Unmarshal([]byte(reqJSON), &reqs); err != nil || len(reqs) != 1 || reqs[0] != "project" {
		t.Errorf("requires_json round-trip = %q err=%v", reqJSON, err)
	}

	// node_definition：inputs_json / outputs_json 可往返。
	var inJSON, outJSON, dtype string
	if err := s.db.QueryRow(
		`SELECT type, inputs_json, outputs_json FROM node_definition WHERE name='coding-agent'`).Scan(&dtype, &inJSON, &outJSON); err != nil {
		t.Fatalf("query node def: %v", err)
	}
	if dtype != "automation" {
		t.Errorf("type = %q", dtype)
	}
	var ins map[string]InputPort
	if err := json.Unmarshal([]byte(inJSON), &ins); err != nil {
		t.Fatalf("unmarshal inputs_json: %v", err)
	}
	if p, ok := ins["openapi"]; !ok || p.Type != "OpenAPI" || !p.Optional {
		t.Errorf("inputs_json round-trip lost optional/type: %+v", ins)
	}
	if strings.Contains(inJSON, `"Type"`) || !strings.Contains(inJSON, `"type"`) {
		t.Errorf("inputs_json field names are not canonical: %s", inJSON)
	}

	// 五表存在（node_executor 留空，本用例不导入）。
	var execCount int
	if err := s.db.QueryRow(`SELECT count(*) FROM node_executor`).Scan(&execCount); err != nil {
		t.Fatalf("count executors: %v", err)
	}
	if execCount != 0 {
		t.Errorf("exec count = %d, want 0", execCount)
	}
}

func TestImportNormalizesEmptyJSONColumns(t *testing.T) {
	s, _ := openTest(t)
	if err := s.ImportDefinitions(ctxWithNow(),
		[]NodeTypeDefRow{{Name: "automation"}},
		[]NodeDefRow{{Name: "empty", Type: "automation"}},
		nil,
	); err != nil {
		t.Fatalf("import empty definitions: %v", err)
	}

	var requires, inputs, outputs string
	if err := s.db.QueryRow(`
SELECT requires_json, inputs_json, outputs_json FROM node_definition WHERE name = 'empty'`,
	).Scan(&requires, &inputs, &outputs); err != nil {
		t.Fatalf("query empty JSON columns: %v", err)
	}
	if requires != "[]" || inputs != "{}" || outputs != "{}" {
		t.Errorf("empty JSON columns = %q, %q, %q", requires, inputs, outputs)
	}
}

func TestImportWorkflowRoundTrip(t *testing.T) {
	s, _ := openTest(t)
	nt, defs, _, _, _ := seedRows()
	if err := s.ImportDefinitions(ctxWithNow(), nt, defs, nil); err != nil {
		t.Fatalf("import defs: %v", err)
	}
	// 补 executor 引用刚导入的 definition id。
	coderDefID, _ := s.selectID(context.Background(), "node_definition", "name", "coding-agent")
	sdkDefID, _ := s.selectID(context.Background(), "node_definition", "name", "openapi-generator")
	execs := []NodeExecRow{
		{Node: "coding-agent", Version: "v1", Name: "coding-agent-v1", Updates: "首个版本"},
		{Node: "openapi-generator", Version: "v1", Name: "openapi-generator-v1", Updates: "首个版本"},
	}
	if err := s.ImportDefinitions(ctxWithNow(), nil, nil, execs); err != nil {
		t.Fatalf("import execs: %v", err)
	}
	coderExecID := execIDFor(t, s, coderDefID, "v1")
	sdkExecID := execIDFor(t, s, sdkDefID, "v1")

	_, _, _, wf, instances := seedRows()
	instances[0].NodeDefinitionID = coderDefID
	instances[0].NodeExecutorID = coderExecID
	instances[1].NodeDefinitionID = sdkDefID
	instances[1].NodeExecutorID = sdkExecID

	if err := s.ImportWorkflow(ctxWithNow(), wf, instances); err != nil {
		t.Fatalf("ImportWorkflow: %v", err)
	}

	// workflow 行存在且 projects_json 可往返。
	var projJSON string
	if err := s.db.QueryRow(`SELECT projects_json FROM workflow WHERE name='minimal-development' AND version='1.0'`).Scan(&projJSON); err != nil {
		t.Fatalf("query workflow: %v", err)
	}
	var projs []ProjectRow
	if err := json.Unmarshal([]byte(projJSON), &projs); err != nil || len(projs) != 1 || projs[0].Name != "order-system" {
		t.Errorf("projects_json round-trip = %q err=%v", projJSON, err)
	}

	// node_instance：sdk 引用 coder.openapi。
	var cfgJSON, inJSON string
	if err := s.db.QueryRow(
		`SELECT config_json, inputs_json FROM node_instance WHERE node_id='sdk'`).Scan(&cfgJSON, &inJSON); err != nil {
		t.Fatalf("query node instance: %v", err)
	}
	var ins map[string]InputBinding
	if err := json.Unmarshal([]byte(inJSON), &ins); err != nil {
		t.Fatalf("unmarshal inputs_json: %v", err)
	}
	if b, ok := ins["openapi"]; !ok || b.From != "coder.openapi" {
		t.Errorf("instance inputs_json round-trip: %+v", ins)
	}
}

func TestImportIsIdempotent(t *testing.T) {
	s, _ := openTest(t)
	nt, defs, _, _, _ := seedRows()
	if err := s.ImportDefinitions(ctxWithNow(), nt, defs, nil); err != nil {
		t.Fatalf("first import: %v", err)
	}
	defIDBefore, _ := s.selectID(context.Background(), "node_definition", "name", "coding-agent")

	// 第二次导入同份种子：不应报错、不应新增行、id 不变。
	if err := s.ImportDefinitions(ctxWithNow(), nt, defs, nil); err != nil {
		t.Fatalf("second import: %v", err)
	}
	defIDAfter, _ := s.selectID(context.Background(), "node_definition", "name", "coding-agent")
	if defIDBefore != defIDAfter {
		t.Errorf("node_definition id drifted on re-import: %s -> %s", defIDBefore, defIDAfter)
	}

	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM node_definition WHERE name='coding-agent'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("duplicate rows after re-import: %d", n)
	}

	// workflow 重复导入：id 稳定 + 覆盖 projects_json。
	nt2, defs2, execs2, wf, instances := seedRows()
	_ = nt2
	_ = defs2
	_ = execs2
	wf.Description = "updated"
	if err := s.ImportWorkflow(ctxWithNow(), wf, nil); err != nil {
		t.Fatalf("import workflow: %v", err)
	}
	wfID1, _ := s.selectID(context.Background(), "workflow", "name", wf.Name)
	if err := s.ImportWorkflow(ctxWithNow(), wf, nil); err != nil {
		t.Fatalf("re-import workflow: %v", err)
	}
	wfID2, _ := s.selectID(context.Background(), "workflow", "name", wf.Name)
	if wfID1 != wfID2 {
		t.Errorf("workflow id drifted on re-import: %s -> %s", wfID1, wfID2)
	}
	var desc string
	if err := s.db.QueryRow(`SELECT description FROM workflow WHERE id=?`, wfID2).Scan(&desc); err != nil {
		t.Fatalf("query workflow desc: %v", err)
	}
	if desc != "updated" {
		t.Errorf("workflow not updated on re-import: %q", desc)
	}
	_ = instances
}

func TestImportWorkflowReplacesRemovedInstances(t *testing.T) {
	s, _ := openTest(t)
	nodeTypes, definitions, _, wf, instances := seedRows()
	if err := s.ImportDefinitions(ctxWithNow(), nodeTypes, definitions, nil); err != nil {
		t.Fatalf("import definitions: %v", err)
	}
	if err := s.ImportDefinitions(ctxWithNow(), nil, nil, []NodeExecRow{
		{Node: "coding-agent", Version: "v1", Name: "coding-agent-v1"},
		{Node: "openapi-generator", Version: "v1", Name: "openapi-generator-v1"},
	}); err != nil {
		t.Fatalf("import executors: %v", err)
	}

	for i := range instances {
		definitionID, err := s.DefinitionID(context.Background(), definitions[i].Name)
		if err != nil {
			t.Fatalf("definition id: %v", err)
		}
		executorID, err := s.ExecutorID(context.Background(), definitions[i].Name, "v1")
		if err != nil {
			t.Fatalf("executor id: %v", err)
		}
		instances[i].NodeDefinitionID = definitionID
		instances[i].NodeExecutorID = executorID
	}
	if err := s.ImportWorkflow(ctxWithNow(), wf, instances); err != nil {
		t.Fatalf("first import workflow: %v", err)
	}

	var coderID string
	if err := s.db.QueryRow(`SELECT id FROM node_instance WHERE node_id = 'coder'`).Scan(&coderID); err != nil {
		t.Fatalf("query coder id: %v", err)
	}
	if err := s.ImportWorkflow(ctxWithNow(), wf, instances[:1]); err != nil {
		t.Fatalf("replace workflow: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM node_instance`).Scan(&count); err != nil {
		t.Fatalf("count instances: %v", err)
	}
	if count != 1 {
		t.Fatalf("node instances after replacement = %d, want 1", count)
	}
	var coderIDAfter string
	if err := s.db.QueryRow(`SELECT id FROM node_instance WHERE node_id = 'coder'`).Scan(&coderIDAfter); err != nil {
		t.Fatalf("query coder id after replacement: %v", err)
	}
	if coderIDAfter != coderID {
		t.Errorf("unchanged node instance id drifted: %s -> %s", coderID, coderIDAfter)
	}
}

func execIDFor(t *testing.T, s *Store, defID, version string) string {
	t.Helper()
	var id string
	if err := s.db.QueryRow(
		`SELECT id FROM node_executor WHERE node_definition_id=? AND version=?`, defID, version).Scan(&id); err != nil {
		t.Fatalf("lookup executor (%s, %s): %v", defID, version, err)
	}
	return id
}
