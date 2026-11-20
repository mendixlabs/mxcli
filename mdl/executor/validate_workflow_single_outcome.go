// SPDX-License-Identifier: Apache-2.0

// MDL-WF02 for ALTER WORKFLOW: a user task left with one outcome that holds
// activities (CE1876, "Single outcome must not contain any activities in its
// flow.").
//
// CREATE judges this per statement (ValidateWorkflow). ALTER cannot: a later op
// in the same statement, or a later statement in the script, may still add the
// second outcome — which is why ValidateAlterWorkflow skipped the rule, and why
// no alter was ever judged (ako/mxcli#2097, mendixlabs/mxcli#1798). This pass
// replays the script's workflow statements over a small model of each workflow's
// user tasks and judges the state the SCRIPT leaves.
//
// The baseline of a workflow is, in order of preference: the script's own
// CREATE of it; the stored workflow, when a project is given; nothing. With no
// baseline only the tasks the script inserts are known, so dropping a stored
// task's outcome without a project says nothing — the third state of
// check-mxbuild-drift: what cannot be established is not reported.
package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// soActivity is one activity of an outcome's flow, as far as this rule needs it:
// the two names a reference can match. opaque marks an authored activity
// written without a name — the builder generates one, so a later reference may
// name it without this model being able to tell.
type soActivity struct {
	name, caption string
	opaque        bool
}

func (a soActivity) matches(ref string) bool {
	return ref != "" && (a.name == ref || a.caption == ref)
}

type soOutcome struct {
	value string
	top   []soActivity // the flow's own activities, in order
	inner []soActivity // every activity nested below them
}

type soTask struct {
	name, caption string
	outcomes      []*soOutcome
	// touched is set once an ALTER changed the task (or inserted it); only
	// those are judged — an untouched CREATE task is ValidateWorkflow's.
	touched bool
	// unknown is set when an op may have changed the task in a way this model
	// cannot follow; the task is then never reported.
	unknown bool
	removed bool
}

func (t *soTask) label() string {
	if t.name != "" {
		return "'" + t.name + "'"
	}
	if t.caption != "" {
		return "'" + t.caption + "'"
	}
	return "(unnamed)"
}

// violates mirrors ValidateWorkflow's condition: exactly one outcome, holding
// at least one activity.
func (t *soTask) violates() bool {
	return len(t.outcomes) == 1 && len(t.outcomes[0].top) > 0
}

type soWorkflow struct {
	name  ast.QualifiedName
	tasks []*soTask
}

