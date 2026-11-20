// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/internal/zipentry"
	"github.com/mendixlabs/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// A module package normally carries its model as a v1 .mpr — an SQLite
// database. Packages exported as protected (add-on) modules, delivered as
// .mxmodule, carry something else under the same name: one BSON document,
// {_ProductVersion, Contents}, where Contents is the whole project as a single
// tree with every unit nested inside its container. Measured on Menditect's
// MtaPluginModule 11.5.5 (Marketplace 214717): `marketplace install` failed on
// it with "file is not a database", and `mx module-import` refuses it outright
// ("Importing protected module is not supported", exit 111).
//
// The tree holds exactly the units a .mpr would, so it is split back into them
// and written into the reference project with the same raw-unit writer the
// transplant uses. Unit contents are copied as they are, protection settings
// (ModuleSettings.ProtectedModuleType, ExportLevel) included: the result is the
// module as the package ships it, not an unprotected copy.
//
// Two things `mx module-import` does besides writing units are done here too,
// because the reference project is read for more than its units:
//
//   - The package's declared files (javasource/, javascriptsource/, ...) are
//     written next to the reference .mpr. DESCRIBE JAVA ACTION and DESCRIBE
//     JAVASCRIPT ACTION read them, and without them every action of the module
//     compares as "changed locally" in `marketplace diff` and `update`
//     (mendixlabs/mxcli#1441).
//   - The units are NOT migrated. mx converts an imported module to the
//     project's Mendix version; these units keep the schema of the version the
//     package was saved with. A tree saved with a newer Mendix than the project
//     is refused, as mx refuses such an import; an older one is written as it
//     is, which is what the measurement above did (11.5.5 into 11.15). A
//     comparison against such a reference can therefore also report the
//     platform's own migrations of the module.

// unitContainments are the properties under which a unit holds other units
// (mpr.IsTreeUnitContainment); the tests check a split unit holds none.
var unitContainments = map[string]bool{
	"Folders":        true,
	"Documents":      true,
	"DomainModel":    true,
	"ModuleSettings": true,
	"ModuleSecurity": true,
}

// treeUnit is one unit cut out of a module tree, ready for AddRawUnit.
type treeUnit struct {
	id, containerID, containment, unitType string
	contents                               []byte
}

// readModuleTree returns the package's project.mpr and the Mendix version it was
// saved with when it is a module tree, and nil when it is an ordinary .mpr.
func readModuleTree(mpkPath string) (data []byte, productVersion string, err error) {
	data, err = readZipEntry(mpkPath, packageProjectEntry)
	if err != nil {
		return nil, "", err
	}
	productVersion, ok := moduleTreeVersion(data)
	if !ok {
		return nil, "", nil
	}
	return data, productVersion, nil
}

func isModuleTree(data []byte) bool {
	_, ok := moduleTreeVersion(data)
	return ok
}

// moduleTreeVersion reports the _ProductVersion of a module tree, and false when
// data is not one.
func moduleTreeVersion(data []byte) (string, bool) { return mpr.ModuleTreeVersion(data) }

// checkTreeVersion refuses a module tree saved with a newer Mendix than the
// project it is written into: its units may use properties or types that
// version does not have, and nothing here converts them. Only major.minor.patch
// count — _ProductVersion carries a build number (11.5.5.73100) that the
// project's version usually does not.
func checkTreeVersion(treeVersion, projectVersion string) error {
	tv, pv := releaseOf(treeVersion), releaseOf(projectVersion)
	if tv == "" || pv == "" || !versionLess(pv, tv) {
		return nil
	}
	return fmt.Errorf("the package was saved with Mendix %s, which is newer than the project's %s; "+
		"install it from Studio Pro %s or later, or upgrade the project first", tv, pv, tv)
}

// releaseOf keeps the first three components of a dotted version.
func releaseOf(v string) string {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ".")
}

// moduleTreeUnits splits the named module of a module tree into units, parents
// before children (mpr.ModuleTreeUnits). The module unit's containerID is left
// empty: the caller attaches it to its own project unit.
func moduleTreeUnits(data []byte, moduleName string) ([]treeUnit, error) {
	split, err := mpr.ModuleTreeUnits(data, moduleName)
	if err != nil {
		return nil, err
	}
	units := make([]treeUnit, len(split))
	for i, u := range split {
		units[i] = treeUnit{id: u.ID, containerID: u.ContainerID, containment: u.ContainmentName, unitType: u.Type, contents: u.Contents}
	}
	return units, nil
}

// buildFromModuleTree is the module-tree counterpart of `mx module-import` for
// the reference project: it writes the package's module into mprPath, a project
// that does not have it yet, and the package's declared files next to it.
func buildFromModuleTree(tree []byte, treeVersion, mpkPath, mprPath, moduleName, projectVersion string) error {
	if err := checkTreeVersion(treeVersion, projectVersion); err != nil {
		return err
	}
	if _, err := insertModuleTree(tree, mprPath, moduleName); err != nil {
		return err
	}
	if _, _, err := InstallPackageFiles(mpkPath, filepath.Dir(mprPath)); err != nil {
		return fmt.Errorf("write the package's files into the reference project: %w", err)
	}
	return nil
}

// insertModuleTree writes the tree's module into mprPath, attached to that
// project's project unit, and returns the number of units written.
func insertModuleTree(tree []byte, mprPath, moduleName string) (int, error) {
	units, err := moduleTreeUnits(tree, moduleName)
	if err != nil {
		return 0, err
	}
	projectID, err := projectUnitID(mprPath)
	if err != nil {
		return 0, err
	}
	if err := refuseIfPresent(mprPath, moduleName); err != nil {
		return 0, err
	}
	writer, err := modelsdk.OpenForWriting(mprPath)
	if err != nil {
		return 0, fmt.Errorf("open %s for writing: %w", mprPath, err)
	}
	defer writer.Disconnect()
	for i, u := range units {
		container := u.containerID
		if container == "" {
			container = projectID
		}
		if err := writer.AddRawUnit(u.id, container, u.containment, u.unitType, u.contents); err != nil {
			return i, fmt.Errorf("write unit %s (%s): %w", u.id, u.unitType, err)
		}
	}
	return len(units), nil
}

func lookup(doc bson.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func readZipEntry(zipPath, entry string) ([]byte, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != entry {
			continue
		}
		rc, err := zipentry.Open(f)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("no %s in %s", entry, zipPath)
}
