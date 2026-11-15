// SPDX-License-Identifier: Apache-2.0

// Package executor - Workflow SHOW/DESCRIBE commands
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/wfnames"
	"github.com/mendixlabs/mxcli/mdl/doccomment"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// listWorkflows handles SHOW WORKFLOWS command.
func listWorkflows(ctx *ExecContext, moduleName string) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	wfs, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return mdlerrors.NewBackend("list workflows", err)
	}

	type row struct {
		qualifiedName string
		module        string
		name          string
		activities    int
		userTasks     int
		decisions     int
		paramEntity   string
	}
	var rows []row

	for _, wf := range wfs {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}

		qualifiedName := modName + "." + wf.Name
		paramEntity := ""
		if wf.Parameter != nil {
			paramEntity = wf.Parameter.EntityRef
		}

		acts, uts, decs := countWorkflowActivities(wf)

		rows = append(rows, row{qualifiedName, modName, wf.Name, acts, uts, decs, paramEntity})
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Activities", "User Tasks", "Decisions", "Parameter Entity"},
		Summary: fmt.Sprintf("(%d workflows)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.activities, r.userTasks, r.decisions, r.paramEntity})
	}
	return writeResult(ctx, result)
}

// countWorkflowActivities counts total activities, user tasks, and decisions in
// a workflow, over every flow it holds — the catalog's walk (wfnames).
func countWorkflowActivities(wf *workflows.Workflow) (total, userTasks, decisions int) {
	c := wfnames.CountActivities(wf)
	return c.Total, c.UserTasks, c.Decisions
}

// describeWorkflow handles DESCRIBE WORKFLOW command.
func describeWorkflow(ctx *ExecContext, name ast.QualifiedName) error {
	output, _, err := describeWorkflowToString(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Output, output)
	return nil
}

// describeWorkflowToString generates MDL-like output for a workflow and returns it as a string.
func describeWorkflowToString(ctx *ExecContext, name ast.QualifiedName) (string, map[string]elkSourceRange, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return "", nil, mdlerrors.NewBackend("build hierarchy", err)
	}

	allWorkflows, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return "", nil, mdlerrors.NewBackend("list workflows", err)
	}

	var targetWf *workflows.Workflow
	for _, wf := range allWorkflows {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && wf.Name == name.Name {
			targetWf = wf
			break
		}
	}

	if targetWf == nil {
		return "", nil, mdlerrors.NewNotFound("workflow", name.String())
	}

	var lines []string
	qualifiedName := name.Module + "." + name.Name

	// Documentation
	if targetWf.Documentation != "" {
		lines = append(lines, "/**")
		for docLine := range strings.SplitSeq(doccomment.Escape(targetWf.Documentation), "\n") {
			lines = append(lines, " * "+docLine)
		}
		lines = append(lines, " */")
	}

	// Header
	lines = append(lines, fmt.Sprintf("-- Workflow: %s", qualifiedName))
	lines = append(lines, "")

	// `create or modify`: the rewrite carries what describe cannot print (the names
	// of the implicit activities, empty outcome flows, event sub-processes), proven
	// on ako/TestApp's workflows by the round-trip harness (#743).
	lines = append(lines, fmt.Sprintf("create or modify workflow %s", qualifiedName))
	if clause := describeFolderClause(ctx, targetWf.ContainerID); clause != "" {
		lines = append(lines, "  "+strings.TrimSpace(clause))
	}

	// Context parameter
	if targetWf.Parameter != nil && targetWf.Parameter.EntityRef != "" {
		lines = append(lines, fmt.Sprintf("  parameter $WorkflowContext: %s", targetWf.Parameter.EntityRef))
	}

	// Display name
	if targetWf.WorkflowName != "" || len(targetWf.WorkflowNameParams) > 0 {
		lines = append(lines, "  display "+workflowTemplateMDL(targetWf.WorkflowName, targetWf.WorkflowNameParams))
	}

	// Description
	if targetWf.WorkflowDescription != "" || len(targetWf.WorkflowDescriptionParams) > 0 {
		lines = append(lines, "  description "+workflowTemplateMDL(targetWf.WorkflowDescription, targetWf.WorkflowDescriptionParams))
	}

	// Export level (only emit when non-empty)
	if targetWf.ExportLevel != "" {
		lines = append(lines, fmt.Sprintf("  export level %s", targetWf.ExportLevel))
	}

	// Overview page
	if targetWf.OverviewPage != "" {
		lines = append(lines, fmt.Sprintf("  overview page %s", targetWf.OverviewPage))
	}

	// Due date
	if targetWf.DueDate != "" {
		lines = append(lines, fmt.Sprintf("  due date %s", workflowExpressionMDL(targetWf.DueDate)))
	}

	// The note attached to the workflow's start. It was a `--` comment, which
	// a replay dropped (ako/mxcli#707); it is the start activity's, as Studio
	// Pro shows it (ako/mxcli#1717).
	if note := targetWf.StartNote(); note != "" {
		lines = append(lines, fmt.Sprintf("  annotation %s", mdlQuoted(note)))
	}

	lines = append(lines, formatWorkflowEventHandlers(ctx, targetWf.EventHandlers)...)

	lines = append(lines, "")

	lines = append(lines, "begin")
	// Activities
	if targetWf.Flow != nil {
		actLines := formatMainFlowActivities(ctx, targetWf.Flow, "  ")
		lines = append(lines, actLines...)
	}
	lines = append(lines, formatEventSubProcesses(ctx, targetWf.EventSubProcesses, "  ")...)

	lines = append(lines, "end workflow;")

	return strings.Join(lines, "\n"), nil, nil
}