// validateAlterWorkflowSingleOutcomes is the script-level MDL-WF02 for ALTER.
func validateAlterWorkflowSingleOutcomes(prog *ast.Program, project func() backend.FullBackend) []linter.Violation {
	if prog == nil {
		return nil
	}
	hasAlter := false
	for _, s := range prog.Statements {
		if _, ok := s.(*ast.AlterWorkflowStmt); ok {
			hasAlter = true
			break
		}
	}
	if !hasAlter {
		return nil
	}

	var ctx *ExecContext
	ctxLoaded := false
	stored := func(qn ast.QualifiedName) *workflows.Workflow {
		if !ctxLoaded {
			ctxLoaded = true
			if project != nil {
				if b := project(); b != nil {
					ctx = &ExecContext{Backend: b}
				}
			}
		}
		return findStoredWorkflow(ctx, qn)
	}

	sims := map[string]*soWorkflow{}
	var order []string
	key := func(qn ast.QualifiedName) string { return strings.ToLower(qn.String()) }
	put := func(k string, w *soWorkflow) {
		if _, seen := sims[k]; !seen {
			order = append(order, k)
		}
		sims[k] = w
	}

	for _, s := range prog.Statements {
		switch st := s.(type) {
		case *ast.CreateWorkflowStmt:
			w := &soWorkflow{name: st.Name}
			w.addTasks(st.Activities, false)
			for _, esp := range st.EventSubProcesses {
				w.addTasks(esp.Activities, false)
			}
			put(key(st.Name), w)
		case *ast.DropWorkflowStmt:
			if _, ok := sims[key(st.Name)]; ok {
				sims[key(st.Name)] = nil
			}
		case *ast.AlterWorkflowStmt:
			k := key(st.Name)
			w, seen := sims[k]
			if !seen {
				w = &soWorkflow{name: st.Name}
				if wf := stored(st.Name); wf != nil {
					w.addStoredTasks(wf.Flow)
				}
				put(k, w)
			}
			if w == nil { // dropped earlier in the script; exec will say so
				continue
			}
			for _, op := range st.Operations {
				w.apply(op)
			}
		}
	}

	var out []linter.Violation
	for _, k := range order {
		w := sims[k]
		if w == nil {
			continue
		}
		loc := workflowLocation(w.name)
		for _, t := range w.tasks {
			if t.removed || t.unknown || !t.touched || !t.violates() {
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-WF02",
				Severity: linter.SeverityError,
				Location: loc,
				Message: fmt.Sprintf("after the script's ALTER WORKFLOW statements, user task %s has a single outcome ('%s') "+
					"with nested activities — MxBuild rejects this (CE1876 \"Single outcome must not contain any activities in its flow.\")",
					t.label(), t.outcomes[0].value),
				Suggestion: "Keep (or add) a second outcome on the task — `insert into " + strings.Trim(t.label(), "'") +
					" { outcomes '<name>' { } }` — or move the outcome's activities to the flow after the task.",
			})
		}
	}
	return out
}

// addTasks records every user task in an authored body, at any depth.
func (w *soWorkflow) addTasks(acts []ast.WorkflowActivityNode, touched bool) {
	walkWorkflowActivities(acts, func(a ast.WorkflowActivityNode) {
		n, ok := a.(*ast.WorkflowUserTaskNode)
		if !ok {
			return
		}
		t := &soTask{name: n.Name, caption: n.Caption, touched: touched}
		for _, o := range n.Outcomes {
			t.outcomes = append(t.outcomes, authoredOutcome(o.Caption, o.Activities))
		}
		w.tasks = append(w.tasks, t)
	})
}

// authoredOutcome models an outcome the script writes. Its value is the
// caption, as buildWorkflowUserTask and the mutator's InsertOutcome store it.
func authoredOutcome(value string, acts []ast.WorkflowActivityNode) *soOutcome {
	o := &soOutcome{value: value}
	for _, a := range acts {
		if a == nil {
			continue
		}
		o.top = append(o.top, authoredRef(a))
	}
	walkWorkflowActivities(acts, func(a ast.WorkflowActivityNode) { o.inner = append(o.inner, authoredRef(a)) })
	return o
}

// authoredRef reads an authored activity's Name and Caption. Every activity
// node spells them as those two string fields where it has them; reading them
// by field name keeps this from drifting as activity kinds are added.
func authoredRef(a ast.WorkflowActivityNode) soActivity {
	v := reflect.ValueOf(a)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return soActivity{opaque: true}
		}
		v = v.Elem()
	}
	str := func(field string) string {
		if v.Kind() != reflect.Struct {
			return ""
		}
		f := v.FieldByName(field)
		if f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
		return ""
	}
	r := soActivity{name: str("Name"), caption: str("Caption")}
	r.opaque = r.name == ""
	return r
}

// addStoredTasks records every user task in a stored flow, at any depth.
func (w *soWorkflow) addStoredTasks(flow *workflows.Flow) {
	if flow == nil {
		return
	}
	for _, a := range flow.Activities {
		if a == nil {
			continue
		}
		if ut, ok := a.(*workflows.UserTask); ok {
			t := &soTask{name: ut.Name, caption: ut.Caption}
			for _, o := range ut.Outcomes {
				if o == nil {
					continue
				}
				so := &soOutcome{value: o.Value}
				if o.Flow != nil {
					for _, x := range o.Flow.Activities {
						if x != nil && !storedFlowMarker(x) {
							so.top = append(so.top, soActivity{name: x.GetName(), caption: x.GetCaption()})
						}
					}
					collectStoredRefs(o.Flow, &so.inner)
				}
				t.outcomes = append(t.outcomes, so)
			}
			w.tasks = append(w.tasks, t)
		}
		for _, f := range storedNestedFlows(a) {
			w.addStoredTasks(f)
		}
	}
}

