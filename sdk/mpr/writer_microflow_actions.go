// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"

	"go.mongodb.org/mongo-driver/bson"
)

// serializeMicroflowAction serializes a microflow action to BSON.
//
// IMPORTANT: Mendix uses different "storage names" vs "qualified names" for many types.
// The $Type field in BSON must use the STORAGE NAME, not the qualified name from the
// TypeScript SDK or metamodel documentation. Examples:
//
//	Qualified Name (SDK/docs)    Storage Name (BSON $Type)
//	-------------------------    -------------------------
//	CreateObjectAction           CreateChangeAction
//	ChangeObjectAction           ChangeAction
//	DeleteObjectAction           DeleteAction
//	CommitObjectsAction          CommitAction
//	RollbackObjectAction         RollbackAction
//	AggregateListAction          AggregateAction
//	ListOperationAction          ListOperationsAction
//	ShowPageAction               ShowFormAction        (Page was originally called Form)
//	ClosePageAction              CloseFormAction       (Page was originally called Form)
//
// Using the wrong type name causes "TypeCacheUnknownTypeException" when opening in Studio Pro.
// When adding new action types, check existing MPR files or reflection data for the storage name.
func serializeMicroflowAction(action microflows.MicroflowAction) bson.D {
	switch a := action.(type) {
	case *microflows.CastAction:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$CastAction"},
			{Key: "ErrorHandlingType", Value: "Rollback"},
			{Key: "VariableName", Value: a.OutputVariable},
		}

	case *microflows.CreateVariableAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$CreateVariableAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "VariableName", Value: a.VariableName},
			{Key: "InitialValue", Value: a.InitialValue},
		}
		if a.DataType != nil {
			doc = append(doc, bson.E{Key: "VariableType", Value: serializeMicroflowDataType(a.DataType)})
		}
		return doc

	case *microflows.ChangeVariableAction:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$ChangeVariableAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "ChangeVariableName", Value: a.VariableName},
			{Key: "Value", Value: a.Value},
		}

	case *microflows.CreateObjectAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$CreateChangeAction"}, // storageName differs from qualifiedName
			{Key: "Commit", Value: string(a.Commit)},
		}
		// Entity is BY_NAME_REFERENCE - use qualified name string
		if a.EntityQualifiedName != "" {
			doc = append(doc, bson.E{Key: "Entity", Value: a.EntityQualifiedName})
		}
		doc = append(doc, bson.E{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")})
		// Serialize Items (ChangeActionItem) for InitialMembers. Mendix stores
		// this list with storage-list marker 2, not with the item count.
		items := bson.A{int32(2)}
		for _, change := range a.InitialMembers {
			item := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(change.ID))},
				{Key: "$Type", Value: "Microflows$ChangeActionItem"},
			}
			// Association or Attribute as BY_NAME_REFERENCE (mutually exclusive)
			if change.AssociationQualifiedName != "" {
				item = append(item, bson.E{Key: "Association", Value: change.AssociationQualifiedName})
			} else {
				item = append(item, bson.E{Key: "Association", Value: ""}) // Empty for attributes
				if change.AttributeQualifiedName != "" {
					item = append(item, bson.E{Key: "Attribute", Value: change.AttributeQualifiedName})
				}
			}
			item = append(item, bson.E{Key: "Type", Value: string(change.Type)})
			item = append(item, bson.E{Key: "Value", Value: change.Value})
			items = append(items, item)
		}
		doc = append(doc, bson.E{Key: "Items", Value: items})
		// RefreshInClient is required
		doc = append(doc, bson.E{Key: "RefreshInClient", Value: false})
		// outputVariableName has storageName "VariableName"
		doc = append(doc, bson.E{Key: "VariableName", Value: a.OutputVariable})
		return doc

	case *microflows.ChangeObjectAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$ChangeAction"}, // storageName differs from qualifiedName
			{Key: "ChangeVariableName", Value: a.ChangeVariable},
			{Key: "Commit", Value: string(a.Commit)},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		}
		// Serialize Items (ChangeActionItem). Mendix stores this list with
		// storage-list marker 2, not with the item count.
		items := bson.A{int32(2)}
		for _, change := range a.Changes {
			item := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(change.ID))},
				{Key: "$Type", Value: "Microflows$ChangeActionItem"},
			}
			// Association or Attribute as BY_NAME_REFERENCE (mutually exclusive)
			if change.AssociationQualifiedName != "" {
				item = append(item, bson.E{Key: "Association", Value: change.AssociationQualifiedName})
			} else {
				item = append(item, bson.E{Key: "Association", Value: ""}) // Empty for attributes
				if change.AttributeQualifiedName != "" {
					item = append(item, bson.E{Key: "Attribute", Value: change.AttributeQualifiedName})
				}
			}
			item = append(item, bson.E{Key: "Type", Value: string(change.Type)})
			item = append(item, bson.E{Key: "Value", Value: change.Value})
			items = append(items, item)
		}
		doc = append(doc, bson.E{Key: "Items", Value: items})
		doc = append(doc, bson.E{Key: "RefreshInClient", Value: a.RefreshInClient})
		return doc

	case *microflows.CommitObjectsAction:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$CommitAction"},
			{Key: "CommitVariableName", Value: a.CommitVariable},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "RefreshInClient", Value: a.RefreshInClient},
			{Key: "WithEvents", Value: a.WithEvents},
		}

	case *microflows.DeleteObjectAction:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$DeleteAction"},
			{Key: "DeleteVariableName", Value: a.DeleteVariable},
			{Key: "RefreshInClient", Value: a.RefreshInClient},
		}

	case *microflows.RollbackObjectAction:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$RollbackAction"},
			{Key: "RollbackVariableName", Value: a.RollbackVariable},
			{Key: "RefreshInClient", Value: a.RefreshInClient},
		}

	case *microflows.LogMessageAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$LogMessageAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "IncludeLatestStackTrace", Value: false},
			{Key: "Level", Value: string(a.LogLevel)},
			{Key: "Node", Value: a.LogNodeName}, // Already stored as expression (e.g., "'TEST'")
		}
		if a.MessageTemplate != nil {
			doc = append(doc, bson.E{Key: "MessageTemplate", Value: serializeStringTemplate(a.MessageTemplate, a.TemplateParameters)})
		}
		return doc

	case *microflows.CallExternalAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$CallExternalAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "ConsumedODataService", Value: a.ConsumedODataService},
			{Key: "Name", Value: a.Name},
			{Key: "VariableName", Value: a.ResultVariableName},
		}
		// Issue: Mendix's CallExternalAction.Check raises CE7269 ("return type
		// for remote action has changed") when the stored VariableDataType
		// doesn't match the cached schema's return type. Always emit
		// VariableDataType when we know the schema kind — the executor
		// resolves it from the consumed service's cached $metadata.
		if a.ResultDataType != "" {
			doc = append(doc, bson.E{Key: "VariableDataType", Value: serializeExternalActionReturnType(a.ResultDataType)})
		}
		// Serialize parameter mappings
		if len(a.ParameterMappings) > 0 {
			var mappings bson.A
			mappings = append(mappings, int32(3)) // Array marker (storageListType 3)
			for _, pm := range a.ParameterMappings {
				mapping := bson.D{
					{Key: "$ID", Value: idToBsonBinary(string(pm.ID))},
					{Key: "$Type", Value: "Microflows$ExternalActionParameterMapping"},
					{Key: "ParameterName", Value: pm.ParameterName},
					{Key: "Argument", Value: pm.Argument},
					{Key: "CanBeEmpty", Value: pm.CanBeEmpty},
				}
				mappings = append(mappings, mapping)
			}
			doc = append(doc, bson.E{Key: "ParameterMappings", Value: mappings})
		} else {
			doc = append(doc, bson.E{Key: "ParameterMappings", Value: bson.A{int32(3)}})
		}
		return doc

	case *microflows.MicroflowCallAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$MicroflowCallAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		}
		// Serialize nested MicroflowCall structure
		if a.MicroflowCall != nil {
			mfCall := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(a.MicroflowCall.ID))},
				{Key: "$Type", Value: "Microflows$MicroflowCall"},
				{Key: "Microflow", Value: a.MicroflowCall.Microflow},
			}
			// Serialize parameter mappings within MicroflowCall
			if len(a.MicroflowCall.ParameterMappings) > 0 {
				var mappings bson.A
				mappings = append(mappings, int32(2)) // Array marker
				for _, pm := range a.MicroflowCall.ParameterMappings {
					mapping := bson.D{
						{Key: "$ID", Value: idToBsonBinary(string(pm.ID))},
						{Key: "$Type", Value: "Microflows$MicroflowCallParameterMapping"},
						{Key: "Argument", Value: pm.Argument},
						{Key: "Parameter", Value: pm.Parameter},
					}
					mappings = append(mappings, mapping)
				}
				mfCall = append(mfCall, bson.E{Key: "ParameterMappings", Value: mappings})
			} else {
				mfCall = append(mfCall, bson.E{Key: "ParameterMappings", Value: bson.A{int32(2)}}) // Empty array with marker
			}
			mfCall = append(mfCall, bson.E{Key: "QueueSettings", Value: nil})
			doc = append(doc, bson.E{Key: "MicroflowCall", Value: mfCall})
		}
		doc = append(doc,
			bson.E{Key: "ResultVariableName", Value: a.ResultVariableName},
			bson.E{Key: "UseReturnVariable", Value: a.UseReturnVariable},
		)
		return doc

	case *microflows.NanoflowCallAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$NanoflowCallAction"},
			// Mendix metamodel defaults to "Rollback" for all call actions, including nanoflow calls.
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "OutputVariableName", Value: a.OutputVariableName},
			{Key: "UseReturnVariable", Value: a.UseReturnVariable},
		}
		if a.NanoflowCall != nil {
			nfCall := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(a.NanoflowCall.ID))},
				{Key: "$Type", Value: "Microflows$NanoflowCall"},
				{Key: "Nanoflow", Value: a.NanoflowCall.Nanoflow},
			}
			if len(a.NanoflowCall.ParameterMappings) > 0 {
				var mappings bson.A
				mappings = append(mappings, int32(2))
				for _, pm := range a.NanoflowCall.ParameterMappings {
					mapping := bson.D{
						{Key: "$ID", Value: idToBsonBinary(string(pm.ID))},
						{Key: "$Type", Value: "Microflows$NanoflowCallParameterMapping"},
						{Key: "Parameter", Value: pm.Parameter},
						{Key: "Argument", Value: pm.Argument},
					}
					mappings = append(mappings, mapping)
				}
				nfCall = append(nfCall, bson.E{Key: "ParameterMappings", Value: mappings})
			} else {
				nfCall = append(nfCall, bson.E{Key: "ParameterMappings", Value: bson.A{int32(2)}})
			}
			doc = append(doc, bson.E{Key: "NanoflowCall", Value: nfCall})
		}
		return doc

	case *microflows.JavaActionCallAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$JavaActionCallAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "JavaAction", Value: a.JavaAction},
			{Key: "QueueSettings", Value: nil},
			{Key: "ResultVariableName", Value: a.ResultVariableName},
			{Key: "UseReturnVariable", Value: a.UseReturnVariable},
		}
		// Serialize parameter mappings
		if len(a.ParameterMappings) > 0 {
			var mappings bson.A
			mappings = append(mappings, int32(2)) // Array marker
			for _, pm := range a.ParameterMappings {
				mapping := bson.D{
					{Key: "$ID", Value: idToBsonBinary(string(pm.ID))},
					{Key: "$Type", Value: "Microflows$JavaActionParameterMapping"},
					{Key: "Parameter", Value: pm.Parameter},
				}
				// Serialize Value (CodeActionParameterValue)
				if pm.Value != nil {
					mapping = append(mapping, bson.E{Key: "Value", Value: serializeCodeActionParameterValue(pm.Value)})
				}
				mappings = append(mappings, mapping)
			}
			doc = append(doc, bson.E{Key: "ParameterMappings", Value: mappings})
		} else {
			doc = append(doc, bson.E{Key: "ParameterMappings", Value: bson.A{int32(2)}}) // Empty array with marker
		}
		return doc

	case *microflows.JavaScriptActionCallAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$JavaScriptActionCallAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "JavaScriptAction", Value: a.JavaScriptAction},
			{Key: "OutputVariableName", Value: a.OutputVariableName},
			{Key: "UseReturnVariable", Value: a.UseReturnVariable},
		}
		// Serialize parameter mappings
		if len(a.ParameterMappings) > 0 {
			var mappings bson.A
			mappings = append(mappings, int32(2)) // Array marker
			for _, pm := range a.ParameterMappings {
				mapping := bson.D{
					{Key: "$ID", Value: idToBsonBinary(string(pm.ID))},
					{Key: "$Type", Value: "Microflows$JavaScriptActionParameterMapping"},
					{Key: "Parameter", Value: pm.Parameter},
				}
				// Serialize ParameterValue (CodeActionParameterValue) — JS uses "ParameterValue" key, not "Value"
				if pm.Value != nil {
					mapping = append(mapping, bson.E{Key: "ParameterValue", Value: serializeCodeActionParameterValue(pm.Value)})
				}
				mappings = append(mappings, mapping)
			}
			doc = append(doc, bson.E{Key: "ParameterMappings", Value: mappings})
		} else {
			doc = append(doc, bson.E{Key: "ParameterMappings", Value: bson.A{int32(2)}}) // Empty array with marker
		}
		return doc

	case *microflows.RetrieveAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$RetrieveAction"},
			{Key: "ErrorHandlingType", Value: "Rollback"},        // Default error handling type
			{Key: "ResultVariableName", Value: a.OutputVariable}, // storageName differs from qualifiedName
		}
		if a.Source != nil {
			switch src := a.Source.(type) {
			case *microflows.DatabaseRetrieveSource:
				doc = append(doc, bson.E{Key: "RetrieveSource", Value: serializeDatabaseRetrieveSource(src)})
			case *microflows.AssociationRetrieveSource:
				doc = append(doc, bson.E{Key: "RetrieveSource", Value: serializeAssociationRetrieveSource(src)})
			}
		}
		return doc

	case *microflows.ListOperationAction:
		return serializeListOperationAction(a)

	case *microflows.AggregateListAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$AggregateAction"}, // storageName differs from qualifiedName
			{Key: "ErrorHandlingType", Value: "Rollback"},
		}
		doc = append(doc, bson.E{Key: "AggregateFunction", Value: string(a.Function)})
		doc = append(doc, bson.E{Key: "AggregateVariableName", Value: a.InputVariable}) // storageName for inputListVariableName
		if a.UseExpression {
			doc = append(doc, bson.E{Key: "UseExpression", Value: true})
			doc = append(doc, bson.E{Key: "Expression", Value: a.Expression})
		} else if a.AttributeQualifiedName != "" {
			// Attribute is BY_NAME_REFERENCE
			doc = append(doc, bson.E{Key: "Attribute", Value: a.AttributeQualifiedName})
		}
		doc = append(doc, bson.E{Key: "VariableName", Value: a.OutputVariable}) // storageName for outputVariableName
		return doc

	case *microflows.CreateListAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$CreateListAction"},
			{Key: "ErrorHandlingType", Value: "Rollback"},
		}
		// Entity is BY_NAME_REFERENCE
		if a.EntityQualifiedName != "" {
			doc = append(doc, bson.E{Key: "Entity", Value: a.EntityQualifiedName})
		}
		doc = append(doc, bson.E{Key: "VariableName", Value: a.OutputVariable}) // storageName for outputVariableName
		return doc

	case *microflows.ChangeListAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$ChangeListAction"},
			{Key: "ErrorHandlingType", Value: "Rollback"},
			{Key: "ChangeVariableName", Value: a.ChangeVariable},
			{Key: "Type", Value: string(a.Type)},
		}
		if a.Value != "" {
			doc = append(doc, bson.E{Key: "Value", Value: a.Value})
		}
		return doc

	case *microflows.ShowPageAction:
		// ShowFormAction uses FormSettings with Form as BY_NAME_REFERENCE (not Page as BY_ID_REFERENCE)
		// This is the modern format used by Mendix 10+
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$ShowFormAction"}, // storageName differs from qualifiedName
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		}

		// FormSettings contains Form (BY_NAME_REFERENCE) and ParameterMappings
		formSettingsID := a.FormSettingsID
		if formSettingsID == "" {
			formSettingsID = model.ID(generateUUID())
		}

		// Build ParameterMappings inside FormSettings. Mendix storage lists use
		// a marker as the first element; it is not the number of mappings.
		paramMappings := bson.A{int32(2)}
		for _, pm := range a.PageParameterMappings {
			mapping := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(pm.ID))},
				{Key: "$Type", Value: "Forms$PageParameterMapping"}, // Forms$, not Microflows$
				{Key: "Argument", Value: pm.Argument},
				{Key: "Parameter", Value: pm.Parameter}, // BY_NAME_REFERENCE
				{Key: "Variable", Value: emptyPageVariable()},
			}
			paramMappings = append(paramMappings, mapping)
		}

		// Build FormSettings
		formSettings := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(formSettingsID))},
			{Key: "$Type", Value: "Forms$FormSettings"},
			{Key: "Form", Value: a.PageName}, // BY_NAME_REFERENCE (page qualified name)
			{Key: "ParameterMappings", Value: paramMappings},
			{Key: "TitleOverride", Value: emptyTextTemplate()},
		}
		doc = append(doc, bson.E{Key: "FormSettings", Value: formSettings})
		doc = append(doc, bson.E{Key: "NumberOfPagesToClose", Value: ""})

		return doc

	case *microflows.ClosePageAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$CloseFormAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			// The storage name is "NumberOfPages" (matches the metamodel/codec). The
			// old "NumberOfPagesToClose" was tolerated by Mendix <= 11.6 but rejected
			// by 11.12 (CE0117 "Error(s) in expression" — the real NumberOfPages field
			// is then absent and defaults to an empty expression).
			{Key: "NumberOfPages", Value: int32(a.NumberOfPages)},
		}
		return doc

	case *microflows.ShowHomePageAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$ShowHomePageAction"},
			{Key: "ErrorHandlingType", Value: "Rollback"},
		}
		return doc

	case *microflows.ShowMessageAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$ShowMessageAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "Type", Value: string(a.Type)},
			{Key: "Blocking", Value: a.Blocking},
			{Key: "Template", Value: serializeTextTemplate(a.Template, a.TemplateParameters)},
		}
		return doc

	case *microflows.DownloadFileAction:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$DownloadFileAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "FileDocumentVariableName", Value: a.FileDocument},
			{Key: "ShowInBrowser", Value: a.ShowInBrowser},
		}

	case *microflows.ValidationFeedbackAction:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
			{Key: "$Type", Value: "Microflows$ValidationFeedbackAction"},
			{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
			{Key: "ValidationVariableName", Value: a.ObjectVariable},
		}
		// Always write both Attribute and Association fields — they are mutually
		// exclusive but Mendix expects both present (empty string when not set).
		// Follows the same pattern as ChangeObjectAction serialization.
		if a.AssociationName != "" {
			doc = append(doc, bson.E{Key: "Association", Value: a.AssociationName})
			doc = append(doc, bson.E{Key: "Attribute", Value: ""})
		} else {
			doc = append(doc, bson.E{Key: "Association", Value: ""})
			doc = append(doc, bson.E{Key: "Attribute", Value: a.AttributeName})
		}
		// Serialize FeedbackTemplate as Microflows$TextTemplate
		if a.Template != nil {
			doc = append(doc, bson.E{Key: "FeedbackTemplate", Value: serializeTextTemplate(a.Template, a.TemplateParameters)})
		}
		return doc

	case *microflows.RestCallAction:
		return serializeRestCallAction(a)

	case *microflows.WebServiceCallAction:
		return serializeWebServiceCallAction(a)

	case *microflows.RestOperationCallAction:
		return serializeRestOperationCallAction(a)

	case *microflows.ExecuteDatabaseQueryAction:
		return serializeExecuteDatabaseQueryAction(a)

	case *microflows.ImportXmlAction:
		return serializeImportXmlAction(a)

	case *microflows.ExportXmlAction:
		return serializeExportXmlAction(a)

	case *microflows.TransformJsonAction:
		return serializeTransformJsonAction(a)

	// Workflow actions
	case *microflows.WorkflowCallAction:
		return serializeWorkflowCallAction(a)
	case *microflows.GetWorkflowDataAction:
		return serializeGetWorkflowDataAction(a)
	case *microflows.GetWorkflowsAction:
		return serializeGetWorkflowsAction(a)
	case *microflows.GetWorkflowActivityRecordsAction:
		return serializeGetWorkflowActivityRecordsAction(a)
	case *microflows.WorkflowOperationAction:
		return serializeWorkflowOperationAction(a)
	case *microflows.SetTaskOutcomeAction:
		return serializeSetTaskOutcomeAction(a)
	case *microflows.OpenUserTaskAction:
		return serializeOpenUserTaskAction(a)
	case *microflows.NotifyWorkflowAction:
		return serializeNotifyWorkflowAction(a)
	case *microflows.OpenWorkflowAction:
		return serializeOpenWorkflowAction(a)
	case *microflows.LockWorkflowAction:
		return serializeLockWorkflowAction(a)
	case *microflows.UnlockWorkflowAction:
		return serializeUnlockWorkflowAction(a)

	default:
		return nil
	}
}

