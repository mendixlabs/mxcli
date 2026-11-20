// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"sort"
	"testing"
	"unsafe"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
)

// nestedBuilderInherits classifies every field of flowBuilder: true when the
// builder of a nested body (a loop's, a while loop's, an error handler's)
// inherits it from its parent — nestedBuilder copies it — and false when the
// body keeps its own. A field added to flowBuilder fails
// TestNestedBuilder_InheritsSharedState until it is put here, which is the
// point: ako/mxcli#1769 was a field (qualifiedMembersOnly) that three
// hand-written copies each left out.
var nestedBuilderInherits = map[string]bool{
	// Which flow is built, and how its statements are written.
	"varTypes":             true,
	"declaredVars":         true,
	"textLang":             true,
	"measurer":             true,
	"layout":               true, // a nested body is drawn in the flow's style (#1832)
	"backend":              true,
	"quiet":                true,
	"hierarchy":            true,
	"restServices":         true,
	"isNanoflow":           true,
	"self":                 true,
	"scriptFlows":          true,
	"annotationsByLabel":   true,
	"storedNotes":          true,
	"qualifiedMembersOnly": true,
	"statedIncludes":       true, // the carry reads one registry (#1914)
	"resolveKnownOnly":     true,

	// Where the body is drawn: nestedBuilder's arguments, and the body's own
	// lanes and rows (a loop box is sized to fit, so it never wraps).
	"posX": false, "posY": false, "baseY": false, "spacing": false,
	"lowerLane": false, "allowWrap": false, "row": false,

	// What the body builds, merged into the parent by the site.
	"objects": false, "flows": false, "annotationFlows": false, "errors": false,
	"statedNotes": false, "curveByOrigin": false, "rejoinMergeAt": false, "rejoinPastRowX": false,
	"returnEndIDs": false, "fallThroughEndID": false, "endsWithReturn": false,
	"lastReturnEndID": false, "returnValue": false,

	// Per-statement pending state.
	"nextConnectionPoint": false, "incomingRedirect": false, "nextFlowCase": false,
	"nextFlowAnchor": false, "atBodyTail": false, "pendingAnnotations": false,
	"previousStmtAnchor": false, "manualLoopBackTarget": false, "pendingJoin": false,
	"emptyErrorHandlerFrom": false, "errorHandlerTailFrom": false, "errorHandlerSource": false,
	"errorHandlerSkipVar": false, "errorHandlerTailCase": false, "errorHandlerTailAnchor": false,
	"errorHandlerTailIsSource": false, "errorHandlerReturnValue": false, "pendingErrorHandlers": false,

	// Set by the one site that shares it: an error handler's body is in its
	// parent's collection and on its path (addErrorHandlerFlow); a loop's is
	// in a collection a sequence flow may not leave.
	"returnType": false, "labelReg": false,

	// The whole flow's: the stored start event's position, and what
	// buildFlowGraph collects from the body it builds.
	"startPosition": false, "listInputVariables": false, "objectInputVariables": false,
	"generatedVars": false,

	// Caches, filled on first use.
	"assocCatalog": false, "microflowsCache": false, "microflowsCacheLoaded": false,
	"nanoflowsCache": false, "nanoflowsCacheLoaded": false,

	// validateFlowBody's, which builds no nested builder.
	"duplicateNamesOwnedElsewhere": false, "checkAssocShapes": false,
	"assocObjectVars": false, "joinScopes": false,
}

func TestNestedBuilder_InheritsSharedState(t *testing.T) {
	typ := reflect.TypeOf(flowBuilder{})
	fields := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		fields[name] = true
		if _, ok := nestedBuilderInherits[name]; !ok {
			t.Errorf("flowBuilder.%s is not classified: say in nestedBuilderInherits whether a nested body "+
				"inherits it (and copy it in nestedBuilder) or keeps its own", name)
		}
	}
	var stale []string
	for name := range nestedBuilderInherits {
		if !fields[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("nestedBuilderInherits names %s, which flowBuilder no longer has", name)
	}

	parent := &flowBuilder{}
	pv := reflect.ValueOf(parent).Elem()
	for i := 0; i < pv.NumField(); i++ {
		setNonZero(t, typ.Field(i).Name, settable(pv.Field(i)))
	}
	child := parent.nestedBuilder(11, 22)
	cv := reflect.ValueOf(child).Elem()
	placed := map[string]int64{"posX": 11, "posY": 22, "baseY": 22, "spacing": HorizontalSpacing}
	for i := 0; i < cv.NumField(); i++ {
		name := typ.Field(i).Name
		p, c := settable(pv.Field(i)), settable(cv.Field(i))
		switch {
		case placed[name] != 0:
			if c.Int() != placed[name] {
				t.Errorf("nestedBuilder(11, 22).%s = %d, want %d", name, c.Int(), placed[name])
			}
		case nestedBuilderInherits[name]:
			if !sameValue(p, c) {
				t.Errorf("a nested body does not inherit flowBuilder.%s", name)
			}
		case !c.IsZero():
			t.Errorf("a nested body inherits flowBuilder.%s, classified as its own", name)
		}
	}
}

// settable returns v — a field of an addressable struct, exported or not —
// as a value that can be read and set.
func settable(v reflect.Value) reflect.Value {
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
}

// setNonZero gives v a value distinguishable from its zero value and, for a
// reference, from any other instance of its type.
func setNonZero(t *testing.T, name string, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.String:
		v.SetString("x")
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
	case reflect.Interface:
		mb := reflect.ValueOf(&mock.MockBackend{})
		if !mb.Type().AssignableTo(v.Type()) {
			t.Fatalf("flowBuilder.%s: no test value for %s", name, v.Type())
		}
		v.Set(mb)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			setNonZero(t, name, settable(v.Field(i)))
		}
	default:
		t.Fatalf("flowBuilder.%s: no test value for kind %s", name, v.Kind())
	}
}

// sameValue reports whether b is a's value — for a reference, the same
// instance, not an equal one.
func sameValue(a, b reflect.Value) bool {
	switch a.Kind() {
	case reflect.Map, reflect.Slice, reflect.Pointer:
		return a.Pointer() == b.Pointer()
	case reflect.Interface:
		return !a.IsNil() && !b.IsNil() && a.Elem().Pointer() == b.Elem().Pointer()
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}
