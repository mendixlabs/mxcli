// SPDX-License-Identifier: Apache-2.0

package ast

// CreateJsonStructureStmt represents:
//
//	CREATE [OR REPLACE] JSON STRUCTURE Module.Name [COMMENT 'doc'] SNIPPET '...json...' [CUSTOM NAME MAP (...)];
type CreateJsonStructureStmt struct {
	Name            QualifiedName
	JsonSnippet     string            // Raw JSON snippet
	Documentation   string            // Optional documentation comment
	CreateOrReplace bool              // true for CREATE OR REPLACE
	CustomNameMap   map[string]string // Optional: JSON key → custom ExposedName
}

func (s *CreateJsonStructureStmt) isStatement() {}

// DropJsonStructureStmt represents: DROP JSON STRUCTURE Module.Name
type DropJsonStructureStmt struct {
	Name QualifiedName
}

func (s *DropJsonStructureStmt) isStatement() {}
