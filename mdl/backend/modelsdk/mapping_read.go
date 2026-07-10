// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genExp "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/exportmappings"
	genImp "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/importmappings"
	genJson "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/jsonstructures"
	genPrj "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/projects"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
)

// ListImportMappings reads every ImportMappings$ImportMapping unit and converts
// it to the semantic model, mirroring the legacy reader for the fields the
// catalog consumes (Name/Documentation/JsonStructure/XmlSchema/
// MessageDefinition and the root mapping Elements). Deep mapping-element fields
// (object handling, value attribute/datatype trees) are not surfaced: the
// catalog only reads the top-level element count, and no other path reaches
// this method.
func (b *Backend) ListImportMappings() ([]*model.ImportMapping, error) {
	units, err := mprread.ListUnitsWithContainer[*genImp.ImportMapping](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ImportMapping, 0, len(units))
	for _, u := range units {
		g := u.Element
		im := &model.ImportMapping{
			ContainerID:       model.ID(u.ContainerID),
			Name:              g.Name(),
			Documentation:     g.Documentation(),
			Excluded:          g.Excluded(),
			ExportLevel:       g.ExportLevel(),
			JsonStructure:     g.JsonStructureQualifiedName(),
			XmlSchema:         g.XmlSchemaQualifiedName(),
			MessageDefinition: g.MessageDefinitionQualifiedName(),
		}
		im.ID = model.ID(g.ID())
		im.TypeName = "ImportMappings$ImportMapping"
		for _, el := range g.RootMappingElementsItems() {
			im.Elements = append(im.Elements, importMappingElementFromGen(el))
		}
		out = append(out, im)
	}
	return out, nil
}

// ListExportMappings reads every ExportMappings$ExportMapping unit and converts
// it to the semantic model, mirroring the legacy reader for the fields the
// catalog consumes (Name/Documentation/JsonStructure/XmlSchema/
// MessageDefinition/NullValueOption and the root mapping Elements). Deep
// mapping-element trees are not surfaced (see ListImportMappings).
func (b *Backend) ListExportMappings() ([]*model.ExportMapping, error) {
	units, err := mprread.ListUnitsWithContainer[*genExp.ExportMapping](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ExportMapping, 0, len(units))
	for _, u := range units {
		g := u.Element
		em := &model.ExportMapping{
			ContainerID:       model.ID(u.ContainerID),
			Name:              g.Name(),
			Documentation:     g.Documentation(),
			Excluded:          g.Excluded(),
			ExportLevel:       g.ExportLevel(),
			JsonStructure:     g.JsonStructureQualifiedName(),
			XmlSchema:         g.XmlSchemaQualifiedName(),
			MessageDefinition: g.MessageDefinitionQualifiedName(),
			NullValueOption:   g.NullValueOption(),
		}
		em.ID = model.ID(g.ID())
		em.TypeName = "ExportMappings$ExportMapping"
		for _, el := range g.RootMappingElementsItems() {
			em.Elements = append(em.Elements, exportMappingElementFromGen(el))
		}
		out = append(out, em)
	}
	return out, nil
}

// ListJsonStructures reads every JsonStructures$JsonStructure unit and converts
// it to the semantic type, mirroring the legacy reader for the fields the
// catalog consumes (Name/Documentation/JsonSnippet/ExportLevel/Excluded and the
// element tree with children).
func (b *Backend) ListJsonStructures() ([]*types.JsonStructure, error) {
	units, err := mprread.ListUnitsWithContainer[*genJson.JsonStructure](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*types.JsonStructure, 0, len(units))
	for _, u := range units {
		g := u.Element
		js := &types.JsonStructure{
			ContainerID:   model.ID(u.ContainerID),
			Name:          g.Name(),
			Documentation: g.Documentation(),
			JsonSnippet:   g.JsonSnippet(),
			Excluded:      g.Excluded(),
			ExportLevel:   g.ExportLevel(),
		}
		js.ID = model.ID(g.ID())
		js.TypeName = "JsonStructures$JsonStructure"
		for _, el := range g.ElementsItems() {
			if je, ok := el.(*genJson.JsonElement); ok {
				js.Elements = append(js.Elements, jsonElementFromGen(je))
			}
		}
		out = append(out, js)
	}
	return out, nil
}

// jsonElementFromGen recursively converts a gen JsonElement to the semantic type.
func jsonElementFromGen(g *genJson.JsonElement) *types.JsonElement {
	e := &types.JsonElement{
		ExposedName:     g.ExposedName(),
		ExposedItemName: g.ExposedItemName(),
		Path:            g.Path(),
		ElementType:     g.ElementType(),
		PrimitiveType:   g.PrimitiveType(),
		MinOccurs:       int(g.MinOccurs()),
		MaxOccurs:       int(g.MaxOccurs()),
		Nillable:        g.Nillable(),
		IsDefaultType:   g.IsDefaultType(),
		MaxLength:       int(g.MaxLength()),
		FractionDigits:  int(g.FractionDigits()),
		TotalDigits:     int(g.TotalDigits()),
		OriginalValue:   g.OriginalValue(),
	}
	for _, child := range g.ChildrenItems() {
		if cj, ok := child.(*genJson.JsonElement); ok {
			e.Children = append(e.Children, jsonElementFromGen(cj))
		}
	}
	return e
}

