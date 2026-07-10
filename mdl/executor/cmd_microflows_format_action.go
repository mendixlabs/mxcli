// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow activity and action formatting as MDL statements.
package executor

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	mdltypes "github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

// formatActivity formats a single microflow activity as an MDL statement.

// escapeExpressionValue escapes raw control characters inside string literals
// of a Mendix expression value so it can be safely embedded in MDL output.
// The lexer's STRING_LITERAL rule forbids raw \r and \n inside single-quoted
// strings. Only characters inside '...' regions are escaped; characters
// outside string literals (structural whitespace) are preserved as-is.
func escapeExpressionValue(v string) string {
	if !strings.ContainsAny(v, "\n\r\t") {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 32)
	inString := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\'' {
			// Check for escaped quote ('') inside string
			if inString && i+1 < len(v) && v[i+1] == '\'' {
				b.WriteString("''")
				i++
				continue
			}
			inString = !inString
			b.WriteByte(c)
			continue
		}
		if inString {
			switch c {
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteByte(c)
			}
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}
func formatActivity(
	ctx *ExecContext,
	obj microflows.MicroflowObject,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) string {
	switch activity := obj.(type) {
	case *microflows.StartEvent:
		return "" // Skip start events

	case *microflows.EndEvent:
		if activity.ReturnValue != "" {
			returnVal := strings.TrimSuffix(activity.ReturnValue, "\n")
			// Only add $ prefix for bare identifiers (no operators, quotes, or parens)
			if !strings.HasPrefix(returnVal, "$") && !isMendixKeyword(returnVal) && !isQualifiedEnumLiteral(returnVal) &&
				!strings.ContainsAny(returnVal, "+'\"()") && !isNumericLiteral(returnVal) {
				returnVal = "$" + returnVal
			}
			return fmt.Sprintf("return %s;", returnVal)
		}
		if ctx != nil && ctx.DescribingMicroflowHasReturnValue {
			return ""
		}
		// Without render context, default to the void-flow form.
		return "return;"

	case *microflows.ActionActivity:
		return formatAction(ctx, activity.Action, entityNames, microflowNames)

	case *microflows.ExclusiveSplit:
		condition := formatSplitCondition(activity.SplitCondition)
		return fmt.Sprintf("if %s then", condition)

	case *microflows.InheritanceSplit:
		varName := activity.VariableName
		if !strings.HasPrefix(varName, "$") {
			varName = "$" + varName
		}
		return fmt.Sprintf("split type %s;", varName)

	case *microflows.ExclusiveMerge:
		return "end if;"

	case *microflows.LoopedActivity:
		switch ls := activity.LoopSource.(type) {
		case *microflows.WhileLoopCondition:
			return fmt.Sprintf("while %s", ls.WhileExpression)
		case *microflows.IterableList:
			iterVar := "Item"
			listVar := "List"
			if ls.VariableName != "" {
				iterVar = ls.VariableName
			}
			if ls.ListVariableName != "" {
				listVar = ls.ListVariableName
			}
			return fmt.Sprintf("loop $%s in $%s", iterVar, listVar)
		default:
			return "loop $Item in $List"
		}

	case *microflows.BreakEvent:
		return "break;"

	case *microflows.ContinueEvent:
		return "continue;"

	case *microflows.ErrorEvent:
		return "raise error;"

	case *microflows.Annotation:
		return "" // Annotations are emitted separately via @annotation

	default:
		return fmt.Sprintf("-- Unknown activity type: %T", obj)
	}
}

// formatAction formats a microflow action as an MDL statement.
func formatAction(
	ctx *ExecContext,
	action microflows.MicroflowAction,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) string {
	if action == nil {
		return "-- Empty action"
	}

	switch a := action.(type) {
	case *microflows.CastAction:
		outputVar := a.OutputVariable
		if outputVar != "" && !strings.HasPrefix(outputVar, "$") {
			outputVar = "$" + outputVar
		}
		objectVar := a.ObjectVariable
		if objectVar != "" && !strings.HasPrefix(objectVar, "$") {
			objectVar = "$" + objectVar
		}
		if objectVar == "" {
			return fmt.Sprintf("cast %s;", outputVar)
		}
		if outputVar == "" {
			return fmt.Sprintf("cast %s;", objectVar)
		}
		return fmt.Sprintf("%s = cast %s;", outputVar, objectVar)

	case *microflows.CreateVariableAction:
		varType := "Object"
		if a.DataType != nil {
			varType = formatMicroflowDataType(ctx, a.DataType, entityNames)
		}
		initialValue := strings.TrimSuffix(a.InitialValue, "\n")
		if initialValue == "" {
			initialValue = "empty"
		}
		return fmt.Sprintf("declare $%s %s = %s;", a.VariableName, varType, initialValue)

	case *microflows.ChangeVariableAction:
		varName := a.VariableName
		// Check if this is an XPath attribute access (contains /)
		if strings.Contains(varName, "/") {
			// XPath like "$Product/Price" or "$Order/Module.Assoc/Attr"
			// This is actually changing an entity attribute, display as CHANGE statement
			parts := strings.SplitN(varName, "/", 2)
			objectName := parts[0]
			attrPath := parts[1]
			// Extract just the attribute name (last part after any association navigation)
			attrParts := strings.Split(attrPath, "/")
			attrName := attrParts[len(attrParts)-1]
			// objectName might already have $ prefix
			if !strings.HasPrefix(objectName, "$") {
				objectName = "$" + objectName
			}
			return fmt.Sprintf("change %s (%s = %s);", objectName, attrName, a.Value)
		}
		// Simple variable change
		if strings.HasPrefix(varName, "$") {
			return fmt.Sprintf("set %s = %s;", varName, a.Value)
		}
		return fmt.Sprintf("set $%s = %s;", varName, a.Value)

	case *microflows.CreateObjectAction:
		// Use EntityQualifiedName (BY_NAME_REFERENCE) or fall back to EntityID lookup
		entityName := a.EntityQualifiedName
		if entityName == "" {
			entityName = entityNames[a.EntityID]
		}
		if entityName == "" {
			entityName = "Entity"
		}
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "NewObject"
		}

		if len(a.InitialMembers) > 0 {
			var members []string
			// entityModule is the module of the create target (e.g. "TargetModule"
			// for `create TargetModule.Entity`). Associations in other modules
			// must keep their module prefix so the re-exec resolver finds them.
			entityModule := ""
			if parts := strings.SplitN(entityName, ".", 2); len(parts) == 2 {
				entityModule = parts[0]
			}
			for _, m := range a.InitialMembers {
				var memberName string
				// Check if this is an association change or an attribute change
				if m.AssociationQualifiedName != "" {
					// Association: keep the module prefix when the association
					// is defined in a different module than the create target.
					// The authored form `Module.Assoc` is preserved so re-exec
					// resolves against the association's owning module rather
					// than defaulting to the create target's module, which
					// would cause Studio Pro CE1613 "attribute no longer exists"
					// at re-open.
					memberName = m.AssociationQualifiedName
					if parts := strings.SplitN(memberName, ".", 2); len(parts) == 2 {
						if parts[0] == entityModule {
							memberName = parts[1]
						}
					}
				} else {
					// Attribute: extract just the attribute name
					memberName = m.AttributeQualifiedName
					if memberName == "" {
						memberName = string(m.AttributeID)
					}
					// Extract just the attribute name from qualified name (Module.Entity.Attr -> Attr)
					if parts := strings.Split(memberName, "."); len(parts) > 0 {
						memberName = parts[len(parts)-1]
					}
				}
				members = append(members, fmt.Sprintf("%s = %s", memberName, escapeExpressionValue(m.Value)))
			}
			return fmt.Sprintf("$%s = create %s (%s);", outputVar, entityName, strings.Join(members, ", "))
		}
		return fmt.Sprintf("$%s = create %s;", outputVar, entityName)

	case *microflows.ChangeObjectAction:
		varName := a.ChangeVariable
		if varName == "" {
			varName = "Object"
		}
		if len(a.Changes) > 0 {
			var members []string
			for _, m := range a.Changes {
				var memberName string
				// Check if this is an association change or an attribute change
				if m.AssociationQualifiedName != "" {
					// Association: use fully qualified name (Module.AssociationName)
					memberName = m.AssociationQualifiedName
				} else {
					// Attribute: extract just the attribute name
					memberName = m.AttributeQualifiedName
					if memberName == "" {
						memberName = string(m.AttributeID)
					}
					// Extract just the attribute name from qualified name (Module.Entity.Attr -> Attr)
					if parts := strings.Split(memberName, "."); len(parts) > 0 {
						memberName = parts[len(parts)-1]
					}
				}
				members = append(members, fmt.Sprintf("%s = %s", memberName, escapeExpressionValue(m.Value)))
			}
			if a.RefreshInClient {
				return fmt.Sprintf("change $%s (%s) refresh;", varName, strings.Join(members, ", "))
			}
			return fmt.Sprintf("change $%s (%s);", varName, strings.Join(members, ", "))
		}
		if a.RefreshInClient {
			return fmt.Sprintf("change $%s refresh;", varName)
		}
		return fmt.Sprintf("change $%s;", varName)

	case *microflows.CommitObjectsAction:
		varName := a.CommitVariable
		if varName == "" {
			varName = "Object"
		}
		suffix := ""
		if a.WithEvents {
			suffix += " with events"
		}
		if a.RefreshInClient {
			suffix += " refresh"
		}
		return fmt.Sprintf("commit $%s%s;", varName, suffix)

	case *microflows.DeleteObjectAction:
		return fmt.Sprintf("delete $%s;", a.DeleteVariable)

	case *microflows.RollbackObjectAction:
		if a.RefreshInClient {
			return fmt.Sprintf("rollback $%s refresh;", a.RollbackVariable)
		}
		return fmt.Sprintf("rollback $%s;", a.RollbackVariable)

	case *microflows.CreateListAction:
		// Use EntityQualifiedName (BY_NAME_REFERENCE) or fall back to EntityID lookup
		entityName := a.EntityQualifiedName
		if entityName == "" {
			entityName = entityNames[a.EntityID]
		}
		if entityName == "" {
			entityName = "Entity"
		}
		return fmt.Sprintf("$%s = create list of %s;", a.OutputVariable, entityName)

	case *microflows.ChangeListAction:
		varName := a.ChangeVariable
		switch a.Type {
		case microflows.ChangeListTypeAdd:
			return fmt.Sprintf("add %s to $%s;", a.Value, varName)
		case microflows.ChangeListTypeRemove:
			return fmt.Sprintf("remove %s from $%s;", a.Value, varName)
		case microflows.ChangeListTypeClear:
			return fmt.Sprintf("clear $%s;", varName)
		case microflows.ChangeListTypeSet:
			return fmt.Sprintf("set $%s = %s;", varName, a.Value)
		default:
			return fmt.Sprintf("change list $%s (%s);", varName, a.Type)
		}

	case *microflows.ListOperationAction:
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "Result"
		}
		return formatListOperation(ctx, a.Operation, outputVar)

	case *microflows.AggregateListAction:
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "Result"
		}
		fn := string(a.Function)
		if fn == "" {
			fn = "count"
		}
		// Extract attribute name (use last part of qualified name for readability)
		attrName := a.AttributeQualifiedName
		if attrName != "" {
			// Qualified name is like "Module.Entity.Attribute" - extract just attribute name
			parts := strings.Split(attrName, ".")
			if len(parts) > 0 {
				attrName = parts[len(parts)-1]
			}
		}
		// Expression-based aggregate: SUM($list, $currentObject/Attr + 1)
		if a.UseExpression && a.Expression != "" {
			return fmt.Sprintf("$%s = %s($%s, %s);", outputVar, strings.ToLower(fn), a.InputVariable, a.Expression)
		}
		// Attribute-based aggregate: SUM($list.Attr)
		if attrName != "" && a.Function != microflows.AggregateFunctionCount {
			return fmt.Sprintf("$%s = %s($%s.%s);", outputVar, strings.ToLower(fn), a.InputVariable, attrName)
		}
		return fmt.Sprintf("$%s = %s($%s);", outputVar, strings.ToLower(fn), a.InputVariable)

	case *microflows.RetrieveAction:
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "Result"
		}

		if dbSource, ok := a.Source.(*microflows.DatabaseRetrieveSource); ok {
			// Try EntityID lookup first, fall back to EntityQualifiedName
			entityName := entityNames[dbSource.EntityID]
			if entityName == "" && dbSource.EntityQualifiedName != "" {
				entityName = dbSource.EntityQualifiedName
			}
			if entityName == "" {
				entityName = "Entity"
			}

			// A DatabaseRetrieveSource always renders in database form
			// (`from Entity where …`). It must NOT be collapsed to the
			// association shorthand `from $var/Assoc` even when its XPath is a
			// reverse-association constraint — that shorthand is the in-memory
			// retrieve-by-association form, and rendering a database retrieve
			// that way makes the two indistinguishable and breaks round-trip
			// (issue #726).
			stmt := fmt.Sprintf("retrieve $%s from %s", outputVar, entityName)

			if dbSource.XPathConstraint != "" {
				constraint := strings.TrimSpace(dbSource.XPathConstraint)
				// Enrich string literals for enum attributes to qualified names
				// (e.g. Status = 'Open' → Status = Module.OrderStatus.Open) when
				// the entity is known and we are connected to a project.
				constraint = enrichXPathConstraintForDescribe(ctx, entityName, constraint)
				// XPath may contain multiple predicates like [a][b] or [a]\n[b].
				// Split them and join with MDL 'and' so the parser sees
				// separate xpathConstraint nodes.
				if strings.HasPrefix(constraint, "[") && strings.HasSuffix(constraint, "]") {
					// Split on "][" boundary (possibly separated by \n literals),
					// then re-wrap each predicate.
					inner := constraint[1 : len(constraint)-1]
					// Normalise real newlines between predicates: ]\n[ → ][
					inner = strings.ReplaceAll(inner, "]\n[", "][")
					parts := strings.Split(inner, "][")
					if len(parts) > 1 {
						var wrapped []string
						for _, p := range parts {
							wrapped = append(wrapped, "["+strings.TrimSpace(p)+"]")
						}
						constraint = strings.Join(wrapped, "\n    ")
					} else {
						constraint = parts[0]
					}
				}
				stmt += fmt.Sprintf("\n    where %s", constraint)
			}

			// Output SORT BY clause if present
			if len(dbSource.Sorting) > 0 {
				var sortParts []string
				for _, sortItem := range dbSource.Sorting {
					attrName := sortItem.AttributeQualifiedName
					order := "asc"
					if sortItem.Direction == microflows.SortDirectionDescending {
						order = "desc"
					}
					sortParts = append(sortParts, attrName+" "+order)
				}
				stmt += fmt.Sprintf("\n    sort by %s", strings.Join(sortParts, ", "))
			}

			if dbSource.Range != nil {
				switch dbSource.Range.RangeType {
				case microflows.RangeTypeFirst:
					stmt += "\n    limit 1"
				case microflows.RangeTypeCustom:
					if dbSource.Range.Limit != "" {
						stmt += fmt.Sprintf("\n    limit %s", dbSource.Range.Limit)
					}
					if dbSource.Range.Offset != "" {
						stmt += fmt.Sprintf("\n    offset %s", dbSource.Range.Offset)
					}
				}
			}

			return stmt + ";"
		}

		if assocSource, ok := a.Source.(*microflows.AssociationRetrieveSource); ok {
			startVar := assocSource.StartVariable
			if startVar == "" {
				startVar = "Object"
			}
			// Use AssociationQualifiedName (BY_NAME_REFERENCE) if available
			assocName := assocSource.AssociationQualifiedName
			if assocName == "" {
				assocName = "..."
			}
			return fmt.Sprintf("retrieve $%s from $%s/%s;", outputVar, startVar, assocName)
		}

		return fmt.Sprintf("retrieve $%s from ...;", outputVar)

	case *microflows.LogMessageAction:
		level := string(a.LogLevel)
		if level == "" {
			level = "info"
		}
		// Node is an expression in Mendix (e.g., 'TEST' or $variable or 'Prefix' + $var)
		// Output it as-is since it's already stored as an expression
		node := a.LogNodeName
		if node == "" {
			node = defaultLogNodeExpression
		}
		message := "'Message'"
		if a.MessageTemplate != nil && len(a.MessageTemplate.Translations) > 0 {
			// Get message text from template (prefer en_US, fallback to any)
			for _, text := range a.MessageTemplate.Translations {
				message = text
				break
			}
			if text, ok := a.MessageTemplate.Translations["en_US"]; ok {
				message = text
			}
			message = mdlQuote(message)
		}

		// Build WITH clause if there are template parameters
		withClause := ""
		if len(a.TemplateParameters) > 0 {
			var params []string
			for i, expr := range a.TemplateParameters {
				params = append(params, fmt.Sprintf("{%d} = %s", i+1, expr))
			}
			withClause = fmt.Sprintf(" with (%s)", strings.Join(params, ", "))
		}

		return fmt.Sprintf("log %s node %s %s%s;", strings.ToLower(level), node, message, withClause)

	case *microflows.MicroflowCallAction:
		mfName := ""
		if a.MicroflowCall != nil && a.MicroflowCall.Microflow != "" {
			mfName = a.MicroflowCall.Microflow
		} else {
			mfName = "Microflow"
		}

		var params []string
		if a.MicroflowCall != nil {
			for _, pm := range a.MicroflowCall.ParameterMappings {
				// Extract just the parameter name from qualified name (Module.Microflow.Param -> Param)
				paramName := pm.Parameter
				if idx := strings.LastIndex(paramName, "."); idx != -1 {
					paramName = paramName[idx+1:]
				}
				params = append(params, fmt.Sprintf("%s = %s", paramName, pm.Argument))
			}
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.UseReturnVariable && a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = call microflow %s(%s);", a.ResultVariableName, mfName, paramStr)
		}
		return fmt.Sprintf("call microflow %s(%s);", mfName, paramStr)

	case *microflows.NanoflowCallAction:
		nfName := ""
		if a.NanoflowCall != nil && a.NanoflowCall.Nanoflow != "" {
			nfName = a.NanoflowCall.Nanoflow
		} else {
			nfName = "Nanoflow"
		}

		var params []string
		if a.NanoflowCall != nil {
			for _, pm := range a.NanoflowCall.ParameterMappings {
				paramName := pm.Parameter
				if idx := strings.LastIndex(paramName, "."); idx != -1 {
					paramName = paramName[idx+1:]
				}
				params = append(params, fmt.Sprintf("%s = %s", paramName, pm.Argument))
			}
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.UseReturnVariable && a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = call nanoflow %s(%s);", a.OutputVariableName, nfName, paramStr)
		}
		return fmt.Sprintf("call nanoflow %s(%s);", nfName, paramStr)

	case *microflows.JavaActionCallAction:
		javaActionName := a.JavaAction
		if javaActionName == "" {
			javaActionName = "JavaAction"
		}

		var params []string
		for _, pm := range a.ParameterMappings {
			// Extract just the parameter name from qualified name
			paramName := pm.Parameter
			if idx := strings.LastIndex(paramName, "."); idx != -1 {
				paramName = paramName[idx+1:]
			}
			// Get the value based on parameter value type
			valueStr := ""
			switch v := pm.Value.(type) {
			case *microflows.StringTemplateParameterValue:
				if v.TypedTemplate != nil {
					valueStr = mdlQuote(v.TypedTemplate.Text)
				}
			case *microflows.ExpressionBasedCodeActionParameterValue:
				valueStr = v.Expression
			case *microflows.BasicCodeActionParameterValue:
				if v.Argument == "" {
					valueStr = "empty"
				} else {
					valueStr = v.Argument
				}
			case *microflows.MicroflowParameterValue:
				if v.Microflow != "" {
					valueStr = mdlQuote(v.Microflow)
				} else {
					valueStr = "empty"
				}
			case *microflows.EntityTypeCodeActionParameterValue:
				if v.Entity != "" {
					valueStr = mdlQuote(v.Entity)
				}
			}
			if valueStr != "" {
				params = append(params, fmt.Sprintf("%s = %s", paramName, valueStr))
			}
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.UseReturnVariable && a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = call java action %s(%s);", a.ResultVariableName, javaActionName, paramStr)
		}
		return fmt.Sprintf("call java action %s(%s);", javaActionName, paramStr)

	case *microflows.CallExternalAction:
		serviceName := a.ConsumedODataService
		if serviceName == "" {
			serviceName = "ODataService"
		}
		actionName := a.Name
		if actionName == "" {
			actionName = "Action"
		}

		var params []string
		for _, pm := range a.ParameterMappings {
			params = append(params, fmt.Sprintf("%s = %s", pm.ParameterName, pm.Argument))
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.UseReturnVariable && a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = call external action %s.%s(%s);", a.ResultVariableName, serviceName, actionName, paramStr)
		}
		return fmt.Sprintf("call external action %s.%s(%s);", serviceName, actionName, paramStr)

	case *microflows.ShowPageAction:
		// Get page name from action (PageName is BY_NAME_REFERENCE, PageID is legacy BY_ID_REFERENCE)
		pageName := a.PageName
		if pageName == "" && a.PageID != "" && ctx.Connected() {
			// Fall back to looking up by ID (legacy format)
			pages, _ := ctx.Backend.ListPages()
			for _, p := range pages {
				if p.ID == a.PageID {
					h, _ := getHierarchy(ctx)
					if h != nil {
						pageName = h.GetQualifiedName(p.ContainerID, p.Name)
					}
					break
				}
			}
		}
		if pageName == "" {
			pageName = "UnknownPage"
		}

		// Build parameter list
		var params []string
		for _, pm := range a.PageParameterMappings {
			// Extract just the parameter name from the qualified name
			parts := strings.Split(pm.Parameter, ".")
			paramName := parts[len(parts)-1]
			params = append(params, fmt.Sprintf("$%s = %s", paramName, pm.Argument))
		}

		// Build the statement
		paramStr := ""
		if len(params) > 0 {
			paramStr = "(" + strings.Join(params, ", ") + ")"
		}
		return fmt.Sprintf("show page %s%s;", pageName, paramStr)

	case *microflows.ClosePageAction:
		if a.NumberOfPages > 1 {
			return fmt.Sprintf("close page %d;", a.NumberOfPages)
		}
		return "close page;"

	case *microflows.ShowHomePageAction:
		return "show home page;"

	case *microflows.ShowMessageAction:
		msgType := string(a.Type)
		if msgType == "" {
			msgType = "Information"
		}
		message := "'...'"
		if a.Template != nil && len(a.Template.Translations) > 0 {
			// Get message text from template (prefer en_US, fallback to any)
			for _, text := range a.Template.Translations {
				message = text
				break
			}
			if text, ok := a.Template.Translations["en_US"]; ok {
				message = text
			}
			message = mdlQuote(message)
		}
		result := fmt.Sprintf("show message %s type %s", message, msgType)
		if len(a.TemplateParameters) > 0 {
			result += " objects [" + strings.Join(a.TemplateParameters, ", ") + "]"
		}
		return result + ";"

	case *microflows.DownloadFileAction:
		fileDocument := a.FileDocument
		if fileDocument != "" && !strings.HasPrefix(fileDocument, "$") {
			fileDocument = "$" + fileDocument
		}
		result := fmt.Sprintf("download file %s", fileDocument)
		if a.ShowInBrowser {
			result += " show in browser"
		}
		return result + ";"

	case *microflows.ValidationFeedbackAction:
		// Get the message text from template translations (prefer en_US, fallback to any)
		msgText := "'...'"
		if a.Template != nil && len(a.Template.Translations) > 0 {
			for _, text := range a.Template.Translations {
				msgText = text
				break
			}
			if text, ok := a.Template.Translations["en_US"]; ok {
				msgText = text
			}
			msgText = mdlQuote(msgText)
		}
		// Build attribute path from variable and attribute name
		// AttributeName format: Module.Entity.Attribute
		// Add $ prefix to variable name if not present
		varName := a.ObjectVariable
		if !strings.HasPrefix(varName, "$") {
			varName = "$" + varName
		}
		attrPath := varName
		if a.AttributeName != "" {
			// Extract just the attribute part from "Module.Entity.Attribute"
			parts := strings.Split(a.AttributeName, ".")
			if len(parts) >= 3 {
				attrPath = varName + "/" + parts[len(parts)-1]
			}
		} else if a.AssociationName != "" {
			attrPath = varName + "/" + a.AssociationName
		}
		return fmt.Sprintf("validation feedback %s message %s;", attrPath, msgText)

	case *microflows.RestCallAction:
		return formatRestCallAction(ctx, a)

	case *microflows.RestOperationCallAction:
		return formatRestOperationCallAction(ctx, a)

	case *microflows.ExecuteDatabaseQueryAction:
		return formatExecuteDatabaseQueryAction(ctx, a)

	case *microflows.ImportXmlAction:
		return formatImportXmlAction(ctx, a, entityNames)

	case *microflows.ExportXmlAction:
		return formatExportXmlAction(ctx, a)

	case *microflows.TransformJsonAction:
		return formatTransformJsonAction(a)

	// Workflow microflow actions
	case *microflows.GetWorkflowDataAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = get workflow data $%s as %s;", a.OutputVariableName, a.WorkflowVariable, a.Workflow)
		}
		return fmt.Sprintf("get workflow data $%s as %s;", a.WorkflowVariable, a.Workflow)

	case *microflows.WorkflowCallAction:
		if a.UseReturnVariable && a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = call workflow %s ($%s);", a.OutputVariableName, a.Workflow, a.WorkflowContextVariable)
		}
		return fmt.Sprintf("call workflow %s ($%s);", a.Workflow, a.WorkflowContextVariable)

	case *microflows.GetWorkflowsAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = get workflows for $%s;", a.OutputVariableName, a.WorkflowContextVariableName)
		}
		return fmt.Sprintf("get workflows for $%s;", a.WorkflowContextVariableName)

	case *microflows.GetWorkflowActivityRecordsAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = get workflow activity records $%s;", a.OutputVariableName, a.WorkflowVariable)
		}
		return fmt.Sprintf("get workflow activity records $%s;", a.WorkflowVariable)

	case *microflows.WorkflowOperationAction:
		return formatWorkflowOperationAction(ctx, a)

	case *microflows.SetTaskOutcomeAction:
		return fmt.Sprintf("set task outcome $%s %s;", a.WorkflowTaskVariable, mdlQuote(a.OutcomeValue))

	case *microflows.OpenUserTaskAction:
		return fmt.Sprintf("open user task $%s;", a.UserTaskVariable)

	case *microflows.NotifyWorkflowAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = notify workflow $%s;", a.OutputVariableName, a.WorkflowVariable)
		}
		return fmt.Sprintf("notify workflow $%s;", a.WorkflowVariable)

	case *microflows.OpenWorkflowAction:
		return fmt.Sprintf("open workflow $%s;", a.WorkflowVariable)

	case *microflows.LockWorkflowAction:
		if a.PauseAllWorkflows {
			return "lock workflow all;"
		}
		if a.Workflow != "" {
			return fmt.Sprintf("lock workflow %s;", a.Workflow)
		}
		return fmt.Sprintf("lock workflow $%s;", a.WorkflowVariable)

	case *microflows.UnlockWorkflowAction:
		if a.ResumeAllPausedWorkflows {
			return "unlock workflow all;"
		}
		if a.Workflow != "" {
			return fmt.Sprintf("unlock workflow %s;", a.Workflow)
		}
		return fmt.Sprintf("unlock workflow $%s;", a.WorkflowVariable)

	case *microflows.JavaScriptActionCallAction:
		jsActionName := a.JavaScriptAction
		if jsActionName == "" {
			if n := len(a.ParameterMappings); n > 0 {
				label := "params"
				if n == 1 {
					label = "param"
				}
				return fmt.Sprintf("-- JavaScriptAction: missing action reference (%d %s)", n, label)
			}
			return "-- JavaScriptAction: missing action reference"
		}

		var params []string
		for _, pm := range a.ParameterMappings {
			paramName := pm.Parameter
			if idx := strings.LastIndex(paramName, "."); idx != -1 {
				paramName = paramName[idx+1:]
			}
			valueStr := ""
			if pm.Value != nil {
				switch v := pm.Value.(type) {
				case *microflows.StringTemplateParameterValue:
					if v.TypedTemplate != nil {
						valueStr = mdlQuote(v.TypedTemplate.Text)
					}
				case *microflows.ExpressionBasedCodeActionParameterValue:
					valueStr = v.Expression
				case *microflows.BasicCodeActionParameterValue:
					valueStr = v.Argument
				case *microflows.EntityTypeCodeActionParameterValue:
					valueStr = v.Entity
				}
			}
			if valueStr != "" {
				params = append(params, fmt.Sprintf("%s = %s", paramName, valueStr))
			}
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.UseReturnVariable && a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = call javascript action %s(%s);", a.OutputVariableName, jsActionName, paramStr)
		}
		return fmt.Sprintf("call javascript action %s(%s);", jsActionName, paramStr)

	case *microflows.WebServiceCallAction:
		return formatWebServiceCallAction(ctx, a)

	case *microflows.UnknownAction:
		return fmt.Sprintf("-- Unsupported action type: %s", a.TypeName)

	default:
		return fmt.Sprintf("-- Unknown action: %T", action)
	}
}

