// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// checkNoDroppedWorkflowConstructs refuses a CREATE OR REPLACE/MODIFY WORKFLOW
// that would delete a stored construct the statement does not restate.
//
// A rewrite rebuilds the workflow from the statement, so anything the script
// does not mention is gone, and nothing signals the loss afterwards: measured on
// the v1 fixture, a stored interrupting timer boundary event and its whole
// handler flow went 1 -> 0 while exec reported "Created workflow" and exit 0
// (issue #948). That is the same shape as a dropped queue binding
// (checkNoQueuedCalls) — guard-don't-drop, ADR-0005.
//
// Boundary events ARE authorable (`boundary event interrupting timer '…' { … }`),
// so a script that restates them is allowed straight through — that is the normal
// way to edit a workflow that has one. So are event sub-processes and
// notification activities now; only a sub-process with no start event, which MDL
// has no way to state, refuses the rewrite outright.
//
// The stored side is read from the raw unit rather than through the semantic
// model deliberately: the reader is what was blind here in the first place, and a
// guard that shares the reader's blind spot cannot see what it is meant to
// protect. Reading the BSON also covers constructs no engine models yet.
func checkNoDroppedWorkflowConstructs(ctx *ExecContext, workflowID model.ID, qualifiedName string, stmt *ast.CreateWorkflowStmt) error {
	if ctx == nil || ctx.Backend == nil || workflowID == "" {
		return nil
	}
	raw, err := ctx.Backend.GetRawUnit(workflowID)
	if err != nil {
		// An unreadable stored unit is not this guard's business; the rewrite
		// path reports its own errors.
		return nil
	}
	raw = plainRawUnit(raw)

	// Constructs MDL cannot express at all — a rewrite would lose them without a
	// word. Refused outright, and every reason is listed at once.
	var cannotExpress []string
	if n := rawEventSubProcessesWithoutStart(raw); n > 0 {
		cannotExpress = append(cannotExpress, fmt.Sprintf("%d event sub-process(es) with no start event, which it would delete", n))
	}
	cannotExpress = append(cannotExpress, studioProOnlyWorkflowState(raw)...)
	if len(cannotExpress) > 0 {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"workflow %s holds state MDL cannot express, and rewriting it would delete or reset it silently:\n  - %s\n"+
				"  Edit the workflow in Studio Pro, or use ALTER WORKFLOW to change one activity at a time — ALTER edits "+
				"the stored document and keeps what it does not touch.",
			qualifiedName, strings.Join(cannotExpress, "\n  - ")))
	}

	// Every counter below walks the event sub-process bodies as well as the main
	// body: the raw side counts the whole document.
	authoredActs := workflowStatementActivities(stmt)

	// Event sub-processes and notification activities MDL can now state. Before
	// it could, describe printed a notification activity as a comment and left
	// the sub-processes out, so a rewrite from that output would delete them.
	if stored := countEventSubProcesses(raw); stored > len(stmt.EventSubProcesses) {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"workflow %s has %d stored event sub-process(es) but this statement declares %d — rewriting it would "+
				"delete the difference, along with each one's flow.\n"+
				"  Restate them (`event subprocess <name> on interrupting notification <start> { … };`), which "+
				"`describe workflow %s` now emits, or use ALTER WORKFLOW to change one activity at a time.",
			qualifiedName, stored, len(stmt.EventSubProcesses), qualifiedName))
	}
	if stored, authored := countRawWorkflowNodesExact(raw, "Workflows$NotificationActivity"), countAuthoredNotificationActivities(authoredActs); stored > authored {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"workflow %s has %d stored notification activit(ies) but this statement declares %d — rewriting it would "+
				"delete the difference, and a notify action targeting one would target nothing.\n"+
				"  Restate them (`notification <name> comment '…';`), which `describe workflow %s` now emits, or use "+
				"ALTER WORKFLOW to change one activity at a time.",
			qualifiedName, stored, authored, qualifiedName))
	}

	// Handlers MDL can now state, but a statement written before it could (or
	// from an older describe) does not: the rewrite writes what the statement
	// says, so an unstated handler is deleted and an unstated on-created
	// microflow reset to none. Counted, like boundary events below, because a
	// handler has no name to match on and a task may be renamed.
	if stored := rawEventHandlers(raw); len(stored) > len(stmt.EventHandlers) {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"workflow %s has %d stored workflow event handler(s) but this statement declares %d — rewriting it would "+
				"delete the difference:\n  - %s\n"+
				"  Restate them (`on workflow events (…) microflow … as '…'`), which `describe workflow %s` now emits, "+
				"or use ALTER WORKFLOW to change one activity at a time.",
			qualifiedName, len(stored), len(stmt.EventHandlers), strings.Join(stored, "\n  - "), qualifiedName))
	}
	if stored, authored := rawOnCreatedMicroflows(raw), countAuthoredOnCreated(authoredActs); len(stored) > authored {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"workflow %s has %d user task(s) with an on-created microflow but this statement declares %d — rewriting "+
				"it would reset the difference to none:\n  - %s\n"+
				"  Restate them (`on created microflow …`), which `describe workflow %s` now emits, or use ALTER "+
				"WORKFLOW to change one activity at a time.",
			qualifiedName, len(stored), authored, strings.Join(stored, "\n  - "), qualifiedName))
	}

	if stored, authored := rawAgentTasks(raw), countAuthoredAgentTasks(authoredActs); len(stored) > authored {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"workflow %s has %d AI agent task(s) but this statement declares %d — rewriting it would delete the "+
				"difference, along with each one's outcome flows:\n  - %s\n"+
				"  Restate them (`call agent microflow …`), which `describe workflow %s` now emits, or use ALTER "+
				"WORKFLOW to change one activity at a time.",
			qualifiedName, len(stored), authored, strings.Join(stored, "\n  - "), qualifiedName))
	}

	// A multi-user task's completion rule, participant count and "wait for all
	// users" are expressible now (`decide by`, `participants`, `await all users`).
	// A rewrite writes what the statement says, and each omitted clause means the
	// default — consensus on the first outcome, all participants, not waiting — so
	// a statement that does not restate stored values resets them. Participants
	// and await were not guarded at all before; a rewrite reset both silently.
	storedRules, storedParticipants, storedAwait := rawMultiUserTaskSettings(raw)
	authoredRules, authoredParticipants, authoredAwait := countAuthoredMultiUserSettings(authoredActs)
	for _, g := range []struct {
		stored   []string
		authored int
		what     string
		restate  string
	}{
		{storedRules, authoredRules, "multi-user task(s) with a completion rule other than consensus on the first outcome", "`decide by …`"},
		{storedParticipants, authoredParticipants, "multi-user task(s) that need only some participants to respond", "`participants …`"},
		{storedAwait, authoredAwait, "multi-user task(s) that wait for all users", "`await all users`"},
	} {
		if len(g.stored) > g.authored {
			return mdlerrors.NewUnsupported(fmt.Sprintf(
				"workflow %s has %d %s but this statement declares %d — rewriting it would reset the difference to the default:\n  - %s\n"+
					"  Restate them (%s), which `describe workflow %s` now emits, or use ALTER WORKFLOW to change one activity at a time.",
				qualifiedName, len(g.stored), g.what, g.authored, strings.Join(g.stored, "\n  - "), g.restate, qualifiedName))
		}
	}

	// Ends inside branches. MDL could not state one until `end workflow`, and
	// describe dropped them, so a rewrite from an older describe output would
	// delete every one — and a branch that ended the workflow would silently
	// fall through into the main flow instead. The main flow's own End is not
	// counted: the builder always writes it.
	// An event sub-process's closing End is implicit too, like the main flow's.
	if storedEnds := countRawWorkflowNodesExact(raw, "Workflows$EndWorkflowActivity") - 1 - rawImplicitSubProcessEnds(raw); storedEnds > 0 {
		if authored := countAuthoredEnds(authoredActs); authored < storedEnds {
			return mdlerrors.NewUnsupported(fmt.Sprintf(
				"workflow %s has %d stored `end workflow` inside its branches but this statement declares %d — "+
					"rewriting it would delete the difference, and each branch that ended the workflow would fall "+
					"through into the main flow instead.\n"+
					"  Restate them (`end workflow;`), which `describe workflow %s` now emits, or use ALTER WORKFLOW "+
					"to change one activity at a time.",
				qualifiedName, storedEnds, authored, qualifiedName))
		}
	}

	storedBE := countRawBoundaryEvents(raw)
	if storedBE == 0 {
		return nil
	}
	authored := countAuthoredBoundaryEvents(authoredActs)
	if authored >= storedBE {
		return nil
	}
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"workflow %s has %d stored boundary event(s) but this statement declares %d — "+
			"rewriting it would delete the difference, along with each one's handler flow.\n"+
			"  Restate them (`boundary event interrupting timer '<expr>' { … }`), which "+
			"`describe workflow %s` now emits, or use ALTER WORKFLOW to change one activity at a time.",
		qualifiedName, storedBE, authored, qualifiedName))
}