func emptyPageVariable() bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$PageVariable"},
		{Key: "PageParameter", Value: ""},
		{Key: "SnippetParameter", Value: ""},
		{Key: "UseAllPages", Value: false},
		{Key: "Widget", Value: ""},
	}
}

// emptyTextTemplate returns an empty Microflows$TextTemplate embedded object.
// TitleOverride on Forms$FormSettings is a Microflows$TextTemplate (not a scalar),
// so it must be written as an empty object rather than nil — same pattern as
// emptyPageVariable() for Forms$PageVariable (see PR #338 / issue #295).
func emptyTextTemplate() bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Microflows$TextTemplate"},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Text", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(2)}},
		}},
	}
}

// serializeRestCallAction serializes a RestCallAction to BSON.
// Storage name is "Microflows$RestCallAction" (same as qualified name).
func serializeRestCallAction(a *microflows.RestCallAction) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$RestCallAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "ErrorResultHandlingType", Value: "HttpResponse"},
	}

	// Serialize HttpConfiguration
	if a.HttpConfiguration != nil {
		httpConfig := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.HttpConfiguration.ID))},
			{Key: "$Type", Value: "Microflows$HttpConfiguration"},
			{Key: "ClientCertificate", Value: ""},
			{Key: "CustomLocation", Value: ""},
		}
		// Serialize CustomLocationTemplate as StringTemplate
		if a.HttpConfiguration.LocationTemplate != "" {
			customLocTemplate := bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "Microflows$StringTemplate"},
				{Key: "Text", Value: a.HttpConfiguration.LocationTemplate},
			}
			// Add parameters if present - each must be wrapped in TemplateParameter object
			if len(a.HttpConfiguration.LocationParams) > 0 {
				var params bson.A
				params = append(params, int32(2)) // Array marker
				for _, p := range a.HttpConfiguration.LocationParams {
					templateParam := bson.D{
						{Key: "$ID", Value: idToBsonBinary(GenerateID())},
						{Key: "$Type", Value: "Microflows$TemplateParameter"},
						{Key: "Expression", Value: p},
					}
					params = append(params, templateParam)
				}
				customLocTemplate = append(customLocTemplate, bson.E{Key: "Parameters", Value: params})
			} else {
				customLocTemplate = append(customLocTemplate, bson.E{Key: "Parameters", Value: bson.A{int32(2)}})
			}
			httpConfig = append(httpConfig, bson.E{Key: "CustomLocationTemplate", Value: customLocTemplate})
		}
		httpConfig = append(httpConfig,
			bson.E{Key: "HttpAuthenticationPassword", Value: a.HttpConfiguration.Password},
			bson.E{Key: "HttpAuthenticationUserName", Value: a.HttpConfiguration.Username},
		)
		// Serialize HttpHeaderEntries
		if len(a.HttpConfiguration.CustomHeaders) > 0 {
			var headers bson.A
			headers = append(headers, int32(2)) // Array marker
			for _, h := range a.HttpConfiguration.CustomHeaders {
				header := bson.D{
					{Key: "$ID", Value: idToBsonBinary(string(h.ID))},
					{Key: "$Type", Value: "Microflows$HttpHeaderEntry"},
					{Key: "Key", Value: h.Name},
					{Key: "Value", Value: h.Value},
				}
				headers = append(headers, header)
			}
			httpConfig = append(httpConfig, bson.E{Key: "HttpHeaderEntries", Value: headers})
		} else {
			httpConfig = append(httpConfig, bson.E{Key: "HttpHeaderEntries", Value: bson.A{int32(2)}})
		}
		httpConfig = append(httpConfig,
			bson.E{Key: "HttpMethod", Value: string(a.HttpConfiguration.HttpMethod)},
			bson.E{Key: "OverrideLocation", Value: true},
			bson.E{Key: "UseHttpAuthentication", Value: a.HttpConfiguration.UseAuthentication},
		)
		doc = append(doc, bson.E{Key: "HttpConfiguration", Value: httpConfig})
	}

	doc = append(doc, bson.E{Key: "ProxyConfiguration", Value: nil})

	// Serialize RequestHandling
	if a.RequestHandling != nil {
		doc = append(doc, bson.E{Key: "RequestHandling", Value: serializeRestRequestHandling(a.RequestHandling)})
	}

	// RequestHandlingType and RequestProxyType are at action level
	doc = append(doc,
		bson.E{Key: "RequestHandlingType", Value: "Custom"},
		bson.E{Key: "RequestProxyType", Value: "DefaultProxy"},
	)

	// Serialize ResultHandling
	resultHandlingType := "String" // default
	if a.ResultHandling != nil {
		doc = append(doc, bson.E{Key: "ResultHandling", Value: serializeRestResultHandling(a.ResultHandling, a.OutputVariable)})
		switch a.ResultHandling.(type) {
		case *microflows.ResultHandlingString:
			resultHandlingType = "String"
		case *microflows.ResultHandlingHttpResponse:
			resultHandlingType = "HttpResponse"
		case *microflows.ResultHandlingMapping:
			resultHandlingType = "Mapping"
		case *microflows.ResultHandlingNone:
			resultHandlingType = "None"
		}
	}
	doc = append(doc, bson.E{Key: "ResultHandlingType", Value: resultHandlingType})

	// Timeout
	if a.TimeoutExpression != "" {
		doc = append(doc,
			bson.E{Key: "TimeOutExpression", Value: a.TimeoutExpression},
			bson.E{Key: "UseRequestTimeOut", Value: true},
		)
	} else {
		doc = append(doc,
			bson.E{Key: "TimeOutExpression", Value: "300"},
			bson.E{Key: "UseRequestTimeOut", Value: true},
		)
	}

	return doc
}