// formatAnnotation renders the note attached to an activity as the
// `@annotation '…'` line before it, the form a microflow activity's note takes.
//
// It was a `-- annotation:` comment, because MDLWorkflow.g4 had no attached
// form: the only workflow `annotation` was the standalone statement, which
// MDL-WF04 and exec refuse (mendixlabs/mxcli#1007). A comment kept the text in
// front of the reader, but a replay dropped it (ako/mxcli#707). A standalone
// note read back from the model is still a comment: see annotationComment.
func formatAnnotation(annotation string, indent string) string {
	if annotation == "" {
		return ""
	}
	return indent + "@annotation " + mdlQuoted(annotation)
}

// annotationComment renders text as one or more `-- annotation:` lines. An
// annotation may contain newlines, and a `--` comment runs to end of line, so a
// multi-line note has to be prefixed line by line or everything after the first
// newline becomes stray tokens — the same failure the statement form had.
func annotationComment(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		l = strings.TrimRight(l, "\r")
		if i == 0 {
			lines[i] = indent + "-- annotation: " + l
			continue
		}
		lines[i] = indent + "--   " + l
	}
	return strings.Join(lines, "\n")
}

// boundaryEventKeyword maps an EventType string to the MDL BOUNDARY EVENT keyword sequence.
func boundaryEventKeyword(eventType string) string {
	switch eventType {
	case "InterruptingTimer":
		return "boundary event interrupting timer"
	case "NonInterruptingTimer":
		return "boundary event non interrupting timer"
	case "InterruptingNotification":
		return "boundary event interrupting notification"
	case "NonInterruptingNotification":
		return "boundary event non interrupting notification"
	default:
		return "boundary event timer"
	}
}

// formatBoundaryEvents formats boundary events for describe output.
func formatBoundaryEvents(ctx *ExecContext, events []*workflows.BoundaryEvent, indent string) []string {
	if len(events) == 0 {
		return nil
	}

	var lines []string
	for _, event := range events {
		keyword := boundaryEventKeyword(event.EventType)
		if event.IsNotification() {
			// Its name is what `notify workflow … target` names, so it is always
			// emitted; the string is the caption.
			header := indent + keyword
			if event.Name != "" {
				header += " " + mdlIdent(event.Name)
			}
			if event.Caption != "" {
				header += " " + mdlQuoted(event.Caption)
			}
			lines = append(lines, header)
		} else {
			header := indent + keyword
			if event.TimerDelay != "" {
				header += " " + workflowExpressionMDL(event.TimerDelay)
			}
			if event.Recurrence != nil {
				header += " " + recurrenceMDL(event.Recurrence)
			}
			if event.Caption != "" {
				header += " caption " + mdlQuoted(event.Caption)
			}
			lines = append(lines, header)
		}
		if event.Flow != nil && len(event.Flow.Activities) > 0 {
			lines = append(lines, fmt.Sprintf("%s{", indent))
			subLines := formatWorkflowActivities(ctx, event.Flow, indent+"  ")
			lines = append(lines, subLines...)
			lines = append(lines, fmt.Sprintf("%s}", indent))
		}
	}

	return lines
}

