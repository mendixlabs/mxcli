// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/mendixexpr"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// # The declared flow, built, against the stored one (ako/mxcli#859)
//
// diff-then-patch matches statements against the stored flow's description,
// and a description is one spelling of a graph MDL can spell several ways: a
// guard clause or an if/else, a fall-through or a `join`, a row the builder
// wrapped read back as crossed branches and an `elsif`. Wherever describe
// prints another spelling than the script's, an identical re-run of the script
// was a "change" the splice could not make — rebuilt under mdl 0, refused
// under mdl 1 — although the script builds exactly the graph that is stored.
//
// So before any statement is matched, the declared flow is built the way
// `create` builds it, and compared with the stored graph itself. When the two
// are the same graph — the same objects in the same places, the same flows
// between them, every property alike — nothing is to be patched, however
// either side would be printed. Only element IDs are set aside: every build
// mints its own, so objects are paired by where they are drawn (unique in a
// flow whose nodes do not sit on each other; a collision is never a match) and
// flows by the objects they connect, and a reference from one element to
// another is compared through that pairing. Everything else is compared
// exactly, so a graph that differs in anything a write would change goes on to
// the statement diff. A graph the layout pass of another release drew, where
// the script states no position, is paired by its flows instead
// (flow_built_match_layout.go, ako/mxcli#2259).

// sameBuiltFlow reports whether a built object collection is the stored one,
// up to element IDs. why says where they first differ, for tests.
func sameBuiltFlow(built, stored *microflows.MicroflowObjectCollection) (same bool, why string) {
	bm, sm := &idPairing{names: map[model.ID]string{}}, &idPairing{names: map[model.ID]string{}}
	if err := bm.name(built, "top"); err != nil {
		return false, "built: " + err.Error()
	}
	if err := sm.name(stored, "top"); err != nil {
		return false, "stored: " + err.Error()
	}
	c := &builtComparer{b: bm, s: sm}
	if !c.collection(built, stored, "top") {
		return false, c.why
	}
	return true, ""
}

// idPairing names the objects and flows of one side by where they are, so
// the two sides' IDs can be compared by name.
type idPairing struct {
	names map[model.ID]string
}