func serializeWebServiceCallAction(a *microflows.WebServiceCallAction) bson.D {
	if len(a.RawBSON) > 0 {
		var raw bson.D
		if err := bson.Unmarshal(a.RawBSON, &raw); err == nil {
			return raw
		}
	}

	// ServiceName: use the local part of the qualified name (after the last dot).
	serviceName := string(a.ServiceID)
	if idx := strings.LastIndex(serviceName, "."); idx >= 0 {
		serviceName = serviceName[idx+1:]
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$CallWebServiceAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "HttpConfiguration", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$HttpConfiguration"},
			{Key: "ClientCertificate", Value: ""},
			{Key: "CustomLocation", Value: ""},
			{Key: "CustomLocationTemplate", Value: nil},
			{Key: "HttpAuthenticationPassword", Value: ""},
			{Key: "HttpAuthenticationUserName", Value: ""},
			{Key: "HttpHeaderEntries", Value: bson.A{int32(3)}},
			{Key: "HttpMethod", Value: "Post"},
			{Key: "OverrideLocation", Value: false},
			{Key: "UseHttpAuthentication", Value: false},
		}},
		// ImportedService is a BY_NAME_REFERENCE qualified name string, not a binary UUID.
		{Key: "ImportedService", Value: string(a.ServiceID)},
		{Key: "IsValidationRequired", Value: false},
	}

	// NewResultHandling uses Microflows$ResultHandling (same type as REST result handling).
	bind := a.OutputVariable != ""
	resultHandling := bson.D{
		{Key: "$ID", Value: idToBsonBinary(GenerateID())},
		{Key: "$Type", Value: "Microflows$ResultHandling"},
		{Key: "Bind", Value: bind},
	}
	if a.ReceiveMappingID != "" {
		// ReturnValueMapping is a BY_NAME_REFERENCE string, not a binary UUID.
		resultHandling = append(resultHandling, bson.E{Key: "ImportMappingCall", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$ImportMappingCall"},
			{Key: "Commit", Value: "YesWithoutEvents"},
			{Key: "ContentType", Value: "Json"},
			{Key: "ForceSingleOccurrence", Value: false},
			{Key: "ObjectHandlingBackup", Value: "Create"},
			{Key: "ParameterVariableName", Value: ""},
			{Key: "Range", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "Microflows$ConstantRange"},
				{Key: "SingleObject", Value: true},
			}},
			{Key: "ReturnValueMapping", Value: string(a.ReceiveMappingID)},
		}})
	} else {
		resultHandling = append(resultHandling, bson.E{Key: "ImportMappingCall", Value: nil})
	}
	resultHandling = append(resultHandling,
		bson.E{Key: "ResultVariableName", Value: a.OutputVariable},
		bson.E{Key: "VariableType", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "DataTypes$VoidType"},
		}},
	)
	doc = append(doc, bson.E{Key: "NewResultHandling", Value: resultHandling})

	doc = append(doc, bson.E{Key: "OperationName", Value: a.OperationName})
	doc = append(doc, bson.E{Key: "ProxyConfiguration", Value: nil})

	// RequestBodyHandling: always SimpleRequestHandling. Mendix$AdvancedRequestHandling
	// (used when a send mapping is configured) requires a Studio Pro-generated example
	// to determine the correct type storage name; use the raw BSON escape hatch for
	// complex SOAP operations that need a send mapping until that is resolved.
	doc = append(doc, bson.E{Key: "RequestBodyHandling", Value: bson.D{
		{Key: "$ID", Value: idToBsonBinary(GenerateID())},
		{Key: "$Type", Value: "Microflows$SimpleRequestHandling"},
		{Key: "NullValueOption", Value: "LeaveOutElement"},
		{Key: "ParameterMappings", Value: bson.A{int32(2)}},
	}})

	// RequestHeaderHandling is always SimpleRequestHandling.
	doc = append(doc, bson.E{Key: "RequestHeaderHandling", Value: bson.D{
		{Key: "$ID", Value: idToBsonBinary(GenerateID())},
		{Key: "$Type", Value: "Microflows$SimpleRequestHandling"},
		{Key: "NullValueOption", Value: "LeaveOutElement"},
		{Key: "ParameterMappings", Value: bson.A{int32(2)}},
	}})

	doc = append(doc, bson.E{Key: "RequestProxyType", Value: "DefaultProxy"})
	doc = append(doc, bson.E{Key: "ServiceName", Value: serviceName})
	doc = append(doc,
		bson.E{Key: "TimeOutExpression", Value: stringOrDefault(a.TimeoutExpression, "300")},
		bson.E{Key: "UseRequestTimeOut", Value: true},
	)
	return doc
}

