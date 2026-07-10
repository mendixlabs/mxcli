// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

// generateID creates a new unique ID for a model element.
func generateID() model.ID {
	return model.ID(mpr.GenerateID())
}

// QualifiedName represents a fully qualified name (Module.Element).
type QualifiedName struct {
	ModuleName  string
	ElementName string
}

// ParseQualifiedName parses a qualified name string into its components.
// Returns the module name and element name.
func ParseQualifiedName(name string) QualifiedName {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return QualifiedName{ModuleName: parts[0], ElementName: parts[1]}
	}
	return QualifiedName{ElementName: name}
}

// String returns the qualified name as a string.
func (qn QualifiedName) String() string {
	if qn.ModuleName == "" {
		return qn.ElementName
	}
	return qn.ModuleName + "." + qn.ElementName
}

// BuildQualifiedName creates a qualified name from module and element names.
func BuildQualifiedName(moduleName, elementName string) string {
	if moduleName == "" {
		return elementName
	}
	return moduleName + "." + elementName
}

// AttributePath represents a fully qualified attribute path (Module.Entity.Attribute).
type AttributePath struct {
	ModuleName    string
	EntityName    string
	AttributeName string
}

// ParseAttributePath parses an attribute path string.
func ParseAttributePath(path string) AttributePath {
	parts := strings.Split(path, ".")
	switch len(parts) {
	case 3:
		return AttributePath{
			ModuleName:    parts[0],
			EntityName:    parts[1],
			AttributeName: parts[2],
		}
	case 2:
		return AttributePath{
			EntityName:    parts[0],
			AttributeName: parts[1],
		}
	case 1:
		return AttributePath{
			AttributeName: parts[0],
		}
	default:
		return AttributePath{}
	}
}

// String returns the attribute path as a string.
func (ap AttributePath) String() string {
	if ap.ModuleName != "" {
		return ap.ModuleName + "." + ap.EntityName + "." + ap.AttributeName
	}
	if ap.EntityName != "" {
		return ap.EntityName + "." + ap.AttributeName
	}
	return ap.AttributeName
}

// EntityQualifiedName returns the entity qualified name.
func (ap AttributePath) EntityQualifiedName() string {
	if ap.ModuleName != "" {
		return ap.ModuleName + "." + ap.EntityName
	}
	return ap.EntityName
}
