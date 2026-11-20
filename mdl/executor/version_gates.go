// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// versionGate is one feature a statement needs from the project's Mendix
// version: the sdk/versions registry entry (area, name) that holds its floor,
// how the refusal names the statement, and what to do instead.
type versionGate struct {
	area, name string
	statement  string
	hint       string
}

// statementVersionGates lists the version gates stmt has to pass, in the order
// exec applies them. It is the one place a statement is mapped to the features
// it needs: the handlers refuse through checkVersionGates, and `mxcli check -p`
// and exec's pre-flight report through CheckVersionGates, so the three cannot
// disagree about which statement needs which version (mendixlabs/mxcli#1434 —
// check passed a script exec then refused for the project's version). The
// floors themselves live in sdk/versions/*.yaml.
//
// A gate that depends on what the project stores, not only on the statement
// (requireMessageDefinitionDocuments on a drop/alter of a document that is not
// there), stays with its handler: check cannot know it from the script.
func statementVersionGates(stmt ast.Statement) []versionGate {
	return append(statementCaseGates(stmt), documentGates(stmt)...)
}

// statementCaseGates are the gates statementVersionGates names case by case;
// documentTypeGates add the document shapes (ako/mxcli#1811).
func statementCaseGates(stmt ast.Statement) []versionGate {
	switch s := stmt.(type) {
	case *ast.CreateRestClientStmt:
		return []versionGate{{"integration", "rest_client_basic", "create rest client",
			"upgrade your project to 10.1+"}}
	case *ast.AlterRestClientStmt:
		return []versionGate{{"integration", "rest_client_basic", "alter consumed rest service",
			"upgrade your project to 10.1+"}}
	case *ast.CreateDataTransformerStmt:
		return []versionGate{{"integration", "data_transformer", "create data transformer",
			"upgrade your project to 11.9+"}}
	case *ast.CreatePublishedRestServiceStmt:
		return []versionGate{{"integration", "published_rest_service", "create published rest service",
			"upgrade your project to 10.0+"}}
	case *ast.AlterPublishedRestServiceStmt:
		return []versionGate{{"integration", "published_rest_alter", "alter published rest service",
			"upgrade your project to 10.0+"}}
	case *ast.GrantPublishedRestServiceAccessStmt:
		return []versionGate{{"integration", "published_rest_grant_revoke", "grant access on published rest service",
			"upgrade your project to 10.0+"}}
	case *ast.CreateODataServiceStmt:
		// The document mxcli writes, ODataPublish$PublishedODataService2, is
		// 10.4+; before, a published OData service is a Rest$PublishedODataService,
		// a different document mxcli does not write (ako/mxcli#1163).
		gates := []versionGate{{"integration", "published_odata_service", "create published odata service",
			"Before Mendix 10.4 a published OData service is a different document " +
				"(Rest$PublishedODataService), which mxcli does not write. Publish it in Studio Pro, or upgrade the project."}}
		if s.SupportsGraphQL {
			gates = append(gates, versionGate{"integration", "odata_graphql", "SupportsGraphQL on a published OData service",
				"Publishing a service as GraphQL as well arrived in Studio Pro 10.14. " +
					"Remove SupportsGraphQL to publish OData only."})
		}
		return gates
	case *ast.CreateMessageDefinitionStmt:
		return []versionGate{messageDefinitionDocumentGate("create message definition")}
	case *ast.AlterSettingsStmt:
		if (s.AddGroup || s.ModifyGroup || s.UpsertGroup || s.RemoveGroup) && strings.EqualFold(s.Section, "workflows") {
			return []versionGate{{"workflows", "groups", "alter settings workflows add group '<name>'",
				"Workflow groups (Settings$WorkflowGroup) were introduced in the 11.2 metamodel. " +
					"On an earlier version, model the buckets as your own entity and target user tasks with a microflow."}}
		}
	case *ast.CreateViewEntityStmt:
		gates := []versionGate{viewEntityGate.versionGate}
		// An OQL comment — `--` or `/* */`, anywhere in the query — is CE0174
		// "Error(s) in OQL query: The '-' part is incomplete or incorrect" below
		// the floor. The query now keeps a comment before its first token or
		// after its last (ako/mxcli#1632); one between clauses it always kept, and
		// check passed what mxbuild then rejected. Gated on a COMMENT, not on the
		// characters, so `'--'` in a string literal is not one.
		if q := s.Query.RawQuery; stripOQLComments(q) != q {
			gates = append(gates, versionGate{"oql_functions", "comments",
				"a comment in a view entity's OQL query",
				"remove the comments from the query — Mendix rejects them as CE0174 before 11.7 — or upgrade the project"})
		}
		// STRING_AGG is an OQL aggregate from 11.2, and from 10.24.22 on the
		// 10.24 line (ako/mxcli#1650). Below that floor mxbuild has no such
		// function, and the view fails the build.
		if oqlUsesStringAgg(s.Query.RawQuery) {
			gates = append(gates, versionGate{"oql_functions", "string_agg",
				"STRING_AGG in a view entity's OQL query",
				"Mendix added STRING_AGG to OQL in 11.2 and in 10.24.22 — upgrade the project, " +
					"or build the list in a microflow"})
		}
		return gates
	// A domain-model EXPORT LEVEL is 9.3+ (mendixlabs/mxcli#1498). Gated on the
	// clause being STATED, as the deep-link URL is: a statement without one
	// carries the stored level and writes nothing new.
	case *ast.CreateEntityStmt:
		if s.ExportLevel != nil || attributesStateExportLevel(s.Attributes) {
			return []versionGate{exportLevelGate("export level on an entity or attribute")}
		}
	case *ast.AlterEntityStmt:
		if s.Operation == ast.AlterEntitySetExportLevel || s.ModifyExportLevel != nil ||
			(s.Attribute != nil && s.Attribute.ExportLevel != nil) {
			return []versionGate{exportLevelGate("export level on an entity or attribute")}
		}
	case *ast.CreateAssociationStmt:
		if s.ExportLevel != nil {
			return []versionGate{exportLevelGate("export level on an association")}
		}
	case *ast.AlterAssociationStmt:
		if s.Operation == ast.AlterAssociationSetExportLevel {
			return []versionGate{exportLevelGate("export level on an association")}
		}
	case *ast.CreateEnumerationStmt:
		if s.ExportLevel != nil {
			return []versionGate{exportLevelGate("export level on an enumeration")}
		}
	case *ast.AlterEnumerationStmt:
		if s.Operation == ast.AlterEnumSetExportLevel {
			return []versionGate{exportLevelGate("export level on an enumeration")}
		}
	case *ast.CreateModelStmt:
		return []versionGate{agentDocumentGate("agent_model", "create model")}
	case *ast.CreateConsumedMCPServiceStmt:
		return []versionGate{agentDocumentGate("agent_consumed_mcp_service", "create consumed mcp service")}
	case *ast.CreatePublishedMCPServiceStmt:
		return []versionGate{agentDocumentGate("agent_published_mcp_service", "create published mcp service")}
	case *ast.CreateKnowledgeBaseStmt:
		return []versionGate{agentDocumentGate("agent_knowledge_base", "create knowledge base")}
	case *ast.CreateAgentStmt:
		return []versionGate{agentDocumentGate("agent", "create agent")}
	case *ast.CreatePageStmtV3:
		var gates []versionGate
		// Pages$PageParameter is 9.4+ (mendixlabs/mxcli#1121): 9.0–9.3 has none.
		if len(s.Parameters) > 0 {
			gates = append(gates, versionGate{"pages", "page_parameters", "create page with parameters",
				"pass data via a non-persistent entity or microflow parameter instead"})
		}
		return append(gates, flowReturnsGates(s)...)
	case *ast.CreateSnippetStmtV3, *ast.AlterPageStmt:
		return flowReturnsGates(s)
	case *ast.CreateMicroflowStmt:
		gates := microflowBodyGates(s.Parameters, s.Body)
		// Deep links are Mendix 10.6+. Gated on the clause being STATED, not on
		// the resulting value: a rewrite that carries a stored URL forward on
		// an older project preserves what is already there.
		if s.URL != nil || s.URLSearchParameters != nil {
			gates = append(gates, versionGate{"microflows", "deep_link_url", "a microflow URL (deep link)",
				"upgrade your project to 10.6+, or set the URL in Studio Pro"})
		}
		return gates
	case *ast.CreateNanoflowStmt:
		return nanoflowBodyGates(s.Parameters, s.Body)
	case *ast.AlterFlowStmt:
		// What an insert or a replace splices in is gated as the same
		// statements in a `create` are; a drop writes nothing new. Without
		// this, check -p passed an alter that the write then refused
		// (ako/mxcli#1787).
		var inserted []ast.MicroflowStatement
		for _, op := range s.Operations {
			inserted = append(inserted, op.Body...)
		}
		if s.Nanoflow {
			return nanoflowBodyGates(nil, inserted)
		}
		return microflowBodyGates(nil, inserted)
	case *ast.CreateWorkflowStmt:
		// `create or modify workflow` is this statement too: it writes the
		// same Workflows$Workflow (ako/mxcli#1809).
		gates := []versionGate{workflowBasicGate("create workflow")}
		if len(s.EventHandlers) > 0 {
			gates = append(gates, versionGate{"workflows", "event_handlers", "on workflow events",
				"remove the `on … workflow event` clauses, or upgrade the project"})
		}
		activities := workflowStatementActivities(s)
		if workflowUsesAgentTask(activities) {
			gates = append(gates, agentTaskGate)
		}
		if len(s.EventSubProcesses) > 0 {
			gates = append(gates, versionGate{"workflows", "event_subprocesses", "event subprocess",
				"event sub-processes need Mendix 11.8 or later"})
		}
		for _, esp := range s.EventSubProcesses {
			if esp.Timer {
				gates = append(gates, versionGate{"workflows", "timer_event_subprocesses", "event subprocess … on … timer",
					"a timer-started event sub-process needs Mendix 11.13 or later — start it with a notification on older projects"})
				break
			}
		}
		for _, esp := range s.EventSubProcesses {
			if esp.Recurrence != nil {
				gates = append(gates, versionGate{"workflows", "recurring_timer_event_subprocesses", "event subprocess … repeat every",
					"a repeating event sub-process start needs Mendix 11.15 or later — the 11.13 runtime ignores the recurrence and starts it once; " +
						"repeat a non-interrupting boundary timer instead (`boundary event non interrupting timer … repeat every …`)"})
				break
			}
		}
		if workflowUsesNotificationEvents(activities) {
			gates = append(gates, notificationEventsGate)
		}
		gates = append(gates, workflowTypeGates(activities, nil)...)
		gates = append(gates, userTaskGates(activities)...)
		return append(gates, workflowShapeGatesOf(s)...)
	case *ast.AlterWorkflowStmt:
		gates := []versionGate{workflowBasicGate("alter workflow")}
		added := alterWorkflowAddedActivities(s)
		if workflowUsesAgentTask(added) {
			gates = append(gates, agentTaskGate)
		}
		if workflowUsesNotificationEvents(added) {
			gates = append(gates, notificationEventsGate)
		}
		var inserted []string
		for _, op := range s.Operations {
			if o, ok := op.(*ast.InsertBoundaryEventOp); ok {
				inserted = append(inserted, o.EventType)
			}
		}
		gates = append(gates, workflowTypeGates(added, inserted)...)
		gates = append(gates, userTaskGates(added)...)
		return append(gates, workflowShapeGatesOf(s)...)
	}
	return nil
}

