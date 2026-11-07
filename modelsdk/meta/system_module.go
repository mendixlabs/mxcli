// SPDX-License-Identifier: Apache-2.0

package meta

import (
	"fmt"
	"strings"
)

// System module constants — deterministic IDs for the virtual System module.
const (
	SystemModuleID      = "00000000-0000-0000-0000-000000000001"
	SystemDomainModelID = "00000000-0000-0000-0000-000000000002"
)

// SystemAttrDef defines an attribute in a System entity.
type SystemAttrDef struct {
	Name string
	Type string // "String", "Integer", "Decimal", "Boolean", "DateTime", "Enumeration", "Long", "Binary", "HashedString", "AutoNumber"

	// Length is the maximum string length for Type "String", carrying Mendix's
	// own meaning: 0 is UNLIMITED, not unknown. Every String attribute here is
	// measured from a deployed model (testdata/system_string_lengths.txt), and
	// TestSystemStringLengths fails on a String attribute that has no
	// measurement — so a 0 is a finding, never a gap.
	//
	// It went three versions unpopulated, which made every System string read
	// as unlimited and put mxcli's length checks into direct disagreement with
	// mxbuild: a view entity over System.User.Name had its correct String(100)
	// refused and its wrong String waved through, to fail later with CE6770
	// (ako/mxcli#584).
	Length int

	EnumQN string // for Enumeration type, qualified name

	// WriteForbidden marks a system attribute an access rule may not grant
	// write on: mxbuild reports CE6592 "Attribute 'X' cannot have write
	// rights, because it is a system attribute". Measured on 11.14.0 with
	// `grant read *, write *` on specializations of System.FileDocument and
	// System.Image: HasContents and PublicThumbnailPath are refused; Name,
	// DeleteAfterDownload, Contents, Size and EnableCaching are not (FileID is
	// an autonumber, refused for that reason). System.User's attributes are not
	// in an access rule at all — Mendix manages them (see EntityMembersFor).
	WriteForbidden bool
}

// SystemAttributeWriteForbidden reports whether the System attribute named by
// a member-access reference ("System.FileDocument.HasContents") is one Mendix
// refuses write rights on (CE6592).
func SystemAttributeWriteForbidden(ref string) bool {
	rest, ok := strings.CutPrefix(ref, "System.")
	if !ok {
		return false
	}
	entity, attr, ok := strings.Cut(rest, ".")
	if !ok {
		return false
	}
	def, ok := systemEntityByName(entity)
	if !ok {
		return false
	}
	for _, a := range def.Attributes {
		if a.Name == attr {
			return a.WriteForbidden
		}
	}
	return false
}

// SystemAssocDef defines an association between System entities.
type SystemAssocDef struct {
	Name   string
	Parent string // parent entity name (without module prefix)
	Child  string // child entity name (without module prefix)
	Type   string // "Reference" or "ReferenceSet"
	Owner  string // "Default" or "Both"

	// Introduced and Removed bound the Mendix versions whose System module has
	// this association, as "major.minor" (Removed is the first version WITHOUT
	// it); empty means unbounded. The list otherwise describes the current
	// platform, so only MEASURED differences are recorded here — read from the
	// modeler assembly of each version (ako/mxcli#1810).
	Introduced string
	Removed    string
}

// InVersion reports whether the association exists in a project whose Mendix
// version satisfies atLeast(major, minor). A nil atLeast (version unknown)
// answers for the current platform: everything not Removed.
func (a SystemAssocDef) InVersion(atLeast func(major, minor int) bool) bool {
	if atLeast == nil {
		return a.Removed == ""
	}
	if a.Introduced != "" && !atLeast(parseMajorMinor(a.Introduced)) {
		return false
	}
	if a.Removed != "" && atLeast(parseMajorMinor(a.Removed)) {
		return false
	}
	return true
}

func parseMajorMinor(v string) (int, int) {
	var major, minor int
	fmt.Sscanf(v, "%d.%d", &major, &minor)
	return major, minor
}