// storedFlowMarker reports the activities a flow stores as structure rather
// than as work: they are not "activities in its flow" for CE1876.
func storedFlowMarker(a workflows.WorkflowActivity) bool {
	switch a.(type) {
	case *workflows.StartWorkflowActivity, *workflows.EndOfParallelSplitPathActivity, *workflows.EndOfBoundaryEventPathActivity:
		return true
	}
	return false
}

func collectStoredRefs(flow *workflows.Flow, out *[]soActivity) {
	if flow == nil {
		return
	}
	for _, a := range flow.Activities {
		if a == nil {
			continue
		}
		*out = append(*out, soActivity{name: a.GetName(), caption: a.GetCaption()})
		for _, f := range storedNestedFlows(a) {
			collectStoredRefs(f, out)
		}
	}
}

// resolveTask finds the task an op addresses the way the mutator does: a Name
// match wins over caption matches, @N picks among all matches. ok is false
// when the reference is ambiguous or the position is out of range — the
// caller then treats every candidate as unknown rather than guessing.
func (w *soWorkflow) resolveTask(ref string, atPos int) (task *soTask, candidates []*soTask, ok bool) {
	var all, named []*soTask
	for _, t := range w.tasks {
		if t.removed {
			continue
		}
		if t.name == ref || (ref != "" && t.caption == ref) {
			all = append(all, t)
			if t.name == ref {
				named = append(named, t)
			}
		}
	}
	switch {
	case atPos > 0:
		// The model's order is not guaranteed to be DESCRIBE's for inserted
		// tasks, so @N is not resolved here.
		return nil, all, false
	case len(named) == 1:
		return named[0], all, true
	case len(named) == 0 && len(all) == 1:
		return all[0], all, true
	}
	return nil, all, false
}

func markUnknown(ts []*soTask) {
	for _, t := range ts {
		t.unknown = true
	}
}

// apply plays one ALTER op over the model, in the mutator's order.
func (w *soWorkflow) apply(op ast.AlterWorkflowOp) {
	switch o := op.(type) {
	case *ast.InsertAfterOp:
		w.insertAt(o.ActivityRef, o.NewActivities)
	case *ast.InsertBeforeOp:
		w.insertAt(o.ActivityRef, o.NewActivities)
	case *ast.ReplaceActivityOp:
		w.replaceActivity(o.ActivityRef, o.AtPosition, o.NewActivities)
	case *ast.DropActivityOp:
		w.dropActivity(o.ActivityRef, o.AtPosition)
	case *ast.InsertPathOp:
		w.addTasks(o.Activities, true)
	case *ast.InsertBranchOp:
		w.addTasks(o.Activities, true)
	case *ast.InsertBoundaryEventOp:
		w.addTasks(o.Activities, true)
	case *ast.InsertOutcomeOp:
		t, cands, ok := w.resolveTask(o.ActivityRef, o.AtPosition)
		switch {
		case ok:
			t.outcomes = append(t.outcomes, authoredOutcome(o.OutcomeName, o.Activities))
			t.touched = true
		case len(cands) > 0:
			markUnknown(cands)
		default:
			// A task this model does not know (an unnamed inserted task gets
			// a generated name): any inserted task might be it.
			w.markInsertedUnknown()
		}
		w.addTasks(o.Activities, true)
	case *ast.DropOutcomeOp:
		t, cands, ok := w.resolveTask(o.ActivityRef, o.AtPosition)
		if !ok {
			markUnknown(cands)
			return
		}
		at, n := -1, 0
		for i, oc := range t.outcomes {
			if oc.value == o.OutcomeName {
				if at < 0 {
					at = i
				}
				n++
			}
		}
		if n != 1 { // the mutator refuses both; exec will say so
			return
		}
		t.outcomes = append(t.outcomes[:at], t.outcomes[at+1:]...)
		t.touched = true
	}
}

