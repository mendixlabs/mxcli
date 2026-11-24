// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// # What describe states besides the activities (ako/mxcli#818)
//
// A description states a flow's header, its document properties and where
// every node is drawn. `create or modify` patched the activities only, so under
// `mdl 1` — where the fallback is a refusal — re-running an edited description
// failed the moment the edit touched any of the rest, and `describe` could not
// emit the header. The rest is patched now, and the rules are the patch's own:
//
//   - a document property or return type the statement changes is set on the
//     stored document (mfmutator SetHeader), a parameter is added, retyped or
//     removed in place, and a parameter the flow or another document still
//     uses is not removed;
//   - a stated @position or @start that is not where the stored node is drawn
//     moves that node, and only it: its flows keep their ends, sides and
//     curves (no reconnection), and nothing around it moves. A position the
//     script leaves out keeps what is stored, as before;
//   - a node the script draws at a stated position stays there: a fragment
//     that needs more room than the stated positions leave is refused, rather
//     than everything after it moved along, which left the stored flow saying
//     something other than the script (mendixlabs/mxcli#1598, case 6);
//   - two nodes nobody placed follow the node they were derived from, because
//     mxcli's own layout put them there and nothing else says where they go:
//     a start event where the layout derives it follows the first statement
//     (#951's rule for a rewrite), and a trailing end event the script does
//     not place follows the statement before it;
//   - a redrawn connector (@anchor, @curve, @merge) and a node moved inside a
//     loop body or an error handler are refused: they would need the flows
//     rerouted, which the patch does not do.

// flowMove is a stored node the declared statement draws somewhere else.
//
// With fit set it is the box of the loop id instead, fitted to its body once
// the script has moved the body's nodes (ako/mxcli#2564). With optional, the
// node may be gone by the time the move is applied a second time: a body
// activity the splice replaced by one of another kind, whose fragment is
// drawn where the script states.
type flowMove struct {
	id       model.ID
	to       model.Point
	what     string
	fit      *loopFit
	optional bool
}

// loopFit is how a loop box is fitted: to the stated @size, or (nil) to the
// moved body.
type loopFit struct {
	size *model.Size
}

// endSpacing is how far right of the statement before it the builder draws the
// end event a body falls through to: one spacing unit to the next slot, half a
// unit more to the end event (flowBuilder.buildFlowGraph).
const endSpacing = HorizontalSpacing + HorizontalSpacing/2

// patch opens the stored flow and applies the moves and the splice
// operations. The moves go first, so a fragment the script places next to a
// moved node is joined to it where it now is, and again after the operations,
// so a node no one placed that follows another (followingEnd) is drawn where
// the script says even if an insert shifted it to make room (a move sets a
// position, so applying it twice is applying it once). Between the two, every
// point the script states a node at is pinned, so an operation that would
// have to move one to make room is refused (mendixlabs/mxcli#1598, case 6).
func patch(ctx *ExecContext, a *alterFlowContext, ops []*ast.AlterFlowOperation, targets []mfmutator.Candidate,
	moves []flowMove, stated []model.Point, bodyPins []bodyPin) (backend.MicroflowMutator, error) {
	mut, err := ctx.Backend.OpenMicroflowForMutation(a.mf.ID)
	if err != nil {
		return nil, err
	}
	move := func() error {
		for _, mv := range moves {
			if mv.fit != nil {
				if err := mut.FitLoop(mv.id, mv.fit.size, model.Size{Width: MinLoopWidth, Height: MinLoopHeight}); err != nil {
					return fmt.Errorf("fit the %s to its body: %w", mv.what, err)
				}
				continue
			}
			if err := mut.Move(mv.id, mv.to); err != nil && !(mv.optional && mutatorNodeGone(err)) {
				return fmt.Errorf("move the %s: %w", mv.what, err)
			}
		}
		return nil
	}
	if err := move(); err != nil {
		return nil, err
	}
	for _, p := range stated {
		if err := mut.Pin(p); err != nil {
			return nil, err
		}
	}
	for _, p := range bodyPins {
		if err := mut.PinInLoop(p.loop, p.at); err != nil {
			return nil, err
		}
	}
	if err := a.applyTo(ctx, mut, ops, targets); err != nil {
		return nil, err
	}
	if err := move(); err != nil {
		return nil, err
	}
	return mut, nil
}

