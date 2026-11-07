// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// execAlterFlow handles `alter microflow|nanoflow Module.Name { … }`: a patch
// of the stored flow (ADR-0012 decision 3), never a rebuild.
//
// Every target is resolved against the flow AS STORED, before any operation
// runs, so an address means what `describe … with handles` showed: an
// ambiguity or a miss refuses the whole statement before anything changes, and
// a later operation cannot address what an earlier one inserted.
func execAlterFlow(ctx *ExecContext, s *ast.AlterFlowStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	// An annotation the fragment's builder would drop (mendixlabs/mxcli#1597).
	if err := validateAlterFlowRules(s); err != nil {
		return err
	}
	// An action the project's version does not have, in what the statement
	// splices in: statementVersionGates (ako/mxcli#1787).
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}
	a, err := loadAlterFlow(ctx, s)
	if err != nil {
		return err
	}
	mut, err := a.plan(ctx)
	if err != nil {
		return err
	}
	if err := mut.Save(); err != nil {
		return mdlerrors.NewBackend("save altered "+s.Kind(), err)
	}
	fmt.Fprintf(ctx.Output, "Altered %s %s\n", s.Kind(), s.Name)
	for _, c := range a.carriedIncludes {
		fmt.Fprintln(ctx.Output, carriedIncludesWarning(s.Name.String(), c))
	}
	return nil
}

// plan resolves every target of the statement against the flow as stored and
// applies its operations in memory, writing nothing: exec saves the result,
// and check reports the error exec would (ako/mxcli#876).
func (a *alterFlowContext) plan(ctx *ExecContext) (backend.MicroflowMutator, error) {
	s := a.stmt
	if err := a.resolveStoredNotes(); err != nil {
		return nil, err
	}
	targets := make([]mfmutator.Candidate, len(s.Operations))
	for i, op := range s.Operations {
		c, err := mfmutator.ResolveText(a.cands, op.Target)
		if err != nil {
			return nil, mdlerrors.NewValidation(fmt.Sprintf("alter %s %s: %s %s: %v", s.Kind(), s.Name, op.Op, op.Target, err))
		}
		targets[i] = c
	}
	return a.apply(ctx, s.Operations, targets)
}

// apply opens the stored flow for splicing and applies ops, each aimed at the
// candidate at the same index of targets (resolved against the flow as stored).
// It returns the mutator unsaved: the caller saves, or discards it on error so
// nothing is written.
func (a *alterFlowContext) apply(ctx *ExecContext, ops []*ast.AlterFlowOperation, targets []mfmutator.Candidate) (backend.MicroflowMutator, error) {
	mut, err := ctx.Backend.OpenMicroflowForMutation(a.mf.ID)
	if err != nil {
		return nil, mdlerrors.NewBackend("open "+a.stmt.Kind()+" for alter", err)
	}
	if err := a.applyTo(ctx, mut, ops, targets); err != nil {
		return nil, err
	}
	return mut, nil
}

// applyTo applies ops to an open mutator, as apply does.
func (a *alterFlowContext) applyTo(ctx *ExecContext, mut backend.MicroflowMutator, ops []*ast.AlterFlowOperation, targets []mfmutator.Candidate) error {
	s := a.stmt
	// A stored output the block takes away is judged against the flow the
	// whole block writes (mendixlabs/mxcli#1517): a reader that a later
	// operation replaces or drops is no user, and a fragment of any operation
	// that reads it is. So each check runs once every operation is applied.
	var outputChecks []func() error
	for i, op := range ops {
		target := targets[i]
		fail := func(err error) error {
			return mdlerrors.NewValidation(fmt.Sprintf("alter %s %s: %s %s: %v", s.Kind(), s.Name, op.Op, op.Target, err))
		}
		if op.Op == ast.AlterFlowReplaceBody {
			// What is inside a loop, replaced; the loop itself stays
			// (ako/mxcli#1693).
			if err := a.replaceBody(ctx, mut, op, target); err != nil {
				return fail(err)
			}
			continue
		}
		// Inside a loop body the splice inserts, drops and replaces plain
		// activities (ako/mxcli#1693). Anything else changes the body's shape
		// and is refused first, naming `replace body of` the loop — the
		// checks below would otherwise refuse it for a reason that is not why.
		inLoop := a.inLoopRefusal(target)
		if inLoop != nil {
			if why := inBodyTarget(op, target); why != "" {
				return fail(inLoop(why))
			}
		}
		if op.ReplaceNotes {
			// The statement states the activity's notes, so the stored ones
			// go with it rather than stay (ako/mxcli#859).
			if err := mut.RemoveNotes(target.ID); err != nil {
				return fail(err)
			}
		}
		// A decision taken out takes the activities on its paths with it
		// (ako/mxcli#1694): their outputs are judged like the target's.
		takenOut := []mfmutator.Candidate{target}
		if (op.Op == ast.AlterFlowDrop || op.Op == ast.AlterFlowReplace) && !op.SetCondition && !op.SetNotes {
			takenOut = append(takenOut, a.candidatesOf(mut.DecisionNodes(target.ID))...)
		}
		if op.Op == ast.AlterFlowDrop {
			for _, c := range takenOut {
				outputChecks = append(outputChecks, func() error {
					if err := a.checkOutputUnused(c, nil); err != nil {
						return fail(err)
					}
					return nil
				})
			}
			if err := mut.Drop(target.ID); err != nil {
				return fail(err)
			}
			for _, c := range takenOut {
				a.noteRemoved(c, nil)
			}
			continue
		}
		if op.SetNotes {
			// A decision's or an end event's notes, set in place
			// (mendixlabs/mxcli#1517).
			if err := a.setNodeNotes(ctx, mut, op, target); err != nil {
				return fail(err)
			}
			a.noteEdited(op)
			continue
		}
		if op.SetCondition {
			// An `if` whose condition changes is the stored decision with a
			// new condition, set in place (ako/mxcli#888).
			expr, caption, err := a.splitCondition(ctx, op, target)
			if err == nil {
				err = mut.SetCondition(target.ID, expr, caption)
			}
			if err != nil {
				return fail(err)
			}
			a.noteRewritten(target, "if "+expr)
			a.noteEdited(op)
			continue
		}
		if ret, ok := returnValueEdit(op, target); ok {
			// A return replacing an end event is a new value for it, set in
			// place (ako/mxcli#805): the end event, its flows and its notes stay.
			value, err := a.returnValue(ctx, ret)
			if err == nil {
				err = mut.SetReturnValue(target.ID, value)
			}
			if err != nil {
				return fail(err)
			}
			a.noteRewritten(target, "return "+value)
			a.noteEdited(op)
			continue
		}
		frag, vars, err := a.buildFragmentVars(ctx, op.Body)
		if err != nil {
			return fail(err)
		}
		// Lines from stored notes are drawn once the fragment is in place
		// (ako/mxcli#1695).
		noteLines := a.takeStoredNoteLines(frag)
		if op.Op == ast.AlterFlowReplace {
			// An activity with a custom error handler, replaced by one that
			// states the same handler, keeps the stored one (mendixlabs/mxcli#1489).
			if frag, err = a.keepStoredHandler(ctx, op, target, frag); err != nil {
				return fail(err)
			}
			// A call external action replaced by one that leaves out
			// `include associations` keeps the stored tree, as a create or
			// modify rebuild does (ako/mxcli#1914): the splice rebuilt the
			// call and dropped it.
			carried, err := carryIncludedAssociations(
				&microflows.MicroflowObjectCollection{Objects: frag.Objects},
				&microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{target.Object}},
				a.stmt.Name.String(), a.statedIncludes)
			if err != nil {
				return fail(err)
			}
			a.carriedIncludes = append(a.carriedIncludes, carried...)
		}
		if inLoop != nil {
			if what := notPlainFragment(frag); what != "" {
				return fail(inLoop("the fragment holds " + what))
			}
		}
		// A stated @position on the fragment is where it goes (ako/mxcli#818).
		frag.Placed = firstStatementPlaced(op.Body)
		if !frag.Placed && laterStatementPlaced(op.Body) {
			return fail(fmt.Errorf("a later inserted statement states @position but the first does not; the " +
				"inserted statements are placed as a whole, so state @position on the first one too, or on none"))
		}
		if err := a.checkFragmentScope(ctx, op, target, frag); err != nil {
			return fail(err)
		}
		// A variable the fragment declares that a stored activity declares
		// too is a clash — unless the block takes that activity away, as a
		// producer moved upstream is inserted there and dropped here
		// (ako/mxcli#1399). Whether it does is known once every operation
		// is applied, as for the outputs.
		declaring := frag
		outputChecks = append(outputChecks, func() error {
			if err := a.checkDeclaredUnclashed(declaring); err != nil {
				return fail(err)
			}
			return nil
		})
		switch op.Op {
		case ast.AlterFlowInsertAfter:
			err = mut.InsertAfter(target.ID, frag)
		case ast.AlterFlowInsertBefore:
			err = mut.InsertBefore(target.ID, frag)
		case ast.AlterFlowReplace:
			replacement := frag
			for _, c := range takenOut {
				outputChecks = append(outputChecks, func() error {
					if err := a.checkOutputUnused(c, replacement); err != nil {
						return fail(err)
					}
					return nil
				})
			}
			if err = mut.Replace(target.ID, frag); err == nil && mut.EditedInPlace(target.ID) {
				// One activity of the same kind: the stored one, edited
				// (ako/mxcli#1761).
				a.noteEdited(op)
			}
		default:
			err = fmt.Errorf("unknown operation")
		}
		if err == nil {
			err = linkStoredNotes(mut, noteLines)
		}
		if err != nil {
			return fail(err)
		}
		if op.Op == ast.AlterFlowReplace {
			for _, c := range takenOut {
				a.noteRemoved(c, frag)
			}
		}
		a.noteFragment(ctx, op, target, frag, vars)
	}
	for _, check := range outputChecks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// alterFlowContext is what the operations of one statement share: the stored
