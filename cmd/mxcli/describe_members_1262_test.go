// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ako/mxcli#1262 (mendixlabs/mxcli#658, item 1a): `describe <name>` without a
// type resolved documents only. A module role, a user role or a navigation
// profile — each with a describe type of its own — answered "no describable
// document named …".
func TestDescribeAutoDetectsMembers_Issue1262(t *testing.T) {
	mpr := filepath.Join("..", "..", "testdata", "testapp", "TestApp", "TestApp.mpr")
	if _, err := os.Stat(mpr); err != nil {
		t.Skipf("testapp submodule not initialised: %v", err)
	}
	for name, want := range map[string]string{
		"Administration.Administrator": "modulerole",
		"Administrator":                "userrole",
		"Responsive":                   "navigation",
		// Control: a document still resolves as before.
		"Administration.Account": "entity",
	} {
		got, candidates, err := resolveDescribeType(mpr, name)
		if err != nil || got != want {
			t.Errorf("describe %s: resolved to %q (candidates %v, err %v); want %q", name, got, candidates, err, want)
		}
	}
	if _, _, err := resolveDescribeType(mpr, "Administration.NoSuchRole"); err == nil {
		t.Error("a name nothing has resolved")
	}
}
