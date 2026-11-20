// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// projectDigest hashes every file under the project's directory, so a test can
// tell whether anything at all was written.
func projectDigest(t *testing.T, mpr string) string {
	t.Helper()
	root := filepath.Dir(mpr)
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		lines = append(lines, fmt.Sprintf("%s %x", rel, sha256.Sum256(b)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// ako/mxcli#1917's neighbour, mendixlabs/mxcli#1732 (ako/mxcli#1934):
// `marketplace update` dropped the installed module, copied in the new one, and
// only then opened the downloaded package — which was not where it was looked
// for — so every update failed half-way and left the project without the module
// it was meant to update. An input the update cannot use must stop it before
// the first write.
func TestPerformUpdate_RefusesBeforeDroppingWhenAnInputIsMissing(t *testing.T) {
	for name, missing := range map[string]string{
		"package":           "package",
		"reference project": "reference",
	} {
		t.Run(name, func(t *testing.T) {
			target := copyFixture(t)
			ref := copyFixture(t)
			mpk := emptyMpk(t)
			gone := filepath.Join(t.TempDir(), "absent")
			if missing == "package" {
				mpk = gone + ".mpk"
			} else {
				ref = gone + ".mpr"
			}
			before := projectDigest(t, target)

			_, err := PerformUpdate(target, ref, mpk, "Administration",
				"4.3.2", "4.3.3", "00000000-0000-0000-0000-000000000000", testBackend)
			if err == nil {
				t.Fatal("PerformUpdate succeeded with a missing " + name)
			}
			if after := projectDigest(t, target); after != before {
				t.Fatalf("the project was changed by an update that could not complete (missing %s): %v", name, err)
			}
			if !strings.Contains(err.Error(), "nothing was changed") {
				t.Errorf("the error should say the project is untouched, got: %v", err)
			}
		})
	}
}