// formatWorkflowOperationAction formats a workflow operation action as MDL.
func formatWorkflowOperationAction(ctx *ExecContext, a *microflows.WorkflowOperationAction) string {
	if a.Operation == nil {
		return "workflow operation ...;"
	}
	switch op := a.Operation.(type) {
	case *microflows.AbortOperation:
		if op.Reason != "" {
			return fmt.Sprintf("workflow operation abort $%s reason %s;", op.WorkflowVariable, mdlQuote(op.Reason))
		}
		return fmt.Sprintf("workflow operation abort $%s;", op.WorkflowVariable)
	case *microflows.ContinueOperation:
		return fmt.Sprintf("workflow operation continue $%s;", op.WorkflowVariable)
	case *microflows.PauseOperation:
		return fmt.Sprintf("workflow operation pause $%s;", op.WorkflowVariable)
	case *microflows.RestartOperation:
		return fmt.Sprintf("workflow operation restart $%s;", op.WorkflowVariable)
	case *microflows.RetryOperation:
		return fmt.Sprintf("workflow operation retry $%s;", op.WorkflowVariable)
	case *microflows.UnpauseOperation:
		return fmt.Sprintf("workflow operation unpause $%s;", op.WorkflowVariable)
	default:
		return fmt.Sprintf("-- Unknown workflow operation: %T", a.Operation)
	}
}

