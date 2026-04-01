// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow builder: call, control flow, and client actions
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/mpr"
)

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
		templateText = expressionToString(s.Message)
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
				templateParams[p.Index-1] = expressionToString(p.Value)
			}
		}
	} else if lit, ok := s.Message.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		// Simple string literal - use directly as template
		templateText = fmt.Sprintf("%v", lit.Value)
	} else {
		// Complex expression - use {1} placeholder and add expression as parameter
		templateText = "{1}"
		templateParams = []string{expressionToString(s.Message)}
	}

	action := &microflows.LogMessageAction{
		BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
		LogLevel:    logLevel,
		LogNodeName: "'" + s.Node + "'", // Store as expression (e.g., 'TEST')
		MessageTemplate: &model.Text{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
			Translations: map[string]string{
				"en_US": templateText,
			},
		},
		TemplateParameters: templateParams,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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

	// Build parameter mappings for MicroflowCall
	var mappings []*microflows.MicroflowCallParameterMapping
	for _, arg := range s.Arguments {
		// Parameter is the full qualified name: Module.Microflow.ParameterName
		paramQN := mfQN + "." + arg.Name
		mapping := &microflows.MicroflowCallParameterMapping{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
			Parameter:   paramQN,
			Argument:    expressionToString(arg.Value),
		}
		mappings = append(mappings, mapping)
	}

	// Create nested MicroflowCall structure
	mfCall := &microflows.MicroflowCall{
		BaseElement:       model.BaseElement{ID: model.ID(mpr.GenerateID())},
		Microflow:         mfQN,
		ParameterMappings: mappings,
	}

	action := &microflows.MicroflowCallAction{
		BaseElement:        model.BaseElement{ID: model.ID(mpr.GenerateID())},
		ErrorHandlingType:  convertErrorHandlingType(s.ErrorHandling),
		MicroflowCall:      mfCall,
		ResultVariableName: s.OutputVariable,
		UseReturnVariable:  s.OutputVariable != "",
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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

// addCallJavaActionAction creates a CALL JAVA ACTION statement.
func (fb *flowBuilder) addCallJavaActionAction(s *ast.CallJavaActionStmt) model.ID {
	actionQN := s.ActionName.Module + "." + s.ActionName.Name

	// Try to look up the Java action definition to detect EntityTypeParameterType parameters
	var jaDef *javaactions.JavaAction
	if fb.reader != nil {
		jaDef, _ = fb.reader.ReadJavaActionByName(actionQN)
	}

	// Build a map of parameter name -> param type for the Java action
	entityTypeParams := make(map[string]bool)
	if jaDef != nil {
		for _, p := range jaDef.Parameters {
			if _, ok := p.ParameterType.(*javaactions.EntityTypeParameterType); ok {
				entityTypeParams[p.Name] = true
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
			valueExpr := expressionToString(arg.Value)
			entityName := strings.Trim(valueExpr, "'")
			if strings.HasPrefix(entityName, "$") {
				varName := strings.TrimPrefix(entityName, "$")
				if resolvedType, ok := fb.varTypes[varName]; ok {
					entityName = resolvedType
				}
			}
			value = &microflows.EntityTypeCodeActionParameterValue{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
				Entity:      entityName,
			}
		} else {
			// Regular parameter: expression-based value
			valueExpr := expressionToString(arg.Value)
			value = &microflows.BasicCodeActionParameterValue{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
				Argument:    valueExpr,
			}
		}

		mapping := &microflows.JavaActionParameterMapping{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
			Parameter:   paramQN,
			Value:       value,
		}
		mappings = append(mappings, mapping)
	}

	action := &microflows.JavaActionCallAction{
		BaseElement:        model.BaseElement{ID: model.ID(mpr.GenerateID())},
		ErrorHandlingType:  convertErrorHandlingType(s.ErrorHandling),
		JavaAction:         actionQN,
		ParameterMappings:  mappings,
		ResultVariableName: s.OutputVariable,
		UseReturnVariable:  s.OutputVariable != "",
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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

// addCallExternalActionAction creates a CALL EXTERNAL ACTION statement.
func (fb *flowBuilder) addCallExternalActionAction(s *ast.CallExternalActionStmt) model.ID {
	serviceQN := s.ServiceName.Module + "." + s.ServiceName.Name

	// Build parameter mappings
	var mappings []*microflows.ExternalActionParameterMapping
	for _, arg := range s.Arguments {
		mapping := &microflows.ExternalActionParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(mpr.GenerateID())},
			ParameterName: arg.Name,
			Argument:      expressionToString(arg.Value),
		}
		mappings = append(mappings, mapping)
	}

	action := &microflows.CallExternalAction{
		BaseElement:          model.BaseElement{ID: model.ID(mpr.GenerateID())},
		ErrorHandlingType:    convertErrorHandlingType(s.ErrorHandling),
		ConsumedODataService: serviceQN,
		Name:                 s.ActionName,
		ParameterMappings:    mappings,
		ResultVariableName:   s.OutputVariable,
		UseReturnVariable:    s.OutputVariable != "",
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
			Parameter:   paramQN,
			Argument:    expressionToString(arg.Value),
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
		BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
		Location:    location,
		ModalForm:   s.ModalForm,
	}

	// Create the action
	// Use PageName (BY_NAME_REFERENCE) instead of PageID (BY_ID_REFERENCE)
	// The modern Mendix format uses FormSettings.Form as a qualified name string
	action := &microflows.ShowPageAction{
		BaseElement:           model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
				ID:       model.ID(mpr.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": s.Title},
		}
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
		BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
		templateParams = []string{expressionToString(s.Message)}
	}

	// Append template parameters from TemplateArgs (e.g., OBJECTS [$Var1, $Var2])
	for _, arg := range s.TemplateArgs {
		templateParams = append(templateParams, expressionToString(arg))
	}

	template := &model.Text{
		BaseElement:  model.BaseElement{ID: model.ID(mpr.GenerateID())},
		Translations: map[string]string{"en_US": templateText},
	}

	msgType := microflows.MessageType(s.Type)
	if msgType == "" {
		msgType = microflows.MessageTypeInformation
	}

	action := &microflows.ShowMessageAction{
		BaseElement:        model.BaseElement{ID: model.ID(mpr.GenerateID())},
		Template:           template,
		Type:               msgType,
		TemplateParameters: templateParams,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
		BaseElement:   model.BaseElement{ID: model.ID(mpr.GenerateID())},
		NumberOfPages: numPages,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
		templateParams = []string{expressionToString(s.Message)}
	}

	// Create template with translations map (default language "en_US")
	template := &model.Text{
		BaseElement:  model.BaseElement{ID: model.ID(mpr.GenerateID())},
		Translations: map[string]string{"en_US": templateText},
	}

	// Build attribute or association name from variable type and attribute path.
	// Single segment with /: attribute access ($Product/Code → "Module.Entity.Code")
	// Two segments where first uses / and second uses .: association traversal
	//   ($Instructor/Module.Association → AssociationName = "Module.Association")
	//   The grammar splits "Module.Association" into two segments: {Module, /} and {Association, .}
	var attributeName string
	var associationName string
	if entityQName, ok := fb.varTypes[s.AttributePath.Variable]; ok && len(s.AttributePath.Segments) > 0 {
		segs := s.AttributePath.Segments
		if len(segs) == 1 {
			// Single segment: direct attribute access
			attributeName = entityQName + "." + segs[0].Name
		} else if len(segs) >= 2 && segs[0].Separator == "/" && segs[1].Separator == "." {
			// Two+ segments starting with / then .: association qualified name
			// Reconstruct "Module.AssociationName" from segments
			parts := make([]string, len(segs))
			for i, seg := range segs {
				parts[i] = seg.Name
			}
			associationName = strings.Join(parts, ".")
		} else {
			// Fallback: treat first segment as attribute
			attributeName = entityQName + "." + segs[0].Name
		}
	} else if entityQName, ok := fb.varTypes[s.AttributePath.Variable]; ok && len(s.AttributePath.Path) > 0 {
		// Fallback for legacy Path without Segments
		attributeName = entityQName + "." + s.AttributePath.Path[0]
	}

	// Append template parameters from TemplateArgs (e.g., OBJECTS [$Var1, $Var2])
	for _, arg := range s.TemplateArgs {
		templateParams = append(templateParams, expressionToString(arg))
	}

	// Strip the $ prefix from variable name for BSON storage
	varName := s.AttributePath.Variable
	if strings.HasPrefix(varName, "$") {
		varName = varName[1:]
	}

	action := &microflows.ValidationFeedbackAction{
		BaseElement:        model.BaseElement{ID: model.ID(mpr.GenerateID())},
		ObjectVariable:     varName,
		AttributeName:      attributeName,
		AssociationName:    associationName,
		Template:           template,
		TemplateParameters: templateParams,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
		BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
		httpConfig.LocationTemplate = expressionToString(s.URL)
	}

	// Set URL template parameters
	for _, param := range s.URLParams {
		httpConfig.LocationParams = append(httpConfig.LocationParams, expressionToString(param.Value))
	}

	// Set custom headers
	for _, header := range s.Headers {
		h := &microflows.HttpHeader{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
			Name:        header.Name,
			Value:       expressionToString(header.Value),
		}
		httpConfig.CustomHeaders = append(httpConfig.CustomHeaders, h)
	}

	// Set authentication
	if s.Auth != nil {
		httpConfig.UseAuthentication = true
		httpConfig.Username = expressionToString(s.Auth.Username)
		httpConfig.Password = expressionToString(s.Auth.Password)
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
				template = expressionToString(s.Body.Template)
			}
			// Extract template parameters
			var templateParams []string
			for _, param := range s.Body.TemplateParams {
				templateParams = append(templateParams, expressionToString(param.Value))
			}
			requestHandling = &microflows.CustomRequestHandling{
				BaseElement:    model.BaseElement{ID: model.ID(mpr.GenerateID())},
				Template:       template,
				TemplateParams: templateParams,
			}
		case ast.RestBodyMapping:
			// Export mapping
			mappingQN := s.Body.MappingName.Module + "." + s.Body.MappingName.Name
			requestHandling = &microflows.MappingRequestHandling{
				BaseElement:       model.BaseElement{ID: model.ID(mpr.GenerateID())},
				MappingID:         model.ID(mappingQN), // Use qualified name as ID for BY_NAME references
				ParameterVariable: s.Body.SourceVariable,
			}
		default:
			// No body
			requestHandling = &microflows.CustomRequestHandling{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
				Template:    "",
			}
		}
	} else {
		// Default: empty custom request handling
		requestHandling = &microflows.CustomRequestHandling{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
			Template:    "",
		}
	}

	// Build result handling
	var resultHandling microflows.ResultHandling
	switch s.Result.Type {
	case ast.RestResultString:
		resultHandling = &microflows.ResultHandlingString{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
		}
	case ast.RestResultResponse:
		resultHandling = &microflows.ResultHandlingString{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
		}
		// Note: For HttpResponse, we would need a different result type, but using String for now
	case ast.RestResultMapping:
		mappingQN := s.Result.MappingName.Module + "." + s.Result.MappingName.Name
		entityQN := s.Result.ResultEntity.Module + "." + s.Result.ResultEntity.Name
		// Derive the output variable name from the root entity's short name so
		// callers don't need to hard-code it in the MDL assignment.
		s.OutputVariable = s.Result.ResultEntity.Name
		// Determine whether the import mapping returns a single object or a list by
		// looking at the JSON structure it references. If the root JSON element is
		// an Object, the mapping produces one object; if it is an Array, a list.
		singleObject := false
		if fb.reader != nil {
			if im, err := fb.reader.GetImportMappingByQualifiedName(s.Result.MappingName.Module, s.Result.MappingName.Name); err == nil && im.JsonStructure != "" {
				// im.JsonStructure is "Module.Name" — split and look up the JSON structure.
				if parts := strings.SplitN(im.JsonStructure, ".", 2); len(parts) == 2 {
					if js, err := fb.reader.GetJsonStructureByQualifiedName(parts[0], parts[1]); err == nil && len(js.Elements) > 0 {
						singleObject = js.Elements[0].ElementType == "Object"
					}
				}
			}
		}
		resultHandling = &microflows.ResultHandlingMapping{
			BaseElement:    model.BaseElement{ID: model.ID(mpr.GenerateID())},
			MappingID:      model.ID(mappingQN),
			ResultEntityID: model.ID(entityQN),
			ResultVariable: s.OutputVariable,
			SingleObject:   singleObject,
		}
	case ast.RestResultNone:
		resultHandling = &microflows.ResultHandlingNone{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
		}
	default:
		resultHandling = &microflows.ResultHandlingString{
			BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
		}
	}

	// Build timeout expression
	var timeoutExpr string
	if s.Timeout != nil {
		timeoutExpr = expressionToString(s.Timeout)
	} else {
		timeoutExpr = "300" // Default 5 minutes
	}

	action := &microflows.RestCallAction{
		BaseElement:       model.BaseElement{ID: model.ID(mpr.GenerateID())},
		HttpConfiguration: httpConfig,
		RequestHandling:   requestHandling,
		ResultHandling:    resultHandling,
		ErrorHandlingType: convertErrorHandlingType(s.ErrorHandling),
		OutputVariable:    s.OutputVariable,
		UseReturnVariable: s.OutputVariable != "",
		TimeoutExpression: timeoutExpr,
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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

// addSendRestRequestAction creates a SEND REST REQUEST activity that calls
// a consumed REST service operation.
func (fb *flowBuilder) addSendRestRequestAction(s *ast.SendRestRequestStmt) model.ID {
	// Build operation reference: Module.Service.Operation
	operationQN := s.Operation.String()

	// Build OutputVariable
	var outputVar *microflows.RestOutputVar
	if s.OutputVariable != "" {
		outputVar = &microflows.RestOutputVar{
			BaseElement:  model.BaseElement{ID: model.ID(mpr.GenerateID())},
			VariableName: s.OutputVariable,
		}
	}

	// Build BodyVariable
	var bodyVar *microflows.RestBodyVar
	if s.BodyVariable != "" {
		bodyVar = &microflows.RestBodyVar{
			BaseElement:  model.BaseElement{ID: model.ID(mpr.GenerateID())},
			VariableName: s.BodyVariable,
		}
	}

	// RestOperationCallAction does not support custom error handling (CE6035).
	// ON ERROR clauses in the MDL are silently ignored for this action type.
	action := &microflows.RestOperationCallAction{
		BaseElement:    model.BaseElement{ID: model.ID(mpr.GenerateID())},
		Operation:      operationQN,
		OutputVariable: outputVar,
		BodyVariable:   bodyVar,
	}

	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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

// addExecuteDatabaseQueryAction creates an EXECUTE DATABASE QUERY statement.
func (fb *flowBuilder) addExecuteDatabaseQueryAction(s *ast.ExecuteDatabaseQueryStmt) model.ID {
	// DynamicQuery is a Mendix expression — string literals need single quotes
	dynamicQuery := s.DynamicQuery
	if dynamicQuery != "" && !strings.HasPrefix(dynamicQuery, "'") {
		dynamicQuery = "'" + strings.ReplaceAll(dynamicQuery, "'", "''") + "'"
	}

	action := &microflows.ExecuteDatabaseQueryAction{
		BaseElement:        model.BaseElement{ID: model.ID(mpr.GenerateID())},
		ErrorHandlingType:  convertErrorHandlingType(s.ErrorHandling),
		OutputVariableName: s.OutputVariable,
		Query:              s.QueryName,
		DynamicQuery:       dynamicQuery,
	}

	// Build parameter mappings from arguments
	for _, arg := range s.Arguments {
		pm := &microflows.DatabaseQueryParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(mpr.GenerateID())},
			ParameterName: arg.Name,
			Value:         expressionToString(arg.Value),
		}
		action.ParameterMappings = append(action.ParameterMappings, pm)
	}

	// Build connection parameter mappings (runtime connection override)
	for _, arg := range s.ConnectionArguments {
		cm := &microflows.DatabaseConnectionParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(mpr.GenerateID())},
			ParameterName: arg.Name,
			Value:         expressionToString(arg.Value),
		}
		action.ConnectionParameterMappings = append(action.ConnectionParameterMappings, cm)
	}

	activityX := fb.posX
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(mpr.GenerateID())},
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