// viewEntityGate is the gate of `create view entity`. Its floor is the latest
// of what the write stores for one: the query in a separate
// DomainModels$ViewEntitySourceDocument, which the entity's source names
// (OqlViewEntitySource.SourceDocument), both 10.21 — not 10.18, where the
// OQL was inline and the write refuses the document (ako/mxcli#1811).
var viewEntityGate = typeGate{
	versionGate: versionGate{"domain_model", "view_entities", "create view entity",
		"use a regular entity with a microflow data source, or upgrade the project"},
	types:      []string{"DomainModels$ViewEntitySourceDocument", "DomainModels$OqlViewEntitySource"},
	properties: []string{"DomainModels$OqlViewEntitySource.SourceDocument"},
}

// workflowBasicGate is the gate of every statement that writes a workflow
// document: Workflows$Workflow is a 9.0.2 type, and the write refuses it below
// that (ako/mxcli#1809). The hint names no version; the message states the
// registry's floor.
func workflowBasicGate(statement string) versionGate {
	return versionGate{"workflows", "basic", statement,
		"this version has no workflows (Workflows$Workflow): upgrade the project"}
}

// The gates of the workflow activity and event TYPES mxcli writes that an
// older Mendix does not have (ako/mxcli#1774). Each type's introduction is in
// the Model SDK's version data (mendixmodelsdk 4.117.0), and the write path
// refuses the type below it (canon.UndeclaredTypeError); these name the
// statement first.
var (
	waitForNotificationGate = versionGate{"workflows", "wait_for_notification", "wait for notification",
		"Workflows$WaitForNotificationActivity arrived in Mendix 10.0: upgrade the project"}
	waitForTimerGate = versionGate{"workflows", "wait_for_timer", "wait for timer",
		"Workflows$WaitForTimerActivity arrived in Mendix 10.6: upgrade the project"}
	timerBoundaryEventGate = versionGate{"workflows", "timer_boundary_event", "boundary event timer",
		"boundary events arrived in Mendix 10.14: upgrade the project"}
	interruptingTimerBoundaryEventGate = versionGate{"workflows", "interrupting_timer_boundary_event",
		"boundary event interrupting timer / non interrupting timer",
		"the (non-)interrupting timer boundary events are Mendix 10.20 types; 10.14–10.19 store a plain timer " +
			"boundary event, which mxcli writes for `boundary event timer …`, or upgrade the project"}
	workflowDefinitionSelectionGate = versionGate{"workflows", "workflow_definition_selection",
		"lock workflow / unlock workflow on a workflow",
		"before Mendix 10.0 the lock and unlock actions name their workflow differently, which mxcli does not write: " +
			"set it in Studio Pro, or upgrade the project"}
)

