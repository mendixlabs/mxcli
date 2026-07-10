// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow builder: call, control flow, and client actions
package executor

import (
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// defaultLogNodeExpression is the quoted Mendix expression used for the log
// node when none is specified on a LOG statement. Single source of truth shared
// by the builder, the formatter, and cmd_diff_mdl.
const defaultLogNodeExpression = "'Application'"

// addLogMessageAction creates a LOG statement as a LogMessageAction.
func (fb *flowBuilder) addLogMessageAction(s *ast.LogStmt) model.ID {
	logLevel := microflows.LogLevelInfo
	switch s.Level {
	case ast.LogTrace:
		logLevel = microflows.LogLevelTrace
	case ast.LogDebug:
		logLevel = microflows.LogLevelDebug
	case ast.LogWarning:
		logLevel = microflows.LogLevelWarning
	case ast.LogError:
		logLevel = microflows.LogLevelError
	case ast.LogCritical:
		logLevel = microflows.LogLevelCritical
	}

	// Determine template text and parameters
	// If message is a simple string literal, use it directly
	// If message is a complex expression, use {1} as template and add expression as parameter
	var templateText string
	var templateParams []string

	if len(s.Template) > 0 {
		// Use provided template parameters
		if lit, ok := s.Message.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
			templateText = fmt.Sprintf("%v", lit.Value)
		} else {
			templateText = fb.exprToString(s.Message)
		}
		// Sort parameters by index to ensure correct order
		maxIndex := 0
		for _, p := range s.Template {
			if p.Index > maxIndex {
				maxIndex = p.Index
			}
		}
		templateParams = make([]string, maxIndex)
		for _, p := range s.Template {
			if p.Index > 0 && p.Index <= maxIndex {
				templateParams[p.Index-1] = fb.exprToString(p.Value)
			}
		}
	} else if lit, ok := s.Message.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		// Simple string literal - use directly as template
		templateText = fmt.Sprintf("%v", lit.Value)
	} else {
		// Complex expression - use {1} placeholder and add expression as parameter
		templateText = "{1}"
		templateParams = []string{fb.exprToString(s.Message)}
	}

	logNodeName := defaultLogNodeExpression
	if s.Node != nil {
		logNodeName = fb.exprToString(s.Node)
	}

	action := &microflows.LogMessageAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(nil),
		LogLevel:          logLevel,
		LogNodeName:       logNodeName,
		MessageTemplate: &model.Text{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Translations: map[string]string{
				"en_US": templateText,
			},
		},
		TemplateParameters: templateParams,
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

// addCallMicroflowAction creates a CALL MICROFLOW statement.
func (fb *flowBuilder) addCallMicroflowAction(s *ast.CallMicroflowStmt) model.ID {
	mfQN := s.MicroflowName.Module + "." + s.MicroflowName.Name

	if !fb.microflowExists(mfQN) {
		fb.addError("CALL MICROFLOW '%s': microflow not found in the project (check module name and spelling)", mfQN)
	}

	// Build parameter mappings for MicroflowCall
	var mappings []*microflows.MicroflowCallParameterMapping
	for _, arg := range s.Arguments {
		// Parameter is the full qualified name: Module.Microflow.ParameterName
		paramQN := mfQN + "." + arg.Name
		mapping := &microflows.MicroflowCallParameterMapping{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Parameter:   paramQN,
			Argument:    fb.exprToString(arg.Value),
		}
		mappings = append(mappings, mapping)
	}

	// Create nested MicroflowCall structure
	mfCall := &microflows.MicroflowCall{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		Microflow:         mfQN,
		ParameterMappings: mappings,
	}
	action := &microflows.MicroflowCallAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  fb.ehType(s.ErrorHandling),
		MicroflowCall:      mfCall,
		ResultVariableName: s.OutputVariable,
		UseReturnVariable:  s.OutputVariable != "",
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

	if s.OutputVariable != "" {
		fb.registerResultVariableType(s.OutputVariable, fb.lookupMicroflowReturnType(mfQN))
	}

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.OutputVariable)

	return activity.ID
}

// addCallNanoflowAction creates a CALL NANOFLOW statement.
func (fb *flowBuilder) addCallNanoflowAction(s *ast.CallNanoflowStmt) model.ID {
	nfQN := s.NanoflowName.Module + "." + s.NanoflowName.Name

	if !fb.nanoflowExists(nfQN) {
		fb.addError("CALL NANOFLOW '%s': nanoflow not found in the project (check module name and spelling)", nfQN)
	}

	// Build parameter mappings for NanoflowCall
	var mappings []*microflows.NanoflowCallParameterMapping
	for _, arg := range s.Arguments {
		paramQN := nfQN + "." + arg.Name
		mapping := &microflows.NanoflowCallParameterMapping{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Parameter:   paramQN,
			Argument:    fb.exprToString(arg.Value),
		}
		mappings = append(mappings, mapping)
	}

	nfCall := &microflows.NanoflowCall{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		Nanoflow:          nfQN,
		ParameterMappings: mappings,
	}

	action := &microflows.NanoflowCallAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  fb.ehType(s.ErrorHandling),
		NanoflowCall:       nfCall,
		OutputVariableName: s.OutputVariable,
		UseReturnVariable:  s.OutputVariable != "",
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

	if s.OutputVariable != "" {
		fb.registerResultVariableType(s.OutputVariable, fb.lookupNanoflowReturnType(nfQN))
	}

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.OutputVariable)

	return activity.ID
}

