// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func parseCallExternalAction(raw map[string]any) *microflows.CallExternalAction {
	action := &microflows.CallExternalAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.ConsumedODataService = extractString(raw["ConsumedODataService"])
	action.Name = extractString(raw["Name"])
	action.ResultVariableName = extractString(raw["VariableName"])
	action.UseReturnVariable = action.ResultVariableName != ""

	// Parse parameter mappings
	if mappings := extractBsonArray(raw["ParameterMappings"]); len(mappings) > 0 {
		for _, m := range mappings {
			if mMap, ok := m.(map[string]any); ok {
				mapping := &microflows.ExternalActionParameterMapping{}
				mapping.ID = model.ID(extractBsonID(mMap["$ID"]))
				mapping.ParameterName = extractString(mMap["ParameterName"])
				mapping.Argument = extractString(mMap["Argument"])
				mapping.CanBeEmpty = extractBool(mMap["CanBeEmpty"], false)
				action.ParameterMappings = append(action.ParameterMappings, mapping)
			}
		}
	}

	return action
}

func parseMicroflowCallAction(raw map[string]any) *microflows.MicroflowCallAction {
	action := &microflows.MicroflowCallAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.ResultVariableName = extractString(raw["ResultVariableName"])
	action.UseReturnVariable = extractBool(raw["UseReturnVariable"], false)

	// Parse nested MicroflowCall structure
	if mfCall, ok := raw["MicroflowCall"].(map[string]any); ok {
		call := &microflows.MicroflowCall{}
		call.ID = model.ID(extractBsonID(mfCall["$ID"]))
		call.Microflow = extractString(mfCall["Microflow"])

		// Parse parameter mappings from MicroflowCall (use extractBsonArray for BSON array format)
		if mappings := extractBsonArray(mfCall["ParameterMappings"]); len(mappings) > 0 {
			for _, m := range mappings {
				if mMap, ok := m.(map[string]any); ok {
					mapping := &microflows.MicroflowCallParameterMapping{}
					mapping.ID = model.ID(extractBsonID(mMap["$ID"]))
					mapping.Parameter = extractString(mMap["Parameter"])
					mapping.Argument = extractString(mMap["Argument"])
					call.ParameterMappings = append(call.ParameterMappings, mapping)
				}
			}
		}
		action.MicroflowCall = call
	}

	return action
}

func parseJavaActionCallAction(raw map[string]any) *microflows.JavaActionCallAction {
	action := &microflows.JavaActionCallAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.JavaAction = extractString(raw["JavaAction"])
	action.ResultVariableName = extractString(raw["ResultVariableName"])
	action.UseReturnVariable = extractBool(raw["UseReturnVariable"], false)

	// Parse parameter mappings (use extractBsonArray to handle BSON array format)
	if mappings := extractBsonArray(raw["ParameterMappings"]); len(mappings) > 0 {
		for _, m := range mappings {
			if mMap, ok := m.(map[string]any); ok {
				mapping := &microflows.JavaActionParameterMapping{}
				mapping.ID = model.ID(extractBsonID(mMap["$ID"]))
				mapping.Parameter = extractString(mMap["Parameter"])
				// Parse Value - it can be various types
				if value, ok := mMap["Value"].(map[string]any); ok {
					mapping.Value = parseCodeActionParameterValue(value)
				}
				action.ParameterMappings = append(action.ParameterMappings, mapping)
			}
		}
	}

	return action
}

func parseCodeActionParameterValue(raw map[string]any) microflows.CodeActionParameterValue {
	if raw == nil {
		return nil
	}
	typeName := extractString(raw["$Type"])
	switch typeName {
	case "Microflows$StringTemplateParameterValue":
		value := &microflows.StringTemplateParameterValue{}
		value.ID = model.ID(extractBsonID(raw["$ID"]))
		if tt, ok := raw["TypedTemplate"].(map[string]any); ok {
			value.TypedTemplate = &microflows.TypedTemplate{}
			value.TypedTemplate.ID = model.ID(extractBsonID(tt["$ID"]))
			value.TypedTemplate.Text = extractString(tt["Text"])
		}
		return value
	case "Microflows$ExpressionBasedCodeActionParameterValue":
		value := &microflows.ExpressionBasedCodeActionParameterValue{}
		value.ID = model.ID(extractBsonID(raw["$ID"]))
		value.Expression = extractString(raw["Expression"])
		return value
	case "Microflows$BasicCodeActionParameterValue":
		value := &microflows.BasicCodeActionParameterValue{}
		value.ID = model.ID(extractBsonID(raw["$ID"]))
		value.Argument = extractString(raw["Argument"])
		return value
	case "Microflows$EntityTypeCodeActionParameterValue":
		value := &microflows.EntityTypeCodeActionParameterValue{}
		value.ID = model.ID(extractBsonID(raw["$ID"]))
		value.Entity = extractString(raw["Entity"])
		return value
	}
	return nil
}