// workflowTypeGates are the gates of the wait activities and timer boundary
// events among activities, and of the boundary events an `alter workflow`
// inserts (their EventTypes), each once.
func workflowTypeGates(activities []ast.WorkflowActivityNode, insertedEvents []string) []versionGate {
	var notification, timer, plainTimer, interruptingTimer bool
	event := func(eventType string) {
		switch eventType {
		case "Timer":
			plainTimer = true
		case "", "InterruptingTimer", "NonInterruptingTimer":
			interruptingTimer = true
		}
	}
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		switch a.(type) {
		case *ast.WorkflowWaitForNotificationNode:
			notification = true
		case *ast.WorkflowWaitForTimerNode:
			timer = true
		}
		for _, e := range workflowActivityBoundaryEvents(a) {
			event(e.EventType)
		}
	})
	for _, e := range insertedEvents {
		event(e)
	}
	var gates []versionGate
	if notification {
		gates = append(gates, waitForNotificationGate)
	}
	if timer {
		gates = append(gates, waitForTimerGate)
	}
	if plainTimer {
		gates = append(gates, timerBoundaryEventGate)
	}
	if interruptingTimer {
		gates = append(gates, interruptingTimerBoundaryEventGate)
	}
	return gates
}

// microflowBodyGates are the gates of the activities in a microflow body,
// nested ones included (bodyContains walks branches, loops and error
// handlers): what `create microflow` declares and what `alter microflow`
// splices in. params are the microflow's when the statement declares them.
func microflowBodyGates(params []ast.MicroflowParam, body []ast.MicroflowStatement) []versionGate {
	var gates []versionGate
	// SEND EMAIL writes the 11.13+ shape (StringTemplate subject and
	// bodies); an older project stores a different EmailMessage, or no
	// such activity.
	if bodyContains(body, func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.SendEmailStmt); return ok }) {
		gates = append(gates, versionGate{"microflows", "send_email", "send email",
			"use the Email Connector module, or upgrade the project to 11.13+"})
	}
	if bodyContains(body, selectsWorkflowDefinition) {
		gates = append(gates, workflowDefinitionSelectionGate)
	}
	for _, g := range workflowActionGates {
		if bodyContains(body, g.match) {
			gates = append(gates, g.versionGate)
		}
	}
	return append(gates, flowGates(params, body)...)
}

