// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// MDL-WF02 for ALTER WORKFLOW: a user task left with a single outcome that
// holds activities, which MxBuild rejects with CE1876 ("Single outcome must not
// contain any activities in its flow").
//
// CREATE judges this from its body (ValidateWorkflow). An ALTER can reach the
// same state three ways — insert a task whose one outcome holds a flow, insert
// activities into the only outcome a task has, or drop a task's other outcomes
// — and the last two depend on the outcomes the task already has. A single ALTER
// is also not the final word: a later operation, in the same statement or a
// later one, can give the task a second outcome or empty the first. So this is
// judged on the state the whole script leaves each workflow in, replaying every
// operation in order over a small model of the workflow's activities:
//
//   - the stored workflow, when a project is given and the script does not
//     create the workflow itself;
//   - the script's own CREATE WORKFLOW body, when it has one;
//   - otherwise nothing: only the activities the ALTERs insert are known, and an
//     operation that names one of them may in fact address a stored activity
//     (an inserted name that clashes is renamed when it is written), so
//     whatever such an operation could change is not judged at all.
//
// Only a task the script put in this state is reported: a stored task that was
// already broken is not this script's finding.
func validateAlterWorkflowSingleOutcomes(prog *ast.Program, project func() backend.FullBackend) []linter.Violation {
	if prog == nil || !programAltersWorkflows(prog) {
		return nil
	}
	var ctx *ExecContext
	if project != nil {
		if b := project(); b != nil {
			ctx = &ExecContext{Backend: b}
		}
	}
	type tracked struct {
		qn  ast.QualifiedName
		sim *wfSim
	}
	var order []string
	states := map[string]*tracked{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateWorkflowStmt:
			key := strings.ToLower(s.Name.String())
			if _, ok := states[key]; !ok {
				order = append(order, key)
			}
			// A CREATE reports its own body (ValidateWorkflow); what it holds is
			// the base the ALTERs after it change.
			states[key] = &tracked{qn: s.Name, sim: newWfSimFromAST(workflowStatementActivities(s))}
		case *ast.DropWorkflowStmt:
			delete(states, strings.ToLower(s.Name.String()))
		case *ast.AlterWorkflowStmt:
			key := strings.ToLower(s.Name.String())
			st, ok := states[key]
			if !ok {
				st = &tracked{qn: s.Name, sim: newWfSimFromStored(findStoredWorkflow(ctx, s.Name))}
				states[key] = st
				order = append(order, key)
			}
			for _, op := range s.Operations {
				st.sim.apply(op)
			}
		}
	}
	var out []linter.Violation
	for _, key := range order {
		st, ok := states[key]
		if !ok {
			continue
		}
		loc := linter.Location{Module: st.qn.Module, DocumentType: "workflow", DocumentName: st.qn.Name}
		for _, task := range st.sim.newSingleOutcomeTasks() {
			out = append(out, singleOutcomeViolation(task.label(), task.outcomes[0].onlyEnd(), loc))
		}
	}
	return out
}

func programAltersWorkflows(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		if _, ok := stmt.(*ast.AlterWorkflowStmt); ok {
			return true
		}
	}
	return false
}

// singleOutcomeViolation is MDL-WF02's finding, worded alike for CREATE and
// ALTER. label is the task as workflowUserTaskLabel prints it.
func singleOutcomeViolation(label string, onlyEnd bool, loc linter.Location) linter.Violation {
	msg := fmt.Sprintf("user task %s has a single outcome with nested activities — MxBuild rejects this (CE1876)", label)
	suggestion := "Move the activities to the workflow's main flow after the user task, or add a second outcome."
	// "Move them to the main flow" is wrong advice for an End: moved there it
	// is a parse error, and the workflow ends at its own `end workflow` anyway.
	if onlyEnd {
		msg = fmt.Sprintf("user task %s has a single outcome that holds `end workflow` — a single outcome may not hold activities at all, so MxBuild rejects this (CE1876), and nothing after the task could be reached", label)
		suggestion = "Add a second outcome that continues, if the task is meant to decide whether the workflow ends; otherwise remove `end workflow;` — the workflow ends at its own `end workflow` after the task."
	}
	return linter.Violation{
		RuleID:     "MDL-WF02",
		Severity:   linter.SeverityError,
		Location:   loc,
		Message:    msg,
		Suggestion: suggestion,
	}
}

