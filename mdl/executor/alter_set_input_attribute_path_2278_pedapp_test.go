// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2278: `alter page … set (attribute: …)` on a built-in input widget
// did not resolve the value the way `create page` does. The short association
// path `Assoc/Attr` left the AttributeRef null (the input unbound), and the
// fully qualified `"Mod"."Assoc"/"Mod"."Entity"/"Attr"` was written verbatim as
// one flat attribute name. Even a bare `Attr` was nulled. create page resolves
// all three (resolveInputAttribute; #2255 normalises the qualified form).
//
// The control is create page itself: the same path on the same widget kind
// inside the same data view must store the same AttributeRef — final attribute
// and association steps — whether the widget was created with it or altered
// to it. A value already fully qualified (`Mod.Entity.Attr`) was written as is
// before and still is.

const setup2278 = `mdl 1;
create persistent entity MyFirstModule.Cust2278 (Name: String(100), Birth: DateTime, Active: Boolean default false, Notes: String(unlimited));
create persistent entity MyFirstModule.Ord2278 (Number: String(20), Placed: DateTime, Flag: Boolean default false, Memo: String(unlimited));
create association MyFirstModule.Ord2278_Cust2278 from MyFirstModule.Ord2278 to MyFirstModule.Cust2278;
create page MyFirstModule.Ord2278_Control (title: 'C', layout: Atlas_Core.PopupLayout, params: ( $Ord: MyFirstModule.Ord2278 )) {
  dataview dv (datasource: $Ord) {
    textbox tbShort (attribute: Ord2278_Cust2278/Name)
    textbox tbQual (attribute: "MyFirstModule"."Ord2278_Cust2278"/"MyFirstModule"."Cust2278"/"Name")
    textbox tbBare (attribute: Number)
    textbox tbFull (attribute: MyFirstModule.Ord2278.Number)
    textarea taShort (attribute: Ord2278_Cust2278/Notes)
    datepicker dpShort (attribute: Ord2278_Cust2278/Birth)
    checkbox cbShort (attribute: Ord2278_Cust2278/Active)
    radiobuttons rbShort (attribute: Ord2278_Cust2278/Active)
  }
};
create page MyFirstModule.Ord2278_Edit (title: 'E', layout: Atlas_Core.PopupLayout, params: ( $Ord: MyFirstModule.Ord2278 )) {
  dataview dv (datasource: $Ord) {
    textbox tbShort (attribute: Number)
    textbox tbQual (attribute: Number)
    textbox tbBare (attribute: Memo)
    textbox tbFull (attribute: Memo)
    textarea taShort (attribute: Memo)
    datepicker dpShort (attribute: Placed)
    checkbox cbShort (attribute: Flag)
    radiobuttons rbShort (attribute: Flag)
  }
};`

const alter2278 = `mdl 1;
alter page MyFirstModule.Ord2278_Edit {
  set (attribute: Ord2278_Cust2278/Name) on tbShort;
  set (attribute: "MyFirstModule"."Ord2278_Cust2278"/"MyFirstModule"."Cust2278"/"Name") on tbQual;
  set (attribute: Number) on tbBare;
  set (attribute: MyFirstModule.Ord2278.Number) on tbFull;
  set (attribute: Ord2278_Cust2278/Notes) on taShort;
  set (attribute: Ord2278_Cust2278/Birth) on dpShort;
  set (attribute: Ord2278_Cust2278/Active) on cbShort;
  set (attribute: Ord2278_Cust2278/Active) on rbShort;
};`