// formatEventSubProcesses emits each event sub-process as a block after the main
// body. Its flow's End is implicit, like the main flow's — the builder appends
// one when the body does not already end — so it is formatted as a main flow.
func formatEventSubProcesses(ctx *ExecContext, esps []*workflows.EventSubProcess, indent string) []string {
	var lines []string
	for _, esp := range esps {
		start := esp.Start()
		if start == nil {
			// Nothing MDL can state starts it; the rewrite guard refuses to drop it.
			lines = append(lines, fmt.Sprintf("%s-- event subprocess %s has no start event, which MDL cannot state", indent, esp.Name), "")
			continue
		}
		header := indent + "event subprocess " + mdlIdent(esp.Name)
		if esp.Caption != "" {
			header += " " + mdlQuoted(esp.Caption)
		}
		trigger := "non interrupting"
		if start.Interrupting {
			trigger = "interrupting"
		}
		if start.Timer {
			header += fmt.Sprintf(" on %s timer %s", trigger, workflowExpressionMDL(start.FirstExecutionTime))
			if start.Recurrence != nil {
				header += " " + recurrenceMDL(start.Recurrence)
			}
			if start.Name != "" {
				header += " as " + mdlIdent(start.Name)
			}
			if start.Caption != "" {
				header += " caption " + mdlQuoted(start.Caption)
			}
		} else {
			header += fmt.Sprintf(" on %s notification", trigger)
			if start.Name != "" {
				header += " " + mdlIdent(start.Name)
			}
			if start.Caption != "" {
				header += " " + mdlQuoted(start.Caption)
			}
		}
		if esp.Annotation != "" {
			lines = append(lines, formatAnnotation(esp.Annotation, indent))
		}
		lines = append(lines, header+" {")
		lines = append(lines, formatMainFlowActivities(ctx, esp.Flow, indent+"  ")...)
		lines = append(lines, indent+"};", "")
	}
	return lines
}

// formatWorkflowActivities generates MDL-like output for workflow activities.
// formatWorkflowActivities formats a nested flow — an outcome, a branch, a path,
// a boundary-event path — where an End is an `end workflow` the author wrote.
func formatWorkflowActivities(ctx *ExecContext, flow *workflows.Flow, indent string) []string {
	return formatFlowActivities(ctx, flow, indent, false)
}

// formatMainFlowActivities formats the top-level flow, whose Start and End are
// implicit: the body's own `begin` and `end workflow` stand for them.
func formatMainFlowActivities(ctx *ExecContext, flow *workflows.Flow, indent string) []string {
	return formatFlowActivities(ctx, flow, indent, true)
}