// wfSim is the part of a workflow MDL-WF02 needs: every activity with its name
// and caption, and each user task's outcomes in order. known says the base is
// the whole workflow (stored or created by the script); when it is not, the
// activities the ALTERs insert are kept as loose fragments.
type wfSim struct {
	known bool
	roots []*wfSimFlow
}

type wfSimNode struct {
	name, caption string
	userTask      bool
	end           bool
	outcomes      []*wfSimFlow // a user task's outcomes
	nested        []*wfSimFlow // every other sub-flow: paths, branches, boundary events
	parent        *wfSimFlow
	// wasBad marks a stored task already in the state MDL-WF02 reports.
	wasBad bool
	// unsure marks a task an operation may have changed in a way the model
	// cannot follow; it is never reported.
	unsure bool
}

type wfSimFlow struct {
	value string // a user task outcome's value; "" otherwise
	acts  []*wfSimNode
	owner *wfSimNode
}

func (n *wfSimNode) label() string {
	if n.name != "" {
		return "'" + n.name + "'"
	}
	if n.caption != "" {
		return "'" + n.caption + "'"
	}
	return "(unnamed)"
}

func (n *wfSimNode) singleOutcomeWithActivities() bool {
	return n.userTask && len(n.outcomes) == 1 && len(n.outcomes[0].acts) > 0
}

func (f *wfSimFlow) onlyEnd() bool {
	return len(f.acts) == 1 && f.acts[0].end
}

func (f *wfSimFlow) add(at int, nodes []*wfSimNode) {
	for _, n := range nodes {
		n.parent = f
	}
	acts := append([]*wfSimNode{}, f.acts[:at]...)
	acts = append(acts, nodes...)
	f.acts = append(acts, f.acts[at:]...)
}

func (f *wfSimFlow) remove(n *wfSimNode) int {
	for i, a := range f.acts {
		if a == n {
			f.acts = append(f.acts[:i:i], f.acts[i+1:]...)
			return i
		}
	}
	return -1
}

// newWfSimFromAST models a workflow the script creates. A task its body
// already breaks is the CREATE's finding, so it counts as broken before.
func newWfSimFromAST(acts []ast.WorkflowActivityNode) *wfSim {
	root := simFlowFromAST("", acts)
	markBroken(root)
	return &wfSim{known: true, roots: []*wfSimFlow{root}}
}

// newWfSimFromStored models a stored workflow; nil (no project, or no such
// workflow) leaves the base unknown.
func newWfSimFromStored(wf *workflows.Workflow) *wfSim {
	if wf == nil || wf.Flow == nil {
		return &wfSim{}
	}
	root := simFlowFromStored("", wf.Flow)
	markBroken(root)
	return &wfSim{known: true, roots: []*wfSimFlow{root}}
}

func markBroken(root *wfSimFlow) {
	walkSimFlow(root, func(n *wfSimNode) { n.wasBad = n.singleOutcomeWithActivities() })
}

func simFlowFromAST(value string, acts []ast.WorkflowActivityNode) *wfSimFlow {
	f := &wfSimFlow{value: value}
	for _, a := range acts {
		if a == nil {
			continue
		}
		n := &wfSimNode{parent: f}
		n.name, n.caption = workflowActivityNodeLabels(a)
		switch t := a.(type) {
		case *ast.WorkflowUserTaskNode:
			n.userTask = true
			for _, o := range t.Outcomes {
				n.outcomes = append(n.outcomes, simFlowFromAST(o.Caption, o.Activities))
			}
		case *ast.WorkflowEndNode:
			n.end = true
		case *ast.WorkflowDecisionNode:
			for _, o := range t.Outcomes {
				n.nested = append(n.nested, simFlowFromAST("", o.Activities))
			}
		case *ast.WorkflowCallMicroflowNode:
			for _, o := range t.Outcomes {
				n.nested = append(n.nested, simFlowFromAST("", o.Activities))
			}
		case *ast.WorkflowParallelSplitNode:
			for _, p := range t.Paths {
				n.nested = append(n.nested, simFlowFromAST("", p.Activities))
			}
		}
		for _, b := range workflowActivityBoundaryEvents(a) {
			n.nested = append(n.nested, simFlowFromAST("", b.Activities))
		}
		n.own()
		f.acts = append(f.acts, n)
	}
	return f
}

