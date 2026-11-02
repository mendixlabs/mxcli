// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1512 (mendixlabs/mxcli#1602): after `alter settings constant …
// in configuration …` or `alter settings runtime (BcryptCost: 11)`, diff-local
// printed only "Summary: 0 new, 1 modified, 0 deleted". The settings unit had
// no renderer: it fell to the generic case, two comment lines naming the type,
// which are the same at every revision, so the change counted and showed
// nothing — in every format (`--format struct` said "(no changes)").
//
// The class behind it: a unit whose change lies outside what diff-local's
// renderer prints is counted as modified and silent. Navigation, project and
// module security, a page's or a published REST service's access, an entity
// index — all were. Such a unit is now printed as one line naming the
// properties that changed.
package executor

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// diffLocalAfter runs diff-local against a real MPR v2 project, the way a user
// does: setup is "committed", change is applied to the working tree, and
// diff-local compares the two. git is answered from a snapshot of the units
// taken between the two scripts, so the bytes on both sides are the real
// engine's.
func diffLocalAfter(t *testing.T, setup, change string, opts DiffOptions) string {
	t.Helper()
	var out bytes.Buffer
	x := New(&out)
	x.SetQuiet(true)
	x.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { x.Close() })
	run(t, x, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	if setup != "" {
		run(t, x, setup)
	}
	contents := x.backend.ContentsDir()
	head := readUnits(t, contents)
	if change != "" {
		run(t, x, change)
	}
	work := readUnits(t, contents)

	// `git diff --name-status --relative HEAD` and `git show HEAD:./<path>`,
	// from the snapshot. Like git run in the project directory, the paths are
	// relative to it.
	var status strings.Builder
	snap := t.TempDir()
	files := map[string]string{}
	projectDir := filepath.Dir(contents)
	rel := func(path string) string {
		r, err := filepath.Rel(projectDir, path)
		if err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(r)
	}
	for path, data := range head {
		f := filepath.Join(snap, filepath.Base(path))
		if err := os.WriteFile(f, data, 0o644); err != nil {
			t.Fatal(err)
		}
		files["HEAD:./"+rel(path)] = f
		if w, ok := work[path]; !ok {
			status.WriteString("D\t" + rel(path) + "\n")
		} else if !bytes.Equal(w, data) {
			status.WriteString("M\t" + rel(path) + "\n")
		}
	}
	for path := range work {
		if _, ok := head[path]; !ok {
			status.WriteString("A\t" + rel(path) + "\n")
		}
	}
	listing := filepath.Join(snap, "name-status")
	if err := os.WriteFile(listing, []byte(status.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := execCommand
	t.Cleanup(func() { execCommand = orig })
	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "diff" {
			return exec.Command("cat", listing)
		}
		if len(args) > 0 && args[0] == "ls-files" { // the listing already has the new units
			return exec.Command("true")
		}
		if len(args) == 2 && args[0] == "show" {
			if f, ok := files[args[1]]; ok {
				return exec.Command("cat", f)
			}
		}
		return exec.Command("sh", "-c", "exit 128")
	}

	out.Reset()
	if err := x.DiffLocal("HEAD", opts); err != nil {
		t.Fatalf("diff-local: %v", err)
	}
	return out.String()
}

// readUnits reads every .mxunit under dir, by path.
func readUnits(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	units := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".mxunit") {
			return err
		}
		data, err := os.ReadFile(path)
		units[path] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return units
}

const diffSettingsSetup = `create module M;
create constant M.ApiKey type String default 'x';
alter settings constant @M.ApiKey value 'first-secret' in configuration 'Default';`

