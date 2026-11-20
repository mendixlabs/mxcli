// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// ako/mxcli#1780, ako/mxcli#1998 (mendixlabs/mxcli#1762): a module the project
// holds as modules/<Name>.mxmodule — a packaged or sealed add-on module — has
// no unit in the .mpr, so mxcli did not see it at all. `show modules` left it
// out, `describe nanoflow` said "not found", and `check --references` refused
// valid calls into its public nanoflows: a false-negative gate.
//
// The package is a zip of package.xml and project.mpr. That project.mpr comes
// in two forms, and both are covered: an ordinary v1 .mpr (SQLite), and the
// single-BSON module tree protected add-ons ship (measured on Menditect's
// MtaPluginModule, mendixlabs/mxcli#1754). No Studio Pro-made .mxmodule is
// available here, so the packages are built from a module mxcli wrote into
// each fixture, in the shapes those two measurements describe.

const pluginModuleMDL = `mdl 1;
create or modify module Plugin (ModuleType: addon);
create persistent entity Plugin.Item (Code: String(50));
create nanoflow Plugin.PublicNanoflow () returns Boolean export level api
begin
  return true;
end;
`

// pkgHostFixture is a project to copy: its .mpr and, on v2, mprcontents/.
type pkgHostFixture struct {
	name, dir, mpr string
	v2             bool
	own            string // a module the project stores
}

var (
	pedAppFixture = pkgHostFixture{"v2 PedApp", filepath.Join("..", "..", "testdata", "pedapp"), "PedApp.mpr", true, "MyFirstModule"}
	v1Fixture     = pkgHostFixture{"v1 App", filepath.Join("..", "..", "modelsdk", "mpr", "testdata", "v1-project"), "App.mpr", false, "NanoflowCommons"}
)

// copyProject copies f into a new directory and returns the .mpr's path.
func copyHostProject(t *testing.T, f pkgHostFixture) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(f.dir, f.mpr)); err != nil {
		t.Skipf("%s fixture not found: %v", f.name, err)
	}
	dir := t.TempDir()
	if err := copyPedAppFile(filepath.Join(f.dir, f.mpr), filepath.Join(dir, f.mpr)); err != nil {
		t.Fatal(err)
	}
	if f.v2 {
		if err := copyPedAppTree(filepath.Join(f.dir, "mprcontents"), filepath.Join(dir, "mprcontents")); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, f.mpr)
}

func connectHost(t *testing.T, mprPath string) (*Executor, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	exec := New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	return exec, out
}

func runPkgMDL(t *testing.T, exec *Executor, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	return exec.ExecuteProgram(prog)
}

// pluginProject is a copy of f with the Plugin module written into it.
func pluginProject(t *testing.T, f pkgHostFixture) string {
	t.Helper()
	path := copyHostProject(t, f)
	exec, out := connectHost(t, path)
	if err := runPkgMDL(t, exec, pluginModuleMDL); err != nil {
		t.Fatalf("write the Plugin module: %v\n%s", err, out.String())
	}
	_ = exec.Execute(&ast.DisconnectStmt{})
	return path
}

