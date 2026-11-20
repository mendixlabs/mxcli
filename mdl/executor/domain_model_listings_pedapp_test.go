// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

func domainModelListings(t *testing.T, e *Executor) int64 {
	t.Helper()
	b, ok := e.Backend().(interface{ DomainModelListings() int64 })
	if !ok {
		t.Fatalf("backend %T does not count its domain model listings", e.Backend())
	}
	return b.DomainModelListings()
}

// ako/mxcli#2160, the entity stage: a CREATE ASSOCIATION listed and converted
// every domain model of the project seven times — four to find its two
// entities (each end once for itself and once for the persistence rule), twice
// for the name-space check and once for the member-name check — and a CREATE
// ENTITY three times. Measured here: ten listings for each added entity and
// its association. All but the member-name check ask about one named module,
// and now read that module's domain model alone.
//
// What is left is the member-name check, which follows generalizations across
// the whole project: one listing per statement.
func TestEntityScript_AStatementListsTheProjectsDomainModelsAtMostOnce(t *testing.T) {
	listingsFor := func(entities int) int64 {
		exec, out, _ := openPedAppCopy(t)
		before := domainModelListings(t, exec)
		if err := runScript(t, exec, entitiesScript(entities, 0, "")); err != nil {
			t.Fatalf("exec: %v\n%s", err, out.String())
		}
		return domainModelListings(t, exec) - before
	}
	two, six := listingsFor(2), listingsFor(6)
	// Four more entities and four more associations.
	perPair := float64(six-two) / 4
	t.Logf("listings: %d for 2 entities, %d for 6 — %.1f per added entity and association", two, six, perPair)
	if six == two {
		t.Fatal("control: the added statements listed nothing, so the count is not measuring them")
	}
	if perPair > 2 {
		t.Errorf("%.1f whole-project domain model listings per added entity and its association, want at most 2 (one per statement)", perPair)
	}
}

// Reading one module's domain model must answer what listing the project
// answered. The name-space check still refuses a name another kind holds, one
// that differs from its own kind's only in case, and one in a module named in
// another casing; an association still finds an entity of the virtual System
// module, which no stored domain model holds; and an entity that is not there
// is still reported by its own name.
func TestEntityScript_OneModulesDomainModelAnswersWhatTheListingDid(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := runScript(t, exec, entitiesScript(2, 0, "")); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	const attrs = " (Name: string(200));"
	for _, c := range []struct {
		name, stmt, want string
	}{
		{"an entity named like an association",
			"create persistent entity MyFirstModule.RunProbe02_RunProbe01" + attrs,
			"association MyFirstModule.RunProbe02_RunProbe01 already has that name"},
		{"an association named like an entity",
			"create association MyFirstModule.RunProbe01 from MyFirstModule.RunProbe02 to MyFirstModule.RunProbe01;",
			"entity MyFirstModule.RunProbe01 already has that name"},
		{"an entity whose name differs only in case",
			"create persistent entity MyFirstModule.runprobe01" + attrs,
			"entity MyFirstModule.RunProbe01 already has that name"},
		{"an entity in the module spelled in another case",
			"create persistent entity myfirstmodule.RunProbe01" + attrs,
			"entity MyFirstModule.RunProbe01 already has that name"},
		{"an association to an entity that is not there",
			"create association MyFirstModule.RunProbe01_Nope from MyFirstModule.RunProbe01 to MyFirstModule.Nope;",
			"child entity not found: MyFirstModule.Nope"},
		{"an association from an entity that is not there",
			"create association MyFirstModule.RunProbe01_Gone from MyFirstModule.Gone to MyFirstModule.RunProbe01;",
			"parent entity not found: MyFirstModule.Gone"},
	} {
		err := runScript(t, exec, "mdl 1;\n"+c.stmt)
		if err == nil {
			t.Errorf("%s was accepted: %s", c.name, c.stmt)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %q, want it to say %q", c.name, err.Error(), c.want)
		}
	}
	if err := runScript(t, exec, "mdl 1;\ncreate association MyFirstModule.RunProbe01_User from MyFirstModule.RunProbe01 to System.User;"); err != nil {
		t.Errorf("an association to System.User was refused: %v", err)
	}
}