func TestDiffLocal_SettingsConstantValueChangeShowsMasked(t *testing.T) {
	got := diffLocalAfter(t, diffSettingsSetup,
		`alter settings constant @M.ApiKey value 'secret123' in configuration 'Default';`, DiffOptions{})

	for _, want := range []string{
		"-alter settings constant 'M.ApiKey' value <masked: old value>",
		"+alter settings constant 'M.ApiKey' value <masked: new value>",
		"Summary: 0 new, 1 modified, 0 deleted",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, secret := range []string{"secret123", "first-secret"} {
		if strings.Contains(got, secret) {
			t.Errorf("diff-local printed the constant value %q:\n%s", secret, got)
		}
	}
}

// A value set where there was none is a new line, masked like any other.
func TestDiffLocal_SettingsConstantValueAddedShowsMasked(t *testing.T) {
	got := diffLocalAfter(t, "create module M;\ncreate constant M.ApiKey type String default 'x';",
		`alter settings constant @M.ApiKey value 'secret123' in configuration 'Default';`, DiffOptions{})

	if !strings.Contains(got, "+alter settings constant 'M.ApiKey' value <masked>\n+  in configuration 'Default';") {
		t.Errorf("the new constant value is not a masked + line:\n%s", got)
	}
	if strings.Contains(got, "secret123") {
		t.Errorf("diff-local printed the constant value:\n%s", got)
	}
}

func TestDiffLocal_SettingsRuntimeChangeRenders(t *testing.T) {
	for _, tc := range []struct {
		format DiffFormat
		want   []string
	}{
		{DiffFormatUnified, []string{"-  BcryptCost: ", "+  BcryptCost: 11,"}},
		{DiffFormatSideBySide, []string{"BcryptCost: 11, ~"}},
		{DiffFormatStructural, []string{"ProjectSettings: ", "~ Line"}},
	} {
		t.Run(string(tc.format), func(t *testing.T) {
			got := diffLocalAfter(t, "", "alter settings runtime (BcryptCost: 11);", DiffOptions{Format: tc.format})
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
			for _, bad := range []string{"(no changes)", "-- Type: Settings$ProjectSettings"} {
				if strings.Contains(got, bad) {
					t.Errorf("still the silent rendering (%q):\n%s", bad, got)
				}
			}
		})
	}
}

// The class: a unit diff-local's renderer is lossy for (module security
// renders as its module's name only) is named, with what changed, in every
// format — not counted and left blank.
func TestDiffLocal_UnrenderedChangeIsNamedInEveryFormat(t *testing.T) {
	const want = "Modified: ModuleSecurity M: changed in properties the MDL rendering does not show: ModuleRoles (0 -> 1 items)\n"
	for _, format := range []DiffFormat{DiffFormatUnified, DiffFormatSideBySide, DiffFormatStructural} {
		t.Run(string(format), func(t *testing.T) {
			got := diffLocalAfter(t, "create module M;", "create module role M.Admin;", DiffOptions{Format: format})
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
			if strings.Contains(got, "(no changes)") {
				t.Errorf("a changed unit reported as \"(no changes)\":\n%s", got)
			}
			if !strings.Contains(got, "Summary: 0 new, 1 modified, 0 deleted") {
				t.Errorf("summary changed:\n%s", got)
			}
		})
	}
}

// Control: a statement that writes nothing (re-applying the stored value is
// elided) leaves nothing to report — the note is about units git says
// changed, not something every run prints.
func TestDiffLocal_UnchangedUnitStaysUnreported(t *testing.T) {
	got := diffLocalAfter(t, diffSettingsSetup,
		`alter settings constant @M.ApiKey value 'first-secret' in configuration 'Default';`, DiffOptions{})
	if strings.TrimSpace(got) != "No local changes found in mxunit files." {
		t.Errorf("an unchanged project produced:\n%s", got)
	}
}

// Control: a change the rendering always showed renders byte for byte as it
// did before the note existed (output captured from 97e1cceac).
func TestDiffLocal_RenderedChangeIsUnchanged(t *testing.T) {
	got := diffLocalAfter(t, "create module M;\ncreate persistent entity M.Item (Name: String(100));",
		"alter entity M.Item modify attribute Name: String(50);", DiffOptions{})
	want := strings.Join([]string{
		"--- DomainModel.M (HEAD)",
		"+++ DomainModel.M (working tree)",
		"@@ -1,6 +1,6 @@",
		" -- Domain Model: M",
		" ",
		" create persistent entity M.Item (",
		"-  Name: String(100)",
		"+  Name: String(50)",
		" );",
		" ",
		"",
		"",
		"Summary: 0 new, 1 modified, 0 deleted",
		"",
	}, "\n")
	if got != want {
		t.Errorf("rendered change differs from before:\n%s\nwant:\n%s", got, want)
	}
}

func TestChangedPaths_NamesPropertiesNotValues(t *testing.T) {
	doc := func(roles bson.A, value string) []byte {
		b, err := bson.Marshal(bson.D{
			{Key: "$Type", Value: "Forms$Page"},
			{Key: "AllowedModuleRoles", Value: roles},
			{Key: "Secret", Value: value},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	got := ChangedPaths(doc(bson.A{int32(1)}, "old-secret"), doc(bson.A{int32(1), "M.User"}, "new-secret"), 4)
	// The leading int32 is Mendix's typed-array marker, not an item.
	want := []string{"AllowedModuleRoles (0 -> 1 items)", "Secret"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("ChangedPaths = %q, want %q", got, want)
	}
}