// moved records the moves that draw a stored statement where the declared one
// says, the two being the same statement but for where their nodes are drawn:
// the statement's own node, and the nodes of an `if`'s branches, which are
// nodes of the flow like any other. A node moved inside anything else — a loop
// body, whose coordinates are relative to the loop box, or an error handler —
// is refused.
func (pd *patchDiff) moved(d, s ast.MicroflowStatement) error {
	if err := pd.movedNode(d, s); err != nil {
		return err
	}
	di, ok1 := d.(*ast.IfStmt)
	si, ok2 := s.(*ast.IfStmt)
	if ok1 && ok2 {
		for _, br := range [][2][]ast.MicroflowStatement{{di.ThenBody, si.ThenBody}, {di.ElseBody, si.ElseBody}} {
			if len(br[0]) != len(br[1]) {
				return redrawn(s)
			}
			for i := range br[0] {
				if declaredMatches(br[0][i], br[1][i]) {
					continue
				}
				if err := pd.moved(br[0][i], br[1][i]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if loopStatements(s) != nil || loopStatements(d) != nil {
		return pd.movedInLoop(d, s)
	}
	clear := func(a *ast.ActivityAnnotations) { a.Position = nil }
	if !declaredMatches(withAnnotations(d, clear), withAnnotations(s, clear)) {
		return cannotSplice("a node inside the %s is moved; the splice moves the nodes of the flow and of an if's branches, "+
			"not those inside a loop body or an error handler", describeAt(s))
	}
	return nil
}

// movedInLoop records the moves of the nodes of a stored loop's body the
// declared loop draws elsewhere (ako/mxcli#2564). A body is drawn in the loop
// box's coordinates, so its nodes are moved in them, and the box is then
// fitted to where they are — to the stated @size, else the way `create` sizes
// the loop — so the flow reads as a create of the same script would. The
// loop's own node has been moved already (movedNode).
func (pd *patchDiff) movedInLoop(d, s ast.MicroflowStatement) error {
	db, sb := loopStatements(d), loopStatements(s)
	if len(db) != len(sb) {
		return redrawn(s)
	}
	c, err := pd.loc.locate(s)
	if err != nil {
		return err
	}
	loop, ok := c.Object.(*microflows.LoopedActivity)
	if !ok {
		return cannotSplice("the stored %s is not drawn as a loop", describeAt(s))
	}
	outer := pd.loc
	pd.loc = outer.within(loop)
	defer func() { pd.loc = outer }()
	for i := range db {
		if declaredMatches(db[i], sb[i]) {
			continue
		}
		if err := pd.moved(db[i], sb[i]); err != nil {
			return err
		}
	}
	pd.fitLoop(c.ID, d, s)
	return nil
}

// fitLoop records that the box of the stored loop id is fitted once its body's
// moves are made: to the @size the declared loop d states, else to its body.
// A loop whose body moves nothing is left as it is drawn. Recorded once.
func (pd *patchDiff) fitLoop(id model.ID, d, s ast.MicroflowStatement) {
	for _, mv := range pd.moves {
		if mv.fit != nil && mv.id == id {
			return
		}
	}
	fit := &loopFit{}
	if a := statementAnnotations(d); a != nil && a.Size != nil && a.Size.Width > 0 && a.Size.Height > 0 {
		fit.size = &model.Size{Width: a.Size.Width, Height: a.Size.Height}
	}
	pd.moves = append(pd.moves, flowMove{id: id, fit: fit, what: describeAt(s)})
}

// mutatorNodeGone reports whether err is the mutator's "not in the flow" for a
// node an operation took out.
func mutatorNodeGone(err error) bool {
	return err != nil && strings.Contains(err.Error(), "is not in the stored flow")
}

// movedNode records a move of the stored statement's own node when the
// declared one states a position it is not drawn at.
func (pd *patchDiff) movedNode(d, s ast.MicroflowStatement) error {
	da, sa := statementAnnotations(d), statementAnnotations(s)
	if da == nil || da.Position == nil {
		return nil
	}
	if sa != nil && sa.Position != nil && *sa.Position == *da.Position {
		return nil
	}
	c, err := pd.loc.locate(s)
	if err != nil {
		return err
	}
	pd.moves = append(pd.moves, flowMove{id: c.ID, to: model.Point{X: da.Position.X, Y: da.Position.Y}, what: describeAt(s)})
	return nil
}

// statedPoints are the canvas points the declared flow states a node is drawn
// at: each statement's @position, an if's @merge, the @start, every note's
// position and every parameter's. The splice pins whatever is drawn at one, so
// that making room for a fragment does not move a node away from where the
// script draws it (mendixlabs/mxcli#1598, case 6). A loop body is left out:
// its positions are relative to the loop box, and the splice moves no node
// inside one.
func statedPoints(d *flowDecl) []model.Point {
	var out []model.Point
	add := func(p *ast.Position) {
		if p != nil {
			out = append(out, model.Point{X: p.X, Y: p.Y})
		}
	}
	for _, p := range d.params {
		add(p.Position)
	}
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			switch v := rv.Interface().(type) {
			case *ast.ActivityAnnotations:
				add(v.Position)
				add(v.Merge)
				add(v.Start)
				for _, notes := range [][]ast.MicroflowAnnotation{v.Notes, v.FreeNotes} {
					for _, n := range notes {
						add(n.Position)
					}
				}
				return
			case *ast.LoopStmt:
				walk(reflect.ValueOf(v.Annotations))
				return
			case *ast.WhileStmt:
				walk(reflect.ValueOf(v.Annotations))
				return
			}
			walk(rv.Elem())
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i))
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(d.body))
	return out
}

// redrawn is the refusal for a statement whose connectors the script draws
// differently: the patch moves nodes, and does not reroute flows.
func redrawn(st ast.MicroflowStatement) error {
	return cannotSplice("the connectors of the %s are redrawn (its @anchor, @curve or @merge); the splice moves nodes "+
		"but does not reroute or redraw the flows between them", describeAt(st))
}

// withoutStart returns the statements with their @start taken off (as copies),
// and the first one stated.
func withoutStart(stmts []ast.MicroflowStatement) ([]ast.MicroflowStatement, *ast.Position) {
	var start *ast.Position
	out := make([]ast.MicroflowStatement, len(stmts))
	for i, st := range stmts {
		out[i] = st
		if ann := statementAnnotations(st); ann != nil && ann.Start != nil {
			if start == nil {
				p := *ann.Start
				start = &p
			}
			out[i] = withAnnotations(st, func(a *ast.ActivityAnnotations) { a.Start = nil })
		}
	}
	return out, start
}

// startEvent records the move of the start event: to a stated @start, or —
// when none is stated and the start is where mxcli's layout derives it — along
// with a first statement the script places (#951).
func (pd *patchDiff) startEvent(a *alterFlowContext, declared []ast.MicroflowStatement, stated *ast.Position) {
	oc := a.mf.ObjectCollection
	var start microflows.MicroflowObject
	for _, o := range oc.Objects {
		if _, ok := o.(*microflows.StartEvent); ok {
			start = o
		}
	}
	if start == nil {
		return
	}
	cur := start.GetPosition()
	var to *model.Point
	switch {
	case stated != nil:
		to = &model.Point{X: stated.X, Y: stated.Y}
	case len(declared) > 0:
		derived, ok := derivedStartPosition(oc)
		ann := statementAnnotations(declared[0])
		if ok && derived == cur && ann != nil && ann.Position != nil {
			to = &model.Point{X: ann.Position.X - HorizontalSpacing, Y: ann.Position.Y}
		}
	}
	if to != nil && *to != cur {
		pd.moves = append(pd.moves, flowMove{id: start.GetID(), to: *to, what: "start event"})
	}
}

// followingEnd records the move of the end event a body ends at when the
// script does not place it and mxcli's layout drew it — endSpacing right of the
// statement before it, on its line — and the statement before it is now drawn
// elsewhere: the end follows, as it would in a build of the script, rather
// than being left across the canvas (the end-event half of #951).
func (pd *patchDiff) followingEnd(declared, stored []ast.MicroflowStatement) error {
	n, m := len(declared), len(stored)
	if n < 2 || m < 2 {
		return nil
	}
	dr, ok1 := declared[n-1].(*ast.ReturnStmt)
	sr, ok2 := stored[m-1].(*ast.ReturnStmt)
	if !ok1 || !ok2 || (dr.Annotations != nil && dr.Annotations.Position != nil) {
		return nil
	}
	if branches(declared[n-2]) || branches(stored[m-2]) {
		return nil
	}
	e, p, q := statedPosition(sr), statedPosition(stored[m-2]), statedPosition(declared[n-2])
	if e == nil || p == nil || q == nil || *e != (ast.Position{X: p.X + endSpacing, Y: p.Y}) {
		return nil
	}
	to := ast.Position{X: q.X + endSpacing, Y: q.Y}
	if to == *e {
		return nil
	}
	c, err := pd.loc.locate(sr)
	if err != nil {
		return err
	}
	pd.moves = append(pd.moves, flowMove{id: c.ID, to: model.Point{X: to.X, Y: to.Y}, what: describeAt(sr)})
	return nil
}

// branches reports whether the node a statement is drawn as is not the one a
// following statement's flow leaves from: an if joins its branches in a merge
// or ends them, and a loop's flow leaves the loop box.
func branches(st ast.MicroflowStatement) bool {
	switch st.(type) {
	case *ast.IfStmt, *ast.LoopStmt, *ast.WhileStmt:
		return true
	}
	return false
}

func statedPosition(st ast.MicroflowStatement) *ast.Position {
	if ann := statementAnnotations(st); ann != nil {
		return ann.Position
	}
	return nil
}

// withoutParameterPositions returns params with their positions cleared (as a
// copy): a parameter drawn elsewhere is a move, not a header change.
func withoutParameterPositions(params []ast.MicroflowParam) []ast.MicroflowParam {
	if len(params) == 0 {
		return params
	}
	out := append([]ast.MicroflowParam(nil), params...)
	for i := range out {
		out[i].Position = nil
	}
	return out
}

// parameterMoves records the moves of the stored parameters the script draws
// somewhere else. A parameter without @position keeps where it is.
func parameterMoves(d *flowDecl, stored []*microflows.MicroflowParameter) []flowMove {
	var out []flowMove
	for _, p := range d.params {
		if p.Position == nil {
			continue
		}
		for i, sp := range stored {
			if sp.Name != p.Name {
				continue
			}
			cur := microflows.DerivedParameterPosition(i, stored)
			if sp.Position != nil {
				cur = *sp.Position
			}
			if to := (model.Point{X: p.Position.X, Y: p.Position.Y}); to != cur {
				out = append(out, flowMove{id: sp.ID, to: to, what: "parameter $" + p.Name})
			}
		}
	}
	return out
}

// checkRemovedParameters refuses to remove a parameter another document still
// passes an argument to. A call names the parameter it binds by qualified name
// (Module.Flow.Parameter), in a microflow's call action, a page's button or
// data source, a workflow's call, a published service; the patch cannot update
// them, and each would be left binding a parameter that no longer exists. The
// flow's own uses are the mutator's to check (SetHeader), since those are in
// the document it writes.
func checkRemovedParameters(ctx *ExecContext, d *flowDecl, a *alterFlowContext) error {
	declared := map[string]bool{}
	for _, p := range d.params {
		declared[p.Name] = true
	}
	var removed []string
	for _, p := range a.mf.Parameters {
		if !declared[p.Name] {
			removed = append(removed, p.Name)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	units, err := ctx.Backend.ListRawUnitsByType("")
	if err != nil {
		return cannotSplice("parameter $%s is removed, and the documents that might pass it an argument cannot be read: %v", removed[0], err)
	}
	for _, name := range removed {
		qn := []byte(d.name.Module + "." + d.name.Name + "." + name)
		var users []string
		for _, u := range units {
			if u.ID == a.mf.ID || !bytes.Contains(u.Contents, qn) {
				continue
			}
			users = append(users, unitLabel(u.Type, u.Contents))
		}
		if len(users) > 0 {
			if len(users) > 3 {
				users = append(users[:3], fmt.Sprintf("%d more", len(users)-3))
			}
			return cannotSplice("parameter $%s is removed, but %s still pass(es) an argument to it; the patch does not "+
				"change the callers — remove the argument there first", name, strings.Join(users, ", "))
		}
	}
	return nil
}

// unitLabel names a unit for a message: its type and its name.
func unitLabel(typ string, contents []byte) string {
	var head struct {
		Name string `bson:"Name"`
	}
	_ = bson.Unmarshal(contents, &head)
	kind := typ
	if i := strings.LastIndex(kind, "$"); i >= 0 {
		kind = kind[i+1:]
	}
	if head.Name == "" {
		return "a " + kind
	}
	return "the " + strings.ToLower(kind) + " " + head.Name
}

// firstStatementPlaced reports whether a fragment's first statement states
// where it is drawn, which makes the fragment placed (backend.MicroflowFragment).
func firstStatementPlaced(body []ast.MicroflowStatement) bool {
	return len(body) > 0 && statedPosition(body[0]) != nil
}

// laterStatementPlaced reports whether a statement after a fragment's first
// states where it is drawn. Unless the first does too, the fragment is
// translated into the gap as a whole and that position would be lost, the
// statements landing on top of each other.
func laterStatementPlaced(body []ast.MicroflowStatement) bool {
	for _, st := range body[min(1, len(body)):] {
		if statedPosition(st) != nil {
			return true
		}
	}
	return false
}