// ListModuleSettings reads every Projects$ModuleSettings unit and converts it
// to the semantic type, mirroring the legacy reader (top-level fields plus the
// JarDependencies the catalog consumes). The legacy default-value coercion
// (ExportLevel→"Source", ProtectedModuleType→"AddOn", Version→"1.0.0") is
// preserved.
func (b *Backend) ListModuleSettings() ([]*types.ModuleSettings, error) {
	units, err := mprread.ListUnitsWithContainer[*genPrj.ModuleSettings](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*types.ModuleSettings, 0, len(units))
	for _, u := range units {
		g := u.Element
		ms := &types.ModuleSettings{
			ID:                  model.ID(g.ID()),
			ContainerID:         model.ID(u.ContainerID),
			ExportLevel:         g.ExportLevel(),
			ProtectedModuleType: g.ProtectedModuleType(),
			Version:             g.Version(),
			BasedOnVersion:      g.BasedOnVersion(),
			ExtensionName:       g.ExtensionName(),
			SolutionIdentifier:  g.SolutionIdentifier(),
		}
		if ms.ExportLevel == "" {
			ms.ExportLevel = "Source"
		}
		if ms.ProtectedModuleType == "" {
			ms.ProtectedModuleType = "AddOn"
		}
		if ms.Version == "" {
			ms.Version = "1.0.0"
		}
		for _, depEl := range g.JarDependenciesItems() {
			dep, ok := depEl.(*genPrj.JarDependency)
			if !ok {
				continue
			}
			jd := &types.JarDependency{
				ID:         model.ID(dep.ID()),
				GroupID:    dep.GroupId(),
				ArtifactID: dep.ArtifactId(),
				Version:    dep.Version(),
				IsIncluded: dep.IsIncluded(),
			}
			// Reconstruct exclusions so an ALTER MODULE DROP EXCLUSION can find
			// them (the executor reads, mutates, and rewrites the full settings).
			for _, excEl := range dep.ExclusionsItems() {
				exc, ok := excEl.(*genPrj.JarDependencyExclusion)
				if !ok {
					continue
				}
				jd.Exclusions = append(jd.Exclusions, &types.JarDependencyExclusion{
					ID:         model.ID(exc.ID()),
					GroupID:    exc.GroupId(),
					ArtifactID: exc.ArtifactId(),
				})
			}
			ms.JarDependencies = append(ms.JarDependencies, jd)
		}
		out = append(out, ms)
	}
	return out, nil
}

// kindFromElementType maps a mapping element's ElementType ("Object"/"Array"/
// "Value") to the model Kind. Defaults to "Object".
func kindFromElementType(elementType string) string {
	switch elementType {
	case "Array":
		return "Array"
	case "Value":
		return "Value"
	default:
		return "Object"
	}
}

// importMappingElementFromGen recursively converts a gen import-mapping element
// (object or value) to the semantic model. Object elements carry the entity /
// association / object-handling and recurse into children; value elements carry
// the attribute / key / occurrence facets. The microflow builder reads the root
// object element's Entity and MaxOccurs to shape the import result, so these must
// be populated (not just ID/TypeName).
func importMappingElementFromGen(el element.Element) *model.ImportMappingElement {
	e := &model.ImportMappingElement{}
	e.ID = model.ID(el.ID())
	e.TypeName = el.TypeName()
	switch o := el.(type) {
	case *genImp.ImportObjectMappingElement:
		e.Kind = kindFromElementType(o.ElementType())
		e.Entity = o.EntityQualifiedName()
		e.Association = o.AssociationQualifiedName()
		e.ObjectHandling = o.ObjectHandling()
		e.ExposedName = o.ExposedName()
		e.JsonPath = o.JsonPath()
		e.MinOccurs = int(o.MinOccurs())
		e.MaxOccurs = int(o.MaxOccurs())
		e.Nillable = o.Nillable()
		for _, c := range o.ChildrenItems() {
			e.Children = append(e.Children, importMappingElementFromGen(c))
		}
	case *genImp.ImportValueMappingElement:
		e.Kind = "Value"
		e.Attribute = o.AttributeQualifiedName()
		e.IsKey = o.IsKey()
		e.ExposedName = o.ExposedName()
		e.JsonPath = o.JsonPath()
		e.MinOccurs = int(o.MinOccurs())
		e.MaxOccurs = int(o.MaxOccurs())
		e.Nillable = o.Nillable()
		e.OriginalValue = o.OriginalValue()
		e.FractionDigits = int(o.FractionDigits())
		e.TotalDigits = int(o.TotalDigits())
		e.MaxLength = int(o.MaxLength())
	}
	return e
}

// exportMappingElementFromGen recursively converts a gen export-mapping element
// to the semantic model. Mirrors importMappingElementFromGen for the export side.
func exportMappingElementFromGen(el element.Element) *model.ExportMappingElement {
	e := &model.ExportMappingElement{}
	e.ID = model.ID(el.ID())
	e.TypeName = el.TypeName()
	switch o := el.(type) {
	case *genExp.ExportObjectMappingElement:
		e.Kind = kindFromElementType(o.ElementType())
		e.Entity = o.EntityQualifiedName()
		e.Association = o.AssociationQualifiedName()
		e.ObjectHandling = o.ObjectHandling()
		e.ExposedName = o.ExposedName()
		e.JsonPath = o.JsonPath()
		e.MaxOccurs = int(o.MaxOccurs())
		for _, c := range o.ChildrenItems() {
			e.Children = append(e.Children, exportMappingElementFromGen(c))
		}
	case *genExp.ExportValueMappingElement:
		e.Kind = "Value"
		e.Attribute = o.AttributeQualifiedName()
		e.ExposedName = o.ExposedName()
		e.JsonPath = o.JsonPath()
	}
	return e
}