func simFlowFromStored(value string, flow *workflows.Flow) *wfSimFlow {
	f := &wfSimFlow{value: value}
	if flow == nil {
		return f
	}
	for _, a := range flow.Activities {
		if a == nil {
			continue
		}
		n := &wfSimNode{parent: f, name: a.GetName(), caption: a.GetCaption()}
		switch t := a.(type) {
		case *workflows.UserTask:
			n.userTask = true
			for _, o := range t.Outcomes {
				if o != nil {
					n.outcomes = append(n.outcomes, simFlowFromStored(o.Value, o.Flow))
				}
			}
			for _, b := range t.BoundaryEvents {
				if b != nil {
					n.nested = append(n.nested, simFlowFromStored("", b.Flow))
				}
			}
		case *workflows.EndWorkflowActivity:
			n.end = true
		default:
			for _, sub := range storedNestedFlows(a) {
				n.nested = append(n.nested, simFlowFromStored("", sub))
			}
		}
		n.own()
		f.acts = append(f.acts, n)
	}
	return f
}

// own points the node's sub-flows back at it.
func (n *wfSimNode) own() {
	for _, f := range n.outcomes {
		f.owner = n
	}
	for _, f := range n.nested {
		f.owner = n
	}
}

func walkSimFlow(f *wfSimFlow, visit func(*wfSimNode)) {
	for _, n := range f.acts {
		visit(n)
		for _, sub := range n.outcomes {
			walkSimFlow(sub, visit)
		}
		for _, sub := range n.nested {
			walkSimFlow(sub, visit)
		}
	}
}

// resolve finds the activity a reference addresses as the mutator does: by
// name or caption, a name winning without @N, @N choosing among the matches.
// It returns nil when there is no single match.
func (s *wfSim) resolve(ref string, atPos int) *wfSimNode {
	var all, named []*wfSimNode
	for _, root := range s.roots {
		walkSimFlow(root, func(n *wfSimNode) {
			if n.name == ref || n.caption == ref {
				all = append(all, n)
				if n.name == ref {
					named = append(named, n)
				}
			}
		})
	}
	if atPos > 0 {
		if atPos <= len(all) {
			return all[atPos-1]
		}
		return nil
	}
	if len(named) > 0 {
		all = named
	}
	if len(all) == 1 {
		return all[0]
	}
	return nil
}

// matches reports whether any modelled activity answers to ref.
func (s *wfSim) matches(ref string) bool {
	found := false
	for _, root := range s.roots {
		walkSimFlow(root, func(n *wfSimNode) {
			if n.name == ref || n.caption == ref {
				found = true
			}
		})
	}
	return found
}

// loose keeps activities whose place in the workflow is not modelled; the
// user tasks among them are still judged.
func (s *wfSim) loose(acts []ast.WorkflowActivityNode) {
	if len(acts) > 0 {
		s.roots = append(s.roots, simFlowFromAST("", acts))
	}
}

// distrust marks every user task an operation on ref could change: the
// matches themselves, what they hold, and the tasks whose outcomes hold them.
func (s *wfSim) distrust(ref string) {
	for _, root := range s.roots {
		walkSimFlow(root, func(n *wfSimNode) {
			if n.name != ref && n.caption != ref {
				return
			}
			n.unsure = true
			for _, sub := range append(append([]*wfSimFlow{}, n.outcomes...), n.nested...) {
				walkSimFlow(sub, func(m *wfSimNode) { m.unsure = true })
			}
			for f := n.parent; f != nil && f.owner != nil; f = f.owner.parent {
				f.owner.unsure = true
			}
		})
	}
}