// nanoflowBodyGates are the gates of the activities in a nanoflow body, as
// microflowBodyGates are a microflow's.
func nanoflowBodyGates(params []ast.MicroflowParam, body []ast.MicroflowStatement) []versionGate {
	var gates []versionGate
	// SYNCHRONIZE UNSYNCHRONIZED is the one mode with a floor inside
	// Mendix 9: SynchronizationType.Unsynchronized was introduced in 9.4.0,
	// while the activity and its other two modes go back further. Gate the
	// mode, not the statement, so a 9.0-9.3 project keeps `synchronize all`.
	if bodyUsesUnsynchronized(body) {
		gates = append(gates, versionGate{"microflows", "synchronize_unsynchronized", "synchronize unsynchronized",
			"use `synchronize all` or `synchronize $Var`, or upgrade the project to 9.4+"})
	}
	return append(gates, flowGates(params, body)...)
}

// typeGate is a gate whose floor is the introduction of the storage $Types
// its statement writes: the version below which the write refuses them
// (canon.UndeclaredTypeError). Its sdk/versions row holds that version, and
// TestTypeGates_FloorIsTheTypeIntroduction pins the row to the types, so
// check -p and the write cannot disagree about a version.
type typeGate struct {
	versionGate
	// types are the storage $Types the statement writes; the floor is the
	// latest of their introductions and of properties'.
	types []string
	// properties are the "Type.Key" properties the statement writes a value
	// to whose introduction is later than the types': the write refuses a
	// value below it (canon.StripUndeclaredProperties), as it refuses a type.
	properties []string
	// match reports a statement in a flow body that writes them.
	match func(ast.MicroflowStatement) bool
}

