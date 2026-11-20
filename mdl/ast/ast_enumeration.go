// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Enumeration Statements
// ============================================================================

// EnumValue represents a single enumeration value with its caption.
type EnumValue struct {
	Name          string
	Caption       string
	Documentation string // JavaDoc for individual enum value
}

// CreateModuleStmt represents: CREATE MODULE ModuleName
type CreateModuleStmt struct {
	CreateGuard // `create … if not exists` (ako/mxcli#731)
	Name        string
	// CreateOrModify is `create or modify module`: on a module that exists, the
	// statement's documentation is applied rather than the statement being a
	// no-op.
	CreateOrModify bool
	// Documentation is the doc comment, stored as the module's domain model
	// documentation: a Mendix module has none of its own, its
	// DomainModels$DomainModel does (mendixlabs/mxcli#1314).
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
	// Settings is the `( Version: …, ModuleType: … )` list (ako/mxcli#1921).
	Settings ModuleSettingsClause
}

func (s *CreateModuleStmt) isStatement() {}

// DropModuleStmt represents: DROP MODULE ModuleName
type DropModuleStmt struct {
	DropGuard
	Name string
}

func (s *DropModuleStmt) isStatement() {}

// DropFolderStmt represents: DROP FOLDER 'path' IN Module
type DropFolderStmt struct {
	DropGuard
	FolderPath string // Folder path (e.g., "Resources/Images")
	Module     string // Module name
}

func (s *DropFolderStmt) isStatement() {}

// MoveFolderStmt represents: MOVE FOLDER Module.FolderName TO ...
type MoveFolderStmt struct {
	Name         QualifiedName // Source: Module.FolderName (Name may be "Parent/Child" for nested)
	TargetFolder string        // Target folder path (empty = module root)
	TargetModule string        // Target module name (empty = same module)
}

func (s *MoveFolderStmt) isStatement() {}

// CreateEnumerationStmt represents: CREATE ENUMERATION Module.Name (values) COMMENT '...'
type CreateEnumerationStmt struct {
	CreateGuard   // `create … if not exists` (ako/mxcli#731)
	Name          QualifiedName
	Values        []EnumValue
	Documentation string
	// DocumentationSet records whether the statement carried a `/** … */`
	// comment at all, as opposed to carrying an empty one. A rewrite that did
	// not mention documentation preserves the stored value; an explicitly empty
	// comment clears it (mendixlabs/mxcli#1018).
	DocumentationSet bool
	Folder           string // Module folder to place the enumeration in (Bug 12b)
	CreateOrModify   bool   // True if CREATE OR MODIFY was used
	// ExportLevel is "API" or "Hidden" from an EXPORT LEVEL clause; nil keeps
	// the stored level (Hidden on a new element). mendixlabs/mxcli#1498.
	ExportLevel *string
}

func (s *CreateEnumerationStmt) isStatement() {}

// AlterEnumerationStmt represents: ALTER ENUMERATION Module.Name ADD/DROP/RENAME/MODIFY VALUE ...
type AlterEnumerationStmt struct {
	Name      QualifiedName
	Operation AlterEnumOp
	ValueName string
	NewName   string // For RENAME
	Caption   string // For ADD and MODIFY CAPTION
	// Documentation is the text of SET DOCUMENTATION.
	Documentation string
	// ExportLevel is the "API" or "Hidden" of SET EXPORT LEVEL.
	ExportLevel string

	// Idempotency guards, so a script that adds an enumeration value is
	// re-runnable. Without them the second run errors and exec STOPS THERE,
	// leaving every later statement unapplied. Same pair as ALTER ENTITY's
	// ADD ATTRIBUTE / DROP INDEX. (ako/mxcli-rest FINDINGS #60)
	IfNotExists bool // ADD VALUE IF NOT EXISTS
	IfExists    bool // DROP VALUE IF EXISTS
}

func (s *AlterEnumerationStmt) isStatement() {}

// AlterEnumOp represents the type of enumeration alteration.
type AlterEnumOp int

const (
	AlterEnumAdd AlterEnumOp = iota
	AlterEnumDrop
	AlterEnumRename
	AlterEnumModifyCaption // MODIFY VALUE X CAPTION '...' — change an existing value's caption
	// AlterEnumSetDocumentation is SET DOCUMENTATION '…' (SET COMMENT is its
	// deprecated alias, MDL-DEPR135).
	AlterEnumSetDocumentation
	AlterEnumSetExportLevel // SET EXPORT LEVEL API | HIDDEN
)

// DropEnumerationStmt represents: DROP ENUMERATION Module.Name
type DropEnumerationStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropEnumerationStmt) isStatement() {}

// ============================================================================
// Constant Statements
// ============================================================================

// CreateConstantStmt represents: CREATE CONSTANT Module.Name [FOLDER '…'] ( Type: type, DefaultValue: value [, ExposedToClient: true] )
type CreateConstantStmt struct {
	CreateGuard      // `create … if not exists` (ako/mxcli#731)
	Name             QualifiedName
	DataType         DataType
	DefaultValue     any // The default value (can be string, number, boolean, etc.)
	Documentation    string
	DocumentationSet bool   // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Folder           string // Folder path within module (e.g., "Resources/Constants")
	ExposedToClient  bool
	CreateOrModify   bool // True if CREATE OR MODIFY was used
}

func (s *CreateConstantStmt) isStatement() {}

// DropConstantStmt represents: DROP CONSTANT Module.Name
type DropConstantStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropConstantStmt) isStatement() {}