// serializeRestOperationCallAction serializes a Microflows$RestOperationCallAction to BSON.
// Note: RestOperationCallAction does not support custom ErrorHandlingType (CE6035).
func serializeRestOperationCallAction(a *microflows.RestOperationCallAction) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$RestOperationCallAction"},
		{Key: "Operation", Value: a.Operation},
	}

	// OutputVariable
	if a.OutputVariable != nil {
		ov := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.OutputVariable.ID))},
			{Key: "$Type", Value: "Microflows$OutputVariable"},
			{Key: "VariableName", Value: a.OutputVariable.VariableName},
		}
		doc = append(doc, bson.E{Key: "OutputVariable", Value: ov})
	} else {
		doc = append(doc, bson.E{Key: "OutputVariable", Value: nil})
	}

	// BodyVariable
	if a.BodyVariable != nil {
		bv := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(a.BodyVariable.ID))},
			{Key: "$Type", Value: "Microflows$BodyVariable"},
			{Key: "VariableName", Value: a.BodyVariable.VariableName},
		}
		doc = append(doc, bson.E{Key: "BodyVariable", Value: bv})
	} else {
		doc = append(doc, bson.E{Key: "BodyVariable", Value: nil})
	}

	doc = append(doc, bson.E{Key: "BaseUrlParameterMapping", Value: nil})

	// ParameterMappings (path params)
	paramMappings := bson.A{int32(3)}
	for _, pm := range a.ParameterMappings {
		paramMappings = append(paramMappings, bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$ParameterMapping"},
			{Key: "Parameter", Value: pm.Parameter},
			{Key: "Value", Value: pm.Value},
		})
	}
	doc = append(doc, bson.E{Key: "ParameterMappings", Value: paramMappings})

	// QueryParameterMappings
	queryMappings := bson.A{int32(3)}
	for _, qm := range a.QueryParameterMappings {
		queryMappings = append(queryMappings, bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$QueryParameterMapping"},
			{Key: "QueryParameter", Value: qm.Parameter},
			{Key: "Value", Value: qm.Value},
			{Key: "Included", Value: qm.Included},
		})
	}
	doc = append(doc, bson.E{Key: "QueryParameterMappings", Value: queryMappings})

	return doc
}