// countRawWorkflowNodes counts BSON sub-documents whose $Type contains the given
// marker. Matching on a substring rather than an exact type is deliberate: the
// three timer boundary-event variants and the two event-sub-process start
// activities all differ by prefix, and a variant added later should be caught by
// the guard rather than slip past it.
// countRawBoundaryEvents counts the boundary events stored in a raw workflow.
//
// It matches a $Type that ENDS in "BoundaryEvent". A substring match also
// counted Workflows$EndOfBoundaryEventPathActivity — the marker that ends every
// boundary path, and which mxcli now writes — so one event read as two and
// `create or modify` refused a workflow whose statement restated it exactly.
func countRawBoundaryEvents(v any) int {
	switch t := v.(type) {
	case map[string]any:
		n := 0
		if s, ok := t["$Type"].(string); ok && strings.HasSuffix(s, "BoundaryEvent") {
			n++
		}
		for k, child := range t {
			if k != "$Type" {
				n += countRawBoundaryEvents(child)
			}
		}
		return n
	case []any:
		n := 0
		for _, e := range t {
			n += countRawBoundaryEvents(e)
		}
		return n
	}
	return 0
}

func countRawWorkflowNodes(v any, marker string) int {
	switch t := v.(type) {
	case map[string]any:
		n := 0
		if s, ok := t["$Type"].(string); ok && strings.Contains(s, marker) {
			n++
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic traversal; the count itself is order-free
		for _, k := range keys {
			if k == "$Type" {
				continue
			}
			n += countRawWorkflowNodes(t[k], marker)
		}
		return n
	case []any:
		n := 0
		for _, e := range t {
			n += countRawWorkflowNodes(e, marker)
		}
		return n
	}
	return 0
}

// countAuthoredBoundaryEvents counts the boundary events the statement declares,
// walking nested flows the same way the rest of workflow validation does.
func countAuthoredBoundaryEvents(activities []ast.WorkflowActivityNode) int {
	n := 0
	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		switch a := act.(type) {
		case *ast.WorkflowUserTaskNode:
			n += len(a.BoundaryEvents)
		case *ast.WorkflowCallMicroflowNode:
			n += len(a.BoundaryEvents)
		case *ast.WorkflowWaitForNotificationNode:
			n += len(a.BoundaryEvents)
		}
	})
	return n
}