// workflowActionGates are the gates of the workflow microflow actions Mendix
// added after 9.0 (ako/mxcli#1787), each at its types' introduction in
// mendixmodelsdk 4.117.0. Newest first, so a microflow holding several of
// these is refused naming the one whose version covers the rest. A lock or
// unlock is not here: every one check and exec accept names its workflow,
// and the selection that names it is 10.0, past the action's own 9.21
// (workflowDefinitionSelectionGate).
var workflowActionGates = []typeGate{
	{
		versionGate: versionGate{"workflows", "get_workflow_activity_records", "get workflow activity records",
			"this version has no Microflows$GetWorkflowActivityRecordsAction: upgrade the project"},
		types: []string{"Microflows$GetWorkflowActivityRecordsAction"},
		match: func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.GetWorkflowActivityRecordsStmt); return ok },
	},
	{
		versionGate: versionGate{"workflows", "get_workflows", "get workflows",
			"this version has no Microflows$GetWorkflowsAction: upgrade the project"},
		types: []string{"Microflows$GetWorkflowsAction"},
		match: func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.GetWorkflowsStmt); return ok },
	},
	{
		versionGate: versionGate{"workflows", "notify_workflow", "notify workflow",
			"this version has no Microflows$NotifyWorkflowAction, nor the wait for notification it resumes: upgrade the project"},
		types: []string{"Microflows$NotifyWorkflowAction"},
		match: func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.NotifyWorkflowStmt); return ok },
	},
	workflowOperationGate("retry", "Microflows$RetryOperation", "workflow_operation_retry_unpause", resumeOperationHint),
	workflowOperationGate("unpause", "Microflows$UnpauseOperation", "workflow_operation_retry_unpause", resumeOperationHint),
	{
		versionGate: versionGate{"workflows", "get_workflow_data", "get workflow data",
			"this version has no Microflows$GetWorkflowDataAction: upgrade the project"},
		types: []string{"Microflows$GetWorkflowDataAction"},
		match: func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.GetWorkflowDataStmt); return ok },
	},
	workflowOperationGate("pause", "Microflows$PauseOperation", "workflow_operation_pause_continue",
		"this version has no Microflows$PauseOperation: upgrade the project"),
	workflowOperationGate("continue", "Microflows$ContinueOperation", "workflow_operation_pause_continue",
		"this version has no Microflows$ContinueOperation: upgrade the project"),
	workflowOperationGate("abort", "Microflows$AbortOperation", "workflow_operations",
		"this version has no workflow operation action (Microflows$WorkflowOperationAction): upgrade the project"),
	workflowOperationGate("restart", "Microflows$RestartOperation", "workflow_operations",
		"this version has no workflow operation action (Microflows$WorkflowOperationAction): upgrade the project"),
	workflowMicroflowActionGate("call workflow", "Microflows$WorkflowCallAction",
		func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.CallWorkflowStmt); return ok }),
	workflowMicroflowActionGate("set task outcome", "Microflows$SetTaskOutcomeAction",
		func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.SetTaskOutcomeStmt); return ok }),
	workflowMicroflowActionGate("open user task", "Microflows$OpenUserTaskAction",
		func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.OpenUserTaskStmt); return ok }),
	workflowMicroflowActionGate("open workflow", "Microflows$OpenWorkflowAction",
		func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.OpenWorkflowStmt); return ok }),
}