// serializeRestRequestHandling serializes RequestHandling to BSON.
func serializeRestRequestHandling(rh microflows.RequestHandling) bson.D {
	switch h := rh.(type) {
	case *microflows.CustomRequestHandling:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(h.ID))},
			{Key: "$Type", Value: "Microflows$CustomRequestHandling"},
		}
		// Serialize Template as StringTemplate
		template := bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$StringTemplate"},
			{Key: "Text", Value: h.Template},
		}
		// Add parameters - each must be wrapped in TemplateParameter object
		if len(h.TemplateParams) > 0 {
			var params bson.A
			params = append(params, int32(2)) // Array marker
			for _, p := range h.TemplateParams {
				templateParam := bson.D{
					{Key: "$ID", Value: idToBsonBinary(GenerateID())},
					{Key: "$Type", Value: "Microflows$TemplateParameter"},
					{Key: "Expression", Value: p},
				}
				params = append(params, templateParam)
			}
			template = append(template, bson.E{Key: "Parameters", Value: params})
		} else {
			template = append(template, bson.E{Key: "Parameters", Value: bson.A{int32(2)}})
		}
		doc = append(doc, bson.E{Key: "Template", Value: template})
		return doc

	case *microflows.MappingRequestHandling:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(h.ID))},
			{Key: "$Type", Value: "Microflows$MappingRequestHandling"},
			{Key: "MappingId", Value: idToBsonBinary(string(h.MappingID))},
			{Key: "ContentType", Value: h.ContentType},
			{Key: "ParameterVariable", Value: h.ParameterVariable},
		}

	case *microflows.SimpleRequestHandling:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(h.ID))},
			{Key: "$Type", Value: "Microflows$SimpleRequestHandling"},
		}

	default:
		// Default to empty custom request handling
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$CustomRequestHandling"},
			{Key: "RequestHandlingType", Value: "Custom"},
			{Key: "Template", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "Microflows$StringTemplate"},
				{Key: "Text", Value: ""},
				{Key: "Parameters", Value: bson.A{int32(2)}},
			}},
			{Key: "RequestProxyType", Value: "DefaultProxy"},
		}
	}
}

