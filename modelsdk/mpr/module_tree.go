// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A module package (.mpk, .mxmodule) normally carries its model as a v1 .mpr —
// an SQLite database. Packages exported as protected (add-on) modules carry
// something else under the same name: one BSON document, {_ProductVersion,
// Contents}, where Contents is the whole project as a single tree with every
// unit nested inside its container. Measured on Menditect's MtaPluginModule
// 11.5.5 (Marketplace 214717, mendixlabs/mxcli#1754).
//
// The tree holds exactly the units a .mpr would; ModuleTreeUnits cuts them back
// out. Two readers need that: `marketplace install`, which writes them into a
// reference project, and the project reader, which lists the modules a project
// holds as modules/<Name>.mxmodule (ako/mxcli#1780).

// sqliteMagic opens every SQLite database file, and so every v1 .mpr.
var sqliteMagic = []byte("SQLite format 3\x00")

// IsSQLite reports whether data is an SQLite database, as a v1 .mpr is.
func IsSQLite(data []byte) bool { return bytes.HasPrefix(data, sqliteMagic) }

// treeUnitContainments are the properties under which a unit holds other
// units. They are stored as separate units in a .mpr, and the parent's own
// contents carry none of them — measured on an 11.15 MPR v2 project: a
// Projects$Folder unit is {$ID, $Type, Name}, a Projects$ModuleImpl has no
// Folders, Documents, DomainModel, ModuleSettings or ModuleSecurity key.
//
// Only the unit kinds that hold units are split (treeUnitHolders): a document
// that happens to have a property of one of these names keeps it.
var treeUnitContainments = map[string]bool{
	"Folders":        true,
	"Documents":      true,
	"DomainModel":    true,
	"ModuleSettings": true,
	"ModuleSecurity": true,
}

// IsTreeUnitContainment reports whether key is a property under which a module
// or folder holds units.
func IsTreeUnitContainment(key string) bool { return treeUnitContainments[key] }

// treeUnitHolders are the unit types that contain other units in a module.
var treeUnitHolders = map[string]bool{
	"Projects$ModuleImpl": true,
	"Projects$Folder":     true,
}

// TreeUnit is one unit cut out of a module tree.
type TreeUnit struct {
	ID, ContainerID, ContainmentName, Type string
	Contents                               []byte
}

// ModuleTreeVersion reports the _ProductVersion of a module tree, and false
// when data is not one.
func ModuleTreeVersion(data []byte) (string, bool) {
	if IsSQLite(data) || len(data) < 5 {
		return "", false
	}
	var head struct {
		ProductVersion string `bson:"_ProductVersion"`
	}
	if bson.Unmarshal(data, &head) != nil || head.ProductVersion == "" {
		return "", false
	}
	return head.ProductVersion, true
}

// ModuleTreeUnits splits the named module of a module tree into units, parents
// before children. The module unit's ContainerID is left empty and its
// ContainmentName is "Modules": the caller attaches it to its own project unit.
func ModuleTreeUnits(data []byte, moduleName string) ([]TreeUnit, error) {
	var root bson.D
	if err := bson.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode module tree: %w", err)
	}
	contents, ok := treeLookup(root, "Contents").(bson.D)
	if !ok {
		return nil, fmt.Errorf("module tree has no Contents document")
	}
	modules, _ := treeLookup(contents, "Modules").(bson.A)
	for _, m := range modules {
		mod, ok := m.(bson.D)
		if !ok || !strings.EqualFold(treeString(treeLookup(mod, "Name")), moduleName) {
			continue
		}
		var units []TreeUnit
		if err := splitTreeUnit(mod, "", "Modules", &units); err != nil {
			return nil, err
		}
		return units, nil
	}
	return nil, fmt.Errorf("module %q not found in the package's module tree", moduleName)
}

// splitTreeUnit appends doc as a unit, then recurses into the units it holds.
func splitTreeUnit(doc bson.D, containerID, containment string, units *[]TreeUnit) error {
	id, err := treeID(doc)
	if err != nil {
		return err
	}
	unitType := treeString(treeLookup(doc, "$Type"))
	holder := treeUnitHolders[unitType]
	own := make(bson.D, 0, len(doc))
	var children []bson.E
	for _, e := range doc {
		if holder && treeUnitContainments[e.Key] {
			children = append(children, e)
			continue
		}
		own = append(own, e)
	}
	encoded, err := bson.Marshal(own)
	if err != nil {
		return fmt.Errorf("encode unit %s: %w", id, err)
	}
	*units = append(*units, TreeUnit{
		ID:              id,
		ContainerID:     containerID,
		ContainmentName: containment,
		Type:            unitType,
		Contents:        encoded,
	})
	for _, c := range children {
		switch v := c.Value.(type) {
		case bson.D:
			if err := splitTreeUnit(v, id, c.Key, units); err != nil {
				return err
			}
		case bson.A:
			// A Mendix list stores its storage-kind marker as element 0; the
			// units are the documents after it.
			for _, item := range v {
				if child, ok := item.(bson.D); ok {
					if err := splitTreeUnit(child, id, c.Key, units); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func treeLookup(doc bson.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func treeString(v any) string {
	s, _ := v.(string)
	return s
}

func treeID(doc bson.D) (string, error) {
	switch v := treeLookup(doc, "$ID").(type) {
	case bson.Binary:
		if len(v.Data) == 16 {
			return blobToUUID(v.Data), nil
		}
	case string:
		if v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("%s without an $ID (a 16-byte binary or a non-empty string) in the module tree", treeString(treeLookup(doc, "$Type")))
}
