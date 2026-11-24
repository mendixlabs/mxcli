// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// # Editing inside a loop body (ako/mxcli#1693)
//
// A loop body is a statement list of its own, drawn inside the loop box in
// that box's coordinates. The splice edits it the way it edits the top level,
// for plain activities: `create or modify` diffs the body statement by
// statement (inLoopBody, cmd_flow_modify.go) and `alter` addresses an
// activity in it by its handle; an insert is laid out among the body's nodes
// and the box grows when the body no longer fits it (mfmutator, growLoop).
// Everything else — a decision, a nested loop, a return — changes the body's
// shape, and is made by `replace body of` the loop: the loop itself keeps its
// $ID, position, iterator and list, and only what is inside it is rebuilt.
//
// Replacing the whole loop implicitly instead (option (b) of the issue) was
// rejected: it would silently drop the loop's identity and layout, which is
// what the mdl 1 refusal of the rebuild protects.

// replaceBody applies `replace body of <loop> with begin … end`: the stored
// loop target keeps everything but what is inside it, which becomes the
// fragment's statements, built as `create` builds a loop body — the loop
// restated with its stored header, so the iterator is typed from the list as
// it is there.
func (a *alterFlowContext) replaceBody(ctx *ExecContext, mut backend.MicroflowMutator, op *ast.AlterFlowOperation,
	target mfmutator.Candidate) error {
	loop, ok := target.Object.(*microflows.LoopedActivity)
	if !ok {
		return fmt.Errorf("replace body of addresses a loop, and %s is not one; address the loop by its handle from "+
			"`describe %s %s with handles`, e.g. `replace body of loop $It in $Items with begin … end;`",
			target.Statement, a.stmt.Kind(), a.stmt.Name)
	}
	header, err := a.storedLoopStatement(ctx, target)
	if err != nil {
		return err
	}
	restated := withLoopBody(header, op.Body)
	frag, vars, err := a.buildFragmentVars(ctx, []ast.MicroflowStatement{restated})
	if err != nil {
		return err
	}
	// The body reads what is declared before the loop, its iterator and what
	// it declares itself — as a replaced loop's body does.
	scopeOp := &ast.AlterFlowOperation{Op: ast.AlterFlowReplace, Target: op.Target}
	if err := a.checkFragmentScope(ctx, scopeOp, target, frag); err != nil {
		return err
	}
	if err := a.checkBodyUnclashed(frag, loop); err != nil {
		return err
	}
	if err := mut.ReplaceLoopBody(target.ID, frag); err != nil {
		return err
	}
	// What the stored body held is gone: a variable it declared clashes with
	// nothing a later operation declares, and its reads are no uses.
	for _, c := range a.cands {
		if loopHolds(loop, c.ID) {
			a.removedIDs[c.ID] = true
		}
	}
	a.noteFragment(ctx, op, target, frag, vars)
	return nil
}

// withLoopBody is the stored loop statement header with body as its body,
// stated nowhere on the canvas: the builder lays the body out, and the loop
// object it builds around it is discarded.
func withLoopBody(header ast.MicroflowStatement, body []ast.MicroflowStatement) ast.MicroflowStatement {
	switch l := header.(type) {
	case *ast.LoopStmt:
		c := *l
		c.Body, c.Annotations = body, nil
		return &c
	case *ast.WhileStmt:
		c := *l
		c.Body, c.Annotations = body, nil
		return &c
	}
	return header
}

// storedLoopStatement returns the statement the stored loop target is
// described as — a loop or while statement, at any depth — found by where it
// is drawn, level by level through the loops around it (a body's positions
// are relative to its loop box, so a position is unique only within one body).
func (a *alterFlowContext) storedLoopStatement(ctx *ExecContext, target mfmutator.Candidate) (ast.MicroflowStatement, error) {
	if a.storedDesc == nil {
		st, err := describedFlowStmt(ctx, &flowDecl{nanoflow: a.stmt.Nanoflow, name: a.stmt.Name}, a)
		if err != nil {
			return nil, fmt.Errorf("the stored flow's description cannot be read: %v", err)
		}
		a.storedDesc = st
	}
	stmts := storedBody(a.storedDesc)
	around := a.loopsAround(target.ID)
	// The description states canvas coordinates; the stored loops keep frame
	// positions, so each level's position is its stored centre plus the
	// enclosing boxes' canvas origins (#1935).
	origin := model.Point{}
	for i := len(around) - 1; i >= 0; i-- {
		lp := model.Point{X: around[i].Position.X + origin.X, Y: around[i].Position.Y + origin.Y}
		st := statementDrawnAt(stmts, lp)
		if st == nil {
			return nil, fmt.Errorf("no stored loop is drawn at (%d, %d)", lp.X, lp.Y)
		}
		origin = model.Point{
			X: origin.X + around[i].Position.X - around[i].Size.Width/2,
			Y: origin.Y + around[i].Position.Y - around[i].Size.Height/2,
		}
		stmts = loopStatements(st)
	}
	p := model.Point{
		X: target.Object.GetPosition().X + origin.X,
		Y: target.Object.GetPosition().Y + origin.Y,
	}
	switch st := statementDrawnAt(stmts, p).(type) {
	case *ast.LoopStmt, *ast.WhileStmt:
		return st, nil
	}
	return nil, fmt.Errorf("no stored loop is drawn at (%d, %d), where %s is", p.X, p.Y, target.Statement)
}

// checkBodyUnclashed refuses a new body that declares a variable the flow
// already has outside the body it replaces — a parameter, or an activity
// output elsewhere — which it would shadow or clash with. One the stored body
// declares is gone with it.
func (a *alterFlowContext) checkBodyUnclashed(frag *backend.MicroflowFragment, loop *microflows.LoopedActivity) error {
	existing := map[string]bool{}
	for _, p := range a.mf.Parameters {
		existing[p.Name] = true
	}
	for _, c := range a.cands {
		if c.OutputVariable != "" && c.ID != loop.ID && !loopHolds(loop, c.ID) && !a.takenAway(c.ID) {
			existing[c.OutputVariable] = true
		}
	}
	var clash []string
	var walk func(objs []microflows.MicroflowObject)
	walk = func(objs []microflows.MicroflowObject) {
		for _, obj := range objs {
			switch o := obj.(type) {
			case *microflows.ActionActivity:
				if v := mfmutator.OutputVariable(o.Action); v != "" && existing[v] {
					clash = append(clash, "$"+v)
				}
			case *microflows.LoopedActivity:
				if o.ObjectCollection != nil {
					walk(o.ObjectCollection.Objects)
				}
			}
		}
	}
	for _, obj := range frag.Objects {
		if l, ok := obj.(*microflows.LoopedActivity); ok && l.ObjectCollection != nil {
			walk(l.ObjectCollection.Objects)
		}
	}
	if len(clash) > 0 {
		return fmt.Errorf("the new body declares %s, which the %s already has outside the loop; choose another name",
			strings.Join(clash, ", "), a.stmt.Kind())
	}
	return nil
}
