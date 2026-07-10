// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genJson "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/jsonstructures"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

func init() {
	// A JSON structure's Elements list and each element's Children list always
	// serialize with the typed-array marker 2, even when empty (verified against the
	// legacy serializer). The marker is keyed by the child $Type for the populated
	// case; MandatoryListMarkers covers the empty case (leaf elements / no elements).
	codec.RegisterListMarker("JsonStructures$JsonElement", 2)
	codec.RegisterTypeDefaults("JsonStructures$JsonStructure", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Elements": 2},
	})
	codec.RegisterTypeDefaults("JsonStructures$JsonElement", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Children": 2},
	})
}

// CreateJsonStructure inserts a new JsonStructures$JsonStructure document (its
// JSON snippet plus the parsed element tree). Mirrors the legacy writer.
func (b *Backend) CreateJsonStructure(js *types.JsonStructure) error {
	if js == nil {
		return fmt.Errorf("CreateJsonStructure: nil json structure")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateJsonStructure: not connected for writing")
	}
	if js.ID == "" {
		js.ID = model.ID(mmpr.GenerateID())
	}
	g := jsonStructureToGen(js)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateJsonStructure: encode: %w", err)
	}
	return b.writer.InsertUnit(string(js.ID), string(js.ContainerID), "Documents", "JsonStructures$JsonStructure", contents)
}

// UpdateJsonStructure rewrites an existing JSON structure in place (CREATE OR
// MODIFY), preserving its ID.
func (b *Backend) UpdateJsonStructure(js *types.JsonStructure) error {
	if js == nil {
		return fmt.Errorf("UpdateJsonStructure: nil json structure")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateJsonStructure: not connected for writing")
	}
	g := jsonStructureToGen(js)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdateJsonStructure: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(js.ID), contents)
}

// DeleteJsonStructure removes a JSON structure unit by ID.
func (b *Backend) DeleteJsonStructure(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteJsonStructure: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

// GetJsonStructureByQualifiedName finds a JSON structure by module + name.
func (b *Backend) GetJsonStructureByQualifiedName(moduleName, name string) (*types.JsonStructure, error) {
	all, err := b.ListJsonStructures()
	if err != nil {
		return nil, err
	}
	for _, js := range all {
		if js.Name == name && b.moduleNameFor(js.ID) == moduleName {
			return js, nil
		}
	}
	return nil, fmt.Errorf("json structure not found: %s.%s", moduleName, name)
}

// jsonStructureToGen builds a gen JsonStructure from the semantic type (inverse of
// the ListJsonStructures read converter).
func jsonStructureToGen(js *types.JsonStructure) *genJson.JsonStructure {
	g := genJson.NewJsonStructure()
	g.SetID(element.ID(js.ID))
	g.SetName(js.Name)
	g.SetDocumentation(js.Documentation)
	g.SetExcluded(js.Excluded)
	exportLevel := js.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}
	g.SetExportLevel(exportLevel)
	g.SetJsonSnippet(js.JsonSnippet)
	for _, e := range js.Elements {
		g.AddElements(jsonElementToGen(e))
	}
	return g
}

// jsonElementToGen recursively converts a semantic JsonElement to its gen element.
// Numeric properties are int32 (verified against Studio-Pro BSON, unlike most
// document types which use int64).
func jsonElementToGen(e *types.JsonElement) element.Element {
	g := genJson.NewJsonElement()
	assignID(g)
	g.SetElementType(e.ElementType)
	g.SetPrimitiveType(e.PrimitiveType)
	g.SetPath(e.Path)
	g.SetIsDefaultType(e.IsDefaultType)
	g.SetMinOccurs(int32(e.MinOccurs))
	g.SetMaxOccurs(int32(e.MaxOccurs))
	g.SetNillable(e.Nillable)
	g.SetExposedName(e.ExposedName)
	g.SetExposedItemName(e.ExposedItemName)
	g.SetMaxLength(int32(e.MaxLength))
	g.SetFractionDigits(int32(e.FractionDigits))
	g.SetTotalDigits(int32(e.TotalDigits))
	g.SetOriginalValue(e.OriginalValue)
	g.SetErrorMessage("")
	g.SetWarningMessage("")
	for _, c := range e.Children {
		g.AddChildren(jsonElementToGen(c))
	}
	return g
}