func formatWebServiceCallAction(ctx *ExecContext, a *microflows.WebServiceCallAction) string {
	prefix := ""
	if a.OutputVariable != "" {
		prefix = fmt.Sprintf("$%s = ", a.OutputVariable)
	}
	if len(a.RawBSON) > 0 {
		raw := base64.StdEncoding.EncodeToString(canonicalRawBSON(a.RawBSON))
		return prefix + "call web service raw " + mdlQuote(raw) + ";"
	}

	parts := []string{prefix + "call web service " + formatWebServiceReference(resolveWebServiceReference(ctx, a.ServiceID))}
	if a.OperationName != "" {
		parts = append(parts, "operation "+formatWebServiceReference(a.OperationName))
	}
	if a.SendMappingID != "" {
		parts = append(parts, "send mapping "+formatWebServiceReference(resolveWebServiceMappingReference(ctx, a.SendMappingID, true)))
	}
	if a.ReceiveMappingID != "" {
		parts = append(parts, "receive mapping "+formatWebServiceReference(resolveWebServiceMappingReference(ctx, a.ReceiveMappingID, false)))
	}
	if a.TimeoutExpression != "" {
		parts = append(parts, "timeout "+strings.TrimRight(a.TimeoutExpression, " \t\n\r"))
	}
	return strings.Join(parts, "\n") + ";"
}

