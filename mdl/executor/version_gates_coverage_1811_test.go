// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/version"
	"github.com/mendixlabs/mxcli/sdk/versions"
)

// allTypeGates are the gates pinned to the types (and properties) their
// statements write: TestTypeGates_FloorIsTheTypeIntroduction holds each row to
// the latest of them, and TestTypeGates_CoverEveryWrittenType counts a type one
// of them names as gated.
func allTypeGates() []typeGate {
	gates := []typeGate{
		// ako/mxcli#1809: every statement that writes a workflow document.
		{versionGate: workflowBasicGate("create workflow"), types: []string{"Workflows$Workflow"}},
		{versionGate: waitForNotificationGate, types: []string{"Workflows$WaitForNotificationActivity"}},
		{versionGate: waitForTimerGate, types: []string{"Workflows$WaitForTimerActivity"}},
		{versionGate: timerBoundaryEventGate, types: []string{"Workflows$TimerBoundaryEvent"}},
		{versionGate: interruptingTimerBoundaryEventGate, types: []string{
			"Workflows$InterruptingTimerBoundaryEvent", "Workflows$NonInterruptingTimerBoundaryEvent"}},
		{versionGate: workflowDefinitionSelectionGate, types: []string{
			"Workflows$WorkflowDefinitionNameSelection", "Workflows$WorkflowDefinitionObjectSelection"}},
		viewEntityGate,
	}
	gates = append(gates, workflowActionGates...)
	gates = append(gates, flowActionGates...)
	for _, g := range workflowShapeGates {
		gates = append(gates, g.typeGate)
	}
	for _, g := range documentTypeGates {
		gates = append(gates, g.typeGate)
	}
	return gates
}