func parseShowPageAction(raw map[string]any) *microflows.ShowPageAction {
	action := &microflows.ShowPageAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.PageID = model.ID(extractBsonID(raw["Page"]))
	action.PassedObject = extractString(raw["PassedObjectVariableName"])

	// Parse FormSettings (modern Mendix 10+ format with BY_NAME_REFERENCE)
	if fs := toMap(raw["FormSettings"]); fs != nil {
		action.PageName = extractString(fs["Form"])
		action.FormSettingsID = model.ID(extractBsonID(fs["$ID"]))
		// Parse ParameterMappings from FormSettings
		action.PageParameterMappings = parseParameterMappingsAny(fs["ParameterMappings"])
	}

	// Parse PageSettings (legacy format)
	if ps := toMap(raw["PageSettings"]); ps != nil {
		action.PageSettings = &microflows.PageSettings{
			BaseElement: model.BaseElement{ID: model.ID(extractBsonID(ps["$ID"]))},
			Location:    microflows.PageLocation(extractString(ps["Location"])),
		}
	}

	// Parse PageParameterMappings from top-level (legacy format, only if not already parsed from FormSettings)
	if action.PageParameterMappings == nil {
		action.PageParameterMappings = parseParameterMappingsAny(raw["ParameterMappings"])
	}

	return action
}

// parseParameterMappingsAny parses parameter mappings from any array type (primitive.A or []any).
func parseParameterMappingsAny(v any) []*microflows.PageParameterMapping {
	arr := extractBsonArray(v)
	if len(arr) == 0 {
		return nil
	}
	var result []*microflows.PageParameterMapping
	for _, m := range arr {
		mMap := toMap(m)
		if mMap != nil {
			mapping := &microflows.PageParameterMapping{
				BaseElement: model.BaseElement{ID: model.ID(extractBsonID(mMap["$ID"]))},
				Parameter:   extractString(mMap["Parameter"]),
				Argument:    extractString(mMap["Argument"]),
			}
			result = append(result, mapping)
		}
	}
	return result
}

// parseFormParameterMappings parses parameter mappings from FormSettings (primitive.A type).
func parseFormParameterMappings(mappings primitive.A) []*microflows.PageParameterMapping {
	var result []*microflows.PageParameterMapping
	for _, m := range mappings {
		// Skip the count element
		if _, isInt := m.(int32); isInt {
			continue
		}
		mMap := toMap(m)
		if mMap != nil {
			mapping := &microflows.PageParameterMapping{
				BaseElement: model.BaseElement{ID: model.ID(extractBsonID(mMap["$ID"]))},
				Parameter:   extractString(mMap["Parameter"]),
				Argument:    extractString(mMap["Argument"]),
			}
			result = append(result, mapping)
		}
	}
	return result
}

// parseFormParameterMappingsSlice parses parameter mappings from FormSettings ([]interface{} type).
func parseFormParameterMappingsSlice(mappings []any) []*microflows.PageParameterMapping {
	var result []*microflows.PageParameterMapping
	for _, m := range mappings {
		// Skip the count element
		if _, isInt := m.(int32); isInt {
			continue
		}
		mMap := toMap(m)
		if mMap != nil {
			mapping := &microflows.PageParameterMapping{
				BaseElement: model.BaseElement{ID: model.ID(extractBsonID(mMap["$ID"]))},
				Parameter:   extractString(mMap["Parameter"]),
				Argument:    extractString(mMap["Argument"]),
			}
			result = append(result, mapping)
		}
	}
	return result
}

func parseShowHomePageAction(raw map[string]any) *microflows.ShowHomePageAction {
	action := &microflows.ShowHomePageAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	return action
}

