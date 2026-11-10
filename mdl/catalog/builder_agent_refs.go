// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Edges from agent-editor documents and from code action parameters
// (ako/mxcli#2074). Neither produced a refs row, so an agent's tool microflow,
// a chat context's action microflow and an MCP server's authentication
// microflow read as dead code: `show callers` said "(no callers found)" and
// GRAPH_DEAD_ASSETS listed them, though deleting one breaks the app at
// runtime, where neither mx check nor a build would notice.

// docRef is one edge out of a document: what it names and how.
type docRef struct {
	targetType string
	target     string
	kind       string
}

// agentDocRefs is a decoded agent-editor document waiting for buildReferences,
// which resolves its names once every document table is filled.
type agentDocRefs struct {
	owner      string
	sourceType string
	source     string
	moduleName string
	doc        any
}

// agentEditorSourceType is the refs.SourceType of a decoded agent-editor
// document, the name the objects view gives it.
func agentEditorSourceType(doc any) string {
	switch doc.(type) {
	case *types.Agent:
		return RefObjectAgent
	case *types.Model:
		return RefObjectAIModel
	case *types.KnowledgeBase:
		return RefObjectKnowledgeBase
	case *types.ConsumedMCPService:
		return RefObjectConsumedMCPService
	}
	return ""
}

// agentEditorRefs lists the edges out of one decoded agent-editor document.
// resolve gives the object type the catalog holds a qualified name under, ""
// when it holds none.
//
//   - an agent calls its tools: a microflow, a nanoflow or another agent is a
//     `call`, so `show callers` and the dead-asset view see it; a consumed MCP
//     service or a knowledge base it draws on is a `tool`;
//   - an agent runs on its `model`, and is typed on its context entity (`type`);
//   - a model or knowledge base reads its key from a constant (`key`).
func agentEditorRefs(doc any, resolve func(string) string) []docRef {
	var out []docRef
	add := func(qn, targetType, kind string) {
		if qn != "" && targetType != "" {
			out = append(out, docRef{targetType: targetType, target: qn, kind: kind})
		}
	}
	switch d := doc.(type) {
	case *types.Agent:
		for _, tool := range d.Tools {
			if tool.Document == nil || tool.Document.QualifiedName == "" {
				continue
			}
			qn := tool.Document.QualifiedName
			targetType := resolve(qn)
			if targetType == "" {
				// The document is not in the catalog (deleted, or in a module
				// the build skipped): type it from the tool, as a call to a
				// missing microflow is still recorded.
				switch tool.ToolType {
				case "MCP":
					targetType = RefObjectConsumedMCPService
				default:
					targetType = RefObjectMicroflow
				}
			}
			kind := RefKindCall
			if targetType == RefObjectConsumedMCPService || targetType == RefObjectKnowledgeBase {
				kind = RefKindTool
			}
			add(qn, targetType, kind)
		}
		for _, kb := range d.KBTools {
			if kb.Document != nil {
				add(kb.Document.QualifiedName, RefObjectKnowledgeBase, RefKindTool)
			}
		}
		if d.Model != nil {
			add(d.Model.QualifiedName, RefObjectAIModel, RefKindModel)
		}
		if d.Entity != nil {
			add(d.Entity.QualifiedName, RefObjectEntity, RefKindType)
		}
	case *types.Model:
		if d.Key != nil {
			add(d.Key.QualifiedName, RefObjectConstant, RefKindKey)
		}
	case *types.KnowledgeBase:
		if d.Key != nil {
			add(d.Key.QualifiedName, RefObjectConstant, RefKindKey)
		}
	}
	return out
}

// codeActionParameterRefs lists the documents a Java or JavaScript action call
// passes by reference: a microflow for a parameter of type Microflow, and a
// custom document (an agent, model, knowledge base or MCP service) for one of
// a custom document type. Each is a `call`: the action runs it, by name, at
// runtime. An expression argument is not a reference, even when its text is
// a microflow's name.
func codeActionParameterRefs(action microflows.MicroflowAction, resolve func(string) string) []docRef {
	var values []microflows.CodeActionParameterValue
	switch a := action.(type) {
	case *microflows.JavaActionCallAction:
		for _, m := range a.ParameterMappings {
			if m != nil {
				values = append(values, m.Value)
			}
		}
	case *microflows.JavaScriptActionCallAction:
		for _, m := range a.ParameterMappings {
			if m != nil {
				values = append(values, m.Value)
			}
		}
	}
	var out []docRef
	for _, v := range values {
		switch p := v.(type) {
		case *microflows.MicroflowParameterValue:
			if p.Microflow != "" {
				out = append(out, docRef{RefObjectMicroflow, p.Microflow, RefKindCall})
			}
		case *microflows.CustomBlobDocumentCodeActionParameterValue:
			if p.CustomDocument == "" {
				continue
			}
			targetType := resolve(p.CustomDocument)
			if targetType == "" {
				targetType = RefObjectAgent
			}
			out = append(out, docRef{targetType, p.CustomDocument, RefKindCall})
		}
	}
	return out
}

// agentEditorResolver maps the qualified name of every document an agent or
// a code action can name to its object type, from the tables this build has
// filled.
func (b *Builder) agentEditorResolver() func(string) string {
	known := map[string]string{}
	for table, objType := range map[string]string{
		"microflows_data":            "", // MicroflowType says which
		"agents_data":                RefObjectAgent,
		"ai_models_data":             RefObjectAIModel,
		"knowledge_bases_data":       RefObjectKnowledgeBase,
		"consumed_mcp_services_data": RefObjectConsumedMCPService,
		"constants_data":             RefObjectConstant,
		"entities_data":              RefObjectEntity,
	} {
		query := "SELECT QualifiedName, '" + objType + "' FROM " + table
		if table == "microflows_data" {
			query = "SELECT QualifiedName, MicroflowType FROM microflows_data"
		}
		rows, err := b.tx.Query(query)
		if err != nil {
			continue
		}
		for rows.Next() {
			var qn, t string
			if rows.Scan(&qn, &t) == nil {
				known[qn] = t
			}
		}
		rows.Close()
	}
	return func(qn string) string { return known[qn] }
}

// extractAgentEditorRefs writes the edges of every agent-editor document the
// agent pass decoded, each owned by that document's unit.
func (b *Builder) extractAgentEditorRefs(stmt *Stmt, resolve func(string) string, projectID, snapshotID string) int {
	defer b.disown()
	count := 0
	for _, d := range b.agentDocRefs {
		b.as(d.owner)
		for _, r := range agentEditorRefs(d.doc, resolve) {
			if _, err := stmt.Exec(
				d.sourceType, "", d.source,
				r.targetType, "", r.target,
				r.kind, d.moduleName, projectID, snapshotID,
			); err == nil {
				count++
			}
		}
	}
	return count
}
