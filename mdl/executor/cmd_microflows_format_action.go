// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow activity and action formatting as MDL statements.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// formatActivity formats a single microflow activity as an MDL statement.
func (e *Executor) formatActivity(
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
				!strings.ContainsAny(returnVal, "+'\"()") {
				returnVal = "$" + returnVal
			}
			return fmt.Sprintf("RETURN %s;", returnVal)
		}
		return "" // Skip end events without return value

	case *microflows.ActionActivity:
		return e.formatAction(activity.Action, entityNames, microflowNames)

	case *microflows.ExclusiveSplit:
		condition := "true"
		if activity.SplitCondition != nil {
			if exprCond, ok := activity.SplitCondition.(*microflows.ExpressionSplitCondition); ok {
				condition = exprCond.Expression
			}
		}
		return fmt.Sprintf("IF %s THEN", condition)

	case *microflows.ExclusiveMerge:
		return "END IF;"

	case *microflows.LoopedActivity:
		switch ls := activity.LoopSource.(type) {
		case *microflows.WhileLoopCondition:
			return fmt.Sprintf("WHILE %s", ls.WhileExpression)
		case *microflows.IterableList:
			iterVar := "Item"
			listVar := "List"
			if ls.VariableName != "" {
				iterVar = ls.VariableName
			}
			if ls.ListVariableName != "" {
				listVar = ls.ListVariableName
			}
			return fmt.Sprintf("LOOP $%s IN $%s", iterVar, listVar)
		default:
			return "LOOP $Item IN $List"
		}

	case *microflows.BreakEvent:
		return "BREAK;"

	case *microflows.ContinueEvent:
		return "CONTINUE;"

	case *microflows.ErrorEvent:
		return "RAISE ERROR;"

	case *microflows.Annotation:
		return "" // Annotations are emitted separately via @annotation

	default:
		return fmt.Sprintf("-- Unknown activity type: %T", obj)
	}
}

