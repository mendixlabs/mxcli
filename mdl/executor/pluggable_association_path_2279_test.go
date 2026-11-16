// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2279: a pluggable widget's attribute property given a
// context-relative association path, `switch sw (Attribute: Assoc/Attr)`, was
// resolved as one attribute NAME and stored as the flat attribute
// `Entity.Assoc/Attr` — check passed, exec reported "Created page", and mxbuild
// 11.16.0 answered CE1613 "The selected attribute
// 'MyFirstModule.Order.Order_Owner/Active' no longer exists".
//
// Studio Pro stores such a binding as the CustomWidgets$WidgetValue's
// AttributeRef — the FINAL attribute, with an IndirectEntityRef of one
// EntityRefStep per hop (PedApp Administration.Account_Overview, a Data grid 2
// column over System.UserRoles) — the shape a built-in input already writes.
// A property linked to no datasource holds one value of the context object, so
// each hop must arrive at one object: measured on 11.16.0, a reference set is
// CE7247 "should be of type 'Reference'" and a reference followed from the end
// that does not own it CE7247 "has no association". Those, and a path that does
// not resolve at all, are refused by exec and by check alike.

// storedAttributeRefs returns, per named widget of the stored page, the first
// AttributeRef under it: "Attribute|Assoc>Dest|…".
func storedAttributeRefs(t *testing.T, exec *Executor, page string) map[string]string {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	unit, err := ctx.Backend.GetRawUnitByName("page", page)
	if err != nil || unit == nil {
		t.Fatalf("raw unit of %s: %v", page, err)
	}
	var doc bson.D
	if err := bson.Unmarshal(unit.Contents, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", page, err)
	}
	field := func(d bson.D, key string) any {
		for _, e := range d {
			if e.Key == key {
				return e.Value
			}
		}
		return nil
	}
	out := map[string]string{}
	var walk func(v any, owner string)
	walk = func(v any, owner string) {
		switch x := v.(type) {
		case bson.D:
			if name, _ := field(x, "Name").(string); name != "" {
				owner = name
			}
			if ref, ok := field(x, "AttributeRef").(bson.D); ok && out[owner] == "" {
				parts := []string{field(ref, "Attribute").(string)}
				if er, ok := field(ref, "EntityRef").(bson.D); ok {
					if typ := field(er, "$Type"); typ != "DomainModels$IndirectEntityRef" {
						t.Errorf("%s: EntityRef $Type = %v, want DomainModels$IndirectEntityRef", owner, typ)
					}
					steps, _ := field(er, "Steps").(bson.A)
					for _, s := range steps {
						if sd, ok := s.(bson.D); ok {
							parts = append(parts, field(sd, "Association").(string)+">"+field(sd, "DestinationEntity").(string))
						}
					}
				}
				out[owner] = strings.Join(parts, "|")
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
	return out
}

func TestPluggableAttributeAssociationPath_StoredAsSteps(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	const script = `mdl 1;
create page Administration.P2279 (title: 'P', layout: Atlas_Core.Atlas_Default,
  params: ( $Pwd: Administration.AccountPasswordData, $Acc: Administration.Account )) {
  dataview dvPwd (datasource: $Pwd) {
    switch swPath (attribute: AccountPasswordData_Account/Active)
    switch swQualified (attribute: Administration.AccountPasswordData_Account/Administration.Account/Active)
    textbox tbPath (label: 'Name', attribute: AccountPasswordData_Account/FullName)
  }
  dataview dvAcc (datasource: $Acc) {
    switch swPlain (attribute: Active)
  }
};`
	if err := afRun(t, exec, script); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	got := storedAttributeRefs(t, exec, "Administration.P2279")
	const step = "|Administration.AccountPasswordData_Account>Administration.Account"
	for name, want := range map[string]string{
		// The reported shape: the final attribute (declared on System.User,
		// which Account specializes) plus the association hop.
		"swPath": "System.User.Active" + step,
		// The fully qualified spelling normalises to the same steps (#2255).
		"swQualified": "System.User.Active" + step,
		// Control: a built-in input with the same path, unchanged.
		"tbPath": "Administration.Account.FullName" + step,
		// Control: a plain attribute stays a direct reference, no EntityRef.
		"swPlain": "System.User.Active",
	} {
		if got[name] != want {
			t.Errorf("%s AttributeRef = %q, want %q", name, got[name], want)
		}
	}
}

// pluggablePathRefusals is what exec and check say about src: exec's error,
// check's errors.
func pluggablePathRefusals(t *testing.T, src string) (execErr, checkErrs string) {
	t.Helper()
	prog, perrs := visitor.Build(src)
	if len(perrs) > 0 {
		t.Fatalf("parse: %v\n%s", perrs[0], src)
	}
	checker, _ := openPedAppWithWidgets(t)
	refErrs, _ := checker.ValidateProgramWithWarnings(prog)
	var es []string
	for _, e := range refErrs {
		es = append(es, e.Error())
	}
	exec, _ := openPedAppWithWidgets(t)
	if err := afRun(t, exec, src); err != nil {
		execErr = err.Error()
	}
	return execErr, strings.Join(es, "\n")
}

func TestPluggableAttributeAssociationPath_RefusedWhereNotOneObject(t *testing.T) {
	page := func(params, ds, attr string) string {
		return `mdl 1;
create page Administration.P2279R (title: 'P', layout: Atlas_Core.Atlas_Default, params: ( ` + params + ` )) {
  dataview dv (datasource: ` + ds + `) {
    switch sw (attribute: ` + attr + `)
  }
};`
	}
	for _, tc := range []struct {
		name, params, ds, attr, want string
	}{
		{"reference set", "$Acc: Administration.Account", "$Acc", "System.UserRoles/Name", "a reference set"},
		{"reference followed backwards", "$Acc: Administration.Account", "$Acc",
			"AccountPasswordData_Account/OldPassword", "the end that does not own it"},
		{"no such association", "$Pwd: Administration.AccountPasswordData", "$Pwd", "NoSuchAssoc/Active", "does not resolve"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			execErr, checkErrs := pluggablePathRefusals(t, page(tc.params, tc.ds, tc.attr))
			if !strings.Contains(execErr, tc.want) {
				t.Errorf("exec: want a refusal containing %q, got %q", tc.want, execErr)
			}
			if !strings.Contains(checkErrs, tc.want) {
				t.Errorf("check: want a refusal containing %q (check must agree with exec), got %q", tc.want, checkErrs)
			}
		})
	}
	// Control: the forward reference passes check.
	if _, checkErrs := pluggablePathRefusals(t, page("$Pwd: Administration.AccountPasswordData", "$Pwd",
		"AccountPasswordData_Account/Active")); checkErrs != "" {
		t.Errorf("check refused a forward reference path:\n%s", checkErrs)
	}
}
