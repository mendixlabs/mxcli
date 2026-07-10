// SPDX-License-Identifier: Apache-2.0

package mpr

import "github.com/JordtenBulte-OLC/mxcli/modelsdk/meta"

// buildSystemModuleInfo returns a ModuleInfo for the virtual System module.
func buildSystemModuleInfo() *ModuleInfo {
	return &ModuleInfo{
		ID:   meta.SystemModuleID,
		Name: "System",
	}
}
