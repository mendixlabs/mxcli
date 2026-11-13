// SPDX-License-Identifier: Apache-2.0

package mfmutator

// # Document properties as a patch (ako/mxcli#818)
//
// `create or modify` of a stored flow whose header changes — a return type, a
// URL, an export level, a parameter — used to rebuild the whole document,
// because the splice edits activities only; under `mdl 1` it was refused. A
// header is not part of the graph, though: every one of its properties is a
// value of the unit itself, or (a parameter) an object no flow connects to. So
// it is patched like the rest: the declared document is encoded exactly as the
// rebuild would encode it, and each header property whose value differs is
// copied onto the stored document. Nothing else is touched: no object, no
// flow, and no property a create statement cannot state.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
)

// headerKeys are the document properties a create statement states: its
// clauses, its return type and its documentation. A property the statement
// has no syntax for (MarkAsUsed, AllowedModuleRoles, StableId, …) is not
// among them, so it is never written by this path, whatever the encoder
// produces for it.
var headerKeys = []string{
	"Documentation",
	"Excluded",
	"MicroflowReturnType",
	"ReturnVariableName",
	"ApplyEntityAccess",
	"ExportLevel",
	"AllowConcurrentExecution",
	"ConcurrenyErrorMessage", // sic: Mendix's storage name
	"ConcurrencyErrorMessage",
	"ConcurrencyErrorMicroflow",
	"Url",
	"UrlSearchParameters",
	"MicroflowActionInfo",
	"WorkflowActionInfo",
}

const parameterType = "Microflows$MicroflowParameter"

// parameterTypeKey is where a parameter stores its type: the metamodel's
// ParameterType is stored as VariableType.
const parameterTypeKey = "VariableType"

// SetHeader writes the header of declared onto the stored document; see
// backend.MicroflowMutator.
func (m *Mutator) SetHeader(declared any) ([]string, error) {
	d, err := m.deps.SerializeDocument(declared)
	if err != nil {
		return nil, fmt.Errorf("encode the declared header: %w", err)
	}
	if got, want := dString(d, "$Type"), dString(m.doc, "$Type"); got != want {
		return nil, fmt.Errorf("the declared document is a %s, the stored one a %s", got, want)
	}
	var changed []string
	for _, key := range headerKeys {
		nv, inNew := lookup(d, key)
		if !inNew {
			// The encoder does not write it for this document (a void
			// nanoflow's return type, a pre-10 URL): nothing is stated.
			continue
		}
		ov, inOld := lookup(m.doc, key)
		if !inOld {
			if isZero(nv) {
				continue
			}
			if !m.declares(key) {
				return nil, fmt.Errorf("the stored document has no %s property, so the value the statement gives it cannot be "+
					"patched in; set it in Studio Pro", key)
			}
			m.doc = insertAfterPredecessor(m.doc, d, key, nv)
			changed = append(changed, strings.Replace(key, "Concurreny", "Concurrency", 1))
			continue
		}
		carried, same, err := carryValue(key, nv, ov)
		if err != nil {
			return nil, err
		}
		if same {
			continue
		}
		m.replaceValue(ov, carried)
		dSet(m.doc, key, carried)
		changed = append(changed, strings.Replace(key, "Concurreny", "Concurrency", 1))
	}
	params, err := m.setParameters(d)
	if err != nil {
		return nil, err
	}
	return append(changed, params...), nil
}

// PropertyDeclarer is implemented by a Deps that can tell whether the
// project's metamodel declares a document property. SetHeader adds a stated
// header property the stored document lacks only when it does: the key is
// then simply one the writer of the stored document left out — mxcli's own
// nanoflow writer omits ReturnVariableName when the statement has no
// `as $Var`, and so did older builds (#843) — while a key the project's
// version does not declare makes the document unopenable, so without an
// answer the absence is refused as before.
type PropertyDeclarer interface {
	DeclaresProperty(docType, key string) bool
}

func (m *Mutator) declares(key string) bool {
	pd, ok := m.deps.(PropertyDeclarer)
	return ok && pd.DeclaresProperty(dString(m.doc, "$Type"), key)
}

// insertAfterPredecessor adds key to stored after the nearest property that
// precedes it in the declared encoding and is stored too, so the key lands
// where the writer puts it; with none, it is appended.
func insertAfterPredecessor(stored, declared bson.D, key string, v any) bson.D {
	at := len(stored)
	for i := range declared {
		if declared[i].Key != key {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if k := indexOf(stored, declared[j].Key); k >= 0 {
				at = k + 1
				break
			}
		}
		break
	}
	out := make(bson.D, 0, len(stored)+1)
	out = append(out, stored[:at]...)
	out = append(out, bson.E{Key: key, Value: v})
	return append(out, stored[at:]...)
}

func indexOf(d bson.D, key string) int {
	for i := range d {
		if d[i].Key == key {
			return i
		}
	}
	return -1
}