func parseClosePageAction(raw map[string]any) *microflows.ClosePageAction {
	action := &microflows.ClosePageAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	if numPages, ok := raw["NumberOfPagesToClose"].(int32); ok {
		action.NumberOfPages = int(numPages)
	} else if numPages, ok := raw["NumberOfPagesToClose"].(int64); ok {
		action.NumberOfPages = int(numPages)
	} else {
		action.NumberOfPages = 1
	}
	return action
}

func parseShowMessageAction(raw map[string]any) *microflows.ShowMessageAction {
	action := &microflows.ShowMessageAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.Blocking = extractBool(raw["Blocking"], false)

	if msgType, ok := raw["Type"].(string); ok {
		action.Type = microflows.MessageType(msgType)
	}

	// Parse template (nested Microflows$TextTemplate -> Texts$Text)
	if template, ok := raw["Template"].(map[string]any); ok {
		// TextTemplate contains a nested Text property with the actual translations
		if text, ok := template["Text"].(map[string]any); ok {
			action.Template = parseText(text)
		}

		// Extract template parameters from Microflows$TextTemplate.Parameters
		if params := extractBsonArray(template["Parameters"]); len(params) > 0 {
			for _, p := range params {
				if paramMap, ok := p.(map[string]any); ok {
					if expr := extractString(paramMap["Expression"]); expr != "" {
						action.TemplateParameters = append(action.TemplateParameters, expr)
					}
				}
			}
		}
	}

	return action
}

func parseValidationFeedbackAction(raw map[string]any) *microflows.ValidationFeedbackAction {
	action := &microflows.ValidationFeedbackAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ObjectVariable = extractString(raw["ValidationVariableName"])
	action.AttributeName = extractString(raw["Attribute"])     // BY_NAME_REFERENCE
	action.AssociationName = extractString(raw["Association"]) // BY_NAME_REFERENCE

	// Parse template (nested Microflows$TextTemplate -> Texts$Text)
	if template, ok := raw["FeedbackTemplate"].(map[string]any); ok {
		// TextTemplate contains a nested Text property
		if text, ok := template["Text"].(map[string]any); ok {
			action.Template = parseText(text)
		}
	}

	return action
}

func parseDownloadFileAction(raw map[string]any) *microflows.DownloadFileAction {
	action := &microflows.DownloadFileAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.FileDocument = extractString(raw["FileDocumentVariableName"])
	action.ShowInBrowser = extractBool(raw["ShowInBrowser"], false)
	return action
}

func parseLogMessageAction(raw map[string]any) *microflows.LogMessageAction {
	action := &microflows.LogMessageAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.LogNodeName = extractString(raw["Node"])
	action.IncludeLastStackTrace = extractBool(raw["IncludeLatestStackTrace"], false)

	if level, ok := raw["Level"].(string); ok {
		action.LogLevel = microflows.LogLevel(level)
	}

	// Parse message template (Microflows$StringTemplate)
	if template, ok := raw["MessageTemplate"].(map[string]any); ok {
		action.MessageTemplate = parseText(template)

		// Extract template parameters from Microflows$StringTemplate.Parameters
		if params := extractBsonArray(template["Parameters"]); len(params) > 0 {
			for _, p := range params {
				if paramMap, ok := p.(map[string]any); ok {
					if expr := extractString(paramMap["Expression"]); expr != "" {
						action.TemplateParameters = append(action.TemplateParameters, expr)
					}
				}
			}
		}
	}

	return action
}

func parseCastAction(raw map[string]any) *microflows.CastAction {
	action := &microflows.CastAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ObjectVariable = extractString(raw["ObjectVariableName"])
	action.OutputVariable = extractString(raw["OutputVariableName"])
	return action
}

func parseRestCallAction(raw map[string]any) *microflows.RestCallAction {
	action := &microflows.RestCallAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.TimeoutExpression = extractString(raw["TimeOutExpression"])
	action.UseReturnVariable = extractBool(raw["UseRequestTimeOut"], false)
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))

	// Parse HttpConfiguration
	if httpConfig, ok := raw["HttpConfiguration"].(map[string]any); ok {
		action.HttpConfiguration = parseHttpConfiguration(httpConfig)
	} else if httpConfigD, ok := raw["HttpConfiguration"].(primitive.D); ok {
		action.HttpConfiguration = parseHttpConfiguration(httpConfigD.Map())
	}

	// Parse ResultHandling
	resultHandlingType := extractString(raw["ResultHandlingType"])
	if resultHandling, ok := raw["ResultHandling"].(map[string]any); ok {
		action.ResultHandling = parseResultHandling(resultHandling, resultHandlingType)
	} else if resultHandlingD, ok := raw["ResultHandling"].(primitive.D); ok {
		action.ResultHandling = parseResultHandling(resultHandlingD.Map(), resultHandlingType)
	}

	// Parse RequestHandling
	requestHandlingType := extractString(raw["RequestHandlingType"])
	if requestHandling, ok := raw["RequestHandling"].(map[string]any); ok {
		action.RequestHandling = parseRequestHandling(requestHandling, requestHandlingType)
	} else if requestHandlingD, ok := raw["RequestHandling"].(primitive.D); ok {
		action.RequestHandling = parseRequestHandling(requestHandlingD.Map(), requestHandlingType)
	}

	return action
}

