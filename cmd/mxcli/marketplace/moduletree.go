// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/internal/zipentry"
	"github.com/mendixlabs/mxcli/mdl/types"
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

// sqliteMagic opens every SQLite database file, and so every v1 .mpr.
var sqliteMagic = []byte("SQLite format 3\x00")

// unitContainments are the properties under which a unit holds other units.
// They are stored as separate units in a .mpr, and the parent's own contents
// carry none of them — measured on an 11.15 MPR v2 project: a Projects$Folder
// unit is {$ID, $Type, Name}, a Projects$ModuleImpl has no Folders, Documents,
// DomainModel, ModuleSettings or ModuleSecurity key.
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

// IsModuleTreePackage reports whether the package's project.mpr is a module tree
// rather than an SQLite .mpr.
func IsModuleTreePackage(mpkPath string) (bool, error) {
	data, err := readZipEntry(mpkPath, packageProjectEntry)
	if err != nil {
		return false, err
	}
	return isModuleTree(data), nil
}

func isModuleTree(data []byte) bool {
	if bytes.HasPrefix(data, sqliteMagic) || len(data) < 5 {
		return false
	}
	var head struct {
		ProductVersion string `bson:"_ProductVersion"`
	}
	return bson.Unmarshal(data, &head) == nil && head.ProductVersion != ""
}

// moduleTreeUnits splits the named module of a module tree into units, parents
// before children. The module unit's containerID is left empty: the caller
// attaches it to its own project unit.
func moduleTreeUnits(data []byte, moduleName string) ([]treeUnit, error) {
	var root bson.D
	if err := bson.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode module tree: %w", err)
	}
	contents, ok := lookup(root, "Contents").(bson.D)
	if !ok {
		return nil, fmt.Errorf("module tree has no Contents document")
	}
	modules, _ := lookup(contents, "Modules").(bson.A)
	for _, m := range modules {
		mod, ok := m.(bson.D)
		if !ok || !equalFold(stringOf(lookup(mod, "Name")), moduleName) {
			continue
		}
		var units []treeUnit
		if err := splitUnit(mod, "", projectContainment, &units); err != nil {
			return nil, err
		}
		return units, nil
	}
	return nil, fmt.Errorf("module %q not found in the package's module tree", moduleName)
}

// splitUnit appends doc as a unit, then recurses into the units it contains.
func splitUnit(doc bson.D, containerID, containment string, units *[]treeUnit) error {
	id, err := idOf(doc)
	if err != nil {
		return err
	}
	own := make(bson.D, 0, len(doc))
	var children []bson.E
	for _, e := range doc {
		if unitContainments[e.Key] {
			children = append(children, e)
			continue
		}
		own = append(own, e)
	}
	encoded, err := bson.Marshal(own)
	if err != nil {
		return fmt.Errorf("encode unit %s: %w", id, err)
	}
	*units = append(*units, treeUnit{
		id:          id,
		containerID: containerID,
		containment: containment,
		unitType:    stringOf(lookup(doc, "$Type")),
		contents:    encoded,
	})
	for _, c := range children {
		switch v := c.Value.(type) {
		case bson.D:
			if err := splitUnit(v, id, c.Key, units); err != nil {
				return err
			}
		case bson.A:
			// A Mendix list stores its storage-kind marker as element 0; the
			// units are the documents after it.
			for _, item := range v {
				if child, ok := item.(bson.D); ok {
					if err := splitUnit(child, id, c.Key, units); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// InsertModuleTree writes the package's module into mprPath, a project that does
// not have it yet, attached to that project's project unit. It is the
// module-tree counterpart of `mx module-import` for the reference project.
func InsertModuleTree(mpkPath, mprPath, moduleName string) (int, error) {
	data, err := readZipEntry(mpkPath, packageProjectEntry)
	if err != nil {
		return 0, err
	}
	units, err := moduleTreeUnits(data, moduleName)
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

func idOf(doc bson.D) (string, error) {
	switch v := lookup(doc, "$ID").(type) {
	case bson.Binary:
		if len(v.Data) == 16 {
			return types.BlobToUUID(v.Data), nil
		}
	case string:
		if v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("%s without a 16-byte $ID in the module tree", stringOf(lookup(doc, "$Type")))
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