// broaderTypeGates are written types a gate covers without naming them: the
// gate's statement is the only one that writes the type, and its floor is at
// or above the type's (TestTypeGates_CoverEveryWrittenType checks the floor).
var broaderTypeGates = []struct {
	gate  versionGate
	types []string
	why   string
}{
	{workflowBasicGate("create workflow"), []string{"Workflows$Flow", "Workflows$BooleanConditionOutcome",
		"Workflows$CallMicroflowTask", "Workflows$EndWorkflowActivity", "Workflows$EnumerationValueConditionOutcome",
		"Workflows$ExclusiveSplitActivity", "Workflows$MicroflowCallParameterMapping", "Workflows$ParallelSplitActivity",
		"Workflows$ParallelSplitOutcome", "Workflows$UserTaskOutcome", "Workflows$VoidConditionOutcome"},
		"written only inside a workflow"},
	{versionGate{"workflows", "multi_user_task", "multi user task", ""}, []string{"Workflows$ConsensusCompletionCriteria",
		"Workflows$VetoCompletionCriteria", "Workflows$AllUserInput", "Workflows$AbsoluteAmountUserInput",
		"Workflows$PercentageAmountUserInput"}, "a multi-user task's rule and participants"},
	{versionGate{"workflows", "multi_user_task_majority_threshold", "decide by majority", ""},
		[]string{"Workflows$MajorityCompletionCriteria", "Workflows$ThresholdCompletionCriteria"}, "decide by majority / threshold"},
	{versionGate{"workflows", "multi_user_task_microflow_rule", "decide by microflow", ""},
		[]string{"Workflows$MicroflowCompletionCriteria"}, "decide by microflow"},
	{versionGate{"workflows", "event_handlers", "on workflow events", ""},
		[]string{"Workflows$WorkflowEventHandler", "Workflows$MicroflowEventHandler"}, "written only in a workflow event handler"},
	{versionGate{"workflows", "group_targeting", "targeting groups on a user task", ""},
		[]string{"Workflows$XPathGroupTargeting", "Workflows$MicroflowGroupTargeting"}, "targeting groups"},
	{versionGate{"workflows", "event_subprocesses", "event subprocess", ""}, []string{"Workflows$EventSubProcess",
		"Workflows$NonInterruptingNotificationEventSubProcessStartActivity"}, "an event sub-process and its notification start"},
	{versionGate{"workflows", "timer_event_subprocesses", "event subprocess … on … timer", ""},
		[]string{"Workflows$InterruptingTimerEventSubProcessStartActivity", "Workflows$NonInterruptingTimerEventSubProcessStartActivity"},
		"a timer-started event sub-process"},
	{notificationEventsGate, []string{"Workflows$NotificationActivity", "Workflows$InterruptingNotificationBoundaryEvent",
		"Workflows$NonInterruptingNotificationBoundaryEvent"}, "notification activities and boundary events"},
	{agentTaskGate, []string{"Workflows$AIAgentTaskActivity"}, "call agent microflow"},
	{timerBoundaryEventGate, []string{"Workflows$EndOfBoundaryEventPathActivity"},
		"ends a boundary event's path; the earliest boundary event is the 10.14 timer"},
	{interruptingTimerBoundaryEventGate, []string{"Workflows$LinearRecurrence"},
		"a non-interrupting timer boundary event's `repeat every`, or an event sub-process timer's (11.15)"},
	{flowReturnsGate, []string{"Forms$OutputMapping"}, "returns ( … ) on a flow call"},
	{versionGate{"integration", "published_odata_service", "create published odata service", ""}, []string{
		"ODataPublish$PublishedODataService2", "ODataPublish$EntityType", "ODataPublish$EntitySet",
		"ODataPublish$PublishedAttribute", "ODataPublish$PublishedAssociationEnd", "ODataPublish$PublishedId",
		"ODataPublish$PublishedMicroflow", "ODataPublish$PublishedMicroflowParameter", "ODataPublish$QueryOptions",
		"ODataPublish$ReadSource", "ODataPublish$ChangeSource", "ODataPublish$ChangeNotSupported",
		"ODataPublish$CallMicroflowToRead", "ODataPublish$CallMicroflowToChange"}, "a published OData service's document"},
	{versionGate{"integration", "rest_client_basic", "create rest client", ""}, []string{"Rest$ConsumedRestService",
		"Rest$ValueTemplate", "Rest$StringValue", "Rest$RestOperation", "Rest$OperationParameter", "Rest$HeaderWithValueTemplate"},
		"a consumed REST service's document"},
	{versionGate{"integration", "rest_client_authentication", "authentication: basic", ""},
		[]string{"Rest$ConstantValue"}, "a basic authentication's constant user name or password"},
	{versionGate{"integration", "rest_client_operations", "an operation of a consumed REST service", ""},
		[]string{"Rest$ImplicitMappingBody", "Rest$ImplicitMappingResponseHandling"}, "an operation's body and response mapping"},
	{versionGate{"integration", "data_transformer", "create data transformer", ""}, []string{"DataTransformers$DataTransformer",
		"DataTransformers$JsltAction", "DataTransformers$JsonSource", "DataTransformers$Step", "DataTransformers$StructureObject"},
		"a data transformer's document"},
	{agentDocumentGate("agent", "create agent"), []string{"CustomBlobDocuments$CustomBlobDocument",
		"CustomBlobDocuments$CustomBlobDocumentMetadata"}, "the agent-editor documents (all four rows are 11.9)"},
	{messageDefinitionDocumentGate("create message definition"), []string{"MessageDefinitions$MessageDefinition2"},
		"the per-document message definition"},
	{versionGate{"workflows", "groups", "alter settings workflows add group", ""}, []string{"Settings$WorkflowGroup"}, "a workflow group"},
	{viewEntityGate.versionGate, []string{"DomainModels$OqlViewValue", "DomainModels$OqlViewAssociationSource"},
		"a view entity's attributes and associations"},
	{versionGate{"microflows", "send_email", "send email", ""}, []string{"Microflows$SendEmailAction", "Microflows$EmailMessage",
		"Microflows$BasicAuthConfig", "Microflows$EmailConnectionConfig", "Microflows$EmailCustomHeader"},
		"send email writes the 11.13 shape"},
	{versionGate{"integration", "database_connector_basic", "create database connection", ""}, []string{
		"DatabaseConnector$DatabaseQuery", "DatabaseConnector$QueryParameter", "DatabaseConnector$TableMapping",
		"DatabaseConnector$ColumnMapping", "DatabaseConnector$SimpleSqlDataType"}, "a database connection's queries"},
	{workflowDefinitionSelectionGate, []string{"Microflows$LockWorkflowAction", "Microflows$UnlockWorkflowAction"},
		"every lock or unlock check and exec accept names its workflow (#1787)"},
}

