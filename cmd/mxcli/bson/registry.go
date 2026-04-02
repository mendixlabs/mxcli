// Package bson provides BSON inspection and field coverage analysis utilities
// for Mendix project files.
package bson

import (
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/generated/metamodel"
)

// TypeRegistry maps BSON $Type strings to Go reflect.Type for all Workflows$ structs.
var TypeRegistry = map[string]reflect.Type{
	"Workflows$AbsoluteAmountUserInput":           reflect.TypeOf(metamodel.WorkflowsAbsoluteAmountUserInput{}),
	"Workflows$AllUserInput":                      reflect.TypeOf(metamodel.WorkflowsAllUserInput{}),
	"Workflows$Annotation":                        reflect.TypeOf(metamodel.WorkflowsAnnotation{}),
	"Workflows$BezierCurve":                       reflect.TypeOf(metamodel.WorkflowsBezierCurve{}),
	"Workflows$BooleanCase":                       reflect.TypeOf(metamodel.WorkflowsBooleanCase{}),
	"Workflows$BooleanConditionOutcome":           reflect.TypeOf(metamodel.WorkflowsBooleanConditionOutcome{}),
	"Workflows$CallMicroflowTask":                 reflect.TypeOf(metamodel.WorkflowsCallMicroflowTask{}),
	"Workflows$CallWorkflowActivity":              reflect.TypeOf(metamodel.WorkflowsCallWorkflowActivity{}),
	"Workflows$ConsensusCompletionCriteria":       reflect.TypeOf(metamodel.WorkflowsConsensusCompletionCriteria{}),
	"Workflows$EndOfBoundaryEventPathActivity":    reflect.TypeOf(metamodel.WorkflowsEndOfBoundaryEventPathActivity{}),
	"Workflows$EndOfParallelSplitPathActivity":    reflect.TypeOf(metamodel.WorkflowsEndOfParallelSplitPathActivity{}),
	"Workflows$EndWorkflowActivity":               reflect.TypeOf(metamodel.WorkflowsEndWorkflowActivity{}),
	"Workflows$EnumerationValueConditionOutcome":  reflect.TypeOf(metamodel.WorkflowsEnumerationValueConditionOutcome{}),
	"Workflows$ExclusiveSplitActivity":            reflect.TypeOf(metamodel.WorkflowsExclusiveSplitActivity{}),
	"Workflows$FloatingAnnotation":                reflect.TypeOf(metamodel.WorkflowsFloatingAnnotation{}),
	"Workflows$Flow":                              reflect.TypeOf(metamodel.WorkflowsFlow{}),
	"Workflows$FlowLine":                          reflect.TypeOf(metamodel.WorkflowsFlowLine{}),
	"Workflows$InterruptingTimerBoundaryEvent":    reflect.TypeOf(metamodel.WorkflowsInterruptingTimerBoundaryEvent{}),
	"Workflows$JumpToActivity":                    reflect.TypeOf(metamodel.WorkflowsJumpToActivity{}),
	"Workflows$LinearRecurrence":                  reflect.TypeOf(metamodel.WorkflowsLinearRecurrence{}),
	"Workflows$MajorityCompletionCriteria":        reflect.TypeOf(metamodel.WorkflowsMajorityCompletionCriteria{}),
	"Workflows$MergeActivity":                     reflect.TypeOf(metamodel.WorkflowsMergeActivity{}),
	"Workflows$MicroflowBasedEvent":               reflect.TypeOf(metamodel.WorkflowsMicroflowBasedEvent{}),
	"Workflows$MicroflowCallParameterMapping":     reflect.TypeOf(metamodel.WorkflowsMicroflowCallParameterMapping{}),
	"Workflows$MicroflowCompletionCriteria":       reflect.TypeOf(metamodel.WorkflowsMicroflowCompletionCriteria{}),
	"Workflows$MicroflowEventHandler":             reflect.TypeOf(metamodel.WorkflowsMicroflowEventHandler{}),
	"Workflows$MicroflowGroupTargeting":           reflect.TypeOf(metamodel.WorkflowsMicroflowGroupTargeting{}),
	"Workflows$MicroflowUserTargeting":            reflect.TypeOf(metamodel.WorkflowsMicroflowUserTargeting{}),
	"Workflows$MultiUserTaskActivity":             reflect.TypeOf(metamodel.WorkflowsMultiUserTaskActivity{}),
	"Workflows$NoEvent":                           reflect.TypeOf(metamodel.WorkflowsNoEvent{}),
	"Workflows$NoUserTargeting":                   reflect.TypeOf(metamodel.WorkflowsNoUserTargeting{}),
	"Workflows$NonInterruptingTimerBoundaryEvent": reflect.TypeOf(metamodel.WorkflowsNonInterruptingTimerBoundaryEvent{}),
	"Workflows$OrthogonalPath":                    reflect.TypeOf(metamodel.WorkflowsOrthogonalPath{}),
	"Workflows$PageParameterMapping":              reflect.TypeOf(metamodel.WorkflowsPageParameterMapping{}),
	"Workflows$PageReference":                     reflect.TypeOf(metamodel.WorkflowsPageReference{}),
	"Workflows$ParallelSplitActivity":             reflect.TypeOf(metamodel.WorkflowsParallelSplitActivity{}),
	"Workflows$ParallelSplitOutcome":              reflect.TypeOf(metamodel.WorkflowsParallelSplitOutcome{}),
	"Workflows$Parameter":                         reflect.TypeOf(metamodel.WorkflowsParameter{}),
	"Workflows$PercentageAmountUserInput":         reflect.TypeOf(metamodel.WorkflowsPercentageAmountUserInput{}),
	"Workflows$SingleUserTaskActivity":            reflect.TypeOf(metamodel.WorkflowsSingleUserTaskActivity{}),
	"Workflows$StartWorkflowActivity":             reflect.TypeOf(metamodel.WorkflowsStartWorkflowActivity{}),
	"Workflows$StringCase":                        reflect.TypeOf(metamodel.WorkflowsStringCase{}),
	"Workflows$ThresholdCompletionCriteria":       reflect.TypeOf(metamodel.WorkflowsThresholdCompletionCriteria{}),
	"Workflows$UserTaskOutcome":                   reflect.TypeOf(metamodel.WorkflowsUserTaskOutcome{}),
	"Workflows$VetoCompletionCriteria":            reflect.TypeOf(metamodel.WorkflowsVetoCompletionCriteria{}),
	"Workflows$VoidCase":                          reflect.TypeOf(metamodel.WorkflowsVoidCase{}),
	"Workflows$VoidConditionOutcome":              reflect.TypeOf(metamodel.WorkflowsVoidConditionOutcome{}),
	"Workflows$WaitForNotificationActivity":       reflect.TypeOf(metamodel.WorkflowsWaitForNotificationActivity{}),
	"Workflows$WaitForTimerActivity":              reflect.TypeOf(metamodel.WorkflowsWaitForTimerActivity{}),
	"Workflows$Workflow":                          reflect.TypeOf(metamodel.WorkflowsWorkflow{}),
	"Workflows$WorkflowCallParameterMapping":      reflect.TypeOf(metamodel.WorkflowsWorkflowCallParameterMapping{}),
	"Workflows$WorkflowDefinitionNameSelection":   reflect.TypeOf(metamodel.WorkflowsWorkflowDefinitionNameSelection{}),
	"Workflows$WorkflowDefinitionObjectSelection": reflect.TypeOf(metamodel.WorkflowsWorkflowDefinitionObjectSelection{}),
	"Workflows$WorkflowEventHandler":              reflect.TypeOf(metamodel.WorkflowsWorkflowEventHandler{}),
	"Workflows$WorkflowMetaData":                  reflect.TypeOf(metamodel.WorkflowsWorkflowMetaData{}),
	"Workflows$XPathGroupTargeting":               reflect.TypeOf(metamodel.WorkflowsXPathGroupTargeting{}),
	"Workflows$XPathUserTargeting":                reflect.TypeOf(metamodel.WorkflowsXPathUserTargeting{}),
}