// flow (a nanoflow wrapped as a microflow, the way describe renders one), its
// addressable activities, and the name maps rendering needs.
type alterFlowContext struct {
	stmt           *ast.AlterFlowStmt
	mf             *microflows.Microflow
	cands          []mfmutator.Candidate
	entityNames    map[model.ID]string
	microflowNames map[model.ID]string

	// What the statement's earlier operations did to the variables, so a later
	// one is checked against the flow as it will be written, not as stored:
	// the variables their fragments declare, the ones their fragments read
	// (with who reads them), and the stored outputs they took away.
	declaredByOps map[string]bool
	// declaredAt says where each of those fragments went, so a later
	// operation counts a variable as declared only where it is on its path.
	declaredAt map[string][]declSite
	// opVars is what the variables declared by those fragments hold, as each
	// fragment's builder resolved them, so a later fragment is built knowing
	// them (ako/mxcli#1399): `set $Late = …` after an insert that declares
	// $Late, a member of an entity an inserted retrieve returns.
	opVars    fragmentVars
	readByOps map[string][]opRead
	// removedByOps are the stored outputs the statement's operations took
	// away without declaring them again.
	removedByOps map[string]bool
	// removedIDs are the stored activities the statement's operations take
	// out or rewrite (a return given a new value, a decision a new
	// condition); what they read as stored no longer counts as a use.
	removedIDs map[model.ID]bool

	// declaredVarTypes is the entity each variable holds as a full build of
	// the declared flow resolves it ("Module.Entity" or "List of
	// Module.Entity"), when the statement is a `create or modify` whose
	// declared body builds. A spliced fragment is built with it, so it writes
	// the members a full build of the same statement writes (ako/mxcli#885):
	// the stored flow alone does not say what every variable holds.
	declaredVarTypes map[string]string

	// storedDesc is the stored flow as describe prints it with full layout,
	// read once when an operation needs the stored statement behind a target
	// (storedStatementAt).
	storedDesc ast.Statement

	// storedNotes are the stored notes the statement's `@annotation(existing:
	// '…')` name, by text, once resolveStoredNotes has found exactly one for
	// each (ako/mxcli#1695). nil until then — and for the patch of a `create or
	// modify`, which states its notes and has none to resolve.
	storedNotes map[string]*microflows.Annotation

	// edited are the replace operations that edited the stored node in place
	// — a decision's condition, a return's value, a node's notes, an activity
	// replaced by one of the same kind — so that it kept its $ID, as against
	// one whose target was swapped for new elements (ako/mxcli#1761). The
	// report of a `create or modify` tells the two apart.
	edited map[*ast.AlterFlowOperation]bool
	// statedIncludes are the external action mappings whose fragment wrote
	// `include associations ( … )`, and carriedIncludes the stored trees a
	// replace kept because its statement did not (ako/mxcli#1914) — told by
	// whoever reports the write, not here, since check plans the same splice.
	statedIncludes  map[*microflows.ExternalActionParameterMapping]bool
	carriedIncludes []carriedIncludes
}

// noteEdited records that op edited its stored target in place.
func (a *alterFlowContext) noteEdited(op *ast.AlterFlowOperation) {
	if a.edited == nil {
		a.edited = map[*ast.AlterFlowOperation]bool{}
	}
	a.edited[op] = true
}