// SystemEntityDef defines a System entity with name, persistability, and attributes.
type SystemEntityDef struct {
	Name           string
	Persistable    bool
	Generalization string // e.g. "System.FileDocument", "System.Error"
	Attributes     []SystemAttrDef

	// RuntimeOnly marks an entity that exists in the Mendix RUNTIME's metamodel
	// but not in the System module Studio Pro shows — a table the runtime keeps
	// for itself (background jobs, auto-commit entries, offline sync bookkeeping,
	// workflow execution rows).
	//
	// The distinction is load-bearing because a model references System members
	// BY NAME. Writing one of these names produces a document mxbuild rejects
	// with CE1613 "no longer exists" — which is upstream #972: `Thumbnail_Image`
	// is owned by both ends, so System.Image counted as an owner through the
	// child side and every specialization of it inherited an access-rule member
	// for an association the modeler does not have.
	//
	// Use ModelerSystemEntities / ModelerSystemAssociations for anything that
	// resolves against the model. The full lists stay complete for a storage
	// backend that speaks to the runtime metamodel instead.
	RuntimeOnly bool

	// The four system members an entity may store — System.owner,
	// System.changedBy, createdDate, changedDate. On a user entity these are the
	// AutoOwner / AutoChangedBy / AutoCreatedDate / AutoChangedDate pseudo-types;
	// a System entity has them fixed by the platform.
	//
	// MEASURED, from the NoGeneralization of each System entity in the
	// `deployment/model/model.mdp` mxbuild writes (11.14.0); see
	// TestSystemMemberFlagsMatchTheDeployedModel. They are set only on a ROOT
	// entity: Mendix keeps them on NoGeneralization, so a specialization
	// (System.Image, every app entity extending System.FileDocument) inherits
	// its root's — read them through SystemEntityStoresMember, which walks the
	// chain. Reading a specialization's own flags is how `[System.owner = …]` on
	// a FileDocument specialization was refused at check time while mxbuild
	// built it clean.
	HasOwner       bool
	HasChangedBy   bool
	HasCreatedDate bool
	HasChangedDate bool
}

// System member names as XPath spells them. The spelling is significant: XPath
// is case-sensitive, and `System.Owner` is CE1613 where `System.owner` is clean
// (measured on 11.14.0).
const (
	SystemMemberOwner       = "owner"
	SystemMemberChangedBy   = "changedBy"
	SystemMemberCreatedDate = "createdDate"
	SystemMemberChangedDate = "changedDate"
)

// Stores reports whether this entity's OWN flags record the given system
// member (one of the SystemMember* names). It does not walk the generalization
// chain; see SystemEntityStoresMember.
func (e SystemEntityDef) Stores(member string) bool {
	switch member {
	case SystemMemberOwner:
		return e.HasOwner
	case SystemMemberChangedBy:
		return e.HasChangedBy
	case SystemMemberCreatedDate:
		return e.HasCreatedDate
	case SystemMemberChangedDate:
		return e.HasChangedDate
	}
	return false
}

// SystemEntityStoresMember reports whether the System entity named by name
// ("FileDocument" or "System.FileDocument") stores the given system member,
// following its generalization chain to the root that carries the flags.
// known is false when the entity is not a System entity mxcli knows.
func SystemEntityStoresMember(name, member string) (stores, known bool) {
	name = strings.TrimPrefix(name, "System.")
	seen := map[string]bool{}
	for name != "" && !seen[name] {
		seen[name] = true
		def, ok := systemEntityByName(name)
		if !ok {
			return false, false
		}
		if def.Generalization == "" {
			return def.Stores(member), true
		}
		name = strings.TrimPrefix(def.Generalization, "System.")
	}
	return false, false
}

// SystemEntityPersistable reports whether the System entity named by name
// ("Image" or "System.Image") is persistable, which its chain's root decides.
// known is false when the entity is not a System entity mxcli knows.
func SystemEntityPersistable(name string) (persistable, known bool) {
	name = strings.TrimPrefix(name, "System.")
	seen := map[string]bool{}
	for name != "" && !seen[name] {
		seen[name] = true
		def, ok := systemEntityByName(name)
		if !ok {
			return false, false
		}
		if def.Generalization == "" {
			return def.Persistable, true
		}
		name = strings.TrimPrefix(def.Generalization, "System.")
	}
	return false, false
}