// addCallJavaActionAction creates a CALL JAVA ACTION statement.
func (fb *flowBuilder) addCallJavaActionAction(s *ast.CallJavaActionStmt) model.ID {
	actionQN := s.ActionName.Module + "." + s.ActionName.Name

	// Try to look up the Java action definition to detect EntityTypeParameterType parameters
	var jaDef *javaactions.JavaAction
	if fb.backend != nil {
		var err error
		jaDef, err = fb.backend.ReadJavaActionByName(actionQN)
		if err != nil {
			log.Printf("warning: could not look up Java action %s: %v (entity type params will be empty)", actionQN, err)
		}
	}

	// Build a map of parameter name -> param type for the Java action.
	// resolvedBasicParams tracks parameters whose type was successfully
	// resolved via the Java action definition AND is not an entity-type or
	// microflow-type parameter (i.e. anything that lands in
	// BasicCodeActionParameterValue: String, Integer, Boolean, ListType,
	// ParameterizedEntityType, etc.). When the MDL author binds such a
	// parameter to `empty`, Studio Pro authors `Argument: "empty"` (the MDL
	// literal string) rather than `Argument: ""`, the unbound marker. The
	// distinction matters: `mx check` reports CE0126 "Missing value for
	// parameter X" when a typed parameter receives the unbound `""` shape.
	//
	// Without a backend lookup (jaDef == nil) we fall back to the prior
	// `""` behaviour to preserve the documented "intentionally unbound"
	// semantics of PROPOSAL_microflow_empty_java_action_argument.md.
	entityTypeParams := make(map[string]bool)
	microflowTypeParams := make(map[string]bool)
	resolvedBasicParams := make(map[string]bool)
	if jaDef != nil {
		for _, p := range jaDef.Parameters {
			switch p.ParameterType.(type) {
			case *javaactions.EntityTypeParameterType:
				entityTypeParams[p.Name] = true
			case *javaactions.MicroflowType:
				microflowTypeParams[p.Name] = true
			default:
				resolvedBasicParams[p.Name] = true
			}
		}
	}

	// Build parameter mappings with Value structure
	var mappings []*microflows.JavaActionParameterMapping
	for _, arg := range s.Arguments {
		// Parameter qualified name format: Module.JavaAction.ParameterName
		// (both Module and JavaAction are namespaces, so all levels are included)
		paramQN := actionQN + "." + arg.Name

		// Check if this parameter is typed to a type parameter (EntityTypeParameterType)
		var value microflows.CodeActionParameterValue
		if entityTypeParams[arg.Name] {
			// Entity type parameter: value is the entity qualified name, not the variable reference.
			// When the argument is a variable like $Email, resolve its entity type from varTypes.
			valueExpr := fb.exprToString(arg.Value)
			entityName := strings.Trim(valueExpr, "'")
			if strings.HasPrefix(entityName, "$") {
				varName := strings.TrimPrefix(entityName, "$")
				if resolvedType, ok := fb.varTypes[varName]; ok {
					entityName = resolvedType
				}
			}
			value = &microflows.EntityTypeCodeActionParameterValue{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Entity:      entityName,
			}
		} else if isEmptyJavaActionArgument(arg.Value) {
			if microflowTypeParams[arg.Name] {
				value = &microflows.MicroflowParameterValue{
					BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					Microflow:   "",
				}
			} else {
				// When the Java action definition is available and the
				// parameter is a typed BasicParameterType (anything that
				// isn't entity-type or microflow-type — String, Integer,
				// Boolean, ListType, ParameterizedEntityType, etc.), Studio
				// Pro authors `Argument: "empty"` for the MDL `empty`
				// literal. Without that information (jaDef == nil) keep the
				// blank-string "intentionally unbound" marker that
				// PROPOSAL_microflow_empty_java_action_argument.md
				// established for code-action callers without backend
				// resolution.
				argument := ""
				if resolvedBasicParams[arg.Name] {
					argument = "empty"
				}
				value = &microflows.BasicCodeActionParameterValue{
					BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					Argument:    argument,
				}
			}
		} else {
			// Regular parameter: expression-based value
			valueExpr := fb.exprToString(arg.Value)
			if microflowTypeParams[arg.Name] {
				value = &microflows.MicroflowParameterValue{
					BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					Microflow:   strings.Trim(valueExpr, "'"),
				}
			} else {
				value = &microflows.BasicCodeActionParameterValue{
					BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					Argument:    valueExpr,
				}
			}
		}

		mapping := &microflows.JavaActionParameterMapping{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Parameter:   paramQN,
			Value:       value,
		}
		mappings = append(mappings, mapping)
	}

	action := &microflows.JavaActionCallAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  fb.ehType(s.ErrorHandling),
		JavaAction:         actionQN,
		ParameterMappings:  mappings,
		ResultVariableName: s.OutputVariable,
		UseReturnVariable:  s.OutputVariable != "",
	}
	if s.OutputVariable != "" && jaDef != nil && fb.varTypes != nil {
		if varType := javaActionReturnVarType(jaDef.ReturnType); varType != "" {
			fb.varTypes[s.OutputVariable] = varType
		} else if inferred := fb.inferGenericJavaActionReturnType(jaDef, s); inferred != "" {
			fb.varTypes[s.OutputVariable] = inferred
		}
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

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.OutputVariable)

	return activity.ID
}

