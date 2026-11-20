// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2283: `alter page … set (<attribute property>: Assoc/Attr) on <pluggable>`
// resolved the value with the plain-name resolver, so an association path was
// not stored as CREATE PAGE stores it since #2279 — the final attribute plus an
// IndirectEntityRef of one EntityRefStep per hop. ALTER SET must store exactly
// what `create page` stores for the same path, refuse what it refuses, and
// check (which dry-runs the SET) must agree with exec.

// Every switch starts bound to a page parameter's attribute ($Acc.Active: an
// AttributeRef plus a SourceVariable), so each SET below has something to
// change — the path ones and the plain control alike.
const alter2283Page = `mdl 1;
create page Administration.P2283 (title: 'P', layout: Atlas_Core.Atlas_Default,
  params: ( $Pwd: Administration.AccountPasswordData, $Acc: Administration.Account )) {
  dataview dvPwd (datasource: $Pwd) {
    switch swPath (booleanAttribute: $Acc.Active)
    switch swQualified (booleanAttribute: $Acc.Active)
    switch swPlainPwd (booleanAttribute: $Acc.Active)
  }
  dataview dvAcc (datasource: $Acc) {
    switch swPlain (booleanAttribute: $Acc.Active)
    switch swRefSet (booleanAttribute: $Acc.Active)
    switch swBackwards (booleanAttribute: $Acc.Active)
  }
};`

// The control: create page with the same values, the shape ALTER must match.
const alter2283Control = `mdl 1;
create page Administration.P2283C (title: 'P', layout: Atlas_Core.Atlas_Default,
  params: ( $Pwd: Administration.AccountPasswordData, $Acc: Administration.Account )) {
  dataview dvPwd (datasource: $Pwd) {
    switch swPath (attribute: AccountPasswordData_Account/Active)
    switch swQualified (attribute: Administration.AccountPasswordData_Account/Administration.Account/Active)
  }
  dataview dvAcc (datasource: $Acc) {
    switch swPlain (attribute: Active)
  }
};`

// storedBindings is storedAttributeRefs plus whether the WidgetValue holding
// that AttributeRef reads a SourceVariable: "…|sv" when it does.
func storedBindings(t *testing.T, exec *Executor, page string) map[string]string {
	t.Helper()
	refs := storedAttributeRefs(t, exec, page)
	ctx := exec.newExecContext(context.Background())
	unit, err := ctx.Backend.GetRawUnitByName("page", page)
	if err != nil || unit == nil {
		t.Fatalf("raw unit of %s: %v", page, err)
	}
	var doc bson.D
	if err := bson.Unmarshal(unit.Contents, &doc); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var walk func(v any, owner string)
	walk = func(v any, owner string) {
		switch x := v.(type) {
		case bson.D:
			m := map[string]any{}
			for _, e := range x {
				m[e.Key] = e.Value
			}
			if name, ok := m["Name"].(string); ok && name != "" {
				owner = name
			}
			if _, ok := m["AttributeRef"].(bson.D); ok && !seen[owner] {
				seen[owner] = true
				if sv, ok := m["SourceVariable"].(bson.D); ok && sv != nil {
					refs[owner] += "|sv"
				}
			}
			for _, e := range x {
				walk(e.Value, owner)
			}
		case bson.A:
			for _, e := range x {
				walk(e, owner)
			}
		}
	}
	walk(doc, "")
	return refs
}

