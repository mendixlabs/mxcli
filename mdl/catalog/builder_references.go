// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Reference kinds for the refs table
const (
	RefKindCall        = "call"         // Microflow calls microflow (also: a workflow's handler, a consumed OData service's configuration microflow)
	RefKindCreate      = "create"       // Microflow creates entity
	RefKindRetrieve    = "retrieve"     // Microflow retrieves entity
	RefKindShowPage    = "show_page"    // Microflow shows page
	RefKindGeneralize  = "generalize"   // Entity extends entity
	RefKindAssociate   = "associate"    // Association targets entity
	RefKindLayout      = "layout"       // Page uses layout
	RefKindSnippetCall = "snippet_call" // Page, snippet or layout calls a snippet
	RefKindDatasource  = "datasource"   // Page/widget uses entity via datasource
	RefKindParameter   = "parameter"    // Page, snippet or flow parameter entity type
	RefKindAction      = "action"       // Widget calls microflow/nanoflow
	RefKindHomePage    = "home_page"    // Navigation home page reference
	RefKindLoginPage   = "login_page"   // Navigation login page reference
	RefKindMenuItem    = "menu_item"    // Navigation menu item page reference
	RefKindChange      = "change"       // Microflow changes an entity object
	RefKindDelete      = "delete"       // Microflow deletes an entity object
	RefKindCommit      = "commit"       // Microflow commits an entity object (commit action, or create/change with commit)
	RefKindCalculate   = "calculate"    // Calculated attribute uses a microflow
	RefKindReturn      = "return"       // Microflow/nanoflow returns an entity type
	RefKindSchedule    = "schedule"     // Scheduled event runs a microflow
	RefKindValidate    = "validate"     // Attribute validation rule uses a regular expression
	RefKindWidget      = "widget"       // Page/snippet uses a pluggable or custom widget
	RefKindSettings    = "settings"     // A project setting names a microflow
	RefKindSync        = "sync"         // An offline navigation profile synchronizes an entity
	RefKindPublish     = "publish"      // A published service runs a microflow (REST operation, OData resource or action, authentication)
	RefKindEvent       = "event"        // An event handler runs a microflow (entity event, or a business event a service subscribes to)
	RefKindMember      = "member"       // A document binds, reads or writes an attribute, navigates an association, or reaches the entity an association step arrives at
	RefKindXPath       = "xpath"        // An XPath constraint names an attribute, association or enumeration value
	RefKindType        = "type"         // An attribute, parameter or variable is typed as an enumeration, or a result variable, created list or entity-type argument as an entity
	RefKindValue       = "value"        // An expression names an enumeration value (an ENUMERATION target when that value does not exist)
	RefKindMapping     = "mapping"      // An import or export mapping maps an entity, or a message definition exposes one
	RefKindSplit       = "split"        // A decision branches on an object being a specialization
	RefKindMappingCall = "mapping_call" // A flow applies an import or export mapping (import/export action, REST or web service call)
	RefKindStructure   = "structure"    // An import or export mapping is built on a JSON structure
	RefKindTool        = "tool"         // An agent draws on a knowledge base or consumed MCP service (a tool microflow, nanoflow or agent is a call)
	RefKindModel       = "model"        // An agent runs on an AI model
	RefKindKey         = "key"          // An AI model or knowledge base reads its key from a constant
)

// RefKinds is every value that reaches refs.RefKind. It is the vocabulary a
// consumer of the reference graph iterates when it must decide something per
// kind: TestQUAL004CountsEveryRefKind drives each one at the orphan rule, so an
// edge kind added here is covered there without anyone remembering the rule.
// TestEveryObjectTypeConstantIsInAPublishedList holds it complete against the
// RefKind constants above.
var RefKinds = []string{
	RefKindCall, RefKindCreate, RefKindRetrieve, RefKindShowPage,
	RefKindGeneralize, RefKindAssociate, RefKindLayout, RefKindDatasource,
	RefKindParameter, RefKindAction, RefKindHomePage, RefKindLoginPage,
	RefKindMenuItem, RefKindChange, RefKindDelete, RefKindCommit,
	RefKindCalculate, RefKindReturn, RefKindSchedule, RefKindValidate,
	RefKindWidget, RefKindSettings, RefKindSync, RefKindPublish,
	RefKindEvent, RefKindMember, RefKindXPath, RefKindType,
	RefKindValue, RefKindMapping, RefKindSplit, RefKindSnippetCall,
	RefKindMappingCall, RefKindStructure, RefKindTool, RefKindModel,
	RefKindKey,
}

// Object types recorded in refs.SourceType and refs.TargetType — the catalog's
// name for "what kind of document is at this end of the edge". They are the SDK
// names, upper-cased, never Mendix's BSON storage names.
//
// Named here because they are a public vocabulary: Starlark lint rules filter on
// them through `reference.source_type` / `target_type`, and the write-lint-rules
// skill documents them. While they were only string literals at the emit sites
// there was nothing for the documentation to be checked against, and it drifted
// into lower-case examples that match nothing (mendixlabs/mxcli#1027).
const (
	RefObjectEntity                 = "ENTITY"
	RefObjectAssociation            = "ASSOCIATION"
	RefObjectMicroflow              = "MICROFLOW"
	RefObjectNanoflow               = "NANOFLOW"
	RefObjectRule                   = "RULE"
	RefObjectPage                   = "PAGE"
	RefObjectSnippet                = "SNIPPET"
	RefObjectLayout                 = "LAYOUT"
	RefObjectWorkflow               = "WORKFLOW"
	RefObjectNavigation             = "NAVIGATION"
	RefObjectWidget                 = "WIDGET"
	RefObjectJavaAction             = "JAVA_ACTION"
	RefObjectJavaScriptAction       = "JAVASCRIPT_ACTION"
	RefObjectRestOperation          = "REST_OPERATION"
	RefObjectPublishedRestOperation = "PUBLISHED_REST_OPERATION"
	RefObjectPublishedRestService   = "PUBLISHED_REST_SERVICE"
	RefObjectPublishedODataService  = "PUBLISHED_ODATA_SERVICE"
	RefObjectConsumedODataService   = "CONSUMED_ODATA_SERVICE"
	RefObjectBusinessEventService   = "BUSINESS_EVENT_SERVICE"
	RefObjectRegularExpression      = "REGULAR_EXPRESSION"
	RefObjectScheduledEvent         = "SCHEDULED_EVENT"
	RefObjectProjectSettings        = "PROJECT_SETTINGS"
	RefObjectAttribute              = "ATTRIBUTE"
	RefObjectEnumeration            = "ENUMERATION"
	RefObjectEnumerationValue       = "ENUMERATION_VALUE"
	RefObjectImportMapping          = "IMPORT_MAPPING"
	RefObjectExportMapping          = "EXPORT_MAPPING"
	RefObjectJsonStructure          = "JSON_STRUCTURE"
	// Message definitions: the pre-11.15 collection and the 11.15+ document.
	RefObjectMessageDefinitionCollection = "MESSAGE_DEFINITION_COLLECTION"
	RefObjectMessageDefinition           = "MESSAGE_DEFINITION"
	// Agent-editor documents and the constant a model's key is read from
	// (ako/mxcli#2074), named as the objects view names them.
	RefObjectAgent              = "AGENT"
	RefObjectAIModel            = "AI_MODEL"
	RefObjectKnowledgeBase      = "KNOWLEDGE_BASE"
	RefObjectConsumedMCPService = "CONSUMED_MCP_SERVICE"
	RefObjectConstant           = "CONSTANT"
)