func (p *idPairing) name(oc *microflows.MicroflowObjectCollection, path string) error {
	if oc == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, obj := range oc.Objects {
		if obj == nil {
			return fmt.Errorf("a nil object in %s", path)
		}
		pos := obj.GetPosition()
		n := fmt.Sprintf("%s/%T@%d,%d", path, obj, pos.X, pos.Y)
		if seen[n] {
			return fmt.Errorf("two objects at %s", n)
		}
		seen[n] = true
		p.names[obj.GetID()] = n
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			if err := p.name(loop.ObjectCollection, n); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *idPairing) of(id model.ID) string {
	if n, ok := p.names[id]; ok {
		return n
	}
	return "raw:" + string(id)
}

type builtComparer struct {
	b, s *idPairing
	why  string
	// layoutFree sets aside the geometry the script leaves to the layout
	// pass, beyond what stated says it states (flow_built_match_layout.go).
	layoutFree bool
	stated     layoutStatements
}

func (c *builtComparer) fail(format string, args ...any) bool {
	if c.why == "" {
		c.why = fmt.Sprintf(format, args...)
	}
	return false
}

// collection compares two object collections: objects paired by name, flows
// and annotation flows as multisets keyed by the objects they connect.
func (c *builtComparer) collection(b, s *microflows.MicroflowObjectCollection, path string) bool {
	if (b == nil) != (s == nil) {
		return c.fail("%s: one side has no object collection", path)
	}
	if b == nil {
		return true
	}
	if len(b.Objects) != len(s.Objects) {
		return c.fail("%s: %d objects built, %d stored", path, len(b.Objects), len(s.Objects))
	}
	stored := map[string]microflows.MicroflowObject{}
	for _, o := range s.Objects {
		stored[c.s.of(o.GetID())] = o
	}
	for _, o := range b.Objects {
		n := c.b.of(o.GetID())
		so, ok := stored[n]
		if !ok {
			return c.fail("%s: nothing stored at %s", path, n)
		}
		if !c.value(reflect.ValueOf(o), reflect.ValueOf(so), n) {
			return false
		}
	}
	if !c.flows(b.Flows, s.Flows, path) {
		return false
	}
	return c.annotationFlows(b.AnnotationFlows, s.AnnotationFlows, path)
}

func (c *builtComparer) flows(b, s []*microflows.SequenceFlow, path string) bool {
	if len(b) != len(s) {
		return c.fail("%s: %d flows built, %d stored", path, len(b), len(s))
	}
	key := func(p *idPairing, f *microflows.SequenceFlow) string {
		return p.of(f.OriginID) + " -> " + p.of(f.DestinationID)
	}
	bs, ss := append([]*microflows.SequenceFlow(nil), b...), append([]*microflows.SequenceFlow(nil), s...)
	sort.SliceStable(bs, func(i, j int) bool { return key(c.b, bs[i]) < key(c.b, bs[j]) })
	sort.SliceStable(ss, func(i, j int) bool { return key(c.s, ss[i]) < key(c.s, ss[j]) })
	used := make([]bool, len(ss))
	for _, f := range bs {
		k := key(c.b, f)
		found := false
		for j, g := range ss {
			if used[j] || key(c.s, g) != k {
				continue
			}
			// Two flows between the same pair of objects are told apart by
			// their other properties; the first that is alike is the one.
			probe := &builtComparer{b: c.b, s: c.s, layoutFree: c.layoutFree, stated: c.stated}
			if probe.value(reflect.ValueOf(f), reflect.ValueOf(g), k) {
				used[j], found = true, true
				break
			}
			if c.why == "" {
				c.why = probe.why
			}
		}
		if !found {
			return c.fail("%s: no stored flow like the built %s (%s)", path, k, c.why)
		}
	}
	return true
}

func (c *builtComparer) annotationFlows(b, s []*microflows.AnnotationFlow, path string) bool {
	if len(b) != len(s) {
		return c.fail("%s: %d annotation flows built, %d stored", path, len(b), len(s))
	}
	count := map[string]int{}
	for _, f := range s {
		count[c.s.of(f.OriginID)+" -> "+c.s.of(f.DestinationID)]++
	}
	for _, f := range b {
		k := c.b.of(f.OriginID) + " -> " + c.b.of(f.DestinationID)
		if count[k] == 0 {
			return c.fail("%s: no stored annotation flow %s", path, k)
		}
		count[k]--
	}
	return true
}

var (
	idType          = reflect.TypeOf(model.ID(""))
	baseElementType = reflect.TypeOf(model.BaseElement{})
	collectionType  = reflect.TypeOf(microflows.MicroflowObjectCollection{})
	rawBSONType     = reflect.TypeOf([]byte(nil))
)

// value compares two values of the model exactly, except that an element's own
// ID is not compared and a reference to another element is compared by the
// name its side gives that element.
func (c *builtComparer) value(b, s reflect.Value, path string) bool {
	if b.IsValid() != s.IsValid() {
		return c.fail("%s: set on one side only", path)
	}
	if !b.IsValid() {
		return true
	}
	if b.Type() != s.Type() {
		return c.fail("%s: %s built, %s stored", path, b.Type(), s.Type())
	}
	switch b.Kind() {
	case reflect.Pointer, reflect.Interface:
		if b.IsNil() || s.IsNil() {
			if b.IsNil() != s.IsNil() {
				return c.fail("%s: nil on one side only", path)
			}
			return true
		}
		if b.Kind() == reflect.Pointer && b.Type().Elem() == collectionType {
			bc, _ := b.Interface().(*microflows.MicroflowObjectCollection)
			sc, _ := s.Interface().(*microflows.MicroflowObjectCollection)
			return c.collection(bc, sc, path)
		}
		return c.value(b.Elem(), s.Elem(), path)
	case reflect.Struct:
		for i := 0; i < b.NumField(); i++ {
			f := b.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			if b.Type() == baseElementType && f.Name == "ID" {
				continue
			}
			if c.layoutField(b.Type(), f.Name, b) {
				continue
			}
			if expressionFields[b.Type()][f.Name] {
				if !c.expression(b.Field(i), s.Field(i), path+"."+f.Name) {
					return false
				}
				continue
			}
			if !c.value(b.Field(i), s.Field(i), path+"."+f.Name) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		if b.Type() == rawBSONType {
			if !sameRawModuloIDs(b.Bytes(), s.Bytes()) {
				return c.fail("%s: the raw documents differ", path)
			}
			return true
		}
		if b.Len() != s.Len() {
			return c.fail("%s: %d built, %d stored", path, b.Len(), s.Len())
		}
		for i := 0; i < b.Len(); i++ {
			if !c.value(b.Index(i), s.Index(i), fmt.Sprintf("%s[%d]", path, i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if b.Len() != s.Len() {
			return c.fail("%s: %d built, %d stored", path, b.Len(), s.Len())
		}
		for _, k := range b.MapKeys() {
			if !c.value(b.MapIndex(k), s.MapIndex(k), fmt.Sprintf("%s[%v]", path, k)) {
				return false
			}
		}
		return true
	case reflect.String:
		if b.Type() == idType {
			if bn, sn := c.b.of(model.ID(b.String())), c.s.of(model.ID(s.String())); bn != sn {
				return c.fail("%s: refers to %s built, %s stored", path, bn, sn)
			}
			return true
		}
		if b.String() != s.String() {
			return c.fail("%s: %q built, %q stored", path, b.String(), s.String())
		}
		return true
	default:
		if !reflect.DeepEqual(b.Interface(), s.Interface()) {
			return c.fail("%s: %v built, %v stored", path, b.Interface(), s.Interface())
		}
		return true
	}
}

// expressionFields names, per model type, the fields that hold Mendix
// expressions (a []string field holds one per element). They are compared by
// their canonical tokens (mendixexpr.Canonical), not as written: the builder
// stores an expression slot as the script spells it, so a member list laid out
// over several lines kept the line break before `)` in its last value, and a
// keyword's case where source text is kept — the same stored expression either
// way, and a difference on every run when compared byte for byte
// (ako/mxcli#886).
var expressionFields = map[reflect.Type]map[string]bool{
	reflect.TypeOf(microflows.EndEvent{}):                                {"ReturnValue": true},
	reflect.TypeOf(microflows.ExpressionSplitCondition{}):                {"Expression": true},
	reflect.TypeOf(microflows.ExpressionCase{}):                          {"Expression": true},
	reflect.TypeOf(microflows.WhileLoopCondition{}):                      {"WhileExpression": true},
	reflect.TypeOf(microflows.RuleCallParameterMapping{}):                {"Argument": true},
	reflect.TypeOf(microflows.MemberChange{}):                            {"Value": true},
	reflect.TypeOf(microflows.AggregateListAction{}):                     {"Expression": true, "ReduceInitialValue": true},
	reflect.TypeOf(microflows.FindOperation{}):                           {"Expression": true},
	reflect.TypeOf(microflows.FilterOperation{}):                         {"Expression": true},
	reflect.TypeOf(microflows.FindByAttributeOperation{}):                {"Expression": true},
	reflect.TypeOf(microflows.FilterByAttributeOperation{}):              {"Expression": true},
	reflect.TypeOf(microflows.ListRangeOperation{}):                      {"LimitExpression": true, "OffsetExpression": true},
	reflect.TypeOf(microflows.ChangeListAction{}):                        {"Value": true},
	reflect.TypeOf(microflows.CreateVariableAction{}):                    {"InitialValue": true},
	reflect.TypeOf(microflows.ChangeVariableAction{}):                    {"Value": true},
	reflect.TypeOf(microflows.MicroflowCallParameterMapping{}):           {"Argument": true},
	reflect.TypeOf(microflows.NanoflowCallParameterMapping{}):            {"Argument": true},
	reflect.TypeOf(microflows.BasicCodeActionParameterValue{}):           {"Argument": true},
	reflect.TypeOf(microflows.ExpressionBasedCodeActionParameterValue{}): {"Expression": true},
	reflect.TypeOf(microflows.TypedTemplateArgument{}):                   {"Expression": true},
	reflect.TypeOf(microflows.ShowMessageAction{}):                       {"TemplateParameters": true},
	reflect.TypeOf(microflows.ValidationFeedbackAction{}):                {"TemplateParameters": true},
	reflect.TypeOf(microflows.LogMessageAction{}):                        {"TemplateParameters": true},
}

// expression compares two expression fields — a string, or a list of them —
// by their canonical tokens.
func (c *builtComparer) expression(b, s reflect.Value, path string) bool {
	switch {
	case b.Kind() == reflect.String:
		if mendixexpr.Canonical(b.String()) != mendixexpr.Canonical(s.String()) {
			return c.fail("%s: %q built, %q stored", path, b.String(), s.String())
		}
		return true
	case b.Kind() == reflect.Slice && b.Type().Elem().Kind() == reflect.String:
		if b.Len() != s.Len() {
			return c.fail("%s: %d built, %d stored", path, b.Len(), s.Len())
		}
		for i := 0; i < b.Len(); i++ {
			if !c.expression(b.Index(i), s.Index(i), fmt.Sprintf("%s[%d]", path, i)) {
				return false
			}
		}
		return true
	}
	return c.value(b, s, path)
}

// sameRawModuloIDs compares two raw BSON documents — the one a call web
// service activity keeps when the structured form cannot reproduce it — with
// every element's own $ID set aside, at any depth. Like the objects around it,
// each build mints its own, so comparing the bytes made two builds of one
// statement a difference on every run (ako/mxcli#861). Anything that is not a
// BSON document is compared as bytes.
func sameRawModuloIDs(b, s []byte) bool {
	if bytes.Equal(b, s) {
		return true
	}
	var bd, sd bson.D
	if bson.Unmarshal(b, &bd) != nil || bson.Unmarshal(s, &sd) != nil {
		return false
	}
	bn, berr := bson.Marshal(withoutIDs(bd))
	sn, serr := bson.Marshal(withoutIDs(sd))
	return berr == nil && serr == nil && bytes.Equal(bn, sn)
}

// withoutIDs is v with the $ID key dropped from every document in it.
func withoutIDs(v any) any {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, 0, len(x))
		for _, e := range x {
			if e.Key == "$ID" {
				continue
			}
			out = append(out, bson.E{Key: e.Key, Value: withoutIDs(e.Value)})
		}
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = withoutIDs(e)
		}
		return out
	default:
		return v
	}
}