func javaActionReturnVarType(returnType javaactions.CodeActionReturnType) string {
	switch t := returnType.(type) {
	case *javaactions.EntityType:
		return t.Entity
	case *javaactions.ListType:
		if t.Entity != "" {
			return "List of " + t.Entity
		}
	case *javaactions.FileDocumentType:
		return "System.FileDocument"
	}
	return ""
}

// inferGenericJavaActionReturnType infers the element type of a generic
// `ListType{Entity: ""}` Java action return by inspecting the caller's
// variable-typed arguments. When the action parameters don't determine an
// element type, the function returns "" and the caller records the declaration
// as Unknown.
func (fb *flowBuilder) inferGenericJavaActionReturnType(jaDef *javaactions.JavaAction, s *ast.CallJavaActionStmt) string {
	if jaDef == nil || fb.varTypes == nil || s == nil {
		return ""
	}
	// ListType is always stored as a pointer by the parser; there is no value
	// form in the SDK. Only the generic (Entity == "") case reaches the
	// variable-type lookup below.
	t, ok := jaDef.ReturnType.(*javaactions.ListType)
	if !ok || t.Entity != "" {
		return ""
	}
	for _, arg := range s.Arguments {
		varExpr, ok := arg.Value.(*ast.VariableExpr)
		if !ok {
			continue
		}
		if typ := fb.varTypes[varExpr.Name]; strings.HasPrefix(typ, "List of ") {
			return typ
		}
	}
	return ""
}

// addCallJavaScriptActionAction creates a CALL JAVASCRIPT ACTION statement.
func (fb *flowBuilder) addCallJavaScriptActionAction(s *ast.CallJavaScriptActionStmt) model.ID {
	actionQN := s.ActionName.Module + "." + s.ActionName.Name

	// Build parameter mappings with Value structure
	var mappings []*microflows.JavaScriptActionParameterMapping
	for _, arg := range s.Arguments {
		// Parameter qualified name format: Module.JavaScriptAction.ParameterName
		paramQN := actionQN + "." + arg.Name

		// JavaScript actions use BasicCodeActionParameterValue for all parameters
		valueExpr := fb.exprToString(arg.Value)
		value := &microflows.BasicCodeActionParameterValue{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Argument:    valueExpr,
		}

		mapping := &microflows.JavaScriptActionParameterMapping{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Parameter:   paramQN,
			Value:       value,
		}
		mappings = append(mappings, mapping)
	}

	action := &microflows.JavaScriptActionCallAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  fb.ehType(s.ErrorHandling),
		JavaScriptAction:   actionQN,
		ParameterMappings:  mappings,
		OutputVariableName: s.OutputVariable,
		UseReturnVariable:  s.OutputVariable != "",
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

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.OutputVariable)

	return activity.ID
}

func isEmptyJavaActionArgument(expr ast.Expression) bool {
	lit, ok := expr.(*ast.LiteralExpr)
	return ok && (lit.Kind == ast.LiteralEmpty || lit.Kind == ast.LiteralNull)
}

// addCallWebServiceAction creates a legacy SOAP WebServiceCallAction.
func (fb *flowBuilder) addCallWebServiceAction(s *ast.CallWebServiceStmt) model.ID {
	activityX := fb.posX
	action := &microflows.WebServiceCallAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: convertErrorHandlingType(s.ErrorHandling),
		ServiceID:         model.ID(s.ServiceID),
		OperationName:     s.OperationName,
		SendMappingID:     model.ID(fb.resolveMappingRefForWrite(s.SendMappingID, true)),
		ReceiveMappingID:  model.ID(fb.resolveMappingRefForWrite(s.ReceiveMappingID, false)),
		OutputVariable:    s.OutputVariable,
		UseReturnVariable: s.OutputVariable != "",
	}
	if s.RawBSONBase64 != "" {
		raw, err := base64.StdEncoding.DecodeString(s.RawBSONBase64)
		if err != nil {
			fb.addError("invalid raw web service action payload: %v", err)
		} else {
			action.RawBSON = raw
		}
	}
	if s.Timeout != nil {
		action.TimeoutExpression = fb.exprToString(s.Timeout)
	}

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

	if s.OutputVariable != "" && fb.declaredVars != nil {
		fb.declaredVars[s.OutputVariable] = "Unknown"
	}

	if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
		errorY := fb.posY + VerticalSpacing
		mergeID := fb.addErrorHandlerFlow(activity.ID, activityX, s.ErrorHandling.Body)
		fb.handleErrorHandlerMerge(mergeID, activity.ID, errorY)
	}

	return activity.ID
}

func (fb *flowBuilder) resolveMappingRefForWrite(ref string, preferExport bool) string {
	if ref == "" || !strings.Contains(ref, ".") || fb.backend == nil {
		return ref
	}
	moduleName, name, ok := strings.Cut(ref, ".")
	if !ok || moduleName == "" || name == "" {
		return ref
	}
	if preferExport {
		if mapping, err := fb.backend.GetExportMappingByQualifiedName(moduleName, name); err == nil && mapping != nil {
			return string(mapping.ID)
		}
		if mapping, err := fb.backend.GetImportMappingByQualifiedName(moduleName, name); err == nil && mapping != nil {
			return string(mapping.ID)
		}
	} else {
		if mapping, err := fb.backend.GetImportMappingByQualifiedName(moduleName, name); err == nil && mapping != nil {
			return string(mapping.ID)
		}
		if mapping, err := fb.backend.GetExportMappingByQualifiedName(moduleName, name); err == nil && mapping != nil {
			return string(mapping.ID)
		}
	}
	return ref
}