func formatFlowActivities(ctx *ExecContext, flow *workflows.Flow, indent string, mainFlow bool) []string {
	if flow == nil {
		return nil
	}

	var lines []string
	for _, act := range flow.Activities {
		var actLines []string
		isComment := false
		switch a := act.(type) {
		case *workflows.UserTask:
			actLines = formatUserTask(ctx, a, indent)
		case *workflows.CallMicroflowTask:
			actLines = formatCallMicroflowTask(ctx, a, indent)
		case *workflows.SystemTask:
			actLines = formatSystemTask(ctx, a, indent)
		case *workflows.CallWorkflowActivity:
			actLines = formatCallWorkflowActivity(ctx, a, indent)
		case *workflows.ExclusiveSplitActivity:
			actLines = formatExclusiveSplit(ctx, a, indent)
		case *workflows.ParallelSplitActivity:
			actLines = formatParallelSplit(ctx, a, indent)
		case *workflows.JumpToActivity:
			target := a.TargetActivity
			if target == "" {
				target = "?"
			}
			if a.Annotation != "" {
				actLines = append(actLines, formatAnnotation(a.Annotation, indent))
			}
			// Only emit `caption '...'` when it carries information the author
			// wrote. buildJumpTo defaults Caption to the target name, so echoing it
			// unconditionally rendered a plain `jump to Triage;` as
			// `jump to Triage caption 'Triage'` — a phantom comment nobody authored
			// (issuetracker #16). Re-applying the shorter form rebuilds the same
			// Caption, so dropping it is lossless.
			if caption := a.Caption; caption != "" && caption != target && caption != a.Name {
				actLines = append(actLines, fmt.Sprintf("%sjump to %s caption %s", indent, mdlIdent(target), mdlQuoted(caption)))
			} else {
				actLines = append(actLines, fmt.Sprintf("%sjump to %s", indent, mdlIdent(target)))
			}
		case *workflows.WaitForTimerActivity:
			caption := a.Caption
			if caption == "" {
				caption = a.Name
			}
			if a.Annotation != "" {
				actLines = append(actLines, formatAnnotation(a.Annotation, indent))
			}
			nameClause := workflowActivityNameClause(a.Name, caption)
			if a.DelayExpression != "" {
				var delay string
				nameClause, delay = namedWorkflowExpressionMDL(a.Name, nameClause, a.DelayExpression, visitor.WorkflowWaitForTimerReadsBack)
				actLines = append(actLines, fmt.Sprintf("%swait for timer%s %s caption %s", indent, nameClause, delay, mdlQuoted(caption)))
			} else {
				actLines = append(actLines, fmt.Sprintf("%swait for timer%s caption %s", indent, nameClause, mdlQuoted(caption)))
			}
		case *workflows.WaitForNotificationActivity:
			caption := a.Caption
			if caption == "" {
				caption = a.Name
			}
			if a.Annotation != "" {
				actLines = append(actLines, formatAnnotation(a.Annotation, indent))
			}
			actLines = append(actLines, fmt.Sprintf("%swait for notification%s -- %s", indent,
				workflowActivityNameClause(a.Name, caption), caption))
			// BoundaryEvents
			actLines = append(actLines, formatBoundaryEvents(ctx, a.BoundaryEvents, indent+"  ")...)
		case *workflows.NotificationActivity:
			if a.Annotation != "" {
				actLines = append(actLines, formatAnnotation(a.Annotation, indent))
			}
			line := indent + "notification"
			if a.Name != "" {
				line += " " + mdlIdent(a.Name)
			}
			if a.Caption != "" {
				line += " caption " + mdlQuoted(a.Caption)
			}
			actLines = append(actLines, line)
		case *workflows.StartWorkflowActivity:
			// Skip start activities - they are implicit
			continue
		case *workflows.EventSubProcessStartActivity:
			// Stated in the `event subprocess … on …` header.
			continue
		case *workflows.EndWorkflowActivity:
			if mainFlow {
				// The main flow's End is implicit: `end workflow` closes the body.
				continue
			}
			// An End inside a branch ends the workflow early. This used to be
			// skipped like the main one, which described Studio Pro's
			// `Reject -> End` as `'Reject' { }` — and that, re-executed, falls
			// through into the main flow.
			if a.Caption != "" && a.Caption != "End" {
				actLines = []string{fmt.Sprintf("%send workflow caption %s", indent, mdlQuoted(a.Caption))}
			} else {
				actLines = []string{indent + "end workflow"}
			}
		case *workflows.EndOfParallelSplitPathActivity:
			// Skip - auto-generated by Mendix, implicit in MDL syntax
			continue
		case *workflows.EndOfBoundaryEventPathActivity:
			// Skip - auto-generated by Mendix, implicit in MDL syntax
			continue
		case *workflows.WorkflowAnnotationActivity:
			// A standalone annotation (sticky note) read back from the model. Emitted
			// as a comment for the same reason as an attached one: the `annotation`
			// statement it used to produce is refused by MDL-WF04 and by exec, so the
			// describe output could not be re-run (mendixlabs/mxcli#1007).
			if a.Description == "" {
				continue
			}
			isComment = true
			actLines = []string{annotationComment(a.Description, indent)}
		case *workflows.GenericWorkflowActivity:
			isComment = true
			caption := a.Caption
			if caption == "" {
				caption = a.Name
			}
			actLines = []string{fmt.Sprintf("%s-- [%s] %s", indent, a.TypeString, caption)}
		default:
			isComment = true
			actLines = []string{fmt.Sprintf("%s-- [unknown activity]", indent)}
		}
		// Append semicolon to last line of activity (not for comments)
		// Insert before any -- comment to avoid the comment swallowing the semicolon
		if !isComment && len(actLines) > 0 {
			lastLine := actLines[len(actLines)-1]
			if idx := strings.Index(lastLine, " -- "); idx >= 0 {
				actLines[len(actLines)-1] = lastLine[:idx] + ";" + lastLine[idx:]
			} else {
				actLines[len(actLines)-1] = lastLine + ";"
			}
		}
		lines = append(lines, actLines...)
		lines = append(lines, "")
	}

	return lines
}