// studioProOnlyWorkflowState lists what a stored workflow holds that a rebuild
// from MDL would reset: a workflow event handler subscribed to no event types
// (the grammar has no empty list).
func studioProOnlyWorkflowState(raw map[string]any) []string {
	var out []string
	for _, h := range rawList(raw["OnWorkflowEvent"]) {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		types := 0
		for _, t := range rawList(hm["EventTypes"]) {
			if _, ok := t.(string); ok {
				types++
			}
		}
		if types == 0 {
			out = append(out, fmt.Sprintf("workflow event handler %s subscribes to no event types, which MDL cannot state", rawHandlerLabel(hm)))
		}
	}
	return out
}

// rawCompletionRule describes a stored multi-user task's completion rule when it
// is not what an omitted `decide by` writes (consensus falling back to the first
// outcome); "" otherwise.
func rawCompletionRule(d map[string]any) string {
	name, _ := d["Name"].(string)
	cc, ok := d["CompletionCriteria"].(map[string]any)
	if !ok {
		return ""
	}
	kind, _ := cc["$Type"].(string)
	var outcomes []map[string]any
	for _, o := range rawList(d["Outcomes"]) {
		if om, ok := o.(map[string]any); ok {
			outcomes = append(outcomes, om)
		}
	}
	switch kind {
	case "Workflows$ConsensusCompletionCriteria":
		if len(outcomes) > 0 && reflect.DeepEqual(cc["FallbackOutcomePointer"], outcomes[0]["$ID"]) {
			return ""
		}
		fallback := "no outcome"
		for _, o := range outcomes {
			if reflect.DeepEqual(cc["FallbackOutcomePointer"], o["$ID"]) {
				fallback = "outcome '" + rawOutcomeLabel(o) + "'"
			}
		}
		return fmt.Sprintf("multi user task '%s' falls back to %s when consensus fails", name, fallback)
	default:
		rule := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(kind, "Workflows$"), "CompletionCriteria"))
		return fmt.Sprintf("multi user task '%s' decides by %s", name, rule)
	}
}

