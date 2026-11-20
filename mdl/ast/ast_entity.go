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
	CreateGuard // `create … if not exists` (ako/mxcli#731)
	Name        QualifiedName
	Kind        EntityKind
	// KindStated is true when the statement says `persistent` or
	// `non-persistent`; a bare `create entity` defaults Kind to persistent.
	// A specialization inherits its persistability, so only a stated one can
	// contradict the chain (ako/mxcli#2005).
	KindStated     bool
	Generalization *QualifiedName // Parent entity for inheritance (e.g., System.Image)
	Attributes     []Attribute
	Indexes        []Index
	EventHandlers  []EventHandlerDef // ON BEFORE/AFTER CREATE/COMMIT/DELETE/ROLLBACK CALL ...
	Position       *Position
	Documentation  string
	// DocumentationSet records whether the statement carried a `/** … */`
	// comment at all, as opposed to carrying an empty one. A rewrite that did
	// not mention documentation preserves the stored value; an explicitly empty
	// comment clears it (mendixlabs/mxcli#1018).
	DocumentationSet bool
	CreateOrModify   bool // true for CREATE OR MODIFY
	// ExportLevel is "API" or "Hidden" from an EXPORT LEVEL clause; nil keeps
	// the stored level (Hidden on a new element). mendixlabs/mxcli#1498.
	ExportLevel *string
}

func (s *CreateEntityStmt) isStatement() {}

// DropEntityStmt represents: DROP [EXTERNAL] ENTITY Module.Name
type DropEntityStmt struct {
	DropGuard
	Name QualifiedName
	// External is `drop external entity`: the entity must be an external
	// (OData) entity, and a local one is refused rather than dropped.
	External bool
}

func (s *DropEntityStmt) isStatement() {}

// AlterEntityOp represents the type of entity alteration.
type AlterEntityOp int

const (
	AlterEntityAddAttribute                AlterEntityOp = iota // ADD ATTRIBUTE / ADD COLUMN
	AlterEntityRenameAttribute                                  // RENAME ATTRIBUTE / RENAME COLUMN
	AlterEntityModifyAttribute                                  // MODIFY ATTRIBUTE / MODIFY COLUMN
	AlterEntityDropAttribute                                    // DROP ATTRIBUTE / DROP COLUMN
	AlterEntitySetDocumentation                                 // SET DOCUMENTATION
	AlterEntitySetComment                                       // SET COMMENT
	AlterEntityAddIndex                                         // ADD INDEX
	AlterEntityDropIndex                                        // DROP INDEX
	AlterEntitySetPosition                                      // SET POSITION (x, y)
	AlterEntityAddEventHandler                                  // ADD EVENT HANDLER ON BEFORE/AFTER CREATE/COMMIT/DELETE/ROLLBACK CALL Mod.MF
	AlterEntityDropEventHandler                                 // DROP EVENT HANDLER ON BEFORE/AFTER CREATE/COMMIT/DELETE/ROLLBACK [CALL Mod.MF]
	AlterEntitySetAllowCreateChangeLocally                      // SET ALLOW_CREATE_CHANGE_LOCALLY = true/false
	AlterEntityDropDefault                                      // DROP DEFAULT ON ATTRIBUTE — clear a default value
	AlterEntitySetExportLevel                                   // SET EXPORT LEVEL API | HIDDEN
)