// RefSourceObjectTypes is every value that reaches refs.SourceType, and
// RefTargetObjectTypes every value that reaches refs.TargetType. The two differ:
// a WIDGET is only ever pointed AT, and a SCHEDULED_EVENT or a
// PROJECT_SETTINGS only ever points. A LAYOUT is both: a page uses one, and
// since ako/mxcli#1302 a layout's own widgets make edges of their own. Documenting one list for both would put
// values in front of rule authors that their filter can never match.
var (
	RefSourceObjectTypes = []string{
		RefObjectEntity,
		RefObjectAssociation,
		RefObjectMicroflow,
		RefObjectNanoflow,
		RefObjectRule,
		RefObjectPage,
		RefObjectSnippet,
		RefObjectLayout,
		RefObjectWorkflow,
		RefObjectNavigation,
		RefObjectScheduledEvent,
		RefObjectPublishedRestOperation,
		RefObjectPublishedRestService,
		RefObjectPublishedODataService,
		RefObjectConsumedODataService,
		RefObjectBusinessEventService,
		RefObjectProjectSettings,
		RefObjectImportMapping,
		RefObjectExportMapping,
		RefObjectMessageDefinitionCollection,
		RefObjectMessageDefinition,
		RefObjectAgent,
		RefObjectAIModel,
		RefObjectKnowledgeBase,
	}

	RefTargetObjectTypes = []string{
		RefObjectEntity,
		RefObjectAssociation,
		RefObjectMicroflow,
		RefObjectNanoflow,
		RefObjectRule,
		RefObjectPage,
		RefObjectSnippet,
		RefObjectLayout,
		RefObjectWorkflow,
		RefObjectWidget,
		RefObjectJavaAction,
		RefObjectJavaScriptAction,
		RefObjectRestOperation,
		RefObjectRegularExpression,
		RefObjectAttribute,
		RefObjectEnumeration,
		RefObjectEnumerationValue,
		RefObjectImportMapping,
		RefObjectExportMapping,
		RefObjectJsonStructure,
		RefObjectAgent,
		RefObjectAIModel,
		RefObjectKnowledgeBase,
		RefObjectConsumedMCPService,
		RefObjectConstant,
	}
)

// collectActionActivities returns all ActionActivity objects from an ObjectCollection,
// recursing into LoopedActivity bodies to find nested actions.
func collectActionActivities(oc *microflows.MicroflowObjectCollection) []*microflows.ActionActivity {
	if oc == nil {
		return nil
	}
	var result []*microflows.ActionActivity
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if o.Action != nil {
				result = append(result, o)
			}
		case *microflows.LoopedActivity:
			result = append(result, collectActionActivities(o.ObjectCollection)...)
		}
	}
	return result
}

// collectRuleCalls returns the qualified name of every rule a flow calls from a
// decision, recursing into LoopedActivity bodies like collectActionActivities.
//
// A rule is not an activity: Mendix can only call one from an ExclusiveSplit's
// condition, so the action walk above never sees it and a rule called from a
// decision looked uncalled — `show callers` reported none, which is how #939's
// reporter read the corruption as "the reference never resolves" even after the
// condition was stored correctly.
func collectRuleCalls(oc *microflows.MicroflowObjectCollection) []string {
	if oc == nil {
		return nil
	}
	var out []string
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ExclusiveSplit:
			if rc, ok := o.SplitCondition.(*microflows.RuleSplitCondition); ok && rc.RuleQualifiedName != "" {
				out = append(out, rc.RuleQualifiedName)
			}
		case *microflows.LoopedActivity:
			out = append(out, collectRuleCalls(o.ObjectCollection)...)
		}
	}
	return out
}

// collectInheritanceCases returns the entity of every branch of an
// inheritance split (`split type $x when Module.Entity`), once each,
// recursing into loops. The split is no activity and names the entity only
// on its outgoing flows, so without this a flow that branches on a
// specialization had no edge to it: `show references` missed the flow, and
// the incremental catalog build could not tell it depends on the entity's
// module (found by the ref coverage trace, ako/mxcli#1804).
func collectInheritanceCases(oc *microflows.MicroflowObjectCollection) []string {
	if oc == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(c *microflows.MicroflowObjectCollection) {
		for _, f := range c.Flows {
			if ic, ok := f.CaseValue.(*microflows.InheritanceCase); ok && ic.EntityQualifiedName != "" && !seen[ic.EntityQualifiedName] {
				seen[ic.EntityQualifiedName] = true
				out = append(out, ic.EntityQualifiedName)
			}
		}
		for _, obj := range c.Objects {
			if l, ok := obj.(*microflows.LoopedActivity); ok && l.ObjectCollection != nil {
				walk(l.ObjectCollection)
			}
		}
	}
	walk(oc)
	return out
}

// microflowActionRef returns the cross-reference a microflow action makes to
// another document: the target's catalog object type, its qualified name, and
// the RefKind label. ok is false when the action references no resolvable
// document target — either it operates only on local variables (change/delete/
// commit/cast resolve their entity through a variable's type, which needs
// dataflow analysis) or the reference is by ID and the name wasn't captured.
//
// Like getMicroflowActionType, this is a single switch rather than ref-emission
// scattered through buildReferences, so the set of reference-bearing actions is
// auditable in one place and unit-testable without a full catalog build.
func microflowActionRef(action microflows.MicroflowAction) (targetType, targetName, refKind string, ok bool) {
	switch a := action.(type) {
	case *microflows.MicroflowCallAction:
		if a.MicroflowCall != nil && a.MicroflowCall.Microflow != "" {
			return RefObjectMicroflow, a.MicroflowCall.Microflow, RefKindCall, true
		}
	case *microflows.NanoflowCallAction:
		if a.NanoflowCall != nil && a.NanoflowCall.Nanoflow != "" {
			return RefObjectNanoflow, a.NanoflowCall.Nanoflow, RefKindCall, true
		}
	case *microflows.JavaActionCallAction:
		if a.JavaAction != "" {
			return RefObjectJavaAction, a.JavaAction, RefKindCall, true
		}
	case *microflows.JavaScriptActionCallAction:
		// A nanoflow's JavaScript action call had no edge at all, so `show
		// callers` and refs_from() never saw it (mendixlabs/mxcli#1305).
		if a.JavaScriptAction != "" {
			return RefObjectJavaScriptAction, a.JavaScriptAction, RefKindCall, true
		}
	case *microflows.RestOperationCallAction:
		// Operation is a "Module.Service.Operation" name referencing a consumed
		// REST service operation.
		if a.Operation != "" {
			return RefObjectRestOperation, a.Operation, RefKindCall, true
		}
	case *microflows.WorkflowCallAction:
		// A microflow that starts a workflow is that workflow's caller. Without
		// this edge a workflow started only from a microflow had no inbound
		// reference: `show callers` said "(no callers found)" and `impact` "not
		// referenced" (Evora: AltairIntegration.WF_ScheduleTechnicianAppointment).
		if a.Workflow != "" {
			return RefObjectWorkflow, a.Workflow, RefKindCall, true
		}
	case *microflows.CreateObjectAction:
		if a.EntityQualifiedName != "" {
			return RefObjectEntity, a.EntityQualifiedName, RefKindCreate, true
		}
	case *microflows.ShowPageAction:
		if a.PageName != "" {
			return RefObjectPage, a.PageName, RefKindShowPage, true
		}
	case *microflows.RetrieveAction:
		if a.Source == nil {
			break
		}
		switch src := a.Source.(type) {
		case *microflows.DatabaseRetrieveSource:
			if src.EntityQualifiedName != "" {
				return RefObjectEntity, src.EntityQualifiedName, RefKindRetrieve, true
			}
		case *microflows.AssociationRetrieveSource:
			if src.AssociationQualifiedName != "" {
				return RefObjectAssociation, src.AssociationQualifiedName, RefKindRetrieve, true
			}
		}
	}
	return "", "", "", false
}