// rawParticipants describes a stored multi-user task that needs only some of its
// targeted users to respond; "" when it needs all of them.
func rawParticipants(d map[string]any) string {
	t, ok := d["TargetUserInput"].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := d["Name"].(string)
	switch t["$Type"] {
	case "Workflows$AbsoluteAmountUserInput":
		return fmt.Sprintf("multi user task '%s' needs %v participants", name, t["Amount"])
	case "Workflows$PercentageAmountUserInput":
		return fmt.Sprintf("multi user task '%s' needs %v percent of participants", name, t["Percentage"])
	}
	return ""
}

func rawAwaitsAllUsers(d map[string]any) bool {
	await, _ := d["AwaitAllUsers"].(bool)
	return await
}

// rawMultiUserTaskSettings labels every stored multi-user task setting a rewrite
// resets when the statement omits it.
func rawMultiUserTaskSettings(raw map[string]any) (rules, participants, await []string) {
	walkRawDocs(raw, func(d map[string]any) {
		if t, _ := d["$Type"].(string); t != "Workflows$MultiUserTaskActivity" {
			return
		}
		if r := rawCompletionRule(d); r != "" {
			rules = append(rules, r)
		}
		if p := rawParticipants(d); p != "" {
			participants = append(participants, p)
		}
		if rawAwaitsAllUsers(d) {
			name, _ := d["Name"].(string)
			await = append(await, fmt.Sprintf("multi user task '%s' waits for all users", name))
		}
	})
	return rules, participants, await
}

func countAuthoredMultiUserSettings(activities []ast.WorkflowActivityNode) (rules, participants, await int) {
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		n, ok := a.(*ast.WorkflowUserTaskNode)
		if !ok || !n.IsMultiUser {
			return
		}
		if n.Completion != nil {
			rules++
		}
		if n.Participants != nil && n.Participants.Kind != "all" {
			participants++
		}
		if n.AwaitAllUsers {
			await++
		}
	})
	return rules, participants, await
}

