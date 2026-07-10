// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// System module constants — deterministic IDs for the virtual System module.
const (
	SystemModuleID      = "00000000-0000-0000-0000-000000000001"
	SystemDomainModelID = "00000000-0000-0000-0000-000000000002"
)

// systemAttrDef defines an attribute in a System entity.
type systemAttrDef struct {
	Name   string
	Type   string // "String", "Integer", "Decimal", "Boolean", "DateTime", "Enumeration", "Long", "Binary", "HashedString", "AutoNumber"
	Length int    // for String type
	EnumQN string // for Enumeration type, qualified name
}

// systemAssocDef defines an association between System entities.
type systemAssocDef struct {
	Name   string
	Parent string // parent entity name (without module prefix)
	Child  string // child entity name (without module prefix)
	Type   string // "Reference" or "ReferenceSet"
	Owner  string // "Default" or "Both"
}

// systemEntityDef defines a System entity with name, persistability, and attributes.
type systemEntityDef struct {
	Name           string
	Persistable    bool
	Generalization string // e.g. "System.FileDocument", "System.Error"
	Attributes     []systemAttrDef
}

// systemEntities lists all entities in the System module.
// Extracted from Mendix Studio Pro 11.6.4 via DummySystem module.
var systemEntities = []systemEntityDef{
	{Name: "UserRole", Persistable: true, Attributes: []systemAttrDef{
		{Name: "ModelGUID", Type: "String"},
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
	}},
	{Name: "User", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Password", Type: "HashedString"},
		{Name: "LastLogin", Type: "DateTime"},
		{Name: "Blocked", Type: "Boolean"},
		{Name: "BlockedSince", Type: "DateTime"},
		{Name: "Active", Type: "Boolean"},
		{Name: "FailedLogins", Type: "Integer"},
		{Name: "WebServiceUser", Type: "Boolean"},
		{Name: "IsAnonymous", Type: "Boolean"},
	}},
	{Name: "FileDocument", Persistable: true, Attributes: []systemAttrDef{
		{Name: "FileID", Type: "AutoNumber"},
		{Name: "Name", Type: "String"},
		{Name: "DeleteAfterDownload", Type: "Boolean"},
		{Name: "Contents", Type: "Binary"},
		{Name: "HasContents", Type: "Boolean"},
		{Name: "Size", Type: "Long"},
	}},
	{Name: "Image", Persistable: true, Generalization: "System.FileDocument", Attributes: []systemAttrDef{
		{Name: "PublicThumbnailPath", Type: "String"},
		{Name: "EnableCaching", Type: "Boolean"},
	}},
	{Name: "XASInstance", Persistable: true, Attributes: []systemAttrDef{
		{Name: "XASId", Type: "String"},
		{Name: "LastUpdate", Type: "DateTime"},
		{Name: "AllowedNumberOfConcurrentUsers", Type: "Integer"},
		{Name: "PartnerName", Type: "String"},
		{Name: "CustomerName", Type: "String"},
	}},
	{Name: "Session", Persistable: true, Attributes: []systemAttrDef{
		{Name: "SessionId", Type: "String"},
		{Name: "CSRFToken", Type: "String"},
		{Name: "LastActive", Type: "DateTime"},
	}},
	{Name: "ScheduledEventInformation", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Status", Type: "Enumeration", EnumQN: "System.EventStatus"},
	}},
	{Name: "Language", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Code", Type: "String"},
		{Name: "Description", Type: "String"},
	}},
	{Name: "TimeZone", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Code", Type: "String"},
		{Name: "Description", Type: "String"},
		{Name: "RawOffset", Type: "Integer"},
	}},
	{Name: "Error", Persistable: false, Attributes: []systemAttrDef{
		{Name: "ErrorType", Type: "String"},
		{Name: "Message", Type: "String"},
		{Name: "Stacktrace", Type: "String"},
	}},
	{Name: "SoapFault", Persistable: true, Generalization: "System.Error", Attributes: []systemAttrDef{
		{Name: "Code", Type: "String"},
		{Name: "Reason", Type: "String"},
		{Name: "Node", Type: "String"},
		{Name: "Role", Type: "String"},
		{Name: "Detail", Type: "String"},
	}},
	{Name: "TokenInformation", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Token", Type: "HashedString"},
		{Name: "ExpiryDate", Type: "DateTime"},
		{Name: "UserAgent", Type: "String"},
	}},
	{Name: "HttpMessage", Persistable: false, Attributes: []systemAttrDef{
		{Name: "HttpVersion", Type: "String"},
		{Name: "Content", Type: "String"},
	}},
	{Name: "HttpHeader", Persistable: false, Attributes: []systemAttrDef{
		{Name: "Key", Type: "String"},
		{Name: "Value", Type: "String"},
	}},
	{Name: "UserReportInfo", Persistable: true, Attributes: []systemAttrDef{
		{Name: "UserType", Type: "Enumeration", EnumQN: "System.UserType"},
		{Name: "Hash", Type: "String"},
	}},
	{Name: "HttpRequest", Persistable: true, Generalization: "System.HttpMessage", Attributes: []systemAttrDef{
		{Name: "Uri", Type: "String"},
	}},
	{Name: "HttpResponse", Persistable: true, Generalization: "System.HttpMessage", Attributes: []systemAttrDef{
		{Name: "StatusCode", Type: "Integer"},
		{Name: "ReasonPhrase", Type: "String"},
	}},
	{Name: "Paging", Persistable: false, Attributes: []systemAttrDef{
		{Name: "PageNumber", Type: "Long"},
		{Name: "IsSortable", Type: "Boolean"},
		{Name: "SortAttribute", Type: "String"},
		{Name: "SortAscending", Type: "Boolean"},
		{Name: "HasMoreData", Type: "Boolean"},
	}},
	{Name: "SynchronizationError", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Reason", Type: "String"},
		{Name: "ObjectId", Type: "String"},
		{Name: "ObjectType", Type: "String"},
		{Name: "ObjectContent", Type: "String"},
	}},
	{Name: "SynchronizationErrorFile", Persistable: true, Generalization: "System.FileDocument"},
	{Name: "ProcessedQueueTask", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Sequence", Type: "Long"},
		{Name: "Status", Type: "Enumeration", EnumQN: "System.QueueTaskStatus"},
		{Name: "QueueId", Type: "String"},
		{Name: "QueueName", Type: "String"},
		{Name: "ContextType", Type: "Enumeration", EnumQN: "System.ContextType"},
		{Name: "ContextData", Type: "String"},
		{Name: "MicroflowName", Type: "String"},
		{Name: "UserActionName", Type: "String"},
		{Name: "Arguments", Type: "String"},
		{Name: "XASId", Type: "String"},
		{Name: "ThreadId", Type: "Long"},
		{Name: "Created", Type: "DateTime"},
		{Name: "StartAt", Type: "DateTime"},
		{Name: "Started", Type: "DateTime"},
		{Name: "Finished", Type: "DateTime"},
		{Name: "Duration", Type: "Long"},
		{Name: "Retried", Type: "Long"},
		{Name: "ErrorMessage", Type: "String"},
		{Name: "ScheduledEventName", Type: "String"},
	}},
	{Name: "QueuedTask", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Sequence", Type: "AutoNumber"},
		{Name: "Status", Type: "Enumeration", EnumQN: "System.QueueTaskStatus"},
		{Name: "QueueId", Type: "String"},
		{Name: "QueueName", Type: "String"},
		{Name: "ContextType", Type: "Enumeration", EnumQN: "System.ContextType"},
		{Name: "ContextData", Type: "String"},
		{Name: "MicroflowName", Type: "String"},
		{Name: "UserActionName", Type: "String"},
		{Name: "Arguments", Type: "String"},
		{Name: "XASId", Type: "String"},
		{Name: "ThreadId", Type: "Long"},
		{Name: "Created", Type: "DateTime"},
		{Name: "StartAt", Type: "DateTime"},
		{Name: "Started", Type: "DateTime"},
		{Name: "Retried", Type: "Long"},
		{Name: "Retry", Type: "String"},
		{Name: "ScheduledEventName", Type: "String"},
	}},
	{Name: "WorkflowDefinition", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Title", Type: "String"},
		{Name: "IsObsolete", Type: "Boolean"},
		{Name: "IsLocked", Type: "Boolean"},
	}},
	{Name: "WorkflowUserTaskDefinition", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "IsObsolete", Type: "Boolean"},
	}},
	{Name: "Workflow", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "DueDate", Type: "DateTime"},
		{Name: "CanBeRestarted", Type: "Boolean"},
		{Name: "CanBeContinued", Type: "Boolean"},
		{Name: "CanApplyJumpTo", Type: "Boolean"},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowState"},
		{Name: "Reason", Type: "String"},
	}},
	{Name: "WorkflowUserTask", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "DueDate", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Outcome", Type: "String"},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskState"},
		{Name: "CompletionType", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskCompletionType"},
	}},
	{Name: "TaskQueueToken", Persistable: true, Attributes: []systemAttrDef{
		{Name: "QueueName", Type: "String"},
		{Name: "XASId", Type: "String"},
		{Name: "ValidUntil", Type: "DateTime"},
	}},
	{Name: "ODataResponse", Persistable: false, Attributes: []systemAttrDef{
		{Name: "Count", Type: "Long"},
	}},
	{Name: "WorkflowJumpToDetails", Persistable: false, Attributes: []systemAttrDef{
		{Name: "Error", Type: "String"},
	}},
	{Name: "WorkflowCurrentActivity", Persistable: false, Attributes: []systemAttrDef{
		{Name: "Action", Type: "Enumeration", EnumQN: "System.WorkflowCurrentActivityAction"},
	}},
	{Name: "WorkflowActivityDetails", Persistable: false, Attributes: []systemAttrDef{
		{Name: "ActivityId", Type: "String"},
		{Name: "ActivityCaption", Type: "String"},
		{Name: "ActivityType", Type: "Enumeration", EnumQN: "System.WorkflowActivityType"},
		{Name: "ExistsInCurrentVersion", Type: "Boolean"},
	}},
	{Name: "WorkflowUserTaskOutcome", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Outcome", Type: "String"},
		{Name: "Time", Type: "DateTime"},
	}},
	{Name: "WorkflowRecord", Persistable: false, Attributes: []systemAttrDef{
		{Name: "WorkflowKey", Type: "String"},
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowState"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "DueDate", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Reason", Type: "String"},
	}},
	{Name: "WorkflowActivityRecord", Persistable: false, Attributes: []systemAttrDef{
		{Name: "ModelGUID", Type: "String"},
		{Name: "ActivityKey", Type: "String"},
		{Name: "PreviousActivityKey", Type: "String"},
		{Name: "ActivityType", Type: "Enumeration", EnumQN: "System.WorkflowActivityType"},
		{Name: "Caption", Type: "String"},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowActivityExecutionState"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Outcome", Type: "String"},
		{Name: "MicroflowName", Type: "String"},
		{Name: "TaskName", Type: "String"},
		{Name: "TaskDescription", Type: "String"},
		{Name: "TaskDueDate", Type: "DateTime"},
		{Name: "TaskCompletionType", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskCompletionType"},
		{Name: "TaskRequiredUsers", Type: "Integer"},
		{Name: "TaskKey", Type: "String"},
		{Name: "Reason", Type: "String"},
	}},
	{Name: "WorkflowEvent", Persistable: false, Attributes: []systemAttrDef{
		{Name: "EventTime", Type: "DateTime"},
		{Name: "EventType", Type: "Enumeration", EnumQN: "System.WorkflowEventType"},
	}},
	{Name: "ConsumedODataConfiguration", Persistable: false, Attributes: []systemAttrDef{
		{Name: "ServiceUrl", Type: "String"},
		{Name: "ProxyConfiguration", Type: "Enumeration", EnumQN: "System.ProxyConfiguration"},
		{Name: "ProxyHost", Type: "String"},
		{Name: "ProxyPort", Type: "Integer"},
		{Name: "ProxyUsername", Type: "String"},
		{Name: "ProxyPassword", Type: "String"},
	}},
	{Name: "WorkflowEndedUserTask", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "DueDate", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Outcome", Type: "String"},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskState"},
		{Name: "CompletionType", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskCompletionType"},
		{Name: "UserTaskKey", Type: "String"},
	}},
	{Name: "WorkflowEndedUserTaskOutcome", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Outcome", Type: "String"},
		{Name: "Time", Type: "DateTime"},
	}},
	{Name: "WorkflowGroup", Persistable: true, Attributes: []systemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
	}},
}