// parseRestOperationCallAction parses a Microflows$RestOperationCallAction from BSON.
func parseRestOperationCallAction(raw map[string]any) *microflows.RestOperationCallAction {
	action := &microflows.RestOperationCallAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.Operation = extractString(raw["Operation"])

	// Parse OutputVariable (nested Microflows$OutputVariable)
	if ov := extractBsonMap(raw["OutputVariable"]); ov != nil {
		action.OutputVariable = &microflows.RestOutputVar{
			BaseElement:  model.BaseElement{ID: model.ID(extractBsonID(ov["$ID"]))},
			VariableName: extractString(ov["VariableName"]),
		}
	}

	// Parse BodyVariable (nested object)
	if bv := extractBsonMap(raw["BodyVariable"]); bv != nil {
		action.BodyVariable = &microflows.RestBodyVar{
			BaseElement:  model.BaseElement{ID: model.ID(extractBsonID(bv["$ID"]))},
			VariableName: extractString(bv["VariableName"]),
		}
	}

	return action
}

func parseHttpConfiguration(raw map[string]any) *microflows.HttpConfiguration {
	config := &microflows.HttpConfiguration{}
	config.ID = model.ID(extractBsonID(raw["$ID"]))
	config.HttpMethod = microflows.HttpMethod(extractString(raw["HttpMethod"]))
	config.CustomLocation = extractString(raw["CustomLocation"])
	config.UseAuthentication = extractBool(raw["UseHttpAuthentication"], false)
	config.Username = extractString(raw["HttpAuthenticationUserName"])
	config.Password = extractString(raw["HttpAuthenticationPassword"])

	// Parse CustomLocationTemplate (URL template with parameters)
	if locTemplate, ok := raw["CustomLocationTemplate"].(map[string]any); ok {
		config.LocationTemplate = extractString(locTemplate["Text"])
		config.LocationParams = parseTemplateParameters(locTemplate)
	} else if locTemplateD, ok := raw["CustomLocationTemplate"].(primitive.D); ok {
		locTemplateM := locTemplateD.Map()
		config.LocationTemplate = extractString(locTemplateM["Text"])
		config.LocationParams = parseTemplateParameters(locTemplateM)
	}

	// Parse HttpHeaderEntries
	if headers, ok := raw["HttpHeaderEntries"].(primitive.A); ok {
		for _, h := range headers {
			if hMap, ok := h.(primitive.D); ok {
				header := parseHttpHeader(hMap.Map())
				if header != nil {
					config.CustomHeaders = append(config.CustomHeaders, header)
				}
			} else if hMap, ok := h.(map[string]any); ok {
				header := parseHttpHeader(hMap)
				if header != nil {
					config.CustomHeaders = append(config.CustomHeaders, header)
				}
			}
		}
	}

	return config
}

func parseTemplateParameters(raw map[string]any) []string {
	var params []string
	if paramsArr, ok := raw["Parameters"].(primitive.A); ok {
		for _, p := range paramsArr {
			if pMap, ok := p.(primitive.D); ok {
				expr := extractString(pMap.Map()["Expression"])
				params = append(params, expr)
			} else if pMap, ok := p.(map[string]any); ok {
				expr := extractString(pMap["Expression"])
				params = append(params, expr)
			}
		}
	}
	return params
}

func parseHttpHeader(raw map[string]any) *microflows.HttpHeader {
	if raw == nil {
		return nil
	}
	return &microflows.HttpHeader{
		Name:  extractString(raw["Key"]),
		Value: extractString(raw["Value"]),
	}
}

