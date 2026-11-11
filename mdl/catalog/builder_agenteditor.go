// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// agentEditorTable maps a CustomBlobDocument's CustomDocumentType to its catalog
// table. The agent-editor document types share one BSON wrapper
// (CustomBlobDocuments$CustomBlobDocument) and are distinguished by this field.
var agentEditorTable = map[string]string{
	types.CustomTypeAgent:               "agents",
	types.CustomTypeModel:               "ai_models",
	types.CustomTypeKnowledgeBase:       "knowledge_bases",
	types.CustomTypeConsumedMCPService:  "consumed_mcp_services",
	types.CustomTypePublishedMCPService: "published_mcp_services",
}

// agentEditorColumns is one document's row: the columns its type adds after
// the wrapper's, and their values.
type agentEditorColumns struct {
	names  []string
	values []any
}

func (c *agentEditorColumns) add(name string, value any) {
	c.names = append(c.names, name)
	c.values = append(c.values, value)
}

// nullBool is a JSON boolean as a column: NULL when the document does not
// store it, which an older Agent Editor's does not, so a query can tell "off"
// from "never set".
func nullBool(b *bool) any {
	if b == nil {
		return nil
	}
	return boolToInt(*b)
}

// docRefName is a reference column: the referenced document's qualified name,
// or NULL when there is none. It used to be "", beside optional values that
// were already NULL in the same row, so "which consumed MCP services have no
// endpoint" compared Endpoint to the empty string while "which agents have no selected model" was
// `SelectedModel IS NULL` (ako/mxcli#2093). One rule: absent is NULL.
func docRefName(r *types.DocRef) any {
	if r == nil || r.QualifiedName == "" {
		return nil
	}
	return r.QualifiedName
}

// constantRefName is docRefName for a reference to a constant.
func constantRefName(r *types.ConstantRef) any {
	if r == nil || r.QualifiedName == "" {
		return nil
	}
	return r.QualifiedName
}

// agentEditorDetail is the type-specific part of a decoded document's row
// (ako/mxcli#2075). doc is nil when the Contents did not decode: the row then
// carries the wrapper's columns only.
func agentEditorDetail(doc any) agentEditorColumns {
	var c agentEditorColumns
	switch d := doc.(type) {
	case *types.Agent:
		tools, mcp := 0, 0
		for _, t := range d.Tools {
			if t.ToolType == "MCP" {
				mcp++
			} else {
				tools++
			}
		}
		var vars []string
		for _, v := range d.Variables {
			vars = append(vars, v.Key)
		}
		var selected, selectedName any
		if d.SelectedModel != nil {
			selected, selectedName = d.SelectedModel.ID, d.SelectedModel.Name
		}
		var maxTokens, temperature, topP any
		if d.MaxTokens != nil {
			maxTokens = *d.MaxTokens
		}
		if d.Temperature != nil {
			temperature = *d.Temperature
		}
		if d.TopP != nil {
			topP = *d.TopP
		}
		c.add("UsageType", d.UsageType)
		c.add("Model", docRefName(d.Model))
		c.add("SelectedModel", selected)
		c.add("SelectedModelName", selectedName)
		c.add("ToolChoice", d.ToolChoice)
		c.add("MaxTokens", maxTokens)
		c.add("Temperature", temperature)
		c.add("TopP", topP)
		c.add("EnableBackendStreaming", nullBool(d.EnableBackendStreaming))
		c.add("EnableUIStreaming", nullBool(d.EnableUIStreaming))
		c.add("Entity", docRefName(d.Entity))
		c.add("Variables", strings.Join(vars, ","))
		c.add("ToolCount", tools)
		c.add("KnowledgeBaseCount", len(d.KBTools))
		c.add("MCPServiceCount", mcp)
	case *types.Model:
		c.add("Provider", d.Provider)
		c.add("ModelType", d.Type)
		c.add("DisplayName", d.DisplayName)
		c.add("KeyConstant", constantRefName(d.Key))
	case *types.KnowledgeBase:
		c.add("Provider", d.Provider)
		c.add("KeyConstant", constantRefName(d.Key))
	case *types.ConsumedMCPService:
		c.add("ProtocolVersion", d.ProtocolVersion)
		c.add("Version", d.Version)
		c.add("ConnectionTimeoutSeconds", d.ConnectionTimeoutSeconds)
		c.add("Endpoint", docRefName(d.Endpoint))
		c.add("AuthenticationMicroflow", docRefName(d.AuthenticationMicroflow))
	case *types.PublishedMCPService:
		c.add("ServerName", d.ServerName)
		c.add("EndpointPath", d.EndpointPath)
		c.add("Version", d.Version)
		c.add("ProtocolVersion", d.ProtocolVersion)
		c.add("AuthenticationEnabled", boolToInt(d.AuthenticationEnabled))
		c.add("AuthenticationMicroflow", docRefName(d.AuthenticationMicroflow))
		c.add("ToolCount", len(d.Tools))
	}
	return c
}