// resumeOperationHint is the hint of retry and unpause, which replaced one
// Resume operation (introduced 9.3.0, deleted 9.12.0 in mendixmodelsdk 4.117.0).
const resumeOperationHint = "before this version, retry and unpause are one Resume operation (Microflows$ResumeOperation, " +
	"Mendix 9.3 to 9.11), which mxcli does not write: set it in Studio Pro, or upgrade the project"

// workflowOperationGate is the gate of `workflow operation <kind>`, which
// writes a Microflows$WorkflowOperationAction holding operation.
func workflowOperationGate(kind, operation, row, hint string) typeGate {
	return typeGate{
		versionGate: versionGate{"workflows", row, "workflow operation " + kind, hint},
		types:       []string{"Microflows$WorkflowOperationAction", operation},
		match: func(st ast.MicroflowStatement) bool {
			s, ok := st.(*ast.WorkflowOperationStmt)
			return ok && s.OperationType == kind
		},
	}
}

// workflowMicroflowActionGate is the gate of a microflow action Mendix added
// with workflows themselves.
func workflowMicroflowActionGate(statement, typ string, match func(ast.MicroflowStatement) bool) typeGate {
	return typeGate{
		versionGate: versionGate{"workflows", "workflow_microflow_actions", statement,
			"this version has no " + typ + ": the microflow actions arrived with workflows, so upgrade the project"},
		types: []string{typ},
		match: match,
	}
}

// selectsWorkflowDefinition reports a lock or unlock that names its workflow
// (by name or by variable): the action stores it as a
// Workflows$WorkflowDefinition*Selection, a 10.0 type.
func selectsWorkflowDefinition(st ast.MicroflowStatement) bool {
	switch s := st.(type) {
	case *ast.LockWorkflowStmt:
		return s.Workflow != "" || s.WorkflowVariable != ""
	case *ast.UnlockWorkflowStmt:
		return s.Workflow != "" || s.WorkflowVariable != ""
	}
	return false
}

