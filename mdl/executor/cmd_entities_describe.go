// SPDX-License-Identifier: Apache-2.0

// Package executor - Entity display and describe commands (SHOW/DESCRIBE ENTITY)
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/doccomment"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// listEntities handles SHOW ENTITIES command.
func listEntities(ctx *ExecContext, moduleName string) error {
	// Build module ID -> name map (single query)
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}

	// Get all domain models in a single query (avoids O(n²) behavior)
	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}

	// Build entity ID -> association count map
	assocCounts := make(map[model.ID]int)
	for _, dm := range domainModels {
		for _, assoc := range dm.Associations {
			assocCounts[assoc.ParentID]++
			assocCounts[assoc.ChildID]++
		}
	}

	// Collect System entities referenced via generalizations
	systemEntities := make(map[string]bool)
	for _, dm := range domainModels {
		for _, entity := range dm.Entities {
			if entity.GeneralizationRef != "" && strings.HasPrefix(entity.GeneralizationRef, "System.") {
				systemEntities[entity.GeneralizationRef] = true
			}
		}
	}

	// Collect rows and calculate column widths
	type row struct {
		qualifiedName  string
		entityType     string
		generalization string
		attrs          int
		assocs         int
		validations    int
		indexes        int
		events         int
		accessRules    int
	}
	var rows []row

	// Add System entities first (if showing all or System module)
	if moduleName == "" || moduleName == "System" {
		for sysEntity := range systemEntities {
			r := row{
				qualifiedName: sysEntity,
				entityType:    "System",
				attrs:         -1, // Unknown - from runtime
				assocs:        -1,
				validations:   -1,
				indexes:       -1,
				events:        -1,
				accessRules:   -1,
			}
			rows = append(rows, r)
		}
	}

	for _, dm := range domainModels {
		modName := moduleNames[dm.ContainerID]
		// Filter by module name if specified
		if moduleName != "" && modName != moduleName {
			continue
		}
		for _, entity := range dm.Entities {
			// Determine entity type based on Source field and Persistable flag
			entityType := "Persistent"
			if strings.Contains(entity.Source, "OqlView") {
				entityType = "View"
			} else if strings.Contains(entity.Source, "OData") || entity.RemoteSource != "" || entity.RemoteSourceDocument != "" {
				entityType = "External"
			} else if !entity.Persistable {
				entityType = "Non-Persistent"
			}

			qualifiedName := modName + "." + entity.Name
			r := row{
				qualifiedName:  qualifiedName,
				entityType:     entityType,
				generalization: entity.GeneralizationRef,
				attrs:          len(entity.Attributes),
				assocs:         assocCounts[entity.ID],
				validations:    len(entity.ValidationRules),
				indexes:        len(entity.Indexes),
				events:         len(entity.EventHandlers),
				accessRules:    len(entity.AccessRules),
			}
			rows = append(rows, r)
		}
	}

	// Check if any entity has a generalization — only show column if needed
	hasGeneralizations := false
	for _, r := range rows {
		if r.generalization != "" {
			hasGeneralizations = true
			break
		}
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	// Build TableResult
	columns := []string{"Entity", "Type"}
	if hasGeneralizations {
		columns = append(columns, "Extends")
	}
	columns = append(columns, "Attrs", "Assocs", "Validations", "Indexes", "Events", "AccessRules")

	result := &TableResult{
		Columns: columns,
		Summary: fmt.Sprintf("(%d entities)", len(rows)),
	}
	for _, r := range rows {
		var rowData []any
		rowData = append(rowData, r.qualifiedName, r.entityType)
		if hasGeneralizations {
			rowData = append(rowData, r.generalization)
		}
		if r.entityType == "System" {
			rowData = append(rowData, "-", "-", "-", "-", "-", "-")
		} else {
			rowData = append(rowData, r.attrs, r.assocs, r.validations, r.indexes, r.events, r.accessRules)
		}
		result.Rows = append(result.Rows, rowData)
	}
	return writeResult(ctx, result)
}

