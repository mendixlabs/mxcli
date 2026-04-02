// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Entity Statements
// ============================================================================

// EntityKind represents the type of entity (persistent, non-persistent, view).
type EntityKind int

const (
	EntityPersistent EntityKind = iota
	EntityNonPersistent
	EntityView
	EntityExternal
)

func (k EntityKind) String() string {
	switch k {
	case EntityPersistent:
		return "PERSISTENT"
	case EntityNonPersistent:
		return "NON-PERSISTENT"
	case EntityView:
		return "VIEW"
	case EntityExternal:
		return "EXTERNAL"
	default:
		return "PERSISTENT"
	}
}

// CreateEntityStmt represents: CREATE [OR MODIFY] PERSISTENT|NON-PERSISTENT ENTITY Module.Name [EXTENDS Parent] (attributes) ...
type CreateEntityStmt struct {
	Name           QualifiedName
	Kind           EntityKind
	Generalization *QualifiedName // Parent entity for inheritance (e.g., System.Image)
	Attributes     []Attribute
	Indexes        []Index
	Position       *Position
	Documentation  string
	Comment        string
	CreateOrModify bool // true for CREATE OR MODIFY
}

func (s *CreateEntityStmt) isStatement() {}

// DropEntityStmt represents: DROP ENTITY Module.Name
type DropEntityStmt struct {
	Name QualifiedName
}

func (s *DropEntityStmt) isStatement() {}

// AlterEntityOp represents the type of entity alteration.
type AlterEntityOp int

const (
	AlterEntityAddAttribute     AlterEntityOp = iota // ADD ATTRIBUTE / ADD COLUMN
	AlterEntityRenameAttribute                       // RENAME ATTRIBUTE / RENAME COLUMN
	AlterEntityModifyAttribute                       // MODIFY ATTRIBUTE / MODIFY COLUMN
	AlterEntityDropAttribute                         // DROP ATTRIBUTE / DROP COLUMN
	AlterEntitySetDocumentation                      // SET DOCUMENTATION
	AlterEntitySetComment                            // SET COMMENT
	AlterEntityAddIndex                              // ADD INDEX
	AlterEntityDropIndex                             // DROP INDEX
	AlterEntitySetStoreOwner                         // SET STORE OWNER
	AlterEntitySetPosition                           // SET POSITION (x, y)
)

// AlterEntityStmt represents: ALTER ENTITY Module.Name ADD/DROP/RENAME/MODIFY ATTRIBUTE ...
type AlterEntityStmt struct {
	Name                QualifiedName
	Operation           AlterEntityOp
	Attribute           *Attribute     // For ADD ATTRIBUTE
	AttributeName       string         // For RENAME/MODIFY/DROP ATTRIBUTE
	NewName             string         // For RENAME ATTRIBUTE
	DataType            DataType       // For MODIFY ATTRIBUTE
	Calculated          bool           // For MODIFY ATTRIBUTE with CALCULATED
	CalculatedMicroflow *QualifiedName // For MODIFY ATTRIBUTE with CALCULATED microflow
	Documentation       string         // For SET DOCUMENTATION
	Comment             string         // For SET COMMENT
	Index               *Index         // For ADD INDEX
	IndexName           string         // For DROP INDEX
	Position            *Position      // For SET POSITION
}

func (s *AlterEntityStmt) isStatement() {}

// ============================================================================
// View Entity Statements
// ============================================================================

// ViewAttribute represents an attribute in a view entity.
type ViewAttribute struct {
	Name string
	Type DataType
}

// OQLQuery represents a simplified OQL query for view entities.
type OQLQuery struct {
	RawQuery string     // The raw OQL query string for pass-through
	Parsed   *OQLParsed // Structured parse (nil if not parsed)
}

// CreateViewEntityStmt represents: CREATE [OR MODIFY|REPLACE] VIEW ENTITY Module.Name (attrs) AS SELECT ...
type CreateViewEntityStmt struct {
	Name            QualifiedName
	Attributes      []ViewAttribute
	Query           OQLQuery
	Position        *Position
	Documentation   string
	CreateOrModify  bool
	CreateOrReplace bool
}

func (s *CreateViewEntityStmt) isStatement() {}
