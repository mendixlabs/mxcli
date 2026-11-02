// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow builder: call, control flow, and client actions
package executor

import (
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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

	// The same function `mxcli check` runs (MDL-TEMPLATE01): an expression
	// that also states `with (…)` has no template for the parameters to fill;
	// its source text used to become the template (ako/mxcli#1683).
	fb.refuseMessageTemplates(s)
	// MDL-TEMPLATE02: a template text whose braces mxbuild refuses (CE0712…,
	// ako/mxcli#1397). Same function `mxcli check` runs.
	fb.refuseTemplateGrammar(s)

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

	activityX := fb.posX

	action := &microflows.LogMessageAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// fb.ehType, not explicitErrorHandling — see ehType's doc comment.
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
		LogLevel:          logLevel,
		LogNodeName:       logNodeName,
		MessageTemplate: &model.Text{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Translations: map[string]string{
				fb.textLangOrDefault(): templateText,
			},
		},
		TemplateParameters:    templateParams,
		IncludeLastStackTrace: s.IncludeLatestStackTrace,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

	return activity.ID
}

// addCallMicroflowAction creates a CALL MICROFLOW statement.
// buildQueueSettings resolves an `IN QUEUE Module.Name` clause into the
// Queues$QueueSettings element that binds a call activity to a task queue.
//
// The queue must exist: mxbuild reports CE1613 ("The selected task queue … no
// longer exists") on the call activity otherwise, and that error names the
// activity rather than the script, which makes a typo here expensive to trace.
//
// `what` names the statement in the error (e.g. "CALL MICROFLOW").
func (fb *flowBuilder) buildQueueSettings(q *ast.QualifiedName, what string) *microflows.QueueSettings {
	if q == nil {
		return nil
	}
	qn := q.Module + "." + q.Name
	if !fb.queueExists(q.Module, q.Name) {
		fb.addError("%s ... IN QUEUE '%s': task queue not found in the project (create it with `create task queue %s (Parallelism: 1)`)", what, qn, qn)
	}
	return &microflows.QueueSettings{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Queue:       qn,
	}
}

