// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/model"
)

// check -p and exec must agree on a bare `Module.Name` return type that names
// an enumeration: check resolves the name against entities AND enumerations,
// so exec has to as well. Run on the Studio Pro-authored PedApp fixture, with a
// control that stores the explicit `enum Module.Name` spelling.

const enumReturnSetup = "mdl 1;\ncreate enumeration MyFirstModule.Variant ( Original 'Original', Released 'Released', None 'None' );"

// enumReturnDecl is one flow declaration returning `spelling`, named name.
func enumReturnDecl(kind, name, spelling string) string {
	switch kind {
	case "microflow":
		return fmt.Sprintf("create or modify microflow MyFirstModule.%s () returns %s as $Variant begin return MyFirstModule.Variant.Original; end;", name, spelling)
	case "nanoflow":
		return fmt.Sprintf("create or modify nanoflow MyFirstModule.%s () returns %s as $Variant begin return MyFirstModule.Variant.Original; end;", name, spelling)
	default:
		return fmt.Sprintf("create or modify rule MyFirstModule.%s ($c: MyFirstModule.Variant) returns %s as $Variant begin return MyFirstModule.Variant.Original; end;", name, spelling)
	}
}

func TestCheckExecAgree_BareEnumerationReturnType(t *testing.T) {
	for _, kind := range []string{"microflow", "nanoflow", "rule"} {
		t.Run(kind, func(t *testing.T) {
			exec, out, dir := openPedAppCopy(t)
			if err := agreeExec(t, exec, enumReturnSetup); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			// Control: the explicit spellings both accept.
			assertAgree(t, exec, dir, "mdl 1;\n"+enumReturnDecl(kind, "Ctl", "enum MyFirstModule.Variant"), "")
			assertAgree(t, exec, dir, "mdl 1;\n"+enumReturnDecl(kind, "Ctl2", "Enumeration(MyFirstModule.Variant)"), "")
			// The bare name: check and exec both accept.
			assertAgree(t, exec, dir, "mdl 1;\n"+enumReturnDecl(kind, "Bare", "MyFirstModule.Variant"), "")
			// A name that is neither an entity nor an enumeration: both refuse.
			nope := "mdl 1;\n" + enumReturnDecl(kind, "NopeFlow", "MyFirstModule.Nope")
			assertAgree(t, exec, dir, nope, "MyFirstModule.Nope")
			// Both say the name may have meant either kind.
			if got := strings.Join(agreeCheck(t, exec, dir, nope), "\n"); !strings.Contains(got, "return type of "+kind+" MyFirstModule.NopeFlow: entity or enumeration MyFirstModule.Nope does not exist") {
				t.Errorf("check must name both kinds, reported:\n%s", got)
			}
			if err := agreeExec(t, exec, nope); err == nil || !strings.Contains(err.Error(), "entity or enumeration 'MyFirstModule.Nope' not found for return type") {
				t.Errorf("exec must name both kinds, got: %v", err)
			}
		})
	}
}

// A rule that returns an entity is still refused by both: the fallback widens
// what a bare name can mean, not what a rule may return.
func TestCheckExecAgree_RuleReturningEntity(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	setup := enumReturnSetup + "\ncreate persistent entity MyFirstModule.Thing (Name: String(10));"
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, "mdl 1;\n"+enumReturnDecl("rule", "Bad", "MyFirstModule.Thing"),
		"a rule must return a Boolean or an enumeration, not Entity")
}

// The same script in one run: the enumeration is created by the script itself.
func TestCheckExecAgree_BareEnumerationReturnType_SameScript(t *testing.T) {
	for _, kind := range []string{"microflow", "nanoflow", "rule"} {
		t.Run(kind, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, enumReturnSetup+"\n"+enumReturnDecl(kind, "Bare", "MyFirstModule.Variant"), "")
		})
	}
}

// What the bare spelling stores equals what `enum X` stores, byte for byte
// apart from the document's own id and name.
func TestBareEnumerationReturnType_StoredBytesEqualExplicit(t *testing.T) {
	for _, kind := range []string{"microflow", "nanoflow", "rule"} {
		t.Run(kind, func(t *testing.T) {
			exec, out, _ := openPedAppCopy(t)
			script := enumReturnSetup + "\n" +
				enumReturnDecl(kind, "Explicit", "enum MyFirstModule.Variant") + "\n" +
				enumReturnDecl(kind, "Function", "Enumeration(MyFirstModule.Variant)") + "\n" +
				enumReturnDecl(kind, "Bare", "MyFirstModule.Variant")
			if err := agreeExec(t, exec, script); err != nil {
				t.Fatalf("exec: %v\n%s", err, out.String())
			}
			explicit := normalisedFlowBytes(t, exec, kind, "Explicit")
			for _, name := range []string{"Function", "Bare"} {
				if got := normalisedFlowBytes(t, exec, kind, name); got != explicit {
					t.Errorf("%s stores something other than the explicit `enum` spelling:\n got: %s\nwant: %s", name, got, explicit)
				}
			}
		})
	}
}

// normalisedFlowBytes returns the stored unit of MyFirstModule.<name> as
// extended JSON with the identities removed: every binary value (the $ID, the
// flow pointers, the StableId are all GUIDs) and the document's own name.
func normalisedFlowBytes(t *testing.T, exec *Executor, kind, name string) string {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	var id model.ID
	switch kind {
	case "microflow":
		all, err := ctx.Backend.ListMicroflows()
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range all {
			if m.Name == name {
				id = m.ID
			}
		}
	case "nanoflow":
		all, err := ctx.Backend.ListNanoflows()
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range all {
			if m.Name == name {
				id = m.ID
			}
		}
	default:
		all, err := ctx.Backend.ListRules()
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range all {
			if m.Name == name {
				id = m.ID
			}
		}
	}
	if id == "" {
		t.Fatalf("%s MyFirstModule.%s not found", kind, name)
	}
	raw, err := ctx.Backend.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	out, err := bson.MarshalExtJSON(stripFlowIdentity(d), false, false)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// stripFlowIdentity drops every binary value and the top-level Name, recursively.
func stripFlowIdentity(v bson.D) bson.D {
	var walk func(x any, top bool) any
	walk = func(x any, top bool) any {
		switch t := x.(type) {
		case bson.D:
			var out bson.D
			for _, e := range t {
				if _, isID := e.Value.(primitive.Binary); isID || (top && e.Key == "Name") {
					continue
				}
				out = append(out, bson.E{Key: e.Key, Value: walk(e.Value, false)})
			}
			return out
		case bson.A:
			out := make(bson.A, len(t))
			for i, e := range t {
				out[i] = walk(e, false)
			}
			return out
		default:
			return x
		}
	}
	return walk(v, true).(bson.D)
}
