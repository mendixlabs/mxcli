// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// agentEditorTable maps a CustomBlobDocument's CustomDocumentType to its catalog
// table. The four agent-editor document types share one BSON wrapper
// (CustomBlobDocuments$CustomBlobDocument) and are distinguished by this field.
var agentEditorTable = map[string]string{
	"agenteditor.agent":              "agents",
	"agenteditor.model":              "ai_models",
	"agenteditor.knowledgebase":      "knowledge_bases",
	"agenteditor.consumedMCPService": "consumed_mcp_services",
}

// buildAgentEditorDocs catalogs the agent-editor documents (agent / model /
// knowledge base / consumed MCP service). The document name is a top-level field
// of the BSON wrapper (the type-specific config lives in the inner JSON blob,
// which the catalog does not need), so this reads through the raw-unit surface
// and decodes only the wrapper fields — no CatalogReader/backend change.
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
	stmtFor := func(table string) (*Stmt, error) {
		if s, ok := stmts[table]; ok {
			return s, nil
		}
		s, err := b.tx.Prepare("INSERT INTO " + table + "_data " +
			"(Id, Name, QualifiedName, ModuleName, Folder, Description, ProjectId, SnapshotId) " +
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			return nil, err
		}
		stmts[table] = s
		return s, nil
	}

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, u := range units {
		if !b.own(string(u.ID)) {
			continue
		}
		var doc struct {
			Name               string `bson:"Name"`
			Documentation      string `bson:"Documentation"`
			CustomDocumentType string `bson:"CustomDocumentType"`
		}
		if err := bson.Unmarshal(u.Contents, &doc); err != nil || doc.Name == "" {
			continue
		}
		table, ok := agentEditorTable[doc.CustomDocumentType]
		if !ok {
			continue // not an agent-editor document type we catalog
		}
		stmt, err := stmtFor(table)
		if err != nil {
			return err
		}
		moduleID := b.hierarchy.findModuleID(u.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + doc.Name
		folderPath := b.hierarchy.buildFolderPath(u.ContainerID)

		if _, err := stmt.Exec(
			string(u.ID), doc.Name, qualifiedName, moduleName, folderPath,
			doc.Documentation, projectID, snapshotID,
		); err != nil {
			return err
		}
		count++

		// What the document references; resolved and written by
		// buildReferences (ako/mxcli#2074). A payload that does not decode
		// still has its row above, only no edges.
		if decoded, err := mprread.DecodeAgentEditorUnit(string(u.ID), string(u.ContainerID), u.Contents); err == nil && decoded != nil {
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
