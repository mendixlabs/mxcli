// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow builder: CRUD & data actions
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// addCreateVariableAction creates a DECLARE statement as a CreateVariableAction.
func (fb *flowBuilder) addCreateVariableAction(s *ast.DeclareStmt) model.ID {
	// Resolve TypeEnumeration → TypeEntity ambiguity using the domain model
	declType := s.Type
	if declType.Kind == ast.TypeEnumeration && declType.EnumRef != nil && fb.backend != nil {
		if fb.isEntity(declType.EnumRef.Module, declType.EnumRef.Name) {
			declType = ast.DataType{Kind: ast.TypeEntity, EntityRef: declType.EnumRef}
		}
	}

	// Register the variable as declared
	typeName := declType.Kind.String()
	fb.declaredVars[s.Variable] = typeName

	action := &microflows.CreateVariableAction{
		BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
		VariableName: s.Variable,
		DataType:     convertASTToMicroflowDataType(declType, nil),
		InitialValue: fb.exprToString(s.InitialValue),
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addChangeVariableAction creates a SET statement as a ChangeVariableAction.
func (fb *flowBuilder) addChangeVariableAction(s *ast.MfSetStmt) model.ID {
	// Validate that the variable has been declared
	if !fb.isVariableDeclared(s.Target) {
		fb.addErrorWithExample(
			fmt.Sprintf("variable '%s' is not declared", s.Target),
			errorExampleDeclareVariable(s.Target))
	}

	action := &microflows.ChangeVariableAction{
		BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
		VariableName: s.Target,
		Value:        fb.exprToString(s.Value),
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addCreateObjectAction creates a CREATE OBJECT statement.
func (fb *flowBuilder) addCreateObjectAction(s *ast.CreateObjectStmt) model.ID {
	action := &microflows.CreateObjectAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		OutputVariable: s.Variable,
		Commit:         microflows.CommitTypeNo,
	}
	// Set entity reference as qualified name (BY_NAME_REFERENCE)
	entityQN := ""
	if s.EntityType.Module != "" && s.EntityType.Name != "" {
		entityQN = s.EntityType.Module + "." + s.EntityType.Name
		action.EntityQualifiedName = entityQN
	}

	// Register variable type for CHANGE statements
	if fb.varTypes != nil && entityQN != "" {
		fb.varTypes[s.Variable] = entityQN
	}

	// Build InitialMembers for each SET assignment
	for _, change := range s.Changes {
		memberChange := &microflows.MemberChange{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Type:        microflows.MemberChangeTypeSet,
			Value:       fb.memberExpressionToString(change.Value, entityQN, change.Attribute),
		}
		fb.resolveMemberChange(memberChange, change.Attribute, entityQN)
		action.InitialMembers = append(action.InitialMembers, memberChange)
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
			ErrorHandlingType:   convertErrorHandlingType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	// Build custom error handler flow if present
	if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
		errorY := fb.posY + VerticalSpacing
		mergeID := fb.addErrorHandlerFlow(activity.ID, activityX, s.ErrorHandling.Body)
		fb.handleErrorHandlerMerge(mergeID, activity.ID, errorY)
	}

	return activity.ID
}

// addCommitAction creates a COMMIT statement.
func (fb *flowBuilder) addCommitAction(s *ast.MfCommitStmt) model.ID {
	action := &microflows.CommitObjectsAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: convertErrorHandlingType(s.ErrorHandling),
		CommitVariable:    s.Variable,
		WithEvents:        s.WithEvents,
		RefreshInClient:   s.RefreshInClient,
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	// Build custom error handler flow if present
	if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
		errorY := fb.posY + VerticalSpacing
		mergeID := fb.addErrorHandlerFlow(activity.ID, activityX, s.ErrorHandling.Body)
		fb.handleErrorHandlerMerge(mergeID, activity.ID, errorY)
	}

	return activity.ID
}

// addDeleteAction creates a DELETE statement.
func (fb *flowBuilder) addDeleteAction(s *ast.DeleteObjectStmt) model.ID {
	action := &microflows.DeleteObjectAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		DeleteVariable: s.Variable,
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
			ErrorHandlingType:   convertErrorHandlingType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	// Build custom error handler flow if present
	if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
		errorY := fb.posY + VerticalSpacing
		mergeID := fb.addErrorHandlerFlow(activity.ID, activityX, s.ErrorHandling.Body)
		fb.handleErrorHandlerMerge(mergeID, activity.ID, errorY)
	}

	return activity.ID
}

// addRollbackAction creates a ROLLBACK statement.
func (fb *flowBuilder) addRollbackAction(s *ast.RollbackStmt) model.ID {
	action := &microflows.RollbackObjectAction{
		BaseElement:      model.BaseElement{ID: model.ID(types.GenerateID())},
		RollbackVariable: s.Variable,
		RefreshInClient:  s.RefreshInClient,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	return activity.ID
}

// addChangeObjectAction creates a CHANGE statement.
func (fb *flowBuilder) addChangeObjectAction(s *ast.ChangeObjectStmt) model.ID {
	action := &microflows.ChangeObjectAction{
		BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
		ChangeVariable:  s.Variable,
		Commit:          microflows.CommitTypeNo,
		RefreshInClient: false,
	}

	// Look up entity type from variable scope
	entityQN := ""
	if fb.varTypes != nil {
		entityQN = fb.varTypes[s.Variable]
	}

	// Build MemberChange items for each SET assignment
	for _, change := range s.Changes {
		memberChange := &microflows.MemberChange{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Type:        microflows.MemberChangeTypeSet,
			Value:       fb.memberExpressionToString(change.Value, entityQN, change.Attribute),
		}
		fb.resolveMemberChange(memberChange, change.Attribute, entityQN)
		action.Changes = append(action.Changes, memberChange)
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addRetrieveAction creates a RETRIEVE statement.
func (fb *flowBuilder) addRetrieveAction(s *ast.RetrieveStmt) model.ID {
	var source microflows.RetrieveSource

	if s.StartVariable != "" {
		// Association retrieve: RETRIEVE $List FROM $Parent/Module.AssocName
		assocQN := s.Source.Module + "." + s.Source.Name

		// Look up association to determine type and direction.
		// For Reference associations, AssociationRetrieveSource always returns a single
		// object (the entity on the other end). When the user navigates from the child
		// (non-owner) side, the intent is to get a list of parent entities — we must use
		// a DatabaseRetrieveSource with XPath constraint instead.
		assocInfo := fb.lookupAssociation(s.Source.Module, s.Source.Name)
		startVarType := ""
		if fb.varTypes != nil {
			startVarType = fb.varTypes[s.StartVariable]
		}

		if assocInfo != nil && assocInfo.Type == domainmodel.AssociationTypeReference &&
			assocInfo.childEntityQN != "" && startVarType == assocInfo.childEntityQN {
			// Reverse traversal on Reference: child → parent (one-to-many)
			// Use DatabaseRetrieveSource with XPath to get a list of parent entities
			dbSource := &microflows.DatabaseRetrieveSource{
				BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
				EntityQualifiedName: assocInfo.parentEntityQN,
				XPathConstraint:     "[" + assocQN + " = $" + s.StartVariable + "]",
			}
			source = dbSource
			if fb.varTypes != nil {
				fb.varTypes[s.Variable] = "List of " + assocInfo.parentEntityQN
			}
		} else {
			// Forward traversal or ReferenceSet: use AssociationRetrieveSource
			source = &microflows.AssociationRetrieveSource{
				BaseElement:              model.BaseElement{ID: model.ID(types.GenerateID())},
				StartVariable:            s.StartVariable,
				AssociationQualifiedName: assocQN,
			}
			if fb.varTypes != nil {
				if assocInfo != nil && assocInfo.Type == domainmodel.AssociationTypeReference {
					// Reference forward traversal: returns single object
					otherEntity := assocInfo.childEntityQN
					if startVarType == assocInfo.childEntityQN {
						otherEntity = assocInfo.parentEntityQN
					}
					fb.varTypes[s.Variable] = otherEntity
				} else {
					// ReferenceSet or unknown: returns a list
					fb.varTypes[s.Variable] = "List of " + assocQN
				}
			}
		}
	} else {
		// Database retrieve: RETRIEVE $List FROM Module.Entity WHERE ...
		entityQN := s.Source.Module + "." + s.Source.Name
		dbSource := &microflows.DatabaseRetrieveSource{
			BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
			EntityQualifiedName: entityQN,
		}

		// Set range if LIMIT is specified
		if s.Limit != "" {
			rangeType := microflows.RangeTypeCustom
			// LIMIT 1 with no offset uses RangeTypeFirst for single object retrieval
			if s.Limit == "1" && s.Offset == "" {
				rangeType = microflows.RangeTypeFirst
			}
			dbSource.Range = &microflows.Range{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				RangeType:   rangeType,
				Limit:       s.Limit,
				Offset:      s.Offset,
			}
		}

		// Convert WHERE expression if present
		// XPath constraints are stored with square brackets in BSON: [expression]
		if s.Where != nil {
			dbSource.XPathConstraint = "[" + expressionToXPath(s.Where) + "]"
		}

		// Convert SORT BY columns if present
		if len(s.SortColumns) > 0 {
			for _, col := range s.SortColumns {
				// Resolve attribute path - if just a simple name, prefix with entity
				attrPath := col.Attribute
				if !strings.Contains(attrPath, ".") {
					attrPath = entityQN + "." + attrPath
				} else {
					// Validate that qualified attribute path belongs to the retrieved entity
					// Expected format: Module.Entity.Attribute
					parts := strings.Split(attrPath, ".")
					if len(parts) >= 3 {
						// Extract entity from attribute path (first two parts)
						attrEntityQN := parts[0] + "." + parts[1]
						if attrEntityQN != entityQN {
							fb.addError("SORT BY attribute '%s' does not belong to entity '%s'", col.Attribute, entityQN)
							continue // Skip this sort column but continue processing others
						}
					}
				}

				direction := microflows.SortDirectionAscending
				if col.Order == "DESC" {
					direction = microflows.SortDirectionDescending
				}

				dbSource.Sorting = append(dbSource.Sorting, &microflows.SortItem{
					BaseElement:            model.BaseElement{ID: model.ID(types.GenerateID())},
					AttributeQualifiedName: attrPath,
					Direction:              direction,
				})
			}
		}

		source = dbSource

		// Register variable type for CHANGE statements
		// RETRIEVE with LIMIT 1 returns a single entity, otherwise returns a List
		if fb.varTypes != nil {
			if s.Limit == "1" {
				// LIMIT 1 returns a single entity
				fb.varTypes[s.Variable] = entityQN
			} else {
				// No LIMIT or LIMIT > 1 returns a list
				fb.varTypes[s.Variable] = "List of " + entityQN
			}
		}
	}

	action := &microflows.RetrieveAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		OutputVariable: s.Variable,
		Source:         source,
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
			ErrorHandlingType:   convertErrorHandlingType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	// Build custom error handler flow if present
	if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
		errorY := fb.posY + VerticalSpacing
		mergeID := fb.addErrorHandlerFlow(activity.ID, activityX, s.ErrorHandling.Body)
		fb.handleErrorHandlerMerge(mergeID, activity.ID, errorY)
	}

	return activity.ID
}

// addListOperationAction creates list operations like HEAD, TAIL, FIND, etc.
func (fb *flowBuilder) addListOperationAction(s *ast.ListOperationStmt) model.ID {
	var operation microflows.ListOperation

	switch s.Operation {
	case ast.ListOpHead:
		operation = &microflows.HeadOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
		}
	case ast.ListOpTail:
		operation = &microflows.TailOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
		}
	case ast.ListOpFind:
		operation = &microflows.FindOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
			Expression:   fb.exprToString(s.Condition),
		}
	case ast.ListOpFilter:
		operation = &microflows.FilterOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
			Expression:   fb.exprToString(s.Condition),
		}
	case ast.ListOpSort:
		// Resolve entity type from input variable for qualified attribute names
		entityType := ""
		if fb.varTypes != nil {
			listType := fb.varTypes[s.InputVariable]
			if after, ok := strings.CutPrefix(listType, "List of "); ok {
				entityType = after
			}
		}

		// Build sort items from SortSpecs
		var sortItems []*microflows.SortItem
		for _, spec := range s.SortSpecs {
			direction := microflows.SortDirectionAscending
			if !spec.Ascending {
				direction = microflows.SortDirectionDescending
			}
			// Build fully qualified attribute name: Entity.Attribute
			attrQN := spec.Attribute
			if entityType != "" && !strings.Contains(spec.Attribute, ".") {
				attrQN = entityType + "." + spec.Attribute
			}
			sortItems = append(sortItems, &microflows.SortItem{
				BaseElement:            model.BaseElement{ID: model.ID(types.GenerateID())},
				AttributeQualifiedName: attrQN,
				Direction:              direction,
			})
		}
		operation = &microflows.SortOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
			Sorting:      sortItems,
		}
	case ast.ListOpUnion:
		operation = &microflows.UnionOperation{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable1: s.InputVariable,
			ListVariable2: s.SecondVariable,
		}
	case ast.ListOpIntersect:
		operation = &microflows.IntersectOperation{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable1: s.InputVariable,
			ListVariable2: s.SecondVariable,
		}
	case ast.ListOpSubtract:
		operation = &microflows.SubtractOperation{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable1: s.InputVariable,
			ListVariable2: s.SecondVariable,
		}
	case ast.ListOpContains:
		operation = &microflows.ContainsOperation{
			BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable:   s.InputVariable,
			ObjectVariable: s.SecondVariable, // The item to check
		}
	case ast.ListOpEquals:
		operation = &microflows.EqualsOperation{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable1: s.InputVariable,
			ListVariable2: s.SecondVariable,
		}
	case ast.ListOpRange:
		rangeOp := &microflows.ListRangeOperation{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			ListVariable: s.InputVariable,
		}
		if s.OffsetExpr != nil {
			rangeOp.OffsetExpression = fb.exprToString(s.OffsetExpr)
		}
		if s.LimitExpr != nil {
			rangeOp.LimitExpression = fb.exprToString(s.LimitExpr)
		}
		operation = rangeOp
	default:
		return ""
	}

	action := &microflows.ListOperationAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		Operation:      operation,
		OutputVariable: s.OutputVariable,
	}

	// Track output variable type for operations that preserve/produce list types
	if fb.varTypes != nil && s.OutputVariable != "" && s.InputVariable != "" {
		inputType := fb.varTypes[s.InputVariable]
		switch s.Operation {
		case ast.ListOpFilter, ast.ListOpSort, ast.ListOpTail, ast.ListOpUnion, ast.ListOpIntersect, ast.ListOpSubtract, ast.ListOpRange:
			// These operations preserve the list type
			if inputType != "" {
				fb.varTypes[s.OutputVariable] = inputType
			}
		case ast.ListOpHead, ast.ListOpFind:
			// These return a single element (remove "List of " prefix)
			if after, ok := strings.CutPrefix(inputType, "List of "); ok {
				fb.varTypes[s.OutputVariable] = after
			}
			// CONTAINS and EQUALS return Boolean, no need to track
		}
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addAggregateListAction creates aggregate operations like COUNT, SUM, AVERAGE, etc.
func (fb *flowBuilder) addAggregateListAction(s *ast.AggregateListStmt) model.ID {
	var function microflows.AggregateFunction
	switch s.Operation {
	case ast.AggregateCount:
		function = microflows.AggregateFunctionCount
	case ast.AggregateSum:
		function = microflows.AggregateFunctionSum
	case ast.AggregateAverage:
		function = microflows.AggregateFunctionAverage
	case ast.AggregateMinimum:
		function = microflows.AggregateFunctionMin
	case ast.AggregateMaximum:
		function = microflows.AggregateFunctionMax
	default:
		return ""
	}

	action := &microflows.AggregateListAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		InputVariable:  s.InputVariable,
		OutputVariable: s.OutputVariable,
		Function:       function,
	}

	// For SUM/AVG/MIN/MAX, we need the attribute
	if s.Attribute != "" {
		// Build qualified name: need entity type from variable
		if fb.varTypes != nil {
			listType := fb.varTypes[s.InputVariable]
			if after, ok := strings.CutPrefix(listType, "List of "); ok {
				entityType := after
				action.AttributeQualifiedName = entityType + "." + s.Attribute
			}
		}
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addCreateListAction creates a CREATE LIST OF statement.
func (fb *flowBuilder) addCreateListAction(s *ast.CreateListStmt) model.ID {
	entityQN := ""
	if s.EntityType.Module != "" && s.EntityType.Name != "" {
		entityQN = s.EntityType.Module + "." + s.EntityType.Name
	}

	action := &microflows.CreateListAction{
		BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
		OutputVariable:      s.Variable,
		EntityQualifiedName: entityQN,
	}

	// Register variable type as list
	if fb.varTypes != nil && entityQN != "" {
		fb.varTypes[s.Variable] = "List of " + entityQN
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addAddToListAction creates an ADD TO list statement.
func (fb *flowBuilder) addAddToListAction(s *ast.AddToListStmt) model.ID {
	action := &microflows.ChangeListAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		Type:           microflows.ChangeListTypeAdd,
		ChangeVariable: s.List,
		Value:          "$" + s.Item,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// addRemoveFromListAction creates a REMOVE FROM list statement.
func (fb *flowBuilder) addRemoveFromListAction(s *ast.RemoveFromListStmt) model.ID {
	action := &microflows.ChangeListAction{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		Type:           microflows.ChangeListTypeRemove,
		ChangeVariable: s.List,
		Value:          "$" + s.Item,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	return activity.ID
}

// isEntity checks whether a qualified name refers to an entity in the domain model.
func (fb *flowBuilder) isEntity(moduleName, entityName string) bool {
	if fb.backend == nil {
		return false
	}
	mod, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return false
	}
	dm, err := fb.backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return false
	}
	for _, e := range dm.Entities {
		if e.Name == entityName {
			return true
		}
	}
	return false
}

// resolveMemberChange determines whether a member name is an association or attribute
// and sets the appropriate field on the MemberChange. It queries the domain model
// to check if the name matches an association on the entity; if no backend is available,
// it falls back to the dot-contains heuristic.
//
// memberName can be either bare ("Order_Customer") or qualified ("MfTest.Order_Customer").
func (fb *flowBuilder) resolveMemberChange(mc *microflows.MemberChange, memberName string, entityQN string) {
	if entityQN == "" {
		return
	}

	// Split entity qualified name into module and entity
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		mc.AttributeQualifiedName = entityQN + "." + memberName
		return
	}
	moduleName := parts[0]

	// If memberName is already qualified (e.g., "MfTest.Order_Customer"),
	// extract the bare name for association lookup.
	bareName := memberName
	qualifiedName := memberName
	if dot := strings.Index(memberName, "."); dot >= 0 {
		bareName = memberName[dot+1:]
		// qualifiedName is already set to the full memberName
	} else {
		qualifiedName = moduleName + "." + memberName
	}

	// Query domain model to check if this member is an association
	if fb.backend != nil {
		if mod, err := fb.backend.GetModuleByName(moduleName); err == nil && mod != nil {
			if dm, err := fb.backend.GetDomainModel(mod.ID); err == nil && dm != nil {
				for _, a := range dm.Associations {
					if a.Name == bareName {
						mc.AssociationQualifiedName = qualifiedName
						return
					}
				}
				for _, a := range dm.CrossAssociations {
					if a.Name == bareName {
						mc.AssociationQualifiedName = qualifiedName
						return
					}
				}
				// Not an association — it's an attribute
				if strings.Contains(memberName, ".") {
					// Already qualified, don't double-qualify
					mc.AttributeQualifiedName = memberName
				} else {
					mc.AttributeQualifiedName = entityQN + "." + memberName
				}
				return
			}
		}
	}

	// Fallback: if already qualified (contains dot), treat as association
	if strings.Contains(memberName, ".") {
		mc.AssociationQualifiedName = memberName
	} else {
		mc.AttributeQualifiedName = entityQN + "." + memberName
	}
}

// assocLookupResult holds resolved association metadata.
type assocLookupResult struct {
	Type           domainmodel.AssociationType
	parentEntityQN string // Qualified name of the parent (FROM/owner) entity
	childEntityQN  string // Qualified name of the child (TO/referenced) entity
}

// lookupAssociation finds an association by module and name, returning its type
// and the qualified names of its parent and child entities. Returns nil if the
// association cannot be found (e.g., backend is nil or module doesn't exist).
func (fb *flowBuilder) lookupAssociation(moduleName, assocName string) *assocLookupResult {
	if fb.backend == nil {
		return nil
	}
	mod, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return nil
	}
	dm, err := fb.backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return nil
	}

	// Build entity ID → qualified name map
	entityNames := make(map[model.ID]string, len(dm.Entities))
	for _, e := range dm.Entities {
		entityNames[e.ID] = moduleName + "." + e.Name
	}

	for _, a := range dm.Associations {
		if a.Name == assocName {
			return &assocLookupResult{
				Type:           a.Type,
				parentEntityQN: entityNames[a.ParentID],
				childEntityQN:  entityNames[a.ChildID],
			}
		}
	}
	return nil
}
