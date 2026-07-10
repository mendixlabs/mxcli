// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
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

	mfStmt, err := b.tx.Prepare(`
		INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, Folder, MicroflowType,
			Description, ReturnType, ParameterCount, ActivityCount, Complexity, Excluded,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer mfStmt.Close()

	// Prepare activity statement only in full mode
	var actStmt *sql.Stmt
	if b.fullMode {
		actStmt, err = b.tx.Prepare(`
			INSERT INTO activities_data (Id, Name, Caption, ActivityType, Sequence, MicroflowId, MicroflowQualifiedName,
				ModuleName, Folder, EntityRef, ActionType, ServiceRef, ActionRef, Description,
				ProjectId, SnapshotId)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer actStmt.Close()
	}

	projectID, snapshotID := b.snapshotMeta()

	mfCount := 0
	nfCount := 0
	actCount := 0

	// Process microflows
	for _, mf := range mfs {
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
		activityCount := countMicroflowActivities(mf)

		// Calculate McCabe cyclomatic complexity
		complexity := calculateMcCabeComplexity(mf)

		_, err = mfStmt.Exec(
			string(mf.ID),
			mf.Name,
			qualifiedName,
			moduleName,
			moduleName, // Folder
			"MICROFLOW",
			mf.Documentation,
			returnType,
			len(mf.Parameters),
			activityCount,
			complexity,
			mf.Excluded,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		mfCount++

		// Insert activities only in full mode
		if b.fullMode && mf.ObjectCollection != nil {
			for seq, obj := range mf.ObjectCollection.Objects {
				activityType := getMicroflowObjectType(obj)
				activityName := activityType
				caption := "Activity"
				entityRef := ""
				actionType := ""
				serviceRef := ""
				actionRef := ""

				if act, ok := obj.(*microflows.ActionActivity); ok {
					if act.Action != nil {
						actionType = getMicroflowActionType(act.Action)
						activityName = actionType

						switch a := act.Action.(type) {
						case *microflows.CreateObjectAction:
							entityRef = a.EntityQualifiedName
						case *microflows.CallExternalAction:
							serviceRef = a.ConsumedODataService
							actionRef = a.Name
						}
					}
				}

				_, err = actStmt.Exec(
					string(obj.GetID()),
					activityName,
					caption,
					activityType,
					seq+1, // 1-based sequence number
					string(mf.ID),
					qualifiedName,
					moduleName,
					moduleName,
					entityRef,
					actionType,
					serviceRef,
					actionRef,
					"",
					projectID, snapshotID,
				)
				if err != nil {
					return err
				}
				actCount++
			}
		}
	}

	// Process nanoflows
	for _, nf := range nfs {
		// Get module name
		moduleID := b.hierarchy.findModuleID(nf.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + nf.Name

		returnType := ""
		if nf.ReturnType != nil {
			returnType = getDataTypeName(nf.ReturnType)
		}

		// Count activities (excluding structural elements like Start/End events)
		activityCount := countNanoflowActivities(nf)

		// Calculate McCabe cyclomatic complexity
		complexity := calculateNanoflowComplexity(nf)

		_, err = mfStmt.Exec(
			string(nf.ID),
			nf.Name,
			qualifiedName,
			moduleName,
			moduleName, // Folder
			"NANOFLOW",
			nf.Documentation,
			returnType,
			len(nf.Parameters),
			activityCount,
			complexity,
			nf.Excluded,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
		nfCount++

		// Insert activities only in full mode
		if b.fullMode && nf.ObjectCollection != nil {
			for seq, obj := range nf.ObjectCollection.Objects {
				activityType := getMicroflowObjectType(obj)
				activityName := activityType
				caption := "Activity"
				entityRef := ""
				actionType := ""
				serviceRef := ""
				actionRef := ""

				if act, ok := obj.(*microflows.ActionActivity); ok {
					if act.Action != nil {
						actionType = getMicroflowActionType(act.Action)
						activityName = actionType

						switch a := act.Action.(type) {
						case *microflows.CreateObjectAction:
							entityRef = a.EntityQualifiedName
						case *microflows.CallExternalAction:
							serviceRef = a.ConsumedODataService
							actionRef = a.Name
						}
					}
				}

				_, err = actStmt.Exec(
					string(obj.GetID()),
					activityName,
					caption,
					activityType,
					seq+1, // 1-based sequence number
					string(nf.ID),
					qualifiedName,
					moduleName,
					moduleName,
					entityRef,
					actionType,
					serviceRef,
					actionRef,
					"",
					projectID, snapshotID,
				)
				if err != nil {
					return err
				}
				actCount++
			}
		}
	}

	b.report("Microflows", mfCount)
	b.report("Nanoflows", nfCount)
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

// countMicroflowActivities counts activities in a microflow, excluding structural elements.
// This excludes Start/End events and Merge nodes which are structural, not business logic.
func countMicroflowActivities(mf *microflows.Microflow) int {
	if mf.ObjectCollection == nil {
		return 0
	}

	count := 0
	for _, obj := range mf.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.StartEvent, *microflows.EndEvent:
			// Don't count start/end events
		case *microflows.ExclusiveMerge:
			// Don't count merge nodes (they're structural)
		default:
			// Count all other activities (ActionActivity, ExclusiveSplit, LoopedActivity, etc.)
			count++
		}
	}
	return count
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

// countNanoflowActivities counts activities in a nanoflow, excluding structural elements.
func countNanoflowActivities(nf *microflows.Nanoflow) int {
	if nf.ObjectCollection == nil {
		return 0
	}

	count := 0
	for _, obj := range nf.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.StartEvent, *microflows.EndEvent:
			// Don't count start/end events
		case *microflows.ExclusiveMerge:
			// Don't count merge nodes (they're structural)
		default:
			count++
		}
	}
	return count
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