func formatWebServiceReference(ref string) string {
	if isBareQualifiedReference(ref) {
		return ref
	}
	return mdlQuote(ref)
}

func isBareQualifiedReference(ref string) bool {
	if ref == "" {
		return false
	}
	for _, part := range strings.Split(ref, ".") {
		if !isBareIdentifier(part) {
			return false
		}
	}
	return true
}

func isBareIdentifier(part string) bool {
	if part == "" {
		return false
	}
	for i, r := range part {
		if i == 0 {
			if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return false
			}
			continue
		}
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func resolveWebServiceReference(ctx *ExecContext, id model.ID) string {
	raw := string(id)
	if raw == "" || ctx == nil || ctx.Backend == nil {
		return raw
	}
	units, err := ctx.Backend.ListRawUnitsByType("WebServices$ImportedWebService")
	if err != nil {
		return raw
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return raw
	}
	for _, unit := range units {
		if unit == nil || unit.ID != id {
			continue
		}
		return qualifiedRawUnitName(h, unit, raw)
	}
	return raw
}

func qualifiedRawUnitName(h *ContainerHierarchy, unit *mdltypes.RawUnit, fallback string) string {
	name := rawUnitName(unit.Contents)
	if name == "" {
		return fallback
	}
	if h == nil {
		return name
	}
	if qn := h.GetQualifiedName(unit.ContainerID, name); qn != "." && qn != "" {
		return qn
	}
	return name
}

func resolveWebServiceMappingReference(ctx *ExecContext, id model.ID, preferExport bool) string {
	if preferExport {
		if qn := resolveExportMappingReference(ctx, id); qn != "" {
			return qn
		}
		if qn := resolveImportMappingReference(ctx, id); qn != "" {
			return qn
		}
	} else {
		if qn := resolveImportMappingReference(ctx, id); qn != "" {
			return qn
		}
		if qn := resolveExportMappingReference(ctx, id); qn != "" {
			return qn
		}
	}
	return string(id)
}

func resolveImportMappingReference(ctx *ExecContext, id model.ID) string {
	if id == "" || ctx == nil || ctx.Backend == nil {
		return ""
	}
	mappings, err := ctx.Backend.ListImportMappings()
	if err != nil {
		return ""
	}
	for _, mapping := range mappings {
		if mapping != nil && mapping.ID == id {
			return qualifiedNameForContainer(ctx, mapping.ContainerID, mapping.Name)
		}
	}
	return ""
}

func resolveExportMappingReference(ctx *ExecContext, id model.ID) string {
	if id == "" || ctx == nil || ctx.Backend == nil {
		return ""
	}
	mappings, err := ctx.Backend.ListExportMappings()
	if err != nil {
		return ""
	}
	for _, mapping := range mappings {
		if mapping != nil && mapping.ID == id {
			return qualifiedNameForContainer(ctx, mapping.ContainerID, mapping.Name)
		}
	}
	return ""
}

func qualifiedNameForContainer(ctx *ExecContext, containerID model.ID, name string) string {
	if name == "" {
		return ""
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return name
	}
	if qn := h.GetQualifiedName(containerID, name); qn != "." && qn != "" {
		return qn
	}
	return name
}

func rawUnitName(contents []byte) string {
	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return ""
	}
	name, _ := raw["Name"].(string)
	return name
}