// userTaskGates are the gates of what the user tasks among activities state
// that an older user task cannot store, each once (ako/mxcli#1771). Below 11.2
// a task targets users only, and a multi-user task, its later rules and
// `await all users` arrived in 9.22, 10.0, 10.3 and 10.2 (mendixmodelsdk
// 4.117.0). The write refuses them too (canon.LowerToProjectShape); this names
// the statement before anything is written.
func userTaskGates(activities []ast.WorkflowActivityNode) []versionGate {
	var groups, multi, majority, threshold, microflow, await bool
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		ut, ok := a.(*ast.WorkflowUserTaskNode)
		if !ok {
			return
		}
		switch ut.Targeting.Kind {
		case "group_xpath", "group_microflow":
			groups = true
		}
		if !ut.IsMultiUser {
			return
		}
		multi = true
		await = await || ut.AwaitAllUsers
		if ut.Completion != nil {
			switch ut.Completion.Rule {
			case "majority":
				majority = true
			case "threshold":
				threshold = true
			case "microflow":
				microflow = true
			}
		}
	})
	var gates []versionGate
	if groups {
		gates = append(gates, versionGate{"workflows", "group_targeting", "targeting groups on a user task",
			"Before Mendix 11.2 a user task targets users only: `targeting users xpath […]`, or a microflow " +
				"returning users. Workflow groups arrived in 11.2."})
	}
	if multi {
		gates = append(gates, versionGate{"workflows", "multi_user_task", "multi user task",
			"Mendix added multi-user tasks in 9.22: use a user task, or upgrade the project"})
	}
	for _, r := range []struct {
		used bool
		rule string
	}{{majority, "majority"}, {threshold, "threshold"}} {
		if r.used {
			gates = append(gates, versionGate{"workflows", "multi_user_task_majority_threshold", "decide by " + r.rule,
				"Mendix added the majority and threshold rules in 10.0: decide by consensus or veto, or upgrade the project"})
		}
	}
	if microflow {
		gates = append(gates, versionGate{"workflows", "multi_user_task_microflow_rule", "decide by microflow",
			"Mendix added deciding a multi-user task by a microflow in 10.3: decide by consensus or veto, or upgrade the project"})
	}
	if await {
		gates = append(gates, versionGate{"workflows", "multi_user_task_await_all", "await all users",
			"Mendix added `await all users` in 10.2: leave it out, so the task completes once its rule is met, or upgrade the project"})
	}
	return gates
}

// The floors of a flow call's `returns ( … )` mappings (ako/mxcli#977). The
// builder applies the same two, for the widgets this walk cannot see (a
// fragment's, expanded at build time).
var (
	flowReturnsGate = versionGate{"pages", "flow_call_output_mappings",
		"returns ( … ) on a flow call", "set the values from inside the flow instead"}
	flowReturnsToAttributeGate = versionGate{"pages", "flow_call_output_mappings_to_attribute",
		"returns ( … ) into an attribute", "map into a page variable ($name = …) instead"}
)

// flowReturnsGates are the gates of the `returns ( … )` mappings a page,
// snippet or alter page statement writes: 11.5 for any, 11.15 for one into an
// attribute — mx 11.13 and 11.14 report CE7006 on it.
func flowReturnsGates(stmt ast.Statement) []versionGate {
	var mapped, toAttribute bool
	visit := func(v any) {
		for a, _ := v.(*ast.ActionV3); a != nil; a = a.ThenAction {
			for _, r := range a.Returns {
				mapped = true
				toAttribute = toAttribute || !r.Variable
			}
		}
	}
	forEachWidget(stmt, func(w *ast.WidgetV3, _ string) {
		for _, v := range w.Properties {
			visit(v)
		}
	})
	if s, ok := stmt.(*ast.AlterPageStmt); ok {
		for _, op := range s.Operations {
			if o, ok := op.(*ast.SetPropertyOp); ok {
				for _, v := range o.Properties {
					visit(v)
				}
			}
		}
	}
	var gates []versionGate
	if mapped {
		gates = append(gates, flowReturnsGate)
	}
	if toAttribute {
		gates = append(gates, flowReturnsToAttributeGate)
	}
	return gates
}