// EventHandlerDef represents an event handler in CREATE/ALTER ENTITY syntax.
type EventHandlerDef struct {
	Moment            string        // "Before" or "After"
	Event             string        // "Create", "Commit", "Delete", "Rollback"
	Microflow         QualifiedName // Microflow to call; on DROP, empty unless CALL names one
	RaiseErrorOnFalse bool          // RAISE ERROR clause present
	PassEventObject   bool          // Whether to pass entity object (default true)
}

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
	// MODIFY ATTRIBUTE constraint changes (Bug 12a). A nil pointer means
	// "preserve the existing constraint"; a non-nil pointer means "set it".
	// NULLABLE sets ModifyNotNull=false (clears the Required rule); NOT NULL /
	// REQUIRED sets it true. UNIQUE sets ModifyUnique=true (there is no
	// "not unique" keyword). DEFAULT sets ModifyHasDefault + ModifyDefaultValue.
	ModifyNotNull      *bool
	ModifyNotNullError string
	ModifyUnique       *bool
	ModifyUniqueError  string
	ModifyHasDefault   bool
	ModifyDefaultValue any
	// ModifyExportLevel is MODIFY ATTRIBUTE's EXPORT LEVEL clause; nil keeps
	// the stored level.
	ModifyExportLevel *string
	ExportLevel       string           // For SET EXPORT LEVEL: "API" or "Hidden"
	Documentation     string           // For SET DOCUMENTATION
	Comment           string           // For SET COMMENT
	Index             *Index           // For ADD INDEX, and DROP INDEX by column list
	IndexName         string           // For DROP INDEX by ordinal name ("idx1", "idx2", ...)
	Position          *Position        // For SET POSITION
	EventHandler      *EventHandlerDef // For ADD/DROP EVENT HANDLER
	BoolValue         bool             // For SET ALLOW_CREATE_CHANGE_LOCALLY
	// Idempotency guards (findings #10). IfNotExists on ADD ATTRIBUTE skips the
	// add (with a notice) when the attribute already exists; IfExists on DROP
	// ATTRIBUTE skips the drop when it is already gone — so a domain script
	// re-runs cleanly instead of erroring and halting.
	IfNotExists bool // For ADD ATTRIBUTE / ADD INDEX / ADD EVENT HANDLER IF NOT EXISTS
	IfExists    bool // For DROP ATTRIBUTE / DROP INDEX / DROP EVENT HANDLER IF EXISTS
}

func (s *AlterEntityStmt) isStatement() {}

// EntityPersistenceFilter narrows ALTER ENTITIES to one persistence kind.
type EntityPersistenceFilter int

const (
	// EntityFilterAll is the absence of a WHERE clause.
	EntityFilterAll EntityPersistenceFilter = iota
	EntityFilterPersistent
	EntityFilterNonPersistent
)

// AlterEntitiesStmt is the bulk form: one statement applied to every entity in
// a module, rather than one statement per entity.
//
// It carries a LIST of AlterEntityStmt rather than its own operation fields.
// The executor resolves the target set and then runs each action through the
// single-entity path unchanged, so every domain rule, refusal and idempotence
// guard that applies to ALTER ENTITY applies here for free and cannot drift.
type AlterEntitiesStmt struct {
	// Module is the module to sweep. Empty means every module in the project.
	Module string
	// Filter narrows the set by persistence; EntityFilterAll means no WHERE.
	Filter EntityPersistenceFilter
	// Actions are applied to each matched entity, in order. Each carries a
	// zero Name; the executor fills it in per entity.
	Actions []*AlterEntityStmt
}

func (s *AlterEntitiesStmt) isStatement() {}

// ============================================================================
// View Entity Statements
// ============================================================================

// ViewAttribute represents an attribute in a view entity.
type ViewAttribute struct {
	Name        string
	Type        DataType
	ExportLevel *string // EXPORT LEVEL clause; nil keeps the stored level (#1498)
}

// OQLQuery represents a simplified OQL query for view entities.
type OQLQuery struct {
	RawQuery string     // The raw OQL query string for pass-through
	Parsed   *OQLParsed // Structured parse (nil if not parsed)
}

// CreateViewEntityStmt represents: CREATE [OR MODIFY|REPLACE] VIEW ENTITY Module.Name (attrs) AS SELECT ...
type CreateViewEntityStmt struct {
	CreateGuard      // `create … if not exists` (ako/mxcli#731)
	Name             QualifiedName
	Attributes       []ViewAttribute
	Query            OQLQuery
	Position         *Position
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
	CreateOrModify   bool
	CreateOrReplace  bool
	// ExportLevel: see CreateEntityStmt.ExportLevel.
	ExportLevel *string
}

func (s *CreateViewEntityStmt) isStatement() {}
