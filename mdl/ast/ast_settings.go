// SPDX-License-Identifier: Apache-2.0

package ast

// AlterSettingsStmt represents ALTER SETTINGS commands.
type AlterSettingsStmt struct {
	Section    string         // "MODEL", "CONFIGURATION", "CONSTANT", "LANGUAGE", "WORKFLOWS"
	ConfigName string         // For CONFIGURATION section: the configuration name (e.g., "Default")
	Properties map[string]any // Key-value pairs to set
	// For CONSTANT section:
	ConstantId   string // Qualified constant name
	Value        string // Constant value
	DropConstant bool   // If true, remove the constant override instead of setting it
}

func (s *AlterSettingsStmt) isStatement() {}

// CreateConfigurationStmt represents CREATE CONFIGURATION 'name' [properties...].
type CreateConfigurationStmt struct {
	Name       string
	Properties map[string]any
}

func (s *CreateConfigurationStmt) isStatement() {}

// DropConfigurationStmt represents DROP CONFIGURATION 'name'.
type DropConfigurationStmt struct {
	Name string
}

func (s *DropConfigurationStmt) isStatement() {}