// formatUserTask formats a user task for describe output.
func formatUserTask(ctx *ExecContext, a *workflows.UserTask, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}
	nameStr := a.Name
	if nameStr == "" {
		nameStr = "unnamed"
	}

	taskKeyword := "user task"
	if a.IsMulti {
		taskKeyword = "multi user task"
	}
	lines = append(lines, fmt.Sprintf("%s%s %s %s", indent, taskKeyword, mdlIdent(nameStr), mdlQuoted(caption)))

	if a.Page != "" {
		lines = append(lines, fmt.Sprintf("%s  page %s", indent, a.Page))
	}

	// User targeting
	if a.UserSource != nil {
		switch us := a.UserSource.(type) {
		case *workflows.MicroflowBasedUserSource:
			if us.Microflow != "" {
				lines = append(lines, fmt.Sprintf("%s  targeting users microflow %s", indent, us.Microflow))
			}
		case *workflows.XPathBasedUserSource:
			if us.XPath != "" {
				lines = append(lines, fmt.Sprintf("%s  targeting users xpath %s", indent, targetingXPathMDL(ctx, us.XPath)))
			}
		case *workflows.MicroflowGroupSource:
			if us.Microflow != "" {
				lines = append(lines, fmt.Sprintf("%s  targeting groups microflow %s", indent, us.Microflow))
			}
		case *workflows.XPathGroupSource:
			if us.XPath != "" {
				lines = append(lines, fmt.Sprintf("%s  targeting groups xpath %s", indent, targetingXPathMDL(ctx, us.XPath)))
			}
		}
	}
	// Off is what a task gets without the clause, so only on is printed.
	if a.AutoAssignSingleTargetUser {
		lines = append(lines, indent+"  auto assign single user")
	}

	if a.OnCreated != "" {
		lines = append(lines, fmt.Sprintf("%s  on created microflow %s", indent, a.OnCreated))
	}

	if a.UserTaskEntity != "" {
		lines = append(lines, fmt.Sprintf("%s  entity %s", indent, a.UserTaskEntity))
	}

	// Due date (task-level)
	if a.DueDate != "" {
		lines = append(lines, fmt.Sprintf("%s  due date %s", indent, workflowExpressionMDL(a.DueDate)))
	}

	// Task name: printed only when it is not what a task without the clause
	// gets, its caption (ako/mxcli#1714).
	if (a.TaskName != "" && a.TaskName != caption) || len(a.TaskNameParams) > 0 {
		lines = append(lines, indent+"  task name "+workflowTemplateMDL(a.TaskName, a.TaskNameParams))
	}

	// Task description
	if a.TaskDescription != "" || len(a.TaskDescriptionParams) > 0 {
		lines = append(lines, indent+"  description "+workflowTemplateMDL(a.TaskDescription, a.TaskDescriptionParams))
	}

	if a.IsMulti {
		lines = append(lines, formatMultiUserTaskCompletion(ctx, a, indent)...)
	}

	// Outcomes
	if len(a.Outcomes) > 0 {
		lines = append(lines, fmt.Sprintf("%s  outcomes", indent))
		for _, outcome := range a.Outcomes {
			outValue := outcome.Value
			if outValue == "" {
				outValue = outcome.Caption
			}
			if outValue == "" {
				outValue = outcome.Name
			}
			if outcome.Flow != nil && len(outcome.Flow.Activities) > 0 {
				lines = append(lines, fmt.Sprintf("%s    %s {", indent, mdlQuoted(outValue)))
				subLines := formatWorkflowActivities(ctx, outcome.Flow, indent+"      ")
				lines = append(lines, subLines...)
				lines = append(lines, fmt.Sprintf("%s    }", indent))
			} else {
				lines = append(lines, fmt.Sprintf("%s    %s { }", indent, mdlQuoted(outValue)))
			}
		}
	}

	// BoundaryEvents
	lines = append(lines, formatBoundaryEvents(ctx, a.BoundaryEvents, indent+"  ")...)

	return lines
}

// formatWorkflowEventHandlers emits the header's handler clauses. A handler
// subscribed to exactly the types the project version knows is written as
// `on any workflow event`, which is what re-executing it would store again; any
// other list is written out, one type per line when it is long.
func formatWorkflowEventHandlers(ctx *ExecContext, handlers []*workflows.WorkflowEventHandler) []string {
	var all []string
	if pv := ctx.Backend.ProjectVersion(); pv != nil {
		all, _, _ = allWorkflowEventTypes(pv.MajorVersion, pv.MinorVersion, pv.PatchVersion)
	}
	var lines []string
	for _, h := range handlers {
		if h == nil {
			continue
		}
		as := ""
		if h.Description != "" {
			as = " as " + mdlQuoted(h.Description)
		}
		switch {
		case len(h.EventTypes) == 0:
			// The grammar has no spelling for an empty list; say so rather than
			// emit a clause that means something else. Rewrites refuse it.
			lines = append(lines, fmt.Sprintf("  -- workflow event handler %s (microflow %s) subscribes to no event types, which MDL cannot state",
				mdlQuoted(h.Description), h.Microflow))
		case len(all) > 0 && sameStringSet(h.EventTypes, all):
			lines = append(lines, fmt.Sprintf("  on any workflow event microflow %s%s", h.Microflow, as))
		case len(h.EventTypes) <= 3:
			lines = append(lines, fmt.Sprintf("  on workflow events (%s) microflow %s%s", strings.Join(h.EventTypes, ", "), h.Microflow, as))
		default:
			lines = append(lines, "  on workflow events (")
			for i, t := range h.EventTypes {
				sep := ","
				if i == len(h.EventTypes)-1 {
					sep = ""
				}
				lines = append(lines, "    "+t+sep)
			}
			lines = append(lines, fmt.Sprintf("  ) microflow %s%s", h.Microflow, as))
		}
	}
	return lines
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if !set[s] {
			return false
		}
	}
	return len(set) == len(b)
}

