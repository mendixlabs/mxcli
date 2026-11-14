// SPDX-License-Identifier: Apache-2.0

package ast

// AlterModuleJarDepStmt represents: ALTER MODULE name <jarDepAction>+
type AlterModuleJarDepStmt struct {
	ModuleName string
	Actions    []JarDepAction
}

func (s *AlterModuleJarDepStmt) isStatement() {}

// ModuleSettingsClause is the General tab of Studio Pro's Module settings
// dialog as MDL states it (ako/mxcli#1921). A nil field is a setting the
// statement does not name, which keeps its stored value.
type ModuleSettingsClause struct {
	Version    *string // "2.3.1"; SemVer, checked by the visitor
	ModuleType *string // "app" | "addon"
}

// IsEmpty reports whether the clause names no setting.
func (c ModuleSettingsClause) IsEmpty() bool {
	return c.Version == nil && c.ModuleType == nil
}

// AlterModuleSetStmt represents: ALTER MODULE name SET ( Key: value, … )
type AlterModuleSetStmt struct {
	ModuleName string
	Settings   ModuleSettingsClause
}

func (s *AlterModuleSetStmt) isStatement() {}

// JarDepAction is the common interface for JAR dependency sub-actions.
type JarDepAction interface {
	isJarDepAction()
}

// AddJarDepAction represents: ADD JAR DEPENDENCY [IF NOT EXISTS] (group = '...', artifact = '...', version = '...' [, included = true])
type AddJarDepAction struct {
	Group    string
	Artifact string
	Version  string
	Included bool // defaults to true
	// IfNotExists leaves a coordinate the module already declares untouched —
	// whatever its stored version — instead of refusing (ako/mxcli#2239).
	IfNotExists bool
}

func (a *AddJarDepAction) isJarDepAction() {}

// SetJarDepVersionAction represents: SET JAR DEPENDENCY 'group:artifact' VERSION '...'
type SetJarDepVersionAction struct {
	Coordinate string // "group:artifact"
	Version    string
}

func (a *SetJarDepVersionAction) isJarDepAction() {}

// SetJarDepIncludedAction represents: SET JAR DEPENDENCY 'group:artifact' INCLUDED true|false
type SetJarDepIncludedAction struct {
	Coordinate string
	Included   bool
}

func (a *SetJarDepIncludedAction) isJarDepAction() {}

// DropJarDepAction represents: DROP JAR DEPENDENCY 'group:artifact'
type DropJarDepAction struct {
	Coordinate string
}

func (a *DropJarDepAction) isJarDepAction() {}

// AddJarDepExclusionAction represents: SET JAR DEPENDENCY 'group:artifact' ADD EXCLUSION 'exc:artifact'
type AddJarDepExclusionAction struct {
	Coordinate string
	Exclusion  string
}

func (a *AddJarDepExclusionAction) isJarDepAction() {}

// DropJarDepExclusionAction represents: SET JAR DEPENDENCY 'group:artifact' DROP EXCLUSION 'exc:artifact'
type DropJarDepExclusionAction struct {
	Coordinate string
	Exclusion  string
}

func (a *DropJarDepExclusionAction) isJarDepAction() {}