// buildAgentEditorDocs catalogs the agent-editor documents (agent, model,
// knowledge base, consumed and published MCP service) with the settings their
// JSON Contents holds, an agent's tools and knowledge base tools as rows of
// their own, and queues each document's references for buildReferences.
func (b *Builder) buildAgentEditorDocs() error {
	units, err := b.lister().ListRawUnitsByType("CustomBlobDocuments$CustomBlobDocument")
	if err != nil {
		return err
	}

	stmts := map[string]*Stmt{}
	defer func() {
		for _, s := range stmts {
			s.Close()
		}
	}()
	prepare := func(sql string) (*Stmt, error) {
		if s, ok := stmts[sql]; ok {
			return s, nil
		}
		s, err := b.tx.Prepare(sql)
		if err != nil {
			return nil, err
		}
		stmts[sql] = s
		return s, nil
	}

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, u := range units {
		if !b.own(string(u.ID)) {
			continue
		}
		var wrap struct {
			Name               string `bson:"Name"`
			Documentation      string `bson:"Documentation"`
			CustomDocumentType string `bson:"CustomDocumentType"`
		}
		if err := bson.Unmarshal(u.Contents, &wrap); err != nil || wrap.Name == "" {
			continue
		}
		table, ok := agentEditorTable[wrap.CustomDocumentType]
		if !ok {
			continue // not an agent-editor document type we catalog
		}
		moduleID := b.hierarchy.findModuleID(u.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + wrap.Name
		folderPath := b.hierarchy.buildFolderPath(u.ContainerID)

		// A payload that does not decode still gets its row, with the
		// wrapper's columns only, and no edges.
		decoded, err := mprread.DecodeAgentEditorUnit(string(u.ID), string(u.ContainerID), u.Contents)
		if err != nil {
			decoded = nil
		}
		detail := agentEditorDetail(decoded)
		names := append([]string{"Id", "Name", "QualifiedName", "ModuleName", "Folder", "Description"}, detail.names...)
		names = append(names, "ProjectId", "SnapshotId")
		values := append([]any{string(u.ID), wrap.Name, qualifiedName, moduleName, folderPath, wrap.Documentation}, detail.values...)
		values = append(values, projectID, snapshotID)
		stmt, err := prepare(fmt.Sprintf("INSERT INTO %s_data (%s) VALUES (?%s)",
			table, strings.Join(names, ", "), strings.Repeat(", ?", len(names)-1)))
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(values...); err != nil {
			return err
		}
		count++

		if agent, ok := decoded.(*types.Agent); ok {
			if err := b.insertAgentTools(prepare, agent, qualifiedName, moduleName, projectID, snapshotID); err != nil {
				return err
			}
		}

		// What the document references; resolved and written by
		// buildReferences (ako/mxcli#2074).
		if decoded != nil {
			b.agentDocRefs = append(b.agentDocRefs, agentDocRefs{
				owner: string(u.ID), sourceType: agentEditorSourceType(decoded),
				source: qualifiedName, moduleName: moduleName, doc: decoded,
			})
		}
	}
	b.disown()

	b.report("Agent-editor documents", count)
	return nil
}

// insertAgentTools writes one agent_tools row per entry of the agent's tools
// list and one agent_knowledge_bases row per knowledge base tool, owned by the
// agent's unit (ako/mxcli#2075).
func (b *Builder) insertAgentTools(prepare func(string) (*Stmt, error), a *types.Agent, agent, moduleName, projectID, snapshotID string) error {
	toolStmt, err := prepare(`INSERT INTO agent_tools_data (Id, AgentId, Agent, ModuleName,
		Name, ToolType, Document, Enabled, UserAccessApproval, Description,
		DisplayTitle, DisplayDescription, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	optional := func(s *string) any {
		if s == nil {
			return nil
		}
		return *s
	}
	for i, t := range a.Tools {
		if _, err := toolStmt.Exec(
			rowID(string(a.ID), t.ID, i), string(a.ID), agent, moduleName,
			t.Name, t.ToolType, docRefName(t.Document), boolToInt(t.Enabled),
			t.UserAccessApproval, t.Description,
			optional(t.DisplayTitle), optional(t.DisplayDescription),
			projectID, snapshotID,
		); err != nil {
			return err
		}
	}
	kbStmt, err := prepare(`INSERT INTO agent_knowledge_bases_data (Id, AgentId, Agent, ModuleName,
		Name, KnowledgeBase, Collection, MaxResults, MinSimilarity, IsVisible,
		Enabled, Description, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for i, k := range a.KBTools {
		var minSimilarity any
		if k.MinSimilarity != nil {
			minSimilarity = *k.MinSimilarity
		}
		if _, err := kbStmt.Exec(
			rowID(string(a.ID), k.ID, i), string(a.ID), agent, moduleName,
			k.Name, docRefName(k.Document), k.CollectionIdentifier, k.MaxResults,
			minSimilarity, nullBool(k.IsVisible), boolToInt(k.Enabled), k.Description,
			projectID, snapshotID,
		); err != nil {
			return err
		}
	}
	return nil
}

// rowID is a list entry's own id, or its agent's id and position when the
// entry has none.
func rowID(agentID, id string, i int) string {
	if id != "" {
		return id
	}
	return fmt.Sprintf("%s#%d", agentID, i)
}
