// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
)

// TestIssue628_MoveEntitySeesExistingCrossAssociations guards ako/mxcli#628:
// MoveEntity converted the plain associations of the moved entity and never
// looked at the cross-associations that already existed. Moving the second
// endpoint of an association across — the normal way to reorganise a module one
// entity at a time — left a cross-association whose ParentPointer named an
// element absent from its unit, and Studio Pro could no longer load the project
// ("The given key was not present in the dictionary").
//
// The issue's honest assertion is on the stored documents: after every sequence,
// no association pointer may name an element absent from the unit it lives in,
// and no cross-association may name an entity that is not where it says. The
// association's storage GUID must survive each conversion, which only a Studio
// Pro-authored subject (PedApp, GUID != $ID) can detect.
func TestIssue628_MoveEntitySeesExistingCrossAssociations(t *testing.T) {
	type step struct {
		moveFrom bool // move the FROM (parent) entity; otherwise the TO (child) entity
		to       int  // index into the three other modules
	}
	for _, tc := range []struct {
		name  string
		steps []step
		// wantPlainIn is the module index (into others) whose unit must hold the
		// association as a plain association afterwards; -1 means it stays a
		// cross-association.
		wantPlainIn int
		// wantCrossIn / wantChild, when the association stays cross-module: the
		// unit holding it (-1 = the original module) and the module of its TO end.
		wantCrossIn, wantChildIn int
	}{
		// The issue's reproduction: shape 1 seen from the FROM end.
		{name: "ToThenFrom_BackToPlain", steps: []step{{false, 0}, {true, 0}}, wantPlainIn: 0},
		// Shape 1 seen from the TO end: the cross-association is in the target.
		{name: "FromThenTo_BackToPlain", steps: []step{{true, 0}, {false, 0}}, wantPlainIn: 0},
		// Shape 2: the FROM end moves on, the cross-association travels with it.
		{name: "ToThenFromElsewhere_Travels", steps: []step{{false, 0}, {true, 1}}, wantPlainIn: -1, wantCrossIn: 1, wantChildIn: 0},
		// Shape 3: the TO end moves again; the cross-association stays in a module
		// that is neither source nor target of the second move and is re-pointed.
		{name: "ToThenToElsewhere_Repointed", steps: []step{{false, 0}, {false, 1}}, wantPlainIn: -1, wantCrossIn: -1, wantChildIn: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyPedAppFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			srcID, srcMod, assocName, guid, childID, parentID := associationSubjectDistinct(t, b)
			if guid == "" || guid == assocRawID(t, b, srcID, assocName) {
				t.Fatalf("subject %s.%s: GUID %q must exist and differ from $ID", srcMod, assocName, guid)
			}
			others := otherDomainModels(t, b, srcID, 2)

			where := map[model.ID]int{childID: -1, parentID: -1} // -1 = srcMod
			dmOf := func(i int) (model.ID, string) {
				if i < 0 {
					return srcID, srcMod
				}
				return others[i].id, others[i].name
			}
			for _, s := range tc.steps {
				entID := childID
				if s.moveFrom {
					entID = parentID
				}
				fromDM, fromMod := dmOf(where[entID])
				toDM, toMod := dmOf(s.to)
				ent := entityByID(t, b, fromDM, entID)
				if _, err := b.MoveEntity(ent, fromDM, toDM, fromMod, toMod); err != nil {
					t.Fatalf("MoveEntity %s.%s -> %s: %v", fromMod, ent.Name, toMod, err)
				}
				where[entID] = s.to
			}
			if err := b.Disconnect(); err != nil {
				t.Fatalf("disconnect: %v", err)
			}

			b2 := New()
			if err := b2.Connect(proj); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
			t.Cleanup(func() { _ = b2.Disconnect() })

			assertNoDanglingAssociations(t, b2)

			if tc.wantPlainIn >= 0 {
				dmID, mod := dmOf(tc.wantPlainIn)
				if got := crossAssocGUID(t, b2, dmID, assocName); got != "" {
					t.Errorf("%s is still a cross-association in %s; both endpoints are there", assocName, mod)
				}
				got, ok := plainAssocGUID(t, b2, dmID, assocName)
				if !ok {
					t.Fatalf("%s is not a plain association in %s after both endpoints moved there", assocName, mod)
				}
				if got != guid {
					t.Errorf("%s: storage GUID changed %s -> %s", assocName, guid, got)
				}
				return
			}
			dmID, mod := dmOf(tc.wantCrossIn)
			got := crossAssocGUID(t, b2, dmID, assocName)
			if got != guid {
				t.Errorf("%s in %s: storage GUID %q, want %s", assocName, mod, got, guid)
			}
			_, childMod := dmOf(tc.wantChildIn)
			if ref := crossAssocChild(t, b2, dmID, assocName); !strings.HasPrefix(ref, childMod+".") {
				t.Errorf("%s in %s names TO entity %q, want one in %s", assocName, mod, ref, childMod)
			}
		})
	}
}

type dmRef struct {
	id   model.ID
	name string
}

