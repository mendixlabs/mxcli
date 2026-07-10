// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// XPath usage types
const (
	XPathUsageRetrieve   = "RETRIEVE"
	XPathUsageSecurity   = "SECURITY"
	XPathUsageDatasource = "DATASOURCE"
)

// buildXPathExpressions extracts all XPath constraint expressions from microflows,
// domain model access rules, and page data sources into the xpath_expressions table.
// Only runs in full mode.
func (b *Builder) buildXPathExpressions() error {
	if !b.fullMode {
		return nil
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO xpath_expressions_data (
			Id, DocumentType, DocumentId, DocumentQualifiedName,
			ComponentType, ComponentId, ComponentName,
			XPathExpression, TargetEntity, ReferencedEntities,
			IsParameterized, UsageType, ModuleName,
			ProjectId, SnapshotId
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()
	count := 0

	// 1. Extract from microflow/nanoflow retrieve actions
	mfs, err := b.cachedMicroflows()
	if err == nil {
		for _, mf := range mfs {
			moduleID := b.hierarchy.findModuleID(mf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			sourceQN := moduleName + "." + mf.Name

			count += b.extractMicroflowXPath(stmt, mf.ObjectCollection, "MICROFLOW", string(mf.ID), sourceQN, moduleName,
				projectID, snapshotID)
		}
	}

	nfs, err := b.cachedNanoflows()
	if err == nil {
		for _, nf := range nfs {
			moduleID := b.hierarchy.findModuleID(nf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			sourceQN := moduleName + "." + nf.Name

			count += b.extractMicroflowXPath(stmt, nf.ObjectCollection, "NANOFLOW", string(nf.ID), sourceQN, moduleName,
				projectID, snapshotID)
		}
	}

	// 2. Extract from entity access rules
	dms, err := b.cachedDomainModels()
	if err == nil {
		for _, dm := range dms {
			moduleID := b.hierarchy.findModuleID(dm.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)

			for _, ent := range dm.Entities {
				entityQN := moduleName + "." + ent.Name

				for _, rule := range ent.AccessRules {
					if rule.XPathConstraint == "" {
						continue
					}

					id := xpathID(string(ent.ID), string(rule.ID), rule.XPathConstraint)
					isParam := boolToInt(containsVariable(rule.XPathConstraint))
					refs := extractReferencedEntities(rule.XPathConstraint)

					stmt.Exec(id, "DOMAIN_MODEL", string(dm.ID), entityQN,
						"ACCESS_RULE", string(rule.ID), "",
						rule.XPathConstraint, entityQN, refs,
						isParam, XPathUsageSecurity, moduleName,
						projectID, snapshotID)
					count++
				}
			}
		}
	}

	// 3. Extract from page/snippet data sources by scanning raw BSON
	// The page parser only reads metadata, not the widget tree, so we scan the
	// raw BSON documents for XPathConstraint fields at any nesting depth.
	for _, typePrefix := range []string{"Forms$Page", "Pages$Page", "Forms$Snippet", "Pages$Snippet"} {
		rawPages, err := b.reader.ListRawUnitsByType(typePrefix)
		if err != nil {
			continue
		}
		for _, ru := range rawPages {
			moduleID := b.hierarchy.findModuleID(ru.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)

			// Parse just the Name field from the BSON
			var raw map[string]any
			if err := bson.Unmarshal(ru.Contents, &raw); err != nil {
				continue
			}
			name, _ := raw["Name"].(string)
			sourceQN := moduleName + "." + name

			docType := "PAGE"
			if strings.Contains(typePrefix, "Snippet") {
				docType = "SNIPPET"
			}

			// Scan the full BSON tree for XPathConstraint fields
			count += scanBSONForXPath(stmt, raw, docType, string(ru.ID), sourceQN, moduleName,
				projectID, snapshotID)
		}
	}

	b.report("XPath Expressions", count)
	return nil
}

// extractMicroflowXPath extracts XPath constraints from microflow/nanoflow actions.
func (b *Builder) extractMicroflowXPath(stmt *sql.Stmt,
	oc *microflows.MicroflowObjectCollection, docType, docID, docQN, moduleName,
	projectID, snapshotID string) int {

	if oc == nil {
		return 0
	}

	count := 0
	for _, obj := range oc.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok || act.Action == nil {
			continue
		}

		var xpath, entityQN, componentName string
		var componentID string

		switch a := act.Action.(type) {
		case *microflows.RetrieveAction:
			if a.Source == nil {
				continue
			}
			dbSrc, ok := a.Source.(*microflows.DatabaseRetrieveSource)
			if !ok || dbSrc.XPathConstraint == "" {
				continue
			}
			xpath = dbSrc.XPathConstraint
			entityQN = dbSrc.EntityQualifiedName
			componentID = string(act.ID)
			componentName = act.Caption
		default:
			continue
		}

		id := xpathID(docID, componentID, xpath)
		isParam := boolToInt(containsVariable(xpath))
		refs := extractReferencedEntities(xpath)

		stmt.Exec(id, docType, docID, docQN,
			"RETRIEVE_ACTION", componentID, componentName,
			xpath, entityQN, refs,
			isParam, XPathUsageRetrieve, moduleName,
			projectID, snapshotID)
		count++
	}
	return count
}

// scanBSONForXPath recursively scans a BSON map for XPathConstraint fields
// and inserts records into the xpath_expressions table.
// This works on raw BSON data, avoiding the need for a full widget tree parser.
func scanBSONForXPath(stmt *sql.Stmt, raw map[string]any,
	docType, docID, docQN, moduleName,
	projectID, snapshotID string) int {

	count := 0

	// Check if this node has an XPathConstraint
	xpath := extractString(raw["XPathConstraint"])
	if xpath == "" {
		xpath = extractString(raw["XpathConstraint"])
	}
	if xpath != "" {
		bsonType, _ := raw["$Type"].(string)
		bsonID := extractBsonIDString(raw["$ID"])
		entityQN := resolveEntityRefFromBSON(raw)

		componentType := "WIDGET"
		if strings.Contains(bsonType, "AccessRule") {
			componentType = "ACCESS_RULE"
		}

		id := xpathID(docID, bsonID, xpath)
		isParam := boolToInt(containsVariable(xpath))
		refs := extractReferencedEntities(xpath)

		stmt.Exec(id, docType, docID, docQN,
			componentType, bsonID, bsonType,
			xpath, entityQN, refs,
			isParam, XPathUsageDatasource, moduleName,
			projectID, snapshotID)
		count++
	}

	// Recurse into all nested maps and arrays
	for _, v := range raw {
		switch val := v.(type) {
		case map[string]any:
			count += scanBSONForXPath(stmt, val, docType, docID, docQN, moduleName,
				projectID, snapshotID)
		case bson.D:
			m := make(map[string]any)
			for _, elem := range val {
				m[elem.Key] = elem.Value
			}
			count += scanBSONForXPath(stmt, m, docType, docID, docQN, moduleName,
				projectID, snapshotID)
		default:
			scanBSONArray(v, func(child map[string]any) {
				count += scanBSONForXPath(stmt, child, docType, docID, docQN, moduleName,
					projectID, snapshotID)
			})
		}
	}

	return count
}

// scanBSONArray iterates array values, calling fn for each map element.
func scanBSONArray(v any, fn func(map[string]any)) {
	switch arr := v.(type) {
	case bson.A:
		for _, item := range arr {
			switch m := item.(type) {
			case map[string]any:
				fn(m)
			case bson.D:
				mapped := make(map[string]any)
				for _, elem := range m {
					mapped[elem.Key] = elem.Value
				}
				fn(mapped)
			}
		}
	case []any:
		for _, item := range arr {
			switch m := item.(type) {
			case map[string]any:
				fn(m)
			case bson.D:
				mapped := make(map[string]any)
				for _, elem := range m {
					mapped[elem.Key] = elem.Value
				}
				fn(mapped)
			}
		}
	}
}

// resolveEntityRefFromBSON extracts a qualified entity name from a BSON node
// that has an EntityRef field (common in data source nodes).
func resolveEntityRefFromBSON(raw map[string]any) string {
	// Try EntityRef (used by most data sources)
	if entityRef, ok := raw["EntityRef"].(map[string]any); ok {
		if name, ok := entityRef["QualifiedName"].(string); ok {
			return name
		}
	}
	// Try bson.D format
	if entityRef, ok := raw["EntityRef"].(bson.D); ok {
		for _, elem := range entityRef {
			if elem.Key == "QualifiedName" {
				if name, ok := elem.Value.(string); ok {
					return name
				}
			}
		}
	}
	return ""
}

// extractBsonIDString extracts a BSON ID as a string from various formats.
func extractBsonIDString(v any) string {
	if v == nil {
		return ""
	}
	switch id := v.(type) {
	case string:
		return id
	case []byte:
		return fmt.Sprintf("%x", id)
	default:
		return fmt.Sprintf("%v", id)
	}
}

// xpathID generates a deterministic ID from the document, component, and expression.
func xpathID(docID, componentID, xpath string) string {
	h := sha256.Sum256([]byte(docID + "|" + componentID + "|" + xpath))
	return fmt.Sprintf("%x", h[:16])
}

// containsVariable checks if an XPath expression uses $variable references.
func containsVariable(xpath string) bool {
	return strings.Contains(xpath, "$")
}

// extractReferencedEntities extracts qualified entity/association names from XPath.
// Returns a comma-separated list of Module.Name patterns found.
func extractReferencedEntities(xpath string) string {
	var refs []string
	seen := make(map[string]bool)

	// Scan for qualified names (Module.Name pattern) that aren't string literals
	inString := false
	for i := 0; i < len(xpath); i++ {
		if xpath[i] == '\'' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}

		// Look for uppercase letter starting a potential qualified name
		if isUpperLetter(xpath[i]) {
			// Scan ahead for Module.Name pattern
			j := i
			for j < len(xpath) && (isIdentChar(xpath[j]) || xpath[j] == '.') {
				j++
			}
			name := xpath[i:j]
			if strings.Contains(name, ".") && !seen[name] {
				refs = append(refs, name)
				seen[name] = true
			}
			i = j - 1
		}
	}

	return strings.Join(refs, ",")
}

func isUpperLetter(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