// formatMultiUserTaskCompletion emits a multi-user task's `participants`,
// `decide by` and `await all users` clauses, in grammar order. What a rebuild
// writes anyway — all participants, consensus falling back to the first outcome,
// not waiting — is omitted, so a task that never had them describes as before.
func formatMultiUserTaskCompletion(ctx *ExecContext, a *workflows.UserTask, indent string) []string {
	var lines []string
	if t := a.TargetUserInput; t != nil {
		switch t.Kind {
		case "Absolute":
			lines = append(lines, fmt.Sprintf("%s  participants %d", indent, t.Amount))
		case "Percentage":
			lines = append(lines, fmt.Sprintf("%s  participants %d percent", indent, t.Percentage))
		}
	}
	if cc := a.CompletionCriteria; cc != nil {
		fallback := ""
		if cc.FallbackOutcome != "" {
			fallback = " fallback " + mdlQuoted(cc.FallbackOutcome)
		}
		firstOutcome := ""
		if len(a.Outcomes) > 0 {
			firstOutcome = a.Outcomes[0].Value
			if firstOutcome == "" {
				firstOutcome = a.Outcomes[0].Caption
			}
		}
		switch cc.Kind {
		case "Consensus":
			if cc.FallbackOutcome != firstOutcome || firstOutcome == "" {
				lines = append(lines, fmt.Sprintf("%s  decide by consensus%s", indent, fallback))
			}
		case "Majority":
			rule := "most chosen"
			if cc.CompletionType == "Absolute" {
				rule = "more than half"
			}
			lines = append(lines, fmt.Sprintf("%s  decide by majority %s%s", indent, rule, fallback))
		case "Threshold":
			unit := "votes"
			if cc.CompletionType == "Relative" {
				unit = "percent"
			}
			lines = append(lines, fmt.Sprintf("%s  decide by threshold %d %s%s", indent, cc.Threshold, unit, fallback))
		case "Veto":
			lines = append(lines, fmt.Sprintf("%s  decide by veto %s", indent, mdlQuoted(cc.VetoOutcome)))
		case "Microflow":
			lines = append(lines, fmt.Sprintf("%s  decide by microflow %s", indent, cc.Microflow))
		}
	}
	if a.AwaitAllUsers {
		lines = append(lines, indent+"  await all users")
	}
	return lines
}

// formatCallMicroflowTask formats a call microflow task for describe output.
func formatCallMicroflowTask(ctx *ExecContext, a *workflows.CallMicroflowTask, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}

	mf := a.Microflow
	if mf == "" {
		mf = "?"
	}

	verb := "call microflow"
	if a.IsAgent {
		verb = "call agent microflow"
	}
	// A caption the author set is emitted as `caption '…'`, which the grammar
	// reads back into the caption. It used to be emitted only as a trailing
	// `-- caption` comment, so describe → exec replaced it with the microflow's
	// name. The derived default (the microflow's short name) carries nothing and
	// stays a plain comment, as for jump and wait activities.
	asAndComment := workflowActivityAsClause(a.Name, shortDocName(mf))
	trailing := " -- " + caption
	if a.Caption != "" && a.Caption != shortDocName(mf) {
		asAndComment += " caption " + mdlQuoted(a.Caption)
		trailing = ""
	}
	args, legacy := workflowCallArguments(a.ParameterMappings)
	lines = append(lines, fmt.Sprintf("%s%s %s%s%s%s%s", indent, verb, mf, args, asAndComment, legacy, trailing))

	// Outcomes, then boundary events — the order the grammar requires
	// (workflowCallMicroflowStmt: … OUTCOMES? BOUNDARY EVENT?). Emitting them
	// the other way round produced DESCRIBE output that would not re-parse:
	// "mismatched input 'outcomes' expecting ';'" (issue #948). It only showed
	// once the default engine could read boundary events back at all.
	lines = append(lines, formatConditionOutcomes(ctx, a.Outcomes, indent)...)
	lines = append(lines, formatBoundaryEvents(ctx, a.BoundaryEvents, indent+"  ")...)

	return lines
}

// formatSystemTask formats a system task for describe output.
func formatSystemTask(ctx *ExecContext, a *workflows.SystemTask, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}

	mf := a.Microflow
	if mf == "" {
		mf = "?"
	}

	lines = append(lines, fmt.Sprintf("%scall microflow %s%s -- %s", indent, mf,
		workflowActivityAsClause(a.Name, shortDocName(mf)), caption))

	// Outcomes
	lines = append(lines, formatConditionOutcomes(ctx, a.Outcomes, indent)...)

	return lines
}