// listEntity handles SHOW ENTITY command.
func listEntity(ctx *ExecContext, name *ast.QualifiedName) error {
	if name == nil {
		return mdlerrors.NewValidation("entity name required")
	}

	module, err := findModule(ctx, name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	for _, entity := range dm.Entities {
		if entity.Name == name.Name {
			fmt.Fprintf(ctx.Output, "**Entity: %s.%s**\n\n", module.Name, entity.Name)
			fmt.Fprintf(ctx.Output, "- Persistable: %v\n", entity.Persistable)
			if entity.GeneralizationRef != "" {
				fmt.Fprintf(ctx.Output, "- Extends: %s\n", entity.GeneralizationRef)
			}
			fmt.Fprintf(ctx.Output, "- Location: (%d, %d)\n\n", entity.Location.X, entity.Location.Y)

			if len(entity.Attributes) > 0 {
				// Calculate column widths
				nameWidth, typeWidth := len("Attribute"), len("Type")
				type attrRow struct {
					name, typeName string
				}
				var rows []attrRow
				for _, attr := range entity.Attributes {
					typeName := getAttributeTypeName(attr.Type)
					rows = append(rows, attrRow{attr.Name, typeName})
					if len(attr.Name) > nameWidth {
						nameWidth = len(attr.Name)
					}
					if len(typeName) > typeWidth {
						typeWidth = len(typeName)
					}
				}

				fmt.Fprintf(ctx.Output, "| %-*s | %-*s |\n", nameWidth, "Attribute", typeWidth, "Type")
				fmt.Fprintf(ctx.Output, "|-%s-|-%s-|\n", strings.Repeat("-", nameWidth), strings.Repeat("-", typeWidth))
				for _, r := range rows {
					fmt.Fprintf(ctx.Output, "| %-*s | %-*s |\n", nameWidth, r.name, typeWidth, r.typeName)
				}
				fmt.Fprintf(ctx.Output, "\n(%d attributes)\n", len(entity.Attributes))
			}
			return nil
		}
	}

	return mdlerrors.NewNotFound("entity", name.String())
}

// describeEntity handles DESCRIBE ENTITY command.
func describeEntity(ctx *ExecContext, name ast.QualifiedName) error {
	return describeEntityWith(ctx, name, true)
}

// describeEntityWith describes an entity, with its access rules as grants when
// withGrants is set. `describe module` leaves them out and prints them after
// the module's associations, which a grant may name: replayed in entity order,
// such a grant was refused because the association did not exist yet
// (mendixlabs/mxcli#1409).
func describeEntityWith(ctx *ExecContext, name ast.QualifiedName, withGrants bool) error {
	module, err := findModule(ctx, name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	lang := describeDefaultLanguage(ctx)

	for _, entity := range dm.Entities {
		if entity.Name == name.Name {
			// Output JavaDoc documentation if present
			if entity.Documentation != "" {
				outputJavadocIndented(ctx.Output, entity.Documentation, "")
			}

			// Output position annotation
			fmt.Fprintf(ctx.Output, "@Position(%d, %d)\n", entity.Location.X, entity.Location.Y)

			// Determine entity type based on Source field and Persistable flag
			entityType := "persistent"
			if strings.Contains(entity.Source, "OqlView") {
				entityType = "view"
			} else if strings.Contains(entity.Source, "OData") || entity.RemoteSource != "" || entity.RemoteSourceDocument != "" {
				entityType = "external"
			} else if !entity.Persistable {
				entityType = "non-persistent"
			}

			if entity.GeneralizationRef != "" {
				fmt.Fprintf(ctx.Output, "create or modify %s entity %s.%s extends %s (\n", entityType, module.Name, entity.Name, entity.GeneralizationRef)
			} else {
				fmt.Fprintf(ctx.Output, "create or modify %s entity %s.%s (\n", entityType, module.Name, entity.Name)
			}

			// Build validation rules map by attribute ID and name
			// The AttributeID can be a UUID or a qualified name like "DmTest.Cars.CarId"
			validationsByAttr := make(map[model.ID][]*domainmodel.ValidationRule)
			validationsByName := make(map[string][]*domainmodel.ValidationRule)
			for _, vr := range entity.ValidationRules {
				validationsByAttr[vr.AttributeID] = append(validationsByAttr[vr.AttributeID], vr)
				// Also index by attribute name extracted from qualified name
				attrName := extractAttrNameFromQualified(string(vr.AttributeID))
				if attrName != "" {
					validationsByName[attrName] = append(validationsByName[attrName], vr)
				}
			}

			// Build the list of attribute lines (regular + system pseudo-types)
			type attrLine struct {
				text string
			}
			var attrLines []attrLine

			// Output regular attributes
			for _, attr := range entity.Attributes {
				var line strings.Builder

				// Attribute documentation
				if attr.Documentation != "" {
					// The inline form only holds one line that does not itself
					// start with `*`: the parser strips one leading `*` per line
					// as the gutter, so anything else needs the guttered block
					// or a markdown bullet is eaten (mendixlabs/mxcli#1428).
					if !strings.Contains(attr.Documentation, "\n") && !strings.HasPrefix(attr.Documentation, "*") {
						line.WriteString(fmt.Sprintf("  /** %s */\n", doccomment.Escape(attr.Documentation)))
					} else {
						outputJavadocIndented(&line, attr.Documentation, "  ")
					}
				}

				typeStr := formatAttributeType(attr.Type)
				var constraints strings.Builder
				// A view's DateTime LocalizeDate is derived from its OQL source
				// (#1297); stating it would pin a value that must follow it.
				if entityType != "view" {
					constraints.WriteString(localizeClause(attr.Type))
				}

				// Check for validation rules - try by ID first, then by name
				attrValidations := validationsByAttr[attr.ID]
				if len(attrValidations) == 0 {
					attrValidations = validationsByName[attr.Name]
				}
				for _, vr := range attrValidations {
					if vr.Type == "Required" {
						constraints.WriteString(" not null")
						if errMsg := pickTextTranslation(vr.ErrorMessage, lang); errMsg != "" {
							constraints.WriteString(" error message " + mdlQuoted(errMsg))
						}
					}
					if vr.Type == "Unique" {
						constraints.WriteString(" unique")
						if errMsg := pickTextTranslation(vr.ErrorMessage, lang); errMsg != "" {
							constraints.WriteString(" error message " + mdlQuoted(errMsg))
						}
					}
				}

				// Value type: CALCULATED or DEFAULT
				if attr.Value != nil && attr.Value.Type == "CalculatedValue" {
					constraints.WriteString(" calculated")
					if attr.Value.MicroflowName != "" {
						constraints.WriteString(" by " + attr.Value.MicroflowName)
					} else if attr.Value.MicroflowID != "" {
						if mfName := lookupMicroflowName(ctx, attr.Value.MicroflowID); mfName != "" {
							constraints.WriteString(" by " + mfName)
						}
					}
				} else if attr.Value != nil && attr.Value.DefaultValue != "" {
					defaultVal := attr.Value.DefaultValue
					// Quote string defaults
					if _, ok := attr.Type.(*domainmodel.StringAttributeType); ok {
						defaultVal = mdlQuoted(defaultVal)
					}
					// Re-qualify enum defaults for MDL syntax (BSON stores just the value name)
					if enumType, ok := attr.Type.(*domainmodel.EnumerationAttributeType); ok {
						if enumType.EnumerationRef != "" && !strings.Contains(defaultVal, ".") {
							defaultVal = enumType.EnumerationRef + "." + defaultVal
						}
					}
					constraints.WriteString(fmt.Sprintf(" default %s", defaultVal))
				}
				// In a Protected module the level decides whether other modules
				// may use the member (mendixlabs/mxcli#1498).
				if c := exportLevelClause(attr.ExportLevel); c != "" {
					constraints.WriteString(" " + c)
				}

				line.WriteString(fmt.Sprintf("  %s: %s%s", attr.Name, typeStr, constraints.String()))
				attrLines = append(attrLines, attrLine{text: line.String()})
			}

			// Append system attributes as pseudo-typed entries
			if entity.HasOwner {
				attrLines = append(attrLines, attrLine{text: "  Owner: AutoOwner"})
			}
			if entity.HasChangedBy {
				attrLines = append(attrLines, attrLine{text: "  ChangedBy: AutoChangedBy"})
			}
			if entity.HasCreatedDate {
				attrLines = append(attrLines, attrLine{text: "  CreatedDate: AutoCreatedDate"})
			}
			if entity.HasChangedDate {
				attrLines = append(attrLines, attrLine{text: "  ChangedDate: AutoChangedDate"})
			}

			// Output with commas
			for i, al := range attrLines {
				comma := ","
				if i == len(attrLines)-1 {
					comma = ""
				}
				fmt.Fprintf(ctx.Output, "%s%s\n", al.text, comma)
			}
			fmt.Fprint(ctx.Output, ")")
			// In a Protected module an entity's export level decides whether
			// other modules may use it (CE1406) and whether a Usable microflow
			// may expose it (CE5400). Before a view's `as (…)`, which ends the
			// statement's options (mendixlabs/mxcli#1498).
			if c := exportLevelClause(entity.ExportLevel); c != "" {
				fmt.Fprintf(ctx.Output, "\n%s", c)
			}

			// For VIEW entities, output the OQL query
			if entityType == "view" && entity.OqlQuery != "" {
				fmt.Fprint(ctx.Output, describedViewQuery(entity.OqlQuery))
			}

			// Build attribute name map
			attrNames := make(map[model.ID]string)
			for _, attr := range entity.Attributes {
				attrNames[attr.ID] = attr.Name
			}

			// Output indexes
			for _, idx := range entity.Indexes {
				var cols []string
				unresolved := false
				for _, ia := range idx.Attributes {
					colName := indexSegmentName(ia, attrNames)
					if colName == "" {
						unresolved = true
						break
					}
					if !ia.Ascending {
						colName += " desc"
					}
					cols = append(cols, colName)
				}
				// A segment naming no attribute and no system member has no MDL
				// spelling; `index ( desc)` is refused by check (#1401), so say
				// it in a comment rather than print a statement that cannot run.
				if unresolved {
					fmt.Fprint(ctx.Output, "\n-- index skipped: a column points at no attribute of this entity\n")
					continue
				}
				if len(cols) > 0 {
					fmt.Fprintf(ctx.Output, "\nindex (%s)", strings.Join(cols, ", "))
				}
			}

			// Output event handlers
			for _, eh := range entity.EventHandlers {
				mfName := eh.MicroflowName
				if mfName == "" && eh.MicroflowID != "" {
					mfName = lookupMicroflowName(ctx, eh.MicroflowID)
				}
				if mfName == "" {
					continue
				}
				eventName := strings.ToLower(string(eh.Event))
				// Show parameter: ($currentObject) or ()
				paramStr := "()"
				if eh.PassEventObject {
					paramStr = "($currentObject)"
				}
				var options string
				// RAISE ERROR only applies to Before handlers (they return Boolean)
				if eh.RaiseErrorOnFalse && strings.EqualFold(string(eh.Moment), "Before") {
					options = " raise error"
				}
				fmt.Fprintf(ctx.Output, "\non %s %s call %s%s%s",
					strings.ToLower(string(eh.Moment)), eventName, mfName, paramStr, options)
			}

			fmt.Fprintln(ctx.Output, ";")

			// RegEx and Range rules are standalone statements, not attribute
			// constraints like Required/Unique above, so they follow the entity.
			outputEntityValidationRules(ctx, entity, name.Module, name.Name, attrNames)

			// Output access rule GRANT statements
			if withGrants {
				outputEntityAccessGrants(ctx, entity, name.Module, name.Name)
			}

			return nil
		}
	}

	return mdlerrors.NewNotFound("entity", name.String())
}

// describeEntityToString generates MDL source for an entity and returns it as a string.
func describeEntityToString(ctx *ExecContext, name ast.QualifiedName) (string, error) {
	var buf strings.Builder
	origOutput := ctx.Output
	ctx.Output = &buf
	err := describeEntity(ctx, name)
	ctx.Output = origOutput
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// extractAttrNameFromQualified extracts the attribute name from a qualified name.
// e.g., "DmTest.Cars.CarId" -> "CarId"
func extractAttrNameFromQualified(qualifiedName string) string {
	// Split by "." and return the last part
	parts := strings.Split(qualifiedName, ".")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return ""
}

// resolveMicroflowByName resolves a qualified microflow name to its ID.
// It checks both microflows created during this session and existing microflows in the project.
func resolveMicroflowByName(ctx *ExecContext, qualifiedName string) (model.ID, error) {
	parts := strings.Split(qualifiedName, ".")
	if len(parts) < 2 {
		return "", mdlerrors.NewValidationf("invalid microflow name: %s (expected Module.Name)", qualifiedName)
	}
	moduleName := parts[0]
	mfName := strings.Join(parts[1:], ".")

	// Check microflows created during this session
	if ctx.Cache != nil && ctx.Cache.createdMicroflows != nil {
		if info, ok := ctx.Cache.createdMicroflows[qualifiedName]; ok {
			return info.ID, nil
		}
	}

	// Search existing microflows
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return "", mdlerrors.NewBackend("list microflows", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, mf := range allMicroflows {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == moduleName && mf.Name == mfName {
			return mf.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("microflow", qualifiedName)
}

// lookupMicroflowName reverse-looks up a microflow ID to its qualified name.
func lookupMicroflowName(ctx *ExecContext, mfID model.ID) string {
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return ""
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return ""
	}

	for _, mf := range allMicroflows {
		if mf.ID == mfID {
			modID := h.FindModuleID(mf.ContainerID)
			modName := h.GetModuleName(modID)
			if modName != "" {
				return modName + "." + mf.Name
			}
			return mf.Name
		}
	}
	return ""
}

// --- Executor method wrappers for callers not yet migrated ---