// typeCoverageExceptions are the written types above 9.0.0 that no gate
// names, each with why no statement reaches a project below the type's floor
// with it — or, marked RESIDUAL, why the statement cannot tell and the write
// alone refuses it there.
var typeCoverageExceptions = map[string]string{
	// Lowered: an older version stores the same thing in another shape, which
	// the writer or canon.LowerToProjectShape writes instead.
	"Microflows$BezierCurve":                            "lowered: below 10.16 a flow stores Origin/DestinationBezierVector (microflow_write.go hasLine)",
	"Workflows$SingleUserTaskActivity":                  "lowered: below 10.12 a user task is a Workflows$UserTask (canon lowerUserTaskType)",
	"Workflows$MultiUserTaskActivity":                   "lowered: below 10.12 a user task is a Workflows$UserTask (canon lowerUserTaskType)",
	"Workflows$EndOfParallelSplitPathActivity":          "lowered: below 10.21 a parallel path holds its activities only (canon unmarkedPaths)",
	"Workflows$StartWorkflowActivity":                   "lowered: the writer stores a start activity from 11.3 only (workflowStoresStartAndEnd)",
	"Workflows$CallMicroflowActivity":                   "lowered: below 11.9 the activity is stored as Workflows$CallMicroflowTask (usesCallMicroflowActivityName)",
	"Workflows$XPathUserTargeting":                      "lowered: below 11.2 a user task's targeting is its user source (canon userSource)",
	"Workflows$MicroflowUserTargeting":                  "lowered: below 11.2 a user task's targeting is its user source (canon userSource)",
	"Workflows$NoUserTargeting":                         "lowered: below 11.2 a user task's targeting is its user source (canon userSource)",
	"Settings$PrivateValue":                             "lowered: below 10.9 a configuration's constant value is flat (canon flatConstant)",
	"Settings$SharedValue":                              "lowered: below 10.9 a configuration's constant value is flat (canon flatConstant)",
	"Rest$JsonBody":                                     "lowered: below 11.0 a JSON body is a Rest$StringBody (restBodyForms)",
	"Workflows$NotifyWaitForNotificationActivityTarget": "lowered: below 11.7 notify workflow stores its target as the action's Activity (setNotifyTarget)",
	// Unreachable: the type is written only for something a project below
	// its floor cannot hold.
	"Workflows$NonInterruptingNotificationEventSubProcessStartActivityTarget": "unreachable: a notify target of that kind, which the stored workflow holds only from 11.8 (notifyTargetType)",
	"Workflows$InterruptingNotificationEventSubProcessStartActivityTarget":    "unreachable: a notify target of that kind, which the stored workflow holds only from 11.10 (notifyTargetType)",
	"Workflows$NotifyNotificationActivityTarget":                              "unreachable: a notify target of that kind, which the stored workflow holds only from 11.11 (notifyTargetType)",
	"Workflows$NotifyNotificationBoundaryEventTarget":                         "unreachable: a notify target of that kind, which the stored workflow holds only from 11.11 (notifyTargetType)",
	"Forms$SnippetParameterMapping":                                           "unreachable: a snippet call maps only the parameters the called snippet declares, and a snippet holds parameters from 9.21 (Forms$Snippet.Parameters)",
	"CodeActions$CustomBlobDocumentParameterType":                             "unreachable: carried only from a stored Java action parameter of that type, which a project below 11.6 cannot hold",
	"Microflows$CustomBlobDocumentCodeActionParameterValue":                   "unreachable: written only for a Java action parameter of a custom document type, which a project below 11.6 cannot hold",
	"Microflows$IncludedAssociation":                                          "unreachable: carried only from a stored external action parameter's associations to include, which a project below 10.11 cannot hold (#1914)",
	// Residual: the statement does not say which shape the writer picks, so
	// no gate can; the write refuses it below its floor.
	"Forms$ToggleDesignPropertyValue":                "lowered below 10.2 to a flat DesignPropertyValue (canon lowerDesignProperty)",
	"Forms$OptionDesignPropertyValue":                "lowered below 10.2 to a flat DesignPropertyValue (canon lowerDesignProperty)",
	"Forms$CustomDesignPropertyValue":                "RESIDUAL: below 10.2 the lowering refuses it at write time; which kind a design property is comes from the theme's definitions, not the statement",
	"Forms$CompoundDesignPropertyValue":              "RESIDUAL: below 10.2 the lowering refuses it at write time; which kind a design property is comes from the theme's definitions, not the statement",
	"Rest$ODataEntityTypeSource":                     "RESIDUAL: an external entity's source kind comes from the consumed service's $metadata (cmd_contract.go: a type with no entity set), not the statement",
	"Rest$ODataPrimitiveCollectionEntitySource":      "RESIDUAL: chosen by the consumed service's $metadata (a primitive collection), not the statement",
	"Rest$ODataPrimitiveCollectionAssociationSource": "RESIDUAL: chosen by the consumed service's $metadata (a primitive collection), not the statement",
	"Rest$ODataMappedPrimitiveCollectionValue":       "RESIDUAL: chosen by the consumed service's $metadata (a primitive collection), not the statement",
	"Microflows$QueryParameterMapping":               "RESIDUAL: written for an argument that is not one of the operation's path parameters: a query parameter (Rest$QueryParameter, itself 11.0) or a name the stored operation lacks",
	"Projects$ModuleSettings":                        "RESIDUAL: every module creation writes it — create module, and the module any statement auto-creates — so no one statement can be gated, and below 9.12 the write refuses the module on ModuleSettings.ProtectedModuleType anyway: the fix is to lower the settings unit, not a gate",
}

