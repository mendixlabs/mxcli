// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/mendixexpr"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// # The same graph wherever the layout of the day draws it (ako/mxcli#2259)
//
// sameBuiltFlow pairs a built object with a stored one by where each is
// drawn. Where the script states no @position, the builder's layout pass
// decides where a node goes — and the layout pass is improved from release to
// release. A flow mxcli wrote from a script that states no positions is then
// stored at the positions of the release that wrote it, and the same script
// built by a later release draws its nodes somewhere else: nothing pairs, the
// built graph "differs", and the body goes on to the statement diff — which
// does not match every shape describe prints back (an empty else inside an
// error handler, a handler that rejoins through a chain of merges, a decision
// inside a loop body) and so refused an unchanged re-run with MDL-V1-REBUILD.
// Every release that moved the layout made previously in-sync scripts
// unrunnable.
//
// A position the script does not state is not something a re-run changes:
// the splice keeps a stored node where it is drawn unless the script states
// where it goes. So when the pairing by position finds a difference, the two
// graphs are compared once more with the objects paired by the flows between
// them — from the start, each object's outgoing flows taken in the order of
// what tells them apart (the error handler, the case) — and the geometry the
// layout pass computed set aside: an object's position, size and middle when
// the script states no position for it, and a flow's connection indices when
// the script states no anchor anywhere. Everything else, curves included, is
// compared as exactly as before, and a graph whose objects cannot be paired by
// their flows (two unconnected objects of a type, two flows leaving one object
// alike) is a difference.

// layoutStatements is what a declared flow states of its drawing: the points
// at which it places a node, and whether it states any connection side.
type layoutStatements struct {
	points  map[model.Point]bool
	anchors bool
}

// declaredLayout collects what d states of its drawing, at any depth — loop
// bodies and error handlers included.
func declaredLayout(d *flowDecl) layoutStatements {
	l := layoutStatements{points: map[model.Point]bool{}}
	add := func(p *ast.Position) {
		if p != nil {
			l.points[model.Point{X: p.X, Y: p.Y}] = true
		}
	}
	for _, p := range d.params {
		add(p.Position)
	}
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Interface, reflect.Pointer:
			if rv.IsNil() {
				return
			}
			if v, ok := rv.Interface().(*ast.ActivityAnnotations); ok {
				add(v.Position)
				add(v.Merge)
				add(v.Start)
				for _, notes := range [][]ast.MicroflowAnnotation{v.Notes, v.FreeNotes} {
					for _, n := range notes {
						add(n.Position)
					}
				}
				if v.Anchor != nil || v.TrueBranchAnchor != nil || v.FalseBranchAnchor != nil ||
					v.IteratorAnchor != nil || v.BodyTailAnchor != nil || len(v.CaseAnchors) > 0 {
					l.anchors = true
				}
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
	return l
}

// sameBuiltFlowUpToLayout reports whether a built object collection is the
// stored one up to element IDs and the geometry the script leaves to the
// layout pass. why says where they first differ, for tests.
func sameBuiltFlowUpToLayout(built, stored *microflows.MicroflowObjectCollection, stated layoutStatements) (same bool, why string) {
	bm, sm := &idPairing{names: map[model.ID]string{}}, &idPairing{names: map[model.ID]string{}}
	if err := bm.nameByFlows(built, "top"); err != nil {
		return false, "built: " + err.Error()
	}
	if err := sm.nameByFlows(stored, "top"); err != nil {
		return false, "stored: " + err.Error()
	}
	c := &builtComparer{b: bm, s: sm, layoutFree: true, stated: stated}
	if !c.collection(built, stored, "top") {
		return false, c.why
	}
	return true, ""
}

// nameByFlows names the objects of oc by the path of flows that reaches each
// from where the collection begins, so two drawings of one graph give every
// object the same name.
//
// A loop body's flows are stored with the flow's own, not in the loop's
// collection, so the flows of every collection are gathered first.
func (p *idPairing) nameByFlows(oc *microflows.MicroflowObjectCollection, path string) error {
	g := flowIndex{out: map[model.ID][]*microflows.SequenceFlow{}, in: map[model.ID]int{}}
	g.add(oc)
	return p.nameCollection(oc, path, g)
}

// flowIndex is every sequence flow of a flow, by the objects it connects.
type flowIndex struct {
	out map[model.ID][]*microflows.SequenceFlow
	in  map[model.ID]int
}

func (g flowIndex) add(oc *microflows.MicroflowObjectCollection) {
	if oc == nil {
		return
	}
	for _, f := range oc.Flows {
		g.out[f.OriginID] = append(g.out[f.OriginID], f)
		g.in[f.DestinationID]++
	}
	for _, obj := range oc.Objects {
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			g.add(loop.ObjectCollection)
		}
	}
}

