// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TypeResolver maps human-friendly type aliases to BSON $Type prefixes
// and provides type-aware unit lookups that mpr.Reader shouldn't own.
type TypeResolver struct {
	prefixes map[string]string
}

// NewTypeResolver returns a TypeResolver pre-populated with all known Mendix type mappings.
func NewTypeResolver() *TypeResolver {
	return &TypeResolver{
		prefixes: map[string]string{
			"page":             "Forms$Page",
			"entity":           "DomainModels$Entity",
			"association":      "DomainModels$Association",
			"microflow":        "Microflows$Microflow",
			"nanoflow":         "Microflows$Nanoflow",
			"enumeration":      "Enumerations$Enumeration",
			"snippet":          "Forms$Snippet",
			"layout":           "Forms$Layout",
			"constant":         "Constants$Constant",
			"workflow":         "Workflows$Workflow",
			"imagecollection":  "Images$ImageCollection",
			"javaaction":       "JavaActions$JavaAction",
			"javascriptaction": "JavaScriptActions$JavaScriptAction",
		},
	}
}

// RegisterPrefix adds or overrides a type alias mapping.
func (r *TypeResolver) RegisterPrefix(alias, bsonType string) {
	r.prefixes[strings.ToLower(alias)] = bsonType
}

// BSONType returns the BSON $Type prefix for a given alias.
func (r *TypeResolver) BSONType(alias string) (string, bool) {
	t, ok := r.prefixes[strings.ToLower(alias)]
	return t, ok
}

// GetRawUnitByName finds a unit by human-friendly type alias and qualified name.
// For entities and associations, searches within domain model units.
func (r *TypeResolver) GetRawUnitByName(reader *mpr.Reader, objectType, qualifiedName string) (*mpr.RawUnitInfo, error) {
	lower := strings.ToLower(objectType)

	// Entities and associations are nested inside DomainModel units
	switch lower {
	case "entity":
		return r.getRawEntityByName(reader, qualifiedName)
	case "association":
		return r.getRawAssociationByName(reader, qualifiedName)
	}

	typePrefix, ok := r.prefixes[lower]
	if !ok {
		return nil, fmt.Errorf("unsupported object type: %s", objectType)
	}

	units, err := reader.ListUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}

	modules, err := reader.ListModules()
	if err != nil {
		return nil, err
	}
	moduleMap := make(map[string]string)
	for _, m := range modules {
		moduleMap[string(m.ID)] = m.Name
	}
	containerParent, err := reader.BuildContainerParent()
	if err != nil {
		return nil, err
	}

	for _, u := range units {
		var raw map[string]any
		if err := bson.Unmarshal(u.Contents, &raw); err != nil {
			continue
		}
		name, _ := raw["Name"].(string)
		moduleName := mpr.ResolveModuleName(u.ContainerID, moduleMap, containerParent)
		var fullName string
		if moduleName != "" {
			fullName = moduleName + "." + name
		} else {
			fullName = name
		}
		if fullName == qualifiedName {
			return &mpr.RawUnitInfo{
				ID: u.ID, QualifiedName: fullName,
				Type: u.Type, ModuleName: moduleName,
				Contents: u.Contents,
			}, nil
		}
	}
	return nil, fmt.Errorf("%s not found: %s", objectType, qualifiedName)
}

// ListRawUnits returns all units of a given type alias.
func (r *TypeResolver) ListRawUnits(reader *mpr.Reader, objectType string) ([]*mpr.RawUnitInfo, error) {
	typePrefix := ""
	if objectType != "" {
		var ok bool
		typePrefix, ok = r.prefixes[strings.ToLower(objectType)]
		if !ok {
			return nil, fmt.Errorf("unsupported object type: %s", objectType)
		}
	}

	units, err := reader.ListUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}

	modules, err := reader.ListModules()
	if err != nil {
		return nil, err
	}
	moduleMap := make(map[string]string)
	for _, m := range modules {
		moduleMap[string(m.ID)] = m.Name
	}
	containerParent, err := reader.BuildContainerParent()
	if err != nil {
		return nil, err
	}

	var result []*mpr.RawUnitInfo
	for _, u := range units {
		var raw map[string]any
		if err := bson.Unmarshal(u.Contents, &raw); err != nil {
			continue
		}
		name, _ := raw["Name"].(string)
		moduleName := mpr.ResolveModuleName(u.ContainerID, moduleMap, containerParent)
		fullName := name
		if moduleName != "" {
			fullName = moduleName + "." + name
		}
		result = append(result, &mpr.RawUnitInfo{
			ID: u.ID, QualifiedName: fullName,
			Type: u.Type, ModuleName: moduleName,
			Contents: u.Contents,
		})
	}
	return result, nil
}

