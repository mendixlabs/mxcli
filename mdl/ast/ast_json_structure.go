// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// JSON Structure Statements
// ============================================================================

// CreateJsonStructureStmt represents:
//
//	CREATE JSON STRUCTURE Module.Name [FOLDER 'path'] FROM '{"json": "snippet"}'
type CreateJsonStructureStmt struct {
	Name        QualifiedName
	JsonSnippet string // raw JSON string to auto-derive element tree from
	Folder      string // optional folder path within the module
}

func (s *CreateJsonStructureStmt) isStatement() {}

// DropJsonStructureStmt represents: DROP JSON STRUCTURE Module.Name
type DropJsonStructureStmt struct {
	Name QualifiedName
}

func (s *DropJsonStructureStmt) isStatement() {}

// ============================================================================
// Import Mapping Statements
// ============================================================================

// CreateImportMappingStmt represents:
//
//	CREATE IMPORT MAPPING Module.Name
//	  [FROM JSON STRUCTURE Module.JsonStructure | FROM XML SCHEMA Module.Schema]
//	{ root -> Module.Entity (Create) { ... } }
type CreateImportMappingStmt struct {
	Name        QualifiedName
	SchemaKind  string        // "JSON_STRUCTURE" or "XML_SCHEMA" or ""
	SchemaRef   QualifiedName // qualified name of the schema source
	RootElement *ImportMappingElementDef
}

func (s *CreateImportMappingStmt) isStatement() {}

// DropImportMappingStmt represents: DROP IMPORT MAPPING Module.Name
type DropImportMappingStmt struct {
	Name QualifiedName
}

func (s *DropImportMappingStmt) isStatement() {}

// ImportMappingElementDef represents one element in the mapping tree.
// It may be an object mapping (→ entity) or a value mapping (→ attribute).
type ImportMappingElementDef struct {
	// JSON field name (or "root" for the root element)
	JsonName string
	// Object mapping fields (set when mapping to an entity)
	Entity         string // qualified entity name (e.g. "Module.Customer")
	ObjectHandling string // "Create", "Find", "FindOrCreate", "Custom"
	Association    string // qualified association name for via clause
	Children       []*ImportMappingElementDef
	// Value mapping fields (set when mapping to an attribute)
	Attribute string // attribute name (unqualified, e.g. "Name")
	DataType  string // "String", "Integer", "Boolean", "Decimal", "DateTime"
	IsKey     bool
}