// candidatesOf returns the stored candidates with the given $IDs.
func (a *alterFlowContext) candidatesOf(ids []model.ID) []mfmutator.Candidate {
	want := map[model.ID]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []mfmutator.Candidate
	for _, c := range a.cands {
		if want[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// inLoopRefusal returns, for a target inside a loop body, the refusal of an
// operation the splice cannot make there, given why; nil at top level. The
// refusal names the innermost loop around the target and `replace body of`
// it, which makes any change of that body (ako/mxcli#1693).
func (a *alterFlowContext) inLoopRefusal(target mfmutator.Candidate) func(why string) error {
	loops := a.loopsAround(target.ID)
	if len(loops) == 0 {
		return nil
	}
	handle := "the loop"
	for i := range a.cands {
		if a.cands[i].ID == loops[0].ID {
			handle = a.cands[i].Statement
		}
	}
	s := a.stmt
	return func(why string) error {
		return fmt.Errorf("%s is inside the body of %s, and %s; %s. %s", target.Statement, handle, why,
			strings.Replace(plainInBodyNote, "the splice", "alter", 1), replaceBodyAdvice(s.Kind(), s.Name.String(), handle))
	}
}

// inBodyTarget says why an operation aimed at target, inside a loop body, is
// not one the splice makes there, or "" when it may be: an insert next to an
// activity or a loop, or a drop or replace of an activity without a custom
// error handler. A decision's condition, notes set in place on a decision or
// an end event, or an operation on any other node change the body's shape.
func inBodyTarget(op *ast.AlterFlowOperation, target mfmutator.Candidate) string {
	if op.SetCondition {
		return "its condition would change"
	}
	switch o := target.Object.(type) {
	case *microflows.ActionActivity:
		if microflows.IsCustomErrorHandling(microflows.ObjectErrorHandlingType(o)) &&
			(op.Op == ast.AlterFlowDrop || op.Op == ast.AlterFlowReplace) {
			return "it has an error handler"
		}
		if op.SetNotes {
			return "its notes would be set in place"
		}
		return ""
	case *microflows.LoopedActivity:
		if op.Op == ast.AlterFlowInsertAfter || op.Op == ast.AlterFlowInsertBefore {
			return ""
		}
		return "it is a loop"
	case *microflows.ExclusiveSplit, *microflows.InheritanceSplit:
		return "it is a decision"
	case *microflows.EndEvent:
		return "it is a return"
	case *microflows.ErrorEvent:
		return "it is a raise error"
	case *microflows.BreakEvent:
		return "it is a break"
	case *microflows.ContinueEvent:
		return "it is a continue"
	}
	return "it is not an activity"
}

// notPlainFragment says what a built fragment holds that is not a plain
// activity — the first decision, loop, event or error handler — or "" when it
// is a run of plain activities (with their notes), the only fragment the
// splice puts into a loop body (ako/mxcli#1693).
func notPlainFragment(frag *backend.MicroflowFragment) string {
	for _, obj := range frag.Objects {
		switch obj.(type) {
		case *microflows.ActionActivity, *microflows.Annotation:
		case *microflows.ExclusiveSplit, *microflows.InheritanceSplit:
			return "a decision"
		case *microflows.LoopedActivity:
			return "a loop"
		case *microflows.EndEvent:
			return "a return"
		case *microflows.ErrorEvent:
			return "a raise error"
		case *microflows.BreakEvent:
			return "a break"
		case *microflows.ContinueEvent:
			return "a continue"
		case *microflows.ExclusiveMerge:
			return "a merge"
		default:
			return "a " + strings.TrimPrefix(fmt.Sprintf("%T", obj), "*microflows.")
		}
	}
	for _, f := range frag.Flows {
		if f.IsErrorHandler {
			return "an activity with an error handler"
		}
	}
	if frag.ExitCase != nil {
		return "a decision"
	}
	return ""
}

// loopsAround returns the loops whose body holds the stored object id, the
// innermost first; none when id is at top level.
func (a *alterFlowContext) loopsAround(id model.ID) []*microflows.LoopedActivity {
	var path []*microflows.LoopedActivity
	var find func(oc *microflows.MicroflowObjectCollection) bool
	find = func(oc *microflows.MicroflowObjectCollection) bool {
		if oc == nil {
			return false
		}
		for _, obj := range oc.Objects {
			if obj.GetID() == id {
				return true
			}
			if l, ok := obj.(*microflows.LoopedActivity); ok {
				if find(l.ObjectCollection) {
					path = append(path, l)
					return true
				}
			}
		}
		return false
	}
	find(a.mf.ObjectCollection)
	return path
}

// iteratorsAround returns the iterator variables of the loops around the
// stored object id: they exist wherever it runs.
func (a *alterFlowContext) iteratorsAround(id model.ID) []string {
	var out []string
	for _, l := range a.loopsAround(id) {
		if src, ok := l.LoopSource.(*microflows.IterableList); ok && src.VariableName != "" {
			out = append(out, src.VariableName)
		}
	}
	return out
}

func loopHolds(loop *microflows.LoopedActivity, id model.ID) bool {
	if loop.ObjectCollection == nil {
		return false
	}
	for _, obj := range loop.ObjectCollection.Objects {
		if obj.GetID() == id {
			return true
		}
		if inner, ok := obj.(*microflows.LoopedActivity); ok && loopHolds(inner, id) {
			return true
		}
	}
	return false
}

// noteRemoved records that target's output is gone, unless the fragment that
// replaces it declares it again.
func (a *alterFlowContext) noteRemoved(target mfmutator.Candidate, replacement *backend.MicroflowFragment) {
	a.removedIDs[target.ID] = true
	v := target.OutputVariable
	if v == "" || fragmentDeclares(replacement, v) {
		return
	}
	a.removedByOps[v] = true
}

// noteRewritten records that the stored node target now reads text instead of
// what it read as stored: a return value or a condition set in place.
func (a *alterFlowContext) noteRewritten(target mfmutator.Candidate, text string) {
	a.removedIDs[target.ID] = true
	for _, m := range variableRef.FindAllStringSubmatch(withoutStringLiterals(text), -1) {
		a.readByOps[m[1]] = append(a.readByOps[m[1]], opRead{text: text, site: declSite{op: ast.AlterFlowReplace, target: target.ID}})
	}
}

// declSite is where an operation of the statement put a fragment that
// declares a variable: the operation, and the stored node it is aimed at.
type declSite struct {
	op     ast.AlterFlowOpKind
	target model.ID
}

// opRead is a read of a variable by what an operation of the statement puts
// in: the activity's text, where the operation puts it, and whether the
// fragment declares the variable itself.
type opRead struct {
	text string
	site declSite
	own  bool
}

// fragmentVars are the variables a fragment declares, as its builder keeps
// them: entity-typed ones with their entity ("M.E" / "List of M.E"), the rest
// with their type name.
type fragmentVars struct {
	varTypes, declared map[string]string
}

// noteFragment records what an inserted or replacing fragment declares and
// reads, and where it goes. vars is what its builder resolved the variables
// it declares to hold.
func (a *alterFlowContext) noteFragment(ctx *ExecContext, op *ast.AlterFlowOperation, target mfmutator.Candidate, frag *backend.MicroflowFragment, vars fragmentVars) {
	for _, obj := range frag.Objects {
		if act, ok := obj.(*microflows.ActionActivity); ok {
			if v := mfmutator.OutputVariable(act.Action); v != "" {
				a.declaredByOps[v] = true
				a.declaredAt[v] = append(a.declaredAt[v], declSite{op: op.Op, target: target.ID})
			}
		}
	}
	a.opVars = a.opVars.with(vars)
	site := declSite{op: op.Op, target: target.ID}
	a.fragmentReads(ctx, frag.Objects, func(v, text string) {
		a.readByOps[v] = append(a.readByOps[v], opRead{text: text, site: site, own: fragmentDeclares(frag, v)})
	})
}

// with returns fv with the variables of more added, a variable already there
// keeping what it holds.
func (fv fragmentVars) with(more fragmentVars) fragmentVars {
	add := func(into, from map[string]string) map[string]string {
		for name, t := range from {
			if into == nil {
				into = map[string]string{}
			}
			if _, ok := into[name]; !ok {
				into[name] = t
			}
		}
		return into
	}
	fv.varTypes = add(fv.varTypes, more.varTypes)
	fv.declared = add(fv.declared, more.declared)
	return fv
}

// takenAway reports whether the statement's operations take the stored object
// id out or rewrite it, itself or with a loop that holds it.
func (a *alterFlowContext) takenAway(id model.ID) bool {
	if a.removedIDs[id] {
		return true
	}
	for _, l := range a.loopsAround(id) {
		if a.removedIDs[l.ID] {
			return true
		}
	}
	return false
}

// fragmentReads calls read with each variable the objects read, and the
// activity text that reads it — inside a loop's body too, which the text of
// the loop itself (its header) does not show (mendixlabs/mxcli#1517: a
// replaced loop whose body read an undeclared variable was written, and mx
// check refused it with CE0109). A body's reads of the loop's iterator and of
// what the body itself declares are the body's own and are left out.
func (a *alterFlowContext) fragmentReads(ctx *ExecContext, objs []microflows.MicroflowObject, read func(v, text string)) {
	var walk func(objs []microflows.MicroflowObject, local map[string]bool)
	walk = func(objs []microflows.MicroflowObject, local map[string]bool) {
		for _, obj := range objs {
			text := formatActivity(ctx, obj, a.entityNames, a.microflowNames)
			for _, m := range variableRef.FindAllStringSubmatch(withoutStringLiterals(text), -1) {
				if !local[m[1]] {
					read(m[1], text)
				}
			}
			loop, ok := obj.(*microflows.LoopedActivity)
			if !ok || loop.ObjectCollection == nil {
				continue
			}
			inner := map[string]bool{}
			for v := range local {
				inner[v] = true
			}
			if src, ok := loop.LoopSource.(*microflows.IterableList); ok && src.VariableName != "" {
				inner[src.VariableName] = true
			}
			for _, b := range loop.ObjectCollection.Objects {
				if act, ok := b.(*microflows.ActionActivity); ok {
					if v := mfmutator.OutputVariable(act.Action); v != "" {
						inner[v] = true
					}
				}
			}
			walk(loop.ObjectCollection.Objects, inner)
		}
	}
	walk(objs, nil)
}

func fragmentDeclares(frag *backend.MicroflowFragment, v string) bool {
	if frag == nil {
		return false
	}
	for _, obj := range frag.Objects {
		if act, ok := obj.(*microflows.ActionActivity); ok && mfmutator.OutputVariable(act.Action) == v {
			return true
		}
	}
	return false
}

func loadAlterFlow(ctx *ExecContext, s *ast.AlterFlowStmt) (*alterFlowContext, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("build hierarchy", err)
	}
	a := &alterFlowContext{stmt: s, entityNames: getEntityNames(ctx, h),
		declaredByOps: map[string]bool{}, declaredAt: map[string][]declSite{}, readByOps: map[string][]opRead{}, removedByOps: map[string]bool{},
		removedIDs: map[model.ID]bool{}}
	// A copy: nanoflow names are added below, and the cached map is shared.
	a.microflowNames = map[model.ID]string{}
	for id, n := range getMicroflowNames(ctx, h) {
		a.microflowNames[id] = n
	}
	inModule := func(container model.ID, name string) bool {
		return h.GetModuleName(h.FindModuleID(container)) == s.Name.Module && name == s.Name.Name
	}
	if s.Nanoflow {
		nfs, err := ctx.Backend.ListNanoflows()
		if err != nil {
			return nil, mdlerrors.NewBackend("list nanoflows", err)
		}
		for _, nf := range nfs {
			a.microflowNames[nf.ID] = h.GetQualifiedName(nf.ContainerID, nf.Name)
		}
		nf, ok := pickLive(nfs,
			func(nf *microflows.Nanoflow) bool { return inModule(nf.ContainerID, nf.Name) },
			func(nf *microflows.Nanoflow) bool { return nf.Excluded })
		if !ok {
			return nil, mdlerrors.NewNotFound("nanoflow", s.Name.String())
		}
		a.mf = &microflows.Microflow{
			BaseElement:        nf.BaseElement,
			ContainerID:        nf.ContainerID,
			Name:               nf.Name,
			Parameters:         nf.Parameters,
			ReturnType:         nf.ReturnType,
			ReturnVariableName: nf.ReturnVariableName,
			ObjectCollection:   nf.ObjectCollection,
		}
	} else {
		mfs, err := microflowsNamed(ctx, s.Name.Name)
		if err != nil {
			return nil, mdlerrors.NewBackend("list microflows", err)
		}
		mf, ok := pickLive(mfs,
			func(mf *microflows.Microflow) bool { return inModule(mf.ContainerID, mf.Name) },
			func(mf *microflows.Microflow) bool { return mf.Excluded })
		if !ok {
			return nil, mdlerrors.NewNotFound("microflow", s.Name.String())
		}
		a.mf = mf
	}
	if a.mf.ObjectCollection == nil {
		return nil, mdlerrors.NewValidation(fmt.Sprintf("%s %s has no flow to alter", s.Kind(), s.Name))
	}
	a.cands, _, _, _ = microflowTargets(ctx, a.mf, a.entityNames, a.microflowNames)
	return a, nil
}

// returnValueEdit reports whether op replaces an end event with a single
// return, which is an edit of the value the end event returns rather than a
// fragment: an end event ends its path, so nothing could lead on from a
// fragment put in its place.
func returnValueEdit(op *ast.AlterFlowOperation, target mfmutator.Candidate) (*ast.ReturnStmt, bool) {
	if op.Op != ast.AlterFlowReplace || len(op.Body) != 1 {
		return nil, false
	}
	if _, ok := target.Object.(*microflows.EndEvent); !ok {
		return nil, false
	}
	ret, ok := op.Body[0].(*ast.ReturnStmt)
	return ret, ok
}

// returnValue renders a return's value as the expression the end event
// stores, the way the builder writes it for `create microflow`.
func (a *alterFlowContext) returnValue(ctx *ExecContext, ret *ast.ReturnStmt) (string, error) {
	if ret.Value == nil {
		return "", nil
	}
	fb := a.fragmentBuilder(ctx)
	value := fb.exprToString(ret.Value)
	if errs := fb.GetErrors(); len(errs) > 0 {
		return "", fmt.Errorf("the return value has errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return value, nil
}

// splitCondition renders the condition of a SetCondition operation as the
// expression a decision stores, and the caption the decision gets with it: a
// caption that read as the old condition (the builder's default, which
// describe leaves out) reads as the new one, and any other is kept — the
// declared `if` states the same @caption as the stored one, or the operation
// would not be a condition edit.
func (a *alterFlowContext) splitCondition(ctx *ExecContext, op *ast.AlterFlowOperation, target mfmutator.Candidate) (expr, caption string, err error) {
	split, ok := target.Object.(*microflows.ExclusiveSplit)
	if !ok || len(op.Body) != 1 {
		return "", "", fmt.Errorf("a condition can be set on a decision only")
	}
	ifs, ok := op.Body[0].(*ast.IfStmt)
	if !ok {
		return "", "", fmt.Errorf("a condition can be set from an if only")
	}
	old, ok := split.SplitCondition.(*microflows.ExpressionSplitCondition)
	if !ok {
		return "", "", fmt.Errorf("the decision calls a rule; the splice sets an expression's text only")
	}
	fb := a.fragmentBuilder(ctx)
	if fb.tryBuildRuleSplitCondition(ifs.Condition) != nil {
		return "", "", fmt.Errorf("the new condition calls a rule, which would replace the decision's expression; " +
			"the splice sets an expression's text only")
	}
	expr = fb.exprToString(ifs.Condition)
	fb.buildSplitCondition(ifs.Condition, expr) // reports a call that is not a rule
	if errs := fb.GetErrors(); len(errs) > 0 {
		return "", "", fmt.Errorf("the condition has errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	if missing := a.unscopedRefs(expr, target.ID); len(missing) > 0 {
		return "", "", fmt.Errorf("the condition uses %s, which is not declared on the path to the decision", strings.Join(missing, ", "))
	}
	// The `if` states the condition as describe prints it, which is the stored
	// one: describe no longer folds a decision whose true path goes straight
	// on into not(X) with its paths swapped (ako/mxcli#1963), so there is no
	// fold to undo here (#1930).
	caption = split.Caption
	if caption == old.Expression {
		caption = expr
	}
	// A caption the statement changes (mendixlabs/mxcli#1610): the stated
	// one, or — taken off — the condition, as the builder captions a
	// decision with no @caption.
	if ann := ifs.Annotations; ann != nil {
		caption = expr
		if ann.CaptionSet {
			caption = ann.Caption
		}
	}
	return expr, caption, nil
}

// fragmentBuilder is the builder `create microflow` uses, seeded with the
// variables the stored flow declares and those the statement's earlier
// operations declare (ako/mxcli#1399). Seeding a variable only lets the
// fragment build; whether it is in scope where the fragment goes is
// checkFragmentScope's to say.
func (a *alterFlowContext) fragmentBuilder(ctx *ExecContext) *flowBuilder {
	varTypes, declared := a.storedVariables(ctx)
	for name, t := range a.opVars.varTypes {
		varTypes[name] = t
		delete(declared, name)
	}
	for name, t := range a.opVars.declared {
		if _, ok := varTypes[name]; !ok {
			declared[name] = t
		}
	}
	for name, t := range a.declaredVarTypes {
		varTypes[name] = t
		delete(declared, name)
	}
	hierarchy, _ := getHierarchy(ctx)
	restServices, _ := loadRestServices(ctx)
	return &flowBuilder{
		textLang:     authoringLanguage(ctx),
		posX:         200,
		posY:         200,
		baseY:        200,
		spacing:      HorizontalSpacing,
		varTypes:     varTypes,
		declaredVars: declared,
		measurer:     &layoutMeasurer{varTypes: varTypes},
		backend:      flowBuildBackend(ctx.Backend),
		hierarchy:    hierarchy,
		restServices: restServices,
		isNanoflow:   a.stmt.Nanoflow,
		scriptFlows:  ctx.scriptFlows,
		storedNotes:  a.storedNotes,

		// A fragment is spliced into a stored flow: a member it cannot
		// qualify is refused, never written bare (ako/mxcli#885).
		qualifiedMembersOnly: true,
	}
}

// buildFragment builds a fragment's statements with the builder `create
// microflow` uses, seeded with the variables the stored flow declares, and
// cuts it out of the start and end events the builder wraps it in.
func (a *alterFlowContext) buildFragment(ctx *ExecContext, body []ast.MicroflowStatement) (*backend.MicroflowFragment, error) {
	frag, _, err := a.buildFragmentVars(ctx, body)
	return frag, err
}

// buildFragmentVars is buildFragment that also returns the variables the
// fragment declares — the outputs of its own activities, not of a loop's body —
// as its builder resolved them.
func (a *alterFlowContext) buildFragmentVars(ctx *ExecContext, body []ast.MicroflowStatement) (*backend.MicroflowFragment, fragmentVars, error) {
	if len(body) == 0 {
		return nil, fragmentVars{}, fmt.Errorf("the fragment is empty; use drop to remove an activity")
	}
	fb := a.fragmentBuilder(ctx)
	oc := fb.buildFlowGraph(body, nil)
	if errs := fb.GetErrors(); len(errs) > 0 {
		return nil, fragmentVars{}, fmt.Errorf("the fragment has errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	if fb.endsWithReturn {
		return nil, fragmentVars{}, fmt.Errorf("every path through the fragment ends the flow with a return, so nothing would lead on " +
			"to the rest of it; a return can be inserted only on a path that branches off the rest (a guard clause)")
	}
	frag, err := cutFragment(oc, fb.fallThroughEndID, fb.returnEndIDs)
	if err != nil {
		return nil, fragmentVars{}, err
	}
	frag.Pinned = fb.statedNotes
	for pm := range fb.statedIncludes {
		if a.statedIncludes == nil {
			a.statedIncludes = map[*microflows.ExternalActionParameterMapping]bool{}
		}
		a.statedIncludes[pm] = true
	}
	return frag, declaredVarsOf(fb, frag.Objects), nil
}

// declaredVarsOf returns the variables objs declare — the outputs of their
// own activities, not of a loop's body — as fb, which built them, resolved
// them.
func declaredVarsOf(fb *flowBuilder, objs []microflows.MicroflowObject) fragmentVars {
	vars := fragmentVars{varTypes: map[string]string{}, declared: map[string]string{}}
	for _, obj := range objs {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		v := mfmutator.OutputVariable(act.Action)
		if v == "" {
			continue
		}
		if t, ok := fb.varTypes[v]; ok {
			vars.varTypes[v] = t
		} else if t, ok := fb.declaredVars[v]; ok {
			vars.declared[v] = t
		}
	}
	return vars
}

// cutFragment removes the builder's start event and the end event the body
// falls through to (fallThrough), and says where the fragment is entered and
// left. Several paths reaching that end (an if without a merge before it, an
// error handler that rejoins at the end) are joined by a merge, which becomes
// the exit.
//
// An end event a `return` in the fragment drew (returns) is part of the
// fragment: a guard clause's return ends its own path, which branches off the
// rest of the flow, and is written as a new end event of the flow where the
// builder drew it relative to the fragment (ako/mxcli#888). So is the return
// that ends an error handler of the fragment (ako/mxcli#905). Any other end
// event is one the builder adds to end a handler that states no return, with
// the flow's default value, and is refused.
func cutFragment(oc *microflows.MicroflowObjectCollection, fallThrough model.ID, returns map[model.ID]bool) (*backend.MicroflowFragment, error) {
	var start, end microflows.MicroflowObject
	for _, obj := range oc.Objects {
		switch obj.(type) {
		case *microflows.StartEvent:
			start = obj
		case *microflows.EndEvent:
			switch {
			case obj.GetID() == fallThrough:
				end = obj
			case !returns[obj.GetID()]:
				return nil, fmt.Errorf("an error handler in the fragment ends at an end event the script does not state; " +
					"an inserted error handler has to rejoin the rest of the flow or end in a return of its own")
			}
		}
	}
	if start == nil || end == nil {
		return nil, fmt.Errorf("the fragment does not continue: its last statement ends the flow, so nothing would lead on to the rest of it")
	}
	frag := &backend.MicroflowFragment{}
	var intoEnd []*microflows.SequenceFlow
	for _, f := range oc.Flows {
		switch {
		case f.OriginID == start.GetID():
			if frag.Entry != "" {
				return nil, fmt.Errorf("the fragment starts with more than one flow")
			}
			frag.Entry = f.DestinationID
		case f.DestinationID == end.GetID():
			intoEnd = append(intoEnd, f)
		default:
			frag.Flows = append(frag.Flows, f)
		}
	}
	for _, obj := range oc.Objects {
		if obj != start && obj != end {
			frag.Objects = append(frag.Objects, obj)
		}
	}
	frag.AnnotationFlows = oc.AnnotationFlows
	switch {
	case frag.Entry == "" || frag.Entry == end.GetID() || len(frag.Objects) == 0:
		return nil, fmt.Errorf("the fragment builds no activity")
	case len(intoEnd) == 0:
		return nil, fmt.Errorf("no path through the fragment leads on to the rest of the flow")
	case len(intoEnd) == 1:
		frag.Exit = intoEnd[0].OriginID
		// The flow leaving a decision that ends the fragment is one of its
		// paths, and carries its case on (ako/mxcli#888).
		switch cv := intoEnd[0].CaseValue; cv.(type) {
		case nil, microflows.NoCase, *microflows.NoCase:
		default:
			frag.ExitCase = cv
		}
	default:
		p := end.GetPosition()
		merge := &microflows.ExclusiveMerge{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    p,
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		}}
		for _, f := range intoEnd {
			f.DestinationID = merge.ID
			frag.Flows = append(frag.Flows, f)
		}
		frag.Objects = append(frag.Objects, merge)
		frag.Exit = merge.ID
	}
	return frag, nil
}

// storedVariables returns the variables the stored flow declares, in the two
// maps the builder keeps: entity-typed ones with their entity (a change or a
// member access resolves attributes through it), and the rest as declared.
func (a *alterFlowContext) storedVariables(ctx *ExecContext) (varTypes, declared map[string]string) {
	varTypes, declared = map[string]string{}, map[string]string{}
	add := func(name string, dt microflows.DataType) {
		if name == "" {
			return
		}
		t := "Unknown"
		if dt != nil {
			t = formatMicroflowDataType(ctx, dt, a.entityNames)
		}
		switch dt.(type) {
		case *microflows.ObjectType, *microflows.ListType:
			varTypes[name] = t
		default:
			declared[name] = t
		}
	}
	for _, p := range a.mf.Parameters {
		add(p.Name, p.Type)
	}
	// Every output is an origin of its name. What is typed from another
	// variable — a retrieve over an association from its start, a loop's
	// iterator or a list operation's output from its list, a Java action's
	// list of its type parameter from an argument — is typed once the rest
	// are, below. What is typed outright is an origin too (dependsOnNothing),
	// so two origins of one name that type it differently leave it untyped,
	// whatever their kind: a direct one used to win over a dependent one, and
	// of two direct ones the last (ako/mxcli#1796).
	deps := storedDependents{}
	fb := &flowBuilder{backend: ctx.Backend}
	direct := func(name, entityType, declaredAs string) {
		if entityType == "" {
			declared[name] = declaredAs
		} else {
			declared[name] = "Unknown"
		}
		deps.add(name, storedDependent{how: dependsOnNothing, typ: entityType})
	}
	directOf := func(name string, dt microflows.DataType) {
		switch dt.(type) {
		case *microflows.ObjectType, *microflows.ListType:
			direct(name, formatMicroflowDataType(ctx, dt, a.entityNames), "")
		case nil:
			direct(name, "", "Unknown")
		default:
			direct(name, "", formatMicroflowDataType(ctx, dt, a.entityNames))
		}
	}
	entityName := func(qn string, id model.ID) string {
		if qn != "" {
			return qn
		}
		return a.entityNames[id]
	}
	for _, c := range a.cands {
		if loop, ok := c.Object.(*microflows.LoopedActivity); ok {
			if src, ok := loop.LoopSource.(*microflows.IterableList); ok {
				deps.add(src.VariableName, storedDependent{how: dependsOnList, list: listResultElement, from: src.ListVariableName})
			}
			continue
		}
		act, ok := c.Object.(*microflows.ActionActivity)
		if !ok || c.OutputVariable == "" {
			continue
		}
		switch x := act.Action.(type) {
		case *microflows.CreateVariableAction:
			directOf(c.OutputVariable, x.DataType)
		case *microflows.CreateObjectAction:
			direct(c.OutputVariable, entityName(x.EntityQualifiedName, x.EntityID), "Object")
		case *microflows.CreateListAction:
			// `create list of`, typed as the full build types it
			// (addCreateListAction).
			if e := entityName(x.EntityQualifiedName, x.EntityID); e != "" {
				direct(c.OutputVariable, "List of "+e, "")
			} else {
				direct(c.OutputVariable, "", "Unknown")
			}
		case *microflows.CastAction:
			// A cast in a type split's case holds that case's
			// specialization, as the full build types a cast in a case body.
			if e := a.castCaseEntity(act.ID); e != "" {
				direct(c.OutputVariable, e, "")
			} else {
				direct(c.OutputVariable, "", "Unknown")
			}
		case *microflows.RetrieveAction:
			// A database retrieve names its entity; one over an association
			// yields an end of it, which depends on the end it starts from
			// and is typed below (ako/mxcli#1769).
			if src, ok := x.Source.(*microflows.AssociationRetrieveSource); ok {
				declared[c.OutputVariable] = "Unknown"
				deps.add(c.OutputVariable, storedDependent{how: dependsOnAssociation, from: src.StartVariable, assoc: src.AssociationQualifiedName})
				break
			}
			src, ok := x.Source.(*microflows.DatabaseRetrieveSource)
			entity := ""
			if ok {
				entity = entityName(src.EntityQualifiedName, src.EntityID)
			}
			switch {
			case entity == "":
				direct(c.OutputVariable, "", "Unknown")
			case src.Range != nil && src.Range.RangeType == microflows.RangeTypeFirst:
				direct(c.OutputVariable, entity, "")
			default:
				direct(c.OutputVariable, "List of "+entity, "")
			}
		case *microflows.MicroflowCallAction:
			// A call's result has the called flow's return type.
			var rt microflows.DataType
			if x.MicroflowCall != nil {
				rt = fb.lookupMicroflowReturnType(x.MicroflowCall.Microflow)
			}
			directOfEntity(directOf, direct, c.OutputVariable, rt)
		case *microflows.NanoflowCallAction:
			var rt microflows.DataType
			if x.NanoflowCall != nil {
				rt = fb.lookupNanoflowReturnType(x.NanoflowCall.Nanoflow)
			}
			directOfEntity(directOf, direct, c.OutputVariable, rt)
		case *microflows.ImportXmlAction:
			// An import mapping's result: its root entity, one object or a
			// list (mappingResultType, the full build's own reading).
			direct(c.OutputVariable, mappingResultType(x.ResultHandling), "Unknown")
		case *microflows.RestCallAction:
			h, _ := x.ResultHandling.(*microflows.ResultHandlingMapping)
			direct(c.OutputVariable, mappingResultType(h), "Unknown")
		case *microflows.ListOperationAction:
			// The output holds what the operation keeps of its input list,
			// as the full build types it (ako/mxcli#1777); a contains or an
			// equals holds a Boolean.
			declared[c.OutputVariable] = "Unknown"
			if r := listOperationResult(x.Operation); r != listResultNone {
				deps.add(c.OutputVariable, storedDependent{how: dependsOnList, list: r, from: listOperationInput(x.Operation)})
			} else {
				deps.add(c.OutputVariable, storedDependent{how: dependsOnNothing})
			}
		case *microflows.AggregateListAction:
			direct(c.OutputVariable, "", aggregateResultType(x.Function, x.ReduceReturnType))
		case *microflows.JavaActionCallAction:
			// The result has the action's declared return type, read the
			// way the full build reads it (ako/mxcli#1777); a list of a type
			// parameter takes the list an argument holds.
			var ja *javaactions.JavaAction
			if fb.backend != nil && x.JavaAction != "" {
				ja, _ = fb.backend.ReadJavaActionByName(x.JavaAction)
			}
			switch {
			case ja == nil:
				direct(c.OutputVariable, "", "Unknown")
			case returnsTypeParameterList(ja):
				declared[c.OutputVariable] = "Unknown"
				deps.add(c.OutputVariable, storedDependent{how: dependsOnFirstList, from: strings.Join(storedArgVariables(x), ",")})
			default:
				direct(c.OutputVariable, javaActionResultType(ja, nil, nil), "Unknown")
			}
		default:
			direct(c.OutputVariable, "", "Unknown")
		}
	}
	deps.typeAll(fb, varTypes, declared)
	return varTypes, declared
}

// directOfEntity registers a call's result: its return type when that holds
// an entity, otherwise a name declared Unknown, as the full build declares a
// call whose result it does not type.
func directOfEntity(directOf func(string, microflows.DataType), direct func(string, string, string), name string, rt microflows.DataType) {
	switch rt.(type) {
	case *microflows.ObjectType, *microflows.ListType:
		directOf(name, rt)
	default:
		direct(name, "", "Unknown")
	}
}

// castCaseEntity is the specialization a stored cast holds: the case of the
// type split whose branch reaches it through plain activities only. That is
// the full build's rule — a cast among a case body's own statements is typed
// with the case — so a cast past a merge, inside a nested decision, or not
// after a type split at all is "" and stays untyped.
func (a *alterFlowContext) castCaseEntity(id model.ID) string {
	oc := a.collectionOf(id)
	if oc == nil {
		return ""
	}
	objects := map[model.ID]microflows.MicroflowObject{}
	for _, o := range oc.Objects {
		objects[o.GetID()] = o
	}
	for cur, seen := id, map[model.ID]bool{}; !seen[cur]; {
		seen[cur] = true
		var in []*microflows.SequenceFlow
		for _, f := range oc.Flows {
			if f.DestinationID == cur && !f.IsErrorHandler {
				in = append(in, f)
			}
		}
		if len(in) != 1 {
			return ""
		}
		switch objects[in[0].OriginID].(type) {
		case *microflows.InheritanceSplit:
			switch c := in[0].CaseValue.(type) {
			case *microflows.InheritanceCase:
				return a.caseEntity(c.EntityQualifiedName, c.EntityID)
			case microflows.InheritanceCase:
				return a.caseEntity(c.EntityQualifiedName, c.EntityID)
			}
			return ""
		case *microflows.ActionActivity:
			cur = in[0].OriginID
		default:
			return ""
		}
	}
	return ""
}

func (a *alterFlowContext) caseEntity(qn string, id model.ID) string {
	if qn != "" {
		return qn
	}
	return a.entityNames[id]
}

// collectionOf is the object collection of the stored flow, or of a loop in
// it, that holds the object id.
func (a *alterFlowContext) collectionOf(id model.ID) *microflows.MicroflowObjectCollection {
	var find func(oc *microflows.MicroflowObjectCollection) *microflows.MicroflowObjectCollection
	find = func(oc *microflows.MicroflowObjectCollection) *microflows.MicroflowObjectCollection {
		if oc == nil {
			return nil
		}
		for _, o := range oc.Objects {
			if o.GetID() == id {
				return oc
			}
			if l, ok := o.(*microflows.LoopedActivity); ok {
				if found := find(l.ObjectCollection); found != nil {
					return found
				}
			}
		}
		return nil
	}
	if a.mf == nil {
		return nil
	}
	return find(a.mf.ObjectCollection)
}

// storedArgVariable is an argument a stored Java action call passes as a
// variable as it stands: `$Lines`.
var storedArgVariable = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)$`)

// storedArgVariables are the variables a stored Java action call passes as
// arguments as they stand, in the order it stores them: what
// javaActionArgVariables is to the statement the full build reads.
func storedArgVariables(x *microflows.JavaActionCallAction) []string {
	var vars []string
	for _, m := range x.ParameterMappings {
		if v, ok := m.Value.(*microflows.BasicCodeActionParameterValue); ok {
			if sub := storedArgVariable.FindStringSubmatch(strings.TrimSpace(v.Argument)); sub != nil {
				vars = append(vars, sub[1])
			}
		}
	}
	return vars
}

// storedDependent is a stored variable whose type follows from another's, or
// is one a different origin of the same name can contradict; how says which.
type storedDependent struct {
	how storedDependence
	// from is the variable it follows from; for dependsOnFirstList, the
	// variables a Java action call passes, comma-separated.
	from string
	// assoc is the association a retrieve follows (dependsOnAssociation).
	assoc string
	// list is what it holds of the list from holds (dependsOnList).
	list listResult
	// typ is its type outright (dependsOnNothing).
	typ string
}

// storedDependence is how a stored dependent's type follows.
type storedDependence int

const (
	// dependsOnList: from a list, by list — a loop's iterator holds an
	// element of the list it walks, a list operation's output the list or
	// an element of it (listOperationResult).
	dependsOnList storedDependence = iota
	// dependsOnAssociation: the output of a retrieve over assoc from the
	// variable from (flowBuilder.associationRetrieveType).
	dependsOnAssociation
	// dependsOnFirstList: the result of a Java action returning a list of
	// its type parameter, the list the first of from that holds one holds
	// (javaActionResultType).
	dependsOnFirstList
	// dependsOnNothing: typ, a Java action's declared return type.
	dependsOnNothing
)

// storedDependents are the stored flow's dependent variables by name. A name
// two of them give different origins is nil: which one a fragment means
// depends on where it goes (two loops can share an iterator name, two loop
// bodies a list operation's output), and a guess would qualify a member with
// the wrong entity. It stays untyped and the member is refused. Two origins
// that say the same — a head and a find of one list, two calls returning one
// entity — are one.
type storedDependents map[string]*storedDependent

func (d storedDependents) add(name string, dep storedDependent) {
	if name == "" {
		return
	}
	if old, seen := d[name]; seen {
		if old == nil || *old != dep {
			d[name] = nil
		}
		return
	}
	d[name] = &dep
}

// typeAll types every dependent whose origin is typed — a parameter, a
// database retrieve, another dependent — whatever order the stored flow lists
// them in, until a pass types nothing new. A name the rest of the stored flow
// already types keeps that type.
func (d storedDependents) typeAll(fb *flowBuilder, varTypes, declared map[string]string) {
	for progress := true; progress; {
		progress = false
		for name, dep := range d {
			if dep == nil {
				continue
			}
			if _, typed := varTypes[name]; typed {
				continue
			}
			if t := d.typeOf(fb, dep, varTypes); t != "" {
				varTypes[name] = t
				delete(declared, name)
				progress = true
			}
		}
	}
}

// typeOf is the type dep gives its variable with varTypes as typed so far,
// through the full build's own typing; "" while what it follows from is not
// typed, or when it holds no entity.
func (d storedDependents) typeOf(fb *flowBuilder, dep *storedDependent, varTypes map[string]string) string {
	switch dep.how {
	case dependsOnNothing:
		return dep.typ
	case dependsOnFirstList:
		// The first argument that holds a list, once each of them that
		// another dependent may still type is typed.
		args := strings.Split(dep.from, ",")
		for _, v := range args {
			if _, typed := varTypes[v]; !typed && d[v] != nil {
				return ""
			}
		}
		return firstListType(args, varTypes)
	}
	from, ok := varTypes[dep.from]
	if !ok {
		return ""
	}
	if dep.how == dependsOnAssociation {
		return fb.associationRetrieveType(from, dep.assoc)
	}
	return dep.list.typeFrom(from)
}

// systemVariables are in scope everywhere they exist at all; the platform
// reports a misuse (a $latestError outside an error handler) itself.
var systemVariables = map[string]bool{
	"currentUser": true, "currentSession": true, "currentObject": true, "currentDeviceType": true,
	"latestError": true, "latestHttpResponse": true, "latestSoapFault": true,
}

var variableRef = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// withoutStringLiterals blanks the contents of every '…' string literal in an
// activity's MDL text, so a `$` inside one — `'/odata?$filter='` — is not read
// as a variable reference by the scope checks below (ako/mxcli#859, rehearsal
// M4). describe doubles a quote inside a string in both language versions.
func withoutStringLiterals(text string) string {
	b := []byte(text)
	in := false
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '\'' && in && i+1 < len(b) && b[i+1] == '\'':
			b[i], b[i+1] = ' ', ' '
			i++
		case b[i] == '\'':
			in = !in
		case in:
			b[i] = ' '
		}
	}
	return string(b)
}

// checkFragmentScope is plan item 4.2d: the fragment is checked in the scope
// of its insertion point. A variable it declares that the flow already has is
// an error (it would shadow or clash with the stored one; for a stored
// activity's output, see checkDeclaredUnclashed); a variable it uses that is
// not declared upstream of where it goes, nor by the fragment itself, is an
// error too, since the fragment would read something that does not exist yet
// on that path.
func (a *alterFlowContext) checkFragmentScope(ctx *ExecContext, op *ast.AlterFlowOperation, target mfmutator.Candidate, frag *backend.MicroflowFragment) error {
	// A stored activity declaring the same variable is a clash only if the
	// statement keeps it, which is known once every operation is applied
	// (checkDeclaredUnclashed); a parameter is never taken away.
	existing := map[string]bool{}
	for _, p := range a.mf.Parameters {
		existing[p.Name] = true
	}
	own := map[string]bool{}
	for _, obj := range frag.Objects {
		// A loop's iterator exists only inside the loop, which the fragment
		// brings along; it reads it there, so it is the fragment's own.
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			if src, ok := loop.LoopSource.(*microflows.IterableList); ok && src.VariableName != "" {
				own[src.VariableName] = true
			}
			continue
		}
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		v := mfmutator.OutputVariable(act.Action)
		if v == "" {
			continue
		}
		replacingSame := op.Op == ast.AlterFlowReplace && v == target.OutputVariable
		if existing[v] && !replacingSame {
			return fmt.Errorf("the fragment declares $%s, which the %s already has; choose another name", v, a.stmt.Kind())
		}
		if a.declaredByOps[v] {
			return fmt.Errorf("the fragment declares $%s, which an earlier operation of this alter already declares; choose another name", v)
		}
		own[v] = true
	}

	inScope := map[string]bool{}
	for _, p := range a.mf.Parameters {
		inScope[p.Name] = true
	}
	for _, v := range a.iteratorsAround(target.ID) {
		inScope[v] = true
	}
	for id := range a.upstreamOf(target.ID, op.Op == ast.AlterFlowInsertAfter) {
		for _, c := range a.cands {
			if c.ID == id && c.OutputVariable != "" {
				inScope[c.OutputVariable] = true
			}
		}
	}
	var missing []string
	seen := map[string]bool{}
	a.fragmentReads(ctx, frag.Objects, func(v, _ string) {
		if seen[v] || systemVariables[v] || own[v] || (inScope[v] && !a.removedByOps[v]) ||
			a.declaredOnPath(v, target.ID, op.Op) {
			return
		}
		seen[v] = true
		missing = append(missing, "$"+v)
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		where := "before " + op.Target
		if op.Op == ast.AlterFlowInsertAfter {
			where = "after " + op.Target
		}
		return fmt.Errorf("the fragment uses %s, which is not declared on the path %s", strings.Join(missing, ", "), where)
	}
	return nil
}

// unscopedRefs returns the variables expr reads that do not exist where the
// stored node id runs: not a parameter, not the output of an activity before
// it (or one an earlier operation took away), not declared by an earlier
// operation on the path to it, and not a system variable.
func (a *alterFlowContext) unscopedRefs(expr string, id model.ID) []string {
	inScope := map[string]bool{}
	for _, p := range a.mf.Parameters {
		inScope[p.Name] = true
	}
	for _, v := range a.iteratorsAround(id) {
		inScope[v] = true
	}
	for up := range a.upstreamOf(id, false) {
		for _, c := range a.cands {
			if c.ID == up && c.OutputVariable != "" {
				inScope[c.OutputVariable] = true
			}
		}
	}
	var missing []string
	seen := map[string]bool{}
	for _, m := range variableRef.FindAllStringSubmatch(withoutStringLiterals(expr), -1) {
		v := m[1]
		if seen[v] || systemVariables[v] || a.declaredOnPath(v, id, ast.AlterFlowReplace) || (inScope[v] && !a.removedByOps[v]) {
			continue
		}
		seen[v] = true
		missing = append(missing, "$"+v)
	}
	sort.Strings(missing)
	return missing
}

// declaredOnPath reports whether a fragment an earlier operation of the
// statement put in declares v upstream of where an operation of kind op aimed
// at the stored node id goes (mendixlabs/mxcli#1517). Where a fragment goes
// follows from its own operation and target, both resolved against the flow
// as stored:
//
//   - inserted after t, or in t's place: upstream of whatever t is upstream
//     of. Two inserts after the same node do not say which runs first, so
//     neither counts for the other;
//   - inserted before t: upstream of t too, so upstream of a replacement of
//     t and of an insert after it, but not of another insert before it.
func (a *alterFlowContext) declaredOnPath(v string, id model.ID, op ast.AlterFlowOpKind) bool {
	for _, site := range a.declaredAt[v] {
		including := false
		if site.op == ast.AlterFlowInsertBefore {
			including = op != ast.AlterFlowInsertBefore
		}
		if a.upstreamOf(id, including)[site.target] {
			return true
		}
	}
	return false
}

// upstreamOf returns every object from which id can be reached along the
// stored flows — the activities whose outputs exist when the flow gets there.
// id itself is included only when including says so (an insert after it runs
// once it has).
//
// Inside a loop body, what runs before the loop runs before id too, at every
// level of nesting (ako/mxcli#1106).
func (a *alterFlowContext) upstreamOf(id model.ID, including bool) map[model.ID]bool {
	preds := map[model.ID][]model.ID{}
	var collect func(oc *microflows.MicroflowObjectCollection)
	collect = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, f := range oc.Flows {
			preds[f.DestinationID] = append(preds[f.DestinationID], f.OriginID)
		}
		for _, obj := range oc.Objects {
			if l, ok := obj.(*microflows.LoopedActivity); ok {
				collect(l.ObjectCollection)
			}
		}
	}
	collect(a.mf.ObjectCollection)
	out := map[model.ID]bool{}
	queue := append([]model.ID(nil), preds[id]...)
	for _, l := range a.loopsAround(id) {
		queue = append(queue, preds[l.ID]...)
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if out[n] {
			continue
		}
		out[n] = true
		queue = append(queue, preds[n]...)
	}
	if including {
		out[id] = true
	}
	return out
}

// checkOutputUnused refuses to take away an activity whose output variable
// another activity still reads — unless the replacement declares it again.
// It runs once every operation of the statement is applied, so the readers it
// counts are those of the flow the statement writes: a stored activity that an
// operation takes out or rewrites is none, a fragment that reads it is one.
//
// Nor is a reader that another operation's fragment declares the variable
// for, upstream of it (ako/mxcli#1399): a producer moved upstream is
// inserted there and dropped here, and its readers read the inserted one.
func (a *alterFlowContext) checkOutputUnused(target mfmutator.Candidate, replacement *backend.MicroflowFragment) error {
	v := target.OutputVariable
	if v == "" || fragmentDeclares(replacement, v) {
		return nil
	}
	var readers []string
	for _, r := range a.readByOps[v] {
		if r.own || a.declaredOnPath(v, r.site.target, r.site.op) {
			continue
		}
		readers = append(readers, r.text)
	}
	if len(readers) > 0 {
		return fmt.Errorf("$%s is read by what an operation of this alter adds: %s", v, strings.Join(readers, "; "))
	}
	ref := regexp.MustCompile(`\$` + regexp.QuoteMeta(v) + `\b`)
	var users []string
	for _, c := range a.cands {
		if c.ID == target.ID || a.takenAway(c.ID) || a.declaredOnPath(v, c.ID, ast.AlterFlowReplace) {
			continue
		}
		for _, text := range append([]string{c.Statement}, c.Alternates...) {
			if ref.MatchString(withoutStringLiterals(text)) {
				users = append(users, c.Statement)
				break
			}
		}
	}
	if len(users) > 0 {
		return fmt.Errorf("$%s is still used by: %s", v, strings.Join(users, "; "))
	}
	return nil
}

// checkDeclaredUnclashed refuses a fragment that declares a variable a stored
// activity also declares, unless the statement takes that activity away. It
// runs once every operation is applied, like checkOutputUnused: a producer
// moved upstream is inserted by one operation and dropped by another, in
// either order (ako/mxcli#1399).
func (a *alterFlowContext) checkDeclaredUnclashed(frag *backend.MicroflowFragment) error {
	for _, obj := range frag.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		v := mfmutator.OutputVariable(act.Action)
		if v == "" {
			continue
		}
		for _, c := range a.cands {
			if c.OutputVariable == v && !a.takenAway(c.ID) {
				return fmt.Errorf("the fragment declares $%s, which the %s already has; choose another name", v, a.stmt.Kind())
			}
		}
	}
	return nil
}