// PropertyMeta describes a single field's metadata derived from reflection.
type PropertyMeta struct {
	GoFieldName string
	StorageName string // from json tag
	GoType      string
	IsList      bool
	IsPointer   bool
	IsRequired  bool // json tag lacks "omitempty"
	Category    FieldCategory
}

// GetFieldMeta returns all field metadata for a given BSON $Type string.
// Returns nil if the type is not found in TypeRegistry.
func GetFieldMeta(bsonType string) []PropertyMeta {
	rt, ok := TypeRegistry[bsonType]
	if !ok {
		return nil
	}

	var fields []PropertyMeta
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Anonymous {
			continue // skip embedded BaseElement
		}

		jsonTag := f.Tag.Get("json")
		storageName, ok := parseJSONTag(jsonTag)
		if !ok {
			continue
		}
		isRequired := !strings.Contains(jsonTag, "omitempty")

		fields = append(fields, PropertyMeta{
			GoFieldName: f.Name,
			StorageName: storageName,
			GoType:      f.Type.String(),
			IsList:      f.Type.Kind() == reflect.Slice,
			IsPointer:   f.Type.Kind() == reflect.Ptr,
			IsRequired:  isRequired,
			Category:    classifyField(storageName),
		})
	}
	return fields
}

func parseJSONTag(tag string) (string, bool) {
	if tag == "" || tag == "-" {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	return name, true
}