// rawReplaceLosses lists what a stored activity holds that its replacement does
// not restate, so REPLACE ACTIVITY would reset it.
func rawReplaceLosses(d map[string]any, replacement ast.WorkflowActivityNode) []string {
	var out []string
	mut, _ := replacement.(*ast.WorkflowUserTaskNode)
	isMulti := mut != nil && mut.IsMultiUser
	if r := rawCompletionRule(d); r != "" && (!isMulti || mut.Completion == nil) {
		out = append(out, r+", which the replacement does not restate (add `decide by …`)")
	}
	if p := rawParticipants(d); p != "" && (!isMulti || mut.Participants == nil || mut.Participants.Kind == "all") {
		out = append(out, p+", which the replacement does not restate (add `participants …`)")
	}
	if rawAwaitsAllUsers(d) && (!isMulti || !mut.AwaitAllUsers) {
		out = append(out, "it waits for all users, which the replacement does not restate (add `await all users`)")
	}
	return out
}

// validateAlterReplaceKeepsStudioProState refuses REPLACE ACTIVITY on an activity
// whose stored document holds state the replacement is rebuilt without. Measured
// on ako/TestApp: replacing a user task with an identical one reset its
// on-created microflow to NoEvent while exec reported "Altered workflow"; SET
// ACTIVITY, which edits the stored document in place, kept it.
func validateAlterReplaceKeepsStudioProState(ctx *ExecContext, s *ast.AlterWorkflowStmt) []string {
	var replaces []*ast.ReplaceActivityOp
	for _, op := range s.Operations {
		if o, ok := op.(*ast.ReplaceActivityOp); ok {
			replaces = append(replaces, o)
		}
	}
	if len(replaces) == 0 || ctx == nil || ctx.Backend == nil {
		return nil
	}
	wf := findStoredWorkflow(ctx, s.Name)
	if wf == nil || wf.Flow == nil {
		return nil
	}
	raw, err := ctx.Backend.GetRawUnit(wf.ID)
	if err != nil || raw == nil {
		return nil
	}
	raw = plainRawUnit(raw)
	var errs []string
	for _, o := range replaces {
		act := resolveStoredActivity(wf.Flow, o.ActivityRef, o.AtPosition)
		if act == nil {
			continue
		}
		var reasons []string
		walkRawDocs(raw, func(d map[string]any) {
			if n, _ := d["Name"].(string); n != "" && n == act.GetName() {
				reasons = append(reasons, rawReplaceLosses(d, o.NewActivity)...)
				if mf := rawOnCreatedMicroflow(d); mf != "" && !restatesOnCreated(o.NewActivity) {
					reasons = append(reasons, fmt.Sprintf(
						"it runs on-created microflow %s, which the replacement does not restate (add `on created microflow %s`)", mf, mf))
				}
			}
		})
		if len(reasons) > 0 {
			errs = append(errs, fmt.Sprintf(
				"replace activity '%s' is refused: the activity is rebuilt from the statement, and the statement does not carry what it "+
					"holds — %s. Change it with SET ACTIVITY, which edits it in place, or in Studio Pro.",
				o.ActivityRef, strings.Join(reasons, "; ")))
		}
	}
	return errs
}

// rawEventHandlers labels each stored workflow event handler.
func rawEventHandlers(raw map[string]any) []string {
	var out []string
	for _, h := range rawList(raw["OnWorkflowEvent"]) {
		if hm, ok := h.(map[string]any); ok {
			out = append(out, rawHandlerLabel(hm))
		}
	}
	return out
}

func rawHandlerLabel(h map[string]any) string {
	microflow := ""
	if mh, ok := h["MicroflowEventHandler"].(map[string]any); ok {
		microflow, _ = mh["Microflow"].(string)
	}
	if d, _ := h["Description"].(string); d != "" {
		return fmt.Sprintf("'%s' (microflow %s)", d, orUnnamed(microflow))
	}
	return "running microflow " + orUnnamed(microflow)
}

