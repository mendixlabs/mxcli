// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"crypto/sha256"
	"fmt"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/microflows"
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
// xpathInsertSQL writes one xpath_expressions row; every extractor uses it.
const xpathInsertSQL = `
		INSERT INTO xpath_expressions_data (
			Id, DocumentType, DocumentId, DocumentQualifiedName,
			ComponentType, ComponentId, ComponentName,
			XPathExpression, TargetEntity, ReferencedEntities,
			IsParameterized, UsageType, ModuleName,
			WidgetId, WidgetName,
			ProjectId, SnapshotId
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

func (b *Builder) buildXPathExpressions() error {
	if !b.fullMode {
		return nil
	}

	stmt, err := b.tx.Prepare(xpathInsertSQL)
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
			if !b.own(string(mf.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(mf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			sourceQN := moduleName + "." + mf.Name

			count += b.extractMicroflowXPath(stmt, mf.ObjectCollection, "MICROFLOW", string(mf.ID), sourceQN, moduleName,
				projectID, snapshotID)
		}
		b.disown()
	}

	nfs, err := b.cachedNanoflows()
	if err == nil {
		for _, nf := range nfs {
			if !b.own(string(nf.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(nf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			sourceQN := moduleName + "." + nf.Name

			count += b.extractMicroflowXPath(stmt, nf.ObjectCollection, "NANOFLOW", string(nf.ID), sourceQN, moduleName,
				projectID, snapshotID)
		}
		b.disown()
	}

	// 2. Extract from entity access rules
	dms, err := b.cachedDomainModels()
	if err == nil {
		for _, dm := range dms {
			if !b.own(string(dm.ID)) {
				continue
			}
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
						"", "",
						projectID, snapshotID)
					count++
				}
			}
		}
		b.disown()
	}

	// 3. Extract from page/snippet data sources by scanning raw BSON
	// The page parser only reads metadata, not the widget tree, so we scan the
	// raw BSON documents for XPathConstraint fields at any nesting depth.
	for _, typePrefix := range []string{"Forms$Page", "Pages$Page", "Forms$Snippet", "Pages$Snippet"} {
		rawPages, err := b.lister().ListRawUnitsByType(typePrefix)
		if err != nil {
			continue
		}
		for _, ru := range rawPages {
			if !b.own(string(ru.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(ru.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)

			// Parse just the Name field from the BSON
			var raw map[string]any
			if err := bsoncompat.UnmarshalDocMap(ru.Contents, &raw); err != nil {
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
		b.disown()
	}

	b.report("XPath Expressions", count)
	return nil
}

// extractMicroflowXPath extracts XPath constraints from microflow/nanoflow actions.
func (b *Builder) extractMicroflowXPath(stmt *Stmt,
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
			"", "",
			projectID, snapshotID)
		count++
	}
	return count
}

// scanBSONForXPath recursively scans a BSON map for XPathConstraint fields
// and inserts records into the xpath_expressions table.
// This works on raw BSON data, avoiding the need for a full widget tree parser.
func scanBSONForXPath(stmt *Stmt, raw map[string]any,
	docType, docID, docQN, moduleName,
	projectID, snapshotID string) int {
	return scanXPathUnder(stmt, raw, docType, docID, docQN, moduleName, projectID, snapshotID, "", "", true)
}

// isWidgetNode reports whether a node is a widget the widgets table indexes:
// a named page or pluggable element below the document root.
func isWidgetNode(raw map[string]any, root bool) bool {
	if root {
		return false
	}
	ty, _ := raw["$Type"].(string)
	name, _ := raw["Name"].(string)
	return name != "" && (strings.HasPrefix(ty, "Forms$") || strings.HasPrefix(ty, "Pages$") || ty == "CustomWidgets$CustomWidget")
}

// scanXPathUnder is scanBSONForXPath carrying the nearest enclosing widget, so
// each row names the widget its data source belongs to (ako/mxcli#1879).
func scanXPathUnder(stmt *Stmt, raw map[string]any,
	docType, docID, docQN, moduleName,
	projectID, snapshotID, widgetID, widgetName string, root bool) int {

	count := 0
	if isWidgetNode(raw, root) {
		widgetID, widgetName = extractBsonID(raw["$ID"]), extractString(raw["Name"])
	}

	// Check if this node has an XPathConstraint
	xpath := extractString(raw["XPathConstraint"])
	if xpath == "" {
		xpath = extractString(raw["XpathConstraint"])
	}
	if xpath != "" {
		bsonType, _ := raw["$Type"].(string)
		// The catalog's GUID form, as every other table writes an id; %v of a
		// bson.Binary was `{0 [57 221 …]}` (ako/mxcli#1879).
		bsonID := extractBsonID(raw["$ID"])
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
			widgetID, widgetName,
			projectID, snapshotID)
		count++
	}

	// Recurse into all nested maps and arrays
	for _, v := range raw {
		switch val := v.(type) {
		case map[string]any:
			count += scanXPathUnder(stmt, val, docType, docID, docQN, moduleName,
				projectID, snapshotID, widgetID, widgetName, false)
		case bson.D:
			m := make(map[string]any)
			for _, elem := range val {
				m[elem.Key] = elem.Value
			}
			count += scanXPathUnder(stmt, m, docType, docID, docQN, moduleName,
				projectID, snapshotID, widgetID, widgetName, false)
		default:
			scanBSONArray(v, func(child map[string]any) {
				count += scanXPathUnder(stmt, child, docType, docID, docQN, moduleName,
					projectID, snapshotID, widgetID, widgetName, false)
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
//
// Studio Pro stores a DomainModels$DirectEntityRef with the name under
// `Entity`, and a DomainModels$IndirectEntityRef (a data source over an
// association path) as `Steps` ending on the entity the constraint applies to.
// `QualifiedName` is kept for the synthetic shape older callers pass; no stored
// EntityRef carries it, which is why every page XPath used to be recorded with
// no target entity.
func resolveEntityRefFromBSON(raw map[string]any) string {
	ref := bsonAsMap(raw["EntityRef"])
	if ref == nil {
		return ""
	}
	for _, key := range []string{"Entity", "QualifiedName"} {
		if name, ok := ref[key].(string); ok && name != "" {
			return name
		}
	}
	last := ""
	scanBSONArray(ref["Steps"], func(step map[string]any) {
		if dest, ok := step["DestinationEntity"].(string); ok && dest != "" {
			last = dest
		}
	})
	return last
}

// bsonAsMap returns a decoded sub-document as a map, whichever shape the
// decoder produced it in; nil when v is not a document.
func bsonAsMap(v any) map[string]any {
	switch d := v.(type) {
	case map[string]any:
		return d
	case bson.M:
		return d
	case bson.D:
		m := make(map[string]any, len(d))
		for _, e := range d {
			m[e.Key] = e.Value
		}
		return m
	}
	return nil
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