// formatListOperation formats a list operation as MDL.
func formatListOperation(ctx *ExecContext, op microflows.ListOperation, outputVar string) string {
	if op == nil {
		return fmt.Sprintf("$%s = list operation ...;", outputVar)
	}

	switch o := op.(type) {
	case *microflows.HeadOperation:
		return fmt.Sprintf("$%s = head($%s);", outputVar, o.ListVariable)
	case *microflows.TailOperation:
		return fmt.Sprintf("$%s = tail($%s);", outputVar, o.ListVariable)
	case *microflows.FindOperation:
		return fmt.Sprintf("$%s = find($%s, %s);", outputVar, o.ListVariable, o.Expression)
	case *microflows.FilterOperation:
		return fmt.Sprintf("$%s = filter($%s, %s);", outputVar, o.ListVariable, o.Expression)
	case *microflows.SortOperation:
		if len(o.Sorting) > 0 {
			var sortCols []string
			for _, s := range o.Sorting {
				dir := "asc"
				if s.Direction == microflows.SortDirectionDescending {
					dir = "desc"
				}
				// Extract attribute name (use last part of qualified name for readability)
				attrName := s.AttributeQualifiedName
				if attrName != "" {
					parts := strings.Split(attrName, ".")
					if len(parts) > 0 {
						attrName = parts[len(parts)-1]
					}
				}
				if attrName == "" {
					attrName = "..."
				}
				sortCols = append(sortCols, fmt.Sprintf("%s %s", mdlIdent(attrName), dir))
			}
			return fmt.Sprintf("$%s = sort($%s, %s);", outputVar, o.ListVariable, strings.Join(sortCols, ", "))
		}
		return fmt.Sprintf("$%s = sort($%s);", outputVar, o.ListVariable)
	case *microflows.UnionOperation:
		return fmt.Sprintf("$%s = union($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.IntersectOperation:
		return fmt.Sprintf("$%s = intersect($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.SubtractOperation:
		return fmt.Sprintf("$%s = subtract($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.ContainsOperation:
		return fmt.Sprintf("$%s = contains($%s, $%s);", outputVar, o.ListVariable, o.ObjectVariable)
	case *microflows.EqualsOperation:
		return fmt.Sprintf("$%s = equals($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.FindByAttributeOperation:
		fieldName := extractFieldName(o.Attribute, o.Association)
		if fieldName != "" && o.Expression != "" {
			return fmt.Sprintf("$%s = find($%s, %s = %s);", outputVar, o.ListVariable, fieldName, o.Expression)
		} else if o.Expression != "" {
			return fmt.Sprintf("$%s = find($%s, %s);", outputVar, o.ListVariable, o.Expression)
		}
		return fmt.Sprintf("-- $%s = find($%s) — missing attribute/expression", outputVar, o.ListVariable)
	case *microflows.FilterByAttributeOperation:
		fieldName := extractFieldName(o.Attribute, o.Association)
		if fieldName != "" && o.Expression != "" {
			return fmt.Sprintf("$%s = filter($%s, %s = %s);", outputVar, o.ListVariable, fieldName, o.Expression)
		} else if o.Expression != "" {
			return fmt.Sprintf("$%s = filter($%s, %s);", outputVar, o.ListVariable, o.Expression)
		}
		return fmt.Sprintf("-- $%s = filter($%s) — missing attribute/expression", outputVar, o.ListVariable)
	case *microflows.ListRangeOperation:
		if o.OffsetExpression != "" && o.LimitExpression != "" {
			return fmt.Sprintf("$%s = range($%s, %s, %s);", outputVar, o.ListVariable, o.OffsetExpression, o.LimitExpression)
		} else if o.OffsetExpression != "" {
			return fmt.Sprintf("$%s = range($%s, %s);", outputVar, o.ListVariable, o.OffsetExpression)
		} else if o.LimitExpression != "" {
			return fmt.Sprintf("$%s = range($%s, 0, %s);", outputVar, o.ListVariable, o.LimitExpression)
		}
		return fmt.Sprintf("$%s = range($%s);", outputVar, o.ListVariable)
	default:
		return fmt.Sprintf("$%s = list operation %T;", outputVar, op)
	}
}

// extractFieldName returns the short field name from a qualified attribute
// or association reference (e.g. "MyModule.Entity.Status" → "Status",
// "MyModule.Order_Customer" → "Order_Customer"). Returns the association
// name if attribute is empty.
func extractFieldName(attribute, association string) string {
	ref := attribute
	if ref == "" {
		ref = association
	}
	if ref == "" {
		return ""
	}
	parts := strings.Split(ref, ".")
	return parts[len(parts)-1]
}

// formatRestCallAction formats a REST call action as MDL.
func formatRestCallAction(ctx *ExecContext, a *microflows.RestCallAction) string {
	var sb strings.Builder

	// Output variable assignment (may be on RestCallAction or ResultHandling)
	outputVar := a.OutputVariable
	if outputVar == "" && a.ResultHandling != nil {
		switch rh := a.ResultHandling.(type) {
		case *microflows.ResultHandlingString:
			outputVar = rh.VariableName
		case *microflows.ResultHandlingHttpResponse:
			outputVar = rh.VariableName
		case *microflows.ResultHandlingMapping:
			outputVar = rh.ResultVariable
		}
	}
	if outputVar != "" {
		sb.WriteString("$")
		sb.WriteString(outputVar)
		sb.WriteString(" = ")
	}

	sb.WriteString("rest call ")

	// HTTP method
	method := "get"
	if a.HttpConfiguration != nil {
		switch a.HttpConfiguration.HttpMethod {
		case microflows.HttpMethodGet:
			method = "get"
		case microflows.HttpMethodPost:
			method = "post"
		case microflows.HttpMethodPut:
			method = "put"
		case microflows.HttpMethodPatch:
			method = "patch"
		case microflows.HttpMethodDelete:
			method = "delete"
		}
	}
	sb.WriteString(method)
	sb.WriteString(" ")

	// URL
	url := "''"
	if a.HttpConfiguration != nil && a.HttpConfiguration.LocationTemplate != "" {
		url = mdlQuote(a.HttpConfiguration.LocationTemplate)
	}
	sb.WriteString(url)

	// URL parameters
	if a.HttpConfiguration != nil && len(a.HttpConfiguration.LocationParams) > 0 {
		sb.WriteString(" with (")
		for i, param := range a.HttpConfiguration.LocationParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("{%d} = %s", i+1, param))
		}
		sb.WriteString(")")
	}

	// Headers
	if a.HttpConfiguration != nil && len(a.HttpConfiguration.CustomHeaders) > 0 {
		for _, h := range a.HttpConfiguration.CustomHeaders {
			sb.WriteString("\n    header ")
			sb.WriteString(mdlQuote(h.Name))
			sb.WriteString(" = ")
			sb.WriteString(h.Value)
		}
	}

	// Authentication
	if a.HttpConfiguration != nil && a.HttpConfiguration.UseAuthentication {
		sb.WriteString("\n    auth basic ")
		sb.WriteString(a.HttpConfiguration.Username)
		sb.WriteString(" password ")
		sb.WriteString(a.HttpConfiguration.Password)
	}

	// Body
	if a.RequestHandling != nil {
		switch rh := a.RequestHandling.(type) {
		case *microflows.CustomRequestHandling:
			if rh.Template != "" {
				sb.WriteString("\n    body ")
				sb.WriteString(mdlQuote(rh.Template))
				// Add template parameters if present
				if len(rh.TemplateParams) > 0 {
					sb.WriteString(" with (")
					for i, param := range rh.TemplateParams {
						if i > 0 {
							sb.WriteString(", ")
						}
						sb.WriteString(fmt.Sprintf("{%d} = %s", i+1, param))
					}
					sb.WriteString(")")
				}
			}
		case *microflows.MappingRequestHandling:
			if rh.MappingID != "" {
				sb.WriteString("\n    body mapping ")
				sb.WriteString(string(rh.MappingID))
				if rh.ParameterVariable != "" {
					sb.WriteString(" from $")
					sb.WriteString(rh.ParameterVariable)
				}
			}
		}
	}

	// Timeout
	if a.TimeoutExpression != "" {
		sb.WriteString("\n    timeout ")
		sb.WriteString(a.TimeoutExpression)
	}

	// Returns
	sb.WriteString("\n    returns ")
	if a.ResultHandling != nil {
		switch rh := a.ResultHandling.(type) {
		case *microflows.ResultHandlingString:
			sb.WriteString("String")
			_ = rh // used for type assertion only
		case *microflows.ResultHandlingHttpResponse:
			sb.WriteString("response")
		case *microflows.ResultHandlingMapping:
			sb.WriteString("mapping ")
			sb.WriteString(string(rh.MappingID))
			if rh.ResultEntityID != "" {
				// `as list of Entity` when the mapping yields a list,
				// otherwise `as Entity` for a single object. Studio Pro
				// keeps this on the ImportMappingCall (Range.SingleObject
				// + ForceSingleOccurrence); the parser collapses both into
				// SingleObject, so a list is `!SingleObject`.
				if rh.SingleObject {
					sb.WriteString(" as ")
				} else {
					sb.WriteString(" as list of ")
				}
				sb.WriteString(string(rh.ResultEntityID))
			}
		case *microflows.ResultHandlingNone:
			sb.WriteString("Nothing")
		default:
			sb.WriteString("String")
		}
	} else {
		sb.WriteString("String")
	}

	// Note: Error handling suffix is added at the activity level, not here
	sb.WriteString(";")
	return sb.String()
}

// formatRestOperationCallAction formats a RestOperationCallAction as MDL.
func formatRestOperationCallAction(ctx *ExecContext, a *microflows.RestOperationCallAction) string {
	var sb strings.Builder

	if a.OutputVariable != nil && a.OutputVariable.VariableName != "" {
		sb.WriteString("$")
		sb.WriteString(a.OutputVariable.VariableName)
		sb.WriteString(" = ")
	}

	sb.WriteString("send rest request ")
	sb.WriteString(a.Operation)

	// WITH clause for parameter mappings
	allParams := make([]struct{ name, value string }, 0)
	for _, pm := range a.ParameterMappings {
		// Strip operation prefix from parameter name
		name := pm.Parameter
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		allParams = append(allParams, struct{ name, value string }{name, pm.Value})
	}
	for _, qm := range a.QueryParameterMappings {
		name := qm.Parameter
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		allParams = append(allParams, struct{ name, value string }{name, qm.Value})
	}
	if len(allParams) > 0 {
		sb.WriteString("\n    with (")
		for i, p := range allParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("$")
			sb.WriteString(p.name)
			sb.WriteString(" = ")
			sb.WriteString(p.value)
		}
		sb.WriteString(")")
	}

	if a.BodyVariable != nil && a.BodyVariable.VariableName != "" {
		sb.WriteString("\n    body $")
		sb.WriteString(a.BodyVariable.VariableName)
	}

	// RestOperationCallAction does not support custom error handling (CE6035)
	// so we don't emit ON ERROR clauses.

	sb.WriteString(";")
	return sb.String()
}

// formatExecuteDatabaseQueryAction formats a DatabaseConnector ExecuteDatabaseQueryAction as MDL.
func formatExecuteDatabaseQueryAction(ctx *ExecContext, a *microflows.ExecuteDatabaseQueryAction) string {
	var sb strings.Builder

	if a.OutputVariableName != "" {
		sb.WriteString(fmt.Sprintf("$%s = ", a.OutputVariableName))
	}

	sb.WriteString("execute database query ")
	sb.WriteString(a.Query)

	// Dynamic query override
	if a.DynamicQuery != "" {
		sb.WriteString(fmt.Sprintf(" dynamic %s", a.DynamicQuery))
	}

	// Parameter mappings
	if len(a.ParameterMappings) > 0 {
		sb.WriteString(" (")
		for i, pm := range a.ParameterMappings {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s = %s", pm.ParameterName, pm.Value))
		}
		sb.WriteString(")")
	}

	// Connection parameter mappings (runtime connection override)
	if len(a.ConnectionParameterMappings) > 0 {
		sb.WriteString("\n    connection (")
		for i, cm := range a.ConnectionParameterMappings {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s = %s", cm.ParameterName, cm.Value))
		}
		sb.WriteString(")")
	}

	sb.WriteString(";")
	return sb.String()
}

// isMendixKeyword returns true for Mendix expression keywords that must not be
// prefixed with "$" when serialized as a RETURN value.
func isMendixKeyword(s string) bool {
	switch s {
	case "empty", "true", "false", "null":
		return true
	}
	return false
}

// isQualifiedEnumLiteral returns true for qualified enum literals (e.g., "Module.Enum.Value")
// that must not be prefixed with "$" when serialized as a RETURN value.
func isQualifiedEnumLiteral(s string) bool {
	return strings.Count(s, ".") >= 2
}

// isNumericLiteral returns true for numeric literals (integers and decimals)
// that must not be prefixed with "$" when serialized as a RETURN value.
func isNumericLiteral(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	if s[0] == '-' {
		start = 1
		if len(s) == 1 {
			return false
		}
	}
	dotSeen := false
	hasDigit := false
	for i := start; i < len(s); i++ {
		if s[i] == '.' && !dotSeen {
			dotSeen = true
		} else if s[i] >= '0' && s[i] <= '9' {
			hasDigit = true
		} else {
			return false
		}
	}
	return hasDigit && s[len(s)-1] != '.'
}

// formatImportXmlAction formats an import mapping action as MDL.
// Syntax: [$Var =] IMPORT FROM MAPPING Module.IMM($SourceVar);
func formatImportXmlAction(ctx *ExecContext, a *microflows.ImportXmlAction, entityNames map[model.ID]string) string {
	var sb strings.Builder

	// Resolve mapping qualified name
	mappingName := ""
	resultVar := ""
	if a.ResultHandling != nil {
		mappingName = string(a.ResultHandling.MappingID)
		resultVar = a.ResultHandling.ResultVariable
	}

	// Optional assignment
	if resultVar != "" {
		sb.WriteString("$")
		sb.WriteString(resultVar)
		sb.WriteString(" = ")
	}

	sb.WriteString("import from mapping ")
	sb.WriteString(mappingName)
	sb.WriteString("($")
	sb.WriteString(a.XmlDocumentVariable)
	sb.WriteString(");")

	return sb.String()
}

// formatExportXmlAction formats an export mapping action as MDL.
// Syntax: $Var = EXPORT TO MAPPING Module.EMM($SourceVar);
func formatExportXmlAction(ctx *ExecContext, a *microflows.ExportXmlAction) string {
	var sb strings.Builder

	// Output variable
	if a.OutputVariable != "" {
		sb.WriteString("$")
		sb.WriteString(a.OutputVariable)
		sb.WriteString(" = ")
	}

	sb.WriteString("export to mapping ")

	mappingName := ""
	paramVar := ""
	if a.RequestHandling != nil {
		mappingName = string(a.RequestHandling.MappingID)
		paramVar = a.RequestHandling.ParameterVariable
	}

	sb.WriteString(mappingName)
	if paramVar != "" {
		sb.WriteString("($")
		sb.WriteString(paramVar)
		sb.WriteString(")")
	}
	sb.WriteString(";")

	return sb.String()
}

// formatTransformJsonAction formats a TRANSFORM JSON action as MDL.
// Syntax: $Result = TRANSFORM $Input WITH Module.Transformer;
func formatTransformJsonAction(a *microflows.TransformJsonAction) string {
	var sb strings.Builder
	if a.OutputVariableName != "" {
		sb.WriteString("$")
		sb.WriteString(a.OutputVariableName)
		sb.WriteString(" = ")
	}
	sb.WriteString("transform $")
	sb.WriteString(a.InputVariableName)
	sb.WriteString(" with ")
	sb.WriteString(a.Transformation)
	sb.WriteString(";")
	return sb.String()
}

func parseReverseAssociationRetrieve(
	ctx *ExecContext,
	source *microflows.DatabaseRetrieveSource,
	entityName string,
) (string, string, bool) {
	if ctx == nil || ctx.Backend == nil || source == nil || entityName == "" {
		return "", "", false
	}
	if len(source.Sorting) > 0 || !isRangeAllOrNil(source.Range) {
		return "", "", false
	}

	assocName, startVar, ok := parseReverseAssociationXPath(source.XPathConstraint)
	if !ok || !databaseRetrieveMatchesAssociationTarget(ctx, entityName, assocName) {
		return "", "", false
	}
	return startVar, assocName, true
}

func isRangeAllOrNil(r *microflows.Range) bool {
	return r == nil || r.RangeType == "" || r.RangeType == microflows.RangeTypeAll
}

func parseReverseAssociationXPath(raw string) (string, string, bool) {
	parts, ok := splitTopLevelXPathPredicates(raw)
	if !ok || len(parts) != 1 {
		return "", "", false
	}

	condition := strings.TrimSpace(parts[0])
	if strings.ContainsAny(condition, "<>!") || strings.Count(condition, "=") != 1 {
		return "", "", false
	}

	sides := strings.SplitN(condition, "=", 2)
	assocName := strings.TrimSpace(sides[0])
	startVar := strings.TrimSpace(sides[1])
	if !isQualifiedAssociationName(assocName) || !strings.HasPrefix(startVar, "$") {
		return "", "", false
	}

	startVar = strings.TrimPrefix(startVar, "$")
	if !isSimpleMendixName(startVar) {
		return "", "", false
	}
	return assocName, startVar, true
}

func isQualifiedAssociationName(name string) bool {
	parts := strings.Split(name, ".")
	return len(parts) == 2 && isSimpleMendixName(parts[0]) && isSimpleMendixName(parts[1])
}

func isSimpleMendixName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
				continue
			}
			return false
		}
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func databaseRetrieveMatchesAssociationTarget(ctx *ExecContext, entityName, assocQualifiedName string) bool {
	moduleName, assocName, ok := strings.Cut(assocQualifiedName, ".")
	if !ok {
		return false
	}

	mod, err := ctx.Backend.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return false
	}
	dm, err := ctx.Backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return false
	}

	entityNames := make(map[model.ID]string, len(dm.Entities))
	for _, entity := range dm.Entities {
		entityNames[entity.ID] = moduleName + "." + entity.Name
	}
	for _, assoc := range dm.Associations {
		if assoc.Name == assocName {
			return entityNames[assoc.ParentID] == entityName
		}
	}
	return false
}