// rawAgentTasks labels each stored AI agent task.
func rawAgentTasks(raw map[string]any) []string {
	var out []string
	walkRawDocs(raw, func(d map[string]any) {
		if t, _ := d["$Type"].(string); t == "Workflows$AIAgentTaskActivity" {
			name, _ := d["Name"].(string)
			mf, _ := d["Microflow"].(string)
			out = append(out, fmt.Sprintf("AI agent task '%s' (microflow %s)", name, orUnnamed(mf)))
		}
	})
	return out
}

// rawOnCreatedMicroflows labels each stored user task that runs an on-created
// microflow.
func rawOnCreatedMicroflows(raw map[string]any) []string {
	var out []string
	walkRawDocs(raw, func(d map[string]any) {
		if mf := rawOnCreatedMicroflow(d); mf != "" {
			name, _ := d["Name"].(string)
			out = append(out, fmt.Sprintf("user task '%s' runs %s", name, mf))
		}
	})
	return out
}

// rawOnCreatedMicroflow returns the microflow a stored activity's OnCreatedEvent
// runs, "" for NoEvent or none.
func rawOnCreatedMicroflow(d map[string]any) string {
	ev, ok := d["OnCreatedEvent"].(map[string]any)
	if !ok {
		return ""
	}
	if t, _ := ev["$Type"].(string); t != "Workflows$MicroflowBasedEvent" {
		return ""
	}
	mf, _ := ev["Microflow"].(string)
	return orUnnamed(mf)
}

func countAuthoredOnCreated(activities []ast.WorkflowActivityNode) int {
	n := 0
	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		if restatesOnCreated(act) {
			n++
		}
	})
	return n
}

func restatesOnCreated(act ast.WorkflowActivityNode) bool {
	t, ok := act.(*ast.WorkflowUserTaskNode)
	return ok && t.OnCreated.Module != ""
}

// plainRawUnit returns a copy of a stored unit with every array and document in
// the plain []any / map[string]any shape the guards in this file walk.
//
// The legacy engine decodes a unit's arrays as primitive.A — measured on a
// workflow it wrote: OnWorkflowEvent and Flow.Activities both — and a type switch
// on []any does not match that named type. So on the legacy engine every guard
// here saw no list at all, and allowed each rewrite it exists to refuse (stored
// handlers, on-created microflows, boundary events, nested Ends, AI agent tasks,
// completion rules), while the modelsdk engine, which already returns plain
// slices, refused them. Normalised here rather than in the backend, because
// other raw consumers (catalog, linter) assert the primitive types.
func plainRawUnit(raw map[string]any) map[string]any {
	out, _ := plainRawValue(raw).(map[string]any)
	return out
}

func plainRawValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, e := range t {
			m[k] = plainRawValue(e)
		}
		return m
	case primitive.M:
		return plainRawValue(map[string]any(t))
	case primitive.D:
		m := make(map[string]any, len(t))
		for _, e := range t {
			m[e.Key] = plainRawValue(e.Value)
		}
		return m
	case []any:
		l := make([]any, len(t))
		for i, e := range t {
			l[i] = plainRawValue(e)
		}
		return l
	case primitive.A:
		return plainRawValue([]any(t))
	default:
		return v
	}
}

// walkRawDocs visits every sub-document of a raw unit, keys in sorted order so
// the reasons come out the same way every time.
func walkRawDocs(v any, visit func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		visit(t)
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walkRawDocs(t[k], visit)
		}
	case []any:
		for _, e := range t {
			walkRawDocs(e, visit)
		}
	}
}

// rawList returns a stored list's elements; a list's first element may be the
// typed-array marker, which is not a map and is skipped by callers.
func rawList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

func rawOutcomeLabel(o map[string]any) string {
	for _, k := range []string{"Value", "Caption", "Name"} {
		if s, _ := o[k].(string); s != "" {
			return s
		}
	}
	return "?"
}

func orUnnamed(s string) string {
	if s == "" {
		return "(unnamed)"
	}
	return s
}