// formatCallWorkflowActivity formats a call workflow activity for describe output.
func formatCallWorkflowActivity(ctx *ExecContext, a *workflows.CallWorkflowActivity, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}

	wf := a.Workflow
	if wf == "" {
		wf = "?"
	}

	args, legacy := workflowCallArguments(a.ParameterMappings)
	lines = append(lines, fmt.Sprintf("%scall workflow %s%s%s caption %s%s", indent, wf, args,
		workflowActivityAsClause(a.Name, shortDocName(wf)), mdlQuoted(caption), legacy))

	// BoundaryEvents
	lines = append(lines, formatBoundaryEvents(ctx, a.BoundaryEvents, indent+"  ")...)

	return lines
}

// workflowActivityNameClause renders an activity's explicit name for describe
// output, or "" when the name is what the builder would derive from the caption
// anyway. Mendix resolves `jump to` by JumpToActivity.TargetActivity, which
// stores an activity NAME, and Studio Pro names every activity by type and
// ordinal (decision1, split1) independently of its caption — so without this the
// described workflow's jump wiring did not survive re-execution (ako/mxcli#408).
// Derived names are left off so mxcli-authored workflows describe unchanged.
func workflowActivityNameClause(name, caption string) string {
	if name == "" || name == caption || name == sanitizeActivityName(caption) {
		return ""
	}
	return " " + mdlIdent(name)
}

// shortDocName returns the document name of a qualified name.
func shortDocName(qn string) string {
	if i := strings.LastIndex(qn, "."); i >= 0 {
		return qn[i+1:]
	}
	return qn
}

// workflowActivityAsClause renders an `as <name>` clause for a call activity,
// whose name is otherwise derived from the document it calls. Studio Pro names
// these callMicroflow1 / callWorkflow1, so the derived name is almost never the
// stored one. See workflowActivityNameClause.
func workflowActivityAsClause(name, derived string) string {
	if name == "" || name == derived || name == sanitizeActivityName(derived) {
		return ""
	}
	return " as " + mdlIdent(name)
}

// workflowCaptionClauses renders the name slot and the caption of an activity
// whose writer defaults the caption to defaultCaption and the name to the
// caption (decision, parallel split).
//
// A caption the author set is emitted as `caption '…'`, which the grammar reads
// back into the caption. It used to be emitted only as a trailing `-- caption`
// comment, so describe → exec replaced it with the default (ako/mxcli#707). The
// default carries nothing and stays a plain trailing comment, as for call
// activities. The name clause is computed against the caption the writer will
// actually store, since that is what an omitted name falls back to.
func workflowCaptionClauses(name, caption, defaultCaption string) (nameClause, captionClause string) {
	written := defaultCaption
	if caption != "" && caption != defaultCaption {
		written = caption
		captionClause = " caption " + mdlQuoted(caption)
	} else {
		shown := caption
		if shown == "" {
			shown = name
		}
		captionClause = " -- " + shown
	}
	return workflowActivityNameClause(name, written), captionClause
}

// formatExclusiveSplit formats an exclusive split (decision) for describe output.
func formatExclusiveSplit(ctx *ExecContext, a *workflows.ExclusiveSplitActivity, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	nameClause, captionClause := workflowCaptionClauses(a.Name, a.Caption, "Decision")
	if a.Expression != "" {
		var expr string
		nameClause, expr = namedWorkflowExpressionMDL(a.Name, nameClause, a.Expression, visitor.WorkflowDecisionReadsBack)
		lines = append(lines, fmt.Sprintf("%sdecision%s %s%s", indent, nameClause, expr, captionClause))
	} else {
		lines = append(lines, fmt.Sprintf("%sdecision%s%s", indent, nameClause, captionClause))
	}

	lines = append(lines, formatConditionOutcomes(ctx, a.Outcomes, indent)...)

	return lines
}

// formatParallelSplit formats a parallel split for describe output.
func formatParallelSplit(ctx *ExecContext, a *workflows.ParallelSplitActivity, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	nameClause, captionClause := workflowCaptionClauses(a.Name, a.Caption, "Parallel split")
	lines = append(lines, fmt.Sprintf("%sparallel split%s%s", indent, nameClause, captionClause))
	for i, outcome := range a.Outcomes {
		lines = append(lines, fmt.Sprintf("%s  path %d {", indent, i+1))
		if outcome.Flow != nil && len(outcome.Flow.Activities) > 0 {
			subLines := formatWorkflowActivities(ctx, outcome.Flow, indent+"    ")
			lines = append(lines, subLines...)
		}
		lines = append(lines, fmt.Sprintf("%s  }", indent))
	}

	return lines
}

