// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Java Action Statements
// ============================================================================

// JavaActionParam represents a parameter in a Java action definition.
type JavaActionParam struct {
	Name       string   // Parameter name
	Type       DataType // Parameter type
	IsRequired bool     // NOT NULL constraint
}

// CreateJavaActionStmt represents:
//
//	CREATE JAVA ACTION Module.Name(
//	  EntityType: ENTITY <pEntity> NOT NULL,
//	  Source: pEntity NOT NULL
//	) RETURNS type
//	EXPOSED AS 'caption' IN 'category'
//	AS $$ ... $$;
type CreateJavaActionStmt struct {
	Folder           string            // Folder path within module (empty = leave placement alone)
	Name             QualifiedName     // Qualified name (Module.ActionName)
	Parameters       []JavaActionParam // Input parameters
	ReturnType       DataType          // Return type (can be nil for void)
	JavaCode         string            // The executeAction() body
	ExtraCode        string            // Optional extra code section
	Imports          []string          // Optional additional imports
	Documentation    string            // Optional documentation comment
	DocumentationSet bool              // see mendixlabs/mxcli#1018: absent preserves, empty clears
	TypeParameters   []string          // Type parameter names (e.g., ["pEntity"])
	ExposedCaption   string            // EXPOSED AS 'caption'
	ExposedCategory  string            // IN 'category'
	// NotExposed is NOT EXPOSED: remove the toolbox entry. Distinct from an
	// absent clause, which preserves whatever is stored.
	NotExposed bool
	// ExposedBitmaps are the ICON/IMAGE clauses. An omitted one is preserved.
	ExposedBitmaps []ExposeBitmap
	CreateOrModify bool // true for CREATE OR MODIFY / CREATE OR REPLACE
}

func (s *CreateJavaActionStmt) isStatement() {}

// DropJavaActionStmt represents: DROP JAVA ACTION Module.Name
type DropJavaActionStmt struct {
	Name QualifiedName
	DropIfExists
}

func (s *DropJavaActionStmt) isStatement() {}

// CreateJavaScriptActionStmt represents:
//
//	CREATE JAVASCRIPT ACTION Module.Name(
//	  Param: type NOT NULL
//	) RETURNS type
//	EXPOSED AS 'caption' IN 'category'
//	PLATFORM Web
//	AS $$ ... $$;
//
// It mirrors CreateJavaActionStmt with an added Platform (Web/Native/Hybrid/All,
// default Web). The inline source is JavaScript rather than Java.
type CreateJavaScriptActionStmt struct {
	Folder           string            // Folder path within module (empty = leave placement alone)
	Name             QualifiedName     // Qualified name (Module.ActionName)
	Parameters       []JavaActionParam // Input parameters
	ReturnType       DataType          // Return type (can be nil for void)
	JavaScriptCode   string            // The exported function body (user code)
	Documentation    string            // Optional documentation comment
	DocumentationSet bool              // see mendixlabs/mxcli#1018: absent preserves, empty clears
	TypeParameters   []string          // Type parameter names (e.g., ["pEntity"])
	ExposedCaption   string            // EXPOSED AS 'caption'
	ExposedCategory  string            // IN 'category'
	// NotExposed is NOT EXPOSED: remove the toolbox entry. Distinct from an
	// absent clause, which preserves whatever is stored.
	NotExposed bool
	// ExposedBitmaps are the ICON/IMAGE clauses. An omitted one is preserved.
	ExposedBitmaps []ExposeBitmap
	Platform       string // PLATFORM Web|Native|Hybrid|All (default Web)
	CreateOrModify bool   // true for CREATE OR MODIFY / CREATE OR REPLACE
}

func (s *CreateJavaScriptActionStmt) isStatement() {}

// DropJavaScriptActionStmt represents: DROP JAVASCRIPT ACTION Module.Name
type DropJavaScriptActionStmt struct {
	Name QualifiedName
}

func (s *DropJavaScriptActionStmt) isStatement() {}