// storedInputAttributeRefs returns, per named widget of the stored page, its
// AttributeRef as "Attribute [Assoc>Dest …]", or "<null>".
func storedInputAttributeRefs(t *testing.T, exec *Executor, page string) map[string]string {
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
	get := func(d bson.D, k string) (any, bool) {
		for _, e := range d {
			if e.Key == k {
				return e.Value, true
			}
		}
		return nil, false
	}
	out := map[string]string{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			name, _ := get(x, "Name")
			if n, _ := name.(string); n != "" {
				if ref, has := get(x, "AttributeRef"); has {
					out[n] = describeAttributeRef(ref)
				}
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
	return out
}

func describeAttributeRef(ref any) string {
	d, ok := ref.(bson.D)
	if !ok {
		return "<null>"
	}
	var attr string
	var steps []string
	for _, e := range d {
		switch e.Key {
		case "Attribute":
			attr, _ = e.Value.(string)
		case "EntityRef":
			er, _ := e.Value.(bson.D)
			for _, f := range er {
				if f.Key != "Steps" {
					continue
				}
				arr, _ := f.Value.(bson.A)
				for _, s := range arr {
					sd, ok := s.(bson.D)
					if !ok {
						continue
					}
					var assoc, dest string
					for _, g := range sd {
						switch g.Key {
						case "Association":
							assoc, _ = g.Value.(string)
						case "DestinationEntity":
							dest, _ = g.Value.(string)
						}
					}
					steps = append(steps, assoc+">"+dest)
				}
			}
		}
	}
	return fmt.Sprintf("%s %v", attr, steps)
}

func TestAlterSetInputAttribute_MatchesCreatePage(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, setup2278); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, alter2278, "")

	want := storedInputAttributeRefs(t, exec, "MyFirstModule.Ord2278_Control")
	got := storedInputAttributeRefs(t, exec, "MyFirstModule.Ord2278_Edit")
	for _, w := range []string{"tbShort", "tbQual", "tbBare", "tbFull", "taShort", "dpShort", "cbShort", "rbShort"} {
		if want[w] == "" || want[w] == "<null>" {
			t.Fatalf("control: create page left %s unbound (%q)", w, want[w])
		}
		if got[w] != want[w] {
			t.Errorf("%s: alter set stored %q, create page stores %q", w, got[w], want[w])
		}
	}
	// The plain-attribute control: a fully qualified attribute is stored as written.
	if got["tbFull"] != "MyFirstModule.Ord2278.Number []" {
		t.Errorf("tbFull: a fully qualified attribute must be stored as written, got %q", got["tbFull"])
	}

	// Idempotent: the same SET again writes the same bindings.
	if err := agreeExec(t, exec, alter2278); err != nil {
		t.Fatalf("second alter: %v", err)
	}
	if again := storedInputAttributeRefs(t, exec, "MyFirstModule.Ord2278_Edit"); fmt.Sprint(again) != fmt.Sprint(got) {
		t.Errorf("second alter changed the bindings:\n%v\n%v", got, again)
	}
}

// check judges the altered path as it judges create page's: the qualified form
// exec normalises warns MDL-ATTRPATH01 naming what exec stores, the short form
// says nothing.
func TestAlterSetInputAttribute_CheckJudgesQualifiedPath(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, setup2278); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	warnsOf := func(src string) string {
		prog, perrs := visitor.Build(src)
		if len(perrs) > 0 {
			t.Fatalf("parse: %v", perrs[0])
		}
		errs, warns := exec.ValidateProgramWithWarnings(prog)
		if len(errs) > 0 {
			t.Fatalf("check refused %q: %v", src, errs)
		}
		return strings.Join(warns, "\n")
	}
	qual := warnsOf(`mdl 1;
alter page MyFirstModule.Ord2278_Edit { set (attribute: "MyFirstModule"."Ord2278_Cust2278"/"MyFirstModule"."Cust2278"/"Name") on tbQual; };`)
	if !strings.Contains(qual, "MDL-ATTRPATH01") || !strings.Contains(qual, "`MyFirstModule.Ord2278_Cust2278/Name`") {
		t.Errorf("check must say exec stores `MyFirstModule.Ord2278_Cust2278/Name` [MDL-ATTRPATH01]; warnings:\n%s", qual)
	}
	short := warnsOf(`mdl 1;
alter page MyFirstModule.Ord2278_Edit { set (attribute: Ord2278_Cust2278/Name) on tbShort; };`)
	if strings.Contains(short, "ATTRPATH") {
		t.Errorf("control: the short form must say nothing; warnings:\n%s", short)
	}
}

// A path that does not resolve, and the `Mod.Entity/Attr` shape exec cannot
// normalise, are refused by check and exec alike — before, both were written
// and the input left unbound.
func TestAlterSetInputAttribute_UnresolvablePathsRefused(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, setup2278); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, `mdl 1;
alter page MyFirstModule.Ord2278_Edit { set (attribute: Nope2278_Assoc/Name) on tbShort; };`,
		"`Nope2278_Assoc/Name` does not resolve from MyFirstModule.Ord2278")
	assertAgree(t, exec, dir, `mdl 1;
alter page MyFirstModule.Ord2278_Edit { set (attribute: "MyFirstModule"."Ord2278"/"Number") on tbShort; };`,
		"MDL-ATTRPATH02")
}