// queueExists reports whether a Queues$Queue with the given module-qualified
// name is in the project. A backend that cannot answer is treated as "exists",
// matching microflowExists — this check exists to catch typos, not to be the
// last line of defence.
func (fb *flowBuilder) queueExists(moduleName, name string) bool {
	if fb.backend == nil {
		return true
	}
	queues, err := fb.backend.ListQueues()
	if err != nil {
		return true
	}
	for _, q := range queues {
		if !strings.EqualFold(q.Name, name) {
			continue
		}
		if fb.hierarchy == nil {
			return true
		}
		if strings.EqualFold(fb.hierarchy.GetModuleName(fb.hierarchy.FindModuleID(q.ContainerID)), moduleName) {
			return true
		}
	}
	return false
}

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
		QueueSettings:     fb.buildQueueSettings(s.Queue, "CALL MICROFLOW"),
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
		if err != nil && !fb.quiet {
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
	// A text template and a custom document take neither an expression nor
	// `empty` as Argument text; each has its own value (ako/mxcli#1451).
	templateParams := make(map[string]bool)
	documentParams := make(map[string]bool)
	if jaDef != nil {
		for _, p := range jaDef.Parameters {
			switch p.ParameterType.(type) {
			case *javaactions.EntityTypeParameterType:
				entityTypeParams[p.Name] = true
			case *javaactions.MicroflowType:
				microflowTypeParams[p.Name] = true
			case *javaactions.StringTemplateParameterType:
				templateParams[p.Name] = true
			case *javaactions.CustomBlobDocumentType:
				documentParams[p.Name] = true
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
		if len(arg.TemplateParams) > 0 && !templateParams[arg.Name] {
			if jaDef == nil {
				fb.addError("call java action %s: parameter %s: the action's parameters could not be read, "+
					"so `with (…)` cannot be written as a text template", actionQN, arg.Name)
			} else {
				fb.addError("%s", notTemplateParamRefusal(actionQN, arg.Name))
			}
		}
		if templateParams[arg.Name] {
			v, msg := fb.javaActionTemplateValue(actionQN, arg)
			if msg != "" {
				fb.addError("%s", msg)
			}
			value = v
		} else if documentParams[arg.Name] {
			v, msg := javaActionDocumentValue(actionQN, arg)
			if msg != "" {
				fb.addError("%s", msg)
			}
			value = v
		} else if entityTypeParams[arg.Name] {
			// Entity type parameter: value is the entity qualified name, not the variable reference.
			// When the argument is a variable like $Email, resolve its entity type from varTypes.
			value = &microflows.EntityTypeCodeActionParameterValue{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Entity:      fb.entityTypeArgument(arg.Value),
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
			if microflowTypeParams[arg.Name] {
				// A Microflow parameter takes a name, never an expression
				// (mendixlabs/mxcli#1210); the same decision reference
				// validation reports. The name is read without the whitespace
				// written after it (mendixlabs/mxcli#1282).
				if msg := microflowParamArgRefusal(actionQN, arg.Name, arg.Value); msg != "" {
					fb.addError("%s", msg)
				}
				value = &microflows.MicroflowParameterValue{
					BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					Microflow:   strings.Trim(fb.exprToString(javaActionArgumentValue(arg.Value)), "'"),
				}
			} else {
				value = &microflows.BasicCodeActionParameterValue{
					BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
					Argument:    fb.exprToString(arg.Value),
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
		QueueSettings:      fb.buildQueueSettings(s.Queue, "CALL JAVA ACTION"),
		ResultVariableName: s.OutputVariable,
		UseReturnVariable:  s.OutputVariable != "",
	}
	if s.OutputVariable != "" && fb.varTypes != nil {
		if varType := javaActionResultType(jaDef, javaActionArgVariables(s), fb.varTypes); varType != "" {
			fb.varTypes[s.OutputVariable] = varType
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

// javaActionResultType is the type a call of jaDef binds its result to,
// "Module.Entity" or "List of Module.Entity": the declared return type, or —
// for a list of a type parameter — the list the first of argVars that holds
// one holds (varTypes), argVars being the variables the call passes as they
// stand. "" when the result holds no entity or that is not known, and the
// caller records the declaration as Unknown. It reads the definition and the
// argument variables, not the statement, so a stored call's result can be
// typed the way the full build (addCallJavaActionAction) types it
// (ako/mxcli#1777).
func javaActionResultType(jaDef *javaactions.JavaAction, argVars []string, varTypes map[string]string) string {
	if jaDef == nil {
		return ""
	}
	if varType := javaActionReturnVarType(jaDef.ReturnType); varType != "" {
		return varType
	}
	if !returnsTypeParameterList(jaDef) {
		return ""
	}
	return firstListType(argVars, varTypes)
}

// returnsTypeParameterList reports whether jaDef returns a list of one of its
// type parameters, whose entity is the one a list argument holds. ListType is
// always stored as a pointer by the parser; there is no value form in the SDK.
func returnsTypeParameterList(jaDef *javaactions.JavaAction) bool {
	t, ok := jaDef.ReturnType.(*javaactions.ListType)
	return ok && t.Entity == ""
}

// firstListType is the type of the first of vars that holds a list, or "".
func firstListType(vars []string, varTypes map[string]string) string {
	for _, v := range vars {
		if typ := varTypes[v]; strings.HasPrefix(typ, "List of ") {
			return typ
		}
	}
	return ""
}

// javaActionArgVariables are the variables s passes as arguments as they
// stand (`P = $Lines`), in the order it states them.
func javaActionArgVariables(s *ast.CallJavaActionStmt) []string {
	var vars []string
	for _, arg := range s.Arguments {
		if varExpr, ok := arg.Value.(*ast.VariableExpr); ok {
			vars = append(vars, varExpr.Name)
		}
	}
	return vars
}

// addCallJavaScriptActionAction creates a CALL JAVASCRIPT ACTION statement.
func (fb *flowBuilder) addCallJavaScriptActionAction(s *ast.CallJavaScriptActionStmt) model.ID {
	actionQN := s.ActionName.Module + "." + s.ActionName.Name

	// Look up the JavaScript action definition to detect entity-type
	// parameters — the `entity <>` slots Studio Pro renders as an entity
	// picker (CodeActions$EntityTypeParameterType). Their value is the
	// chosen entity's qualified name under
	// Microflows$EntityTypeCodeActionParameterValue.Entity, NOT an
	// expression under BasicCodeActionParameterValue.Argument. Writing the
	// Basic shape leaves the picker empty in Studio Pro, and fails the build
	// with CE0115 "the arguments ... do not match the expected parameters"
	// (measured on mxbuild 11.6.6). What reports the nanoflow as correct is
	// mxcli itself: `mxcli check` passes, exec says "Created nanoflow", and
	// DESCRIBE renders the broken and the correct document as identical MDL
	// (mendixlabs/mxcli#1137). The Java-action builder above has drawn this
	// distinction since ako/mxcli#656; this path hardcoded Basic for every
	// parameter.
	//
	// Note this is NOT the same as a parameter typed to a concrete entity
	// (CodeActions$EntityType, e.g. NanoflowCommons.TakePicture's `Picture:
	// System.Image`): that one takes an object-valued expression and keeps
	// the Basic shape.
	//
	// Without a backend (mxcli check with no project) the signature cannot
	// be resolved, so the prior Basic shape stands rather than guessing the
	// parameter's kind from how the argument is spelled.
	entityTypeParams := make(map[string]bool)
	if fb.backend != nil {
		jsaDef, err := fb.backend.ReadJavaScriptActionByName(actionQN)
		if err != nil {
			if !fb.quiet {
				log.Printf("warning: could not look up JavaScript action %s: %v (entity type params will be empty)", actionQN, err)
			}
		} else if jsaDef != nil {
			for _, p := range jsaDef.Parameters {
				if _, ok := p.ParameterType.(*types.EntityTypeParameterType); ok {
					entityTypeParams[p.Name] = true
				}
			}
		}
	}

	// Build parameter mappings with Value structure
	var mappings []*microflows.JavaScriptActionParameterMapping
	for _, arg := range s.Arguments {
		// Parameter qualified name format: Module.JavaScriptAction.ParameterName
		paramQN := actionQN + "." + arg.Name

		var value microflows.CodeActionParameterValue
		valueExpr := fb.exprToString(arg.Value)
		if entityTypeParams[arg.Name] {
			value = &microflows.EntityTypeCodeActionParameterValue{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Entity:      fb.entityTypeArgument(arg.Value),
			}
		} else {
			value = &microflows.BasicCodeActionParameterValue{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Argument:    valueExpr,
			}
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

// entityTypeArgument returns the entity qualified name an entity-type
// (`entity <>`) code-action argument names. The value is stored as a bare
// name under EntityTypeCodeActionParameterValue.Entity, so it is NOT the
// argument's expression text: the visitor keeps an argument's trailing
// whitespace so expressions round-trip as written, and an argument on its own
// line therefore arrives as "Mod.Entity\n". Stored verbatim that names an
// entity that does not exist — CE0115 at build, with `mxcli check` clean
// (mendixlabs/mxcli#1171). A variable argument such as $Order resolves to the
// entity it holds.
func (fb *flowBuilder) entityTypeArgument(expr ast.Expression) string {
	entityName := strings.Trim(strings.TrimSpace(fb.exprToString(expr)), "'")
	if varName, ok := strings.CutPrefix(entityName, "$"); ok {
		if resolvedType, ok := fb.varTypes[varName]; ok {
			entityName = resolvedType
		}
	}
	return entityName
}

func isEmptyJavaActionArgument(expr ast.Expression) bool {
	lit, ok := javaActionArgumentValue(expr).(*ast.LiteralExpr)
	return ok && (lit.Kind == ast.LiteralEmpty || lit.Kind == ast.LiteralNull)
}

// addCallWebServiceAction creates a legacy SOAP WebServiceCallAction.
func (fb *flowBuilder) addCallWebServiceAction(s *ast.CallWebServiceStmt) model.ID {
	activityX := fb.posX
	// The same function `mxcli check` runs, so the two cannot drift on what a
	// valid request body is (MDL-SOAP01).
	if err := checkWebServiceRequestBodyStmt(s); err != nil {
		fb.addError("%v", err)
	}
	action := &microflows.WebServiceCallAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType: convertErrorHandlingType(s.ErrorHandling),
		ServiceID:         model.ID(s.ServiceID),
		// The WSDL service name, read off the imported service document. Empty
		// when it cannot be established (a dangling reference, a backend that
		// cannot list raw units), and the writers then derive it as before.
		ServiceName:   resolveWebServiceName(fb.backend, s.ServiceID, s.OperationName),
		OperationName: s.OperationName,
		// The QUALIFIED NAME, verbatim.
		//
		// Both are BY_NAME_REFERENCEs: the stored ImportMappingCall's
		// ReturnValueMapping is an ImportMappingIdentifier, and Studio Pro
		// writes "Clients.SoapOrdersImportMapping" there. These used to be
		// resolved to the mapping's unit `$ID`, which does not merely fail
		// validation — it makes the project impossible to LOAD. Measured on
		// 11.14.0 against ako/TestApp (baseline 0 errors): one
		// `receive mapping Clients.SoapOrdersImportMapping` written by mxcli
		// and `mx check` stops before validation with
		// StorageLoadException, "The text 'c2d1682f-…' is not a valid
		// ImportMappingIdentifier."
		//
		// Why it survived is worth more than the fix: the substitution only
		// happened when the lookup SUCCEEDED, and the only SOAP fixture
		// (06b-soap-examples.mdl) names mappings that do not exist, on
		// purpose, to show dangling refs. It took the fallback every time and
		// passed. The gate was green BECAUSE the fixture was broken — a valid
		// reference was the one input that triggered the defect, and nothing
		// tested one.
		SendMappingID: model.ID(s.SendMappingID),
		// The variable the export mapping maps FROM. Mendix stores it as
		// MappingRequestHandling.MappingVariableName, and a send mapping without
		// one cannot be written — refused above rather than written incomplete.
		SendMappingVariable: s.SendMappingVariable,
		ReceiveMappingID:    model.ID(s.ReceiveMappingID),
		// The entity the receive mapping produces, which types the result
		// variable. Empty when unresolvable, and the writers keep VoidType.
		ResultEntity:      resolveImportMappingEntity(fb.backend, s.ReceiveMappingID),
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
	action.Arguments = fb.webServiceArguments(s)

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
		errorY := fb.posY + ErrorHandlerPitch
		mergeID := fb.addErrorHandlerFlow(activity.ID, activityX, s.ErrorHandling.Body)
		fb.handleErrorHandlerMerge(mergeID, activity.ID, errorY)
	}

	return activity.ID
}

// webServiceArguments binds the statement's arguments to the stored
// ParameterPath each one needs.
//
// The path is derived from the OPERATION document rather than written by the
// author: Mendix stores
// `http%3A//www.example.com/:GetOrder|OrderId` where MDL says `OrderId`, and
// putting that in a script would fail every readability test the language is
// held to. The same move `send rest request` already makes — it stores each
// parameter under a qualified key and shows only the last segment.
//
// A path that cannot be derived is an ERROR, not a fallback. The other
// resolvers in this file fall back because their alternative is the value that
// ships today; there is no shipping value for a path that has never been
// written, and a fabricated one reproduces CE0178 with different text in it.
func (fb *flowBuilder) webServiceArguments(s *ast.CallWebServiceStmt) []microflows.WebServiceArgument {
	if len(s.Arguments) == 0 {
		return nil
	}
	element := resolveWebServiceOperationElement(fb.backend, s.ServiceID, s.OperationName)
	if element == "" {
		fb.addError("call web service %s: cannot resolve operation %s in the imported "+
			"service document, so the arguments have no parameter path to bind to.\n"+
			"  Arguments need the consumed service to be present and to declare the "+
			"operation — check the name against `describe microflow` on an existing call, "+
			"or drop the argument list",
			s.ServiceID, s.OperationName)
		return nil
	}

	out := make([]microflows.WebServiceArgument, 0, len(s.Arguments))
	for _, arg := range s.Arguments {
		path := webServiceParameterPath(element, arg.Name)
		if path == "" {
			fb.addError("call web service %s: cannot build the parameter path for %q "+
				"from operation element %q — a name containing '%%' is refused because "+
				"Mendix's escaping of it is unverified",
				s.ServiceID, arg.Name, element)
			return nil
		}
		out = append(out, microflows.WebServiceArgument{
			Name:       arg.Name,
			Path:       path,
			Expression: fb.exprToString(arg.Value),
			// Both reference mappings carry true, and Studio Pro's checkbox is
			// ticked for a parameter that is bound at all — which an argument is,
			// by being written.
			Checked: true,
		})
	}
	return out
}

// resolveExternalActionReturnKind looks up the called OData action in the
// consumed service's cached $metadata and returns the Mendix kind name
// ("Boolean", "String", "Integer", "Long", "Decimal", "DateTime", "Binary",
// "Object", "List" or "Void") of its return type, plus the qualified name of
// the external entity an Object/List is typed on. Used to populate
// CallExternalAction.ResultDataType/ResultEntity so the writer can emit
// VariableDataType BSON; without it Mendix raises CE7269 ("the return type for
// remote action '<x>' has changed") whenever the schema declares any return
// type.
//
// Returns "" if the service or action can't be resolved — the writer omits
// VariableDataType, falling back to the prior (buggy) behavior rather than
// emitting a wrong type.
func (fb *flowBuilder) resolveExternalActionReturnKind(serviceRef ast.QualifiedName, actionName string) (string, string) {
	if fb.backend == nil {
		return "", ""
	}
	services, err := fb.backend.ListConsumedODataServices()
	if err != nil {
		return "", ""
	}
	for _, svc := range services {
		modName := fb.hierarchy.GetModuleName(fb.hierarchy.FindModuleID(svc.ContainerID))
		if !strings.EqualFold(modName, serviceRef.Module) || !strings.EqualFold(svc.Name, serviceRef.Name) {
			continue
		}
		if svc.Metadata == "" {
			return "", ""
		}
		doc, err := types.ParseEdmx(svc.Metadata)
		if err != nil {
			return "", ""
		}
		for _, act := range doc.Actions {
			if !strings.EqualFold(act.Name, actionName) {
				continue
			}
			kind := edmReturnTypeToKind(act.ReturnType)
			if kind != "" {
				return kind, ""
			}
			// Not a primitive. An entity-typed return needs the local external
			// entity that was imported for it, because DataTypes$ObjectType and
			// DataTypes$ListType store an Entity by qualified name.
			return fb.resolveExternalActionReturnEntity(serviceRef, act.ReturnType)
		}
		return "", ""
	}
	return "", ""
}

// externalParamKind is one action parameter's resolved Mendix type, plus
// whether the contract lets the argument be left empty.
type externalParamKind struct {
	kind       string // "String", "Object", … — same vocabulary as the return type
	entity     string // set only for Object/List
	canBeEmpty bool   // the contract's Nullable, which Mendix stores as CanBeEmpty
}

// paramCanBeEmpty reads a parameter's nullability the way Mendix does.
//
// CSDL makes Nullable OPTIONAL on <Parameter> and defaults it to true, so an
// absent attribute means nullable — the opposite of Go's zero value. Getting
// this backwards is invisible in a contract that spells every Nullable out and
// only shows up on one that does not.
func paramCanBeEmpty(p *types.EdmActionParameter) bool {
	if p.Nullable == nil {
		return true
	}
	return *p.Nullable
}

// resolveExternalActionParameterKinds types every parameter of the called action
// from the cached contract, keyed by lower-cased parameter name.
//
// Returns an empty map when the service, contract or action cannot be resolved,
// in which case the writer omits ParameterType exactly as before — a wrong type
// is worse than the known-missing one.
func (fb *flowBuilder) resolveExternalActionParameterKinds(serviceRef ast.QualifiedName, actionName string) map[string]externalParamKind {
	out := map[string]externalParamKind{}
	if fb.backend == nil {
		return out
	}
	services, err := fb.backend.ListConsumedODataServices()
	if err != nil {
		return out
	}
	for _, svc := range services {
		modName := fb.hierarchy.GetModuleName(fb.hierarchy.FindModuleID(svc.ContainerID))
		if !strings.EqualFold(modName, serviceRef.Module) || !strings.EqualFold(svc.Name, serviceRef.Name) {
			continue
		}
		if svc.Metadata == "" {
			return out
		}
		doc, err := types.ParseEdmx(svc.Metadata)
		if err != nil {
			return out
		}
		for _, act := range doc.Actions {
			if !strings.EqualFold(act.Name, actionName) {
				continue
			}
			for _, p := range act.Parameters {
				// Nullability is recorded even when the type does not resolve:
				// CanBeEmpty is read straight off the contract and does not
				// depend on mxcli being able to name the Mendix type.
				pk := externalParamKind{canBeEmpty: paramCanBeEmpty(p)}
				if kind := edmReturnTypeToKind(p.Type); kind != "" && kind != "Void" {
					pk.kind = kind
				} else if kind, entity := fb.resolveExternalActionReturnEntity(serviceRef, p.Type); kind != "" {
					pk.kind, pk.entity = kind, entity
				}
				out[strings.ToLower(p.Name)] = pk
			}
			return out
		}
		return out
	}
	return out
}

// resolveExternalActionReturnEntity maps a non-primitive OData return type onto
// the external entity imported for it.
//
// The contract names the type in its own namespace (`Trippin.Person`, or
// `Collection(Trippin.Person)` for a list); the model names it by the entity
// mxcli created for that type, whose RemoteEntityName is the bare type name and
// whose RemoteServiceName is the consumed service. Matching on those two is the
// same linkage `CREATE OR MODIFY EXTERNAL ENTITIES` writes.
//
// Returns ("", "") when no entity has been imported for the type. That is the
// honest outcome: emitting an ObjectType with no Entity is as unaligned as
// emitting nothing, so the writer keeps omitting VariableDataType and
// ValidateExternalActionCalls reports the missing import at check time with the
// statement that fixes it.
func (fb *flowBuilder) resolveExternalActionReturnEntity(serviceRef ast.QualifiedName, returnType string) (string, string) {
	typeName, isList := edmBareTypeName(returnType)
	if typeName == "" {
		return "", ""
	}
	qn := fb.findExternalEntityFor(serviceRef.String(), typeName)
	if qn == "" {
		return "", ""
	}
	if isList {
		return "List", qn
	}
	return "Object", qn
}

// findExternalEntityFor finds the external entity imported from serviceQN for
// the remote type remoteName, returning its qualified name.
func (fb *flowBuilder) findExternalEntityFor(serviceQN, remoteName string) string {
	if fb.backend == nil {
		return ""
	}
	dms, err := fb.backend.ListDomainModels()
	if err != nil {
		return ""
	}
	for _, dm := range dms {
		modName := fb.hierarchy.GetModuleName(fb.hierarchy.FindModuleID(dm.ContainerID))
		for _, ent := range dm.Entities {
			if !strings.EqualFold(ent.RemoteServiceName, serviceQN) {
				continue
			}
			if strings.EqualFold(ent.RemoteEntityName, remoteName) {
				return modName + "." + ent.Name
			}
		}
	}
	return ""
}

// edmBareTypeName strips OData's Collection() wrapper and the type's namespace,
// reporting the bare type name and whether it was a collection.
//
// The namespace has to go: the contract says `Trippin.Person`, while the
// imported entity records `Person` as its RemoteEntityName. Returns "" for a
// primitive or an unparseable type, so only entity-typed returns reach the
// entity lookup.
func edmBareTypeName(edmType string) (string, bool) {
	t := strings.TrimSpace(edmType)
	isList := false
	if strings.HasPrefix(t, "Collection(") && strings.HasSuffix(t, ")") {
		t = strings.TrimSuffix(strings.TrimPrefix(t, "Collection("), ")")
		isList = true
	}
	t = strings.TrimSpace(t)
	if t == "" || strings.HasPrefix(t, "Edm.") {
		return "", isList
	}
	if i := strings.LastIndex(t, "."); i >= 0 {
		t = t[i+1:]
	}
	return t, isList
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
	case "Edm.DateTime", "Edm.DateTimeOffset", "Edm.Date", "Edm.TimeOfDay":
		// Edm.TimeOfDay is the one primitive Mendix accepts on an external
		// action that mxcli did not map, so the call was written with no type
		// at all and no MDL reached the field: CE7252 on a parameter, CE7269 on
		// a return (mendixlabs/mxcli#1089). Measured on mxbuild 11.12.0 —
		// typing it as DateTime clears both.
		return "DateTime"
	default:
		// Edm.Binary is deliberately absent, though it maps cleanly to
		// DataTypes$BinaryType: Mendix rejects an external action carrying one
		// as CE7255 "Action '<x>' ... is not supported", whatever mxcli writes.
		// Returning a kind here would route it past the refusal in
		// checkExternalActionTypes and back to the unfixable CE7252 of #1089.
		//
		// Not a primitive. Entity-typed and collection returns are resolved by
		// resolveExternalActionReturnEntity, which needs the project to map the
		// contract's type onto the entity imported for it. Complex types
		// (ComplexType, not EntityType) are still unmapped and end up here.
		return ""
	}
}

// externalActionContract returns the called service's cached contract and the
// action within it, or nils when either cannot be resolved. An unresolvable
// service or action is reported by the reference validation, not here.
func (fb *flowBuilder) externalActionContract(serviceRef ast.QualifiedName, actionName string) (*types.EdmxDocument, *types.EdmAction) {
	if fb.backend == nil {
		return nil, nil
	}
	services, err := fb.backend.ListConsumedODataServices()
	if err != nil {
		return nil, nil
	}
	for _, svc := range services {
		modName := fb.hierarchy.GetModuleName(fb.hierarchy.FindModuleID(svc.ContainerID))
		if !strings.EqualFold(modName, serviceRef.Module) || !strings.EqualFold(svc.Name, serviceRef.Name) {
			continue
		}
		if svc.Metadata == "" {
			return nil, nil
		}
		doc, err := types.ParseEdmx(svc.Metadata)
		if err != nil {
			return nil, nil
		}
		for _, act := range doc.Actions {
			if strings.EqualFold(act.Name, actionName) {
				return doc, act
			}
		}
		return doc, nil
	}
	return nil, nil
}

// refuseUntypableExternalAction applies the type rule at the WRITE choke point,
// from the same function `mxcli check --references` calls.
//
// Both tiers are needed and neither substitutes for the other: `exec` does not
// run the project-resolved reference pass, so without this the statement still
// executed silently and left a project whose build fails with CE7252 and no MDL
// to clear it — which is the whole of mendixlabs/mxcli#1089. This is the shape
// CheckLayoutPlaceholderNames already has: one rule, applied by the checker and
// by the writer.
func (fb *flowBuilder) refuseUntypableExternalAction(s *ast.CallExternalActionStmt) {
	doc, action := fb.externalActionContract(s.ServiceName, s.ActionName)
	if doc == nil || action == nil {
		return
	}
	svcQN := s.ServiceName.String()
	err := checkExternalActionTypes(externalCall{stmt: s}, action, doc, svcQN, func(remoteName string) string {
		return fb.findExternalEntityFor(svcQN, remoteName)
	})
	if err != nil {
		fb.addError("%s", err.Error())
	}
}

// addCallExternalActionAction creates a CALL EXTERNAL ACTION statement.
func (fb *flowBuilder) addCallExternalActionAction(s *ast.CallExternalActionStmt) model.ID {
	serviceQN := s.ServiceName.Module + "." + s.ServiceName.Name
	fb.refuseUntypableExternalAction(s)
	returnKind, returnEntity := fb.resolveExternalActionReturnKind(s.ServiceName, s.ActionName)

	// Build parameter mappings. Each carries the parameter's TYPE as well as its
	// name: generated/metamodel declares ParameterType without omitempty, and a
	// mapping without one is CE7252 plus a CE0117 per argument, because Mendix
	// cannot type-check an argument against an untyped parameter.
	paramKinds := fb.resolveExternalActionParameterKinds(s.ServiceName, s.ActionName)
	var mappings []*microflows.ExternalActionParameterMapping
	for _, arg := range s.Arguments {
		mapping := &microflows.ExternalActionParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ParameterName: arg.Name,
			Argument:      fb.exprToString(arg.Value),
		}
		if pk, ok := paramKinds[strings.ToLower(arg.Name)]; ok {
			mapping.ParameterDataType = pk.kind
			mapping.ParameterEntity = pk.entity
			// Mendix compares CanBeEmpty against the contract's Nullable and
			// raises CE7252 when they disagree, so leaving it at Go's false
			// makes every call on a nullable parameter unbuildable.
			mapping.CanBeEmpty = pk.canBeEmpty
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
		ResultDataType:       returnKind,
		ResultEntity:         returnEntity,
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
	activityX := fb.posX

	action := &microflows.ShowPageAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// fb.ehType, not explicitErrorHandling — see ehType's doc comment.
		ErrorHandlingType:     fb.ehType(s.ErrorHandling),
		PageName:              pageQN, // BY_NAME_REFERENCE - qualified name string
		PageSettings:          pageSettings,
		PageParameterMappings: mappings,
	}

	// Set passed object if FOR syntax was used
	if s.ForObject != "" {
		action.PassedObject = "$" + s.ForObject
	}

	// "Number of pages to close" (mendixlabs/mxcli#1617); none when unstated.
	if s.NumberOfPagesToClose != nil {
		action.NumberOfPagesToClose = fb.exprToString(s.NumberOfPagesToClose)
	}

	// Set title override if specified
	if s.Title != "" {
		action.OverridePageTitle = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{fb.textLangOrDefault(): s.Title},
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
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

	return activity.ID
}

// addShowHomePageAction creates a SHOW HOME PAGE statement.
func (fb *flowBuilder) addShowHomePageAction(s *ast.ShowHomePageStmt) model.ID {
	activityX := fb.posX
	action := &microflows.ShowHomePageAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// Explicit only: no clause keeps the writer's literal "Rollback".
		ErrorHandlingType: explicitErrorHandling(fb, s.ErrorHandling),
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

// addShowMessageAction creates a SHOW MESSAGE statement.
func (fb *flowBuilder) addShowMessageAction(s *ast.ShowMessageStmt) model.ID {
	// Build template text and parameters from message expression.
	// For string literals, use the raw value directly as template text.
	// For complex expressions, use {1} placeholder and add expression as parameter.
	var templateText string
	var templateParams []string

	// MDL-TEMPLATE01: an expression followed by `with (…)` stored '{1}' with
	// the expression and then the parameters, which no placeholder read
	// (ako/mxcli#1683).
	fb.refuseMessageTemplates(s)
	// MDL-TEMPLATE02: a template text whose braces mxbuild refuses (CE0712…,
	// ako/mxcli#1397). Same function `mxcli check` runs.
	fb.refuseTemplateGrammar(s)

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
		Translations: map[string]string{fb.textLangOrDefault(): templateText},
	}

	// The stored value is case-sensitive (mendixlabs/mxcli#1280).
	msgType, _ := microflows.CanonicalMessageType(s.Type)
	if msgType == "" {
		msgType = microflows.MessageTypeInformation
	}

	activityX := fb.posX

	action := &microflows.ShowMessageAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// fb.ehType, not explicitErrorHandling — see ehType's doc comment.
		ErrorHandlingType:  fb.ehType(s.ErrorHandling),
		Template:           template,
		Type:               msgType,
		TemplateParameters: templateParams,
		Blocking:           s.Blocking,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Position:    model.Point{X: fb.posX, Y: fb.posY},
				Size:        model.Size{Width: ActivityWidth, Height: ActivityHeight},
			},
			AutoGenerateCaption: true,
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

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

// addSynchronizeAction creates a SYNCHRONIZE activity (nanoflow-only).
func (fb *flowBuilder) addSynchronizeAction(s *ast.SynchronizeStmt) model.ID {
	action := &microflows.SynchronizeAction{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		SyncType:          microflows.SynchronizationType(s.SyncType),
		VariableNames:     s.Variables,
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	}
	if action.SyncType == "" {
		action.SyncType = microflows.SynchronizationTypeAll
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

	activityX := fb.posX
	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	// Wire the custom error handler's own branch. Without this the activity
	// stores CustomWithoutRollback with no outgoing error flow and mxbuild fails
	// CE0011 "If custom error handling is enabled there must be an outgoing
	// sequence flow set as error handler."
	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")
	return activity.ID
}

// addClosePageAction creates a CLOSE PAGE statement.
func (fb *flowBuilder) addClosePageAction(s *ast.ClosePageStmt) model.ID {
	numPages := s.NumberOfPages
	if numPages <= 0 {
		numPages = 1
	}

	activityX := fb.posX

	action := &microflows.ClosePageAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// fb.ehType, not explicitErrorHandling — see ehType's doc comment.
		ErrorHandlingType: fb.ehType(s.ErrorHandling),
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
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

	return activity.ID
}

// declaringMemberRef qualifies a member against the entity that DECLARES it,
// which is an ancestor when the attribute is inherited. Mendix stores member
// references that way, and qualifying against the entity that merely USES one is
// CE1613 "The selected attribute ... no longer exists" (upstream #974) — the same
// rule the access-rule writer learned in #758 and the mapping writer in #703.
//
// When nothing resolves — no backend, an unknown entity, an unknown member — the
// naive spelling is kept. The reference is wrong either way and mxbuild names it;
// substituting something else would only make the error harder to read.
func (fb *flowBuilder) declaringMemberRef(entityQN, memberName string) string {
	if entityQN == "" {
		return memberName
	}
	if fb.backend != nil {
		if ref, ok := DeclaringMemberRef(fb.backend, entityQN, memberName); ok {
			return ref
		}
	}
	return entityQN + "." + memberName
}

// bareFeedbackMember resolves a dotless validation-feedback member. An attribute
// in the generalization chain wins; failing that, an association the entity (or
// an ancestor) owns is stored as one. Neither keeps the old attribute spelling,
// which mxbuild names.
func (fb *flowBuilder) bareFeedbackMember(entityQN, memberName string) (attributeName, associationName string) {
	if entityQN != "" && fb.backend != nil {
		if _, ok := DeclaringMemberRef(fb.backend, entityQN, memberName); !ok {
			if ref, ok := DeclaringAssociationRef(fb.backend, entityQN, memberName); ok {
				return "", ref
			}
		}
	}
	return fb.declaringMemberRef(entityQN, memberName), ""
}

// addValidationFeedbackAction creates a VALIDATION FEEDBACK statement as a ValidationFeedbackAction.
func (fb *flowBuilder) addValidationFeedbackAction(s *ast.ValidationFeedbackStmt) model.ID {
	// Build the template text from the message expression.
	// For string literals, use the raw value (without quotes) since the template
	// text is plain text, not a microflow expression. For complex expressions,
	// use {1} placeholder with the expression as a parameter (same pattern as LogMessageAction).
	var templateText string
	var templateParams []string

	// MDL-TEMPLATE01, as show message (ako/mxcli#1683).
	fb.refuseMessageTemplates(s)
	// MDL-TEMPLATE02: a template text whose braces mxbuild refuses (CE0712…,
	// ako/mxcli#1397). Same function `mxcli check` runs.
	fb.refuseTemplateGrammar(s)

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
		Translations: map[string]string{fb.textLangOrDefault(): templateText},
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
				// Single bare segment: an attribute, or an association the
				// entity owns (#1322) — the dot count cannot tell them apart.
				attributeName, associationName = fb.bareFeedbackMember(entityQName, segs[0].Name)
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
			attributeName = fb.declaringMemberRef(entityQName, segs[0].Name)
		}
	} else if entityQName != "" && len(s.AttributePath.Path) > 0 {
		// Fallback for legacy Path without Segments
		attributeName = fb.declaringMemberRef(entityQName, s.AttributePath.Path[0])
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

	activityX := fb.posX

	action := &microflows.ValidationFeedbackAction{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		// fb.ehType, not explicitErrorHandling — see ehType's doc comment.
		ErrorHandlingType:  fb.ehType(s.ErrorHandling),
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
			ErrorHandlingType:   fb.ehType(s.ErrorHandling),
		},
		Action: action,
	}

	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing

	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")

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

	// The same function `mxcli check` runs (MDL-REST05): an expression that
	// also states `with (…)` has no template for the parameters to fill.
	if err := checkRestCallTemplatesStmt(s); err != nil {
		fb.addError("%v", err)
	}
	// MDL-TEMPLATE02: a URL or body text whose braces mxbuild refuses — a JSON
	// body's `{` is written `{{` (ako/mxcli#1397).
	fb.refuseTemplateGrammar(s)

	// Set URL template. An expression URL is the template '{1}' with the
	// expression as its parameter, not its own source text (ako/mxcli#1444).
	url, urlParams := restCallTemplate(s.URL, s.URLParams)
	if text, ok := restTemplateText(url); ok {
		httpConfig.LocationTemplate = text
	} else {
		httpConfig.LocationTemplate = fb.exprToString(url)
	}

	// Set URL template parameters
	for _, param := range urlParams {
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
			// Custom body template. A body written as an expression
			// (`Body: $Payload`) is the template '{1}' with the expression as
			// its parameter — what Studio Pro stores for a template that only
			// holds a variable. Storing the expression's source as the
			// template sent the text "$Payload" (ako/mxcli#1444).
			tmpl, params := restCallTemplate(s.Body.Template, s.Body.TemplateParams)
			var template string
			if text, ok := restTemplateText(tmpl); ok {
				template = text
			} else {
				template = fb.exprToString(tmpl)
			}
			// Extract template parameters
			var templateParams []string
			for _, param := range params {
				templateParams = append(templateParams, fb.exprToString(param.Value))
			}
			requestHandling = &microflows.CustomRequestHandling{
				BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
				Template:       template,
				TemplateParams: templateParams,
			}
		case ast.RestBodyBinary:
			// Binary body — the raw bytes the expression yields. Studio Pro stores
			// a FileDocument's Contents member here (`$Doc/Contents`), and the
			// action's RequestHandlingType becomes "Binary".
			requestHandling = &microflows.BinaryRequestHandling{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
				Expression:  fb.exprToString(s.Body.Template),
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
	case ast.RestResultFileDocument:
		// `returns Module.Entity` — store the response in a file document. The
		// entity is always a System.FileDocument specialization; the base type
		// is rejected by Mendix as a return type (CE0362), which checkRestCall
		// reports before the write. Issue #922.
		resultHandling = &microflows.ResultHandlingFileDocument{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID())},
			VariableName: s.OutputVariable,
			EntityRef:    s.Result.ResultEntity.String(),
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
		// BSON — the same import mapping can yield either single or list
		// depending on the call site. The describer emits `as list of
		// Module.Entity` for a list and `as Module.Entity` for a single
		// object; the builder trusts that explicit choice for the result
		// VARIABLE's type. The two BSON flags beside it are not the same
		// axis and are not authored here (issue #242):
		//
		//	ForceSingleOccurrence  "the mapping yields many, bind one" — true
		//	                       only when the mapping's ROOT is a list and
		//	                       the call site binds a single object
		//	Range.SingleObject     Studio Pro's First/All range. MDL's REST
		//	                       syntax has no range keyword, so it is All.
		//
		// Mirroring both onto SingleObject wrote ForceSingleOccurrence=true
		// for `as Entity` against an OBJECT-rooted mapping, which builds
		// cleanly (mxcli check, mx check and mxbuild all pass) and then
		// throws when the activity runs:
		//
		//	MicroflowException: key not found: Path(QName(None,),None,)
		//	  at ...importer.mapping.MappingCache.storeValueMappingElement
		//
		// Measured on 11.13.0 over all four flag combinations: the exception
		// tracks ForceSingleOccurrence alone — Range.SingleObject does not
		// affect it either way. Both are written to match the shapes Studio
		// Pro stores: PrivateCloudData.REST_GetEnvironmentByUUID (list-rooted
		// mapping, single object) keeps ForceSingleOccurrence=true, and all
		// three reference documents store Range.SingleObject=false.
		singleObject := !s.Result.IsList
		fso := false
		contentType := ""
		if fb.backend != nil {
			if im, err := fb.backend.GetImportMappingByQualifiedName(
				s.Result.MappingName.Module, s.Result.MappingName.Name); err == nil && im != nil {
				if singleObject {
					fso = fb.mappingRootIsList(im)
				}
				contentType = importMappingContentType(im)
			}
		}
		rangeAll := false
		resultHandling = &microflows.ResultHandlingMapping{
			BaseElement:           model.BaseElement{ID: model.ID(types.GenerateID())},
			MappingID:             model.ID(mappingQN),
			ResultEntityID:        model.ID(entityQN),
			ResultVariable:        s.OutputVariable,
			SingleObject:          singleObject,
			ForceSingleOccurrence: &fso,
			RangeSingleObject:     &rangeAll,
			ContentType:           contentType,
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

// dynamicQueryExpression renders the statement's dynamic query as the Mendix
// expression the action stores.
//
// A literal SQL string has to be quoted, because the field holds an expression —
// but an expression must be passed through untouched. Quoting `$Sql` sends the
// four characters "$Sql" to the database:
//
//	ERROR - ExternalDatabaseConnector: Parser Error: syntax error at or near "$"
//
// which blocks runtime-built SQL entirely. The two spellings are only
// distinguishable at parse time, hence DynamicQueryIsExpression.
func dynamicQueryExpression(s *ast.ExecuteDatabaseQueryStmt) string {
	q := s.DynamicQuery
	if q == "" || s.DynamicQueryIsExpression || strings.HasPrefix(q, "'") {
		return q
	}
	return "'" + strings.ReplaceAll(q, "'", "''") + "'"
}

// addExecuteDatabaseQueryAction creates an EXECUTE DATABASE QUERY statement.
func (fb *flowBuilder) addExecuteDatabaseQueryAction(s *ast.ExecuteDatabaseQueryStmt) model.ID {
	dynamicQuery := dynamicQueryExpression(s)

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

// mappingRootIsList reports whether the import mapping's ROOT yields many
// objects rather than one — the mapping's own shape, independent of what any
// call site binds it to.
//
// JSON-backed mappings answer from the structure's root element kind; XML-schema
// and message-definition mappings carry no JsonStructure, so the root mapping
// element's occurrence bound answers instead (unbounded is stored as -1).
// A mapping that cannot be resolved reads as object-rooted: that is the shape
// mxbuild rejects loudly with CE0243 when it is wrong, whereas guessing "list"
// writes ForceSingleOccurrence=true, which fails only once the activity runs.
func (fb *flowBuilder) mappingRootIsList(im *model.ImportMapping) bool {
	if im == nil {
		return false
	}
	var lookup jsonStructureLookup
	if fb.backend != nil {
		lookup = fb.backend
	}
	if list, known := jsonMappingRootIsList(lookup, im); known {
		return list
	}
	if len(im.Elements) > 0 && im.Elements[0] != nil {
		root := im.Elements[0]
		return root.MaxOccurs == -1 || root.MaxOccurs > 1
	}
	return false
}

// jsonStructureLookup is the one backend call deciding a JSON mapping's root
// shape needs.
type jsonStructureLookup = types.JsonStructureLookup

// jsonMappingRootIsList answers mappingRootIsList's question from the sources
// that are definite; see types.JsonMappingRootIsList, shared with the lint
// rules so both decide the shape the same way.
func jsonMappingRootIsList(lookup jsonStructureLookup, im *model.ImportMapping) (list, known bool) {
	return types.JsonMappingRootIsList(lookup, im)
}

// importMappingContentType is the content type an import mapping call takes
// for the mapping: XML when it is sourced from an XML schema or a web service
// operation, JSON otherwise — what Studio Pro sets when the activity is opened
// ("This action now imports XML"; mendixlabs/mxcli#1232).
func importMappingContentType(im *model.ImportMapping) string {
	if im.XmlSchema != "" || im.WebServiceSource.IsSet() {
		return "Xml"
	}
	return "Json"
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
		// The settings list (ako/mxcli#1681).
		ParameterVariable: s.Parameter,
		Commit:            s.Commit,
	}

	// The RANGE and the RESULT VARIABLE's type are two independent axes, and
	// conflating them is what made the first attempt at this emit CE0243 ("the
	// mapping used to return 'List of X' but now returns 'X'"):
	//
	//   Range          which part of the source is imported — the authored keyword
	//   SingleObject   whether the bound variable is an object or a list — the
	//                  MAPPING's own root cardinality, inferred below
	//
	// Mendix's own SUB_Feedback_PostToAppInsights proves they are separate: it
	// stores ConstantRange{SingleObject:false} (i.e. "all") against an ObjectType
	// variable, because its mapping is object-rooted. So ALL and LIMIT set only
	// the range and leave the inference alone; only FIRST also pins the variable
	// to a single object, which is exactly what "first" means. (issue #881)
	if s.All {
		f := false
		resultHandling.RangeSingleObject = &f
		resultHandling.ForceSingleOccurrence = &f
	} else if s.First {
		t := true
		resultHandling.SingleObject = true
		resultHandling.RangeSingleObject = &t
		resultHandling.ForceSingleOccurrence = &t
	} else if s.LimitExpr != nil || s.OffsetExpr != nil {
		f := false
		resultHandling.ForceSingleOccurrence = &f
		if s.LimitExpr != nil {
			resultHandling.LimitExpression = fb.exprToString(s.LimitExpr)
		}
		if s.OffsetExpr != nil {
			resultHandling.OffsetExpression = fb.exprToString(s.OffsetExpr)
		}
	} else {
		// No range keyword: the range is ALL, written explicitly. Leaving both
		// pointers nil let them fall back to SingleObject — true for an
		// object-rooted mapping — which stores Studio Pro's FIRST. That builds
		// cleanly (mx check: 0 errors) and then throws at import time:
		//
		//	MicroflowException: key not found: Path(QName(None,),None,)
		//	  at ...importer.mapping.MappingCache.storeValueMappingElement
		//
		// Studio Pro writes both flags false for a plain single-object import and
		// expresses "one object" solely through VariableType=ObjectType. Only the
		// RANGE is set here; the cardinality inference below is untouched.
		f := false
		resultHandling.RangeSingleObject = &f
		resultHandling.ForceSingleOccurrence = &f
	}
	// Only FIRST overrides the mapping's cardinality; saying nothing leaves both
	// axes to the inference, which is what keeps existing scripts writing what
	// they always did.
	rangeAuthored := s.First

	// Determine single vs list and result entity from the import mapping.
	// JSON structure check covers JSON-backed mappings; for XML schema or
	// message-definition mappings JsonStructure is empty and the root
	// element kind on the mapping itself indicates Array vs Object.
	//
	// A mapping that does not resolve is refused, not written around. The
	// result entity is stored as the activity's VariableType.Entity, and an
	// empty one makes the whole project unloadable — `mx check` dies with
	// ArgumentNullException in EntityType.set_EntityId before validating
	// anything (mendixlabs/mxcli#1418). The usual cause is a script that calls
	// the mapping above its `create import mapping`; MDL-ORDER01 reports that
	// at check time, this is the exec-time half.
	resultEntityQN := ""
	if fb.backend != nil {
		im, err := fb.backend.GetImportMappingByQualifiedName(s.Mapping.Module, s.Mapping.Name)
		switch {
		case err != nil || im == nil:
			fb.addError("import from mapping %s: import mapping not found in the project (create it above this microflow; an activity without its result entity makes the project unloadable)", s.Mapping.String())
		default:
			// The call's input object must match the mapping's declaration:
			// mxbuild refuses either mismatch, and check cannot see it without
			// the mapping (ako/mxcli#1681).
			switch {
			case im.ParameterEntity != "" && s.Parameter == "":
				fb.addError("import from mapping %s: the mapping takes an input object of %s; pass it with `(Parameter: $Obj)` (mxbuild refuses the call without one: CE0244)",
					s.Mapping.String(), im.ParameterEntity)
			case im.ParameterEntity == "" && s.Parameter != "":
				fb.addError("import from mapping %s: the mapping declares no input object, so `Parameter: $%s` has nothing to bind (mxbuild CE0246)",
					s.Mapping.String(), s.Parameter)
			}
			if !rangeAuthored && fb.mappingRootIsList(im) {
				resultHandling.SingleObject = false
			}
			resultHandling.ContentType = importMappingContentType(im)
			if len(im.Elements) > 0 && im.Elements[0] != nil && im.Elements[0].Entity != "" {
				resultEntityQN = im.Elements[0].Entity
				resultHandling.ResultEntityID = model.ID(resultEntityQN)
			} else {
				fb.addError("import from mapping %s: the import mapping has no root entity, so the activity's result type cannot be written (an empty one makes the project unloadable)", s.Mapping.String())
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

// mappingRootPathCrossesArray reports whether a mapping root's stored JsonPath
// passes through an array; see types.MappingRootPathCrossesArray.
func mappingRootPathCrossesArray(jsonPath string) bool {
	return types.MappingRootPathCrossesArray(jsonPath)
}

// addSendEmailAction creates a SEND EMAIL activity (Microflows$SendEmailAction).
func (fb *flowBuilder) addSendEmailAction(s *ast.SendEmailStmt) model.ID {
	// MDL-TEMPLATE01: a subject or body written as an expression with
	// `with (…)` (ako/mxcli#1683).
	fb.refuseMessageTemplates(s)
	// MDL-TEMPLATE02: a template text whose braces mxbuild refuses (CE0712…,
	// ako/mxcli#1397). Same function `mxcli check` runs.
	fb.refuseTemplateGrammar(s)
	action := &microflows.SendEmailAction{
		BaseElement:         model.BaseElement{ID: model.ID(types.GenerateID())},
		ErrorHandlingType:   microflows.ErrorHandlingTypeRollback,
		From:                fb.optExprToString(s.From),
		To:                  fb.optExprToString(s.To),
		Cc:                  fb.optExprToString(s.Cc),
		Bcc:                 fb.optExprToString(s.Bcc),
		Subject:             fb.emailTemplate(&s.Subject),
		BodyPlainText:       fb.emailTemplate(s.BodyText),
		BodyHTML:            fb.emailTemplate(s.BodyHTML),
		Attachment:          s.Attachment,
		Host:                fb.optExprToString(s.Host),
		Port:                fb.optExprToString(s.Port),
		SecurityType:        microflows.EmailSecurityTLS,
		CheckServerIdentity: s.CheckServerIdentity,
		ConnectionTimeout:   microflows.DefaultEmailConnectionTimeout,
	}
	if s.Security != "" {
		sec, ok := emailSecurityType(s.Security)
		if !ok {
			fb.addError("send email: unknown security mode %q — use none, ssl or tls", s.Security)
		}
		action.SecurityType = sec
	}
	if s.Timeout > 0 {
		action.ConnectionTimeout = s.Timeout
	}
	for _, h := range s.Headers {
		action.CustomHeaders = append(action.CustomHeaders, microflows.EmailCustomHeader{Name: h.Name, Value: h.Value})
	}
	if s.Auth != nil {
		action.UseAuthentication = true
		action.Username = fb.optExprToString(s.Auth.Username)
		action.Password = fb.optExprToString(s.Auth.Password)
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

	activityX := fb.posX
	fb.objects = append(fb.objects, activity)
	fb.posX += fb.spacing
	fb.finishCustomErrorHandler(activity.ID, activityX, s.ErrorHandling, "")
	return activity.ID
}

// emailSecurityType maps the MDL security word to the stored enum value.
func emailSecurityType(word string) (microflows.EmailSecurityType, bool) {
	switch strings.ToLower(word) {
	case "none":
		return microflows.EmailSecurityNone, true
	case "ssl":
		return microflows.EmailSecuritySSL, true
	case "tls":
		return microflows.EmailSecurityTLS, true
	}
	return microflows.EmailSecurityTLS, false
}

// emailTemplate turns a subject/body clause into a StringTemplate: a string
// literal is the template text and the `with` parameters fill its {n}
// placeholders; any other expression becomes `{1}` with the expression as its
// one parameter — the same rule `log` applies to its message. A nil clause is
// an empty template, which is what Studio Pro stores for an unused body.
func (fb *flowBuilder) emailTemplate(c *ast.EmailTemplateClause) microflows.EmailTemplate {
	if c == nil || c.Text == nil {
		return microflows.EmailTemplate{}
	}
	lit, isLit := c.Text.(*ast.LiteralExpr)
	if !isLit || lit.Kind != ast.LiteralString {
		if len(c.Params) == 0 {
			return microflows.EmailTemplate{Text: "{1}", Parameters: []string{fb.exprToString(c.Text)}}
		}
		// With parameters the text must be a string: a literal kept with its
		// source because it spans lines is still one, stored as its value —
		// its source, quotes and all, was the template. Any other expression
		// is refused (MDL-TEMPLATE01, ako/mxcli#1683); its source text used to
		// be stored as the template.
		text, _ := restTemplateText(c.Text)
		return microflows.EmailTemplate{Text: text, Parameters: fb.templateParamStrings(c.Params)}
	}
	return microflows.EmailTemplate{Text: fmt.Sprintf("%v", lit.Value), Parameters: fb.templateParamStrings(c.Params)}
}

// templateParamStrings orders `with ({n} = …)` parameters by placeholder index.
func (fb *flowBuilder) templateParamStrings(params []ast.TemplateParam) []string {
	maxIndex := 0
	for _, p := range params {
		if p.Index > maxIndex {
			maxIndex = p.Index
		}
	}
	if maxIndex == 0 {
		return nil
	}
	out := make([]string, maxIndex)
	for _, p := range params {
		if p.Index > 0 {
			out[p.Index-1] = fb.exprToString(p.Value)
		}
	}
	return out
}

// optExprToString renders an optional expression; nil is the empty string.
func (fb *flowBuilder) optExprToString(e ast.Expression) string {
	if e == nil {
		return ""
	}
	return fb.exprToString(e)
}