func parseResultHandling(raw map[string]any, handlingType string) microflows.ResultHandling {
	switch handlingType {
	case "String":
		result := &microflows.ResultHandlingString{}
		result.ID = model.ID(extractBsonID(raw["$ID"]))
		result.VariableName = extractString(raw["ResultVariableName"])
		return result
	case "HttpResponse":
		result := &microflows.ResultHandlingHttpResponse{}
		result.ID = model.ID(extractBsonID(raw["$ID"]))
		result.VariableName = extractString(raw["ResultVariableName"])
		return result
	case "Mapping":
		result := &microflows.ResultHandlingMapping{}
		result.ID = model.ID(extractBsonID(raw["$ID"]))
		result.ResultVariable = extractString(raw["ResultVariableName"])
		if call := toMap(raw["ImportMappingCall"]); call != nil {
			// Newer BSON uses "Mapping", older uses "ReturnValueMapping"
			mappingRef := extractString(call["Mapping"])
			if mappingRef == "" {
				mappingRef = extractString(call["ReturnValueMapping"])
			}
			result.MappingID = model.ID(mappingRef)
		}
		if varType := toMap(raw["VariableType"]); varType != nil {
			result.ResultEntityID = model.ID(extractString(varType["Entity"]))
		}
		return result
	case "None":
		result := &microflows.ResultHandlingNone{}
		result.ID = model.ID(extractBsonID(raw["$ID"]))
		return result
	default:
		return nil
	}
}

func parseRequestHandling(raw map[string]any, handlingType string) microflows.RequestHandling {
	switch handlingType {
	case "Custom":
		result := &microflows.CustomRequestHandling{}
		result.ID = model.ID(extractBsonID(raw["$ID"]))
		if template, ok := raw["Template"].(map[string]any); ok {
			result.Template = extractString(template["Text"])
			result.TemplateParams = parseTemplateParameters(template)
		} else if templateD, ok := raw["Template"].(primitive.D); ok {
			templateM := templateD.Map()
			result.Template = extractString(templateM["Text"])
			result.TemplateParams = parseTemplateParameters(templateM)
		}
		return result
	case "Binary":
		result := &microflows.BinaryRequestHandling{}
		result.ID = model.ID(extractBsonID(raw["$ID"]))
		result.Expression = extractString(raw["Expression"])
		return result
	case "Mapping":
		result := &microflows.MappingRequestHandling{}
		result.ID = model.ID(extractBsonID(raw["$ID"]))
		// ExportMappingCall would be parsed here if needed
		return result
	case "FormData":
		result := &microflows.FormDataRequestHandling{}
		result.ID = model.ID(extractBsonID(raw["$ID"]))
		return result
	default:
		return nil
	}
}

func parseExecuteDatabaseQueryAction(raw map[string]any) *microflows.ExecuteDatabaseQueryAction {
	action := &microflows.ExecuteDatabaseQueryAction{}
	action.ID = model.ID(extractBsonID(raw["$ID"]))
	action.ErrorHandlingType = microflows.ErrorHandlingType(extractString(raw["ErrorHandlingType"]))
	action.OutputVariableName = extractString(raw["OutputVariableName"])
	action.Query = extractString(raw["Query"])
	action.DynamicQuery = extractString(raw["DynamicQuery"])

	// Parse ParameterMappings
	if mappings := extractBsonArray(raw["ParameterMappings"]); len(mappings) > 0 {
		for _, m := range mappings {
			if mMap, ok := m.(map[string]any); ok {
				mapping := &microflows.DatabaseQueryParameterMapping{}
				mapping.ID = model.ID(extractBsonID(mMap["$ID"]))
				mapping.ParameterName = extractString(mMap["ParameterName"])
				mapping.Value = extractString(mMap["Value"])
				action.ParameterMappings = append(action.ParameterMappings, mapping)
			}
		}
	}

	// Parse ConnectionParameterMappings
	if mappings := extractBsonArray(raw["ConnectionParameterMappings"]); len(mappings) > 0 {
		for _, m := range mappings {
			if mMap, ok := m.(map[string]any); ok {
				mapping := &microflows.DatabaseConnectionParameterMapping{}
				mapping.ID = model.ID(extractBsonID(mMap["$ID"]))
				mapping.ParameterName = extractString(mMap["ParameterName"])
				mapping.Value = extractString(mMap["Value"])
				action.ConnectionParameterMappings = append(action.ConnectionParameterMappings, mapping)
			}
		}
	}

	return action
}
