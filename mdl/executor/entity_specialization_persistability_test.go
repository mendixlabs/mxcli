// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#2005 (mendixlabs/mxcli#1772): `create persistent entity Child
// extends Base`, with Base non-persistent, reported "Created entity" and wrote
// a non-persistent Child — and the other way round. A specialization has no
// persistability of its own: Mendix stores Persistable on the NoGeneralization
// of the root of the chain, and a DomainModels$Generalization holds only the
// parent's name. So the stated keyword was dropped without a word. A keyword
// that contradicts the chain is refused by check and exec alike; one that
// matches it, or none, is accepted.

// storedEntity returns the stored entity qn, or nil.
func storedEntity(t *testing.T, exec *Executor, qn string) *domainmodel.Entity {
	t.Helper()
	return buildEntityIndex(exec.newExecContext(t.Context()))[qn]
}

func TestSpecializationPersistability_ContradictionIsRefused(t *testing.T) {
	const want = "persistability is inherited"
	for _, tc := range []struct {
		name, src, refuse string
		persistable       bool // of the created child, when accepted
	}{
		{"persistent child of a non-persistent parent (the report)",
			"create non-persistent entity MyFirstModule.NBase (Name: String(50));\n" +
				"create persistent entity MyFirstModule.Child extends MyFirstModule.NBase (Code: String(20));", want, false},
		{"non-persistent child of a persistent parent",
			"create persistent entity MyFirstModule.PBase (Name: String(50));\n" +
				"create non-persistent entity MyFirstModule.Child extends MyFirstModule.PBase (Code: String(20));", want, false},
		{"non-persistent child of a persistent System root",
			"create non-persistent entity MyFirstModule.Child extends System.Image (Code: String(20));", want, false},
		{"grandchild: the root decides, not the parent",
			"create non-persistent entity MyFirstModule.NBase (Name: String(50));\n" +
				"create entity MyFirstModule.Mid extends MyFirstModule.NBase (M: String(5));\n" +
				"create persistent entity MyFirstModule.Child extends MyFirstModule.Mid (Code: String(20));", want, false},

		// Controls: the keyword matches the chain, or is left out.
		{"control: non-persistent child of a non-persistent parent",
			"create non-persistent entity MyFirstModule.NBase (Name: String(50));\n" +
				"create non-persistent entity MyFirstModule.Child extends MyFirstModule.NBase (Code: String(20));", "", false},
		{"control: persistent child of a persistent parent",
			"create persistent entity MyFirstModule.PBase (Name: String(50));\n" +
				"create persistent entity MyFirstModule.Child extends MyFirstModule.PBase (Code: String(20));", "", true},
		{"control: no keyword inherits the non-persistent parent's",
			"create non-persistent entity MyFirstModule.NBase (Name: String(50));\n" +
				"create entity MyFirstModule.Child extends MyFirstModule.NBase (Code: String(20));", "", false},
		{"control: persistent child of System.FileDocument",
			"create persistent entity MyFirstModule.Child extends System.FileDocument (Code: String(20));", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, "mdl 1;\n"+tc.src, tc.refuse)
			child := storedEntity(t, exec, "MyFirstModule.Child")
			switch {
			case tc.refuse != "" && child != nil:
				t.Errorf("the refused child was written anyway (persistable %v)", child.Persistable)
			case tc.refuse == "" && child == nil:
				t.Error("control: the child was not written")
			}
		})
	}
}

// The parent is already in the project: exec of a contradicting script after
// the parent was written in an earlier run is refused the same way, and the
// refusal names the root to change.
func TestSpecializationPersistability_StoredParent(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, "mdl 1;\ncreate non-persistent entity MyFirstModule.NBase (Name: String(50));"); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	src := "mdl 1;\ncreate persistent entity MyFirstModule.Child extends MyFirstModule.NBase (Code: String(20));"
	assertAgree(t, exec, dir, src, "MyFirstModule.NBase is non-persistent")
	if storedEntity(t, exec, "MyFirstModule.Child") != nil {
		t.Error("the refused child was written anyway")
	}
}

// describe prints a specialization with the persistability it inherits, so its
// output re-runs: the round trip must not trip the refusal.
func TestSpecializationPersistability_DescribeRoundTrips(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, "mdl 1;\ncreate non-persistent entity MyFirstModule.NBase (Name: String(50));\n"+
		"create entity MyFirstModule.Child extends MyFirstModule.NBase (Code: String(20));"); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := afRun(t, exec, "describe entity MyFirstModule.Child;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	if !strings.Contains(described, "non-persistent entity MyFirstModule.Child extends") {
		t.Fatalf("control: describe does not print the inherited keyword:\n%s", described)
	}
	if !strings.HasPrefix(strings.TrimSpace(described), "mdl ") {
		described = "mdl 1;\n" + described
	}
	if err := agreeExec(t, exec, described); err != nil {
		t.Errorf("re-running the description is refused: %v\n%s", err, described)
	}
}