// serializeRestResultHandling serializes ResultHandling to BSON.
// Note: ResultHandlingType is serialized at the action level, not here.
func serializeRestResultHandling(rh microflows.ResultHandling, outputVar string) bson.D {
	switch h := rh.(type) {
	case *microflows.ResultHandlingString:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(h.ID))},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "Bind", Value: outputVar != ""},
			{Key: "ImportMappingCall", Value: nil},
		}
		if outputVar != "" {
			doc = append(doc,
				bson.E{Key: "ResultVariableName", Value: outputVar},
				bson.E{Key: "VariableType", Value: bson.D{
					{Key: "$ID", Value: idToBsonBinary(GenerateID())},
					{Key: "$Type", Value: "DataTypes$StringType"},
				}},
			)
		} else {
			doc = append(doc,
				bson.E{Key: "ResultVariableName", Value: ""},
				bson.E{Key: "VariableType", Value: bson.D{
					{Key: "$ID", Value: idToBsonBinary(GenerateID())},
					{Key: "$Type", Value: "DataTypes$StringType"},
				}},
			)
		}
		return doc

	case *microflows.ResultHandlingMapping:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(h.ID))},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "Bind", Value: true},
		}
		// ImportMappingCall uses ReturnValueMapping (Studio Pro field name) with
		// all required fields to make the mapping link visible in Studio Pro.
		forceSingleOccurrence := h.SingleObject
		if h.ForceSingleOccurrence != nil {
			forceSingleOccurrence = *h.ForceSingleOccurrence
		}
		importCall := bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$ImportMappingCall"},
			{Key: "Commit", Value: "YesWithoutEvents"},
			{Key: "ContentType", Value: "Json"},
			{Key: "ForceSingleOccurrence", Value: forceSingleOccurrence},
			{Key: "ObjectHandlingBackup", Value: "Create"},
			{Key: "ParameterVariableName", Value: ""},
			{Key: "Range", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "Microflows$ConstantRange"},
				{Key: "SingleObject", Value: h.SingleObject},
			}},
			{Key: "ReturnValueMapping", Value: string(h.MappingID)},
		}
		doc = append(doc, bson.E{Key: "ImportMappingCall", Value: importCall})
		// VariableType: ObjectType for single-object mappings, ListType for multi-object.
		varTypeID := idToBsonBinary(GenerateID())
		var varType bson.D
		if h.SingleObject {
			varType = bson.D{
				{Key: "$ID", Value: varTypeID},
				{Key: "$Type", Value: "DataTypes$ObjectType"},
			}
		} else {
			varType = bson.D{
				{Key: "$ID", Value: varTypeID},
				{Key: "$Type", Value: "DataTypes$ListType"},
			}
		}
		if h.ResultEntityID != "" {
			varType = append(varType, bson.E{Key: "Entity", Value: string(h.ResultEntityID)})
		}
		doc = append(doc,
			bson.E{Key: "ResultVariableName", Value: h.ResultVariable},
			bson.E{Key: "VariableType", Value: varType},
		)
		return doc

	case *microflows.ResultHandlingHttpResponse:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(h.ID))},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "Bind", Value: outputVar != ""},
			{Key: "ImportMappingCall", Value: nil},
			{Key: "ResultVariableName", Value: outputVar},
			{Key: "VariableType", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "DataTypes$ObjectType"},
				{Key: "Entity", Value: "System.HttpResponse"},
			}},
		}

	case *microflows.ResultHandlingNone:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(h.ID))},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "Bind", Value: false},
			{Key: "ImportMappingCall", Value: nil},
			{Key: "ResultVariableName", Value: ""},
			{Key: "VariableType", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "DataTypes$VoidType"},
			}},
		}

	default:
		// Default to string result handling
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "Bind", Value: outputVar != ""},
			{Key: "ImportMappingCall", Value: nil},
			{Key: "ResultVariableName", Value: outputVar},
			{Key: "VariableType", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(GenerateID())},
				{Key: "$Type", Value: "DataTypes$StringType"},
			}},
		}
	}
}

// serializeListOperationAction serializes a ListOperationAction to BSON.
func serializeListOperationAction(a *microflows.ListOperationAction) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$ListOperationsAction"}, // storageName differs from qualifiedName
		{Key: "ErrorHandlingType", Value: "Rollback"},
	}

	// Serialize the operation - storage name is "NewOperation"
	if a.Operation != nil {
		doc = append(doc, bson.E{Key: "NewOperation", Value: serializeListOperation(a.Operation)})
	}
	doc = append(doc, bson.E{Key: "ResultVariableName", Value: a.OutputVariable}) // storageName differs
	return doc
}

// serializeListOperation serializes a ListOperation to BSON.
// Storage names differ from qualified names in Mendix metamodel.
func serializeListOperation(op microflows.ListOperation) bson.D {
	switch o := op.(type) {
	case *microflows.HeadOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Head"},
			{Key: "ListName", Value: o.ListVariable}, // storageName: ListName
		}
	case *microflows.TailOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Tail"},
			{Key: "ListName", Value: o.ListVariable}, // storageName: ListName
		}
	case *microflows.FindOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$FindByExpression"}, // storageName differs
			{Key: "Expression", Value: o.Expression},
			{Key: "ListName", Value: o.ListVariable}, // storageName: ListName
		}
	case *microflows.FilterOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$FilterByExpression"}, // storageName differs
			{Key: "Expression", Value: o.Expression},
			{Key: "ListName", Value: o.ListVariable}, // storageName: ListName
		}
	case *microflows.FindByAttributeOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Find"},
			{Key: "Association", Value: o.Association},
			{Key: "Attribute", Value: o.Attribute},
			{Key: "Expression", Value: o.Expression},
			{Key: "ListName", Value: o.ListVariable},
		}
	case *microflows.FilterByAttributeOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Filter"},
			{Key: "Association", Value: o.Association},
			{Key: "Attribute", Value: o.Attribute},
			{Key: "Expression", Value: o.Expression},
			{Key: "ListName", Value: o.ListVariable},
		}
	case *microflows.SortOperation:
		// Build sorting items
		sortings := bson.A{int32(3)} // Array with items marker
		for _, item := range o.Sorting {
			sortItem := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(item.ID))},
				{Key: "$Type", Value: "Microflows$RetrieveSorting"}, // storageName for SortItem
				{Key: "SortOrder", Value: string(item.Direction)},
			}
			// AttributeRef is a nested DomainModels$AttributeRef object
			if item.AttributeQualifiedName != "" {
				attrRef := bson.D{
					{Key: "$ID", Value: idToBsonBinary(generateUUID())},
					{Key: "$Type", Value: "DomainModels$AttributeRef"},
					{Key: "Attribute", Value: item.AttributeQualifiedName}, // BY_NAME_REFERENCE stored as string
				}
				if len(item.EntityRefSteps) > 0 {
					attrRef = append(attrRef, bson.E{Key: "EntityRef", Value: serializeIndirectEntityRef(item.EntityRefSteps)})
				}
				sortItem = append(sortItem, bson.E{Key: "AttributeRef", Value: attrRef})
			}
			sortings = append(sortings, sortItem)
		}
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Sort"},
			{Key: "ListName", Value: o.ListVariable}, // storageName: ListName
			{Key: "Sortings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Microflows$SortingsList"}, // storageName for SortItemList
				{Key: "Sortings", Value: sortings},
			}},
		}
	case *microflows.UnionOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Union"},
			{Key: "ListName", Value: o.ListVariable1},               // storageName: ListName
			{Key: "SecondListOrObjectName", Value: o.ListVariable2}, // storageName differs
		}
	case *microflows.IntersectOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Intersect"},
			{Key: "ListName", Value: o.ListVariable1},               // storageName: ListName
			{Key: "SecondListOrObjectName", Value: o.ListVariable2}, // storageName differs
		}
	case *microflows.SubtractOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Subtract"},
			{Key: "ListName", Value: o.ListVariable1},               // storageName: ListName
			{Key: "SecondListOrObjectName", Value: o.ListVariable2}, // storageName differs
		}
	case *microflows.ContainsOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Contains"},
			{Key: "ListName", Value: o.ListVariable},                 // storageName: ListName
			{Key: "SecondListOrObjectName", Value: o.ObjectVariable}, // storageName differs
		}
	case *microflows.EqualsOperation:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(o.ID))},
			{Key: "$Type", Value: "Microflows$Equals"},              // storageName for ListEquals
			{Key: "ListName", Value: o.ListVariable1},               // storageName: ListName
			{Key: "SecondListOrObjectName", Value: o.ListVariable2}, // storageName differs
		}
	default:
		return nil
	}
}