// carryValue prepares a declared property value for the stored document: the
// languages of a text the statement could not state are carried from the
// stored one, and every element that corresponds to a stored one keeps the
// stored $ID — the same two carries a rebuild's write gets (canon.Reconcile),
// confined to this property. same reports that the result is canonically the
// stored value, so there is nothing to write.
func carryValue(key string, declared, stored any) (any, bool, error) {
	nb, err := bson.Marshal(bson.D{{Key: key, Value: declared}})
	if err != nil {
		return nil, false, fmt.Errorf("encode %s: %w", key, err)
	}
	ob, err := bson.Marshal(bson.D{{Key: key, Value: stored}})
	if err != nil {
		return nil, false, fmt.Errorf("encode the stored %s: %w", key, err)
	}
	nb = canon.CarryTranslations(nb, ob)
	nb = canon.TransplantIDs(nb, ob)
	if eq, err := canon.Equal(nb, ob); err == nil && eq {
		return stored, true, nil
	}
	var out bson.D
	if err := bson.Unmarshal(nb, &out); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", key, err)
	}
	v, _ := lookup(out, key)
	return v, false, nil
}

// replaceValue records the elements of a stored value that the new value no
// longer holds, so Save can prove nothing still points at one of them.
func (m *Mutator) replaceValue(old, replacement any) {
	keep := map[string]bool{}
	collectIDs(replacement, keep)
	gone := map[string]bool{}
	collectIDs(old, gone)
	for id := range gone {
		if !keep[id] {
			m.removed[id] = true
		}
	}
}

func collectIDs(v any, into map[string]bool) {
	switch t := v.(type) {
	case bson.D:
		for _, e := range t {
			if e.Key == "$ID" {
				if b, ok := e.Value.(bson.Binary); ok {
					into[string(b.Data)] = true
				}
				continue
			}
			collectIDs(e.Value, into)
		}
	case bson.A:
		for _, el := range t {
			collectIDs(el, into)
		}
	}
}

// setParameters makes the stored parameters the declared ones, matched by
// name: a parameter the statement adds is inserted after the one it follows, a
// retyped one gets the declared type in place (its $ID, position and
// documentation stay), and a removed one is taken out — unless the flow still
// uses it, which would leave an expression naming a variable that no longer
// exists. Where a kept parameter is drawn is not this function's concern
// (Move is), and neither are the properties MDL cannot state.
//
// Reordering is refused: the order of the parameters is the order a caller
// passes arguments in, and it is the order they are stored in.
func (m *Mutator) setParameters(declared bson.D) ([]string, error) {
	oc := dDoc(m.doc, "ObjectCollection")
	objs := arrayElements(dGet(oc, "Objects"))
	var declParams []bson.D
	for _, el := range arrayElements(dGet(dDoc(declared, "ObjectCollection"), "Objects")) {
		if p, ok := el.(bson.D); ok && dString(p, "$Type") == parameterType {
			declParams = append(declParams, p)
		}
	}
	storedAt := map[string]int{} // name -> index in objs
	var storedOrder []string
	for i, el := range objs {
		if p, ok := el.(bson.D); ok && dString(p, "$Type") == parameterType {
			storedAt[dString(p, "Name")] = i
			storedOrder = append(storedOrder, dString(p, "Name"))
		}
	}
	inDecl := map[string]bool{}
	var keptOrder []string
	for _, p := range declParams {
		name := dString(p, "Name")
		inDecl[name] = true
		if _, ok := storedAt[name]; ok {
			keptOrder = append(keptOrder, name)
		}
	}
	var storedKept []string
	for _, name := range storedOrder {
		if inDecl[name] {
			storedKept = append(storedKept, name)
		}
	}
	for i := range keptOrder {
		if keptOrder[i] != storedKept[i] {
			return nil, fmt.Errorf("the parameters are reordered ($%s where $%s is stored); the order is the order "+
				"callers pass arguments in, and a reorder is not patched: reorder them in Studio Pro", keptOrder[i], storedKept[i])
		}
	}

	var changed []string
	var removed []string
	for _, name := range storedOrder {
		if !inDecl[name] {
			removed = append(removed, name)
		}
	}
	if len(removed) > 0 {
		drop := map[int]bool{}
		for _, name := range removed {
			drop[storedAt[name]] = true
		}
		for _, name := range removed {
			if where := m.usesVariable(name, drop); where != "" {
				return nil, fmt.Errorf("parameter $%s is removed, but the flow still uses it (%s); an expression "+
					"or action naming it would be left behind", name, where)
			}
		}
		var keep []any
		for i, el := range objs {
			if drop[i] {
				m.markRemoved(el)
				continue
			}
			keep = append(keep, el)
		}
		objs = keep
		for _, name := range removed {
			changed = append(changed, "parameter $"+name+" removed")
		}
	}

	index := func(name string) int {
		for i, el := range objs {
			if p, ok := el.(bson.D); ok && dString(p, "$Type") == parameterType && dString(p, "Name") == name {
				return i
			}
		}
		return -1
	}
	insertAt := 0 // parameters go first, as the rebuild writes them
	var added []bson.D
	for _, dp := range declParams {
		name := dString(dp, "Name")
		if i := index(name); i >= 0 {
			sp, _ := objs[i].(bson.D)
			key := parameterTypeKey
			nv, inNew := lookup(dp, key)
			ov, inOld := lookup(sp, key)
			if !inNew || !inOld {
				return nil, fmt.Errorf("parameter $%s stores no %s", name, key)
			}
			carried, same, err := carryValue(key, nv, ov)
			if err != nil {
				return nil, err
			}
			if !same {
				m.replaceValue(ov, carried)
				dSet(sp, key, carried)
				changed = append(changed, "parameter $"+name+" type")
			}
			insertAt = i + 1
			continue
		}
		objs = append(objs[:insertAt], append([]any{dp}, objs[insertAt:]...)...)
		insertAt++
		added = append(added, dp)
		changed = append(changed, "parameter $"+name+" added")
	}
	placeAddedParameters(objs, added)
	if len(changed) > 0 && !setArray(oc, "Objects", objs) {
		return nil, fmt.Errorf("the object collection has no Objects list")
	}
	return changed, nil
}

