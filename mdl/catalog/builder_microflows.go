// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Values of microflows.MicroflowType — which of the three flow flavours sharing
// microflows_data a row is. Upper-case, like every other catalog type
// vocabulary (refs.SourceType, objects.ObjectType).
//
// Named because readers outside this package filter on them: `show structure`
// compared against 'microflow' / 'nanoflow' for as long as the column has held
// upper case, matched nothing, and so never showed a flow count. SQLite's `=`
// is case-sensitive; a literal that disagrees with the writer fails silently
// as an empty result, never as an error. Filter on these, not on a literal.
const (
	MicroflowTypeMicroflow = "MICROFLOW"
	MicroflowTypeNanoflow  = "NANOFLOW"
	MicroflowTypeRule      = "RULE"
)

func (b *Builder) buildMicroflows() error {
	// Get all microflows (cached — avoids re-parsing in later phases)
	mfs, err := b.cachedMicroflows()
	if err != nil {
		return err
	}

	// Get all nanoflows (cached)
	nfs, err := b.cachedNanoflows()
	if err != nil {
		return err
	}

	// Get all rules (cached). A rule is a distinct doctype and lands in
	// microflows_data with MicroflowType "RULE", the way a nanoflow does —
	// without a row here a rule is not an object at all, so `show callers` can
	// never resolve it and GRAPH_DEAD_ASSETS cannot see it.
	rules, err := b.cachedRules()
	if err != nil {
		return err
	}

	mfStmt, err := b.tx.Prepare(`
		INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, Folder, MicroflowType,
			Description, ReturnType, ParameterCount, ActivityCount, TotalActivityCount,
			Complexity, Excluded, ApplyEntityAccess, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer mfStmt.Close()

	paramStmt, err := b.tx.Prepare(`
		INSERT INTO microflow_parameters_data (Id, MicroflowId, MicroflowQualifiedName,
			ModuleName, Name, ParameterType, Description, Ordinal, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer paramStmt.Close()

	// Prepare activity statement only in full mode
	var actStmt *Stmt
	if b.fullMode {
		actStmt, err = b.tx.Prepare(`
			INSERT INTO activities_data (Id, Name, Caption, ActivityType, Sequence, MicroflowId, MicroflowQualifiedName,
				ModuleName, Folder, EntityRef, ActionType, ServiceRef, ActionRef,
				UseRequestTimeout, TimeoutExpression, Description,
				ParentLoopId, LoopDepth,
				AutoGenerateCaption, ConditionExpression, ConditionRule, ErrorHandlingType,
				LogLevel, LogNodeExpression, LogMessage, CommitType, WithEvents, RetrieveSource,
				QueueRef, ProjectId, SnapshotId)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
				?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer actStmt.Close()
	}

	// A delete names a variable, not an entity; its EntityRef is resolved the
	// way refs_from()'s delete edge is, from the flow's own variables.
	var assocs map[string]assocEnds
	if b.fullMode {
		assocs = b.associationEnds()
	}

	projectID, snapshotID := b.snapshotMeta()

	mfCount := 0
	nfCount := 0
	ruleCount := 0
	paramCount := 0

	// insertParams writes one row per parameter. Microflows and nanoflows share
	// it: both carry []*MicroflowParameter and both land in microflows_data, so
	// splitting them here would only invite the two paths to drift.
	insertParams := func(flowID, qualifiedName, moduleName string, params []*microflows.MicroflowParameter) error {
		for i, prm := range params {
			if prm == nil {
				continue
			}
			id := string(prm.ID)
			if id == "" {
				id = flowID + "/" + prm.Name
			}
			if _, err := paramStmt.Exec(
				id,
				flowID,
				qualifiedName,
				moduleName,
				prm.Name,
				getDataTypeName(prm.Type),
				prm.Documentation,
				i,
				projectID, snapshotID,
			); err != nil {
				return err
			}
			paramCount++
		}
		return nil
	}
	actCount := 0

	// Process microflows
	for _, mf := range mfs {
		if !b.own(string(mf.ID)) {
			continue
		}
		// Get module name
		moduleID := b.hierarchy.findModuleID(mf.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + mf.Name

		// ListMicroflows() already returns fully-parsed objects — no need to call GetMicroflow()
		returnType := ""
		if mf.ReturnType != nil {
			returnType = getDataTypeName(mf.ReturnType)
		}

		// Count activities (excluding structural elements like Start/End events)
		activityCount := countFlowActivities(mf.ObjectCollection, false)

		// Calculate McCabe cyclomatic complexity
		complexity := calculateMcCabeComplexity(mf)

		_, err = mfStmt.Exec(
			string(mf.ID),
			mf.Name,
			qualifiedName,
			moduleName,
			b.hierarchy.buildFolderPath(mf.ContainerID), // real folder path (Bug 12b class)
			MicroflowTypeMicroflow,
			mf.Documentation,
			returnType,
			len(mf.Parameters),
			activityCount,
			countFlowActivities(mf.ObjectCollection, true),
			complexity,
			mf.Excluded,
			mf.ApplyEntityAccess,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		if err := insertParams(string(mf.ID), qualifiedName, moduleName, mf.Parameters); err != nil {
			return err
		}
		mfCount++

		// Insert activities only in full mode
		if b.fullMode {
			n, err := insertFlowActivities(actStmt, string(mf.ID), qualifiedName, moduleName, mf.ObjectCollection,
				buildVarEntityMap(mf.Parameters, mf.ObjectCollection, assocs, b.probe(siteAssocEnds)), projectID, snapshotID)
			if err != nil {
				return err
			}
			actCount += n
		}
	}
	b.disown()

	// Process nanoflows
	for _, nf := range nfs {
		if !b.own(string(nf.ID)) {
			continue
		}
		// Get module name
		moduleID := b.hierarchy.findModuleID(nf.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + nf.Name

		returnType := ""
		if nf.ReturnType != nil {
			returnType = getDataTypeName(nf.ReturnType)
		}

		// Count activities (excluding structural elements like Start/End events)
		activityCount := countFlowActivities(nf.ObjectCollection, false)

		// Calculate McCabe cyclomatic complexity
		complexity := calculateNanoflowComplexity(nf)

		_, err = mfStmt.Exec(
			string(nf.ID),
			nf.Name,
			qualifiedName,
			moduleName,
			b.hierarchy.buildFolderPath(nf.ContainerID), // real folder path (Bug 12b class)
			MicroflowTypeNanoflow,
			nf.Documentation,
			returnType,
			len(nf.Parameters),
			activityCount,
			countFlowActivities(nf.ObjectCollection, true),
			complexity,
			nf.Excluded,
			false, // a nanoflow runs client-side: entity access always applies, there is no setting
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		if err := insertParams(string(nf.ID), qualifiedName, moduleName, nf.Parameters); err != nil {
			return err
		}
		nfCount++

		// Insert activities only in full mode
		if b.fullMode {
			n, err := insertFlowActivities(actStmt, string(nf.ID), qualifiedName, moduleName, nf.ObjectCollection,
				buildVarEntityMap(nf.Parameters, nf.ObjectCollection, assocs, b.probe(siteAssocEnds)), projectID, snapshotID)
			if err != nil {
				return err
			}
			actCount += n
		}
	}
	b.disown()

	// Process rules. Their bodies are walked for activities in full mode like
	// any other flow; the reference edges out of those bodies are emitted by
	// builder_references.go, and the two must land together — a rule that is an
	// object but whose body is not walked reports every document it calls as
	// dead, which is worse than not knowing about rules at all.
	for _, rule := range rules {
		if !b.own(string(rule.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(rule.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + rule.Name

		returnType := ""
		if rule.ReturnType != nil {
			returnType = getDataTypeName(rule.ReturnType)
		}

		_, err = mfStmt.Exec(
			string(rule.ID),
			rule.Name,
			qualifiedName,
			moduleName,
			b.hierarchy.buildFolderPath(rule.ContainerID),
			MicroflowTypeRule,
			rule.Documentation,
			returnType,
			len(rule.Parameters),
			countFlowActivities(rule.ObjectCollection, false),
			countFlowActivities(rule.ObjectCollection, true),
			calculateRuleComplexity(rule),
			rule.Excluded,
			rule.ApplyEntityAccess,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		if err := insertParams(string(rule.ID), qualifiedName, moduleName, rule.Parameters); err != nil {
			return err
		}
		ruleCount++

		if b.fullMode {
			n, err := insertFlowActivities(actStmt, string(rule.ID), qualifiedName, moduleName, rule.ObjectCollection,
				buildVarEntityMap(rule.Parameters, rule.ObjectCollection, assocs, b.probe(siteAssocEnds)), projectID, snapshotID)
			if err != nil {
				return err
			}
			actCount += n
		}
	}
	b.disown()

	b.report("Microflows", mfCount)
	b.report("Nanoflows", nfCount)
	b.report("Rules", ruleCount)
	b.report("Flow parameters", paramCount)
	if b.fullMode {
		b.report("Activities", actCount)
	}
	return nil
}

// getMicroflowObjectType returns the type name for a microflow object.
func getMicroflowObjectType(obj microflows.MicroflowObject) string {
	switch obj.(type) {
	case *microflows.ActionActivity:
		return "ActionActivity"
	case *microflows.StartEvent:
		return "StartEvent"
	case *microflows.EndEvent:
		return "EndEvent"
	case *microflows.ExclusiveSplit:
		return "ExclusiveSplit"
	case *microflows.InheritanceSplit:
		return "InheritanceSplit"
	case *microflows.ExclusiveMerge:
		return "ExclusiveMerge"
	case *microflows.LoopedActivity:
		return "LoopedActivity"
	case *microflows.Annotation:
		return "Annotation"
	case *microflows.BreakEvent:
		return "BreakEvent"
	case *microflows.ContinueEvent:
		return "ContinueEvent"
	case *microflows.ErrorEvent:
		return "ErrorEvent"
	default:
		return "MicroflowObject"
	}
}

// getMicroflowActionType returns the catalog ActionType label for a microflow
// action. Every modelled action's Go type name is exactly its Mendix action type
// (CreateObjectAction, RestCallAction, ShowPageAction, …), so the label is derived
// directly from the concrete type rather than a hand-maintained switch. A parallel
// switch silently buckets anything it forgets under a generic "MicroflowAction" —
// which is how REST, web-service, nanoflow-call, XML import/export, JavaScript
// action, execute-database-query, transform-json and show-home-page actions all
// went unlabelled. Deriving the name keeps the table correct for every action
// the parser models, including ones added later.
func getMicroflowActionType(action microflows.MicroflowAction) string {
	switch a := action.(type) {
	case nil:
		return "MicroflowAction"
	case *microflows.UnknownAction:
		// An action type the parser does not model yet — surface its real Mendix
		// storage name (e.g. "SomeAction") instead of a generic label.
		if a.TypeName != "" {
			return strings.TrimPrefix(a.TypeName, "Microflows$")
		}
		return "UnknownAction"
	case *microflows.UnsupportedAction:
		// A stored action the reader recognises but does not model (e.g.
		// Microflows$GenerateJumpToOptionsAction): label it by its storage type,
		// not by the placeholder Go type, which no rule could ever ask for.
		if a.StorageType != "" {
			return strings.TrimPrefix(a.StorageType, "Microflows$")
		}
		return "UnsupportedAction"
	default:
		return strings.TrimPrefix(fmt.Sprintf("%T", action), "*microflows.")
	}
}

// getDataTypeName returns a string representation of a data type.
func getDataTypeName(dt microflows.DataType) string {
	if dt == nil {
		return ""
	}
	switch t := dt.(type) {
	case *microflows.BooleanType:
		return "Boolean"
	case *microflows.IntegerType:
		return "Integer"
	case *microflows.LongType:
		return "Long"
	case *microflows.DecimalType:
		return "Decimal"
	case *microflows.StringType:
		return "String"
	case *microflows.DateTimeType:
		return "DateTime"
	case *microflows.DateType:
		return "Date"
	case *microflows.ObjectType:
		return "Object:" + t.EntityQualifiedName
	case *microflows.ListType:
		return "List:" + t.EntityQualifiedName
	case *microflows.EnumerationType:
		return "Enumeration:" + t.EnumerationQualifiedName
	case *microflows.VoidType:
		return "Void"
	default:
		return "Unknown"
	}
}

// countMicroflowActivities counts the top-level activities of a microflow,
// excluding structural elements — the ActivityCount column.
func countMicroflowActivities(mf *microflows.Microflow) int {
	return countFlowActivities(mf.ObjectCollection, false)
}

// calculateMcCabeComplexity calculates the McCabe cyclomatic complexity of a microflow.
// McCabe complexity = 1 + number of decision points (IF, LOOP, error handlers)
// A higher complexity indicates more paths through the code and higher testing burden.
// Typical thresholds: 1-10 (simple), 11-20 (moderate), 21-50 (complex), 50+ (untestable)
func calculateMcCabeComplexity(mf *microflows.Microflow) int {
	// Base complexity is 1 (the main path through the microflow)
	complexity := 1

	if mf.ObjectCollection == nil {
		return complexity
	}

	// Count decision points in the main flow
	complexity += countDecisionPoints(mf.ObjectCollection.Objects)

	return complexity
}

// countDecisionPoints counts decision points in a list of microflow objects.
// This recursively processes nested structures like LoopedActivity.
func countDecisionPoints(objects []microflows.MicroflowObject) int {
	count := 0

	for _, obj := range objects {
		switch activity := obj.(type) {
		case *microflows.ExclusiveSplit:
			// Each IF/decision adds 1 to complexity
			count++

		case *microflows.InheritanceSplit:
			// Type check split adds 1 to complexity
			count++

		case *microflows.LoopedActivity:
			// Each loop adds 1 to complexity
			count++
			// Also count decision points inside the loop body
			if activity.ObjectCollection != nil {
				count += countDecisionPoints(activity.ObjectCollection.Objects)
			}

		case *microflows.ErrorEvent:
			// Error handling path adds complexity
			count++
		}
	}

	return count
}

// countNanoflowActivities counts the top-level activities of a nanoflow.
func countNanoflowActivities(nf *microflows.Nanoflow) int {
	return countFlowActivities(nf.ObjectCollection, false)
}

// calculateNanoflowComplexity calculates the McCabe cyclomatic complexity of a nanoflow.
func calculateNanoflowComplexity(nf *microflows.Nanoflow) int {
	complexity := 1

	if nf.ObjectCollection == nil {
		return complexity
	}

	complexity += countDecisionPoints(nf.ObjectCollection.Objects)
	return complexity
}

// countFlowActivities counts the activities of a flow body, excluding the
// structural elements: start and end events and merges. Microflows, nanoflows
// and rules share it, so the three cannot drift apart.
//
// nested=false counts the top level only — ActivityCount, whose meaning is
// unchanged since bundled rules (the microflow-size checks) are calibrated on
// it; a loop counts as one activity. nested=true also counts every loop body,
// at any depth — TotalActivityCount (mendixlabs/mxcli#1266).
func countFlowActivities(oc *microflows.MicroflowObjectCollection, nested bool) int {
	if oc == nil {
		return 0
	}
	count := 0
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.StartEvent, *microflows.EndEvent, *microflows.ExclusiveMerge:
			// Structural, not activities.
		case *microflows.LoopedActivity:
			count++
			if nested {
				count += countFlowActivities(o.ObjectCollection, true)
			}
		default:
			count++
		}
	}
	return count
}

// calculateRuleComplexity calculates McCabe cyclomatic complexity for a rule.
func calculateRuleComplexity(rule *microflows.Rule) int {
	if rule.ObjectCollection == nil {
		return 1
	}
	complexity := 1
	for _, obj := range rule.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.ExclusiveSplit, *microflows.InheritanceSplit, *microflows.LoopedActivity:
			complexity++
		}
	}
	return complexity
}

// insertFlowActivities writes one activities_data row per object of a flow
// body, loop bodies included, and returns how many it wrote. Microflows,
// nanoflows and rules share it: the three near-copies it replaces each walked
// the top level only, so nothing inside a loop — at any depth — was catalogued
// (mendixlabs/mxcli#1266), although the reader loads the loop body.
//
// Sequence is one pre-order number across the whole flow: a loop, then its
// body, then the loop's next sibling. ParentLoopId is the enclosing loop's Id
// (empty at the top level) and LoopDepth how many loops enclose the object (0
// at the top level), so filtering on an empty ParentLoopId gives the rows the
// table held before #1266.
//
// varEntity is the flow's variable→entity map (buildVarEntityMap), which a
// delete's EntityRef is resolved through.
func insertFlowActivities(stmt *Stmt, flowID, qualifiedName, moduleName string,
	oc *microflows.MicroflowObjectCollection, varEntity map[string]string, projectID, snapshotID string) (int, error) {
	if stmt == nil || oc == nil {
		return 0, nil
	}
	seq := 0
	var walk func(objs []microflows.MicroflowObject, parentLoopID string, depth int) error
	walk = func(objs []microflows.MicroflowObject, parentLoopID string, depth int) error {
		for _, obj := range objs {
			if obj == nil {
				continue
			}
			seq++
			r := describeFlowObject(obj, varEntity)
			if _, err := stmt.Exec(
				string(obj.GetID()),
				r.name,
				r.caption,
				r.activityType,
				seq,
				flowID,
				qualifiedName,
				moduleName,
				moduleName,
				r.entityRef,
				r.actionType,
				r.serviceRef,
				r.actionRef,
				boolInt(r.useRequestTimeout),
				r.timeoutExpression,
				r.description,
				parentLoopID,
				depth,
				boolInt(r.autoGenerateCaption),
				r.conditionExpression,
				r.conditionRule,
				r.errorHandlingType,
				r.logLevel,
				r.logNodeExpression,
				r.logMessage,
				r.commitType,
				boolInt(r.withEvents),
				r.retrieveSource,
				r.queueRef,
				projectID, snapshotID,
			); err != nil {
				return err
			}
			if loop, ok := obj.(*microflows.LoopedActivity); ok && loop.ObjectCollection != nil {
				if err := walk(loop.ObjectCollection.Objects, string(loop.ID), depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(oc.Objects, "", 0); err != nil {
		return seq, err
	}
	return seq, nil
}

// Values of activities_data.RetrieveSource — where a retrieve reads from.
// Lower-case, as mendixlabs/mxcli#1267 agreed. Named because a rule filters on
// them: a literal that disagrees with the writer matches nothing, silently.
const (
	RetrieveSourceDatabase    = "database"
	RetrieveSourceAssociation = "association"
)

// flowObjectRow is what one flow object contributes to its activities_data row.
//
// Every value is the stored one, in Mendix's own spelling: ErrorHandlingType
// is Rollback / Custom / CustomWithoutRollBack (capital B) / Continue / Abort,
// LogLevel Trace … Critical, CommitType Yes / YesWithoutEvents / No. A rule
// compares against these, so translating them here would only add a second
// vocabulary to keep in step.
type flowObjectRow struct {
	name, caption, activityType, actionType string
	description                             string
	autoGenerateCaption                     bool
	entityRef, serviceRef, actionRef        string
	useRequestTimeout                       bool
	timeoutExpression                       string
	// conditionExpression is an exclusive split's expression; conditionRule
	// the rule a rule-based split calls (its condition is a call, not text).
	conditionExpression, conditionRule string
	errorHandlingType                  string
	logLevel, logNodeExpression        string
	logMessage                         string
	commitType                         string
	withEvents                         bool
	retrieveSource                     string
	// queueRef is the task queue a microflow or Java action call runs in —
	// asynchronously, outside the caller's transaction and loop.
	queueRef string
}

// describeFlowObject derives the catalog columns of one flow object.
//
// The caption is the stored one — a split's, an activity's, an annotation's
// text. It was the placeholder "Activity" on every row (mendixlabs/mxcli#1267);
// an object Mendix stores no caption for (events, merges, loops) now has none.
func describeFlowObject(obj microflows.MicroflowObject, varEntity map[string]string) flowObjectRow {
	r := flowObjectRow{activityType: getMicroflowObjectType(obj)}
	r.name = r.activityType
	r.errorHandlingType = string(microflows.ObjectErrorHandlingType(obj))

	switch o := obj.(type) {
	case *microflows.Annotation:
		r.caption = o.Caption
	case *microflows.ExclusiveSplit:
		r.caption = o.Caption
		r.description = o.Documentation
		switch c := o.SplitCondition.(type) {
		case *microflows.ExpressionSplitCondition:
			r.conditionExpression = c.Expression
		case *microflows.RuleSplitCondition:
			r.conditionRule = c.RuleQualifiedName
		}
	case *microflows.InheritanceSplit:
		r.caption = o.Caption
		r.description = o.Documentation
	case *microflows.LoopedActivity:
		r.description = o.Documentation
	case *microflows.ActionActivity:
		r.caption = o.Caption
		r.autoGenerateCaption = o.AutoGenerateCaption
		r.description = o.Documentation
		if o.Action != nil {
			r.actionType = getMicroflowActionType(o.Action)
			r.name = r.actionType
			describeAction(o.Action, varEntity, &r)
		}
	}
	return r
}

// describeAction fills the action-specific columns.
//
// A call's ActionRef is the document it calls and a delete's EntityRef the
// entity of the variable it deletes — the targets refs_from() already had,
// which every call and delete row left empty (mendixlabs/mxcli#1305).
func describeAction(action microflows.MicroflowAction, varEntity map[string]string, r *flowObjectRow) {
	queue := func(qs *microflows.QueueSettings) string {
		if qs == nil {
			return ""
		}
		return qs.Queue
	}
	switch a := action.(type) {
	case *microflows.MicroflowCallAction:
		if a.MicroflowCall != nil {
			r.actionRef = a.MicroflowCall.Microflow
			r.queueRef = queue(a.MicroflowCall.QueueSettings)
		}
	case *microflows.NanoflowCallAction:
		if a.NanoflowCall != nil {
			r.actionRef = a.NanoflowCall.Nanoflow
		}
	case *microflows.JavaActionCallAction:
		r.actionRef = a.JavaAction
		r.queueRef = queue(a.QueueSettings)
	case *microflows.JavaScriptActionCallAction:
		r.actionRef = a.JavaScriptAction
	case *microflows.DeleteObjectAction:
		r.entityRef = varEntity[strings.TrimPrefix(a.DeleteVariable, "$")]
	case *microflows.CreateObjectAction:
		r.entityRef = a.EntityQualifiedName
		r.commitType = string(a.Commit)
		r.withEvents = a.Commit == microflows.CommitTypeYes
	case *microflows.ChangeObjectAction:
		r.commitType = string(a.Commit)
		r.withEvents = a.Commit == microflows.CommitTypeYes
	case *microflows.CommitObjectsAction:
		r.withEvents = a.WithEvents
	case *microflows.RetrieveAction:
		switch src := a.Source.(type) {
		case *microflows.DatabaseRetrieveSource:
			r.retrieveSource = RetrieveSourceDatabase
			r.entityRef = src.EntityQualifiedName
		case *microflows.AssociationRetrieveSource:
			r.retrieveSource = RetrieveSourceAssociation
		}
	case *microflows.LogMessageAction:
		r.logLevel = string(a.LogLevel)
		r.logNodeExpression = a.LogNodeName
		r.logMessage = textValue(a.MessageTemplate)
	case *microflows.CallExternalAction:
		r.serviceRef = a.ConsumedODataService
		r.actionRef = a.Name
	case *microflows.RestCallAction:
		// "Use a timeout" plus the seconds, which Studio Pro stores as an
		// expression string (e.g. "300").
		r.useRequestTimeout = a.UseRequestTimeOut
		r.timeoutExpression = a.TimeoutExpression
	case *microflows.WebServiceCallAction:
		// ServiceID holds the imported web service's qualified name (a
		// by-name reference).
		r.serviceRef = string(a.ServiceID)
		r.actionRef = a.OperationName
		r.useRequestTimeout = a.UseRequestTimeOut
		r.timeoutExpression = a.TimeoutExpression
	}
}

// textValue returns a stored text's value: en_US when present, otherwise the
// first language in sorted order, so the choice is deterministic.
func textValue(t *model.Text) string {
	if t == nil || len(t.Translations) == 0 {
		return ""
	}
	if v, ok := t.Translations["en_US"]; ok {
		return v
	}
	keys := make([]string, 0, len(t.Translations))
	for k := range t.Translations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return t.Translations[keys[0]]
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
