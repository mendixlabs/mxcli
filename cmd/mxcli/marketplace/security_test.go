// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"go.mongodb.org/mongo-driver/bson"
)

// This ran over both engines while the legacy sdk/mpr writer existed, because
// the reconcile below was a separate implementation in each and a defect fixed
// in one stayed latent in the other until something switched engines — which is
// how mendixlabs/mxcli#1047 reached two engines at once. Legacy is gone
// (docs/plans/2026-09-14-retire-legacy-engine.md), so that class of divergence
// is gone with it and one entry remains. The loop is kept because these tests
// read as a matrix and a second backend would otherwise have to reintroduce it.
var engines = []struct {
	name    string
	backend func() backend.FullBackend
}{
	{"modelsdk", testBackend},
}

// A marketplace update copies the incoming module's units in verbatim, so an
// access rule that does not cover every member of its entity arrives in the
// project exactly as the package shipped it. Mendix rejects that model with
// CE0066 "Entity access is out of date. Please update security by clicking the
// 'Update security' button in the domain model editor" — and the update, having
// written nothing of its own, exits 0 and says nothing about it
// (mendixlabs/mxcli#1085).
//
// Measured before the fix on the vendored 11.6.6 app with Administration 4.3.2,
// mxbuild 11.14.0: one member entry removed from the incoming version's rules
// took `mx check` from 0 errors to
//
//	[error] [CE0066] "Entity access is out of date…" at Domain model of module 'Administration'
//
// while the same update of an UNCHANGED module checked clean — so the machinery
// does not create the staleness, it transmits it. `update security` then
// repaired it (`Reconciled 3 access rule(s) in module Administration`, 1 -> 0
// errors), which is what this test asserts the update now does for itself.
func TestPerformUpdate_ReconcilesAnIncompleteAccessRule(t *testing.T) {
	for _, eng := range engines {
		t.Run(eng.name, func(t *testing.T) {
			target := copyFixture(t)
			ref := copyFixture(t)

			// The control: the incoming version is genuinely missing a member
			// entry. A fixture that failed to strip one would pass this test
			// against unfixed code, which is the failure mode that makes a repro
			// worthless.
			removed := stripLastMemberAccess(t, ref, "Administration", "Account")
			if got := memberRefs(t, ref, "Administration", "Account"); containsRef(got, removed) {
				t.Fatalf("control: %s is still in the reference's rules %v — nothing was stripped", removed, got)
			}

			res, err := PerformUpdate(target, ref, emptyMpk(t), "Administration",
				"4.3.2", "4.3.3", "00000000-0000-0000-0000-000000000000", eng.backend)
			if err != nil {
				t.Fatalf("PerformUpdate: %v", err)
			}
			if res.RulesReconciled == 0 {
				t.Errorf("RulesReconciled = 0; the update reported nothing about repairing the rules")
			}

			got := memberRefs(t, target, "Administration", "Account")
			if !containsRef(got, removed) {
				t.Fatalf("the updated module's rules are still missing %s: %v\n"+
					"this is the CE0066 in mendixlabs/mxcli#1085", removed, got)
			}
		})
	}
}

// The other half of the contract, and the reason this cannot simply reconcile
// unconditionally and call it done: a module whose rules already cover every
// member must come through an update untouched. Reconciling one that needs
// nothing would rewrite a marketplace module's domain model on every update,
// which `marketplace diff` reads back as a local edit — and a local edit is what
// makes the NEXT update refuse.
func TestPerformUpdate_LeavesCompleteRulesAlone(t *testing.T) {
	target := copyFixture(t)
	ref := copyFixture(t)

	before := memberRefs(t, ref, "Administration", "Account")
	res, err := PerformUpdate(target, ref, emptyMpk(t), "Administration",
		"4.3.2", "4.3.3", "00000000-0000-0000-0000-000000000000", testBackend)
	if err != nil {
		t.Fatalf("PerformUpdate: %v", err)
	}
	if res.RulesReconciled != 0 {
		t.Errorf("RulesReconciled = %d on a module whose rules were already complete; want 0",
			res.RulesReconciled)
	}
	if after := memberRefs(t, target, "Administration", "Account"); !sameRefs(before, after) {
		t.Errorf("an already-complete module was rewritten by the update:\nbefore %v\nafter  %v", before, after)
	}
}

// TestPerformInstall_ReconcilesAnIncompleteAccessRule — a first install copies
// the same units through the same transplant, so it lands the same CE0066.
func TestPerformInstall_ReconcilesAnIncompleteAccessRule(t *testing.T) {
	target := copyFixture(t)
	ref := copyFixture(t)

	removed := stripLastMemberAccess(t, ref, "Administration", "Account")
	execMDL(t, target, "drop module Administration;")

	res, err := PerformInstall(target, ref, emptyMpk(t), "Administration",
		"4.3.2", "00000000-0000-0000-0000-000000000000", testBackend)
	if err != nil {
		t.Fatalf("PerformInstall: %v", err)
	}
	if res.RulesReconciled == 0 {
		t.Errorf("RulesReconciled = 0; the install reported nothing about repairing the rules")
	}
	if got := memberRefs(t, target, "Administration", "Account"); !containsRef(got, removed) {
		t.Fatalf("the installed module's rules are still missing %s: %v", removed, got)
	}
}