func splitTopLevelXPathPredicates(raw string) ([]string, bool) {
	var parts []string
	input := strings.TrimSpace(raw)
	if input == "" {
		return nil, false
	}

	i := 0
	for i < len(input) {
		for i < len(input) && (input[i] == ' ' || input[i] == '\t' || input[i] == '\r' || input[i] == '\n') {
			i++
		}
		if i >= len(input) {
			break
		}
		if input[i] != '[' {
			return nil, false
		}

		start := i + 1
		depth := 1
		var quote byte
		for i = start; i < len(input); i++ {
			ch := input[i]
			if quote != 0 {
				if ch == quote {
					quote = 0
				}
				continue
			}
			switch ch {
			case '\'', '"':
				quote = ch
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					part := strings.TrimSpace(input[start:i])
					parts = append(parts, part)
					i++
					goto nextPredicate
				}
			}
		}
		return nil, false

	nextPredicate:
	}

	if len(parts) == 0 {
		return nil, false
	}

	return parts, true
}

// --- Executor method wrappers for callers in unmigrated code and tests ---

func (e *Executor) formatActivity(obj microflows.MicroflowObject, entityNames map[model.ID]string, microflowNames map[model.ID]string) string {
	return formatActivity(e.newExecContext(context.Background()), obj, entityNames, microflowNames)
}

