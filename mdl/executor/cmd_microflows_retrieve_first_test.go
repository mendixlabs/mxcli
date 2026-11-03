// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// `retrieve … first` vs `limit 1` (ako/mxcli#734): the writer stores the range
// the visitor resolved, and describe spells the object range `first`, which
// means the same in every language version.

// retrieveRangeOf builds a retrieve from MDL source and returns the range the
// writer stores and the type the builder gives the output variable.
func retrieveRangeOf(t *testing.T, src string) (*microflows.Range, string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("%q: %v", src, errs)
	}
	var mf *ast.CreateMicroflowStmt
	for _, s := range prog.Statements {
		if m, ok := s.(*ast.CreateMicroflowStmt); ok {
			mf = m
		}
	}
	if mf == nil {
		t.Fatalf("%q: no microflow", src)
	}
	r := mf.Body[0].(*ast.RetrieveStmt)
	fb := &flowBuilder{varTypes: map[string]string{}}
	fb.addRetrieveAction(r)
	if len(fb.errors) > 0 {
		t.Fatalf("%q: builder errors: %v", src, fb.errors)
	}
	act := fb.objects[0].(*microflows.ActionActivity).Action.(*microflows.RetrieveAction)
	return act.Source.(*microflows.DatabaseRetrieveSource).Range, fb.varTypes[r.Variable]
}

func TestRetrieveRange_WrittenPerLanguageVersion(t *testing.T) {
	const body = "create microflow M.F () begin\n  retrieve $O from M.Order where [Status = 'Open'] %s;\nend;"
	cases := []struct {
		name, src       string
		wantType        microflows.RangeType
		wantLimit       string
		wantObjectTyped bool
	}{
		{"first, mdl 0", strings.Replace(body, "%s", "first", 1), microflows.RangeTypeFirst, "", true},
		{"first, mdl 1", "mdl 1;\n" + strings.Replace(body, "%s", "first", 1), microflows.RangeTypeFirst, "", true},
		{"limit 1, mdl 0 (alpha meaning)", strings.Replace(body, "%s", "limit 1", 1), microflows.RangeTypeFirst, "", true},
		{"limit 1, mdl 1 (a list of one)", "mdl 1;\n" + strings.Replace(body, "%s", "limit 1", 1), microflows.RangeTypeCustom, "1", false},
		{"limit 2, mdl 0", strings.Replace(body, "%s", "limit 2", 1), microflows.RangeTypeCustom, "2", false},
		// The offset makes it a Custom range; the variable type used to say
		// object here while the writer stored a list.
		{"limit 1 offset 3, mdl 0", strings.Replace(body, "%s", "limit 1 offset 3", 1), microflows.RangeTypeCustom, "1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rng, varType := retrieveRangeOf(t, tc.src)
			if rng == nil || rng.RangeType != tc.wantType || rng.Limit != tc.wantLimit {
				t.Fatalf("range = %+v, want %s with limit %q", rng, tc.wantType, tc.wantLimit)
			}
			if isObject := !strings.HasPrefix(varType, "List of "); isObject != tc.wantObjectTyped {
				t.Errorf("output typed %q; want object=%v", varType, tc.wantObjectTyped)
			}
		})
	}
}

// describe prints the object range as `first` (not `limit 1`, which is a list
// under mdl 1) and a Custom range as limit/offset, so its output reads back to
// the same range.
func TestFormatAction_Retrieve_RangeSpelling(t *testing.T) {
	e := newTestExecutor()
	cases := []struct {
		rng  *microflows.Range
		want string
	}{
		{&microflows.Range{RangeType: microflows.RangeTypeFirst}, "retrieve $O from M.Order\n    first;"},
		{&microflows.Range{RangeType: microflows.RangeTypeCustom, Limit: "1"}, "retrieve $O from M.Order\n    limit 1;"},
		{&microflows.Range{RangeType: microflows.RangeTypeCustom, Limit: "10", Offset: "20"}, "retrieve $O from M.Order\n    limit 10\n    offset 20;"},
	}
	for _, tc := range cases {
		got := e.formatAction(&microflows.RetrieveAction{
			OutputVariable: "O",
			Source:         &microflows.DatabaseRetrieveSource{EntityQualifiedName: "M.Order", Range: tc.rng},
		}, nil, nil)
		if got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.rng, got, tc.want)
		}
	}
}

// What check reports for each spelling in each version: the MDL-V1-LIMIT1
// warning on an mdl 0 `limit 1`, and MDL-RETRIEVE01 wherever the variable is an
// object used as a list.
func TestRetrieveRange_CheckPerLanguageVersion(t *testing.T) {
	// A loop rather than count(): it is a list use in both versions, where the
	// call form of an aggregate is not (ako/mxcli#733).
	const body = `create microflow M.F ()
begin
  retrieve $O from M.Order where [Status = 'Open'] %s;
  loop $Item in $O
  begin
    log info node 'F' 'item';
  end loop;
end;`
	cases := []struct {
		name, header, clause string
		wantLimitOneWarning  bool
		wantRetrieve01       bool
	}{
		{"mdl 0 limit 1: object, warned, and looping over it is CE0097", "", "limit 1", true, true},
		{"mdl 1 limit 1: a list of one, nothing to report", "mdl 1;\n", "limit 1", false, false},
		{"mdl 0 first: object, looping over it is CE0097", "", "first", false, true},
		{"mdl 1 first: object, looping over it is CE0097", "mdl 1;\n", "first", false, true},
		{"mdl 0 limit 2: a list", "", "limit 2", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.header + strings.Replace(body, "%s", tc.clause, 1)
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			gotWarn, gotR01 := false, false
			for _, v := range ValidateProgram(prog, "") {
				switch v.RuleID {
				case "MDL-V1-LIMIT1":
					gotWarn = true
				case retrieveSingleRule:
					gotR01 = true
				}
			}
			if gotWarn != tc.wantLimitOneWarning {
				t.Errorf("MDL-V1-LIMIT1 reported = %v, want %v", gotWarn, tc.wantLimitOneWarning)
			}
			if gotR01 != tc.wantRetrieve01 {
				t.Errorf("%s reported = %v, want %v", retrieveSingleRule, gotR01, tc.wantRetrieve01)
			}
		})
	}
}

// mendixlabs/mxcli#1737 (ako/mxcli#1941): `retrieve … offset 5000` without a
// limit was written as the "all" range: the offset was dropped, so the
// retrieve returned the first objects instead of skipping 5000, with check and
// exec both silent. An offset alone is a Custom range too.
func TestRetrieveRange_OffsetWithoutLimit(t *testing.T) {
	const body = "create microflow M.F () begin\n  retrieve $O from M.Order offset 5000;\nend;"
	for _, header := range []string{"", "mdl 1;\n"} {
		t.Run(strings.TrimSpace(header), func(t *testing.T) {
			rng, varType := retrieveRangeOf(t, header+body)
			if rng == nil || rng.RangeType != microflows.RangeTypeCustom || rng.Offset != "5000" || rng.Limit != "" {
				t.Fatalf("range = %+v, want a Custom range with offset 5000 and no limit", rng)
			}
			if !strings.HasPrefix(varType, "List of ") {
				t.Errorf("output typed %q, want a list", varType)
			}
		})
	}
}