// moduleTree packs the named module of mprPath into the single BSON document a
// protected module package carries as project.mpr: {_ProductVersion,
// Contents}, the project with every unit nested in its container under its
// containment name, lists led by their storage-kind marker.
func moduleTree(t *testing.T, mprPath, module string) []byte {
	t.Helper()
	r, err := mpr.Open(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	units, err := r.ListUnits()
	if err != nil {
		t.Fatal(err)
	}
	children := map[string][]*mpr.UnitInfo{}
	for _, u := range units {
		if u.ID != u.ContainerID {
			children[u.ContainerID] = append(children[u.ContainerID], u)
		}
	}
	var nest func(u *mpr.UnitInfo) bson.D
	nest = func(u *mpr.UnitInfo) bson.D {
		raw, err := r.GetRawUnitBytes(u.ID)
		if err != nil {
			t.Fatal(err)
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		lists := map[string]bson.A{}
		for _, c := range children[u.ID] {
			switch c.ContainmentName {
			case "DomainModel", "ModuleSettings", "ModuleSecurity":
				doc = append(doc, bson.E{Key: c.ContainmentName, Value: nest(c)})
			default:
				if lists[c.ContainmentName] == nil {
					lists[c.ContainmentName] = bson.A{int32(3)}
				}
				lists[c.ContainmentName] = append(lists[c.ContainmentName], nest(c))
			}
		}
		for _, key := range []string{"Documents", "Folders"} {
			if mpr.IsTreeUnitContainment(key) && (u.Type == "Projects$ModuleImpl" || u.Type == "Projects$Folder") {
				l := lists[key]
				if l == nil {
					l = bson.A{int32(3)}
				}
				doc = append(doc, bson.E{Key: key, Value: l})
			}
		}
		return doc
	}
	var mod bson.D
	for _, u := range units {
		if u.Type != "Projects$ModuleImpl" {
			continue
		}
		raw, _ := r.GetRawUnitBytes(u.ID)
		var head struct{ Name string }
		_ = bson.Unmarshal(raw, &head)
		if head.Name == module {
			mod = nest(u)
		}
	}
	if mod == nil {
		t.Fatalf("no module %s in %s", module, mprPath)
	}
	data, err := bson.Marshal(bson.D{
		{Key: "_ProductVersion", Value: "11.0.0.73100"},
		{Key: "Contents", Value: bson.D{
			{Key: "$Type", Value: "Projects$Project"},
			{Key: "Modules", Value: bson.A{int32(3), mod}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// writeMxModule writes modules/<name>.mxmodule beside hostMpr, with the
// package.xml layout of a Studio Pro module package.
func writeMxModule(t *testing.T, hostMpr, name string, projectMpr []byte) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(hostMpr), "modules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".mxmodule")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range []struct {
		name string
		data []byte
	}{
		{"package.xml", []byte(`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/">
  <modelerProject xmlns="http://www.mendix.com/modelerProject/1.0/">
    <module name="` + name + `" />
    <projectFile path="project.mpr" />
    <files />
  </modelerProject>
</package>`)},
		{"project.mpr", projectMpr},
	} {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// packagedCases are the host project and package forms covered.
func packagedCases(t *testing.T) []struct {
	name string
	host pkgHostFixture
	pkg  func(t *testing.T) []byte
} {
	tree := func(f pkgHostFixture) func(t *testing.T) []byte {
		return func(t *testing.T) []byte { return moduleTree(t, pluginProject(t, f), "Plugin") }
	}
	sqlite := func(t *testing.T) []byte {
		data, err := os.ReadFile(pluginProject(t, v1Fixture))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return []struct {
		name string
		host pkgHostFixture
		pkg  func(t *testing.T) []byte
	}{
		{"v2 host, module tree package", pedAppFixture, tree(pedAppFixture)},
		{"v2 host, SQLite package", pedAppFixture, sqlite},
		{"v1 host, module tree package", v1Fixture, tree(v1Fixture)},
		{"v1 host, SQLite package", v1Fixture, sqlite},
	}
}

func showModulesRow(t *testing.T, exec *Executor, out *bytes.Buffer, module string) string {
	t.Helper()
	out.Reset()
	if err := runPkgMDL(t, exec, "show modules;"); err != nil {
		t.Fatalf("show modules: %v", err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "|")), module+" ") {
			return line
		}
	}
	return ""
}

func usesPlugin(f pkgHostFixture) string {
	return strings.ReplaceAll(usesPluginMDL, "MyFirstModule", f.own)
}

const usesPluginMDL = `mdl 1;
create nanoflow MyFirstModule.UsesPlugin () returns Boolean
begin
  $Ok = call nanoflow Plugin.PublicNanoflow();
  return $Ok;
end;
`

func TestPackagedModule_IsReadLikeAnyOther(t *testing.T) {
	for _, tc := range packagedCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			host := copyHostProject(t, tc.host)
			pkg := tc.pkg(t)

			// Control: without the package the module is not there, and the
			// reference check reports the call — the symptom of #1998.
			exec, out := connectHost(t, host)
			if row := showModulesRow(t, exec, out, "Plugin"); row != "" {
				t.Fatalf("control: Plugin listed before the package exists:\n%s", row)
			}
			prog, _ := visitor.Build(usesPlugin(tc.host))
			if errs, _ := exec.ValidateProgramWithWarnings(prog); len(errs) == 0 || !strings.Contains(fmt.Sprint(errs), "Plugin.PublicNanoflow") {
				t.Fatalf("control: want the reference check to report the call into a module that is not there, got %v", errs)
			}
			_ = exec.Execute(&ast.DisconnectStmt{})

			writeMxModule(t, host, "Plugin", pkg)
			exec, out = connectHost(t, host)

			row := showModulesRow(t, exec, out, "Plugin")
			if row == "" {
				t.Fatalf("show modules omits the packaged module:\n%s", out.String())
			}
			for _, want := range []string{"Packaged", "Protected AddOn", "Add-on"} {
				if !strings.Contains(row, want) {
					t.Errorf("show modules row lacks %q:\n%s", want, row)
				}
			}

			out.Reset()
			if err := runPkgMDL(t, exec, "describe nanoflow Plugin.PublicNanoflow;"); err != nil {
				t.Fatalf("describe nanoflow: %v\n%s", err, out.String())
			}
			if !strings.Contains(out.String(), "Plugin.PublicNanoflow") {
				t.Errorf("describe nanoflow printed:\n%s", out.String())
			}
			out.Reset()
			if err := runPkgMDL(t, exec, "describe entity Plugin.Item;"); err != nil || !strings.Contains(out.String(), "Code") {
				t.Errorf("describe entity: %v\n%s", err, out.String())
			}
			out.Reset()
			if err := runPkgMDL(t, exec, "describe module Plugin;"); err != nil {
				t.Errorf("describe module: %v\n%s", err, out.String())
			}

			prog, _ = visitor.Build(usesPlugin(tc.host))
			if errs, _ := exec.ValidateProgramWithWarnings(prog); len(errs) != 0 {
				t.Errorf("check --references refuses a call into the packaged module's public nanoflow: %v", errs)
			}
		})
	}
}

// The package is the module: nothing in the project holds its units, so a
// write into it would put a unit under a container the .mpr does not have, or
// update a unit that is not stored. Every such write is refused, and nothing
// is written.
func TestPackagedModule_RefusesWrites(t *testing.T) {
	tc := packagedCases(t)[0]
	host := copyHostProject(t, tc.host)
	writeMxModule(t, host, "Plugin", tc.pkg(t))
	exec, out := connectHost(t, host)
	showModulesRow(t, exec, out, "Plugin") // the read's own cache files are not writes
	before := projectFiles(t, filepath.Dir(host))

	for _, src := range []string{
		"mdl 1;\ncreate microflow Plugin.Added () begin end;",
		"mdl 1;\ncreate or modify nanoflow Plugin.PublicNanoflow () returns Boolean export level api\nbegin\n  return false;\nend;",
		"mdl 1;\ndrop nanoflow Plugin.PublicNanoflow;",
		"mdl 1;\nalter entity Plugin.Item add attribute Extra: String(10);",
		"mdl 1;\nalter module Plugin set (Version: '2.0.0');",
	} {
		prog, _ := visitor.Build(src)
		if errs, _ := exec.ValidateProgramWithWarnings(prog); !strings.Contains(fmt.Sprint(errs), "Plugin.mxmodule") {
			t.Errorf("%s\ncheck --references = %v, want it to predict the refusal", src, errs)
		}
		out.Reset()
		err := runPkgMDL(t, exec, src)
		if err == nil || !strings.Contains(err.Error(), "Plugin.mxmodule") {
			t.Errorf("%s\n= %v, want a refusal naming the package\n%s", src, err, out.String())
		}
	}
	if after := projectFiles(t, filepath.Dir(host)); !sameFiles(before, after) {
		t.Errorf("a refused write changed the project:\n%s", diffFiles(before, after))
	}

	// Control: the same kind of write into a module the project stores lands,
	// and check does not refuse it.
	control := "mdl 1;\ncreate microflow MyFirstModule.Added () begin end;"
	prog, _ := visitor.Build(control)
	if errs, _ := exec.ValidateProgramWithWarnings(prog); len(errs) != 0 {
		t.Errorf("control: check refuses a write into a stored module: %v", errs)
	}
	if err := runPkgMDL(t, exec, control); err != nil {
		t.Fatalf("control: %v\n%s", err, out.String())
	}
}

func sameFiles(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func diffFiles(a, b map[string]string) string {
	var lines []string
	for k, v := range a {
		if b[k] != v {
			lines = append(lines, "changed or removed: "+k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			lines = append(lines, "added: "+k)
		}
	}
	return strings.Join(lines, "\n")
}
