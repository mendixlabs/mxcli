// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// ako/mxcli#2075: the catalog listed agent-editor documents by name only, so
// "which agents can run a refund tool without the user's approval" took one
// `describe agent` per agent and text parsing. The Contents of real Agent
// Editor documents (the 11.16 fixtures the writer's carry tests use) are the
// input here.

const agentDetailsModuleID = model.ID("mod-agents")

func agentEditorFixtureContents(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../backend/modelsdk/testdata/agenteditor/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func agentEditorRawUnit(t *testing.T, id, name, docType, contents string) *types.RawUnit {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "CustomBlobDocuments$CustomBlobDocument"},
		{Key: "Name", Value: name},
		{Key: "Documentation", Value: ""},
		{Key: "CustomDocumentType", Value: docType},
		{Key: "Contents", Value: contents},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &types.RawUnit{ID: model.ID(id), ContainerID: agentDetailsModuleID,
		Type: "CustomBlobDocuments$CustomBlobDocument", Contents: b}
}

func agentDetailsFixture(t *testing.T) *Catalog {
	t.Helper()
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	tx, err := cat.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	units := []*types.RawUnit{
		// Agent Editor 2.2+: streaming switches, selected model, a microflow tool.
		agentEditorRawUnit(t, "ag-1", "GraphExplorer", types.CustomTypeAgent,
			agentEditorFixtureContents(t, "ICXGraphPilot.GraphExplorer.11_16.json")),
		// No streaming keys stored, one knowledge base tool.
		agentEditorRawUnit(t, "ag-2", "WorkInstructionAgent", types.CustomTypeAgent,
			agentEditorFixtureContents(t, "FactoryManagement.WorkInstructionAgent.11_16.json")),
		// An MCP tool and an approval-gated microflow tool.
		agentEditorRawUnit(t, "ag-3", "RefundAgent", types.CustomTypeAgent, `{"usageType":"Task",
			"tools":[
				{"id":"t1","name":"IssueRefund","toolType":"Microflow","enabled":true,
				 "userAccessApproval":"UserConfirmationRequired",
				 "document":{"qualifiedName":"Agents.Tool_IssueRefund"}},
				{"id":"t2","name":"Jira","toolType":"MCP","enabled":false,
				 "document":{"qualifiedName":"Agents.JiraMCP"}}],
			"toolChoice":"Auto","maxTokens":2048,"temperature":0.2}`),
		agentEditorRawUnit(t, "kb-1", "KB_WorkInstructions", types.CustomTypeKnowledgeBase,
			agentEditorFixtureContents(t, "FactoryManagement.KB_WorkInstructions.11_16.json")),
		agentEditorRawUnit(t, "m-1", "Gpt", types.CustomTypeModel, `{"type":"Text","name":"gpt","displayName":"GPT",
			"provider":"MxCloudGenAI","providerFields":{"key":{"qualifiedName":"Agents.GptKey"}}}`),
		agentEditorRawUnit(t, "mcp-1", "JiraMCP", types.CustomTypeConsumedMCPService,
			`{"endpoint":{"qualifiedName":"Agents.JiraUrl"},"protocolVersion":"v2025_03_26","version":"1.0",
			  "connectionTimeoutSeconds":30,"authenticationMicroflow":{"qualifiedName":"Agents.Jira_GetHeaders"}}`),
		// Control: a service with no endpoint, the #2060 failure, reads empty.
		agentEditorRawUnit(t, "mcp-2", "NoEndpointMCP", types.CustomTypeConsumedMCPService,
			`{"protocolVersion":"v2025_03_26","version":"1.0","connectionTimeoutSeconds":30}`),
		agentEditorRawUnit(t, "pub-1", "Published_MCP_service", types.CustomTypePublishedMCPService,
			agentEditorFixtureContents(t, "AgentBuilderStarterApp.Published_MCP_service.11_16.json")),
		// Contents that do not decode: the row stays, without details.
		agentEditorRawUnit(t, "ag-bad", "Broken", types.CustomTypeAgent, `{not json`),
	}
	b := &Builder{
		catalog: cat,
		reader: &mock.MockBackend{
			ListRawUnitsByTypeFunc: func(prefix string) ([]*types.RawUnit, error) {
				return units, nil
			},
		},
		snapshot: &Snapshot{ID: "snap-1"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{agentDetailsModuleID: true},
			moduleNames:     map[model.ID]string{agentDetailsModuleID: "Agents"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx:       wrapTx(tx),
		fullMode: true,
	}
	if err := b.buildAgentEditorDocs(); err != nil {
		t.Fatalf("buildAgentEditorDocs: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return cat
}

// agentQueryRows renders every row of q as "a|b|c", NULL as "∅".
func agentQueryRows(t *testing.T, cat *Catalog, q string) []string {
	t.Helper()
	rows, err := cat.db.Query(q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, v := range vals {
			if !v.Valid {
				parts = append(parts, "∅")
			} else {
				parts = append(parts, v.String)
			}
		}
		out = append(out, strings.Join(parts, "|"))
	}
	return out
}

func assertAgentRows(t *testing.T, cat *Catalog, q string, want ...string) {
	t.Helper()
	got := agentQueryRows(t, cat, q)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s\ngot:\n  %s\nwant:\n  %s", q, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestAgentDetails_Issue2075(t *testing.T) {
	cat := agentDetailsFixture(t)

	assertAgentRows(t, cat, `SELECT Name, UsageType, Model, SelectedModel, ToolChoice, MaxTokens, Temperature,
			EnableBackendStreaming, EnableUIStreaming, Entity, Variables, ToolCount, KnowledgeBaseCount, MCPServiceCount
		FROM agents ORDER BY Name`,
		// The row survives undecodable Contents, with no details.
		"Broken|∅|∅|∅|∅|∅|∅|∅|∅|∅|∅|0|0|0",
		"GraphExplorer|Chat|GraphPilot_Explorer.LLM_GenAIResource|eu.anthropic.claude-sonnet-5||∅|∅|1|0|GraphPilot_Explorer.OnboardingHelper|EnvironmentUrl,GraphmartUri,GeneralInfo,OntologyOverview,Questions|1|0|0",
		"RefundAgent|Task||∅|Auto|2048|0.2|∅|∅|||1|0|1",
		// Control: an agent that stores no streaming switch reads NULL, not "off".
		"WorkInstructionAgent|Task|AgenticAssistants.ClaudeSonnet46|∅||∅|∅|∅|∅|AgenticAssistants.GenericAgentInterface|Question|0|1|0",
	)

	// The issue's query: which tools can run without the user's approval.
	assertAgentRows(t, cat, `SELECT Agent, Name, ToolType, Document, Enabled, UserAccessApproval, IFNULL(DisplayTitle, '∅')
		FROM agent_tools ORDER BY Agent, Name`,
		"Agents.GraphExplorer|Tool_ExceuteSparql|Microflow|GraphPilot_Explorer.Tool_ExecuteSparql|1|VisibleForUser|Tool: Execute Sparql",
		"Agents.RefundAgent|IssueRefund|Microflow|Agents.Tool_IssueRefund|1|UserConfirmationRequired|∅",
	)
	// AGENT_TOOLS holds what ToolCount counts; the MCP services are in
	// AGENT_MCP_SERVICES only (ako/mxcli#2090).
	assertAgentRows(t, cat, `SELECT a.Name, a.ToolCount, (SELECT count(*) FROM agent_tools t WHERE t.Agent = a.QualifiedName),
			a.MCPServiceCount, (SELECT count(*) FROM agent_mcp_services m WHERE m.Agent = a.QualifiedName)
		FROM agents a WHERE a.Name = 'RefundAgent'`,
		"RefundAgent|1|1|1|1")
	assertAgentRows(t, cat, `SELECT Agent, Service, Enabled FROM agent_mcp_services`,
		"Agents.RefundAgent|Agents.JiraMCP|0")
	assertAgentRows(t, cat, `SELECT Agent, Name, KnowledgeBase, Collection, MaxResults, MinSimilarity, IsVisible, Enabled
		FROM agent_knowledge_bases`,
		"Agents.WorkInstructionAgent|WorkInstructionKnowledgeBase|AgenticAssistants.KB_WorkInstructions|PortalManagedContent|5|0.2|∅|1")

	assertAgentRows(t, cat, `SELECT Name, Provider, ModelType, DisplayName, KeyConstant FROM ai_models`,
		"Gpt|MxCloudGenAI|Text|GPT|Agents.GptKey")
	assertAgentRows(t, cat, `SELECT Name, Provider, KeyConstant FROM knowledge_bases`,
		"KB_WorkInstructions|MxCloudGenAI|AgenticAssistants.KB_WorkInstructionsKey")
	assertAgentRows(t, cat, `SELECT Name, ProtocolVersion, Version, ConnectionTimeoutSeconds,
			Endpoint, AuthenticationMicroflow
		FROM consumed_mcp_services ORDER BY Name`,
		"JiraMCP|v2025_03_26|1.0|30|Agents.JiraUrl|Agents.Jira_GetHeaders",
		"NoEndpointMCP|v2025_03_26|1.0|30||")
	assertAgentRows(t, cat, `SELECT Name, ServerName, EndpointPath, Version, ProtocolVersion, AuthenticationEnabled, AuthenticationMicroflow, ToolCount
		FROM published_mcp_services`,
		"Published_MCP_service|Published_MCP_service|published_mcp_service|1.0.0|v2025_03_26|0||1")
	assertAgentRows(t, cat, `SELECT ObjectType, QualifiedName FROM objects WHERE ObjectType = 'PUBLISHED_MCP_SERVICE'`,
		"PUBLISHED_MCP_SERVICE|Agents.Published_MCP_service")
}

// A published MCP service runs each tool's microflow and its authentication
// microflow, an entry point as a published REST operation is (`publish`):
// without the edge GRAPH_DEAD_ASSETS lists them.
func TestPublishedMCPServiceRefs_Issue2075(t *testing.T) {
	u := agentEditorRawUnit(t, "pub-1", "Published_MCP_service", types.CustomTypePublishedMCPService,
		`{"authenticationEnabled":true,"authenticationMicroflow":{"qualifiedName":"Agents.MCP_Auth"},
		  "tools":[{"uuid":"u1","name":"tool1","sourceMicroflow":{"qualifiedName":"AgentTest.mcptool1"}},
		           {"uuid":"u2","name":"unset"}]}`)
	doc, err := mprread.DecodeAgentEditorUnit("pub-1", string(agentDetailsModuleID), u.Contents)
	if err != nil {
		t.Fatal(err)
	}
	if got := agentEditorSourceType(doc); got != RefObjectPublishedMCPService {
		t.Errorf("source type = %q", got)
	}
	want := "publish MICROFLOW AgentTest.mcptool1|publish MICROFLOW Agents.MCP_Auth"
	if got := strings.Join(edgeStrings(agentEditorRefs(doc, resolveFixture)), "|"); got != want {
		t.Errorf("published MCP service edges = %q, want %q", got, want)
	}
}

// A consumed MCP service reads its server URL from a String constant and
// calls its credentials microflow for the request headers: without the edges
// GRAPH_DEAD_ASSETS lists both, and `show callers` finds no user
// (ako/mxcli#2090).
func TestConsumedMCPServiceRefs_Issue2090(t *testing.T) {
	u := agentEditorRawUnit(t, "mcp-1", "JiraMCP", types.CustomTypeConsumedMCPService,
		`{"endpoint":{"qualifiedName":"Agents.JiraUrl"},"protocolVersion":"v2025_03_26",
		  "authenticationMicroflow":{"qualifiedName":"Agents.Jira_GetHeaders"}}`)
	doc, err := mprread.DecodeAgentEditorUnit("mcp-1", string(agentDetailsModuleID), u.Contents)
	if err != nil {
		t.Fatal(err)
	}
	want := "call MICROFLOW Agents.Jira_GetHeaders|endpoint CONSTANT Agents.JiraUrl"
	if got := strings.Join(edgeStrings(agentEditorRefs(doc, resolveFixture)), "|"); got != want {
		t.Errorf("consumed MCP service edges = %q, want %q", got, want)
	}
	// Control: a service with neither has no edges.
	bare := agentEditorRawUnit(t, "mcp-2", "Bare", types.CustomTypeConsumedMCPService, `{"version":"1.0"}`)
	doc, err = mprread.DecodeAgentEditorUnit("mcp-2", string(agentDetailsModuleID), bare.Contents)
	if err != nil {
		t.Fatal(err)
	}
	if got := edgeStrings(agentEditorRefs(doc, resolveFixture)); len(got) != 0 {
		t.Errorf("bare service edges = %q, want none", got)
	}
}
