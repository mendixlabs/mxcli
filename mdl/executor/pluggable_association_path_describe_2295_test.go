// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2295: since #2279 / #2283 a pluggable widget's attribute property
// can be bound over an association — stored as the final attribute plus an
// IndirectEntityRef of steps — but describe printed only the final attribute,
// `switch swPath (booleanAttribute: System.User.Active)`. Re-executing that
// bound the property to the context entity's own attribute, or was refused: the
// steps were lost. describe now prints the path, through the formatter the
// Data grid 2 column and the built-in inputs already use, so the round trip
// describe → `create or modify` writes nothing.

// pageUnitBytes is the stored unit of page, as bytes.
func pageUnitBytes(t *testing.T, exec *Executor, page string) []byte {
	t.Helper()
	unit, err := exec.newExecContext(context.Background()).Backend.GetRawUnitByName("page", page)
	if err != nil || unit == nil {
		t.Fatalf("raw unit of %s: %v", page, err)
	}
	return unit.Contents
}

// allAttributeRefs returns every AttributeRef of the stored page, sorted, each
// as "Attribute|Assoc>Dest|…" — nameless elements (Data grid 2 columns)
// included.
func allAttributeRefs(t *testing.T, exec *Executor, page string) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(pageUnitBytes(t, exec, page), &doc); err != nil {
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
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			if ref, ok := field(x, "AttributeRef").(bson.D); ok {
				attr, _ := field(ref, "Attribute").(string)
				parts := []string{attr}
				if er, ok := field(ref, "EntityRef").(bson.D); ok {
					steps, _ := field(er, "Steps").(bson.A)
					for _, s := range steps {
						if sd, ok := s.(bson.D); ok {
							a, _ := field(sd, "Association").(string)
							d, _ := field(sd, "DestinationEntity").(string)
							parts = append(parts, a+">"+d)
						}
					}
				}
				out = append(out, strings.Join(parts, "|"))
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	slices.Sort(out)
	return out
}

// describeReexecUnchanged describes page, re-executes that output, and asserts
// the stored unit is byte-identical afterwards. It returns the describe output.
func describeReexecUnchanged(t *testing.T, exec *Executor, out *bytes.Buffer, page string) string {
	t.Helper()
	described := describeOn(t, exec, out, page)
	before := pageUnitBytes(t, exec, page)
	refsBefore := allAttributeRefs(t, exec, page)
	// check agrees with exec: the describe output is clean.
	prog, perrs := visitor.Build(described)
	if len(perrs) > 0 {
		t.Fatalf("describe output of %s does not parse: %v\n%s", page, perrs[0], described)
	}
	if refErrs, _ := exec.ValidateProgramWithWarnings(prog); len(refErrs) > 0 {
		t.Errorf("check refuses the describe output of %s: %v\n%s", page, refErrs, described)
	}
	projectPath := exec.newExecContext(context.Background()).Backend.Path()
	for _, v := range ValidateWidgetProperties(prog, projectPath) {
		if v.Severity == linter.SeverityError {
			t.Errorf("check refuses the describe output of %s: %s %s", page, v.RuleID, v.Message)
		}
	}
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-executing the describe output of %s: %v\n%s\n%s", page, err, out.String(), described)
	}
	if refsAfter := allAttributeRefs(t, exec, page); !slices.Equal(refsBefore, refsAfter) {
		t.Errorf("%s: bindings after the round trip\n%q\nwant\n%q", page, refsAfter, refsBefore)
	}
	if after := pageUnitBytes(t, exec, page); !bytes.Equal(before, after) {
		t.Errorf("%s: re-executing the describe output rewrote the unit\nexec said: %s\ndescribe:\n%s", page, out.String(), described)
	}
	if again := describeOn(t, exec, out, page); again != described {
		t.Errorf("%s: a second describe differs\nfirst:\n%s\nsecond:\n%s", page, described, again)
	}
	return described
}