// serializeDatabaseRetrieveSource serializes a DatabaseRetrieveSource to BSON.
func serializeDatabaseRetrieveSource(source *microflows.DatabaseRetrieveSource) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(source.ID))},
		{Key: "$Type", Value: "Microflows$DatabaseRetrieveSource"},
	}

	// Entity is BY_NAME_REFERENCE - use qualified name string
	if source.EntityQualifiedName != "" {
		doc = append(doc, bson.E{Key: "Entity", Value: source.EntityQualifiedName})
	}

	// NewSortings (storageName) wraps a Microflows$SortingsList with Sortings array
	sortItems := bson.A{int32(2)} // storageListType: 2 array marker
	for _, sortItem := range source.Sorting {
		sortItems = append(sortItems, serializeSortItem(sortItem))
	}
	sortingsList := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Microflows$SortingsList"},
		{Key: "Sortings", Value: sortItems},
	}
	doc = append(doc, bson.E{Key: "NewSortings", Value: sortingsList})

	// Range for limiting results - always include for Studio Pro compatibility
	if source.Range != nil {
		doc = append(doc, bson.E{Key: "Range", Value: serializeRange(source.Range)})
	} else {
		// Create default Range (retrieve all objects)
		defaultRange := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Microflows$ConstantRange"},
			{Key: "SingleObject", Value: false},
		}
		doc = append(doc, bson.E{Key: "Range", Value: defaultRange})
	}

	// XPath constraint - note: BSON field name uses lowercase 'p' (XpathConstraint)
	if source.XPathConstraint != "" {
		doc = append(doc, bson.E{Key: "XpathConstraint", Value: source.XPathConstraint})
	}

	return doc
}

// serializeAssociationRetrieveSource serializes an AssociationRetrieveSource to BSON.
func serializeAssociationRetrieveSource(source *microflows.AssociationRetrieveSource) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(source.ID))},
		{Key: "$Type", Value: "Microflows$AssociationRetrieveSource"},
	}
	if source.StartVariable != "" {
		doc = append(doc, bson.E{Key: "StartVariableName", Value: source.StartVariable})
	}
	// AssociationId is BY_NAME_REFERENCE - use qualified name string
	if source.AssociationQualifiedName != "" {
		doc = append(doc, bson.E{Key: "AssociationId", Value: source.AssociationQualifiedName})
	}
	return doc
}

// serializeRange serializes a Range to BSON.
// ConstantRange only has SingleObject; CustomRange has LimitExpression/OffsetExpression.
func serializeRange(r *microflows.Range) bson.D {
	if r.RangeType == microflows.RangeTypeCustom {
		// CustomRange: expression-based LIMIT/OFFSET
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(r.ID))},
			{Key: "$Type", Value: "Microflows$CustomRange"},
		}
		if r.Limit != "" {
			doc = append(doc, bson.E{Key: "LimitExpression", Value: r.Limit})
		}
		if r.Offset != "" {
			doc = append(doc, bson.E{Key: "OffsetExpression", Value: r.Offset})
		}
		return doc
	}

	// ConstantRange: SingleObject=true (LIMIT 1) or retrieve all
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(r.ID))},
		{Key: "$Type", Value: "Microflows$ConstantRange"},
		{Key: "SingleObject", Value: r.RangeType == microflows.RangeTypeFirst},
	}
}

// serializeSortItem serializes a SortItem to BSON.
// Storage name is Microflows$RetrieveSorting (qualified: Microflows$SortItem).
func serializeSortItem(s *microflows.SortItem) bson.D {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(s.ID))},
		{Key: "$Type", Value: "Microflows$RetrieveSorting"},
	}

	// AttributeRef is a DomainModels$AttributeRef object containing Attribute as BY_NAME_REFERENCE
	if s.AttributeQualifiedName != "" {
		attrRef := bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DomainModels$AttributeRef"},
			{Key: "Attribute", Value: s.AttributeQualifiedName}, // BY_NAME_REFERENCE stored as string
		}
		if len(s.EntityRefSteps) > 0 {
			attrRef = append(attrRef, bson.E{Key: "EntityRef", Value: serializeIndirectEntityRef(s.EntityRefSteps)})
		}
		doc = append(doc, bson.E{Key: "AttributeRef", Value: attrRef})
	} else if s.AttributeID != "" {
		// Legacy fallback: binary ID reference
		doc = append(doc, bson.E{Key: "AttributeRef", Value: idToBsonBinary(string(s.AttributeID))})
	}

	doc = append(doc, bson.E{Key: "SortOrder", Value: string(s.Direction)})
	return doc
}

func serializeIndirectEntityRef(steps []microflows.EntityRefStep) bson.D {
	items := bson.A{int32(2)}
	for _, step := range steps {
		items = append(items, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DomainModels$EntityRefStep"},
			{Key: "Association", Value: step.Association},
			{Key: "DestinationEntity", Value: step.DestinationEntity},
		})
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
		{Key: "Steps", Value: items},
	}
}

// serializeCodeActionParameterValue serializes a CodeActionParameterValue to BSON.
func serializeCodeActionParameterValue(v microflows.CodeActionParameterValue) bson.D {
	switch value := v.(type) {
	case *microflows.StringTemplateParameterValue:
		doc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(value.ID))},
			{Key: "$Type", Value: "Microflows$StringTemplateParameterValue"},
		}
		if value.TypedTemplate != nil {
			tt := bson.D{
				{Key: "$ID", Value: idToBsonBinary(string(value.TypedTemplate.ID))},
				{Key: "$Type", Value: "Microflows$TypedTemplate"},
				{Key: "Arguments", Value: bson.A{int32(2)}}, // Empty array marker
				{Key: "Text", Value: value.TypedTemplate.Text},
			}
			doc = append(doc, bson.E{Key: "TypedTemplate", Value: tt})
		}
		return doc
	case *microflows.ExpressionBasedCodeActionParameterValue:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(value.ID))},
			{Key: "$Type", Value: "Microflows$ExpressionBasedCodeActionParameterValue"},
			{Key: "Expression", Value: value.Expression},
		}
	case *microflows.BasicCodeActionParameterValue:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(value.ID))},
			{Key: "$Type", Value: "Microflows$BasicCodeActionParameterValue"},
			{Key: "Argument", Value: value.Argument},
		}
	case *microflows.MicroflowParameterValue:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(value.ID))},
			{Key: "$Type", Value: "Microflows$MicroflowParameterValue"},
			{Key: "Microflow", Value: value.Microflow},
		}
	case *microflows.EntityTypeCodeActionParameterValue:
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(value.ID))},
			{Key: "$Type", Value: "Microflows$EntityTypeCodeActionParameterValue"},
			{Key: "Entity", Value: value.Entity},
		}
	}
	return nil
}

