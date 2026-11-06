// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func gid(last byte) bson.Binary {
	b := make([]byte, 16)
	b[15] = last
	return bson.Binary{Subtype: 0, Data: b}
}

// syntheticTree has the shape of a protected module package's project.mpr: one
// BSON document with the project as a tree and every unit nested in its
// container, lists led by their storage-kind marker.
func syntheticTree(t *testing.T) []byte {
	t.Helper()
	module := bson.D{
		{Key: "$ID", Value: gid(1)},
		{Key: "$Type", Value: "Projects$ModuleImpl"},
		{Key: "Documents", Value: bson.A{int32(3)}},
		{Key: "DomainModel", Value: bson.D{{Key: "$ID", Value: gid(2)}, {Key: "$Type", Value: "DomainModels$DomainModel"}, {Key: "Entities", Value: bson.A{int32(3)}}}},
		{Key: "Folders", Value: bson.A{int32(3), bson.D{
			{Key: "$ID", Value: gid(3)},
			{Key: "$Type", Value: "Projects$Folder"},
			{Key: "Documents", Value: bson.A{int32(3), bson.D{{Key: "$ID", Value: gid(4)}, {Key: "$Type", Value: "Constants$Constant"}, {Key: "Name", Value: "C"}}}},
			{Key: "Folders", Value: bson.A{int32(3)}},
			{Key: "Name", Value: "USE_ME"},
		}}},
		{Key: "ModuleSecurity", Value: bson.D{{Key: "$ID", Value: gid(5)}, {Key: "$Type", Value: "Security$ModuleSecurity"}}},
		{Key: "ModuleSettings", Value: bson.D{{Key: "$ID", Value: gid(6)}, {Key: "$Type", Value: "Projects$ModuleSettings"}, {Key: "ProtectedModuleType", Value: "AddOn"}}},
		{Key: "Name", Value: "Plugin"},
	}
	root := bson.D{
		{Key: "_ProductVersion", Value: "11.0.0.73100"},
		{Key: "Contents", Value: bson.D{
			{Key: "$ID", Value: gid(9)},
			{Key: "$Type", Value: "Projects$Project"},
			{Key: "Modules", Value: bson.A{int32(3), module}},
		}},
	}
	data, err := bson.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestIsModuleTree(t *testing.T) {
	if !isModuleTree(syntheticTree(t)) {
		t.Error("a module tree was not recognised")
	}
	if isModuleTree(append([]byte("SQLite format 3\x00"), make([]byte, 100)...)) {
		t.Error("an SQLite .mpr was taken for a module tree")
	}
}

func TestModuleTreeUnits(t *testing.T) {
	units, err := moduleTreeUnits(syntheticTree(t), "plugin")
	if err != nil {
		t.Fatal(err)
	}
	type want struct{ id, container, containment, unitType string }
	id := func(b byte) string { return types.BlobToUUID(gid(b).Data) }
	expect := []want{
		{id(1), "", "Modules", "Projects$ModuleImpl"},
		{id(2), id(1), "DomainModel", "DomainModels$DomainModel"},
		{id(3), id(1), "Folders", "Projects$Folder"},
		{id(4), id(3), "Documents", "Constants$Constant"},
		{id(5), id(1), "ModuleSecurity", "Security$ModuleSecurity"},
		{id(6), id(1), "ModuleSettings", "Projects$ModuleSettings"},
	}
	if len(units) != len(expect) {
		t.Fatalf("got %d units, want %d", len(units), len(expect))
	}
	for i, w := range expect {
		u := units[i]
		if u.id != w.id || u.containerID != w.container || u.containment != w.containment || u.unitType != w.unitType {
			t.Errorf("unit %d = {%s %s %s %s}, want %+v", i, u.id, u.containerID, u.containment, u.unitType, w)
		}
	}

	// A unit's own contents carry none of its child units, as in a .mpr.
	var folder bson.D
	if err := bson.Unmarshal(units[2].contents, &folder); err != nil {
		t.Fatal(err)
	}
	for _, e := range folder {
		if unitContainments[e.Key] {
			t.Errorf("folder unit still holds %s", e.Key)
		}
	}
	// Protection travels with the module, untouched.
	var settings bson.D
	if err := bson.Unmarshal(units[5].contents, &settings); err != nil {
		t.Fatal(err)
	}
	if stringOf(lookup(settings, "ProtectedModuleType")) != "AddOn" {
		t.Error("ProtectedModuleType was not kept")
	}
}

func TestModuleTreeUnitsUnknownModule(t *testing.T) {
	if _, err := moduleTreeUnits(syntheticTree(t), "Other"); err == nil {
		t.Error("want an error for a module the tree does not hold")
	}
}

func TestCheckTreeVersion(t *testing.T) {
	for _, c := range []struct {
		tree, project string
		refused       bool
	}{
		{"11.5.5.73100", "11.15.0", false},        // older package: written as is
		{"11.5.5.73100", "11.5.5", false},         // same release; the build number does not count
		{"11.15.0.80000", "11.15.0.12345", false}, // same release, both with builds
		{"11.16.0.1", "11.15.0", true},            // newer than the project: refused
		{"12.0.0", "11.15.0", true},
		{"", "11.15.0", false},
		{"11.16.0", "", false},
	} {
		err := checkTreeVersion(c.tree, c.project)
		if (err != nil) != c.refused {
			t.Errorf("checkTreeVersion(%q, %q) = %v, refused want %v", c.tree, c.project, err, c.refused)
		}
	}
}

const pluginPackageXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/">
  <modelerProject xmlns="http://www.mendix.com/modelerProject/1.0/">
    <module name="Plugin" />
    <projectFile path="project.mpr" />
    <files>
      <file path="javasource/plugin/actions/Start.java" />
    </files>
  </modelerProject>
</package>`

// The reference built from a module tree must carry what `mx module-import`
// would have written beside the units: DESCRIBE JAVA ACTION reads
// javasource/<module>/actions/, and without it every action of the module
// compares as changed (mendixlabs/mxcli#1441).
func TestBuildFromModuleTree_WritesTheModuleAndThePackageFiles(t *testing.T) {
	const javaBody = "package plugin.actions; public class Start {}"
	mpk := writeZip(t, map[string][]byte{
		"package.xml":                          []byte(pluginPackageXML),
		packageProjectEntry:                    syntheticTree(t),
		"javasource/plugin/actions/Start.java": []byte(javaBody),
	})
	mprPath := copyFixture(t)

	tree, version, err := readModuleTree(mpk)
	if err != nil || tree == nil {
		t.Fatalf("readModuleTree = %v, %v; want the tree", tree != nil, err)
	}
	if version != "11.0.0.73100" {
		t.Errorf("tree version = %q", version)
	}
	if err := buildFromModuleTree(tree, version, mpk, mprPath, "Plugin", ""); err != nil {
		t.Fatalf("buildFromModuleTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(filepath.Dir(mprPath), "javasource", "plugin", "actions", "Start.java"))
	if err != nil {
		t.Fatalf("the package's Java source was not written next to the reference: %v", err)
	}
	if string(got) != javaBody {
		t.Errorf("Start.java = %q", got)
	}
	if err := refuseIfPresent(mprPath, "Plugin"); err == nil {
		t.Error("the module is not in the reference project")
	}
}

func TestBuildFromModuleTree_RefusesANewerPackage(t *testing.T) {
	mpk := writeZip(t, map[string][]byte{"package.xml": []byte(pluginPackageXML), packageProjectEntry: syntheticTree(t)})
	mprPath := copyFixture(t)
	tree, version, err := readModuleTree(mpk)
	if err != nil {
		t.Fatal(err)
	}
	if err := buildFromModuleTree(tree, version, mpk, mprPath, "Plugin", "10.24.0"); err == nil {
		t.Fatal("a tree saved with 11.0 was written into a 10.24 project")
	}
	if err := refuseIfPresent(mprPath, "Plugin"); err != nil {
		t.Error("the refused module was written anyway")
	}
}

func TestReadModuleTree_AnSQLiteProjectIsNotATree(t *testing.T) {
	mpk := writeZip(t, map[string][]byte{packageProjectEntry: append([]byte("SQLite format 3\x00"), make([]byte, 100)...)})
	tree, _, err := readModuleTree(mpk)
	if err != nil || tree != nil {
		t.Fatalf("readModuleTree = %v, %v; want no tree and no error", tree != nil, err)
	}
}