func (e *Executor) formatAction(action microflows.MicroflowAction, entityNames map[model.ID]string, microflowNames map[model.ID]string) string {
	return formatAction(e.newExecContext(context.Background()), action, entityNames, microflowNames)
}

func (e *Executor) formatListOperation(op microflows.ListOperation, outputVar string) string {
	return formatListOperation(e.newExecContext(context.Background()), op, outputVar)
}

func (e *Executor) formatRestCallAction(a *microflows.RestCallAction) string {
	return formatRestCallAction(e.newExecContext(context.Background()), a)
}

// formatSplitCondition renders an ExclusiveSplit's condition as an MDL expression.
// ExpressionSplitCondition is emitted verbatim. RuleSplitCondition is rendered as
// a rule call expression using the rule's qualified name — Mendix expressions
// allow calling a rule the same way as a microflow, so this is re-parseable.
// Unknown or nil conditions fall back to "true" so the describer still produces
// valid MDL; callers should rely on the original Caption (emitted via @caption)
// to preserve human-readable intent.
func formatSplitCondition(cond microflows.SplitCondition) string {
	switch c := cond.(type) {
	case *microflows.ExpressionSplitCondition:
		expr := strings.TrimRight(c.Expression, " \t\n\r")
		if expr == "" {
			return "true"
		}
		return expr
	case *microflows.RuleSplitCondition:
		name := c.RuleQualifiedName
		if name == "" {
			return "true"
		}
		args := make([]string, 0, len(c.ParameterMappings))
		for _, pm := range c.ParameterMappings {
			paramName := pm.ParameterName
			if idx := strings.LastIndex(paramName, "."); idx >= 0 {
				paramName = paramName[idx+1:]
			}
			arg := strings.TrimRight(pm.Argument, " \t\n\r")
			if paramName != "" {
				args = append(args, fmt.Sprintf("%s = %s", paramName, arg))
			} else {
				args = append(args, arg)
			}
		}
		return fmt.Sprintf("%s(%s)", name, strings.Join(args, ", "))
	default:
		return "true"
	}
}