// mappingRef is one mapping an action applies: IMPORT_MAPPING or EXPORT_MAPPING
// and its qualified name.
type mappingRef struct{ targetType, name string }

// microflowMappingRefs returns every mapping an action applies. Each holder
// stores the mapping by qualified name — the field is typed model.ID but the
// reader fills it from the BY_NAME key. A REST or web service call can name an
// export mapping for the request AND an import mapping for the response, so
// this returns a list rather than joining microflowActionRef's single target.
// Without these edges a mapping in use had no inbound reference and read as
// dead (ako/mxcli#1861).
func microflowMappingRefs(action microflows.MicroflowAction) []mappingRef {
	var out []mappingRef
	add := func(targetType string, id model.ID) {
		if id != "" {
			out = append(out, mappingRef{targetType, string(id)})
		}
	}
	importOf := func(rh microflows.ResultHandling) {
		if m, ok := rh.(*microflows.ResultHandlingMapping); ok && m != nil {
			add(RefObjectImportMapping, m.MappingID)
		}
	}
	switch a := action.(type) {
	case *microflows.ImportXmlAction:
		if a.ResultHandling != nil {
			add(RefObjectImportMapping, a.ResultHandling.MappingID)
		}
	case *microflows.ExportXmlAction:
		if a.RequestHandling != nil {
			add(RefObjectExportMapping, a.RequestHandling.MappingID)
		}
	case *microflows.RestCallAction:
		if m, ok := a.RequestHandling.(*microflows.MappingRequestHandling); ok && m != nil {
			add(RefObjectExportMapping, m.MappingID)
		}
		importOf(a.ResultHandling)
	case *microflows.WebServiceCallAction:
		add(RefObjectExportMapping, a.SendMappingID)
		add(RefObjectImportMapping, a.ReceiveMappingID)
	}
	return out
}

// entityOfDataType returns the entity qualified name a microflow data type refers
// to (object or list of objects), or "" for primitive/enum/void types.
func entityOfDataType(dt microflows.DataType) string {
	switch t := dt.(type) {
	case *microflows.ObjectType:
		return t.EntityQualifiedName
	case *microflows.ListType:
		return t.EntityQualifiedName
	}
	return ""
}

// microflowVarActionRef resolves the entity reference of an action that operates
// on a *variable* (change/delete) rather than naming an entity directly. varEntity
// maps a (normalised) variable name to the entity qualified name it holds, built
// by the caller from parameters + create/retrieve outputs in the same flow. This
// is intra-flow resolution only — no cross-flow dataflow — so it returns ok=false
// when the variable's entity isn't known locally.
func microflowVarActionRef(action microflows.MicroflowAction, varEntity map[string]string) (targetType, targetName, refKind string, ok bool) {
	resolve := func(v string) (string, bool) {
		qn, found := varEntity[strings.TrimPrefix(v, "$")]
		return qn, found && qn != ""
	}
	switch a := action.(type) {
	case *microflows.ChangeObjectAction:
		if qn, found := resolve(a.ChangeVariable); found {
			return RefObjectEntity, qn, RefKindChange, true
		}
	case *microflows.DeleteObjectAction:
		if qn, found := resolve(a.DeleteVariable); found {
			return RefObjectEntity, qn, RefKindDelete, true
		}
	}
	return "", "", "", false
}

// microflowCommitRef resolves the entity an action commits: a commit action's
// variable, or the object of a create or change that commits (Yes or
// YesWithoutEvents). It is a second edge beside the create/change one, so
// "which flows commit entity X" is one refs query — the question a commit in a
// loop or a commit without events starts from (mendixlabs/mxcli#1266, #1267).
// A commit of a variable whose entity is not known in the flow emits nothing,
// as for change and delete (ako/mxcli#963).
func microflowCommitRef(action microflows.MicroflowAction, varEntity map[string]string) (entity string, ok bool) {
	commits := func(c microflows.CommitType) bool {
		return c == microflows.CommitTypeYes || c == microflows.CommitTypeYesWithoutEvents
	}
	resolve := func(v string) (string, bool) {
		qn, found := varEntity[strings.TrimPrefix(v, "$")]
		return qn, found && qn != ""
	}
	switch a := action.(type) {
	case *microflows.CommitObjectsAction:
		return resolve(a.CommitVariable)
	case *microflows.CreateObjectAction:
		if commits(a.Commit) && a.EntityQualifiedName != "" {
			return a.EntityQualifiedName, true
		}
	case *microflows.ChangeObjectAction:
		if commits(a.Commit) {
			return resolve(a.ChangeVariable)
		}
	}
	return "", false
}

// assocEnds are an association's endpoint entities: From owns the reference
// (BSON ParentPointer), To is referenced (ChildPointer).
type assocEnds struct{ From, To string }