// systemAssociations lists all associations in the System module.
// Extracted from Mendix Studio Pro 11.6.4 via DummySystem module.
var systemAssociations = []systemAssocDef{
	{Name: "grantableRoles", Parent: "UserRole", Child: "UserRole", Type: "ReferenceSet", Owner: "Default"},
	{Name: "UserRoles", Parent: "User", Child: "UserRole", Type: "ReferenceSet", Owner: "Default"},
	{Name: "Session_User", Parent: "Session", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "User_Language", Parent: "User", Child: "Language", Type: "Reference", Owner: "Default"},
	{Name: "User_TimeZone", Parent: "User", Child: "TimeZone", Type: "Reference", Owner: "Default"},
	{Name: "TokenInformation_User", Parent: "TokenInformation", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "HttpHeaders", Parent: "HttpHeader", Child: "HttpMessage", Type: "Reference", Owner: "Default"},
	{Name: "UserReportInfo_User", Parent: "UserReportInfo", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "ScheduledEventInformation_XASInstance", Parent: "ScheduledEventInformation", Child: "XASInstance", Type: "Reference", Owner: "Default"},
	{Name: "SynchronizationErrorFile_SynchronizationError", Parent: "SynchronizationErrorFile", Child: "SynchronizationError", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowUserTaskDefinition_WorkflowDefinition", Parent: "WorkflowUserTaskDefinition", Child: "WorkflowDefinition", Type: "Reference", Owner: "Default"},
	{Name: "Workflow_WorkflowDefinition", Parent: "Workflow", Child: "WorkflowDefinition", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowUserTask_TargetUsers", Parent: "WorkflowUserTask", Child: "User", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowUserTask_Assignees", Parent: "WorkflowUserTask", Child: "User", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowUserTask_Workflow", Parent: "WorkflowUserTask", Child: "Workflow", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowUserTask_WorkflowUserTaskDefinition", Parent: "WorkflowUserTask", Child: "WorkflowUserTaskDefinition", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowJumpToDetails_Workflow", Parent: "WorkflowJumpToDetails", Child: "Workflow", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowJumpToDetails_CurrentActivities", Parent: "WorkflowJumpToDetails", Child: "WorkflowCurrentActivity", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowCurrentActivity_ActivityDetails", Parent: "WorkflowCurrentActivity", Child: "WorkflowActivityDetails", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowCurrentActivity_ApplicableTargets", Parent: "WorkflowCurrentActivity", Child: "WorkflowActivityDetails", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowCurrentActivity_JumpToTarget", Parent: "WorkflowCurrentActivity", Child: "WorkflowActivityDetails", Type: "Reference", Owner: "Default"},
	{Name: "Workflow_ParentWorkflow", Parent: "Workflow", Child: "Workflow", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowUserTaskOutcome_WorkflowUserTask", Parent: "WorkflowUserTaskOutcome", Child: "WorkflowUserTask", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowUserTaskOutcome_User", Parent: "WorkflowUserTaskOutcome", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowRecord_Workflow", Parent: "WorkflowRecord", Child: "Workflow", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowRecord_Owner", Parent: "WorkflowRecord", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowRecord_WorkflowDefinition", Parent: "WorkflowRecord", Child: "WorkflowDefinition", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivityRecord_PreviousActivity", Parent: "WorkflowActivityRecord", Child: "WorkflowActivityRecord", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivityRecord_Actor", Parent: "WorkflowActivityRecord", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivityRecord_SubWorkflow", Parent: "WorkflowActivityRecord", Child: "WorkflowRecord", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivityRecord_UserTask", Parent: "WorkflowActivityRecord", Child: "WorkflowUserTask", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivityRecord_WorkflowUserTaskDefinition", Parent: "WorkflowActivityRecord", Child: "WorkflowUserTaskDefinition", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowEvent_Initiator", Parent: "WorkflowEvent", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivityRecord_TaskTargetedUsers", Parent: "WorkflowActivityRecord", Child: "User", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowActivityRecord_TaskAssignedUsers", Parent: "WorkflowActivityRecord", Child: "User", Type: "ReferenceSet", Owner: "Default"},
	{Name: "HttpHeader_ConsumedODataConfiguration", Parent: "HttpHeader", Child: "ConsumedODataConfiguration", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowEndedUserTask_Assignees", Parent: "WorkflowEndedUserTask", Child: "User", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowEndedUserTask_TargetUsers", Parent: "WorkflowEndedUserTask", Child: "User", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowEndedUserTask_WorkflowUserTaskDefinition", Parent: "WorkflowEndedUserTask", Child: "WorkflowUserTaskDefinition", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowEndedUserTask_Workflow", Parent: "WorkflowEndedUserTask", Child: "Workflow", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowEndedUserTaskOutcome_User", Parent: "WorkflowEndedUserTaskOutcome", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowEndedUserTaskOutcome_WorkflowEndedUserTask", Parent: "WorkflowEndedUserTaskOutcome", Child: "WorkflowEndedUserTask", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowGroup_User", Parent: "WorkflowGroup", Child: "User", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowUserTask_TargetGroups", Parent: "WorkflowUserTask", Child: "WorkflowGroup", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowEndedUserTask_TargetGroups", Parent: "WorkflowEndedUserTask", Child: "WorkflowGroup", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowActivityRecord_TaskTargetedGroups", Parent: "WorkflowActivityRecord", Child: "WorkflowGroup", Type: "ReferenceSet", Owner: "Default"},
}

// BuildSystemDomainModel returns a virtual DomainModel for the System module.
func BuildSystemDomainModel() *domainmodel.DomainModel {
	dm := &domainmodel.DomainModel{
		ContainerID: model.ID(SystemModuleID),
	}
	dm.ID = model.ID(SystemDomainModelID)
	dm.TypeName = "DomainModels$DomainModel"

	// Build entity name -> ID map for association resolution
	entityIDMap := make(map[string]model.ID, len(systemEntities))

	for _, def := range systemEntities {
		entityID := GenerateDeterministicID("System." + def.Name)
		entity := &domainmodel.Entity{
			ContainerID: model.ID(SystemDomainModelID),
			Name:        def.Name,
			Persistable: def.Persistable,
		}
		entity.ID = model.ID(entityID)
		entityIDMap[def.Name] = entity.ID

		if def.Generalization != "" {
			genID := GenerateDeterministicID("System." + def.Generalization + ".gen")
			gen := domainmodel.GeneralizationBase{}
			gen.ID = model.ID(genID)
			gen.GeneralizationID = model.ID(GenerateDeterministicID(def.Generalization))
			entity.Generalization = gen
			entity.GeneralizationRef = def.Generalization
		}

		// Add attributes
		for _, attrDef := range def.Attributes {
			attrID := GenerateDeterministicID("System." + def.Name + "." + attrDef.Name)
			attr := &domainmodel.Attribute{
				ContainerID: entity.ID,
				Name:        attrDef.Name,
			}
			attr.ID = model.ID(attrID)

			switch attrDef.Type {
			case "String":
				attr.Type = &domainmodel.StringAttributeType{Length: attrDef.Length}
			case "Integer":
				attr.Type = &domainmodel.IntegerAttributeType{}
			case "Long":
				attr.Type = &domainmodel.LongAttributeType{}
			case "Decimal":
				attr.Type = &domainmodel.DecimalAttributeType{}
			case "Boolean":
				attr.Type = &domainmodel.BooleanAttributeType{}
			case "DateTime":
				attr.Type = &domainmodel.DateTimeAttributeType{}
			case "Enumeration":
				attr.Type = &domainmodel.EnumerationAttributeType{
					EnumerationRef: attrDef.EnumQN,
				}
			case "AutoNumber":
				attr.Type = &domainmodel.AutoNumberAttributeType{}
			case "Binary":
				attr.Type = &domainmodel.BinaryAttributeType{}
			case "HashedString":
				attr.Type = &domainmodel.HashedStringAttributeType{}
			}

			entity.Attributes = append(entity.Attributes, attr)
		}

		dm.Entities = append(dm.Entities, entity)
	}

	// Add associations
	for _, def := range systemAssociations {
		assocID := GenerateDeterministicID("System." + def.Name)
		assoc := &domainmodel.Association{
			ContainerID: model.ID(SystemDomainModelID),
			Name:        def.Name,
			ParentID:    entityIDMap[def.Parent],
			ChildID:     entityIDMap[def.Child],
			Type:        domainmodel.AssociationType(def.Type),
			Owner:       domainmodel.AssociationOwner(def.Owner),
		}
		assoc.ID = model.ID(assocID)
		dm.Associations = append(dm.Associations, assoc)
	}

	return dm
}

// BuildSystemModule returns a virtual Module for the System module.
func BuildSystemModule() *model.Module {
	m := &model.Module{
		Name: "System",
	}
	m.ID = model.ID(SystemModuleID)
	return m
}