func TestPluggableAttributeAssociationPath_DescribeRoundTrips(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	const script = `mdl 1;
create page Administration.P2295 (title: 'P', layout: Atlas_Core.Atlas_Default,
  params: ( $Pwd: Administration.AccountPasswordData, $Acc: Administration.Account )) {
  dataview dvPwd (datasource: $Pwd) {
    switch swPath (attribute: AccountPasswordData_Account/Active)
    datagrid dgPath (datasource: database from Administration.Account,
      configurationAttribute: AccountPasswordData_Account/FullName) {
      column (Attribute: Name, Caption: 'Login')
    }
  }
  dataview dvAcc (datasource: $Acc) {
    switch swPlain (attribute: Active)
  }
};`
	if err := afRun(t, exec, script); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	const page = "Administration.P2295"
	stored := storedAttributeRefs(t, exec, page)
	const step = "|Administration.AccountPasswordData_Account>Administration.Account"
	if stored["swPath"] != "System.User.Active"+step || stored["swPlain"] != "System.User.Active" ||
		!slices.Contains(allAttributeRefs(t, exec, page), "Administration.Account.FullName"+step) {
		t.Fatalf("setup: stored bindings %v", stored)
	}

	described := describeReexecUnchanged(t, exec, out, page)
	// The association path, in the spelling create page accepts.
	if !strings.Contains(described, "switch swPath (booleanAttribute: AccountPasswordData_Account/Active)") {
		t.Errorf("describe should print the association path of swPath:\n%s", described)
	}
	// The same on a widget whose properties describe reads by key: it was
	// left out, with a note that no MDL spelling carries it.
	if !strings.Contains(described, "configurationAttribute: AccountPasswordData_Account/FullName") {
		t.Errorf("describe should print the association path of dgPath's configurationAttribute:\n%s", described)
	}
	// Control: a plain attribute is described as before.
	if !strings.Contains(described, "switch swPlain (booleanAttribute: System.User.Active)") {
		t.Errorf("describe should print swPlain's plain attribute as before:\n%s", described)
	}
}

// The Studio Pro-authored reference for this shape: PedApp
// Administration.Account_Overview's Data grid 2 column over System.UserRoles.
func TestPluggableAttributeAssociationPath_StudioProColumnRoundTrips(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	stored := allAttributeRefs(t, exec, accountOverview)
	if !slices.Contains(stored, "System.UserRole.Name|System.UserRoles>System.UserRole") {
		t.Fatalf("fixture: no AttributeRef System.UserRole.Name over System.UserRoles in %s: %v", accountOverview, stored)
	}
	described := describeReexecUnchanged(t, exec, out, accountOverview)
	if !strings.Contains(described, "column (Attribute: UserRoles/Name, Caption: 'Roles', Sortable: false)") {
		t.Errorf("describe should print the column's association path:\n%s", described)
	}
}

// associationAttributePathText takes only a path, never an expression that
// happens to hold a `/`.
func TestAssociationAttributePathText(t *testing.T) {
	for text, want := range map[string]string{
		"Order_Customer/Active":                      "Order_Customer/Active",
		"Order_Customer / Customer_Country / Code":   "Order_Customer/Customer_Country/Code",
		"Sales.Order_Customer/Sales.Customer/Active": "Sales.Order_Customer/Sales.Customer/Active",
		"$currentObject/Order_Customer/Active":       "$currentObject/Order_Customer/Active",
		"$currentObject/Active":                      "",
		"$Param/Assoc/Attr":                          "",
		"Active":                                     "",
		"Price / 2":                                  "",
		"Total + Order_Customer/Active":              "",
		"Order_Customer/Active != empty":             "",
		"Sales.Module.Entity/Attr":                   "",
	} {
		got, ok := associationAttributePathText(text)
		if (want != "") != ok || got != want {
			t.Errorf("associationAttributePathText(%q) = %q, %v; want %q", text, got, ok, want)
		}
	}
}
