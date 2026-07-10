// SPDX-License-Identifier: Apache-2.0

package enginecompare

import (
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	genDm "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	genConst "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/constants"
	_ "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatypes" // register DataTypes$* for constant decode
	genEnum "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/enumerations"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

// MicroflowCanonBSON returns the canonicalized raw BSON of a named microflow unit
// in a module (microflows are top-level documents).
func MicroflowCanonBSON(projectPath, moduleName, mfName string) (string, error) {
	r, err := mmpr.OpenWithOptions(projectPath, mmpr.OpenOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer r.Close()

	mod, err := r.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return "", fmt.Errorf("module %q not found: %v", moduleName, err)
	}
	units, err := mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	if err != nil {
		return "", err
	}
	for _, u := range units {
		if string(u.ContainerID) != mod.ID || u.Element.Name() != mfName {
			continue
		}
		raw, err := r.GetRawUnitBytes(string(u.Element.ID()))
		if err != nil {
			return "", err
		}
		return CanonicalizeRaw(bson.Raw(raw)), nil
	}
	return "", fmt.Errorf("microflow %q not found in module %q", mfName, moduleName)
}

// ConstCanonBSON returns the canonicalized raw BSON of a named constant unit in a
// module (constants are top-level documents).
func ConstCanonBSON(projectPath, moduleName, constName string) (string, error) {
	r, err := mmpr.OpenWithOptions(projectPath, mmpr.OpenOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer r.Close()

	mod, err := r.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return "", fmt.Errorf("module %q not found: %v", moduleName, err)
	}
	units, err := mprread.ListUnitsWithContainer[*genConst.Constant](r)
	if err != nil {
		return "", err
	}
	for _, u := range units {
		if string(u.ContainerID) != mod.ID || u.Element.Name() != constName {
			continue
		}
		raw, err := r.GetRawUnitBytes(string(u.Element.ID()))
		if err != nil {
			return "", err
		}
		return CanonicalizeRaw(bson.Raw(raw)), nil
	}
	return "", fmt.Errorf("constant %q not found in module %q", constName, moduleName)
}

// EnumCanonBSON returns the canonicalized raw BSON of a named enumeration unit in
// a module (enumerations are top-level documents, not domain-model children).
func EnumCanonBSON(projectPath, moduleName, enumName string) (string, error) {
	r, err := mmpr.OpenWithOptions(projectPath, mmpr.OpenOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer r.Close()

	mod, err := r.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return "", fmt.Errorf("module %q not found: %v", moduleName, err)
	}
	units, err := mprread.ListUnitsWithContainer[*genEnum.Enumeration](r)
	if err != nil {
		return "", err
	}
	for _, u := range units {
		if string(u.ContainerID) != mod.ID || u.Element.Name() != enumName {
			continue
		}
		raw, err := r.GetRawUnitBytes(string(u.Element.ID()))
		if err != nil {
			return "", err
		}
		return CanonicalizeRaw(bson.Raw(raw)), nil
	}
	return "", fmt.Errorf("enumeration %q not found in module %q", enumName, moduleName)
}

// volatileKeys hold per-write-random values (IDs, GUIDs); masked before compare
// so two engines' BSON can be diffed on structure + stable values.
var volatileKeys = map[string]bool{
	"$ID":             true,
	"GUID":            true,
	"DataStorageGuid": true,
}

// CanonicalizeRaw renders a BSON document into an order-independent, ID-masked
// string: keys sort, embedded docs/arrays recurse, binaries and volatile keys
// collapse to placeholders. A residual difference is a real structural/value
// divergence. Walks bson.Raw directly — unmarshalling Mendix unit BSON into
// bson.M mis-decodes typed arrays.
func CanonicalizeRaw(raw bson.Raw) string {
	elems, err := raw.Elements()
	if err != nil {
		return "<malformed:" + err.Error() + ">"
	}
	parts := make([]string, 0, len(elems))
	for _, e := range elems {
		k := e.Key()
		if volatileKeys[k] {
			parts = append(parts, k+"=<masked>")
			continue
		}
		parts = append(parts, k+"="+canonValue(e.Value()))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

func canonValue(v bson.RawValue) string {
	switch v.Type {
	case bson.TypeEmbeddedDocument:
		return CanonicalizeRaw(v.Document())
	case bson.TypeArray:
		vals, _ := v.Array().Values()
		parts := make([]string, len(vals))
		for i, rv := range vals {
			parts[i] = canonValue(rv)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case bson.TypeBinary:
		return "<binary>"
	default:
		return v.String()
	}
}

// EntityCanonBSON opens a written project, decodes the named module's domain
// model via the codec, finds the named entity, and returns the canonicalized
// raw BSON of that entity sub-document — the unit of write-parity comparison.
func EntityCanonBSON(projectPath, moduleName, entityName string) (string, error) {
	r, err := mmpr.OpenWithOptions(projectPath, mmpr.OpenOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer r.Close()

	mod, err := r.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return "", fmt.Errorf("module %q not found: %v", moduleName, err)
	}
	units, err := mprread.ListUnitsWithContainer[*genDm.DomainModel](r)
	if err != nil {
		return "", err
	}
	dec := codec.NewDecoder(codec.DefaultRegistry)
	for _, u := range units {
		if string(u.ContainerID) != mod.ID {
			continue
		}
		raw, err := r.GetRawUnitBytes(string(u.Element.ID()))
		if err != nil {
			return "", err
		}
		el, err := dec.Decode(raw)
		if err != nil {
			return "", err
		}
		dm, ok := el.(*genDm.DomainModel)
		if !ok {
			continue
		}
		for _, ee := range dm.EntitiesItems() {
			ent, ok := ee.(*genDm.Entity)
			if !ok || ent.Name() != entityName {
				continue
			}
			return CanonicalizeRaw(bson.Raw(ee.Raw())), nil
		}
	}
	return "", fmt.Errorf("entity %q not found in module %q", entityName, moduleName)
}

// AssociationCanonBSON returns the canonicalized raw BSON of a named association
// in a module's domain model (associations are DM children, not entity children).
func AssociationCanonBSON(projectPath, moduleName, assocName string) (string, error) {
	r, err := mmpr.OpenWithOptions(projectPath, mmpr.OpenOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer r.Close()

	mod, err := r.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return "", fmt.Errorf("module %q not found: %v", moduleName, err)
	}
	units, err := mprread.ListUnitsWithContainer[*genDm.DomainModel](r)
	if err != nil {
		return "", err
	}
	dec := codec.NewDecoder(codec.DefaultRegistry)
	for _, u := range units {
		if string(u.ContainerID) != mod.ID {
			continue
		}
		raw, err := r.GetRawUnitBytes(string(u.Element.ID()))
		if err != nil {
			return "", err
		}
		el, err := dec.Decode(raw)
		if err != nil {
			return "", err
		}
		dm, ok := el.(*genDm.DomainModel)
		if !ok {
			continue
		}
		for _, ae := range dm.AssociationsItems() {
			as, ok := ae.(*genDm.Association)
			if !ok || as.Name() != assocName {
				continue
			}
			return CanonicalizeRaw(bson.Raw(ae.Raw())), nil
		}
	}
	return "", fmt.Errorf("association %q not found in module %q", assocName, moduleName)
}