// --- helpers -------------------------------------------------------------

// emptyMpk is a package with no payload. InstallPackageFiles is not what these
// tests are about, and a real .mpk would drag widget binaries into a temp dir
// for nothing.
func emptyMpk(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "empty.mpk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("package.xml")
	if err != nil {
		t.Fatalf("write package.xml: %v", err)
	}
	if _, err := w.Write([]byte("<package/>")); err != nil {
		t.Fatalf("write package.xml: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
	return path
}

// stripLastMemberAccess removes the last MemberAccess from every populated
// access rule of one entity, and returns the reference it removed. This is what
// a package whose author never pressed "Update security" ships: rules that name
// some of the entity's members and not all of them.
//
// It edits the stored BSON rather than going through MDL on purpose — every
// mxcli write path reconciles as it writes, so no script can produce the state
// being reproduced here.
func stripLastMemberAccess(t *testing.T, mprPath, moduleName, entityName string) string {
	t.Helper()

	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		t.Fatalf("open %s: %v", mprPath, err)
	}
	units, err := reader.ListRawUnitsByType("DomainModels$DomainModel")
	if err != nil {
		reader.Disconnect()
		t.Fatalf("list domain models: %v", err)
	}

	var unitID, removed string
	var contents []byte
	for _, u := range units {
		var doc bson.D
		if bson.Unmarshal(u.Contents, &doc) != nil {
			continue
		}
		ent := findEntityDoc(doc, entityName)
		if ent == nil {
			continue
		}
		rules, _ := bsonLookup(ent, "AccessRules").(bson.A)
		for _, r := range rules {
			rule, ok := r.(bson.D)
			if !ok {
				continue
			}
			for i := range rule {
				if rule[i].Key != "MemberAccesses" {
					continue
				}
				arr, ok := rule[i].Value.(bson.A)
				if !ok || len(arr) < 2 {
					continue // empty rule, or the storage marker alone
				}
				if ref := memberRefOf(arr[len(arr)-1]); ref != "" {
					removed = ref
				}
				rule[i].Value = arr[:len(arr)-1]
			}
		}
		enc, merr := bson.Marshal(doc)
		if merr != nil {
			reader.Disconnect()
			t.Fatalf("re-encode domain model: %v", merr)
		}
		unitID, contents = string(u.ID), enc
		break
	}
	reader.Disconnect()

	if unitID == "" || removed == "" {
		t.Fatalf("no populated access rule found on %s.%s to strip", moduleName, entityName)
	}
	writer, err := modelsdk.OpenForWriting(mprPath)
	if err != nil {
		t.Fatalf("open %s for writing: %v", mprPath, err)
	}
	defer writer.Disconnect()
	if err := writer.UpdateRawUnit(unitID, contents); err != nil {
		t.Fatalf("write domain model: %v", err)
	}
	return removed
}

// memberRefs returns every member reference named by the entity's access rules,
// sorted and deduplicated.
func memberRefs(t *testing.T, mprPath, moduleName, entityName string) []string {
	t.Helper()

	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		t.Fatalf("open %s: %v", mprPath, err)
	}
	defer reader.Disconnect()
	units, err := reader.ListRawUnitsByType("DomainModels$DomainModel")
	if err != nil {
		t.Fatalf("list domain models: %v", err)
	}

	seen := map[string]bool{}
	for _, u := range units {
		var doc bson.D
		if bson.Unmarshal(u.Contents, &doc) != nil {
			continue
		}
		ent := findEntityDoc(doc, entityName)
		if ent == nil {
			continue
		}
		rules, _ := bsonLookup(ent, "AccessRules").(bson.A)
		for _, r := range rules {
			rule, ok := r.(bson.D)
			if !ok {
				continue
			}
			mas, _ := bsonLookup(rule, "MemberAccesses").(bson.A)
			for _, ma := range mas {
				if ref := memberRefOf(ma); ref != "" {
					seen[ref] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for ref := range seen {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func memberRefOf(v any) string {
	d, ok := v.(bson.D)
	if !ok {
		return ""
	}
	if s, _ := bsonLookup(d, "Attribute").(string); s != "" {
		return s
	}
	s, _ := bsonLookup(d, "Association").(string)
	return s
}

func findEntityDoc(dm bson.D, entityName string) bson.D {
	ents, _ := bsonLookup(dm, "Entities").(bson.A)
	for _, e := range ents {
		ed, ok := e.(bson.D)
		if !ok {
			continue
		}
		if name, _ := bsonLookup(ed, "Name").(string); name == entityName {
			return ed
		}
	}
	return nil
}

func bsonLookup(d bson.D, key string) any {
	for _, kv := range d {
		if kv.Key == key {
			return kv.Value
		}
	}
	return nil
}

func containsRef(refs []string, want string) bool {
	for _, r := range refs {
		if r == want {
			return true
		}
	}
	return false
}

func sameRefs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
