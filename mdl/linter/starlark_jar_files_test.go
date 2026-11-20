// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// pathReader is a reader that knows the project file, as the MPR backend does.
type pathReader struct {
	minimalReader
	path string
}

func (r *pathReader) Path() string { return r.path }

// ako/mxcli#1304: a rule could not see the .jar files in userlib/ and
// vendorlib/. jar_dependencies() lists the managed Maven dependencies from
// module settings only, so two versions of one library on the classpath -- a
// hand-added jar in userlib/ and the managed one sync-java-deps put in
// vendorlib/ -- went unseen. The project below is the issue's repro.
func TestJarFiles_Issue1304(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("App.mpr")
	write("userlib/commons-lang3-3.12.0.jar")
	write("userlib/commons-lang3-3.12.0.jar.MyFirstModule.RequiredLib")
	write("userlib/commons-lang3-3.12.0.jar.Sales.RequiredLib")
	write("userlib/log4j-1.2-api-2.17.1.jar") // a name with a version-like part
	write("userlib/README.txt")               // control: not a jar
	write("userlib/sub/nested-1.0.jar")       // control: the runtime does not read subfolders
	write("vendorlib/commons-lang3-3.14.0.jar")
	write("vendorlib/unversioned.jar")

	ctx := linter.NewLintContext(emptyCatalog(t), &pathReader{path: filepath.Join(dir, "App.mpr")})
	vs, _ := runSrc(t, ctx, dumpRule("jar_files()"))
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	want := []string{
		"file_name=commons-lang3-3.12.0.jar|folder=userlib|name=commons-lang3|required_by=[\"MyFirstModule\", \"Sales\"]|version=3.12.0",
		"file_name=log4j-1.2-api-2.17.1.jar|folder=userlib|name=log4j-1.2-api|required_by=[]|version=2.17.1",
		"file_name=commons-lang3-3.14.0.jar|folder=vendorlib|name=commons-lang3|required_by=[]|version=3.14.0",
		"file_name=unversioned.jar|folder=vendorlib|name=unversioned|required_by=[]|version=",
	}
	if !slices.Equal(got, want) {
		t.Errorf("jar_files():\n got  %q\n want %q", got, want)
	}

	// Without a project directory there is nothing to read: [] rather than an
	// error, as the other reader-backed builtins answer without a reader.
	vs, _ = runSrc(t, linter.NewLintContext(emptyCatalog(t), &minimalReader{}), dumpRule("jar_files()"))
	if len(vs) != 0 {
		t.Errorf("jar_files() without a project path: %d rows, want 0", len(vs))
	}
}