// getRawEntityByName finds an entity within domain models.
func (r *TypeResolver) getRawEntityByName(reader *mpr.Reader, qualifiedName string) (*mpr.RawUnitInfo, error) {
	parts := strings.Split(qualifiedName, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid entity name: %s (expected Module.Entity)", qualifiedName)
	}
	targetModule := parts[0]
	targetEntity := parts[1]

	units, err := reader.ListUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return nil, err
	}

	modules, err := reader.ListModules()
	if err != nil {
		return nil, err
	}
	moduleMap := make(map[string]string)
	for _, m := range modules {
		moduleMap[string(m.ID)] = m.Name
	}

	for _, u := range units {
		moduleName := moduleMap[u.ContainerID]
		if moduleName != targetModule {
			continue
		}

		var rawD bson.D
		if err := bson.Unmarshal(u.Contents, &rawD); err != nil {
			continue
		}

		var entitiesVal any
		for _, field := range rawD {
			if field.Key == "Entities" {
				entitiesVal = field.Value
				break
			}
		}

		entities, ok := entitiesVal.(bson.A)
		if !ok {
			continue
		}

		// Skip version marker (first element is int32 array type indicator)
		for i := 1; i < len(entities); i++ {
			entity, ok := entities[i].(bson.D)
			if !ok {
				continue
			}

			for _, field := range entity {
				if field.Key == "Name" {
					if name, ok := field.Value.(string); ok && name == targetEntity {
						entityBytes, err := bson.Marshal(entity)
						if err != nil {
							return nil, err
						}
						return &mpr.RawUnitInfo{
							ID:            u.ID,
							QualifiedName: qualifiedName,
							Type:          "DomainModels$Entity",
							ModuleName:    moduleName,
							Contents:      entityBytes,
						}, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("entity not found: %s", qualifiedName)
}

// getRawAssociationByName finds an association within domain models.
func (r *TypeResolver) getRawAssociationByName(reader *mpr.Reader, qualifiedName string) (*mpr.RawUnitInfo, error) {
	parts := strings.Split(qualifiedName, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid association name: %s (expected Module.AssociationName)", qualifiedName)
	}
	targetModule := parts[0]
	targetAssoc := parts[1]

	units, err := reader.ListUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return nil, err
	}

	modules, err := reader.ListModules()
	if err != nil {
		return nil, err
	}
	moduleMap := make(map[string]string)
	for _, m := range modules {
		moduleMap[string(m.ID)] = m.Name
	}

	for _, u := range units {
		moduleName := moduleMap[u.ContainerID]
		if moduleName != targetModule {
			continue
		}

		var rawD bson.D
		if err := bson.Unmarshal(u.Contents, &rawD); err != nil {
			continue
		}

		var assocsVal any
		for _, field := range rawD {
			if field.Key == "Associations" {
				assocsVal = field.Value
				break
			}
		}

		assocs, ok := assocsVal.(bson.A)
		if !ok {
			continue
		}

		// Skip version marker (first element is int32 array type indicator)
		for i := 1; i < len(assocs); i++ {
			assoc, ok := assocs[i].(bson.D)
			if !ok {
				continue
			}

			for _, field := range assoc {
				if field.Key == "Name" {
					if name, ok := field.Value.(string); ok && name == targetAssoc {
						assocBytes, err := bson.Marshal(assoc)
						if err != nil {
							return nil, err
						}
						return &mpr.RawUnitInfo{
							ID:            u.ID,
							QualifiedName: qualifiedName,
							Type:          "DomainModels$Association",
							ModuleName:    moduleName,
							Contents:      assocBytes,
						}, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("association not found: %s", qualifiedName)
}