// buildVarEntityMap builds the variable→entity map for a single flow: seed with
// object-typed parameters, then walk the flow — loop bodies included — mapping
// create/retrieve outputs whose entity is known and each loop iterator to the
// entity of the list it iterates.
//
// The loop iterator is the commonest object variable in a batch flow, and
// leaving it out made `delete $Order` inside `loop $Order in $Orders` emit no
// edge while `delete $Orders` did (side note on mendixlabs/mxcli#1266).
//
// A variable can be defined after the object that uses it in collection order
// (Objects is not flow order), so the walk repeats until nothing new is mapped;
// each pass only adds entries, so it ends within one pass per variable.
func buildVarEntityMap(params []*microflows.MicroflowParameter, oc *microflows.MicroflowObjectCollection, assocs map[string]assocEnds, probe func(string)) map[string]string {
	norm := func(v string) string { return strings.TrimPrefix(v, "$") }
	varEntity := map[string]string{}
	for _, p := range params {
		if qn := entityOfDataType(p.Type); qn != "" {
			varEntity[norm(p.Name)] = qn
		}
	}
	set := func(v, qn string) bool {
		v = norm(v)
		if v == "" || qn == "" || varEntity[v] != "" {
			return false
		}
		varEntity[v] = qn
		return true
	}
	var walk func(oc *microflows.MicroflowObjectCollection) bool
	walk = func(oc *microflows.MicroflowObjectCollection) bool {
		if oc == nil {
			return false
		}
		added := false
		for _, obj := range oc.Objects {
			switch o := obj.(type) {
			case *microflows.ActionActivity:
				switch a := o.Action.(type) {
				case *microflows.CreateObjectAction:
					added = set(a.OutputVariable, a.EntityQualifiedName) || added
				case *microflows.RetrieveAction:
					switch src := a.Source.(type) {
					case *microflows.DatabaseRetrieveSource:
						added = set(a.OutputVariable, src.EntityQualifiedName) || added
					case *microflows.AssociationRetrieveSource:
						if probe != nil {
							probe(src.AssociationQualifiedName)
						}
						added = set(a.OutputVariable, associationTarget(assocs[src.AssociationQualifiedName], varEntity[norm(src.StartVariable)])) || added
					}
				}
			case *microflows.LoopedActivity:
				if it, ok := o.LoopSource.(*microflows.IterableList); ok {
					added = set(it.VariableName, varEntity[norm(it.ListVariableName)]) || added
				}
				added = walk(o.ObjectCollection) || added
			}
		}
		return added
	}
	for walk(oc) {
	}
	return varEntity
}

// associationTarget is the entity an association retrieve from an object of
// entity start yields: the other end. It is "" when start is neither end — a
// specialization, or an unknown start variable — rather than a guess.
func associationTarget(ends assocEnds, start string) string {
	switch {
	case start == "" || ends.From == "" || ends.To == "":
		return ""
	case start == ends.From:
		return ends.To
	case start == ends.To:
		return ends.From
	}
	return ""
}

// associationEnds maps every association's qualified name to its endpoint
// entities, for resolving an association retrieve's output variable. An
// incremental build reads the associations buildAssociations wrote, which
// hold every module's while the domain models listed are only the rebuilt
// ones. There, an end that did not resolve to an entity is stored as its ID, which
// has no dot, and is "" here; a cross-module association's TO end is stored
// as the qualified name the model holds.
func (b *Builder) associationEnds() map[string]assocEnds {
	out := map[string]assocEnds{}
	if b.scope == nil {
		dms, err := b.cachedDomainModels()
		if err != nil {
			return out
		}
		for _, dm := range dms {
			moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(dm.ContainerID))
			for _, a := range dm.Associations {
				out[moduleName+"."+a.Name] = assocEnds{From: b.resolveEntityID(a.ParentID), To: b.resolveEntityID(a.ChildID)}
			}
			for _, ca := range dm.CrossAssociations {
				out[moduleName+"."+ca.Name] = assocEnds{From: b.resolveEntityID(ca.ParentID), To: ca.ChildRef}
			}
		}
		return out
	}
	rows, err := b.tx.Query(`SELECT QualifiedName, FromEntity, ToEntity FROM associations_data`)
	if err != nil {
		return out
	}
	defer rows.Close()
	end := func(name string) string {
		if !strings.Contains(name, ".") {
			return ""
		}
		return name
	}
	for rows.Next() {
		var qn, from, to string
		if rows.Scan(&qn, &from, &to) == nil {
			out[qn] = assocEnds{From: end(from), To: end(to)}
		}
	}
	return out
}