func serializeExecuteDatabaseQueryAction(a *microflows.ExecuteDatabaseQueryAction) bson.D {
	// ConnectionParameterMappings
	connMappings := bson.A{int32(2)}
	for _, cm := range a.ConnectionParameterMappings {
		cmDoc := bson.D{
			{Key: "$Type", Value: "DatabaseConnector$ConnectionParameterMapping"},
			{Key: "ParameterName", Value: cm.ParameterName},
			{Key: "Value", Value: cm.Value},
		}
		if cm.ID != "" {
			cmDoc = append(bson.D{{Key: "$ID", Value: idToBsonBinary(string(cm.ID))}}, cmDoc...)
		} else {
			cmDoc = append(bson.D{{Key: "$ID", Value: idToBsonBinary(generateUUID())}}, cmDoc...)
		}
		connMappings = append(connMappings, cmDoc)
	}

	// ParameterMappings
	paramMappings := bson.A{int32(2)}
	for _, pm := range a.ParameterMappings {
		pmDoc := bson.D{
			{Key: "$Type", Value: "DatabaseConnector$QueryParameterMapping"},
			{Key: "ParameterName", Value: pm.ParameterName},
			{Key: "Value", Value: pm.Value},
		}
		if pm.ID != "" {
			pmDoc = append(bson.D{{Key: "$ID", Value: idToBsonBinary(string(pm.ID))}}, pmDoc...)
		} else {
			pmDoc = append(bson.D{{Key: "$ID", Value: idToBsonBinary(generateUUID())}}, pmDoc...)
		}
		paramMappings = append(paramMappings, pmDoc)
	}

	// Fields in alphabetical order (matches Studio Pro BSON layout)
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "DatabaseConnector$ExecuteDatabaseQueryAction"},
		{Key: "ConnectionParameterMappings", Value: connMappings},
		{Key: "DynamicQuery", Value: a.DynamicQuery},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "OutputVariableName", Value: a.OutputVariableName},
		{Key: "ParameterMappings", Value: paramMappings},
		{Key: "Query", Value: a.Query},
	}

	return doc
}

func serializeImportXmlAction(a *microflows.ImportXmlAction) bson.D {
	forceSingleOccurrence := false
	if a.ResultHandling.ForceSingleOccurrence != nil {
		forceSingleOccurrence = *a.ResultHandling.ForceSingleOccurrence
	}

	// Build ImportMappingCall
	importCall := bson.D{
		{Key: "$ID", Value: idToBsonBinary(GenerateID())},
		{Key: "$Type", Value: "Microflows$ImportMappingCall"},
		{Key: "Commit", Value: "YesWithoutEvents"},
		{Key: "ContentType", Value: "Json"},
		{Key: "ForceSingleOccurrence", Value: forceSingleOccurrence},
		{Key: "ObjectHandlingBackup", Value: "Create"},
		{Key: "ParameterVariableName", Value: ""},
		{Key: "Range", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "Microflows$ConstantRange"},
			{Key: "SingleObject", Value: a.ResultHandling.SingleObject},
		}},
		{Key: "ReturnValueMapping", Value: string(a.ResultHandling.MappingID)},
	}

	// Build VariableType
	var varType bson.D
	if a.ResultHandling.SingleObject {
		varType = bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "DataTypes$ObjectType"},
			{Key: "Entity", Value: string(a.ResultHandling.ResultEntityID)},
		}
	} else {
		varType = bson.D{
			{Key: "$ID", Value: idToBsonBinary(GenerateID())},
			{Key: "$Type", Value: "DataTypes$ListType"},
			{Key: "Entity", Value: string(a.ResultHandling.ResultEntityID)},
		}
	}

	bind := a.ResultHandling.ResultVariable != ""

	resultHandling := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ResultHandling.ID))},
		{Key: "$Type", Value: "Microflows$ResultHandling"},
		{Key: "Bind", Value: bind},
		{Key: "ImportMappingCall", Value: importCall},
		{Key: "ResultVariableName", Value: a.ResultHandling.ResultVariable},
		{Key: "VariableType", Value: varType},
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$ImportXmlAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "IsValidationRequired", Value: a.IsValidationRequired},
		{Key: "ResultHandling", Value: resultHandling},
		{Key: "XmlDocumentVariableName", Value: a.XmlDocumentVariable},
	}
}

func serializeTransformJsonAction(a *microflows.TransformJsonAction) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$TransformJsonAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "InputVariableName", Value: a.InputVariableName},
		{Key: "OutputVariableName", Value: a.OutputVariableName},
		{Key: "Transformation", Value: a.Transformation},
	}
}

func serializeExportXmlAction(a *microflows.ExportXmlAction) bson.D {
	// OutputMethod: ExportXmlAction$StringExport
	outputMethod := bson.D{
		{Key: "$ID", Value: idToBsonBinary(GenerateID())},
		{Key: "$Type", Value: "ExportXmlAction$StringExport"},
		{Key: "OutputVariableName", Value: a.OutputVariable},
	}

	// ResultHandling: MappingRequestHandling
	mappingID := ""
	paramVar := ""
	if a.RequestHandling != nil {
		mappingID = string(a.RequestHandling.MappingID)
		paramVar = a.RequestHandling.ParameterVariable
	}

	resultHandling := bson.D{
		{Key: "$ID", Value: idToBsonBinary(GenerateID())},
		{Key: "$Type", Value: "Microflows$MappingRequestHandling"},
		{Key: "ContentType", Value: "Json"},
		{Key: "MappingId", Value: mappingID},
		{Key: "MappingVariableName", Value: paramVar},
	}

	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(a.ID))},
		{Key: "$Type", Value: "Microflows$ExportXmlAction"},
		{Key: "ErrorHandlingType", Value: stringOrDefault(string(a.ErrorHandlingType), "Rollback")},
		{Key: "IsValidationRequired", Value: a.IsValidationRequired},
		{Key: "OutputMethod", Value: outputMethod},
		{Key: "ResultHandling", Value: resultHandling},
	}
}

// serializeExternalActionReturnType maps a Mendix kind name (resolved from a
// consumed OData service's cached $metadata) to a DataTypes$* BSON sub-doc
// suitable for ODataPublish$CallExternalAction.VariableDataType. Mendix's
// CE7269 fires when this field's $Type doesn't match what the cached schema
// declares for the action's return.
func serializeExternalActionReturnType(kind string) bson.D {
	typeID := idToBsonBinary(generateUUID())
	bsonType := "DataTypes$VoidType"
	switch kind {
	case "Boolean":
		bsonType = "DataTypes$BooleanType"
	case "String":
		bsonType = "DataTypes$StringType"
	case "Integer", "Long":
		bsonType = "DataTypes$IntegerType"
	case "Decimal", "Float":
		bsonType = "DataTypes$DecimalType"
	case "DateTime":
		bsonType = "DataTypes$DateTimeType"
	case "Binary":
		bsonType = "DataTypes$BinaryType"
	case "Void", "":
		bsonType = "DataTypes$VoidType"
	}
	return bson.D{
		{Key: "$ID", Value: typeID},
		{Key: "$Type", Value: bsonType},
	}
}