// apply replays one ALTER operation.
func (s *wfSim) apply(op ast.AlterWorkflowOp) {
	if _, ok := op.(*ast.SetActivityPropertyOp); ok {
		return // a property, never an outcome or an activity
	}
	t, ok := alterWorkflowOpTarget(op)
	if !ok {
		return
	}
	ref, atPos := t.Path[0], t.Ordinal
	var target *wfSimNode
	if s.known {
		target = s.resolve(ref, atPos)
	}
	if target == nil && s.matches(ref) {
		// Without the base, a reference to an inserted activity may still
		// address a stored one; with it, an ambiguous reference is one the
		// mutator may settle differently (an inserted name that clashes is
		// renamed). Either way nothing the operation could change is judged.
		s.distrust(ref)
	}
	switch o := op.(type) {
	case *ast.InsertAfterOp:
		s.insertBeside(target, o.NewActivities, 1)
	case *ast.InsertBeforeOp:
		s.insertBeside(target, o.NewActivities, 0)
	case *ast.ReplaceActivityOp:
		if target == nil {
			s.loose(o.NewActivities)
			return
		}
		at := target.parent.remove(target)
		target.parent.add(at, simFlowFromAST("", o.NewActivities).acts)
	case *ast.DropActivityOp:
		if target != nil {
			target.parent.remove(target)
		}
	case *ast.InsertOutcomeOp:
		if target == nil || !target.userTask {
			s.loose(o.Activities)
			return
		}
		f := simFlowFromAST(o.OutcomeName, o.Activities)
		f.owner = target
		target.outcomes = append(target.outcomes, f)
	case *ast.DropOutcomeOp:
		if target == nil || !target.userTask {
			return
		}
		at := -1
		for i, f := range target.outcomes {
			if f.value == o.OutcomeName {
				if at >= 0 {
					return // two of that value: the mutator refuses
				}
				at = i
			}
		}
		if at >= 0 {
			target.outcomes = append(target.outcomes[:at:at], target.outcomes[at+1:]...)
		}
	case *ast.InsertPathOp:
		s.insertNested(target, o.Activities)
	case *ast.InsertBranchOp:
		s.insertNested(target, o.Activities)
	case *ast.InsertBoundaryEventOp:
		s.insertNested(target, o.Activities)
	case *ast.DropPathOp, *ast.DropBranchOp, *ast.DropBoundaryEventOp:
		// Which sub-flow goes is not modelled; whatever it held is not judged.
		if target != nil {
			for _, sub := range target.nested {
				walkSimFlow(sub, func(m *wfSimNode) { m.unsure = true })
			}
		}
	}
}

func (s *wfSim) insertBeside(target *wfSimNode, acts []ast.WorkflowActivityNode, after int) {
	if target == nil {
		s.loose(acts)
		return
	}
	at := 0
	for i, a := range target.parent.acts {
		if a == target {
			at = i + after
		}
	}
	target.parent.add(at, simFlowFromAST("", acts).acts)
}

func (s *wfSim) insertNested(target *wfSimNode, acts []ast.WorkflowActivityNode) {
	if target == nil {
		s.loose(acts)
		return
	}
	f := simFlowFromAST("", acts)
	f.owner = target
	target.nested = append(target.nested, f)
}

// newSingleOutcomeTasks lists the user tasks the script left with a single
// outcome holding activities, in describe order.
func (s *wfSim) newSingleOutcomeTasks() []*wfSimNode {
	var out []*wfSimNode
	for _, root := range s.roots {
		walkSimFlow(root, func(n *wfSimNode) {
			if n.singleOutcomeWithActivities() && !n.wasBad && !n.unsure {
				out = append(out, n)
			}
		})
	}
	return out
}