// insertAt records what an insert next to ref adds: its tasks, and — when ref
// sits in a task's outcome — the outcome's extra activities.
func (w *soWorkflow) insertAt(ref string, acts []ast.WorkflowActivityNode) {
	for _, t := range w.tasks {
		if t.removed {
			continue
		}
		for _, o := range t.outcomes {
			for _, a := range o.top {
				if a.matches(ref) {
					for _, x := range acts {
						if x != nil {
							o.top = append(o.top, authoredRef(x))
						}
					}
				}
			}
		}
	}
	w.addTasks(acts, true)
}

// dropActivity removes the activity ref names: a task, or an activity of an
// outcome. Ambiguity or a nested match makes the affected tasks unknown.
func (w *soWorkflow) dropActivity(ref string, atPos int) {
	if t, cands, ok := w.resolveTask(ref, atPos); ok {
		t.removed = true
		w.removeNestedTasks(t)
		return
	} else if len(cands) > 0 {
		markUnknown(cands)
		return
	}
	if !w.editOutcomeActivity(ref, atPos, nil) {
		w.markOpaqueUnknown()
	}
}

// replaceActivity is REPLACE: the task ref names goes and the new activities
// take its place; an outcome activity is swapped in place, so the outcome
// keeps whatever the replacement holds.
func (w *soWorkflow) replaceActivity(ref string, atPos int, acts []ast.WorkflowActivityNode) {
	if t, cands, ok := w.resolveTask(ref, atPos); ok {
		t.removed = true
		w.removeNestedTasks(t)
	} else if len(cands) > 0 {
		markUnknown(cands)
	} else {
		var repl []soActivity
		for _, x := range acts {
			if x != nil {
				repl = append(repl, authoredRef(x))
			}
		}
		if !w.editOutcomeActivity(ref, atPos, repl) {
			w.markOpaqueUnknown()
		}
	}
	w.addTasks(acts, true)
}

// editOutcomeActivity replaces the outcome activity ref names with repl (nil
// removes it). It reports whether ref named one this model knows; with @N the
// owning task becomes unknown rather than guessing which match is meant.
func (w *soWorkflow) editOutcomeActivity(ref string, atPos int, repl []soActivity) bool {
	found := false
	for _, t := range w.tasks {
		if t.removed {
			continue
		}
		for _, o := range t.outcomes {
			for i, a := range o.top {
				if !a.matches(ref) {
					continue
				}
				found = true
				if atPos > 0 {
					t.unknown = true
					break
				}
				next := append([]soActivity{}, o.top[:i]...)
				next = append(next, repl...)
				o.top = append(next, o.top[i+1:]...)
				break
			}
		}
	}
	return found
}

// markOpaqueUnknown makes unknown every task with an unnamed outcome activity:
// a reference this model could not resolve may be that activity's generated
// name.
func (w *soWorkflow) markOpaqueUnknown() {
	for _, t := range w.tasks {
		for _, o := range t.outcomes {
			for _, a := range o.top {
				if a.opaque {
					t.unknown = true
				}
			}
		}
	}
}

// removeNestedTasks drops the tasks that lived in a removed task's outcomes.
func (w *soWorkflow) removeNestedTasks(parent *soTask) {
	for _, o := range parent.outcomes {
		for _, ref := range o.inner {
			for _, t := range w.tasks {
				if t != parent && !t.removed && ref.matches(t.name) {
					t.removed = true
				}
			}
		}
	}
}

// markInsertedUnknown makes every task the script inserted without a name
// unknown: it may be the target of an op this model could not resolve.
func (w *soWorkflow) markInsertedUnknown() {
	for _, t := range w.tasks {
		if t.touched && t.name == "" {
			t.unknown = true
		}
	}
}