// oqlStringAggCallRe is a STRING_AGG call in query text as written.
var oqlStringAggCallRe = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_."` + "`" + `])string_agg\s*\(`)

// oqlUsesStringAgg reports whether a view's query calls STRING_AGG — outside
// comments and string literals, so `'string_agg('` as a label is not a call.
func oqlUsesStringAgg(oql string) bool {
	return oqlStringAggCallRe.MatchString(oqlStringLiteralRe.ReplaceAllString(stripOQLComments(oql), "''"))
}

func exportLevelGate(statement string) versionGate {
	return versionGate{"domain_model", "export_levels", statement,
		"Mendix stores no export level on domain-model elements before 9.3 — remove the clause, or upgrade the project"}
}

func attributesStateExportLevel(attrs []ast.Attribute) bool {
	for _, a := range attrs {
		if a.ExportLevel != nil {
			return true
		}
	}
	return false
}

var (
	agentTaskGate = versionGate{"workflows", "ai_agent_task", "call agent microflow",
		"AI agent tasks need Mendix 11.9 or later — use `call microflow` on older projects"}
	notificationEventsGate = versionGate{"workflows", "notification_events",
		"notification activities and notification boundary events",
		"these need Mendix 11.11 or later — use `wait for notification` on older projects"}
)

// agentDocumentGate is the gate of an agent-editor document. Nothing downstream
// catches an older project: the documents are custom blobs, so mxbuild does not
// validate them and the build stays green while Studio Pro cannot open the
// result.
func agentDocumentGate(name, statement string) versionGate {
	return versionGate{"agent_documents", name, statement,
		"upgrade your project to Mendix 11.9+ and install the AgentEditorCommons module"}
}

// messageDefinitionDocumentGate refuses the per-document message definition
// form below 11.15.
func messageDefinitionDocumentGate(statement string) versionGate {
	return versionGate{"integration", "message_definition_document", statement,
		"below Mendix 11.15 a message definition is an entry of a collection: " +
			"create message definition collection Module.Name { definition Name for Module.Entity { … } };"}
}

// removedStatementError refuses a statement whose document type the project's
// Mendix version no longer has — a ceiling, where statementVersionGates are
// floors. Mendix 11.15 removed the message definition collection (each
// definition is its own document), so every collection statement is refused on
// an 11.15+ project with a hint naming the document form. It lives here, not in
// the handlers, so `check -p` and exec's pre-flight report it too: as a handler
// check it let check pass a script exec then refused (ako/mxcli#2139).
//
// describe is refused as well. Its handler still describes a collection it
// finds before refusing, but an 11.15+ project cannot hold one — `mx convert`
// turns each into a folder of documents — so the two agree on every project
// Studio Pro has opened.
func removedStatementError(ctx *ExecContext, stmt ast.Statement) error {
	var statement string
	switch s := stmt.(type) {
	case *ast.CreateMessageDefinitionCollectionStmt:
		statement = "create message definition collection"
	case *ast.AlterMessageDefinitionCollectionStmt:
		statement = "alter message definition collection"
	case *ast.DropMessageDefinitionCollectionStmt:
		statement = "drop message definition collection"
	case *ast.DescribeStmt:
		if s.ObjectType == ast.DescribeMessageDefinitionCollection {
			statement = "describe message definition collection"
		}
	}
	if statement == "" || !projectStoresMessageDocuments(ctx) {
		return nil
	}
	return messageCollectionsRemovedError(ctx, statement)
}

// requireMessageDefinitionDocuments refuses the per-document form below 11.15.
func requireMessageDefinitionDocuments(ctx *ExecContext, statement string) error {
	return checkGate(ctx, messageDefinitionDocumentGate(statement))
}

func checkGate(ctx *ExecContext, g versionGate) error {
	return checkFeature(ctx, g.area, g.name, g.statement, g.hint)
}

// checkVersionGates refuses stmt when the connected project's version lacks a
// feature it needs: the first of statementVersionGates that fails. Nil when not
// connected, as checkFeature is.
func checkVersionGates(ctx *ExecContext, stmt ast.Statement) error {
	if err := removedStatementError(ctx, stmt); err != nil {
		return err
	}
	for _, g := range statementVersionGates(stmt) {
		if err := checkGate(ctx, g); err != nil {
			return err
		}
	}
	return nil
}

// CheckVersionGates reports every statement of prog the connected project's
// Mendix version cannot store — the refusals exec's handlers would raise when
// they reach it, after the statements before it. `mxcli check -p` and exec's
// pre-flight run it, so both refuse the script before anything is written.
func (e *Executor) CheckVersionGates(prog *ast.Program) []linter.Violation {
	return versionGateViolations(e.newExecContext(context.Background()), prog)
}

func versionGateViolations(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for i, stmt := range prog.Statements {
		if err := checkVersionGates(ctx, stmt); err != nil {
			msg, hint, _ := strings.Cut(err.Error(), "\n  hint: ")
			out = append(out, linter.Violation{
				RuleID:     "MDL-VERSION01",
				Severity:   linter.SeverityError,
				Message:    fmt.Sprintf("statement %d: %s", i+1, msg),
				Suggestion: hint,
			})
		}
	}
	return out
}
