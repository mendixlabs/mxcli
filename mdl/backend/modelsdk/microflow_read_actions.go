// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDb "github.com/mendixlabs/mxcli/modelsdk/gen/databaseconnector"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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
			RefreshInClient:     a.RefreshInClient(),
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
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			DeleteVariable:    a.DeleteVariableName(),
			RefreshInClient:   a.RefreshInClient(),
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
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			OutputVariable:    a.OutputVariableName(),
			Source:            retrieveSourceFromGen(a.RetrieveSource()),
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
			call.QueueSettings = queueSettingsFromRaw(mc.Raw())
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
			// Reduce's fold: what it starts from and what it folds to. Studio Pro
			// stores both on every AggregateAction, so a rewrite that did not read
			// them back silently deleted the fold (#1004).
			ReduceInitialValue: a.ReduceInitialValueExpression(),
			ReduceReturnType:   dataTypeFromGen(a.ReduceReturnDataType()),
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
		out.QueueSettings = queueSettingsFromRaw(raw)
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
			out.ResultHandling = restResultHandlingFromRaw(rh, rawStr(raw, "ResultHandlingType"))
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
		// RequestHandling / ExportMappingCall is a shape NO reference document
		// carries — all three of ako/TestApp's calls store RequestBodyHandling —
		// so SendMappingID was never populated from a real project. Kept because
		// removing it would be a guess in the other direction, and read the real
		// key below.
		if rqh, ok := raw.Lookup("RequestHandling").DocumentOK(); ok {
			if emc, ok := rqh.Lookup("ExportMappingCall").DocumentOK(); ok {
				out.SendMappingID = model.ID(rawStr(emc, "Mapping"))
			}
		}
		readWebServiceRequestBody(raw, out)
		if webServiceActionRequiresRawBSON(raw) {
			out.RawBSON = raw
		}
		return out

	case *genDb.ExecuteDatabaseQueryAction:
		// EXECUTE DATABASE QUERY. Read from raw against the keys the writer builds
		// directly (Query / DynamicQuery / OutputVariableName / the two mapping
		// lists) rather than through the gen accessors: gen exposes the named
		// query as QueryQualifiedName, and pinning reader and writer to the same
		// key set is what keeps the round trip honest.
		//
		// The action is authorable in MDL and already has a DESCRIBE formatter —
		// only this mapping was missing, so it wrote fine and read back as
		// "-- Empty action".
		raw := a.Raw()
		out := &microflows.ExecuteDatabaseQueryAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(rawStr(raw, "ErrorHandlingType")),
			OutputVariableName: rawStr(raw, "OutputVariableName"),
			Query:              rawStr(raw, "Query"),
			DynamicQuery:       rawStr(raw, "DynamicQuery"),
		}
		// Mapping IDs are deliberately not carried across: the writer mints fresh
		// ones (as the JavaActionCallAction mapping reader above also does), and a
		// half-preserved ID is worse than none.
		for _, md := range rawDocElements(raw, "ParameterMappings") {
			pm := &microflows.DatabaseQueryParameterMapping{
				ParameterName: rawStr(md, "ParameterName"),
				Value:         rawStr(md, "Value"),
			}
			out.ParameterMappings = append(out.ParameterMappings, pm)
		}
		for _, md := range rawDocElements(raw, "ConnectionParameterMappings") {
			cm := &microflows.DatabaseConnectionParameterMapping{
				ParameterName: rawStr(md, "ParameterName"),
				Value:         rawStr(md, "Value"),
			}
			out.ConnectionParameterMappings = append(out.ConnectionParameterMappings, cm)
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.SynchronizeAction:
		// SYNCHRONIZE (nanoflow-only). Type and ErrorHandlingType come off the gen
		// accessors, but VariableNames must be read from raw: the model stores a
		// BSON array of strings, while gen binds it as a scalar Primitive[string]
		// (a codegen mismatch), so the accessor would hand back "" for a
		// Specific-mode action and silently lose its variables.
		//
		// An absent Type means All — the platform default (#863).
		raw := a.Raw()
		out := &microflows.SynchronizeAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			SyncType:          microflows.SynchronizationType(orDefault(a.Type(), string(microflows.SynchronizationTypeAll))),
		}
		if arr, ok := raw.Lookup("VariableNames").ArrayOK(); ok {
			vals, _ := arr.Values()
			for _, v := range vals {
				if s, ok := v.StringValueOK(); ok {
					out.VariableNames = append(out.VariableNames, s)
				}
			}
		}
		out.ID = model.ID(a.ID())
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
				h, force, vtType := readMappingCall(rh, imc)
				// The stored VariableType is the authority on the result
				// variable's cardinality, and it does NOT track the range:
				// Mendix's own SUB_Feedback_PostToAppInsights stores
				// ConstantRange{SingleObject:false} against an ObjectType. Only
				// where no VariableType is stored does ForceSingleOccurrence
				// stand in — and never for a bounded range, which is always a
				// list, or a Custom range would read back as `first` and lose
				// the limit. (issue #881)
				switch vtType {
				case "DataTypes$ObjectType":
					h.SingleObject = true
				case "DataTypes$ListType":
					h.SingleObject = false
				default:
					if !h.SingleObject && force && h.LimitExpression == "" && h.OffsetExpression == "" {
						h.SingleObject = true
					}
				}
				out.ResultHandling = h
			}
		}
		return out

	case *genMf.TransformJsonAction:
		// TRANSFORM $In WITH Module.Transformer. Read against the keys the writer
		// builds (mirroring the legacy serializer) rather than the gen accessors.
		raw := a.Raw()
		out := &microflows.TransformJsonAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(rawStr(raw, "ErrorHandlingType")),
			InputVariableName:  rawStr(raw, "InputVariableName"),
			OutputVariableName: rawStr(raw, "OutputVariableName"),
			Transformation:     rawStr(raw, "Transformation"),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CallExternalAction:
		// CALL EXTERNAL ACTION. `VariableName` holds the result variable here —
		// the model's own key, not gen's — and ResultDataType is deliberately NOT
		// reconstructed: it is resolved from the consumed service's cached
		// $metadata at write time, so inferring it from the stored
		// VariableDataType would let a stale value round-trip as if authored.
		raw := a.Raw()
		out := &microflows.CallExternalAction{
			ErrorHandlingType:    microflows.ErrorHandlingType(rawStr(raw, "ErrorHandlingType")),
			ConsumedODataService: rawStr(raw, "ConsumedODataService"),
			Name:                 rawStr(raw, "Name"),
			ResultVariableName:   rawStr(raw, "VariableName"),
		}
		out.UseReturnVariable = out.ResultVariableName != ""
		for _, md := range rawDocElements(raw, "ParameterMappings") {
			pm := &microflows.ExternalActionParameterMapping{
				ParameterName: rawStr(md, "ParameterName"),
				Argument:      rawStr(md, "Argument"),
			}
			if b, ok := md.Lookup("CanBeEmpty").BooleanOK(); ok {
				pm.CanBeEmpty = b
			}
			out.ParameterMappings = append(out.ParameterMappings, pm)
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.RestOperationCallAction:
		// CALL REST OPERATION. The output and body variables are single-child
		// documents rather than scalars, and the two mapping lists use different
		// key names for the same idea (`Parameter` vs `QueryParameter`) — mirror
		// the writer rather than assuming symmetry.
		raw := a.Raw()
		out := &microflows.RestOperationCallAction{
			ErrorHandlingType: microflows.ErrorHandlingType(rawStr(raw, "ErrorHandlingType")),
			Operation:         rawStr(raw, "Operation"),
		}
		if ov, ok := raw.Lookup("OutputVariable").DocumentOK(); ok {
			out.OutputVariable = &microflows.RestOutputVar{VariableName: rawStr(ov, "VariableName")}
		}
		if bv, ok := raw.Lookup("BodyVariable").DocumentOK(); ok {
			out.BodyVariable = &microflows.RestBodyVar{VariableName: rawStr(bv, "VariableName")}
		}
		for _, md := range rawDocElements(raw, "ParameterMappings") {
			out.ParameterMappings = append(out.ParameterMappings, &microflows.RestParameterMapping{
				Parameter: rawStr(md, "Parameter"),
				Value:     rawStr(md, "Value"),
			})
		}
		for _, md := range rawDocElements(raw, "QueryParameterMappings") {
			out.QueryParameterMappings = append(out.QueryParameterMappings, &microflows.RestQueryParameterMapping{
				Parameter: rawStr(md, "QueryParameter"),
				Value:     rawStr(md, "Value"),
				Included:  rawStr(md, "Included"),
			})
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.WorkflowCallAction,
		*genMf.GetWorkflowDataAction,
		*genMf.GetWorkflowsAction,
		*genMf.GetWorkflowActivityRecordsAction,
		*genMf.WorkflowOperationAction,
		*genMf.OpenWorkflowAction,
		*genMf.LockWorkflowAction,
		*genMf.UnlockWorkflowAction:
		// Workflow call actions — the inverse of workflowMicroflowActionToGen,
		// grouped there and read back together here.
		return workflowActionFromGen(a)

	case *genMf.SetTaskOutcomeAction:
		// SET TASK OUTCOME $UserTask 'Outcome'. Without this case the workflow
		// completion action renders "-- Empty action", so a describe→drop→exec
		// round-trip silently loses it (FINDINGS #54). Mirrors legacy
		// parseSetTaskOutcomeAction.
		out := &microflows.SetTaskOutcomeAction{
			ErrorHandlingType:    microflows.ErrorHandlingType(a.ErrorHandlingType()),
			OutcomeValue:         a.OutcomeValue(),
			WorkflowTaskVariable: a.WorkflowTaskVariable(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.OpenUserTaskAction:
		// OPEN USER TASK $UserTask. Mirrors legacy parseOpenUserTaskAction.
		out := &microflows.OpenUserTaskAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			UserTaskVariable:  a.UserTaskVariable(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.NotifyWorkflowAction:
		// NOTIFY WORKFLOW $Workflow TARGET Module.Workflow.Name. Two of the five
		// target types have no gen type, so the target is read off its raw document
		// by $Type, like any of them.
		out := &microflows.NotifyWorkflowAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(a.ErrorHandlingType()),
			OutputVariableName: a.OutputVariableName(),
			WorkflowVariable:   a.WorkflowVariable(),
			Activity:           a.ActivityQualifiedName(),
		}
		if t := a.NotifyTarget(); t != nil && t.TypeName() != "" {
			target := &microflows.NotifyTarget{TypeName: t.TypeName()}
			target.Name = genWf.RawFieldString(t.Raw(), target.Key())
			out.Target = target
		}
		out.ID = model.ID(a.ID())
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
	case "Microflows$BinaryRequestHandling":
		// Binary request body — the expression yielding the bytes, stored as
		// source text (Studio Pro writes a FileDocument's Contents member,
		// e.g. `$Doc/Contents`). Without this case DESCRIBE dropped the body
		// silently and the round trip produced a request with no payload.
		h := &microflows.BinaryRequestHandling{Expression: rawStr(doc, "Expression")}
		h.ID = id
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
// result carries an ImportMappingCall; the rest are told apart by Mendix's own
// ResultHandlingType discriminator, falling back to the VariableType when that
// property is absent (it is omitempty, so a Studio Pro document need not carry
// it). Inverse of restResultHandlingToGen.
//
// The fallback must distinguish FileDocument from HttpResponse by entity name,
// because both are stored as DataTypes$ObjectType. Reading it as "anything that
// is not literally System.HttpResponse is a String" was issue #922: a REST call
// storing into a file document described as `returns String`, and a describe →
// exec round trip rewrote the stored type from FileDocument to String — with
// mxbuild still reporting zero errors, so nothing downstream noticed.
func restResultHandlingFromRaw(doc bson.Raw, handlingType string) microflows.ResultHandling {
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
	case handlingType == "FileDocument" ||
		(handlingType == "" && vtType == "DataTypes$ObjectType" && entity != "" && entity != "System.HttpResponse"):
		h := &microflows.ResultHandlingFileDocument{VariableName: resultVar, EntityRef: entity}
		h.ID = id
		return h
	case handlingType == "None" || vtType == "DataTypes$VoidType":
		h := &microflows.ResultHandlingNone{}
		h.ID = id
		return h
	case handlingType == "HttpResponse" ||
		(handlingType == "" && vtType == "DataTypes$ObjectType" && entity == "System.HttpResponse"):
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
	// The Range is polymorphic: a ConstantRange carries SingleObject (All/First)
	// while a CustomRange carries the limit and offset expressions. Reading only
	// SingleObject dropped the Custom setting entirely, so a describe→edit→exec
	// cycle turned a bounded import into an unbounded one (issue #881).
	if rng, ok := imc.Lookup("Range").DocumentOK(); ok {
		if rawStr(rng, "$Type") == "Microflows$CustomRange" {
			h.LimitExpression = rawStr(rng, "LimitExpression")
			h.OffsetExpression = rawStr(rng, "OffsetExpression")
		} else if b, ok := rng.Lookup("SingleObject").BooleanOK(); ok {
			// SingleObject is the RANGE's own flag (All/First). It is only a
			// fallback for the result variable's cardinality — where a
			// VariableType is stored, that wins, because the two disagree in
			// Mendix's own models (see RangeSingleObject).
			h.RangeSingleObject = &b
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
// carries anything the structured describe form can't reproduce, in which case
// the renderer emits the `call web service raw '<base64>'` fallback. Mirrors the
// legacy webServiceActionRequiresRawBSON decision exactly so both engines agree
// on when to fall back.
//
// The question is NOT "is this key known" but "would writing this back from the
// structured form produce the same document". Until the request body became
// authorable the two were the same, because the nine keys below were the only
// ones the writer emitted. They are not the same now: a real call carries
// fifteen, and six of the new ones are boilerplate mxcli writes at ONE fixed
// value. Admitting them by name would silently normalise a call with HTTP
// authentication, a custom location or a non-default proxy the moment anyone
// ran describe → exec on it — which is the silent drop this whole change exists
// to remove, reintroduced one layer up.
//
// So each of those six is admitted only AT that value. Anything else keeps the
// raw fallback, which round-trips byte for byte.
func webServiceActionRequiresRawBSON(raw bson.Raw) bool {
	// Keys the structured form carries in full, whatever their value.
	represented := map[string]bool{
		"$ID":               true,
		"$Type":             true,
		"ErrorHandlingType": true,
		"ImportedService":   true,
		"OperationName":     true,
		"TimeOutExpression": true,
		"RequestHandling":   true,
		// ServiceName is not written by DESCRIBE and does not need to be: the
		// write path re-reads it from the imported service document, which is
		// the authoritative source (that is the CE0386 fix). A call whose
		// service cannot be resolved is already broken.
		"ServiceName": true,
	}
	els, err := raw.Elements()
	if err != nil {
		return true
	}
	for _, el := range els {
		key := el.Key()
		if represented[key] {
			continue
		}
		if ok, known := webServiceFixedValueIsDefault(raw, key); known {
			if !ok {
				return true
			}
			continue
		}
		return true
	}
	return false
}

// webServiceFixedValueIsDefault reports whether one of the keys mxcli writes at a
// FIXED value currently holds that value. known is false for a key it does not
// judge, which the caller treats as unrepresentable.
func webServiceFixedValueIsDefault(raw bson.Raw, key string) (ok, known bool) {
	switch key {
	case "IsValidationRequired":
		v, isBool := raw.Lookup(key).BooleanOK()
		return isBool && !v, true
	case "UseRequestTimeOut":
		v, isBool := raw.Lookup(key).BooleanOK()
		return isBool && v, true
	case "RequestProxyType":
		return rawStr(raw, key) == "DefaultProxy", true
	case "ProxyConfiguration":
		return raw.Lookup(key).Type == bson.TypeNull, true
	case "HttpConfiguration":
		doc, isDoc := raw.Lookup(key).DocumentOK()
		return isDoc && isDefaultWebServiceHTTPConfig(doc), true
	case "RequestHeaderHandling":
		doc, isDoc := raw.Lookup(key).DocumentOK()
		return isDoc && isEmptySimpleRequestHandling(doc), true
	case "RequestBodyHandling":
		doc, isDoc := raw.Lookup(key).DocumentOK()
		return isDoc && webServiceRequestBodyIsRepresentable(doc), true
	case "NewResultHandling":
		doc, isDoc := raw.Lookup(key).DocumentOK()
		return isDoc && webServiceResultHandlingIsRepresentable(doc), true
	}
	return false, false
}

// webServiceResultHandlingIsRepresentable reports whether a call's result
// handling is one the writer reproduces exactly.
//
// This key used to be admitted by name, which was safe only while the six
// boilerplate keys kept every real call on the raw path anyway. A describe →
// exec round trip over ako/TestApp caught both ways it is not:
//
//   - Clients.SaveOrder binds $IsSaved with NO import mapping, and its
//     VariableType is DataTypes$BooleanType — the OPERATION's own return type,
//     which lives in the WSDL and is therefore not derivable from MDL. Written
//     back as VoidType it is two errors: CE0366 "Cannot store in variable when
//     there is no return value" and CE6011 "The type of the output variable does
//     not match the return type of the operation."
//   - Clients.GetOrders carries Range.SingleObject false where mxcli writes true
//     — the one divergence from the reference documents still unexplained. No
//     error comes of it, which is precisely why it must not be written silently:
//     the round trip would change the user's document with nothing to show for it.
//
// So a result handling is representable only when the writer's fixed values are
// already the stored ones. mxcli's own calls qualify; Studio Pro's do not, and
// keep the byte-exact raw fallback until SingleObject is settled.
func webServiceResultHandlingIsRepresentable(doc bson.Raw) bool {
	if rawStr(doc, "$Type") != "Microflows$ResultHandling" {
		return false
	}
	bound := rawStr(doc, "ResultVariableName") != ""
	if bind, ok := doc.Lookup("Bind").BooleanOK(); !ok || bind != bound {
		return false
	}
	imc, hasMapping := doc.Lookup("ImportMappingCall").DocumentOK()
	vt, hasType := doc.Lookup("VariableType").DocumentOK()
	if !hasType {
		return false
	}
	if !hasMapping {
		// No mapping: the writer emits VoidType, so only VoidType round-trips.
		return rawStr(vt, "$Type") == "DataTypes$VoidType"
	}
	if rawStr(vt, "$Type") != "DataTypes$ObjectType" {
		return false
	}
	if rawStr(imc, "$Type") != "Microflows$ImportMappingCall" ||
		rawStr(imc, "Commit") != "YesWithoutEvents" ||
		rawStr(imc, "ContentType") != "Xml" ||
		rawStr(imc, "ObjectHandlingBackup") != "Create" ||
		rawStr(imc, "ParameterVariableName") != "" ||
		rawStr(imc, "ReturnValueMapping") == "" {
		return false
	}
	if force, ok := imc.Lookup("ForceSingleOccurrence").BooleanOK(); !ok || force {
		return false
	}
	rng, ok := imc.Lookup("Range").DocumentOK()
	if !ok || rawStr(rng, "$Type") != "Microflows$ConstantRange" {
		return false
	}
	single, ok := rng.Lookup("SingleObject").BooleanOK()
	return ok && single
}

// isDefaultWebServiceHTTPConfig reports whether an HttpConfiguration is the one a
// SOAP call gets when nothing is configured — the only one mxcli writes.
func isDefaultWebServiceHTTPConfig(doc bson.Raw) bool {
	if rawStr(doc, "$Type") != "Microflows$HttpConfiguration" {
		return false
	}
	for _, key := range []string{"ClientCertificate", "CustomLocation",
		"HttpAuthenticationPassword", "HttpAuthenticationUserName"} {
		if rawStr(doc, key) != "" {
			return false
		}
	}
	if doc.Lookup("CustomLocationTemplate").Type != bson.TypeNull {
		return false
	}
	if rawStr(doc, "HttpMethod") != "Post" {
		return false
	}
	for key, want := range map[string]bool{"OverrideLocation": false, "UseHttpAuthentication": false} {
		v, isBool := doc.Lookup(key).BooleanOK()
		if !isBool || v != want {
			return false
		}
	}
	return len(rawDocElements(doc, "HttpHeaderEntries")) == 0
}

// isEmptySimpleRequestHandling reports whether a request handling is the bare
// Simple form — no parameter mappings — which is all mxcli writes for headers.
func isEmptySimpleRequestHandling(doc bson.Raw) bool {
	return rawStr(doc, "$Type") == "Microflows$SimpleRequestHandling" &&
		rawStr(doc, "NullValueOption") == "LeaveOutElement" &&
		len(rawDocElements(doc, "ParameterMappings")) == 0
}

// webServiceRequestBodyIsRepresentable reports whether a RequestBodyHandling is
// one MDL can spell: an export mapping, or simple parameter mappings whose names
// survive the round trip.
func webServiceRequestBodyIsRepresentable(doc bson.Raw) bool {
	switch rawStr(doc, "$Type") {
	case "Microflows$MappingRequestHandling":
		// All three properties are carried on the model and restated by MDL.
		return rawStr(doc, "MappingId") != "" && rawStr(doc, "MappingVariableName") != ""
	case "Microflows$SimpleRequestHandling":
		if rawStr(doc, "NullValueOption") != "LeaveOutElement" {
			return false
		}
		for _, pm := range rawDocElements(doc, "ParameterMappings") {
			// An ADVANCED mapping (a per-parameter export mapping) has no MDL
			// spelling at all, and a non-empty ParameterName is a shape no
			// reference document carries, so neither is judged representable.
			if rawStr(pm, "$Type") != "Microflows$WebServiceOperationSimpleParameterMapping" ||
				rawStr(pm, "ParameterName") != "" {
				return false
			}
			// The parameter name MDL spells is the path's last segment; without
			// one the write path could not rebuild the same path.
			if !strings.Contains(rawStr(pm, "ParameterPath"), "|") {
				return false
			}
		}
		return true
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
		// The bounds are one level down, on a Microflows$CustomRange child —
		// not on the ListRange itself. Reading them flat found nothing in every
		// document Mendix wrote, so a paged range described as the unbounded
		// `range($List)`: no warning, exit 0, and `mxcli check` clean on output
		// that re-executes with pagination silently removed. (issue #966)
		o := &microflows.ListRangeOperation{ListVariable: list}
		if cr, ok := doc.Lookup("CustomRange").DocumentOK(); ok {
			o.LimitExpression = rawStr(cr, "LimitExpression")
			o.OffsetExpression = rawStr(cr, "OffsetExpression")
		} else {
			// A project written by mxcli 0.18 or earlier has the bounds flat
			// here, because the writer put them there. Such a project does not
			// build (CE6520), but the expressions the author typed ARE on disk
			// — so reading them lets DESCRIBE show the range that was meant,
			// and re-executing that output stores the nested form and repairs
			// the project. Mendix itself never writes these keys, so the
			// fallback cannot misfire on a Studio Pro document.
			o.LimitExpression = rawStr(doc, "LimitExpression")
			o.OffsetExpression = rawStr(doc, "OffsetExpression")
		}
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

// errorHandlingTypeOf reads an action's ErrorHandlingType generically, by
// property name rather than by concrete type. Every Microflows$*Action that
// supports custom error handling stores it under this one key, so an action the
// reader has no mapping for can still surrender the one field DESCRIBE needs to
// keep its error handler attached (#863).
//
// Returns "" when the property is absent or is not a string-valued enum; callers
// treat that as "no explicit error handling", the same as the stored default.
func errorHandlingTypeOf(el element.Element) string {
	if el == nil {
		return ""
	}
	for _, p := range el.Properties() {
		if p.Name() != "ErrorHandlingType" {
			continue
		}
		if w, ok := p.(element.WritableProperty); ok {
			if s, ok := w.BSONValue().(string); ok {
				return s
			}
		}
		return ""
	}
	return ""
}

// rawDocElements returns the document entries of a versioned BSON array.
//
// Element 0 of a Mendix array is a version marker (an int32), never data, so it
// simply fails the document check and is skipped — the same shape the mapping
// readers above rely on.
// readWebServiceRequestBody reads a SOAP call's RequestBodyHandling — the
// polymorphic child holding EITHER the operation's arguments or an export
// mapping — back into the semantic model.
//
// Dispatched on $Type, never on which fields happen to be present: the two
// variants differ in arity, and assigning whichever keys are there would
// quietly turn one into the other.
//
// The parameter NAME is recovered from the stored ParameterPath's last segment,
// which is what MDL spells. A path with no "|" yields no name, so the argument
// is read with an empty Name and the describe path renders the action raw —
// better than inventing a name that would write a different path back.
func readWebServiceRequestBody(raw bson.Raw, out *microflows.WebServiceCallAction) {
	body, ok := raw.Lookup("RequestBodyHandling").DocumentOK()
	if !ok {
		return
	}
	switch rawStr(body, "$Type") {
	case "Microflows$MappingRequestHandling":
		// STORAGE NAMES: MappingId / MappingVariableName. gen binds the same two
		// as Mapping / MappingArgumentVariableName — both listed in its key
		// audit — so a reader keyed on gen's names finds nothing here.
		out.SendMappingID = model.ID(rawStr(body, "MappingId"))
		out.SendMappingVariable = rawStr(body, "MappingVariableName")
		out.SendMappingContentType = rawStr(body, "ContentType")
	case "Microflows$SimpleRequestHandling":
		for _, pm := range rawDocElements(body, "ParameterMappings") {
			if rawStr(pm, "$Type") != "Microflows$WebServiceOperationSimpleParameterMapping" {
				// An advanced (per-parameter export mapping) entry, which MDL
				// cannot author. Read nothing rather than half of it; the raw
				// fallback carries the action.
				continue
			}
			path := rawStr(pm, "ParameterPath")
			name := ""
			if i := strings.LastIndex(path, "|"); i >= 0 {
				name = path[i+1:]
			}
			// Absent reads as true: both reference mappings carry true, and a
			// bound parameter is by definition one Studio Pro has ticked.
			checked, ok := pm.Lookup("IsChecked").BooleanOK()
			if !ok {
				checked = true
			}
			out.Arguments = append(out.Arguments, microflows.WebServiceArgument{
				Name:       name,
				Path:       path,
				Expression: rawStr(pm, "Argument"),
				Checked:    checked,
			})
		}
	}
}

func rawDocElements(raw bson.Raw, key string) []bson.Raw {
	arr, ok := raw.Lookup(key).ArrayOK()
	if !ok {
		return nil
	}
	vals, err := arr.Values()
	if err != nil {
		return nil
	}
	out := make([]bson.Raw, 0, len(vals))
	for _, v := range vals {
		if md, ok := v.DocumentOK(); ok {
			out = append(out, md)
		}
	}
	return out
}

// queueSettingsFromRaw reads a call's Queues$QueueSettings child back into the
// semantic model. Retry is carried as raw storage rather than decoded: MDL
// cannot author one, and the rewrite guard needs to know it is there so it can
// refuse rather than drop it (guard-don't-drop, ADR-0005).
func queueSettingsFromRaw(raw bson.Raw) *microflows.QueueSettings {
	doc, ok := raw.Lookup("QueueSettings").DocumentOK()
	if !ok {
		return nil
	}
	qs := &microflows.QueueSettings{Queue: rawStr(doc, "Queue")}
	if v, err := doc.LookupErr("Retry"); err == nil && v.Type != bson.TypeNull {
		qs.Retry = v
	}
	return qs
}