// associationSubjectDistinct is associationSubject restricted to an association
// between two different entities, with no other association between the pair.
func associationSubjectDistinct(t *testing.T, b *Backend) (dmID model.ID, moduleName, assocName, assocGUID string, childID, parentID model.ID) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		gdm, err := b.loadDomainModelGen(d.ID)
		if err != nil {
			continue
		}
		mod, err := b.GetModule(d.ContainerID)
		if err != nil || mod == nil {
			continue
		}
		for _, el := range gdm.AssociationsItems() {
			a, ok := el.(*genDm.Association)
			if !ok || a.Raw() == nil || a.ChildRefID() == a.ParentRefID() {
				continue
			}
			return d.ID, mod.Name, a.Name(), rawKeyHex(t, a.Raw(), "GUID"),
				model.ID(a.ChildRefID()), model.ID(a.ParentRefID())
		}
	}
	t.Fatal("no loadable domain model with an association between two entities")
	return
}

func otherDomainModels(t *testing.T, b *Backend, exclude model.ID, n int) []dmRef {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	var out []dmRef
	for _, d := range dms {
		if d.ID == exclude {
			continue
		}
		if _, err := b.loadDomainModelGen(d.ID); err != nil {
			continue
		}
		mod, err := b.GetModule(d.ContainerID)
		if err != nil || mod == nil {
			continue
		}
		out = append(out, dmRef{d.ID, mod.Name})
		if len(out) == n {
			return out
		}
	}
	t.Fatalf("fixture has fewer than %d other loadable domain models", n)
	return nil
}

func assocRawID(t *testing.T, b *Backend, dmID model.ID, name string) string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	for _, el := range gdm.AssociationsItems() {
		if a, ok := el.(*genDm.Association); ok && a.Name() == name {
			return rawKeyHex(t, a.Raw(), "$ID")
		}
	}
	return ""
}

func plainAssocGUID(t *testing.T, b *Backend, dmID model.ID, name string) (string, bool) {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	for _, el := range gdm.AssociationsItems() {
		if a, ok := el.(*genDm.Association); ok && a.Name() == name {
			return rawKeyHex(t, a.Raw(), "GUID"), true
		}
	}
	return "", false
}

func crossAssocChild(t *testing.T, b *Backend, dmID model.ID, name string) string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	for _, el := range gdm.CrossAssociationsItems() {
		if ca, ok := el.(*genDm.CrossAssociation); ok && ca.Name() == name {
			return ca.ChildQualifiedName()
		}
	}
	return ""
}

// assertNoDanglingAssociations is the stored-document form of "the project
// opens": every association pointer resolves inside its own unit, and every
// cross-association's TO name resolves to an entity in the module it names.
func assertNoDanglingAssociations(t *testing.T, b *Backend) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	type unit struct {
		mod string
		dm  *genDm.DomainModel
	}
	var units []unit
	entitiesByModule := map[string]map[string]bool{}
	for _, d := range dms {
		gdm, err := b.loadDomainModelGen(d.ID)
		if err != nil {
			continue
		}
		mod, err := b.GetModule(d.ContainerID)
		if err != nil || mod == nil {
			continue
		}
		units = append(units, unit{mod.Name, gdm})
		names := map[string]bool{}
		for _, el := range gdm.EntitiesItems() {
			if e, ok := el.(*genDm.Entity); ok {
				names[e.Name()] = true
			}
		}
		entitiesByModule[mod.Name] = names
	}
	for _, u := range units {
		ids := map[string]bool{}
		for _, el := range u.dm.EntitiesItems() {
			ids[string(el.ID())] = true
		}
		for _, el := range u.dm.AssociationsItems() {
			if a, ok := el.(*genDm.Association); ok {
				if !ids[string(a.ParentRefID())] || !ids[string(a.ChildRefID())] {
					t.Errorf("association %s.%s points at an entity outside its unit", u.mod, a.Name())
				}
			}
		}
		for _, el := range u.dm.CrossAssociationsItems() {
			ca, ok := el.(*genDm.CrossAssociation)
			if !ok {
				continue
			}
			if !ids[string(ca.ParentRefID())] {
				t.Errorf("cross-association %s.%s: ParentPointer names an entity absent from its unit", u.mod, ca.Name())
			}
			mod, ent, _ := strings.Cut(ca.ChildQualifiedName(), ".")
			if mod == "System" {
				continue
			}
			if !entitiesByModule[mod][ent] {
				t.Errorf("cross-association %s.%s: Child %q does not resolve", u.mod, ca.Name(), ca.ChildQualifiedName())
			}
			if mod == u.mod {
				t.Errorf("cross-association %s.%s names an entity in its own module; it should be a plain association", u.mod, ca.Name())
			}
		}
	}
}

// copyPedAppFixture copies the Studio Pro-authored PedApp fixture into a temp
// dir; its elements have GUID != $ID, so a lost storage GUID is observable.
func copyPedAppFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../testdata/pedapp"); err != nil {
		t.Fatalf("copy PedApp fixture: %v", err)
	}
	return filepath.Join(dst, "PedApp.mpr")
}