// formatAction formats a microflow action as an MDL statement.
func (e *Executor) formatAction(
	action microflows.MicroflowAction,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) string {
	if action == nil {
		return "-- Empty action"
	}

	switch a := action.(type) {
	case *microflows.CreateVariableAction:
		varType := "Object"
		if a.DataType != nil {
			varType = e.formatMicroflowDataType(a.DataType, entityNames)
		}
		initialValue := strings.TrimSuffix(a.InitialValue, "\n")
		if initialValue == "" {
			initialValue = "empty"
		}
		return fmt.Sprintf("DECLARE $%s %s = %s;", a.VariableName, varType, initialValue)

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
			return fmt.Sprintf("CHANGE %s (%s = %s);", objectName, attrName, a.Value)
		}
		// Simple variable change
		if strings.HasPrefix(varName, "$") {
			return fmt.Sprintf("SET %s = %s;", varName, a.Value)
		}
		return fmt.Sprintf("SET $%s = %s;", varName, a.Value)

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
			for _, m := range a.InitialMembers {
				var memberName string
				// Check if this is an association change or an attribute change
				if m.AssociationQualifiedName != "" {
					// Association: extract just the association name
					memberName = m.AssociationQualifiedName
					if parts := strings.Split(memberName, "."); len(parts) > 0 {
						memberName = parts[len(parts)-1]
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
				members = append(members, fmt.Sprintf("%s = %s", memberName, m.Value))
			}
			return fmt.Sprintf("$%s = CREATE %s (%s);", outputVar, entityName, strings.Join(members, ", "))
		}
		return fmt.Sprintf("$%s = CREATE %s;", outputVar, entityName)

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
				members = append(members, fmt.Sprintf("%s = %s", memberName, m.Value))
			}
			return fmt.Sprintf("CHANGE $%s (%s);", varName, strings.Join(members, ", "))
		}
		return fmt.Sprintf("CHANGE $%s;", varName)

	case *microflows.CommitObjectsAction:
		varName := a.CommitVariable
		if varName == "" {
			varName = "Object"
		}
		suffix := ""
		if a.WithEvents {
			suffix += " WITH EVENTS"
		}
		if a.RefreshInClient {
			suffix += " REFRESH"
		}
		return fmt.Sprintf("COMMIT $%s%s;", varName, suffix)

	case *microflows.DeleteObjectAction:
		return fmt.Sprintf("DELETE $%s;", a.DeleteVariable)

	case *microflows.RollbackObjectAction:
		if a.RefreshInClient {
			return fmt.Sprintf("ROLLBACK $%s REFRESH;", a.RollbackVariable)
		}
		return fmt.Sprintf("ROLLBACK $%s;", a.RollbackVariable)

	case *microflows.CreateListAction:
		// Use EntityQualifiedName (BY_NAME_REFERENCE) or fall back to EntityID lookup
		entityName := a.EntityQualifiedName
		if entityName == "" {
			entityName = entityNames[a.EntityID]
		}
		if entityName == "" {
			entityName = "Entity"
		}
		return fmt.Sprintf("$%s = CREATE LIST of %s;", a.OutputVariable, entityName)

	case *microflows.ChangeListAction:
		varName := a.ChangeVariable
		switch a.Type {
		case microflows.ChangeListTypeAdd:
			return fmt.Sprintf("ADD %s TO $%s;", a.Value, varName)
		case microflows.ChangeListTypeRemove:
			return fmt.Sprintf("REMOVE %s FROM $%s;", a.Value, varName)
		case microflows.ChangeListTypeClear:
			return fmt.Sprintf("CLEAR $%s;", varName)
		case microflows.ChangeListTypeSet:
			return fmt.Sprintf("SET $%s = %s;", varName, a.Value)
		default:
			return fmt.Sprintf("CHANGE LIST $%s (%s);", varName, a.Type)
		}

	case *microflows.ListOperationAction:
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "Result"
		}
		return e.formatListOperation(a.Operation, outputVar)

	case *microflows.AggregateListAction:
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "Result"
		}
		fn := string(a.Function)
		if fn == "" {
			fn = "COUNT"
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
		// For aggregate functions that require an attribute (Sum, Average, Min, Max), show the attribute
		if attrName != "" && a.Function != microflows.AggregateFunctionCount {
			return fmt.Sprintf("$%s = %s($%s.%s);", outputVar, strings.ToUpper(fn), a.InputVariable, attrName)
		}
		return fmt.Sprintf("$%s = %s($%s);", outputVar, strings.ToUpper(fn), a.InputVariable)

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

			stmt := fmt.Sprintf("RETRIEVE $%s FROM %s", outputVar, entityName)

			if dbSource.XPathConstraint != "" {
				constraint := strings.TrimSpace(dbSource.XPathConstraint)
				if strings.HasPrefix(constraint, "[") && strings.HasSuffix(constraint, "]") {
					constraint = constraint[1 : len(constraint)-1]
				}
				stmt += fmt.Sprintf("\n    WHERE %s", constraint)
			}

			// Output SORT BY clause if present
			if len(dbSource.Sorting) > 0 {
				var sortParts []string
				for _, sortItem := range dbSource.Sorting {
					attrName := sortItem.AttributeQualifiedName
					order := "ASC"
					if sortItem.Direction == microflows.SortDirectionDescending {
						order = "DESC"
					}
					sortParts = append(sortParts, attrName+" "+order)
				}
				stmt += fmt.Sprintf("\n    SORT BY %s", strings.Join(sortParts, ", "))
			}

			if dbSource.Range != nil {
				switch dbSource.Range.RangeType {
				case microflows.RangeTypeFirst:
					stmt += "\n    LIMIT 1"
				case microflows.RangeTypeCustom:
					if dbSource.Range.Limit != "" {
						stmt += fmt.Sprintf("\n    LIMIT %s", dbSource.Range.Limit)
					}
					if dbSource.Range.Offset != "" {
						stmt += fmt.Sprintf("\n    OFFSET %s", dbSource.Range.Offset)
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
			return fmt.Sprintf("RETRIEVE $%s FROM $%s/%s;", outputVar, startVar, assocName)
		}

		return fmt.Sprintf("RETRIEVE $%s FROM ...;", outputVar)

	case *microflows.LogMessageAction:
		level := string(a.LogLevel)
		if level == "" {
			level = "INFO"
		}
		// Node is an expression in Mendix (e.g., 'TEST' or $variable or 'Prefix' + $var)
		// Output it as-is since it's already stored as an expression
		node := a.LogNodeName
		if node == "" {
			node = "'Application'" // Default value as a string literal expression
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
			// Wrap message in quotes for MDL syntax (escape any existing single quotes)
			message = "'" + strings.ReplaceAll(message, "'", "''") + "'"
		}

		// Build WITH clause if there are template parameters
		withClause := ""
		if len(a.TemplateParameters) > 0 {
			var params []string
			for i, expr := range a.TemplateParameters {
				params = append(params, fmt.Sprintf("{%d} = %s", i+1, expr))
			}
			withClause = fmt.Sprintf(" WITH (%s)", strings.Join(params, ", "))
		}

		return fmt.Sprintf("LOG %s NODE %s %s%s;", strings.ToUpper(level), node, message, withClause)

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

		if a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = CALL MICROFLOW %s(%s);", a.ResultVariableName, mfName, paramStr)
		}
		return fmt.Sprintf("CALL MICROFLOW %s(%s);", mfName, paramStr)

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
			valueStr := "..."
			switch v := pm.Value.(type) {
			case *microflows.StringTemplateParameterValue:
				if v.TypedTemplate != nil {
					valueStr = v.TypedTemplate.Text
				}
			case *microflows.ExpressionBasedCodeActionParameterValue:
				if v.Expression != "" {
					valueStr = v.Expression
				}
			case *microflows.BasicCodeActionParameterValue:
				if v.Argument != "" {
					valueStr = v.Argument
				}
			case *microflows.EntityTypeCodeActionParameterValue:
				if v.Entity != "" {
					valueStr = "'" + v.Entity + "'"
				}
			}
			params = append(params, fmt.Sprintf("%s = %s", paramName, valueStr))
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = CALL JAVA ACTION %s(%s);", a.ResultVariableName, javaActionName, paramStr)
		}
		return fmt.Sprintf("CALL JAVA ACTION %s(%s);", javaActionName, paramStr)

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

		if a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = CALL EXTERNAL ACTION %s.%s(%s);", a.ResultVariableName, serviceName, actionName, paramStr)
		}
		return fmt.Sprintf("CALL EXTERNAL ACTION %s.%s(%s);", serviceName, actionName, paramStr)

	case *microflows.ShowPageAction:
		// Get page name from action (PageName is BY_NAME_REFERENCE, PageID is legacy BY_ID_REFERENCE)
		pageName := a.PageName
		if pageName == "" && a.PageID != "" && e.reader != nil {
			// Fall back to looking up by ID (legacy format)
			pages, _ := e.reader.ListPages()
			for _, p := range pages {
				if p.ID == a.PageID {
					h, _ := e.getHierarchy()
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
		return fmt.Sprintf("SHOW PAGE %s%s;", pageName, paramStr)

	case *microflows.ClosePageAction:
		if a.NumberOfPages > 1 {
			return fmt.Sprintf("CLOSE PAGE %d;", a.NumberOfPages)
		}
		return "CLOSE PAGE;"

	case *microflows.ShowHomePageAction:
		return "SHOW HOME PAGE;"

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
			// Wrap message in quotes for MDL syntax (escape any existing single quotes)
			message = "'" + strings.ReplaceAll(message, "'", "''") + "'"
		}
		result := fmt.Sprintf("SHOW MESSAGE %s TYPE %s", message, msgType)
		if len(a.TemplateParameters) > 0 {
			result += " OBJECTS [" + strings.Join(a.TemplateParameters, ", ") + "]"
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
			// Wrap message in quotes for MDL syntax (escape any existing single quotes)
			msgText = "'" + strings.ReplaceAll(msgText, "'", "''") + "'"
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
		}
		return fmt.Sprintf("VALIDATION FEEDBACK %s MESSAGE %s;", attrPath, msgText)

	case *microflows.RestCallAction:
		return e.formatRestCallAction(a)

	case *microflows.RestOperationCallAction:
		return e.formatRestOperationCallAction(a)

	case *microflows.ExecuteDatabaseQueryAction:
		return e.formatExecuteDatabaseQueryAction(a)

	case *microflows.ImportXmlAction:
		return e.formatImportXmlAction(a, entityNames)

	case *microflows.ExportXmlAction:
		return e.formatExportXmlAction(a)

	case *microflows.TransformJsonAction:
		return formatTransformJsonAction(a)

	// Workflow microflow actions
	case *microflows.GetWorkflowDataAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = GET WORKFLOW DATA $%s AS %s;", a.OutputVariableName, a.WorkflowVariable, a.Workflow)
		}
		return fmt.Sprintf("GET WORKFLOW DATA $%s AS %s;", a.WorkflowVariable, a.Workflow)

	case *microflows.WorkflowCallAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = CALL WORKFLOW %s ($%s);", a.OutputVariableName, a.Workflow, a.WorkflowContextVariable)
		}
		return fmt.Sprintf("CALL WORKFLOW %s ($%s);", a.Workflow, a.WorkflowContextVariable)

	case *microflows.GetWorkflowsAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = GET WORKFLOWS FOR $%s;", a.OutputVariableName, a.WorkflowContextVariableName)
		}
		return fmt.Sprintf("GET WORKFLOWS FOR $%s;", a.WorkflowContextVariableName)

	case *microflows.GetWorkflowActivityRecordsAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = GET WORKFLOW ACTIVITY RECORDS $%s;", a.OutputVariableName, a.WorkflowVariable)
		}
		return fmt.Sprintf("GET WORKFLOW ACTIVITY RECORDS $%s;", a.WorkflowVariable)

	case *microflows.WorkflowOperationAction:
		return e.formatWorkflowOperationAction(a)

	case *microflows.SetTaskOutcomeAction:
		return fmt.Sprintf("SET TASK OUTCOME $%s '%s';", a.WorkflowTaskVariable, a.OutcomeValue)

	case *microflows.OpenUserTaskAction:
		return fmt.Sprintf("OPEN USER TASK $%s;", a.UserTaskVariable)

	case *microflows.NotifyWorkflowAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = NOTIFY WORKFLOW $%s;", a.OutputVariableName, a.WorkflowVariable)
		}
		return fmt.Sprintf("NOTIFY WORKFLOW $%s;", a.WorkflowVariable)

	case *microflows.OpenWorkflowAction:
		return fmt.Sprintf("OPEN WORKFLOW $%s;", a.WorkflowVariable)

	case *microflows.LockWorkflowAction:
		if a.PauseAllWorkflows {
			return "LOCK WORKFLOW ALL;"
		}
		if a.Workflow != "" {
			return fmt.Sprintf("LOCK WORKFLOW %s;", a.Workflow)
		}
		return fmt.Sprintf("LOCK WORKFLOW $%s;", a.WorkflowVariable)

	case *microflows.UnlockWorkflowAction:
		if a.ResumeAllPausedWorkflows {
			return "UNLOCK WORKFLOW ALL;"
		}
		if a.Workflow != "" {
			return fmt.Sprintf("UNLOCK WORKFLOW %s;", a.Workflow)
		}
		return fmt.Sprintf("UNLOCK WORKFLOW $%s;", a.WorkflowVariable)

	case *microflows.UnknownAction:
		return fmt.Sprintf("-- Unsupported action type: %s", a.TypeName)

	default:
		return fmt.Sprintf("-- Unknown action: %T", action)
	}
}

