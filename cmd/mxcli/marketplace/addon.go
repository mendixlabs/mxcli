// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"path"
	"path/filepath"
	"strings"
)

// addOnModulesDir is the folder Studio Pro keeps packaged add-on modules in,
// one modules/<Name>.mxmodule file each, with no unit in the .mpr. The reader
// side is modelsdk/mpr/packaged_modules.go (ako/mxcli#1780).
const addOnModulesDir = "modules"

// isAddOnModulePackage reports whether a package entry is an add-on module
// package: a .mxmodule file directly under modules/.
//
// A module package can carry one beside its own model. AgentEditorCommons
// 2.7.0 ships modules/AgentEditor.mxmodule, the protected add-on whose Studio
// Pro extension defines the agenteditor.agent document type (ako/mxcli#2205).
// Studio Pro installs it as that file whether or not the outer package.xml
// lists it, so this is a project file on its shape alone. Only a direct child
// in canonical spelling qualifies: "modules/../x.mxmodule" or a nested
// modules/a/b.mxmodule is not one, and goes through the ordinary rules.
func isAddOnModulePackage(name string) bool {
	p := packagePath(name)
	if p != path.Clean(p) || path.Dir(p) != addOnModulesDir {
		return false
	}
	base := path.Base(p)
	return strings.EqualFold(path.Ext(base), ".mxmodule") &&
		len(base) > len(".mxmodule") && !strings.HasPrefix(base, ".")
}

// AddOnModulePackages picks the add-on module packages out of the paths
// InstallPackageFiles wrote, so install and update can name them.
func AddOnModulePackages(written []string) []string {
	var out []string
	for _, w := range written {
		if isAddOnModulePackage(filepath.ToSlash(w)) {
			out = append(out, w)
		}
	}
	return out
}