func canonicalRawBSON(raw []byte) []byte {
	// RawBSON is preserved byte-for-byte when written back to disk. This
	// canonical form only stabilizes the MDL text emitted by describe, so BSON
	// documents with equivalent key/value content do not drift due to map or
	// parser ordering.
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	canonical, err := bson.Marshal(canonicalBSONValue(doc))
	if err != nil {
		return raw
	}
	return canonical
}

func canonicalBSONValue(value any) any {
	switch v := value.(type) {
	case bson.D:
		return canonicalBSONDocument(v)
	case bson.A:
		out := make(bson.A, len(v))
		for i, item := range v {
			out[i] = canonicalBSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = canonicalBSONValue(item)
		}
		return out
	case bson.M:
		return canonicalBSONMap(map[string]any(v))
	case map[string]any:
		return canonicalBSONMap(v)
	default:
		return value
	}
}

func canonicalBSONDocument(doc bson.D) bson.D {
	out := make(bson.D, len(doc))
	copy(out, doc)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	for i := range out {
		out[i].Value = canonicalBSONValue(out[i].Value)
	}
	return out
}

func canonicalBSONMap(m map[string]any) bson.D {
	doc := make(bson.D, 0, len(m))
	for k, v := range m {
		doc = append(doc, bson.E{Key: k, Value: canonicalBSONValue(v)})
	}
	return canonicalBSONDocument(doc)
}

// enrichXPathConstraintForDescribe enriches the raw BSON XPathConstraint string for
// DESCRIBE output. String-literal comparisons against enum attributes are replaced with
// qualified enum value references (e.g. Status = 'Open' → Status = Module.OrderStatus.Open).
// Falls back to the original string on any parse failure.
func enrichXPathConstraintForDescribe(ctx *ExecContext, entityQN, constraint string) string {
	if entityQN == "" || constraint == "" {
		return constraint
	}
	enumAttrs := buildEntityEnumAttrMap(ctx, entityQN)
	if len(enumAttrs) == 0 {
		return constraint
	}
	expr, ok := visitor.ParseXPathConstraint(constraint)
	if !ok || expr == nil {
		return constraint
	}
	enriched := enrichXPathExprWithEnums(expr, enumAttrs)
	return "[" + xpathExprToMDLString(enriched) + "]"
}
