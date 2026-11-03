// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// The General tab of Studio Pro's Module settings dialog (ako/mxcli#1921),
// measured on 11.16:
//
//	Module type     ExportLevel: Source = App module, Protected = Add-on module.
//	                ProtectedModuleType is AddOn on every module, app modules
//	                included, so it alone does not say "add-on".
//	Module version  Version, a SemVer string.
//	Location        Projects$ModuleImpl.FromAppStore — read-only here.
//
// A new module is App, 1.0.0: those are the defaults describe leaves out.
const (
	defaultModuleVersion = "1.0.0"
	moduleTypeApp        = "app"
	moduleTypeAddOn      = "addon"
)

// moduleTypeOf is the MDL ModuleType of stored settings, or "" for one MDL
// cannot name (a solution module).
func moduleTypeOf(ms *types.ModuleSettings) string {
	if ms == nil || ms.ExportLevel != "Protected" {
		return moduleTypeApp
	}
	if ms.ProtectedModuleType == "" || ms.ProtectedModuleType == "AddOn" {
		return moduleTypeAddOn
	}
	return ""
}

// applyModuleSettings writes what the clause states onto the stored settings.
// It reports whether anything changed.
func applyModuleSettings(ms *types.ModuleSettings, c ast.ModuleSettingsClause) bool {
	changed := false
	if c.Version != nil && ms.Version != *c.Version {
		ms.Version = *c.Version
		changed = true
	}
	if c.ModuleType != nil && moduleTypeOf(ms) != *c.ModuleType {
		switch *c.ModuleType {
		case moduleTypeAddOn:
			ms.ExportLevel, ms.ProtectedModuleType = "Protected", "AddOn"
		case moduleTypeApp:
			ms.ExportLevel = "Source"
		}
		changed = true
	}
	return changed
}

// setModuleSettings applies a settings clause to an existing module.
func setModuleSettings(ctx *ExecContext, module *model.Module, c ast.ModuleSettingsClause) error {
	if c.IsEmpty() {
		return nil
	}
	ms, err := ctx.Backend.GetModuleSettings(module.ID)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("read settings of module %s", module.Name), err)
	}
	if !applyModuleSettings(ms, c) {
		fmt.Fprintf(ctx.Output, "Module settings of %s unchanged\n", module.Name)
		return nil
	}
	if err := ctx.Backend.UpdateModuleSettings(ms); err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("update settings of module %s", module.Name), err)
	}
	fmt.Fprintf(ctx.Output, "Updated module settings: %s\n", module.Name)
	return nil
}

// execAlterModuleSet handles ALTER MODULE name SET ( Key: value, … ).
func execAlterModuleSet(ctx *ExecContext, s *ast.AlterModuleSetStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	module, err := ctx.Backend.GetModuleByName(s.ModuleName)
	if err != nil {
		return mdlerrors.NewBackend("find module "+s.ModuleName, err)
	}
	if module == nil {
		return mdlerrors.NewNotFound("module", s.ModuleName)
	}
	return setModuleSettings(ctx, module, s.Settings)
}

// moduleSettingsMDL is the `( … )` list describe prints after the module name:
// only the settings that differ from a new module's, so an untouched module
// describes as it always did. "" when there are none.
func moduleSettingsMDL(ctx *ExecContext, ms *types.ModuleSettings) string {
	if ms == nil {
		return ""
	}
	var props []string
	if ms.Version != "" && ms.Version != defaultModuleVersion {
		props = append(props, fmt.Sprintf("Version: %s", mdlQuote(ctx, ms.Version)))
	}
	if moduleTypeOf(ms) == moduleTypeAddOn {
		props = append(props, "ModuleType: "+moduleTypeAddOn)
	}
	if len(props) == 0 {
		return ""
	}
	return " (\n  " + strings.Join(props, ",\n  ") + ",\n)"
}