// resolveExternalActionReturnKind looks up the called OData action in the
// consumed service's cached $metadata and returns the Mendix kind name
// ("Boolean", "String", "Integer", "Long", "Decimal", "DateTime", "Binary",
// or "Void") of its return type. Used to populate
// CallExternalAction.ResultDataType so the writer can emit VariableDataType
// BSON; without it Mendix raises CE7269 whenever the schema declares any
// return type.
//
// Returns "" if the service or action can't be resolved — the writer omits
// VariableDataType, falling back to the prior (buggy) behavior rather than
// emitting a wrong type.
func (fb *flowBuilder) resolveExternalActionReturnKind(serviceRef ast.QualifiedName, actionName string) string {
	if fb.backend == nil {
		return ""
	}
	services, err := fb.backend.ListConsumedODataServices()
	if err != nil {
		return ""
	}
	for _, svc := range services {
		modName := fb.hierarchy.GetModuleName(fb.hierarchy.FindModuleID(svc.ContainerID))
		if !strings.EqualFold(modName, serviceRef.Module) || !strings.EqualFold(svc.Name, serviceRef.Name) {
			continue
		}
		if svc.Metadata == "" {
			return ""
		}
		doc, err := types.ParseEdmx(svc.Metadata)
		if err != nil {
			return ""
		}
		for _, act := range doc.Actions {
			if strings.EqualFold(act.Name, actionName) {
				return edmReturnTypeToKind(act.ReturnType)
			}
		}
		return ""
	}
	return ""
}

// edmReturnTypeToKind maps an EDM type name (e.g. "Edm.Boolean") to the
// Mendix kind name used by serializeExternalActionReturnType. Returns "Void"
// for an empty/unknown return type so action calls with no return still get
// a valid DataTypes$VoidType BSON sub-doc rather than nothing.
func edmReturnTypeToKind(edmType string) string {
	switch edmType {
	case "":
		return "Void"
	case "Edm.Boolean":
		return "Boolean"
	case "Edm.String", "Edm.Guid":
		return "String"
	case "Edm.Int32", "Edm.Int16", "Edm.Byte", "Edm.SByte":
		return "Integer"
	case "Edm.Int64":
		return "Long"
	case "Edm.Decimal", "Edm.Double", "Edm.Single":
		return "Decimal"
	case "Edm.DateTime", "Edm.DateTimeOffset", "Edm.Date":
		return "DateTime"
	case "Edm.Binary":
		return "Binary"
	default:
		// Complex / collection / entity-typed returns aren't yet mapped.
		// Leave empty so the writer omits VariableDataType rather than
		// emitting a wrong type that would silently mislead Mendix.
		return ""
	}
}

// addCallExternalActionAction creates a CALL EXTERNAL ACTION statement.
func (fb *flowBuilder) addCallExternalActionAction(s *ast.CallExternalActionStmt) model.ID {
	serviceQN := s.ServiceName.Module + "." + s.ServiceName.Name

	// Build parameter mappings
	var mappings []*microflows.ExternalActionParameterMapping
	for _, arg := range s.Arguments {
		mapping := &microflows.ExternalActionParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ParameterName: arg.Name,
			Argument:      fb.exprToString(arg.Value),
		}
		mappings = append(mappings, mapping)
	}

	action := &microflows.CallExternalAction{
		BaseElement:          model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:    fb.ehType(s.ErrorHandling),
		ConsumedODataService: serviceQN,
		Name:                 s.ActionName,
		ParameterMappings:    mappings,
		ResultVariableName:   s.OutputVariable,
		UseReturnVariable:    s.OutputVariable != "",
		ResultDataType:       fb.resolveExternalActionReturnKind(s.ServiceName, s.ActionName),
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

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.OutputVariable)

	return activity.ID
}