// countAuthoredNotificationActivities counts the `notification` statements.
func countAuthoredNotificationActivities(activities []ast.WorkflowActivityNode) int {
	n := 0
	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		if _, ok := act.(*ast.WorkflowNotificationNode); ok {
			n++
		}
	})
	return n
}

// rawSubProcessActivities returns the top-level activities of each stored event
// sub-process, in stored order.
func rawSubProcessActivities(raw map[string]any) [][]map[string]any {
	var out [][]map[string]any
	for _, e := range rawList(raw["EventSubProcesses"]) {
		esp, ok := e.(map[string]any)
		if !ok {
			continue
		}
		flow, _ := esp["Flow"].(map[string]any)
		var acts []map[string]any
		for _, a := range rawList(flow["Activities"]) {
			if am, ok := a.(map[string]any); ok {
				acts = append(acts, am)
			}
		}
		out = append(out, acts)
	}
	return out
}

// rawEventSubProcessesWithoutStart counts stored event sub-processes whose flow
// does not begin with a start event: MDL states a sub-process by its start, so
// one without cannot be restated.
func rawEventSubProcessesWithoutStart(raw map[string]any) int {
	n := 0
	for _, acts := range rawSubProcessActivities(raw) {
		if len(acts) == 0 {
			n++
			continue
		}
		t, _ := acts[0]["$Type"].(string)
		if _, _, ok := workflows.EventSubProcessStartFromStorageType(t); !ok {
			n++
		}
	}
	return n
}

// rawImplicitSubProcessEnds counts stored event sub-processes whose flow closes
// with an End — the End a statement does not write, because the builder adds it.
func rawImplicitSubProcessEnds(raw map[string]any) int {
	n := 0
	for _, acts := range rawSubProcessActivities(raw) {
		if len(acts) == 0 {
			continue
		}
		if t, _ := acts[len(acts)-1]["$Type"].(string); t == "Workflows$EndWorkflowActivity" {
			n++
		}
	}
	return n
}

// countEventSubProcesses counts a workflow's event sub-processes. The substring
// match countRawWorkflowNodes uses also counts each one's start activity
// (…NotificationEventSubProcessStartActivity lives inside it), so the reference
// workflow's single sub-process was reported as two. The exact type is counted;
// start activities stand in only when no sub-process document is present, one
// start per sub-process.
func countEventSubProcesses(raw map[string]any) int {
	subProcesses, starts := 0, 0
	walkRawDocs(raw, func(d map[string]any) {
		t, _ := d["$Type"].(string)
		switch {
		case t == "Workflows$EventSubProcess":
			subProcesses++
		case strings.HasSuffix(t, "EventSubProcessStartActivity"):
			starts++
		}
	})
	if subProcesses > 0 {
		return subProcesses
	}
	return starts
}

// countRawWorkflowNodesExact counts BSON sub-documents whose $Type is exactly
// the given one. The substring match countRawWorkflowNodes uses would also count
// Workflows$EndOfParallelSplitPathActivity and EndOfBoundaryEventPathActivity,
// the markers mxcli writes at the end of every path.
func countRawWorkflowNodesExact(v any, typeName string) int {
	switch t := v.(type) {
	case map[string]any:
		n := 0
		if s, ok := t["$Type"].(string); ok && s == typeName {
			n++
		}
		for k, child := range t {
			if k != "$Type" {
				n += countRawWorkflowNodesExact(child, typeName)
			}
		}
		return n
	case []any:
		n := 0
		for _, e := range t {
			n += countRawWorkflowNodesExact(e, typeName)
		}
		return n
	}
	return 0
}

// countAuthoredEnds counts the `end workflow` statements inside branches.
func countAuthoredEnds(activities []ast.WorkflowActivityNode) int {
	n := 0
	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		if _, ok := act.(*ast.WorkflowEndNode); ok {
			n++
		}
	})
	return n
}
