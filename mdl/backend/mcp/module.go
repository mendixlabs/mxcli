// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// CreateModule creates a module (and its empty domain model) via the PED
// ped_create_module tool, then registers it in the session so module-dependent
// operations later in the same run can resolve it (the local reader does not yet
// know about it). Unlike most PED writes, ped_create_module flushes to disk
// immediately, so the module also becomes visible to local reads after this.
func (b *Backend) CreateModule(mod *model.Module) error {
	if mod.Name == "" {
		return fmt.Errorf("module name is required")
	}
	res, err := b.client.CallTool("ped_create_module", map[string]any{
		"moduleName": mod.Name,
	})
	if err != nil {
		return err
	}
	// ped_create_module reports success as "Module 'X' created successfully." —
	// NOT the "SUCCESS"-prefix convention the document ops use, so pedOpError
	// would mis-flag it. Check for the success phrasing instead.
	text := pedStripReminder(res.Text)
	if res.IsError || !strings.Contains(strings.ToLower(text), "success") {
		return fmt.Errorf("ped_create_module %s: %s", mod.Name, text)
	}
	if mod.ID == "" {
		mod.ID = model.ID("mcp~module~" + mod.Name)
	}
	b.sessionModules = append(b.sessionModules, mod)
	return nil
}