// placeAddedParameters moves an added parameter that would sit on another
// parameter to the slot after the rightmost one. The rebuild derives a
// parameter's position from its index, and the stored parameters after an
// inserted one keep theirs, so a parameter added between two landed exactly on
// the next (ako/mxcli#2185). The pitch is the derived row's
// (microflows.ParameterPitch): 7px a character of the widest name, rounded up
// to 10px, never under 100px. A parameter whose slot is free keeps it.
func placeAddedParameters(objs []any, added []bson.D) {
	if len(added) == 0 {
		return
	}
	var params []bson.D
	for _, el := range objs {
		if p, ok := el.(bson.D); ok && dString(p, "$Type") == parameterType {
			params = append(params, p)
		}
	}
	widest := 0
	for _, p := range params {
		widest = max(widest, utf8.RuneCountInString(dString(p, "Name")))
	}
	pitch := max(100, (widest*7+9)/10*10)
	for _, a := range added {
		pos := parsePoint(dString(a, "RelativeMiddlePoint"))
		taken, right := false, point{}
		for i, p := range params {
			if dString(p, "Name") == dString(a, "Name") {
				continue
			}
			pp := parsePoint(dString(p, "RelativeMiddlePoint"))
			if pp == pos {
				taken = true
			}
			if i == 0 || pp.X > right.X {
				right = pp
			}
		}
		if taken {
			dSet(a, "RelativeMiddlePoint", point{right.X + pitch, right.Y}.String())
		}
	}
}

// usesVariable returns where the flow still names $name in an expression, or
// bare in an action's *VariableName property, or "". It looks at every string in the unit's objects and flows outside the
// parameter objects being removed: a variable is referenced by name, in text,
// so a text search finds every use without knowing which properties hold
// expressions.
func (m *Mutator) usesVariable(name string, skip map[int]bool) string {
	re := regexp.MustCompile(`(?i)\$` + regexp.QuoteMeta(name) + `\b`)
	var hits []string
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch t := v.(type) {
		case bson.D:
			for _, e := range t {
				walk(e.Value, e.Key)
			}
		case bson.A:
			for _, el := range t {
				walk(el, key)
			}
		case string:
			if re.MatchString(t) || (namesVariable(key) && strings.EqualFold(t, name)) {
				hits = append(hits, fmt.Sprintf("%s %q", key, t))
			}
		}
	}
	oc := dDoc(m.doc, "ObjectCollection")
	for i, el := range arrayElements(dGet(oc, "Objects")) {
		if !skip[i] {
			walk(el, "")
		}
	}
	for _, el := range arrayElements(dGet(m.doc, "Flows")) {
		walk(el, "")
	}
	if len(hits) == 0 {
		return ""
	}
	sort.Strings(hits)
	return hits[0]
}

// declaringKeys are the *VariableName properties that declare a variable (an
// action's output, a loop's iterator) rather than name one in use.
var declaringKeys = map[string]bool{
	"VariableName": true, "OutputVariableName": true, "outputVariableName": true,
	"ResultVariableName": true, "LoopVariableName": true, "ReturnVariableName": true,
}

// namesVariable reports whether a property names, bare and without the `$` of
// an expression, a variable an action uses: delete $A is stored as
// DeleteVariableName "A", and so are commit, change, rollback, a retrieve over
// an association and a loop's list.
func namesVariable(key string) bool {
	return strings.HasSuffix(key, "VariableName") && !declaringKeys[key]
}

func lookup(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// isZero reports whether an encoded value says nothing: an empty string, false,
// or a list with no elements.
func isZero(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case bool:
		return !t
	case bson.A:
		return len(arrayElements(t)) == 0
	}
	return false
}
