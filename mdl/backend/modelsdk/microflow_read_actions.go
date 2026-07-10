// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	genTexts "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/texts"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// actionFromGen reconstructs the semantic microflow action from an ActionActivity's
// gen Action child, so DESCRIBE/SHOW can render the activity body (the inverse of
// microflowActionToGen). Returns nil for action types not yet reconstructed — the
// activity then renders as an empty action, which is the prior behaviour, so this
// grows incrementally batch-by-batch without regressing already-handled types.
//
// Actions written via gen setters (this batch) read back through the gen
// accessors; the raw-built actions (list-ops, REST, …) will read their explicit
// keys in a later batch.
func actionFromGen(el element.Element) microflows.MicroflowAction {
	switch a := el.(type) {
	case *genMf.LogMessageAction:
		out := &microflows.LogMessageAction{
			ErrorHandlingType:     microflows.ErrorHandlingType(a.ErrorHandlingType()),
			LogLevel:              microflows.LogLevel(a.Level()),
			LogNodeName:           a.Node(),
			IncludeLastStackTrace: a.IncludeLatestStackTrace(),
		}
		out.ID = model.ID(a.ID())
		// MessageTemplate is a Microflows$StringTemplate (scalar Text + Arguments).
		if st, ok := a.MessageTemplate().(*genMf.StringTemplate); ok && st != nil {
			out.MessageTemplate = &model.Text{Translations: map[string]string{"en_US": st.Text()}}
			for _, argEl := range st.ArgumentsItems() {
				if arg, ok := argEl.(*genMf.TemplateArgument); ok {
					out.TemplateParameters = append(out.TemplateParameters, arg.Expression())
				}
			}
		}
		return out

	case *genMf.CreateVariableAction:
		out := &microflows.CreateVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			VariableName:      a.VariableName(),
			InitialValue:      a.InitialValue(),
			DataType:          dataTypeFromGen(a.VariableType()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ChangeVariableAction:
		out := &microflows.ChangeVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			VariableName:      a.ChangeVariableName(),
			Value:             a.Value(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CreateObjectAction:
		out := &microflows.CreateObjectAction{
			ErrorHandlingType:   microflows.ErrorHandlingType(a.ErrorHandlingType()),
			EntityQualifiedName: a.EntityQualifiedName(),
			OutputVariable:      a.OutputVariableName(),
			Commit:              microflows.CommitType(a.Commit()),
			InitialMembers:      memberChangesFromGen(a.ItemsItems()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ChangeObjectAction:
		out := &microflows.ChangeObjectAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			ChangeVariable:    a.ChangeVariableName(),
			Commit:            microflows.CommitType(a.Commit()),
			RefreshInClient:   a.RefreshInClient(),
			Changes:           memberChangesFromGen(a.ItemsItems()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CommitAction:
		out := &microflows.CommitObjectsAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			CommitVariable:    a.CommitVariableName(),
			WithEvents:        a.WithEvents(),
			RefreshInClient:   a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.DeleteAction:
		out := &microflows.DeleteObjectAction{
			DeleteVariable:  a.DeleteVariableName(),
			RefreshInClient: a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.RollbackAction:
		out := &microflows.RollbackObjectAction{
			RollbackVariable: a.RollbackVariableName(),
			RefreshInClient:  a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.RetrieveAction:
		out := &microflows.RetrieveAction{
			OutputVariable: a.OutputVariableName(),
			Source:         retrieveSourceFromGen(a.RetrieveSource()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.MicroflowCallAction:
		out := &microflows.MicroflowCallAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(a.ErrorHandlingType()),
			ResultVariableName: a.OutputVariableName(),
			UseReturnVariable:  a.UseReturnVariable(),
		}
		out.ID = model.ID(a.ID())
		if mc, ok := a.MicroflowCall().(*genMf.MicroflowCall); ok && mc != nil {
			call := &microflows.MicroflowCall{Microflow: mc.MicroflowQualifiedName()}
			call.ID = model.ID(mc.ID())
			for _, pmEl := range mc.ParameterMappingsItems() {
				if pm, ok := pmEl.(*genMf.MicroflowCallParameterMapping); ok {
					m := &microflows.MicroflowCallParameterMapping{Parameter: pm.ParameterQualifiedName(), Argument: pm.Argument()}
					m.ID = model.ID(pm.ID())
					call.ParameterMappings = append(call.ParameterMappings, m)
				}
			}
			out.MicroflowCall = call
		}
		return out

	case *genMf.NanoflowCallAction:
		out := &microflows.NanoflowCallAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(a.ErrorHandlingType()),
			OutputVariableName: a.OutputVariableName(),
			UseReturnVariable:  a.UseReturnVariable(),
		}
		out.ID = model.ID(a.ID())
		if nc, ok := a.NanoflowCall().(*genMf.NanoflowCall); ok && nc != nil {
			call := &microflows.NanoflowCall{Nanoflow: nc.NanoflowQualifiedName()}
			call.ID = model.ID(nc.ID())
			for _, pmEl := range nc.ParameterMappingsItems() {
				if pm, ok := pmEl.(*genMf.NanoflowCallParameterMapping); ok {
					m := &microflows.NanoflowCallParameterMapping{Parameter: pm.ParameterQualifiedName(), Argument: pm.Argument()}
					m.ID = model.ID(pm.ID())
					call.ParameterMappings = append(call.ParameterMappings, m)
				}
			}
			out.NanoflowCall = call
		}
		return out

	case *genMf.JavaScriptActionCallAction:
		out := &microflows.JavaScriptActionCallAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			JavaScriptAction:  a.JavaScriptActionQualifiedName(),
			// The gen binds outputVariableName to the key "VariableName", but a JS
			// action stores its output under "OutputVariableName" (per the legacy
			// parser), so read it from raw.
			OutputVariableName: rawStr(a.Raw(), "OutputVariableName"),
			UseReturnVariable:  a.UseReturnVariable(),
		}
		out.ID = model.ID(a.ID())
		for _, pmEl := range a.ParameterMappingsItems() {
			pm, ok := pmEl.(*genMf.JavaScriptActionParameterMapping)
			if !ok {
				continue
			}
			m := &microflows.JavaScriptActionParameterMapping{Parameter: pm.ParameterQualifiedName()}
			m.ID = model.ID(pm.ID())
			if v := pm.ParameterValue(); v != nil {
				m.Value = codeActionParameterValueFromRaw(v.Raw())
			}
			out.ParameterMappings = append(out.ParameterMappings, m)
		}
		return out

	case *genMf.CreateListAction:
		out := &microflows.CreateListAction{
			EntityQualifiedName: a.EntityQualifiedName(),
			OutputVariable:      a.OutputVariableName(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ChangeListAction:
		out := &microflows.ChangeListAction{
			ChangeVariable: a.ChangeVariableName(),
			Type:           microflows.ChangeListType(a.Type()),
			Value:          a.Value(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.AggregateListAction:
		out := &microflows.AggregateListAction{
			InputVariable:          a.InputListVariableName(),
			OutputVariable:         a.OutputVariableName(),
			Function:               microflows.AggregateFunction(a.AggregateFunction()),
			AttributeQualifiedName: a.AttributeQualifiedName(),
			UseExpression:          a.UseExpression(),
			Expression:             a.Expression(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CastAction:
		// ObjectVariable (the cast input) is not stored via a gen setter, so it is
		// not reconstructable here; OutputVariable is.
		out := &microflows.CastAction{OutputVariable: a.OutputVariableName()}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CloseFormAction:
		out := &microflows.ClosePageAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			NumberOfPages:     int(a.NumberOfPages()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ValidationFeedbackAction:
		out := &microflows.ValidationFeedbackAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			ObjectVariable:    a.ObjectVariableName(),
			AttributeName:     a.AttributeQualifiedName(),
			AssociationName:   a.AssociationQualifiedName(),
		}
		out.ID = model.ID(a.ID())
		out.Template, out.TemplateParameters = textTemplateFromGen(a.FeedbackTemplate())
		return out

	case *genMf.ShowHomePageAction:
		out := &microflows.ShowHomePageAction{}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ShowMessageAction:
		out := &microflows.ShowMessageAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			Type:              microflows.MessageType(a.Type()),
			Blocking:          a.Blocking(),
		}
		out.ID = model.ID(a.ID())
		out.Template, out.TemplateParameters = textTemplateFromGen(a.Template())
		return out

	case *genMf.ShowPageAction:
		// Storage $Type Microflows$ShowFormAction. The FormSettings tree is written
		// raw (Form / ParameterMappings with legacy keys), so read it from raw.
		out := &microflows.ShowPageAction{ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType())}
		out.ID = model.ID(a.ID())
		if fs, ok := a.Raw().Lookup("FormSettings").DocumentOK(); ok {
			out.FormSettingsID = model.ID(rawStr(fs, "$ID"))
			out.PageName = rawStr(fs, "Form")
			if arr, ok := fs.Lookup("ParameterMappings").ArrayOK(); ok {
				vals, _ := arr.Values()
				for _, v := range vals {
					md, ok := v.DocumentOK()
					if !ok {
						continue
					}
					pm := &microflows.PageParameterMapping{Parameter: rawStr(md, "Parameter"), Argument: rawStr(md, "Argument")}
					pm.ID = model.ID(rawStr(md, "$ID"))
					out.PageParameterMappings = append(out.PageParameterMappings, pm)
				}
			}
		}
		return out

	case *genMf.JavaActionCallAction:
		// Storage keys JavaAction / ResultVariableName diverge from the gen
		// accessors, so read from raw — the inverse of the direct write.
		raw := a.Raw()
		out := &microflows.JavaActionCallAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(rawStr(raw, "ErrorHandlingType")),
			JavaAction:         rawStr(raw, "JavaAction"),
			ResultVariableName: rawStr(raw, "ResultVariableName"),
		}
		out.ID = model.ID(a.ID())
		if b, ok := raw.Lookup("UseReturnVariable").BooleanOK(); ok {
			out.UseReturnVariable = b
		}
		if arr, ok := raw.Lookup("ParameterMappings").ArrayOK(); ok {
			vals, _ := arr.Values()
			for _, v := range vals {
				md, ok := v.DocumentOK()
				if !ok {
					continue
				}
				pm := &microflows.JavaActionParameterMapping{Parameter: rawStr(md, "Parameter")}
				pm.ID = model.ID(rawStr(md, "$ID"))
				if vd, ok := md.Lookup("Value").DocumentOK(); ok {
					pm.Value = codeActionParameterValueFromRaw(vd)
				}
				out.ParameterMappings = append(out.ParameterMappings, pm)
			}
		}
		return out

	case *genMf.RestCallAction:
		// The whole RestCall tree is written raw with the verified legacy storage
		// keys (HttpConfiguration / RequestHandling / ResultHandling and their
		// children), so reconstruct it from raw BSON — the inverse of
		// restCallActionToGen.
		raw := a.Raw()
		out := &microflows.RestCallAction{
			ErrorHandlingType: microflows.ErrorHandlingType(rawStr(raw, "ErrorHandlingType")),
			TimeoutExpression: rawStr(raw, "TimeOutExpression"),
		}
		out.ID = model.ID(a.ID())
		if hc, ok := raw.Lookup("HttpConfiguration").DocumentOK(); ok {
			out.HttpConfiguration = httpConfigFromRaw(hc)
		}
		if rh, ok := raw.Lookup("RequestHandling").DocumentOK(); ok {
			out.RequestHandling = restRequestHandlingFromRaw(rh)
		}
		if rh, ok := raw.Lookup("ResultHandling").DocumentOK(); ok {
			out.ResultHandling = restResultHandlingFromRaw(rh)
		}
		return out

	case *genMf.ListOperationAction:
		// Storage $Type Microflows$ListOperationsAction. The write binds the output
		// to "ResultVariableName" and the operation to "NewOperation" (not the gen
		// keys), so read both from the raw BSON — the inverse of the write's
		// listOperationToGen.
		raw := a.Raw()
		out := &microflows.ListOperationAction{OutputVariable: rawStr(raw, "ResultVariableName")}
		out.ID = model.ID(a.ID())
		if opDoc, ok := raw.Lookup("NewOperation").DocumentOK(); ok {
			out.Operation = listOperationFromRaw(opDoc)
		}
		return out

	case *genMf.WebServiceCallAction:
		// Legacy SOAP CALL WEB SERVICE. Without this case it renders
		// "-- Empty action". Mirror legacy parseWebServiceCallAction: read the
		// structured fields, and when the action carries any field the structured
		// describe form can't represent, stash the raw BSON so the renderer emits
		// `call web service raw '<base64>'` (the same fallback legacy uses).
		raw := a.Raw()
		out := &microflows.WebServiceCallAction{
			ErrorHandlingType: microflows.ErrorHandlingType(rawStr(raw, "ErrorHandlingType")),
			ServiceID:         model.ID(rawStr(raw, "ImportedService")),
			OperationName:     rawStr(raw, "OperationName"),
			TimeoutExpression: rawStr(raw, "TimeOutExpression"),
		}
		out.ID = model.ID(a.ID())
		if rh, ok := raw.Lookup("NewResultHandling").DocumentOK(); ok {
			out.OutputVariable = rawStr(rh, "ResultVariableName")
			out.UseReturnVariable = out.OutputVariable != ""
			if imc, ok := rh.Lookup("ImportMappingCall").DocumentOK(); ok {
				out.ReceiveMappingID = model.ID(rawStr(imc, "ReturnValueMapping"))
			}
		}
		if rqh, ok := raw.Lookup("RequestHandling").DocumentOK(); ok {
			if emc, ok := rqh.Lookup("ExportMappingCall").DocumentOK(); ok {
				out.SendMappingID = model.ID(rawStr(emc, "Mapping"))
			}
		}
		if webServiceActionRequiresRawBSON(raw) {
			out.RawBSON = raw
		}
		return out

	case *genMf.DownloadFileAction:
		// DOWNLOAD FILE. Without this case it renders "-- Empty action". Mirror
		// legacy parseDownloadFileAction, including the Rollback default for an
		// empty error-handling type.
		eht := microflows.ErrorHandlingType(a.ErrorHandlingType())
		if eht == "" {
			eht = microflows.ErrorHandlingTypeRollback
		}
		out := &microflows.DownloadFileAction{
			ErrorHandlingType: eht,
			FileDocument:      a.FileDocumentVariableName(),
			ShowInBrowser:     a.ShowFileInBrowser(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ExportXmlAction:
		// EXPORT TO MAPPING. The mapping/argument live in ResultHandling
		// (Microflows$MappingRequestHandling) and the output variable in
		// OutputMethod (ExportXmlAction$StringExport) — read from raw, mirroring
		// legacy parseExportXmlAction. Without this case it renders "-- Empty action".
		out := &microflows.ExportXmlAction{
			ErrorHandlingType:    microflows.ErrorHandlingType(a.ErrorHandlingType()),
			IsValidationRequired: a.IsValidationRequired(),
		}
		out.ID = model.ID(a.ID())
		raw := a.Raw()
		if om, ok := raw.Lookup("OutputMethod").DocumentOK(); ok {
			out.OutputVariable = rawStr(om, "OutputVariableName")
		}
		if rh, ok := raw.Lookup("ResultHandling").DocumentOK(); ok {
			h := &microflows.MappingRequestHandling{
				MappingID:         model.ID(rawStr(rh, "MappingId")),
				ContentType:       rawStr(rh, "ContentType"),
				ParameterVariable: rawStr(rh, "MappingVariableName"),
			}
			h.ID = model.ID(rawStr(rh, "$ID"))
			out.RequestHandling = h
		}
		return out

	case *genMf.ImportXmlAction:
		// IMPORT FROM MAPPING. The mapping ref + cardinality live in
		// ResultHandling.ImportMappingCall — read from raw, mirroring legacy
		// parseImportXmlAction (the force fallback folds ForceSingleOccurrence into
		// SingleObject). Without this case it renders "-- Empty action".
		out := &microflows.ImportXmlAction{
			ErrorHandlingType:    microflows.ErrorHandlingType(a.ErrorHandlingType()),
			IsValidationRequired: a.IsValidationRequired(),
			XmlDocumentVariable:  a.XmlDocumentVariableName(),
		}
		out.ID = model.ID(a.ID())
		if rh, ok := a.Raw().Lookup("ResultHandling").DocumentOK(); ok {
			if imc, ok := rh.Lookup("ImportMappingCall").DocumentOK(); ok {
				h, force, _ := readMappingCall(rh, imc)
				if !h.SingleObject && force {
					h.SingleObject = true
				}
				out.ResultHandling = h
			}
		}
		return out

	default:
		return nil
	}
}

// textTemplateFromGen reconstructs a Microflows$TextTemplate's translations and
// template arguments (the {1},{2},… expressions). Inverse of textTemplateToGen.
func textTemplateFromGen(el element.Element) (*model.Text, []string) {
	tt, ok := el.(*genMf.TextTemplate)
	if !ok || tt == nil {
		return nil, nil
	}
	var text *model.Text
	if txt, ok := tt.Text().(*genTexts.Text); ok && txt != nil {
		trans := map[string]string{}
		for _, trEl := range txt.TranslationsItems() {
			if tr, ok := trEl.(*genTexts.Translation); ok {
				trans[tr.LanguageCode()] = tr.Text()
			}
		}
		if len(trans) > 0 {
			text = &model.Text{Translations: trans}
		}
	}
	var params []string
	for _, argEl := range tt.ArgumentsItems() {
		if arg, ok := argEl.(*genMf.TemplateArgument); ok {
			params = append(params, arg.Expression())
		}
	}
	return text, params
}

// codeActionParameterValueFromRaw reconstructs a java-action parameter value from
// its raw Value sub-document. Inverse of codeActionParameterValueToGen.
func codeActionParameterValueFromRaw(doc bson.Raw) microflows.CodeActionParameterValue {
	id := model.ID(rawStr(doc, "$ID"))
	switch rawStr(doc, "$Type") {
	case "Microflows$StringTemplateParameterValue":
		v := &microflows.StringTemplateParameterValue{}
		v.ID = id
		if tt, ok := doc.Lookup("TypedTemplate").DocumentOK(); ok {
			t := &microflows.TypedTemplate{Text: rawStr(tt, "Text")}
			t.ID = model.ID(rawStr(tt, "$ID"))
			v.TypedTemplate = t
		}
		return v
	case "Microflows$ExpressionBasedCodeActionParameterValue":
		v := &microflows.ExpressionBasedCodeActionParameterValue{Expression: rawStr(doc, "Expression")}
		v.ID = id
		return v
	case "Microflows$BasicCodeActionParameterValue":
		v := &microflows.BasicCodeActionParameterValue{Argument: rawStr(doc, "Argument")}
		v.ID = id
		return v
	case "Microflows$MicroflowParameterValue":
		v := &microflows.MicroflowParameterValue{Microflow: rawStr(doc, "Microflow")}
		v.ID = id
		return v
	case "Microflows$EntityTypeCodeActionParameterValue":
		v := &microflows.EntityTypeCodeActionParameterValue{Entity: rawStr(doc, "Entity")}
		v.ID = id
		return v
	default:
		return nil
	}
}

// stringTemplateFromRaw reads a Microflows$StringTemplate's Text and its
// {1},{2},… parameter expressions. Inverse of stringTemplateElem.
func stringTemplateFromRaw(doc bson.Raw) (string, []string) {
	text := rawStr(doc, "Text")
	var params []string
	if arr, ok := doc.Lookup("Parameters").ArrayOK(); ok {
		vals, _ := arr.Values()
		for _, v := range vals {
			pd, ok := v.DocumentOK()
			if !ok {
				continue
			}
			params = append(params, rawStr(pd, "Expression"))
		}
	}
	return text, params
}

// httpConfigFromRaw reconstructs a REST call's HttpConfiguration (method, URL
// template + params, basic auth, custom headers). Inverse of httpConfigToGen; the
// auth/method/header keys are the verified legacy storage names.
func httpConfigFromRaw(doc bson.Raw) *microflows.HttpConfiguration {
	c := &microflows.HttpConfiguration{
		HttpMethod:     microflows.HttpMethod(rawStr(doc, "HttpMethod")),
		Username:       rawStr(doc, "HttpAuthenticationUserName"),
		Password:       rawStr(doc, "HttpAuthenticationPassword"),
		CustomLocation: rawStr(doc, "CustomLocation"),
	}
	c.ID = model.ID(rawStr(doc, "$ID"))
	if b, ok := doc.Lookup("UseHttpAuthentication").BooleanOK(); ok {
		c.UseAuthentication = b
	}
	if lt, ok := doc.Lookup("CustomLocationTemplate").DocumentOK(); ok {
		c.LocationTemplate, c.LocationParams = stringTemplateFromRaw(lt)
	}
	if arr, ok := doc.Lookup("HttpHeaderEntries").ArrayOK(); ok {
		vals, _ := arr.Values()
		for _, v := range vals {
			hd, ok := v.DocumentOK()
			if !ok {
				continue
			}
			h := &microflows.HttpHeader{Name: rawStr(hd, "Key"), Value: rawStr(hd, "Value")}
			h.ID = model.ID(rawStr(hd, "$ID"))
			c.CustomHeaders = append(c.CustomHeaders, h)
		}
	}
	return c
}

// restRequestHandlingFromRaw reconstructs a REST call's request body handling
// (custom template, response mapping, or simple). Inverse of restRequestHandlingToGen.
func restRequestHandlingFromRaw(doc bson.Raw) microflows.RequestHandling {
	id := model.ID(rawStr(doc, "$ID"))
	switch rawStr(doc, "$Type") {
	case "Microflows$CustomRequestHandling":
		h := &microflows.CustomRequestHandling{}
		h.ID = id
		if t, ok := doc.Lookup("Template").DocumentOK(); ok {
			h.Template, h.TemplateParams = stringTemplateFromRaw(t)
		}
		return h
	case "Microflows$MappingRequestHandling":
		// The export-mapping source variable is stored under "MappingVariableName"
		// (same key ExportXmlAction uses), not "ParameterVariable". Reading the
		// wrong key left it empty, so the renderer emitted `body mapping X` without
		// the grammar-mandatory `from $var` — invalid MDL that broke the DESCRIBE
		// roundtrip (mismatched input, expecting FROM).
		paramVar := rawStr(doc, "MappingVariableName")
		if paramVar == "" {
			paramVar = rawStr(doc, "ParameterVariable")
		}
		h := &microflows.MappingRequestHandling{
			MappingID:         model.ID(rawStr(doc, "MappingId")),
			ContentType:       rawStr(doc, "ContentType"),
			ParameterVariable: paramVar,
		}
		h.ID = id
		return h
	case "Microflows$SimpleRequestHandling":
		h := &microflows.SimpleRequestHandling{}
		h.ID = id
		return h
	default:
		return nil
	}
}

// restResultHandlingFromRaw reconstructs a REST call's result handling. A Mapping
// result carries an ImportMappingCall; the other variants discriminate on the
// VariableType ($Type Void → Nothing, ObjectType System.HttpResponse → response,
// else String). Inverse of restResultHandlingToGen.
func restResultHandlingFromRaw(doc bson.Raw) microflows.ResultHandling {
	id := model.ID(rawStr(doc, "$ID"))
	resultVar := rawStr(doc, "ResultVariableName")
	if imc, ok := doc.Lookup("ImportMappingCall").DocumentOK(); ok {
		h, _, vtType := readMappingCall(doc, imc)
		// An object (not list) result variable type means a single object, so the
		// describer prints "as Entity" rather than "as list of Entity" — matches
		// legacy parseResultHandling; without it single-object mappings wrongly
		// render as lists.
		if vtType == "DataTypes$ObjectType" {
			h.SingleObject = true
		}
		return h
	}
	vtType, entity := "", ""
	if vt, ok := doc.Lookup("VariableType").DocumentOK(); ok {
		vtType = rawStr(vt, "$Type")
		entity = rawStr(vt, "Entity")
	}
	switch {
	case vtType == "DataTypes$VoidType":
		h := &microflows.ResultHandlingNone{}
		h.ID = id
		return h
	case vtType == "DataTypes$ObjectType" && entity == "System.HttpResponse":
		h := &microflows.ResultHandlingHttpResponse{VariableName: resultVar}
		h.ID = id
		return h
	default:
		h := &microflows.ResultHandlingString{VariableName: resultVar}
		h.ID = id
		return h
	}
}

// readMappingCall reconstructs the *ResultHandlingMapping fields shared by REST
// result handling and IMPORT FROM MAPPING: the mapping reference (newer BSON
// uses "Mapping", older "ReturnValueMapping"), the result entity, the
// ForceSingleOccurrence flag, and the Range.SingleObject flag. Callers apply
// their own remaining single-object rule (REST: object var type; XML import:
// the force fallback) so each matches its legacy parser exactly. doc is the
// result-handling document; imc is its ImportMappingCall sub-document.
func readMappingCall(doc, imc bson.Raw) (h *microflows.ResultHandlingMapping, force bool, vtType string) {
	h = &microflows.ResultHandlingMapping{ResultVariable: rawStr(doc, "ResultVariableName")}
	h.ID = model.ID(rawStr(doc, "$ID"))
	mapping := rawStr(imc, "Mapping")
	if mapping == "" {
		mapping = rawStr(imc, "ReturnValueMapping")
	}
	h.MappingID = model.ID(mapping)
	force, _ = imc.Lookup("ForceSingleOccurrence").BooleanOK()
	h.ForceSingleOccurrence = &force
	if rng, ok := imc.Lookup("Range").DocumentOK(); ok {
		if b, ok := rng.Lookup("SingleObject").BooleanOK(); ok {
			h.SingleObject = b
		}
	}
	if vt, ok := doc.Lookup("VariableType").DocumentOK(); ok {
		h.ResultEntityID = model.ID(rawStr(vt, "Entity"))
		vtType = rawStr(vt, "$Type")
	}
	return h, force, vtType
}

// webServiceActionRequiresRawBSON reports whether a CALL WEB SERVICE action
// carries any field the structured describe form can't represent, in which case
// the renderer emits the `call web service raw '<base64>'` fallback. Mirrors the
// legacy webServiceActionRequiresRawBSON supported-key set exactly so both engines
// agree on when to fall back.
func webServiceActionRequiresRawBSON(raw bson.Raw) bool {
	supported := map[string]bool{
		"$ID":               true,
		"$Type":             true,
		"ErrorHandlingType": true,
		"ImportedService":   true,
		"OperationName":     true,
		"TimeOutExpression": true,
		"UseRequestTimeOut": true,
		"NewResultHandling": true,
		"RequestHandling":   true,
	}
	els, err := raw.Elements()
	if err != nil {
		return true
	}
	for _, el := range els {
		if !supported[el.Key()] {
			return true
		}
	}
	return false
}

// rawStr reads a string field from a raw BSON document, returning "" if the field
// is absent or not a string.
func rawStr(doc bson.Raw, key string) string {
	if doc == nil {
		return ""
	}
	v, _ := doc.Lookup(key).StringValueOK()
	return v
}

// listOperationFromRaw reconstructs a list operation from its NewOperation BSON
// sub-document, the inverse of listOperationToGen. Each operation carries the
// verified legacy storage keys (ListName / SecondListOrObjectName / …).
func listOperationFromRaw(doc bson.Raw) microflows.ListOperation {
	id := model.ID(rawStr(doc, "$ID"))
	list := rawStr(doc, "ListName")
	expr := rawStr(doc, "Expression")
	second := rawStr(doc, "SecondListOrObjectName")
	switch rawStr(doc, "$Type") {
	case "Microflows$Head":
		o := &microflows.HeadOperation{ListVariable: list}
		o.ID = id
		return o
	case "Microflows$Tail":
		o := &microflows.TailOperation{ListVariable: list}
		o.ID = id
		return o
	case "Microflows$FindByExpression":
		o := &microflows.FindOperation{ListVariable: list, Expression: expr}
		o.ID = id
		return o
	case "Microflows$FilterByExpression":
		o := &microflows.FilterOperation{ListVariable: list, Expression: expr}
		o.ID = id
		return o
	case "Microflows$Find":
		o := &microflows.FindByAttributeOperation{ListVariable: list, Association: rawStr(doc, "Association"), Attribute: rawStr(doc, "Attribute"), Expression: expr}
		o.ID = id
		return o
	case "Microflows$Filter":
		o := &microflows.FilterByAttributeOperation{ListVariable: list, Association: rawStr(doc, "Association"), Attribute: rawStr(doc, "Attribute"), Expression: expr}
		o.ID = id
		return o
	case "Microflows$Sort":
		o := &microflows.SortOperation{ListVariable: list, Sorting: sortItemsFromRaw(doc)}
		o.ID = id
		return o
	case "Microflows$Union":
		o := &microflows.UnionOperation{ListVariable1: list, ListVariable2: second}
		o.ID = id
		return o
	case "Microflows$Intersect":
		o := &microflows.IntersectOperation{ListVariable1: list, ListVariable2: second}
		o.ID = id
		return o
	case "Microflows$Subtract":
		o := &microflows.SubtractOperation{ListVariable1: list, ListVariable2: second}
		o.ID = id
		return o
	case "Microflows$Contains":
		o := &microflows.ContainsOperation{ListVariable: list, ObjectVariable: second}
		o.ID = id
		return o
	case "Microflows$Equals":
		o := &microflows.EqualsOperation{ListVariable1: list, ListVariable2: second}
		o.ID = id
		return o
	case "Microflows$ListRange":
		o := &microflows.ListRangeOperation{ListVariable: list, LimitExpression: rawStr(doc, "LimitExpression"), OffsetExpression: rawStr(doc, "OffsetExpression")}
		o.ID = id
		return o
	default:
		return nil
	}
}

// sortItemsFromRaw reconstructs a Sort operation's sort columns from its nested
// SortingsList → Sortings array. The first array element is the typed-array marker
// (an int, not a document) and is skipped by the DocumentOK guard.
func sortItemsFromRaw(doc bson.Raw) []*microflows.SortItem {
	// The SortingsList wrapper is keyed "Sortings" on a Sort list operation but
	// "NewSortings" on a DatabaseRetrieveSource — accept both so retrieve "sort by"
	// clauses aren't dropped.
	slDoc, ok := doc.Lookup("Sortings").DocumentOK()
	if !ok {
		slDoc, ok = doc.Lookup("NewSortings").DocumentOK()
	}
	if !ok {
		return nil
	}
	arr, ok := slDoc.Lookup("Sortings").ArrayOK()
	if !ok {
		return nil
	}
	vals, err := arr.Values()
	if err != nil {
		return nil
	}
	var out []*microflows.SortItem
	for _, v := range vals {
		sd, ok := v.DocumentOK()
		if !ok {
			continue
		}
		it := &microflows.SortItem{Direction: microflows.SortDirection(rawStr(sd, "SortOrder"))}
		it.ID = model.ID(rawStr(sd, "$ID"))
		if ref, ok := sd.Lookup("AttributeRef").DocumentOK(); ok {
			// The AttributeRef stores its by-name reference under "Attribute".
			it.AttributeQualifiedName = rawStr(ref, "Attribute")
		}
		out = append(out, it)
	}
	return out
}

// memberChangesFromGen reconstructs the attribute/association assignments of a
// create/change-object action (the inverse of memberChangeToGen).
func memberChangesFromGen(items []element.Element) []*microflows.MemberChange {
	var out []*microflows.MemberChange
	for _, el := range items {
		g, ok := el.(*genMf.MemberChange)
		if !ok {
			continue
		}
		m := &microflows.MemberChange{
			AttributeQualifiedName:   g.AttributeQualifiedName(),
			AssociationQualifiedName: g.AssociationQualifiedName(),
			Type:                     microflows.MemberChangeType(g.Type()),
			Value:                    g.Value(),
		}
		m.ID = model.ID(g.ID())
		out = append(out, m)
	}
	return out
}

// retrieveSourceFromGen reconstructs a retrieve's source (database with XPath/
// range, or association navigation). Inverse of retrieveSourceToGen.
func retrieveSourceFromGen(el element.Element) microflows.RetrieveSource {
	switch g := el.(type) {
	case *genMf.DatabaseRetrieveSource:
		s := &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: g.EntityQualifiedName(),
			XPathConstraint:     g.XPathConstraint(),
			Range:               rangeFromGen(g.Range()),
			// Sort columns live in the NewSortings child; without this the
			// retrieve's "sort by …" clause is dropped.
			Sorting: sortItemsFromRaw(g.Raw()),
		}
		s.ID = model.ID(g.ID())
		return s
	case *genMf.AssociationRetrieveSource:
		s := &microflows.AssociationRetrieveSource{
			StartVariable:            g.StartVariableName(),
			AssociationQualifiedName: g.AssociationQualifiedName(),
		}
		s.ID = model.ID(g.ID())
		return s
	default:
		return nil
	}
}

// rangeFromGen maps a gen range element to the model Range. A ConstantRange with
// SingleObject means "first" (limit 1); SingleObject=false has no range. A
// CustomRange carries limit/offset expressions.
func rangeFromGen(el element.Element) *microflows.Range {
	switch g := el.(type) {
	case *genMf.ConstantRange:
		if g.SingleObject() {
			return &microflows.Range{RangeType: microflows.RangeTypeFirst}
		}
		return nil
	case *genMf.CustomRange:
		return &microflows.Range{RangeType: microflows.RangeTypeCustom, Limit: g.LimitExpression(), Offset: g.OffsetExpression()}
	default:
		return nil
	}
}