// formatWorkflowOperationAction formats a workflow operation action as MDL.
func (e *Executor) formatWorkflowOperationAction(a *microflows.WorkflowOperationAction) string {
	if a.Operation == nil {
		return "WORKFLOW OPERATION ...;"
	}
	switch op := a.Operation.(type) {
	case *microflows.AbortOperation:
		if op.Reason != "" {
			return fmt.Sprintf("WORKFLOW OPERATION ABORT $%s REASON '%s';", op.WorkflowVariable, strings.ReplaceAll(op.Reason, "'", "''"))
		}
		return fmt.Sprintf("WORKFLOW OPERATION ABORT $%s;", op.WorkflowVariable)
	case *microflows.ContinueOperation:
		return fmt.Sprintf("WORKFLOW OPERATION CONTINUE $%s;", op.WorkflowVariable)
	case *microflows.PauseOperation:
		return fmt.Sprintf("WORKFLOW OPERATION PAUSE $%s;", op.WorkflowVariable)
	case *microflows.RestartOperation:
		return fmt.Sprintf("WORKFLOW OPERATION RESTART $%s;", op.WorkflowVariable)
	case *microflows.RetryOperation:
		return fmt.Sprintf("WORKFLOW OPERATION RETRY $%s;", op.WorkflowVariable)
	case *microflows.UnpauseOperation:
		return fmt.Sprintf("WORKFLOW OPERATION UNPAUSE $%s;", op.WorkflowVariable)
	default:
		return fmt.Sprintf("-- Unknown workflow operation: %T", a.Operation)
	}
}