// formatConditionOutcomes formats condition outcomes for describe output.
func formatConditionOutcomes(ctx *ExecContext, outcomes []workflows.ConditionOutcome, indent string) []string {
	if len(outcomes) == 0 {
		return nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s  outcomes", indent))
	for _, outcome := range outcomes {
		name := outcome.GetName()
		if _, ok := outcome.(*workflows.VoidConditionOutcome); ok {
			name = "default" // a keyword: lowercase is canonical (R8)
		}
		flow := outcome.GetFlow()
		if flow != nil && len(flow.Activities) > 0 {
			lines = append(lines, fmt.Sprintf("%s    %s -> {", indent, name))
			subLines := formatWorkflowActivities(ctx, flow, indent+"      ")
			lines = append(lines, subLines...)
			lines = append(lines, fmt.Sprintf("%s    }", indent))
		} else {
			lines = append(lines, fmt.Sprintf("%s    %s -> { }", indent, name))
		}
	}

	return lines
}

// targetingXPathMDL is a user task's targeting XPath as written after `xpath`:
// in [ ] exactly as stored (R5, ako/mxcli#753), so no quote inside it is
// doubled. A stored value the bracketed grammar does not read is written in the
// deprecated quoted form, which carries any string, so the output stays
// re-executable.
//
// A string in it is spelled for the describe language (describeXPath,
// ako/mxcli#825), as is the quoted form (mdlQuote).
func targetingXPathMDL(ctx *ExecContext, xpath string) string {
	if visitor.IsBracketedXPath(xpath) {
		return describeXPath(ctx, xpath)
	}
	return mdlQuote(ctx, xpath)
}

// workflowExpressionMDL writes a stored workflow expression — a due date, a
// timer's delay or first execution time — bare (R5, ako/mxcli#753). One that
// does not read back as itself when written bare (surrounding whitespace, text
// the MDL expression grammar cannot parse) is written in the deprecated string
// form, which carries it byte for byte: describe must never alter a stored
// expression.
// recurrenceMDL is a timer's recurrence as MDL writes it (ako/mxcli#1716).
func recurrenceMDL(r *workflows.Recurrence) string {
	return fmt.Sprintf("repeat every %d %s at most %d times", r.Interval, workflows.RecurrenceUnitWord(r.IntervalType, r.Interval), r.MaxExecutions)
}

// workflowTemplateMDL is a workflow text as MDL writes it: the quoted text, then
// `with ({1} = …, …)` when expressions fill its placeholders (ako/mxcli#1714).
func workflowTemplateMDL(text string, params []string) string {
	out := mdlQuoted(text)
	if len(params) == 0 {
		return out
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = fmt.Sprintf("{%d} = %s", i+1, p)
	}
	return out + " with (" + strings.Join(parts, ", ") + ")"
}

func workflowExpressionMDL(expr string) string {
	if visitor.BareExpression(expr) {
		return expr
	}
	return mdlQuoted(expr)
}

// namedWorkflowExpressionMDL is workflowExpressionMDL for the expression of a
// decision or `wait for timer`, which follows the activity's optional name: an
// expression that starts with a word could be read as the name. When it
// would, the name is written even where the writer would derive it, which
// stores the same name; when even that does not read back, the string form.
func namedWorkflowExpressionMDL(name, nameClause, expr string, readsBack func(nameClause, expr string) bool) (string, string) {
	if !visitor.BareExpression(expr) {
		return nameClause, mdlQuoted(expr)
	}
	if readsBack(nameClause, expr) {
		return nameClause, expr
	}
	if nameClause == "" && name != "" {
		if forced := " " + mdlIdent(name); readsBack(forced, expr) {
			return forced, expr
		}
	}
	return nameClause, mdlQuoted(expr)
}

// workflowCallArguments renders a workflow call's parameter mappings in R4's
// argument form, `(Param = expression)` right after the callee
// (ako/mxcli#751).
//
// A stored expression that does not read back as itself when written bare —
// one with surrounding whitespace, or text the MDL expression grammar cannot
// parse — is rendered in the deprecated string form instead, returned as
// legacy (` with (Param = '<expression>')`, written after the comment): the
// string carries it byte for byte, and describe must never alter a stored
// expression. All mappings take the same form, since a call has one list.
func workflowCallArguments(mappings []*workflows.ParameterMapping) (args, legacy string) {
	if len(mappings) == 0 {
		return "", ""
	}
	bare := true
	for _, pm := range mappings {
		if !visitor.BareExpression(pm.Expression) {
			bare = false
			break
		}
	}
	parts := make([]string, 0, len(mappings))
	for _, pm := range mappings {
		name := pm.Parameter
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		if bare {
			parts = append(parts, visitor.ParameterNameSpelling(name)+" = "+pm.Expression)
		} else {
			parts = append(parts, name+" = "+mdlQuoted(pm.Expression))
		}
	}
	if bare {
		return "(" + strings.Join(parts, ", ") + ")", ""
	}
	return "", " with (" + strings.Join(parts, ", ") + ")"
}