func systemEntityByName(name string) (SystemEntityDef, bool) {
	for _, e := range SystemEntities {
		if e.Name == name {
			return e, true
		}
	}
	return SystemEntityDef{}, false
}

// ModelerSystemEntities returns the System entities Studio Pro exposes — the
// full list minus the runtime-only rows. Every path that resolves a name against
// the model wants this one; see SystemEntityDef.RuntimeOnly.
func ModelerSystemEntities() []SystemEntityDef {
	out := make([]SystemEntityDef, 0, len(SystemEntities))
	for _, e := range SystemEntities {
		if !e.RuntimeOnly {
			out = append(out, e)
		}
	}
	return out
}

// ModelerSystemAssociations returns the System associations Studio Pro exposes.
//
// An association is runtime-only exactly when one of its ends is, so it is
// DERIVED rather than flagged by hand: a flag on both would be two lists to keep
// in step, and the one that fell behind would put a dangling by-name reference
// into a model — which is the failure this whole distinction exists to prevent.
func ModelerSystemAssociations() []SystemAssocDef {
	return ModelerSystemAssociationsIn(nil)
}

// ModelerSystemAssociationsIn is ModelerSystemAssociations for one project
// version, given as its IsAtLeast; nil means the current platform.
func ModelerSystemAssociationsIn(atLeast func(major, minor int) bool) []SystemAssocDef {
	runtimeOnly := make(map[string]bool, len(SystemEntities))
	for _, e := range SystemEntities {
		if e.RuntimeOnly {
			runtimeOnly[e.Name] = true
		}
	}
	out := make([]SystemAssocDef, 0, len(SystemAssociations))
	for _, a := range SystemAssociations {
		if runtimeOnly[a.Parent] || runtimeOnly[a.Child] || !a.InVersion(atLeast) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// SystemEntities lists all entities in the System module.
//
// Entities, attributes and types were extracted from Mendix Studio Pro 11.6.4
// via a DummySystem module. The String LENGTHS come from a different and more
// direct source — the System module's domain model inside the
// `deployment/model/model.mdp` mxbuild writes — because that is the model the
// runtime builds the System tables from, and so the one Mendix judges a length
// against. See testdata/system_string_lengths.txt for the measurement and how
// to redo it.
var SystemEntities = []SystemEntityDef{
	{Name: "UserRole", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "ModelGUID", Type: "String", Length: 36},
		{Name: "Name", Type: "String", Length: 100},
		{Name: "Description", Type: "String", Length: 1000},
	}},
	{Name: "User", Persistable: true, HasOwner: true, HasChangedBy: true, HasCreatedDate: true, HasChangedDate: true, Attributes: []SystemAttrDef{
		{Name: "Name", Type: "String", Length: 100},
		{Name: "Password", Type: "HashedString"},
		{Name: "LastLogin", Type: "DateTime"},
		{Name: "Blocked", Type: "Boolean"},
		{Name: "BlockedSince", Type: "DateTime"},
		{Name: "Active", Type: "Boolean"},
		{Name: "FailedLogins", Type: "Integer"},
		{Name: "WebServiceUser", Type: "Boolean"},
		{Name: "IsAnonymous", Type: "Boolean"},
	}},
	{Name: "FileDocument", Persistable: true, HasOwner: true, HasChangedBy: true, HasCreatedDate: true, HasChangedDate: true, Attributes: []SystemAttrDef{
		{Name: "FileID", Type: "AutoNumber"},
		{Name: "Name", Type: "String", Length: 400},
		{Name: "DeleteAfterDownload", Type: "Boolean"},
		{Name: "Contents", Type: "Binary"},
		{Name: "HasContents", Type: "Boolean", WriteForbidden: true},
		{Name: "Size", Type: "Long"},
	}},
	{Name: "Image", Persistable: true, Generalization: "System.FileDocument", Attributes: []SystemAttrDef{
		{Name: "PublicThumbnailPath", Type: "String", Length: 500, WriteForbidden: true},
		{Name: "EnableCaching", Type: "Boolean"},
	}},
	{Name: "XASInstance", Persistable: true, HasCreatedDate: true, Attributes: []SystemAttrDef{
		{Name: "XASId", Type: "String", Length: 50},
		{Name: "LastUpdate", Type: "DateTime"},
		{Name: "AllowedNumberOfConcurrentUsers", Type: "Integer"},
		{Name: "PartnerName", Type: "String", Length: 200},
		{Name: "CustomerName", Type: "String", Length: 200},
	}},
	{Name: "Session", Persistable: true, HasCreatedDate: true, Attributes: []SystemAttrDef{
		{Name: "SessionId", Type: "String", Length: 50},
		{Name: "CSRFToken", Type: "String", Length: 36},
		{Name: "LastActive", Type: "DateTime"},
	}},
	{Name: "ScheduledEventInformation", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Name", Type: "String", Length: 200},
		{Name: "Description", Type: "String"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Status", Type: "Enumeration", EnumQN: "System.EventStatus"},
	}},
	{Name: "Language", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Code", Type: "String", Length: 20},
		{Name: "Description", Type: "String", Length: 200},
	}},
	{Name: "TimeZone", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Code", Type: "String", Length: 50},
		{Name: "Description", Type: "String", Length: 100},
		{Name: "RawOffset", Type: "Integer"},
	}},
	{Name: "Error", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "ErrorType", Type: "String"},
		{Name: "Message", Type: "String"},
		{Name: "Stacktrace", Type: "String"},
	}},
	{Name: "SoapFault", Persistable: true, Generalization: "System.Error", Attributes: []SystemAttrDef{
		{Name: "Code", Type: "String"},
		{Name: "Reason", Type: "String"},
		{Name: "Node", Type: "String"},
		{Name: "Role", Type: "String"},
		{Name: "Detail", Type: "String"},
	}},
	{Name: "TokenInformation", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Token", Type: "HashedString"},
		{Name: "ExpiryDate", Type: "DateTime"},
		{Name: "UserAgent", Type: "String"},
	}},
	{Name: "HttpMessage", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "HttpVersion", Type: "String", Length: 10},
		{Name: "Content", Type: "String"},
	}},
	{Name: "HttpHeader", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "Key", Type: "String"},
		{Name: "Value", Type: "String"},
	}},
	{Name: "UserReportInfo", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "UserType", Type: "Enumeration", EnumQN: "System.UserType"},
		{Name: "Hash", Type: "String", Length: 64},
	}},
	{Name: "HttpRequest", Persistable: true, Generalization: "System.HttpMessage", Attributes: []SystemAttrDef{
		{Name: "Uri", Type: "String"},
	}},
	{Name: "HttpResponse", Persistable: true, Generalization: "System.HttpMessage", Attributes: []SystemAttrDef{
		{Name: "StatusCode", Type: "Integer"},
		{Name: "ReasonPhrase", Type: "String"},
	}},
	{Name: "Paging", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "PageNumber", Type: "Long"},
		{Name: "IsSortable", Type: "Boolean"},
		{Name: "SortAttribute", Type: "String", Length: 200},
		{Name: "SortAscending", Type: "Boolean"},
		{Name: "HasMoreData", Type: "Boolean"},
	}},
	{Name: "SynchronizationError", Persistable: true, HasOwner: true, HasCreatedDate: true, Attributes: []SystemAttrDef{
		{Name: "Reason", Type: "String"},
		{Name: "ObjectId", Type: "String", Length: 200},
		{Name: "ObjectType", Type: "String", Length: 1000},
		{Name: "ObjectContent", Type: "String"},
	}},
	{Name: "SynchronizationErrorFile", Persistable: true, Generalization: "System.FileDocument"},
	{Name: "ProcessedQueueTask", Persistable: true, HasOwner: true, Attributes: []SystemAttrDef{
		{Name: "Sequence", Type: "Long"},
		{Name: "Status", Type: "Enumeration", EnumQN: "System.QueueTaskStatus"},
		{Name: "QueueId", Type: "String", Length: 36},
		{Name: "QueueName", Type: "String", Length: 200},
		{Name: "ContextType", Type: "Enumeration", EnumQN: "System.ContextType"},
		{Name: "ContextData", Type: "String"},
		{Name: "MicroflowName", Type: "String", Length: 200},
		{Name: "UserActionName", Type: "String", Length: 200},
		{Name: "Arguments", Type: "String"},
		{Name: "XASId", Type: "String", Length: 50},
		{Name: "ThreadId", Type: "Long"},
		{Name: "Created", Type: "DateTime"},
		{Name: "StartAt", Type: "DateTime"},
		{Name: "Started", Type: "DateTime"},
		{Name: "Finished", Type: "DateTime"},
		{Name: "Duration", Type: "Long"},
		{Name: "Retried", Type: "Long"},
		{Name: "ErrorMessage", Type: "String"},
		{Name: "ScheduledEventName", Type: "String", Length: 200},
	}},
	{Name: "QueuedTask", Persistable: true, HasOwner: true, Attributes: []SystemAttrDef{
		{Name: "Sequence", Type: "AutoNumber"},
		{Name: "Status", Type: "Enumeration", EnumQN: "System.QueueTaskStatus"},
		{Name: "QueueId", Type: "String", Length: 36},
		{Name: "QueueName", Type: "String", Length: 200},
		{Name: "ContextType", Type: "Enumeration", EnumQN: "System.ContextType"},
		{Name: "ContextData", Type: "String"},
		{Name: "MicroflowName", Type: "String", Length: 200},
		{Name: "UserActionName", Type: "String", Length: 200},
		{Name: "Arguments", Type: "String"},
		{Name: "XASId", Type: "String", Length: 50},
		{Name: "ThreadId", Type: "Long"},
		{Name: "Created", Type: "DateTime"},
		{Name: "StartAt", Type: "DateTime"},
		{Name: "Started", Type: "DateTime"},
		{Name: "Retried", Type: "Long"},
		{Name: "Retry", Type: "String", Length: 200},
		{Name: "ScheduledEventName", Type: "String", Length: 200},
	}},
	{Name: "WorkflowDefinition", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Name", Type: "String", Length: 200},
		{Name: "Title", Type: "String", Length: 200},
		{Name: "IsObsolete", Type: "Boolean"},
		{Name: "IsLocked", Type: "Boolean"},
	}},
	{Name: "WorkflowUserTaskDefinition", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Name", Type: "String", Length: 200},
		{Name: "IsObsolete", Type: "Boolean"},
	}},
	{Name: "Workflow", Persistable: true, HasOwner: true, Attributes: []SystemAttrDef{
		{Name: "Name", Type: "String", Length: 200},
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
	{Name: "WorkflowUserTask", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "DueDate", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Outcome", Type: "String", Length: 200},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskState"},
		{Name: "CompletionType", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskCompletionType"},
	}},
	{Name: "TaskQueueToken", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "QueueName", Type: "String", Length: 200},
		{Name: "XASId", Type: "String", Length: 50},
		{Name: "ValidUntil", Type: "DateTime"},
	}},
	{Name: "ODataResponse", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "Count", Type: "Long"},
	}},
	{Name: "WorkflowJumpToDetails", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "Error", Type: "String"},
	}},
	{Name: "WorkflowCurrentActivity", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "Action", Type: "Enumeration", EnumQN: "System.WorkflowCurrentActivityAction"},
	}},
	{Name: "WorkflowActivityDetails", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "ActivityId", Type: "String", Length: 50},
		{Name: "ActivityCaption", Type: "String"},
		{Name: "ActivityType", Type: "Enumeration", EnumQN: "System.WorkflowActivityType"},
		{Name: "ExistsInCurrentVersion", Type: "Boolean"},
	}},
	{Name: "WorkflowUserTaskOutcome", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Outcome", Type: "String", Length: 200},
		{Name: "Time", Type: "DateTime"},
	}},
	{Name: "WorkflowRecord", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "WorkflowKey", Type: "String", Length: 200},
		{Name: "Name", Type: "String", Length: 200},
		{Name: "Description", Type: "String"},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowState"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "DueDate", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Reason", Type: "String"},
	}},
	{Name: "WorkflowActivityRecord", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "ModelGUID", Type: "String", Length: 200},
		{Name: "ActivityKey", Type: "String", Length: 200},
		{Name: "PreviousActivityKey", Type: "String", Length: 200},
		{Name: "ActivityType", Type: "Enumeration", EnumQN: "System.WorkflowActivityType"},
		{Name: "Caption", Type: "String"},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowActivityExecutionState"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Outcome", Type: "String", Length: 200},
		{Name: "MicroflowName", Type: "String", Length: 200},
		{Name: "TaskName", Type: "String"},
		{Name: "TaskDescription", Type: "String"},
		{Name: "TaskDueDate", Type: "DateTime"},
		{Name: "TaskCompletionType", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskCompletionType"},
		{Name: "TaskRequiredUsers", Type: "Integer"},
		{Name: "TaskKey", Type: "String", Length: 200},
		{Name: "Reason", Type: "String"},
	}},
	{Name: "WorkflowEvent", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "EventTime", Type: "DateTime"},
		{Name: "EventType", Type: "Enumeration", EnumQN: "System.WorkflowEventType"},
	}},
	{Name: "ConsumedODataConfiguration", Persistable: false, Attributes: []SystemAttrDef{
		{Name: "ServiceUrl", Type: "String"},
		{Name: "ProxyConfiguration", Type: "Enumeration", EnumQN: "System.ProxyConfiguration"},
		{Name: "ProxyHost", Type: "String"},
		{Name: "ProxyPort", Type: "Integer"},
		{Name: "ProxyUsername", Type: "String"},
		{Name: "ProxyPassword", Type: "String"},
	}},
	{Name: "WorkflowEndedUserTask", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Name", Type: "String"},
		{Name: "Description", Type: "String"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "DueDate", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Outcome", Type: "String", Length: 200},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskState"},
		{Name: "CompletionType", Type: "Enumeration", EnumQN: "System.WorkflowUserTaskCompletionType"},
		{Name: "UserTaskKey", Type: "String", Length: 200},
	}},
	{Name: "WorkflowEndedUserTaskOutcome", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Outcome", Type: "String", Length: 200},
		{Name: "Time", Type: "DateTime"},
	}},
	{Name: "WorkflowGroup", Persistable: true, Attributes: []SystemAttrDef{
		{Name: "Name", Type: "String", Length: 200},
		{Name: "Description", Type: "String"},
	}},
	// --- Entities below extracted from MDP (Phase 3, 2026-04-24) ---
	{Name: "WorkflowVersion", Persistable: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "VersionHash", Type: "String", Length: 200},
		{Name: "ModelJSON", Type: "String"},
	}},
	{Name: "WorkflowActivity", Persistable: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "ModelGUID", Type: "String", Length: 36},
		{Name: "ActivityGUID", Type: "String", Length: 36},
		{Name: "Caption", Type: "String"},
		{Name: "DetailsJson", Type: "String"},
		{Name: "State", Type: "Enumeration", EnumQN: "System.WorkflowActivityState"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "ActionTime", Type: "DateTime"},
		{Name: "Reason", Type: "String"},
		{Name: "ActivityHash", Type: "String", Length: 200},
		{Name: "IsDerivedActivity", Type: "Boolean"},
		{Name: "Outcome", Type: "String", Length: 200},
		{Name: "OutcomeModelGUID", Type: "String", Length: 36},
	}},
	{Name: "WorkflowActivityUserTaskOutcome", Persistable: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "Outcome", Type: "String", Length: 200},
		{Name: "Time", Type: "DateTime"},
	}},
	{Name: "PrivateFileDocument", Persistable: true, RuntimeOnly: true, Generalization: "System.FileDocument"},
	{Name: "Thumbnail", Persistable: true, RuntimeOnly: true, Generalization: "System.Image"},
	{Name: "BackgroundJob", Persistable: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "JobId", Type: "Long"},
		{Name: "StartTime", Type: "DateTime"},
		{Name: "EndTime", Type: "DateTime"},
		{Name: "Result", Type: "String"},
		{Name: "Successful", Type: "Boolean"},
	}},
	{Name: "AutoCommitEntry", Persistable: true, HasCreatedDate: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "SessionId", Type: "String", Length: 36},
		{Name: "ObjectId", Type: "Long"},
	}},
	{Name: "UnreferencedFile", Persistable: true, HasCreatedDate: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "FileKey", Type: "String", Length: 36},
		{Name: "State", Type: "Enumeration", EnumQN: "System.UnreferencedFileState"},
		{Name: "TransactionId", Type: "String", Length: 36},
	}},
	{Name: "OfflineCreatedGuids", Persistable: true, HasCreatedDate: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "Guid", Type: "String", Length: 200},
	}},
	{Name: "OfflineSynchronizationHistory", Persistable: true, HasCreatedDate: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "SyncId", Type: "String", Length: 200},
	}},
	{Name: "ChangeHash", Persistable: true, HasCreatedDate: true, RuntimeOnly: true, Attributes: []SystemAttrDef{
		{Name: "ObjectId", Type: "Long"},
		{Name: "Attribute", Type: "String", Length: 200},
		{Name: "Hash", Type: "String", Length: 200},
	}},
}