// formatListOperation formats a list operation as MDL.
func (e *Executor) formatListOperation(op microflows.ListOperation, outputVar string) string {
	if op == nil {
		return fmt.Sprintf("$%s = LIST OPERATION ...;", outputVar)
	}

	switch o := op.(type) {
	case *microflows.HeadOperation:
		return fmt.Sprintf("$%s = HEAD($%s);", outputVar, o.ListVariable)
	case *microflows.TailOperation:
		return fmt.Sprintf("$%s = TAIL($%s);", outputVar, o.ListVariable)
	case *microflows.FindOperation:
		return fmt.Sprintf("$%s = FIND($%s, %s);", outputVar, o.ListVariable, o.Expression)
	case *microflows.FilterOperation:
		return fmt.Sprintf("$%s = FILTER($%s, %s);", outputVar, o.ListVariable, o.Expression)
	case *microflows.SortOperation:
		if len(o.Sorting) > 0 {
			var sortCols []string
			for _, s := range o.Sorting {
				dir := "ASC"
				if s.Direction == microflows.SortDirectionDescending {
					dir = "DESC"
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
				sortCols = append(sortCols, fmt.Sprintf("%s %s", attrName, dir))
			}
			return fmt.Sprintf("$%s = SORT($%s, %s);", outputVar, o.ListVariable, strings.Join(sortCols, ", "))
		}
		return fmt.Sprintf("$%s = SORT($%s);", outputVar, o.ListVariable)
	case *microflows.UnionOperation:
		return fmt.Sprintf("$%s = UNION($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.IntersectOperation:
		return fmt.Sprintf("$%s = INTERSECT($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.SubtractOperation:
		return fmt.Sprintf("$%s = SUBTRACT($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.ContainsOperation:
		return fmt.Sprintf("$%s = CONTAINS($%s, $%s);", outputVar, o.ListVariable, o.ObjectVariable)
	case *microflows.EqualsOperation:
		return fmt.Sprintf("$%s = EQUALS($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.FindByAttributeOperation:
		ref := o.Attribute
		if ref == "" {
			ref = o.Association
		}
		parts := strings.Split(ref, ".")
		fieldName := parts[len(parts)-1]
		return fmt.Sprintf("$%s = FIND($%s, $currentObject/%s = %s);", outputVar, o.ListVariable, fieldName, o.Expression)
	case *microflows.FilterByAttributeOperation:
		ref := o.Attribute
		if ref == "" {
			ref = o.Association
		}
		parts := strings.Split(ref, ".")
		fieldName := parts[len(parts)-1]
		return fmt.Sprintf("$%s = FILTER($%s, $currentObject/%s = %s);", outputVar, o.ListVariable, fieldName, o.Expression)
	case *microflows.RangeOperation:
		return fmt.Sprintf("$%s = RANGE($%s, %s, %s);", outputVar, o.ListVariable, o.OffsetExpression, o.LimitExpression)
	default:
		return fmt.Sprintf("$%s = LIST OPERATION %T;", outputVar, op)
	}
}

// formatRestCallAction formats a REST call action as MDL.
func (e *Executor) formatRestCallAction(a *microflows.RestCallAction) string {
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

	sb.WriteString("REST CALL ")

	// HTTP method
	method := "GET"
	if a.HttpConfiguration != nil {
		switch a.HttpConfiguration.HttpMethod {
		case microflows.HttpMethodGet:
			method = "GET"
		case microflows.HttpMethodPost:
			method = "POST"
		case microflows.HttpMethodPut:
			method = "PUT"
		case microflows.HttpMethodPatch:
			method = "PATCH"
		case microflows.HttpMethodDelete:
			method = "DELETE"
		}
	}
	sb.WriteString(method)
	sb.WriteString(" ")

	// URL
	url := "''"
	if a.HttpConfiguration != nil && a.HttpConfiguration.LocationTemplate != "" {
		url = "'" + strings.ReplaceAll(a.HttpConfiguration.LocationTemplate, "'", "''") + "'"
	}
	sb.WriteString(url)

	// URL parameters
	if a.HttpConfiguration != nil && len(a.HttpConfiguration.LocationParams) > 0 {
		sb.WriteString(" WITH (")
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
			sb.WriteString("\n    HEADER '")
			sb.WriteString(strings.ReplaceAll(h.Name, "'", "''"))
			sb.WriteString("' = ")
			sb.WriteString(h.Value)
		}
	}

	// Authentication
	if a.HttpConfiguration != nil && a.HttpConfiguration.UseAuthentication {
		sb.WriteString("\n    AUTH BASIC ")
		sb.WriteString(a.HttpConfiguration.Username)
		sb.WriteString(" PASSWORD ")
		sb.WriteString(a.HttpConfiguration.Password)
	}

	// Body
	if a.RequestHandling != nil {
		switch rh := a.RequestHandling.(type) {
		case *microflows.CustomRequestHandling:
			if rh.Template != "" {
				sb.WriteString("\n    BODY '")
				sb.WriteString(strings.ReplaceAll(rh.Template, "'", "''"))
				sb.WriteString("'")
				// Add template parameters if present
				if len(rh.TemplateParams) > 0 {
					sb.WriteString(" WITH (")
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
				sb.WriteString("\n    BODY MAPPING ")
				sb.WriteString(string(rh.MappingID))
				if rh.ParameterVariable != "" {
					sb.WriteString(" FROM $")
					sb.WriteString(rh.ParameterVariable)
				}
			}
		}
	}

	// Timeout
	if a.TimeoutExpression != "" {
		sb.WriteString("\n    TIMEOUT ")
		sb.WriteString(a.TimeoutExpression)
	}

	// Returns
	sb.WriteString("\n    RETURNS ")
	if a.ResultHandling != nil {
		switch rh := a.ResultHandling.(type) {
		case *microflows.ResultHandlingString:
			sb.WriteString("String")
			_ = rh // used for type assertion only
		case *microflows.ResultHandlingMapping:
			sb.WriteString("MAPPING ")
			sb.WriteString(string(rh.MappingID))
			if rh.ResultEntityID != "" {
				sb.WriteString(" AS ")
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
func (e *Executor) formatRestOperationCallAction(a *microflows.RestOperationCallAction) string {
	var sb strings.Builder

	if a.OutputVariable != nil && a.OutputVariable.VariableName != "" {
		sb.WriteString("$")
		sb.WriteString(a.OutputVariable.VariableName)
		sb.WriteString(" = ")
	}

	sb.WriteString("SEND REST REQUEST ")
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
		sb.WriteString("\n    WITH (")
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
		sb.WriteString("\n    BODY $")
		sb.WriteString(a.BodyVariable.VariableName)
	}

	// RestOperationCallAction does not support custom error handling (CE6035)
	// so we don't emit ON ERROR clauses.

	sb.WriteString(";")
	return sb.String()
}

// formatExecuteDatabaseQueryAction formats a DatabaseConnector ExecuteDatabaseQueryAction as MDL.
func (e *Executor) formatExecuteDatabaseQueryAction(a *microflows.ExecuteDatabaseQueryAction) string {
	var sb strings.Builder

	if a.OutputVariableName != "" {
		sb.WriteString(fmt.Sprintf("$%s = ", a.OutputVariableName))
	}

	sb.WriteString("EXECUTE DATABASE QUERY ")
	sb.WriteString(a.Query)

	// Dynamic query override
	if a.DynamicQuery != "" {
		sb.WriteString(fmt.Sprintf(" DYNAMIC %s", a.DynamicQuery))
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
		sb.WriteString("\n    CONNECTION (")
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

// formatImportXmlAction formats an import mapping action as MDL.
// Syntax: [$Var =] IMPORT FROM MAPPING Module.IMM($SourceVar);
func (e *Executor) formatImportXmlAction(a *microflows.ImportXmlAction, entityNames map[model.ID]string) string {
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

	sb.WriteString("IMPORT FROM MAPPING ")
	sb.WriteString(mappingName)
	sb.WriteString("($")
	sb.WriteString(a.XmlDocumentVariable)
	sb.WriteString(");")

	return sb.String()
}

// formatExportXmlAction formats an export mapping action as MDL.
// Syntax: $Var = EXPORT TO MAPPING Module.EMM($SourceVar);
func (e *Executor) formatExportXmlAction(a *microflows.ExportXmlAction) string {
	var sb strings.Builder

	// Output variable
	if a.OutputVariable != "" {
		sb.WriteString("$")
		sb.WriteString(a.OutputVariable)
		sb.WriteString(" = ")
	}

	sb.WriteString("EXPORT TO MAPPING ")

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
	sb.WriteString("TRANSFORM $")
	sb.WriteString(a.InputVariableName)
	sb.WriteString(" WITH ")
	sb.WriteString(a.Transformation)
	sb.WriteString(";")
	return sb.String()
}