// The write paths: the converters and the builders that name storage types.
var writtenTypeSources = []string{
	"../backend/modelsdk/*.go",
	"../backend/pagemutator/*.go",
	"../scheduledevents/*.go",
	"../settingsoverlay/*.go",
	"../../sdk/*/*.go",
	"cmd_microflows_builder*.go",
	"cmd_pages_builder*.go",
}

var (
	typeLiteralRe  = regexp.MustCompile(`"([A-Z][A-Za-z]+\$[A-Za-z0-9_]+)"`)
	genImportRe    = regexp.MustCompile(`(\w+)\s+"github.com/mendixlabs/mxcli/modelsdk/gen/(\w+)"`)
	genCallRe      = regexp.MustCompile(`\b(\w+)\.New([A-Z]\w*)\(`)
	genTypeNameRe  = regexp.MustCompile(`func init(\w+)\(\) \*\w+ \{\n\to := &\w+\{\}\n\to\.SetTypeName\("([^"]+)"\)`)
	readOnlySource = regexp.MustCompile(`_test\.go$|_read|describe|format|mermaid|diff`)
)

// writtenTypes are the storage $Types the write paths construct — a
// "Module$Type" literal, or a modelsdk/gen constructor resolved to the type it
// stamps — with the files that do, leaving out readers and describers.
func writtenTypes(t *testing.T) map[string][]string {
	t.Helper()
	genTypes := map[string]map[string]string{}
	genType := func(pkg, name string) string {
		if genTypes[pkg] == nil {
			genTypes[pkg] = map[string]string{}
			files, _ := filepath.Glob(filepath.Join("..", "..", "modelsdk", "gen", pkg, "*.go"))
			for _, f := range files {
				b, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				for _, m := range genTypeNameRe.FindAllStringSubmatch(string(b), -1) {
					genTypes[pkg][m[1]] = m[2]
				}
			}
		}
		return genTypes[pkg][name]
	}
	out := map[string][]string{}
	for _, pattern := range writtenTypeSources {
		files, err := filepath.Glob(pattern)
		if err != nil || len(files) == 0 {
			t.Fatalf("no write path matches %s", pattern)
		}
		for _, f := range files {
			if readOnlySource.MatchString(filepath.Base(f)) {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			src := string(b)
			aliases := map[string]string{}
			for _, m := range genImportRe.FindAllStringSubmatch(src, -1) {
				aliases[m[1]] = m[2]
			}
			seen := map[string]bool{}
			for _, line := range strings.Split(src, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				var types []string
				for _, m := range typeLiteralRe.FindAllStringSubmatch(line, -1) {
					types = append(types, m[1])
				}
				for _, m := range genCallRe.FindAllStringSubmatch(line, -1) {
					if pkg, ok := aliases[m[1]]; ok {
						if typ := genType(pkg, m[2]); typ != "" {
							types = append(types, typ)
						}
					}
				}
				for _, typ := range types {
					if !seen[typ] {
						seen[typ] = true
						out[typ] = append(out[typ], filepath.ToSlash(f))
					}
				}
			}
		}
	}
	return out
}

// ako/mxcli#1811: every storage $Type a write path constructs whose
// introduction is above the lowest supported version (9.0.0) is gated — named
// by a type gate, or covered by a gate whose floor is at or above it — or
// listed in typeCoverageExceptions with why no statement reaches a project
// below its floor. A converter that starts writing a newer type fails here
// until it is one or the other, instead of passing `check -p` and being
// refused by the write part-way through a script, which is how #1774, #1787,
// #1809 and #1811 shipped.
func TestTypeGates_CoverEveryWrittenType(t *testing.T) {
	reg, err := versions.Load()
	if err != nil {
		t.Fatal(err)
	}
	lowest := version.Version{Major: 9}
	named := map[string]bool{}
	for _, g := range allTypeGates() {
		for _, typ := range g.types {
			named[typ] = true
		}
	}
	broader := map[string]versionGate{}
	for _, b := range broaderTypeGates {
		if b.why == "" {
			t.Errorf("%s.%s covers %v with no reason", b.gate.area, b.gate.name, b.types)
		}
		for _, typ := range b.types {
			broader[typ] = b.gate
		}
	}
	written := writtenTypes(t)
	var types []string
	for typ := range written {
		types = append(types, typ)
	}
	sort.Strings(types)
	counted := 0
	for _, typ := range types {
		since, ok := canon.TypeIntroduced(typ)
		if !ok || since.Compare(lowest) <= 0 {
			continue
		}
		counted++
		at := versions.SemVer{Major: since.Major, Minor: since.Minor, Patch: since.Patch}
		switch gate, ok := broader[typ]; {
		case named[typ]:
		case ok:
			if reg.IsAvailable(gate.area, gate.name, justBelow(at)) {
				t.Errorf("%s (Mendix %s+) is covered by %s.%s, which is available at %s, where the write refuses the type",
					typ, since, gate.area, gate.name, justBelow(at))
			}
		case typeCoverageExceptions[typ] != "":
		default:
			t.Errorf("%s (Mendix %s+) is written by %v and no gate names it: gate the statement that writes it "+
				"(a typeGate pinned to the type), or list it in typeCoverageExceptions with why no project below %s reaches it",
				typ, since, written[typ], since)
		}
	}
	if counted < 100 {
		t.Fatalf("found only %d written types above %s: the scan of the write paths is not reading them", counted, lowest)
	}
	// Nothing listed is stale: each type is still written, above 9.0.0.
	listed := map[string]string{}
	for typ := range typeCoverageExceptions {
		listed[typ] = "typeCoverageExceptions"
	}
	for typ := range broader {
		listed[typ] = "broaderTypeGates"
	}
	for typ, where := range listed {
		since, ok := canon.TypeIntroduced(typ)
		if len(written[typ]) == 0 || !ok || since.Compare(lowest) <= 0 {
			t.Errorf("%s in %s is no longer written above %s: remove it", typ, where, lowest)
		}
	}
}