// SystemAssociations lists all associations in the System module.
// Extracted from Mendix Studio Pro 11.6.4 via DummySystem module.
var SystemAssociations = []SystemAssocDef{
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
	// One association, renamed in 10.0 with the same GUID and ends: before it
	// is WorkflowUserTask_Assignee, a Reference (one user). Read from the
	// modeler assembly of 9.24.30 (Assignee), 10.0.0 and 11.12.6 (Assignees).
	// A 9.x model naming Assignees is CE1613 "no longer exists" (ako/mxcli#1810).
	{Name: "WorkflowUserTask_Assignees", Parent: "WorkflowUserTask", Child: "User", Type: "ReferenceSet", Owner: "Default", Introduced: "10.0"},
	{Name: "WorkflowUserTask_Assignee", Parent: "WorkflowUserTask", Child: "User", Type: "Reference", Owner: "Default", Removed: "10.0"},
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
	// --- Associations below extracted from MDP (Phase 3, 2026-04-24) ---
	{Name: "WorkflowVersion_WorkflowDefinition", Parent: "WorkflowVersion", Child: "WorkflowDefinition", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowVersion_WorkflowUserTaskDefinition", Parent: "WorkflowVersion", Child: "WorkflowUserTaskDefinition", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowVersion_PreviousVersion", Parent: "WorkflowVersion", Child: "WorkflowVersion", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowDefinition_CurrentWorkflowVersion", Parent: "WorkflowDefinition", Child: "WorkflowVersion", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivity_Workflow", Parent: "WorkflowActivity", Child: "Workflow", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivity_WorkflowUserTask", Parent: "WorkflowActivity", Child: "WorkflowUserTask", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivity_PreviousActivity", Parent: "WorkflowActivity", Child: "WorkflowActivity", Type: "ReferenceSet", Owner: "Default"},
	{Name: "Workflow_CurrentActivity", Parent: "Workflow", Child: "WorkflowActivity", Type: "ReferenceSet", Owner: "Default"},
	{Name: "WorkflowActivity_WorkflowVersion", Parent: "WorkflowActivity", Child: "WorkflowVersion", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivity_Actor", Parent: "WorkflowActivity", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivity_SubWorkflow", Parent: "WorkflowActivity", Child: "Workflow", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivityUserTaskOutcome_WorkflowActivity", Parent: "WorkflowActivityUserTaskOutcome", Child: "WorkflowActivity", Type: "Reference", Owner: "Default"},
	{Name: "WorkflowActivityUserTaskOutcome_User", Parent: "WorkflowActivityUserTaskOutcome", Child: "User", Type: "Reference", Owner: "Default"},
	{Name: "Thumbnail_Image", Parent: "Thumbnail", Child: "Image", Type: "Reference", Owner: "Both"},
	{Name: "BackgroundJob_Session", Parent: "BackgroundJob", Child: "Session", Type: "Reference", Owner: "Default"},
	{Name: "BackgroundJob_XASInstance", Parent: "BackgroundJob", Child: "XASInstance", Type: "Reference", Owner: "Default"},
	{Name: "UnreferencedFile_XASInstance", Parent: "UnreferencedFile", Child: "XASInstance", Type: "Reference", Owner: "Default"},
	{Name: "ChangeHash_Session", Parent: "ChangeHash", Child: "Session", Type: "Reference", Owner: "Default"},
}

// SystemEnumDef defines an enumeration in the System module.
type SystemEnumDef struct {
	Name   string   // qualified name, e.g. "System.EventStatus"
	Values []string // enumeration value names
}

// SystemEnumerations lists all enumerations in the System module.
// Extracted from Mendix mxbuild 11.6.4 MDP output.
var SystemEnumerations = []SystemEnumDef{
	{Name: "System.ContextType", Values: []string{
		"System", "User", "Anonymous", "ScheduledEvent",
	}},
	{Name: "System.EventStatus", Values: []string{
		"Running", "Completed", "Error", "Stopped",
	}},
	{Name: "System.DeviceType", Values: []string{
		"Phone", "Tablet", "Desktop",
	}},
	{Name: "System.UserType", Values: []string{
		"Internal", "External",
	}},
	{Name: "System.ProxyConfiguration", Values: []string{
		"UseAppSettings", "Override", "NoProxy",
	}},
	{Name: "System.QueueTaskStatus", Values: []string{
		"Idle", "Running", "Completed", "Failed", "Retrying", "Aborted", "Incompatible",
	}},
	{Name: "System.WorkflowActivityState", Values: []string{
		"Started", "Suspended", "Finished", "Replaced", "Aborted", "Failed",
	}},
	{Name: "System.UnreferencedFileState", Values: []string{
		"New", "Obsolete", "Deleted",
	}},
	{Name: "System.WorkflowActivityExecutionState", Values: []string{
		"Created", "InProgress", "Completed", "Paused", "Aborted", "Failed",
	}},
	{Name: "System.WorkflowState", Values: []string{
		"InProgress", "Paused", "Completed", "Aborted", "Incompatible", "Failed",
	}},
	{Name: "System.WorkflowUserTaskState", Values: []string{
		"Created", "InProgress", "Completed", "Paused", "Aborted", "Failed",
	}},
	{Name: "System.WorkflowActivityType", Values: []string{
		"Start", "End", "ExclusiveSplit", "ParallelSplit",
		"ParallelSplitBranchStopper", "ParallelSplitMerge",
		"UserTask", "CallMicroflow", "CallWorkflow", "JumpTo",
		"MultiInputUserTask", "WaitForNotification", "WaitForTimer",
		"EndOfBoundaryEventPath", "NonInterruptingTimerEvent", "InterruptingTimerEvent",
	}},
	{Name: "System.WorkflowCurrentActivityAction", Values: []string{
		"DoNothing", "JumpTo",
	}},
	{Name: "System.WorkflowUserTaskCompletionType", Values: []string{
		"Single", "Veto", "Consensus", "Majority", "Threshold", "Microflow",
	}},
	{Name: "System.WorkflowEventType", Values: []string{
		"WorkflowCompleted", "WorkflowInitiated", "WorkflowRestarted",
		"WorkflowFailed", "WorkflowAborted", "WorkflowPaused",
		"WorkflowUnpaused", "WorkflowRetried", "WorkflowUpdated",
		"WorkflowUpgraded", "WorkflowConflicted", "WorkflowResolved",
		"WorkflowJumpToOptionApplied",
		"StartEventExecuted", "EndEventExecuted", "DecisionExecuted",
		"JumpExecuted", "ParallelSplitExecuted", "ParallelMergeExecuted",
		"CallWorkflowStarted", "CallWorkflowEnded",
		"CallMicroflowStarted", "CallMicroflowEnded",
		"WaitForNotificationStarted", "WaitForNotificationEnded",
		"WaitForTimerStarted", "WaitForTimerEnded",
		"UserTaskStarted", "MultiUserTaskOutcomeSelected", "UserTaskEnded",
		"NonInterruptingTimerEventExecuted", "InterruptingTimerEventExecuted",
	}},
}