// addShowPageAction creates a SHOW PAGE statement.
func (fb *flowBuilder) addShowPageAction(s *ast.ShowPageStmt) model.ID {
	// Use page qualified name (BY_NAME_REFERENCE) - the modern Mendix format
	// uses FormSettings.Form as a string reference, not a binary UUID
	pageQN := s.PageName.Module + "." + s.PageName.Name

	// Build page parameter mappings
	var mappings []*microflows.PageParameterMapping
	for _, arg := range s.Arguments {
		// Parameter qualified name format: Module.Page.ParameterName
		paramQN := pageQN + "." + arg.ParamName
		mapping := &microflows.PageParameterMapping{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Parameter:   paramQN,
			Argument:    fb.exprToString(arg.Value),
		}
		mappings = append(mappings, mapping)
	}

	// Determine page location
	var location microflows.PageLocation
	switch s.Location {
	case "Popup":
		location = microflows.PageLocationPopup
	case "Modal":
		location = microflows.PageLocationModal
	default:
		location = microflows.PageLocationContent
	}

	// Create page settings
	pageSettings := &microflows.PageSettings{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Location:    location,
		ModalForm:   s.ModalForm,
	}

	// Create the action
	// Use PageName (BY_NAME_REFERENCE) instead of PageID (BY_ID_REFERENCE)
	// The modern Mendix format uses FormSettings.Form as a qualified name string
	action := &microflows.ShowPageAction{
		BaseElement:           model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:     fb.ehType(nil),
		PageName:              pageQN, // BY_NAME_REFERENCE - qualified name string
		PageSettings:          pageSettings,
		PageParameterMappings: mappings,
	}

	// Set passed object if FOR syntax was used
	if s.ForObject != "" {
		action.PassedObject = "$" + s.ForObject
	}

	// Set title override if specified
	if s.Title != "" {
		action.OverridePageTitle = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": s.Title},
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

// addShowHomePageAction creates a SHOW HOME PAGE statement.
func (fb *flowBuilder) addShowHomePageAction(s *ast.ShowHomePageStmt) model.ID {
	action := &microflows.ShowHomePageAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
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

// addShowMessageAction creates a SHOW MESSAGE statement.
func (fb *flowBuilder) addShowMessageAction(s *ast.ShowMessageStmt) model.ID {
	// Build template text and parameters from message expression.
	// For string literals, use the raw value directly as template text.
	// For complex expressions, use {1} placeholder and add expression as parameter.
	var templateText string
	var templateParams []string

	if lit, ok := s.Message.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		templateText = fmt.Sprintf("%v", lit.Value)
	} else {
		templateText = "{1}"
		templateParams = []string{fb.exprToString(s.Message)}
	}

	// Append template parameters from TemplateArgs (e.g., OBJECTS [$Var1, $Var2])
	for _, arg := range s.TemplateArgs {
		templateParams = append(templateParams, fb.exprToString(arg))
	}

	template := &model.Text{
		BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
		Translations: map[string]string{"en_US": templateText},
	}

	msgType := microflows.MessageType(s.Type)
	if msgType == "" {
		msgType = microflows.MessageTypeInformation
	}

	action := &microflows.ShowMessageAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  fb.ehType(nil),
		Template:           template,
		Type:               msgType,
		TemplateParameters: templateParams,
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

// addDownloadFileAction creates a DOWNLOAD FILE statement.
func (fb *flowBuilder) addDownloadFileAction(s *ast.DownloadFileStmt) model.ID {
	action := &microflows.DownloadFileAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		FileDocument:      s.FileDocument,
		ShowInBrowser:     s.ShowInBrowser,
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	}
	if s.ErrorHandling != nil {
		action.ErrorHandlingType = fb.ehType(s.ErrorHandling)
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

// addClosePageAction creates a CLOSE PAGE statement.
func (fb *flowBuilder) addClosePageAction(s *ast.ClosePageStmt) model.ID {
	numPages := s.NumberOfPages
	if numPages <= 0 {
		numPages = 1
	}

	action := &microflows.ClosePageAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(nil),
		NumberOfPages:     numPages,
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

// addValidationFeedbackAction creates a VALIDATION FEEDBACK statement as a ValidationFeedbackAction.
func (fb *flowBuilder) addValidationFeedbackAction(s *ast.ValidationFeedbackStmt) model.ID {
	// Build the template text from the message expression.
	// For string literals, use the raw value (without quotes) since the template
	// text is plain text, not a microflow expression. For complex expressions,
	// use {1} placeholder with the expression as a parameter (same pattern as LogMessageAction).
	var templateText string
	var templateParams []string

	if lit, ok := s.Message.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		// Simple string literal - use raw value directly as template text
		templateText = fmt.Sprintf("%v", lit.Value)
	} else {
		// Complex expression - use {1} placeholder and add expression as parameter
		templateText = "{1}"
		templateParams = []string{fb.exprToString(s.Message)}
	}

	// Create template with translations map (default language "en_US")
	template := &model.Text{
		BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
		Translations: map[string]string{"en_US": templateText},
	}

	// Build attribute or association name from variable type and attribute path.
	// The current grammar keeps `$Product/Module.Association` as one slash
	// segment, so dot count on that segment is the disambiguator:
	//   0 dots: attribute relative to the target entity.
	//   1 dot: association qualified by module.
	//   2+ dots: fully qualified attribute.
	var attributeName string
	var associationName string
	entityQName := ""
	if fb.varTypes != nil {
		entityQName = fb.varTypes[s.AttributePath.Variable]
	}
	if len(s.AttributePath.Segments) > 0 {
		segs := s.AttributePath.Segments
		if len(segs) == 1 {
			switch strings.Count(segs[0].Name, ".") {
			case 0:
				// Single bare segment: direct attribute access.
				if entityQName != "" {
					attributeName = entityQName + "." + segs[0].Name
				} else {
					attributeName = segs[0].Name
				}
			case 1:
				// Qualified association names use Module.Association.
				associationName = segs[0].Name
			default:
				// Fully-qualified attributes use Module.Entity.Attribute.
				attributeName = segs[0].Name
			}
		} else {
			// Multi-hop paths are not a validation-feedback association target.
			// Fall back to the first segment as an attribute so we do not join
			// unrelated traversal pieces into a synthetic association name.
			if entityQName != "" {
				attributeName = entityQName + "." + segs[0].Name
			} else {
				attributeName = segs[0].Name
			}
		}
	} else if entityQName != "" && len(s.AttributePath.Path) > 0 {
		// Fallback for legacy Path without Segments
		attributeName = entityQName + "." + s.AttributePath.Path[0]
	}

	// Append template parameters from TemplateArgs (e.g., OBJECTS [$Var1, $Var2])
	for _, arg := range s.TemplateArgs {
		templateParams = append(templateParams, fb.exprToString(arg))
	}

	// Strip the $ prefix from variable name for BSON storage
	varName := s.AttributePath.Variable
	if strings.HasPrefix(varName, "$") {
		varName = varName[1:]
	}

	action := &microflows.ValidationFeedbackAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  fb.ehType(nil),
		ObjectVariable:     varName,
		AttributeName:      attributeName,
		AssociationName:    associationName,
		Template:           template,
		TemplateParameters: templateParams,
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

// addRestCallAction creates a REST CALL statement as a RestCallAction.
func (fb *flowBuilder) addRestCallAction(s *ast.RestCallStmt) model.ID {
	// Build HTTP configuration
	httpConfig := &microflows.HttpConfiguration{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
	}

	// Set HTTP method
	switch s.Method {
	case ast.HttpMethodGet:
		httpConfig.HttpMethod = microflows.HttpMethodGet
	case ast.HttpMethodPost:
		httpConfig.HttpMethod = microflows.HttpMethodPost
	case ast.HttpMethodPut:
		httpConfig.HttpMethod = microflows.HttpMethodPut
	case ast.HttpMethodPatch:
		httpConfig.HttpMethod = microflows.HttpMethodPatch
	case ast.HttpMethodDelete:
		httpConfig.HttpMethod = microflows.HttpMethodDelete
	default:
		httpConfig.HttpMethod = microflows.HttpMethodGet
	}

	// Set URL template
	if lit, ok := s.URL.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		httpConfig.LocationTemplate = fmt.Sprintf("%v", lit.Value)
	} else {
		httpConfig.LocationTemplate = fb.exprToString(s.URL)
	}

	// Set URL template parameters
	for _, param := range s.URLParams {
		httpConfig.LocationParams = append(httpConfig.LocationParams, fb.exprToString(param.Value))
	}

	// Set custom headers
	for _, header := range s.Headers {
		h := &microflows.HttpHeader{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Name:        header.Name,
			Value:       fb.exprToString(header.Value),
		}
		httpConfig.CustomHeaders = append(httpConfig.CustomHeaders, h)
	}

	// Set authentication
	if s.Auth != nil {
		httpConfig.UseAuthentication = true
		httpConfig.Username = fb.exprToString(s.Auth.Username)
		httpConfig.Password = fb.exprToString(s.Auth.Password)
	}

	// Build request handling
	var requestHandling microflows.RequestHandling
	if s.Body != nil {
		switch s.Body.Type {
		case ast.RestBodyCustom:
			// Custom body template
			var template string
			if lit, ok := s.Body.Template.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
				template = fmt.Sprintf("%v", lit.Value)
			} else {
				template = fb.exprToString(s.Body.Template)
			}
			// Extract template parameters
			var templateParams []string
			for _, param := range s.Body.TemplateParams {
				templateParams = append(templateParams, fb.exprToString(param.Value))
			}
			requestHandling = &microflows.CustomRequestHandling{
				BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
				Template:       template,
				TemplateParams: templateParams,
			}
		case ast.RestBodyMapping:
			// Export mapping
			mappingQN := s.Body.MappingName.Module + "." + s.Body.MappingName.Name
			requestHandling = &microflows.MappingRequestHandling{
				BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
				MappingID:         model.ID(mappingQN), // Use qualified name as ID for BY_NAME references
				ParameterVariable: s.Body.SourceVariable,
			}
		default:
			// No body
			requestHandling = &microflows.CustomRequestHandling{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Template:    "",
			}
		}
	} else {
		// Default: empty custom request handling
		requestHandling = &microflows.CustomRequestHandling{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Template:    "",
		}
	}

	// Build result handling
	var resultHandling microflows.ResultHandling
	switch s.Result.Type {
	case ast.RestResultString:
		resultHandling = &microflows.ResultHandlingString{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		}
	case ast.RestResultResponse:
		// Bind the full HTTP response object to the output variable. The writer
		// emits the matching `DataTypes$ObjectType` bound to System.HttpResponse;
		// the action-level `ResultHandlingType` is derived as "HttpResponse" from
		// this concrete type.
		resultHandling = &microflows.ResultHandlingHttpResponse{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			VariableName: s.OutputVariable,
		}
	case ast.RestResultMapping:
		mappingQN := s.Result.MappingName.Module + "." + s.Result.MappingName.Name
		entityQN := s.Result.ResultEntity.Module + "." + s.Result.ResultEntity.Name
		if s.OutputVariable == "" {
			// Derive a fallback output variable from the root entity only when the
			// MDL did not explicitly assign one.
			s.OutputVariable = s.Result.ResultEntity.Name
		}
		// Cardinality is authored on the microflow's ImportMappingCall in
		// BSON (Range.SingleObject + ForceSingleOccurrence) — the same
		// import mapping can yield either single or list depending on the
		// call site. The describer emits `as list of Module.Entity` for a
		// list and `as Module.Entity` for a single object; the builder
		// trusts that explicit choice. ForceSingleOccurrence mirrors
		// SingleObject so the writer reproduces the BSON shape Studio Pro
		// emits (Range and ForceSingleOccurrence agree on whether one
		// value is bound).
		singleObject := !s.Result.IsList
		fso := singleObject
		resultHandling = &microflows.ResultHandlingMapping{
			BaseElement:           model.BaseElement{ID: model.ID(types.GenerateID())},
			MappingID:             model.ID(mappingQN),
			ResultEntityID:        model.ID(entityQN),
			ResultVariable:        s.OutputVariable,
			SingleObject:          singleObject,
			ForceSingleOccurrence: &fso,
		}
	case ast.RestResultNone:
		resultHandling = &microflows.ResultHandlingNone{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		}
	default:
		resultHandling = &microflows.ResultHandlingString{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		}
	}

	// Build timeout expression
	var timeoutExpr string
	if s.Timeout != nil {
		timeoutExpr = fb.exprToString(s.Timeout)
	} else {
		timeoutExpr = "300" // Default 5 minutes
	}

	action := &microflows.RestCallAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		HttpConfiguration: httpConfig,
		RequestHandling:   requestHandling,
		ResultHandling:    resultHandling,
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
		OutputVariable:    s.OutputVariable,
		UseReturnVariable: s.OutputVariable != "",
		TimeoutExpression: timeoutExpr,
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

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.OutputVariable)

	return activity.ID
}

// addSendRestRequestAction creates a SEND REST REQUEST activity that calls
// a consumed REST service operation.
func (fb *flowBuilder) addSendRestRequestAction(s *ast.SendRestRequestStmt) model.ID {
	// Build operation reference: Module.Service.Operation
	operationQN := s.Operation.String()

	// Look up the operation definition to classify parameters and body kind.
	// s.Operation.Module = "MfTest", s.Operation.Name = "RC_TestApi.PostJsonTemplate"
	var opDef *model.RestClientOperation
	if fb.restServices != nil && s.Operation.Module != "" && strings.Contains(s.Operation.Name, ".") {
		dotIdx := strings.Index(s.Operation.Name, ".")
		serviceName := s.Operation.Name[:dotIdx]
		opName := s.Operation.Name[dotIdx+1:]
		opDef = lookupRestOperation(fb.restServices, serviceName, opName)
	}

	// Build OutputVariable
	var outputVar *microflows.RestOutputVar
	if s.OutputVariable != "" {
		outputVar = &microflows.RestOutputVar{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			VariableName: s.OutputVariable,
		}
	}

	// Build BodyVariable only for EXPORT_MAPPING body kind.
	// For JSON / TEMPLATE / FILE bodies, the body expression lives on the
	// operation definition itself and must NOT be set here (CE7067).
	var bodyVar *microflows.RestBodyVar
	if s.BodyVariable != "" && shouldSetBodyVariable(opDef) {
		bodyVar = &microflows.RestBodyVar{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			VariableName: s.BodyVariable,
		}
	}

	// Build parameter mappings, routing to ParameterMappings (path) or
	// QueryParameterMappings (query) based on the operation definition.
	paramMappings, queryParamMappings := buildRestParameterMappings(s.Parameters, opDef, operationQN)

	// RestOperationCallAction does not support custom error handling (CE6035).
	// ON ERROR clauses in the MDL are silently ignored for this action type.
	action := &microflows.RestOperationCallAction{
		BaseElement:            model.BaseElement{ID: model.ID(types.GenerateID())},
		Operation:              operationQN,
		OutputVariable:         outputVar,
		BodyVariable:           bodyVar,
		ParameterMappings:      paramMappings,
		QueryParameterMappings: queryParamMappings,
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

// lookupRestOperation finds a specific operation in a consumed REST service list.
func lookupRestOperation(services []*model.ConsumedRestService, serviceName, opName string) *model.RestClientOperation {
	for _, svc := range services {
		if svc.Name != serviceName {
			continue
		}
		for _, op := range svc.Operations {
			if op.Name == opName {
				return op
			}
		}
	}
	return nil
}

// shouldSetBodyVariable returns true if a BodyVariable BSON field should be
// emitted for a call to the given operation.
// For JSON, TEMPLATE, and FILE body kinds, the body expression lives on the
// operation definition and must not be overridden by a BodyVariable (CE7067).
// For EXPORT_MAPPING, the caller provides an entity to export via BodyVariable.
// When the operation definition is unknown (nil), we preserve old behaviour and
// set BodyVariable so the caller's intent is not silently dropped.
func shouldSetBodyVariable(op *model.RestClientOperation) bool {
	if op == nil {
		return true // unknown operation — preserve caller intent
	}
	switch op.BodyType {
	case "json", "template", "file":
		return false
	default:
		// EXPORT_MAPPING or empty (no body) — only set if EXPORT_MAPPING
		return op.BodyType == "EXPORT_MAPPING"
	}
}

// buildRestParameterMappings splits parameter bindings from a SEND REST REQUEST
// WITH clause into path parameter mappings and query parameter mappings,
// using the operation definition to determine which is which.
// When op is nil (operation not found), all parameters fall back to query
// parameter mappings (preserves old behaviour).
func buildRestParameterMappings(
	params []ast.SendRestParamDef,
	op *model.RestClientOperation,
	operationQN string,
) ([]*microflows.RestParameterMapping, []*microflows.RestQueryParameterMapping) {
	if len(params) == 0 {
		return nil, nil
	}

	// Build lookup sets from the operation definition.
	pathParamSet := map[string]bool{}
	if op != nil {
		for _, p := range op.Parameters {
			pathParamSet[p.Name] = true
		}
	}

	var pathMappings []*microflows.RestParameterMapping
	var queryMappings []*microflows.RestQueryParameterMapping

	for _, p := range params {
		if pathParamSet[p.Name] {
			pathMappings = append(pathMappings, &microflows.RestParameterMapping{
				Parameter: operationQN + "." + p.Name,
				Value:     p.Expression,
			})
		} else {
			queryMappings = append(queryMappings, &microflows.RestQueryParameterMapping{
				Parameter: operationQN + "." + p.Name,
				Value:     p.Expression,
				Included:  "Yes",
			})
		}
	}

	return pathMappings, queryMappings
}

// addExecuteDatabaseQueryAction creates an EXECUTE DATABASE QUERY statement.
func (fb *flowBuilder) addExecuteDatabaseQueryAction(s *ast.ExecuteDatabaseQueryStmt) model.ID {
	// DynamicQuery is a Mendix expression — string literals need single quotes
	dynamicQuery := s.DynamicQuery
	if dynamicQuery != "" && !strings.HasPrefix(dynamicQuery, "'") {
		dynamicQuery = "'" + strings.ReplaceAll(dynamicQuery, "'", "''") + "'"
	}

	action := &microflows.ExecuteDatabaseQueryAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  fb.ehType(s.ErrorHandling),
		OutputVariableName: s.OutputVariable,
		Query:              s.QueryName,
		DynamicQuery:       dynamicQuery,
	}

	// Build parameter mappings from arguments
	for _, arg := range s.Arguments {
		pm := &microflows.DatabaseQueryParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ParameterName: arg.Name,
			Value:         fb.exprToString(arg.Value),
		}
		action.ParameterMappings = append(action.ParameterMappings, pm)
	}

	// Build connection parameter mappings (runtime connection override)
	for _, arg := range s.ConnectionArguments {
		cm := &microflows.DatabaseConnectionParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ParameterName: arg.Name,
			Value:         fb.exprToString(arg.Value),
		}
		action.ConnectionParameterMappings = append(action.ConnectionParameterMappings, cm)
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

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.OutputVariable)

	return activity.ID
}

// addImportFromMappingAction adds an ImportXmlAction to the microflow.
func (fb *flowBuilder) addImportFromMappingAction(s *ast.ImportFromMappingStmt) model.ID {
	activityX := fb.posX

	action := &microflows.ImportXmlAction{
		BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		XmlDocumentVariable: s.SourceVariable,
	}

	resultHandling := &microflows.ResultHandlingMapping{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		MappingID:      model.ID(s.Mapping.String()),
		ResultVariable: s.OutputVariable,
		SingleObject:   true,
	}

	// Determine single vs list and result entity from the import mapping.
	// JSON structure check covers JSON-backed mappings; for XML schema or
	// message-definition mappings JsonStructure is empty and the root
	// element kind on the mapping itself indicates Array vs Object.
	resultEntityQN := ""
	if fb.backend != nil {
		if im, err := fb.backend.GetImportMappingByQualifiedName(s.Mapping.Module, s.Mapping.Name); err == nil {
			resolved := false
			if im.JsonStructure != "" {
				parts := strings.SplitN(im.JsonStructure, ".", 2)
				if len(parts) == 2 {
					if js, err := fb.backend.GetJsonStructureByQualifiedName(parts[0], parts[1]); err == nil && len(js.Elements) > 0 {
						if js.Elements[0].ElementType == "Array" {
							resultHandling.SingleObject = false
						}
						resolved = true
					}
				}
			}
			if !resolved && len(im.Elements) > 0 && im.Elements[0] != nil {
				// MaxOccurs > 1 or unbounded (-1) signals a list even when
				// the kind is Object.
				root := im.Elements[0]
				if root.MaxOccurs == -1 || root.MaxOccurs > 1 {
					resultHandling.SingleObject = false
				}
			}
			if len(im.Elements) > 0 && im.Elements[0] != nil && im.Elements[0].Entity != "" {
				resultEntityQN = im.Elements[0].Entity
				resultHandling.ResultEntityID = model.ID(resultEntityQN)
			}
		}
	}

	action.ResultHandling = resultHandling

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
	if s.OutputVariable != "" && resultEntityQN != "" && fb.varTypes != nil {
		if resultHandling.SingleObject {
			fb.varTypes[s.OutputVariable] = resultEntityQN
		} else {
			fb.varTypes[s.OutputVariable] = "List of " + resultEntityQN
		}
	}

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, s.OutputVariable)

	return activity.ID
}

// addTransformJsonAction adds a TransformJsonAction to the microflow.
func (fb *flowBuilder) addTransformJsonAction(s *ast.TransformJsonStmt) model.ID {
	activityX := fb.posX

	action := &microflows.TransformJsonAction{
		BaseElement:        model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:  fb.ehType(s.ErrorHandling),
		InputVariableName:  s.InputVariable,
		OutputVariableName: s.OutputVariable,
		Transformation:     s.Transformation.String(),
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

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

	return activity.ID
}

func (fb *flowBuilder) addExportToMappingAction(s *ast.ExportToMappingStmt) model.ID {
	activityX := fb.posX

	action := &microflows.ExportXmlAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
		OutputVariable:    s.OutputVariable,
		RequestHandling: &microflows.MappingRequestHandling{
			BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
			MappingID:         model.ID(s.Mapping.String()),
			ParameterVariable: s.SourceVariable,
		},
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

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

	return activity.ID
}