func (p *idPairing) nameCollection(oc *microflows.MicroflowObjectCollection, path string, g flowIndex) error {
	if oc == nil {
		return nil
	}
	out, in := g.out, g.in
	byID := map[model.ID]microflows.MicroflowObject{}
	var roots []microflows.MicroflowObject
	var notes []*microflows.Annotation
	for _, obj := range oc.Objects {
		if obj == nil {
			return fmt.Errorf("a nil object in %s", path)
		}
		byID[obj.GetID()] = obj
		if n, ok := obj.(*microflows.Annotation); ok {
			notes = append(notes, n)
			continue
		}
		if in[obj.GetID()] == 0 {
			roots = append(roots, obj)
		}
	}
	// Where the collection begins: the start event, or a loop body's first
	// object. Two of one type cannot be told apart without their positions.
	sort.SliceStable(roots, func(i, j int) bool {
		return fmt.Sprintf("%T", roots[i]) < fmt.Sprintf("%T", roots[j])
	})
	for i := 1; i < len(roots); i++ {
		if fmt.Sprintf("%T", roots[i]) == fmt.Sprintf("%T", roots[i-1]) {
			return fmt.Errorf("two unconnected %T in %s", roots[i], path)
		}
	}
	n := 0
	var visit func(obj microflows.MicroflowObject) error
	visit = func(obj microflows.MicroflowObject) error {
		if _, done := p.names[obj.GetID()]; done {
			return nil
		}
		name := fmt.Sprintf("%s/%d:%T", path, n, obj)
		n++
		p.names[obj.GetID()] = name
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			if err := p.nameCollection(loop.ObjectCollection, name, g); err != nil {
				return err
			}
		}
		flows := append([]*microflows.SequenceFlow(nil), out[obj.GetID()]...)
		sort.SliceStable(flows, func(i, j int) bool { return outgoingFlowKey(flows[i]) < outgoingFlowKey(flows[j]) })
		for i, f := range flows {
			if i > 0 && outgoingFlowKey(f) == outgoingFlowKey(flows[i-1]) {
				return fmt.Errorf("two flows alike (%s) leave %s", outgoingFlowKey(f), name)
			}
			next, ok := byID[f.DestinationID]
			if !ok {
				continue // a flow out of the collection is compared as a reference
			}
			if err := visit(next); err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range roots {
		if err := visit(r); err != nil {
			return err
		}
	}
	// A note is named by what it is attached to and what it says; a note
	// attached to nothing by what it says.
	attached := map[model.ID]string{}
	for _, f := range oc.AnnotationFlows {
		for _, end := range [][2]model.ID{{f.OriginID, f.DestinationID}, {f.DestinationID, f.OriginID}} {
			if _, isNote := byID[end[0]].(*microflows.Annotation); isNote {
				if target, ok := p.names[end[1]]; ok {
					attached[end[0]] = target
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, note := range notes {
		name := fmt.Sprintf("%s/note(%s):%q", path, attached[note.GetID()], note.Caption)
		if seen[name] {
			return fmt.Errorf("two notes alike in %s", path)
		}
		seen[name] = true
		p.names[note.GetID()] = name
	}
	for _, obj := range oc.Objects {
		if _, ok := p.names[obj.GetID()]; !ok {
			return fmt.Errorf("%T in %s is reached by no flow", obj, path)
		}
	}
	return nil
}

// outgoingFlowKey is what tells the flows leaving one object apart, whatever the
// drawing: whether the flow is the error handler, and the case it is for.
func outgoingFlowKey(f *microflows.SequenceFlow) string {
	k := fmt.Sprintf("handler=%v ", f.IsErrorHandler)
	switch cv := f.CaseValue.(type) {
	case nil:
	case *microflows.ExpressionCase:
		k += "expr:" + mendixexpr.Canonical(cv.Expression)
	case *microflows.EnumerationCase:
		k += "enum:" + cv.Value
	case *microflows.BooleanCase:
		k += fmt.Sprintf("bool:%v", cv.Value)
	case *microflows.InheritanceCase:
		k += "inherits:" + cv.EntityQualifiedName
	default:
		k += fmt.Sprintf("%T", cv)
	}
	return k
}

var (
	baseMicroflowObjectType = reflect.TypeOf(microflows.BaseMicroflowObject{})
	sequenceFlowType        = reflect.TypeOf(microflows.SequenceFlow{})
)

// layoutField reports whether field f of a value of type t is geometry the
// layout pass computed and the script does not state, which a comparison up
// to layout sets aside. v is the built value.
func (c *builtComparer) layoutField(t reflect.Type, f string, v reflect.Value) bool {
	if !c.layoutFree {
		return false
	}
	switch t {
	case baseMicroflowObjectType:
		switch f {
		case "Position", "Size", "RelativeMiddle":
			pos, _ := v.FieldByName("Position").Interface().(model.Point)
			return !c.stated.points[pos]
		}
	case sequenceFlowType:
		switch f {
		case "OriginConnectionIndex", "DestinationConnectionIndex":
			return !c.stated.anchors
		}
	}
	return false
}
