// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genExp "github.com/mendixlabs/mxcli/modelsdk/gen/exportmappings"
	genImp "github.com/mendixlabs/mxcli/modelsdk/gen/importmappings"
	genJson "github.com/mendixlabs/mxcli/modelsdk/gen/jsonstructures"
	genPrj "github.com/mendixlabs/mxcli/modelsdk/gen/projects"
	genXml "github.com/mendixlabs/mxcli/modelsdk/gen/xmlschemas"
)

// ListImportMappings reads every ImportMappings$ImportMapping unit and converts
// it to the semantic model, mirroring the legacy reader for the fields the
// catalog consumes (Name/Documentation/JsonStructure/XmlSchema/
// MessageDefinition and the root mapping Elements). Deep mapping-element fields
// (object handling, value attribute/datatype trees) are not surfaced: the
// catalog only reads the top-level element count, and no other path reaches
// this method.
func (b *Backend) ListImportMappings() ([]*model.ImportMapping, error) {
	units, err := listUnits[*genImp.ImportMapping](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ImportMapping, 0, len(units))
	for _, u := range units {
		g := u.Element
		im := &model.ImportMapping{
			ContainerID:        model.ID(u.ContainerID),
			Name:               g.Name(),
			Documentation:      g.Documentation(),
			Excluded:           g.Excluded(),
			ExportLevel:        g.ExportLevel(),
			JsonStructure:      g.JsonStructureQualifiedName(),
			XmlSchema:          g.XmlSchemaQualifiedName(),
			MessageDefinition:  optionalStringFromRaw(g.Raw(), "MessageDefinition"),
			ParameterEntity:    parameterEntityFromRaw(g.Raw()),
			MessageDefinition2: messageDefinition2FromRaw(g.Raw()),
			WebServiceSource: model.WebServiceMappingSource{
				ImportedWebService: g.ImportedWebServiceQualifiedName(),
				ServiceName:        g.ServiceName(),
				OperationName:      g.OperationName(),
				RootElementName:    g.RootElementName(),
			},
			RootElementName: g.RootElementName(),
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
	units, err := listUnits[*genExp.ExportMapping](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ExportMapping, 0, len(units))
	for _, u := range units {
		g := u.Element
		em := &model.ExportMapping{
			ContainerID:        model.ID(u.ContainerID),
			Name:               g.Name(),
			Documentation:      g.Documentation(),
			Excluded:           g.Excluded(),
			ExportLevel:        g.ExportLevel(),
			JsonStructure:      g.JsonStructureQualifiedName(),
			XmlSchema:          g.XmlSchemaQualifiedName(),
			MessageDefinition:  optionalStringFromRaw(g.Raw(), "MessageDefinition"),
			NullValueOption:    g.NullValueOption(),
			MessageDefinition2: messageDefinition2FromRaw(g.Raw()),
			// ParameterName and IsHeader are export-only: which SOAP message
			// part the mapping produces, and whether it is a header.
			WebServiceSource: model.WebServiceMappingSource{
				ImportedWebService: g.ImportedWebServiceQualifiedName(),
				ServiceName:        g.ServiceName(),
				OperationName:      g.OperationName(),
				RootElementName:    g.RootElementName(),
				ParameterName:      g.ParameterName(),
				IsHeader:           g.IsHeader(),
			},
			RootElementName: g.RootElementName(),
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

// ListXmlSchemas reads every XmlSchemas$XmlSchema unit.
//
// Shallow on purpose: the entry list holds the imported .xsd, which mxcli does
// not parse, so only the fields that identify the document are converted. Its
// one job is resolving a mapping's `with xml schema` reference (ako/mxcli#259).
func (b *Backend) ListXmlSchemas() ([]*types.XmlSchema, error) {
	units, err := listUnits[*genXml.XmlSchema](b)
	if err != nil {
		return nil, err
	}
	out := make([]*types.XmlSchema, 0, len(units))
	for _, u := range units {
		g := u.Element
		xs := &types.XmlSchema{
			ContainerID:   model.ID(u.ContainerID),
			Module:        b.moduleNameFor(model.ID(g.ID())),
			Name:          g.Name(),
			Documentation: g.Documentation(),
			FilePath:      g.FilePath(),
		}
		xs.ID = model.ID(g.ID())
		xs.TypeName = "XmlSchemas$XmlSchema"
		out = append(out, xs)
	}
	return out, nil
}

// ListJsonStructures reads every JsonStructures$JsonStructure unit and converts
// it to the semantic type, mirroring the legacy reader for the fields the
// catalog consumes (Name/Documentation/JsonSnippet/ExportLevel/Excluded and the
// element tree with children).
func (b *Backend) ListJsonStructures() ([]*types.JsonStructure, error) {
	units, err := listUnits[*genJson.JsonStructure](b)
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
	units, err := listUnits[*genPrj.ModuleSettings](b)
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
		e.ObjectHandlingBackup = o.ObjectHandlingBackup()
		e.BackupAllowOverride = o.ObjectHandlingBackupAllowOverride()
		e.CustomHandler = customHandlerFromRaw(o.Raw())
		e.ExposedName = o.ExposedName()
		e.JsonPath = o.JsonPath()
		e.XmlPath = o.XmlPath()
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
		e.Converter = o.ConverterQualifiedName()
		e.ExposedName = o.ExposedName()
		e.JsonPath = o.JsonPath()
		e.XmlPath = o.XmlPath()
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
		e.CustomHandler = customHandlerFromRaw(o.Raw())
		e.ExposedName = o.ExposedName()
		e.JsonPath = o.JsonPath()
		e.XmlPath = o.XmlPath()
		e.MinOccurs = int(o.MinOccurs())
		e.MaxOccurs = int(o.MaxOccurs())
		for _, c := range o.ChildrenItems() {
			e.Children = append(e.Children, exportMappingElementFromGen(c))
		}
	case *genExp.ExportValueMappingElement:
		e.Kind = "Value"
		e.Attribute = o.AttributeQualifiedName()
		e.Converter = o.ConverterQualifiedName()
		e.ExposedName = o.ExposedName()
		e.JsonPath = o.JsonPath()
		e.XmlPath = o.XmlPath()
		e.OriginalValue = o.OriginalValue()
		e.MinOccurs = int(o.MinOccurs())
		e.MaxOccurs = int(o.MaxOccurs())
		e.MaxLength = int(o.MaxLength())
		e.IsKey = o.IsKey()
	}
	return e
}

// customHandlerFromRaw reads a stored Mappings$MappingMicroflowCallImpl back out
// of the element's raw BSON (#264).
//
// Raw rather than gen: gen's Import/ExportObjectMappingElement declares no
// CustomHandlerCall property at all, so there is no typed accessor to read it
// with — and a property gen does not model is exactly the kind that gets
// silently dropped on a rebuild. The write side is unaffected, because the
// mapping writers build generic elements rather than gen ones.
//
// The parameter's source is recovered from the path marker and level, which is
// how the document distinguishes the four shapes.
func customHandlerFromRaw(raw bson.Raw) *model.MappingMicroflowCall {
	if raw == nil {
		return nil
	}
	callDoc, ok := raw.Lookup("CustomHandlerCall").DocumentOK()
	if !ok {
		return nil
	}
	microflow, _ := callDoc.Lookup("Microflow").StringValueOK()
	if microflow == "" {
		return nil
	}
	out := &model.MappingMicroflowCall{Microflow: microflow}
	arr, ok := callDoc.Lookup("ParameterMappings").ArrayOK()
	if !ok {
		return out
	}
	values, err := arr.Values()
	if err != nil {
		return out
	}
	for _, v := range values {
		pd, ok := v.DocumentOK()
		if !ok {
			continue // the typed-array marker
		}
		jsonPath, _ := pd.Lookup("JsonValueElementPath").StringValueOK()
		// Width-agnostic: Studio Pro stores LevelOfParent as int64 (120 of 120
		// in the ako/mxcli#1264 census), and so does the codec since that fix.
		// Reading Int32OK alone turned every ancestor binding into "parent".
		level := -1
		if l, ok := rawInteger(pd, "LevelOfParent"); ok {
			level = l
		}
		param, _ := pd.Lookup("Parameter").StringValueOK()
		mp := &model.MappingMicroflowParameter{
			Parameter:     param,
			Source:        sourceFromStoredPath(jsonPath, level),
			LevelOfParent: level,
		}
		if mp.Source == "path" {
			mp.ValuePath = jsonPath
			mp.XmlValuePath, _ = pd.Lookup("XmlValueElementPath").StringValueOK()
		}
		out.Parameters = append(out.Parameters, mp)
	}
	return out
}

// sourceFromStoredPath is the reader's half of the source encoding — kept here
// rather than shared with the executor because the backend must not depend on
// it (ADR-0002: the interface speaks the semantic model).
func sourceFromStoredPath(jsonPath string, level int) string {
	switch {
	case jsonPath == "(parent)":
		return "parent"
	case jsonPath == "(parameter)":
		return "parameter"
	case level >= 1:
		return "ancestor"
	case jsonPath != "" && jsonPath != "-":
		return "path"
	default:
		return "parent"
	}
}

// parameterEntityFromRaw reads the mapping's input object (#265). gen models no
// ParameterType property at all, so this goes to the stored document — the same
// route customHandlerFromRaw takes.
//
// Only DataTypes$ObjectType carries an entity; the DataTypes$UnknownType marker
// an unparameterised mapping stores yields "", which is what the model means by
// "no input object".
func parameterEntityFromRaw(raw bson.Raw) string {
	if raw == nil {
		return ""
	}
	doc, ok := raw.Lookup("ParameterType").DocumentOK()
	if !ok {
		return ""
	}
	if t, _ := doc.Lookup("$Type").StringValueOK(); t != "DataTypes$ObjectType" {
		return ""
	}
	entity, _ := doc.Lookup("Entity").StringValueOK()
	return entity
}

// messageDefinition2FromRaw reads the version-introduced MessageDefinition2 key.
//
// gen declares it in version.go (Introduced 11.10.0) but generates NO accessor
// for it, so this goes to the stored document — the same route
// parameterEntityFromRaw and customHandlerFromRaw take.
//
// nil means the key is ABSENT, which the writer must preserve: a document
// written before 11.10 does not carry it, and adding one is the overlay-rule
// mistake (Studio Pro then refuses to open it). Present-and-empty — which is
// what a blank 11.13 app's own mappings store — returns a pointer to "".
func messageDefinition2FromRaw(raw bson.Raw) *string {
	return optionalStringFromRaw(raw, "MessageDefinition2")
}

// optionalStringFromRaw reads a string key whose PRESENCE is version-dependent:
// nil when the stored document does not have the key. MessageDefinition is the
// other one — Mendix 11.15 removed it from every mapping (ako/mxcli#987), so a
// rebuild has to know whether the stored document had it.
func optionalStringFromRaw(raw bson.Raw, key string) *string {
	if raw == nil {
		return nil
	}
	v, err := raw.LookupErr(key)
	if err != nil {
		return nil
	}
	s, ok := v.StringValueOK()
	if !ok {
		return nil
	}
	return &s
}