func TestAlterSetPluggableAttribute_AssociationPathStoredAsCreateStores(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	if err := afRun(t, exec, alter2283Page); err != nil {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	if err := afRun(t, exec, alter2283Control); err != nil {
		t.Fatalf("create control: %v\n%s", err, out.String())
	}
	before := storedBindings(t, exec, "Administration.P2283")
	if before["swPath"] != "System.User.Active|sv" {
		t.Fatalf("precondition: swPath starts as %q, want the parameter binding", before["swPath"])
	}
	const alter = `mdl 1;
alter page Administration.P2283 {
  set (booleanAttribute: AccountPasswordData_Account/Active) on swPath;
  set (booleanAttribute: Administration.AccountPasswordData_Account/Administration.Account/Active) on swQualified;
  set (booleanAttribute: Active) on swPlain;
};`
	if err := afRun(t, exec, alter); err != nil {
		t.Fatalf("alter: %v\n%s", err, out.String())
	}
	got := storedBindings(t, exec, "Administration.P2283")
	want := storedBindings(t, exec, "Administration.P2283C")
	const step = "|Administration.AccountPasswordData_Account>Administration.Account"
	for name, shape := range map[string]string{
		"swPath":      "System.User.Active" + step,
		"swQualified": "System.User.Active" + step, // normalised to the short form's steps
		"swPlain":     "System.User.Active",        // control: a plain attribute, no EntityRef
	} {
		if want[name] != shape {
			t.Fatalf("control: create page stored %s as %q, want %q", name, want[name], shape)
		}
		if got[name] != want[name] {
			t.Errorf("%s: alter stored %q, create page stores %q", name, got[name], want[name])
		}
	}

	// Second exec: nothing to change, nothing written.
	out.Reset()
	if err := afRun(t, exec, alter); err != nil {
		t.Fatalf("second alter: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Unchanged page Administration.P2283") {
		t.Errorf("second exec should write nothing, got:\n%s", out.String())
	}
	if again := storedBindings(t, exec, "Administration.P2283"); again["swPath"] != got["swPath"] ||
		again["swQualified"] != got["swQualified"] || again["swPlain"] != got["swPlain"] {
		t.Errorf("second exec changed the binding: %v -> %v", got, again)
	}
}

func TestAlterSetPluggableAttribute_AssociationPathRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, widget, attr, want string
	}{
		{"reference set", "swRefSet", "System.UserRoles/Name", "a reference set"},
		{"reference followed backwards", "swBackwards", "AccountPasswordData_Account/OldPassword", "the end that does not own it"},
		{"no such association", "swPlainPwd", "NoSuchAssoc/Active", "does not resolve"},
		{"leading entity", "swPlain", "Administration.Account/Active", "MDL-ATTRPATH02"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `mdl 1;
alter page Administration.P2283 {
  set (booleanAttribute: ` + tc.attr + `) on ` + tc.widget + `;
};`
			// check, against the stored page.
			checker, _ := openPedAppWithWidgets(t)
			if err := afRun(t, checker, alter2283Page); err != nil {
				t.Fatalf("create: %v", err)
			}
			checkErrs := alterCheckErrors(t, checker, src)
			exec, _ := openPedAppWithWidgets(t)
			if err := afRun(t, exec, alter2283Page); err != nil {
				t.Fatalf("create: %v", err)
			}
			before := storedAttributeRefs(t, exec, "Administration.P2283")
			execErr := ""
			if err := afRun(t, exec, src); err != nil {
				execErr = err.Error()
			}
			if !strings.Contains(execErr, tc.want) {
				t.Errorf("exec: want a refusal containing %q, got %q", tc.want, execErr)
			}
			if !strings.Contains(checkErrs, tc.want) {
				t.Errorf("check: want a refusal containing %q (check must agree with exec), got %q", tc.want, checkErrs)
			}
			if after := storedAttributeRefs(t, exec, "Administration.P2283"); after[tc.widget] != before[tc.widget] {
				t.Errorf("a refused SET changed %s: %q -> %q", tc.widget, before[tc.widget], after[tc.widget])
			}
		})
	}
	// Control: the forward reference passes check.
	checker, _ := openPedAppWithWidgets(t)
	if err := afRun(t, checker, alter2283Page); err != nil {
		t.Fatalf("create: %v", err)
	}
	if errs := alterCheckErrors(t, checker, `mdl 1;
alter page Administration.P2283 {
  set (booleanAttribute: AccountPasswordData_Account/Active) on swPlainPwd;
};`); errs != "" {
		t.Errorf("check refused a forward reference path:\n%s", errs)
	}
}

// checkErrors is what check --references says about src on exec's project.
func alterCheckErrors(t *testing.T, exec *Executor, src string) string {
	t.Helper()
	prog, perrs := visitor.Build(src)
	if len(perrs) > 0 {
		t.Fatalf("parse: %v\n%s", perrs[0], src)
	}
	errs, _ := exec.ValidateProgramWithWarnings(prog)
	var es []string
	for _, e := range errs {
		es = append(es, e.Error())
	}
	return strings.Join(es, "\n")
}

// The unquoted `Assoc/Attr` now reaches the executor under any key; on a
// column property other than Attribute it is still refused, rather than
// stringified into the column ("&{Assoc/Attr}").
func TestAlterSetPluggableAttribute_AssociationPathOnColumnCaptionRefused(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	dir := filepath.Dir(exec.newExecContext(context.Background()).Backend.Path())
	const setup = `mdl 1;
create persistent entity MyFirstModule.Customer2283 (City: String(100));
create persistent entity MyFirstModule.Order2283 (Number: String(20));
create association MyFirstModule.Order2283_Customer2283 from MyFirstModule.Order2283 to MyFirstModule.Customer2283;
create page MyFirstModule.OrderGrid2283 ( Title: 'Orders', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dgOrders (DataSource: database from MyFirstModule.Order2283) {
    column colNumber (Attribute: Number, Caption: 'Number')
  }
};`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, "mdl 1;\nalter page MyFirstModule.OrderGrid2283 { set (Caption: Order2283_Customer2283/City) on dgOrders column(Number) };",
		"not an expression")
}