// buildReferences extracts cross-references from all documents.
// This is only run in full mode as it requires parsing all documents.
func (b *Builder) buildReferences() error {
	if !b.fullMode {
		return nil
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID := b.catalog.projectID
	snapshotID := b.snapshot.ID
	refCount := 0
	assocs := b.associationEnds()
	resolveDoc := b.agentEditorResolver()

	// emitActionRefs walks the action activities of a microflow or nanoflow and
	// records the document reference each action makes. Nanoflows share the same
	// action types as microflows, so both flow kinds use the same extraction.
	emitActionRefs := func(sourceType, sourceID string, containerID model.ID, name string, params []*microflows.MicroflowParameter, returnType microflows.DataType, oc *microflows.MicroflowObjectCollection) {
		moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(containerID))
		sourceQN := moduleName + "." + name
		emit := func(targetType, targetName, refKind string) {
			if _, err := stmt.Exec(sourceType, sourceID, sourceQN,
				targetType, "", targetName,
				refKind, moduleName, projectID, snapshotID); err == nil {
				refCount++
			}
		}

		// Boundary entity refs: the flow operates on its object-typed parameters
		// and return type even when it never create/retrieves them.
		for _, p := range params {
			if qn := entityOfDataType(p.Type); qn != "" {
				emit(RefObjectEntity, qn, RefKindParameter)
			}
		}
		if qn := entityOfDataType(returnType); qn != "" {
			emit(RefObjectEntity, qn, RefKindReturn)
		}

		if oc == nil {
			return
		}
		acts := collectActionActivities(oc)
		// Intra-flow variable→entity map so change/delete (which operate on a
		// variable, not a named entity) can resolve their target.
		varEntity := buildVarEntityMap(params, oc, assocs, b.probe(siteAssocEnds))
		for _, rule := range collectRuleCalls(oc) {
			emit(RefObjectRule, rule, RefKindCall)
		}
		for _, entity := range collectInheritanceCases(oc) {
			emit(RefObjectEntity, entity, RefKindSplit)
		}
		for _, act := range acts {
			if tt, tn, rk, ok := microflowActionRef(act.Action); ok {
				emit(tt, tn, rk)
			}
			if tt, tn, rk, ok := microflowVarActionRef(act.Action, varEntity); ok {
				emit(tt, tn, rk)
			}
			if qn, ok := microflowCommitRef(act.Action, varEntity); ok {
				emit(RefObjectEntity, qn, RefKindCommit)
			}
			for _, m := range microflowMappingRefs(act.Action) {
				emit(m.targetType, m.name, RefKindMappingCall)
			}
			for _, r := range codeActionParameterRefs(act.Action, resolveDoc) {
				emit(r.targetType, r.target, r.kind)
			}
		}
	}

	// Extract microflow references (using cached list — no re-parsing)
	mfs, err := b.cachedMicroflows()
	if err != nil {
		return err
	}
	for _, mf := range mfs {
		if !b.own(string(mf.ID)) {
			continue
		}
		emitActionRefs(RefObjectMicroflow, string(mf.ID), mf.ContainerID, mf.Name, mf.Parameters, mf.ReturnType, mf.ObjectCollection)
	}
	b.disown()

	// Extract nanoflow references — nanoflows also call microflows/nanoflows,
	// show pages, retrieve, etc.
	nfs, err := b.cachedNanoflows()
	if err == nil {
		for _, nf := range nfs {
			if !b.own(string(nf.ID)) {
				continue
			}
			emitActionRefs(RefObjectNanoflow, string(nf.ID), nf.ContainerID, nf.Name, nf.Parameters, nf.ReturnType, nf.ObjectCollection)
		}
		b.disown()
	}

	// Mapping → JSON structure: the structure an import or export mapping is
	// built on. Without the edge a JSON structure only mappings use had no
	// inbound reference and read as dead (ako/mxcli#1861). XML schemas and
	// message definitions are not catalogued as documents, so they get no edge.
	emitStructure := func(sourceType string, id model.ID, containerID model.ID, name, structure string) {
		if structure == "" || !b.own(string(id)) {
			return
		}
		moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(containerID))
		if _, err := stmt.Exec(sourceType, string(id), moduleName+"."+name,
			RefObjectJsonStructure, "", structure,
			RefKindStructure, moduleName, projectID, snapshotID); err == nil {
			refCount++
		}
	}
	if ims, err := b.lister().ListImportMappings(); err == nil {
		for _, im := range ims {
			emitStructure(RefObjectImportMapping, im.ID, im.ContainerID, im.Name, im.JsonStructure)
		}
		b.disown()
	}
	if ems, err := b.lister().ListExportMappings(); err == nil {
		for _, em := range ems {
			emitStructure(RefObjectExportMapping, em.ID, em.ContainerID, em.Name, em.JsonStructure)
		}
		b.disown()
	}

	// Extract rule references. A rule's body calls microflows, retrieves and
	// evaluates like any other flow, and without this walk every document a rule
	// calls is invisible to the reference graph — reported dead by
	// `show callers`, GRAPH_DEAD_ASSETS and lint rule QUAL004. Same shape as the
	// scheduled-event gap, one layer deeper.
	rules, err := b.cachedRules()
	if err == nil {
		for _, rule := range rules {
			if !b.own(string(rule.ID)) {
				continue
			}
			emitActionRefs(RefObjectRule, string(rule.ID), rule.ContainerID, rule.Name, rule.Parameters, rule.ReturnType, rule.ObjectCollection)
		}
		b.disown()
	}

	// Extract entity references (generalization) — using cached list
	dms, err := b.cachedDomainModels()
	if err == nil {
		for _, dm := range dms {
			if !b.own(string(dm.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(dm.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)

			for _, ent := range dm.Entities {
				sourceQN := moduleName + "." + ent.Name
				// Check generalization
				if ent.GeneralizationRef != "" {
					_, err = stmt.Exec(RefObjectEntity, string(ent.ID), sourceQN,
						RefObjectEntity, "", ent.GeneralizationRef,
						RefKindGeneralize, moduleName, projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
				// Calculated-by: an attribute whose value is computed by a microflow.
				for _, attr := range ent.Attributes {
					if attr.Value != nil && attr.Value.MicroflowName != "" {
						_, err = stmt.Exec(RefObjectEntity, string(ent.ID), sourceQN,
							RefObjectMicroflow, "", attr.Value.MicroflowName,
							RefKindCalculate, moduleName, projectID, snapshotID)
						if err == nil {
							refCount++
						}
					}
				}
			}

			// Association references: an association connects a FROM entity
			// (ParentID) and a TO entity (ChildID). Emit an `associate` ref to
			// each endpoint so the association is discoverable from either entity
			// via `show references of Entity` and impact analysis sees both sides.
			for _, assoc := range dm.Associations {
				assocQN := moduleName + "." + assoc.Name
				for _, target := range []string{b.resolveEntityID(assoc.ParentID), b.resolveEntityID(assoc.ChildID)} {
					if target == "" {
						continue
					}
					if _, err := stmt.Exec(RefObjectAssociation, string(assoc.ID), assocQN,
						RefObjectEntity, "", target,
						RefKindAssociate, moduleName, projectID, snapshotID); err == nil {
						refCount++
					}
				}
			}
			// Cross-module associations: ChildRef is already a qualified entity
			// name (the TO entity lives in another module); ParentID is local.
			for _, ca := range dm.CrossAssociations {
				assocQN := moduleName + "." + ca.Name
				for _, target := range []string{b.resolveEntityID(ca.ParentID), ca.ChildRef} {
					if target == "" {
						continue
					}
					if _, err := stmt.Exec(RefObjectAssociation, string(ca.ID), assocQN,
						RefObjectEntity, "", target,
						RefKindAssociate, moduleName, projectID, snapshotID); err == nil {
						refCount++
					}
				}
			}
		}
		b.disown()
	}

	// Extract page references (layout, datasources, parameters) — using cached list
	pageList, err := b.cachedPages()
	if err == nil {
		for _, pg := range pageList {
			if !b.own(string(pg.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(pg.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			sourceQN := moduleName + "." + pg.Name

			// Layout reference. ListPages() does not populate pg.LayoutCall (only
			// pg.LayoutID), so read the layout name the pages table already
			// resolved from raw BSON (extractLayoutRef) rather than the always-nil
			// LayoutCall. NOTE: widget-level datasource/action refs still require a
			// parsed widget tree, which no reader currently exposes — tracked as the
			// remaining part of #663 gap 3.
			if layoutRef := b.resolvePageLayoutRef(pg.ID); layoutRef != "" {
				_, err = stmt.Exec(RefObjectPage, string(pg.ID), sourceQN,
					RefObjectLayout, "", layoutRef,
					RefKindLayout, moduleName, projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}

			// Page parameter entity types
			for _, param := range pg.Parameters {
				if param.EntityName != "" {
					_, err = stmt.Exec(RefObjectPage, string(pg.ID), sourceQN,
						RefObjectEntity, "", param.EntityName,
						RefKindParameter, moduleName, projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
			}
		}
		b.disown()

		// Widget references: page → entity (datasource) and page → microflow /
		// nanoflow (action). Projected from the widgets table, which buildPages
		// already populated from each page's raw BSON (so no extra parse here).
		// DISTINCT collapses the many widgets on a page that target the same
		// document into a single edge.
		widgetProjections := []struct{ col, targetType, refKind string }{
			{"EntityRef", RefObjectEntity, RefKindDatasource},
			{"MicroflowRef", RefObjectMicroflow, RefKindAction},
			{"NanoflowRef", RefObjectNanoflow, RefKindAction},
			// A widget action that opens a page. Without this row, a page reachable
			// only from a button had no inbound reference and `show callers` /
			// `show references` reported it as unused (issue #773).
			{"PageRef", RefObjectPage, RefKindShowPage},
			// A snippet call: page, snippet or layout → the snippet it places
			// (ako/mxcli#2017).
			{"SnippetRef", RefObjectSnippet, RefKindSnippetCall},
		}
		for _, p := range widgetProjections {
			res, perr := b.tx.Exec(
				`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
				 SELECT DISTINCT ContainerType, '', ContainerQualifiedName, ?, '', `+p.col+`, ?, ModuleName, ?, ?
				 FROM widgets_data WHERE `+p.col+` != ''`,
				p.targetType, p.refKind, projectID, snapshotID)
			if perr == nil {
				if n, e := res.RowsAffected(); e == nil {
					refCount += int(n)
				}
			}
		}

		// The names a widget makes beyond its single column — the second
		// microflow of a data grid 2 with a datasource and an on-click. Without
		// them the datasource microflow had no inbound reference and QUAL004
		// said "not called from anywhere" (mendixlabs/mxcli#1513). NOT EXISTS
		// keeps the projection's one-edge-per-page shape when another widget's
		// column already made the edge.
		for _, r := range b.widgetExtraRefs {
			b.as(r.owner)
			res, xerr := b.tx.Exec(
				`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
				 SELECT ?, '', ?, ?, '', ?, ?, ?, ?, ?
				 WHERE NOT EXISTS (SELECT 1 FROM refs WHERE SourceType = ? AND SourceName = ? AND TargetType = ? AND TargetName = ? AND RefKind = ? AND SnapshotId = ?)`,
				r.containerType, r.containerQN, r.targetType, r.targetName, r.refKind, r.moduleName, projectID, snapshotID,
				r.containerType, r.containerQN, r.targetType, r.targetName, r.refKind, snapshotID)
			if xerr == nil {
				if n, e := res.RowsAffected(); e == nil {
					refCount += int(n)
				}
			}
		}
		b.disown()

		// Page/snippet -> widget definition. buildWidgetDefinitions runs before
		// this pass, so the join target is populated.
		if n, werr := insertWidgetRefs(b.tx, projectID, snapshotID); werr == nil {
			refCount += n
		}
	}

	// Extract navigation references (home pages, menu items, login pages)
	nav, err := b.reader.GetNavigation()
	if err == nil {
		for _, profile := range nav.Profiles {
			sourceName := "Navigation." + profile.Name

			// Default home page
			if profile.HomePage != nil {
				if profile.HomePage.Page != "" {
					_, err = stmt.Exec(RefObjectNavigation, "", sourceName,
						RefObjectPage, "", profile.HomePage.Page,
						RefKindHomePage, "", projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
				if profile.HomePage.Microflow != "" {
					_, err = stmt.Exec(RefObjectNavigation, "", sourceName,
						RefObjectMicroflow, "", profile.HomePage.Microflow,
						RefKindHomePage, "", projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
			}

			// Role-based home pages
			for _, rh := range profile.RoleBasedHomePages {
				if rh.Page != "" {
					_, err = stmt.Exec(RefObjectNavigation, "", sourceName,
						RefObjectPage, "", rh.Page,
						RefKindHomePage, "", projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
				if rh.Microflow != "" {
					_, err = stmt.Exec(RefObjectNavigation, "", sourceName,
						RefObjectMicroflow, "", rh.Microflow,
						RefKindHomePage, "", projectID, snapshotID)
					if err == nil {
						refCount++
					}
				}
			}

			// Login page
			if profile.LoginPage != "" {
				_, err = stmt.Exec(RefObjectNavigation, "", sourceName,
					RefObjectPage, "", profile.LoginPage,
					RefKindLoginPage, "", projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}

			// Menu items (recursive)
			refCount += b.extractMenuItemRefs(stmt, profile.MenuItems, sourceName, projectID, snapshotID)

			// Offline synchronization. Without this edge "which profiles sync
			// this entity?" is unanswerable, while the same question about a
			// page or a Java action is one query — and it is exactly the
			// question an offline change asks, because changing an entity that
			// a profile downloads changes what every device holds.
			//
			// Every sync mode gets an edge, including the ones that download
			// nothing (NEVER, NONE, ONLINE). The profile still NAMES the
			// entity, so renaming or dropping it leaves the config dangling —
			// which is the thing a reference edge exists to reveal. Emitting
			// only the downloading modes would make the quiet ones invisible
			// to exactly the query that would catch them.
			for _, oe := range profile.OfflineEntities {
				if oe.Entity == "" {
					continue
				}
				_, err = stmt.Exec(RefObjectNavigation, "", sourceName,
					RefObjectEntity, "", oe.Entity,
					RefKindSync, "", projectID, snapshotID)
				if err == nil {
					refCount++
				}
			}
		}
	}

	// Extract workflow references — using cached list
	wfs, wfErr := b.cachedWorkflows()
	if wfErr == nil {
		for _, wf := range wfs {
			if !b.own(string(wf.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(wf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			sourceQN := moduleName + "." + wf.Name

			// Every document the workflow names, from every flow it holds —
			// boundary-event paths and event sub-processes included.
			for _, r := range workflowDocRefs(wf) {
				if _, err := stmt.Exec(RefObjectWorkflow, string(wf.ID), sourceQN,
					r.TargetType, "", r.TargetName,
					r.RefKind, moduleName, projectID, snapshotID); err == nil {
					refCount++
				}
			}
		}
		b.disown()
	}

	// Scheduled events run a microflow. Without this edge the microflow looks
	// unreferenced: `show callers` reported none, GRAPH_DEAD_ASSETS listed it,
	// and QUAL004 said "is not called from anywhere" with the suggestion
	// "Remove if unused" — on a microflow that runs nightly in production.
	refCount += b.extractScheduledEventRefs(stmt, projectID, snapshotID)

	// Agent-editor documents call their tools and use their model, knowledge
	// bases and MCP services (ako/mxcli#2074).
	refCount += b.extractAgentEditorRefs(stmt, resolveDoc, projectID, snapshotID)

	// Attribute validation rules use a named regular expression, so
	// `show references to <regex>` can answer which entities depend on a shared
	// pattern.
	refCount += b.extractRegexRuleRefs(stmt, projectID, snapshotID)

	// Entity event handlers run a microflow on every create/commit/delete of
	// the entity. Same class as the scheduled-event edge above, and worse in
	// degree: a handler runs on every commit, so the false "dead" verdict lands
	// on code that is hotter than most of what IS reported as live.
	refCount += b.extractEventHandlerRefs(stmt, projectID, snapshotID)

	// Three project settings name a microflow the runtime calls. Same class as
	// the scheduled-event edge above and found the same way: a microflow wired as
	// AfterStartupMicroflow reported no callers and no references, QUAL004 said
	// "not called from anywhere. Remove if unused", and dropping it left a
	// dangling name that `mx check` did not catch either — it surfaced only when
	// the runtime refused to start (ako/CapTrackV4 049).
	refCount += b.extractProjectSettingsRefs(stmt, projectID, snapshotID)

	// A published REST operation runs a microflow the same way a scheduled event
	// does — the platform invokes it, so nothing in the model calls it. Without
	// this edge every microflow behind the public API reads as dead: on the
	// reported model, 92 of 93 operations name a microflow and all 92 were listed
	// by GRAPH_DEAD_ASSETS, whose advice is to delete them (#1126).
	refCount += b.extractPublishedRestRefs(stmt, projectID, snapshotID)

	// The other microflows an integration service names: a published OData
	// resource's read/change microflow and actions, a published service's
	// authentication microflow, a business event handler, a consumed OData
	// service's configuration microflows. Same class again — the platform runs
	// them — and found the same way: 34 Read_ microflows and the authentication
	// microflow of six published OData services read as dead (ako/mxcli#1639).
	refCount += b.extractServiceMicroflowRefs(stmt, projectID, snapshotID)

	// Members, enumerations and mapped entities. Without these the graph ended
	// at documents, so `impact` on an attribute, an enumeration or a value said
	// "not referenced" however much it was used — the answer an agent reads as
	// "safe to delete". See builder_member_refs.go.
	idx := b.loadMemberRefIndex()
	refCount += b.extractMemberRefs(stmt, idx, projectID, snapshotID)
	refCount += b.extractXPathRefs(stmt, idx, projectID, snapshotID)
	refCount += b.extractEnumerationTypeRefs(projectID, snapshotID)

	b.report("References", refCount)
	return nil
}

// extractScheduledEventRefs emits one `schedule` edge per scheduled event, from
// the event to the microflow it runs.
//
// The edges are collected by buildScheduledEvents, which runs earlier in the
// same transaction.
// extractRegexRuleRefs emits one `validate` edge per attribute validation rule
// that uses a regular expression, from the entity to the regex document.
func (b *Builder) extractRegexRuleRefs(stmt *Stmt, projectID, snapshotID string) int {
	defer b.disown()
	count := 0
	for _, r := range b.regexRuleRefs {
		b.as(r.owner)
		if _, err := stmt.Exec(
			RefObjectEntity, "", r.entityQualifiedName,
			RefObjectRegularExpression, "", r.regexQualifiedName,
			RefKindValidate, r.moduleName, projectID, snapshotID,
		); err == nil {
			count++
		}
	}
	return count
}

func (b *Builder) extractScheduledEventRefs(stmt *Stmt, projectID, snapshotID string) int {
	defer b.disown()
	count := 0
	for _, r := range b.scheduledEventRefs {
		b.as(r.owner)
		if _, err := stmt.Exec(
			RefObjectScheduledEvent, "", r.qualifiedName,
			RefObjectMicroflow, "", r.microflow,
			RefKindSchedule, r.moduleName, projectID, snapshotID,
		); err == nil {
			count++
		}
	}
	return count
}

// extractPublishedRestRefs emits one `publish` edge per published REST operation
// that names a microflow, from the operation to the microflow it runs.
//
// The source is the OPERATION, not the service, because that is the granularity
// the question is asked at: `show references to <microflow>` should name the one
// endpoint that reaches it, not the service holding thirty of them. The
// operation's catalog id travels with the edge so the path and summary are one
// join away.
//
// The edges are collected by buildPublishedRestServices, which runs earlier in
// the same transaction.
func (b *Builder) extractPublishedRestRefs(stmt *Stmt, projectID, snapshotID string) int {
	defer b.disown()
	count := 0
	for _, r := range b.publishedRestRefs {
		b.as(r.owner)
		if _, err := stmt.Exec(
			RefObjectPublishedRestOperation, r.sourceID, r.qualifiedName,
			RefObjectMicroflow, "", r.microflow,
			RefKindPublish, r.moduleName, projectID, snapshotID,
		); err == nil {
			count++
		}
	}
	return count
}

// extractEventHandlerRefs emits one `event` edge per entity event handler, from
// the entity to the microflow it runs.
//
// The edge carries neither the moment nor the event — refs has no column for
// them, and inventing a kind per combination ("before_commit", "after_delete")
// would put eight kinds into every consumer's list to say one thing. Which
// moment and which event is CATALOG.ENTITY_EVENT_HANDLERS' question; this edge
// answers "is the microflow reachable", which is the one three tools were
// getting wrong.
//
// The edges are collected by buildEntityEventHandlers, which runs earlier in
// the same transaction.
func (b *Builder) extractEventHandlerRefs(stmt *Stmt, projectID, snapshotID string) int {
	defer b.disown()
	count := 0
	for _, r := range b.eventHandlerRefs {
		b.as(r.owner)
		if _, err := stmt.Exec(
			RefObjectEntity, "", r.entityQualifiedName,
			RefObjectMicroflow, "", r.microflow,
			RefKindEvent, r.moduleName, projectID, snapshotID,
		); err == nil {
			count++
		}
	}
	return count
}

// extractMenuItemRefs extracts page and microflow references from menu items recursively.
func (b *Builder) extractMenuItemRefs(stmt *Stmt, items []*types.NavMenuItem, sourceName, projectID, snapshotID string) int {
	refCount := 0
	for _, item := range items {
		if item.Page != "" {
			_, err := stmt.Exec(RefObjectNavigation, "", sourceName,
				RefObjectPage, "", item.Page,
				RefKindMenuItem, "", projectID, snapshotID)
			if err == nil {
				refCount++
			}
		}
		if item.Microflow != "" {
			_, err := stmt.Exec(RefObjectNavigation, "", sourceName,
				RefObjectMicroflow, "", item.Microflow,
				RefKindMenuItem, "", projectID, snapshotID)
			if err == nil {
				refCount++
			}
		}
		if len(item.Items) > 0 {
			refCount += b.extractMenuItemRefs(stmt, item.Items, sourceName, projectID, snapshotID)
		}
	}
	return refCount
}

// extractWidgetRefs extracts entity and microflow references from a widget and its children.
func (b *Builder) extractWidgetRefs(stmt *Stmt, w pages.Widget, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID string) int {
	if w == nil {
		return 0
	}

	refCount := 0

	// Extract datasource refs based on widget type
	switch widget := w.(type) {
	case *pages.DataView:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		// Recurse into children
		for _, child := range widget.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
		for _, child := range widget.FooterWidgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.ListView:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		for _, child := range widget.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.DataGrid:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		for _, child := range widget.ControlBarWidgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.TemplateGrid:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		for _, child := range widget.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
		for _, child := range widget.ControlBarWidgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.Gallery:
		refCount += b.extractDataSourceRefs(stmt, widget.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		if widget.ContentWidget != nil {
			refCount += b.extractWidgetRefs(stmt, widget.ContentWidget, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
		for _, child := range widget.FilterWidgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.Container:
		for _, child := range widget.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

	case *pages.LayoutGrid:
		for _, row := range widget.Rows {
			for _, col := range row.Columns {
				for _, child := range col.Widgets {
					refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
				}
			}
		}

	case *pages.CustomWidget:
		// Pluggable widget - extract refs from WidgetObject properties
		if widget.WidgetObject != nil {
			refCount += b.extractWidgetObjectRefs(stmt, widget.WidgetObject, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
	}

	return refCount
}

// extractWidgetObjectRefs extracts refs from a pluggable widget's WidgetObject.
func (b *Builder) extractWidgetObjectRefs(stmt *Stmt, obj *pages.WidgetObject, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID string) int {
	if obj == nil {
		return 0
	}

	refCount := 0

	for _, prop := range obj.Properties {
		if prop.Value == nil {
			continue
		}
		val := prop.Value

		// Extract datasource refs
		if val.DataSource != nil {
			refCount += b.extractDataSourceRefs(stmt, val.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

		// Extract entity ref
		if val.EntityRef != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				RefObjectEntity, "", val.EntityRef,
				RefKindDatasource, moduleName, projectID, snapshotID)
			refCount++
		}

		// Extract microflow ref
		if val.Microflow != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				RefObjectMicroflow, "", val.Microflow,
				RefKindAction, moduleName, projectID, snapshotID)
			refCount++
		}

		// Extract nanoflow ref
		if val.Nanoflow != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				RefObjectNanoflow, "", val.Nanoflow,
				RefKindAction, moduleName, projectID, snapshotID)
			refCount++
		}

		// Extract form (page) ref
		if val.Form != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				RefObjectPage, "", val.Form,
				RefKindShowPage, moduleName, projectID, snapshotID)
			refCount++
		}

		// Recurse into nested widgets
		for _, child := range val.Widgets {
			refCount += b.extractWidgetRefs(stmt, child, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}

		// Recurse into nested objects
		for _, childObj := range val.Objects {
			refCount += b.extractWidgetObjectRefs(stmt, childObj, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID)
		}
	}

	return refCount
}

// extractDataSourceRefs extracts entity and microflow references from a datasource.
func (b *Builder) extractDataSourceRefs(stmt *Stmt, ds pages.DataSource, sourceType, sourceID, sourceQN, moduleName, projectID, snapshotID string) int {
	if ds == nil {
		return 0
	}

	refCount := 0

	switch src := ds.(type) {
	case *pages.DatabaseSource:
		// Resolve entity ID to qualified name
		if src.EntityID != "" {
			entityQN := b.resolveEntityID(src.EntityID)
			if entityQN != "" {
				stmt.Exec(sourceType, sourceID, sourceQN,
					RefObjectEntity, string(src.EntityID), entityQN,
					RefKindDatasource, moduleName, projectID, snapshotID)
				refCount++
			}
		}

	case *pages.DataViewSource:
		// Has either EntityID or EntityName
		entityQN := ""
		if src.EntityName != "" {
			entityQN = src.EntityName
		} else if src.EntityID != "" {
			entityQN = b.resolveEntityID(src.EntityID)
		}
		if entityQN != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				RefObjectEntity, "", entityQN,
				RefKindDatasource, moduleName, projectID, snapshotID)
			refCount++
		}

	case *pages.EntityPathSource:
		// Parse entity path to get root entity (e.g., "Customer/Orders" -> get entity for Customer)
		if src.EntityPath != "" {
			// Entity path starts with entity name or is an association path
			// For now, we'll extract what we can - the first segment might be the entity
			parts := strings.Split(src.EntityPath, "/")
			if len(parts) > 0 && parts[0] != "" {
				// This might be a qualified name or just entity name
				// We store it as-is for now
				stmt.Exec(sourceType, sourceID, sourceQN,
					RefObjectEntity, "", src.EntityPath,
					RefKindDatasource, moduleName, projectID, snapshotID)
				refCount++
			}
		}

	case *pages.AssociationSource:
		// Similar to EntityPathSource
		if src.EntityPath != "" {
			stmt.Exec(sourceType, sourceID, sourceQN,
				RefObjectEntity, "", src.EntityPath,
				RefKindDatasource, moduleName, projectID, snapshotID)
			refCount++
		}

	case *pages.MicroflowSource:
		// Resolve microflow ID to qualified name
		if src.MicroflowID != "" {
			mfQN := b.resolveMicroflowID(src.MicroflowID)
			if mfQN != "" {
				stmt.Exec(sourceType, sourceID, sourceQN,
					RefObjectMicroflow, string(src.MicroflowID), mfQN,
					RefKindDatasource, moduleName, projectID, snapshotID)
				refCount++
			}
		}

	case *pages.NanoflowSource:
		// Resolve nanoflow ID to qualified name
		if src.NanoflowID != "" {
			nfQN := b.resolveMicroflowID(src.NanoflowID) // Uses same table
			if nfQN != "" {
				stmt.Exec(sourceType, sourceID, sourceQN,
					RefObjectNanoflow, string(src.NanoflowID), nfQN,
					RefKindDatasource, moduleName, projectID, snapshotID)
				refCount++
			}
		}
	}

	return refCount
}

// resolveEntityID looks up the qualified name for an entity ID.
func (b *Builder) resolveEntityID(entityID model.ID) string {
	if entityID == "" {
		return ""
	}
	var qualifiedName string
	err := b.tx.QueryRow("SELECT QualifiedName FROM entities WHERE Id = ?", string(entityID)).Scan(&qualifiedName)
	if err != nil {
		return ""
	}
	b.dependsOn(siteEntityByID, qualifiedName)
	return qualifiedName
}

// resolvePageLayoutRef returns the layout qualified name the pages table already
// resolved for a page (from raw-BSON FormCall via extractLayoutRef), or "" if the
// page has none. buildReferences runs after buildPages in full mode, so the value
// is available; this avoids re-parsing the page BSON and reuses the pages table's
// resolution instead of the always-nil pg.LayoutCall.
func (b *Builder) resolvePageLayoutRef(pageID model.ID) string {
	var layoutRef string
	err := b.tx.QueryRow("SELECT LayoutRef FROM pages WHERE Id = ?", string(pageID)).Scan(&layoutRef)
	if err != nil {
		return ""
	}
	return layoutRef
}

// resolveMicroflowID looks up the qualified name for a microflow/nanoflow ID.
func (b *Builder) resolveMicroflowID(mfID model.ID) string {
	if mfID == "" {
		return ""
	}
	var qualifiedName string
	err := b.tx.QueryRow("SELECT QualifiedName FROM microflows WHERE Id = ?", string(mfID)).Scan(&qualifiedName)
	if err != nil {
		return ""
	}
	b.dependsOn(siteFlowByID, qualifiedName)
	return qualifiedName
}

// projectSettingsMicroflowRefs lists the project settings whose value is the
// qualified name of a microflow the RUNTIME calls, with the key each is stored
// under so a reference reads as the setting that made it.
//
// It is a literal list rather than reflection over ProjectSettings because most
// of that struct is strings that are not microflow names, and a wrong entry here
// would invent an edge rather than miss one.
var projectSettingsMicroflowRefs = []struct {
	setting string
	value   func(*model.ModelSettings) string
}{
	{"AfterStartupMicroflow", func(ms *model.ModelSettings) string { return ms.AfterStartupMicroflow }},
	{"BeforeShutdownMicroflow", func(ms *model.ModelSettings) string { return ms.BeforeShutdownMicroflow }},
	{"HealthCheckMicroflow", func(ms *model.ModelSettings) string { return ms.HealthCheckMicroflow }},
}

// extractProjectSettingsRefs emits one `settings` edge per project setting that
// names a microflow, from the setting to the microflow it runs.
//
// The source is the SETTING, not the project, so `show references to <microflow>`
// names which setting depends on it — which is the thing you need before dropping
// it, and what `describe settings` would otherwise be the only way to learn.
func (b *Builder) extractProjectSettingsRefs(stmt *Stmt, projectID, snapshotID string) int {
	ps, err := b.reader.GetProjectSettings()
	if err != nil || ps == nil || ps.Model == nil {
		return 0 // a project may have no settings document; not an error
	}
	count := 0
	for _, s := range projectSettingsMicroflowRefs {
		target := strings.TrimSpace(s.value(ps.Model))
		if target == "" {
			continue
		}
		// The module is the microflow's own — a project setting belongs to no
		// module, and leaving it blank would drop the edge out of any per-module
		// view of the graph.
		moduleName := ""
		if i := strings.Index(target, "."); i > 0 {
			moduleName = target[:i]
		}
		if _, err := stmt.Exec(
			RefObjectProjectSettings, "", s.setting,
			RefObjectMicroflow, "", target,
			RefKindSettings, moduleName, projectID, snapshotID,
		); err == nil {
			count++
		}
	}
	return count
}
