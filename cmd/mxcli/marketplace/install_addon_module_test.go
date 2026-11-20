// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// agentEditorCommonsPackageXML has the shape of AgentEditorCommons 2.7.0's
// outer package.xml for this purpose: a <files> list that names the module's
// own project files and not the add-on module package it also carries.
const agentEditorCommonsPackageXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/">
  <modelerProject xmlns="http://www.mendix.com/modelerProject/1.0/">
    <module name="AgentEditorCommons" />
    <projectFile path="project.mpr" />
    <files>
      <file path="themesource/agenteditorcommons/settings.json" />
    </files>
  </modelerProject>
</package>`

// addOnModuleBytes builds a small stand-in for a protected add-on module
// package (modules/<Name>.mxmodule is itself a zip). The marker makes two
// versions distinguishable byte for byte.
func addOnModuleBytes(t *testing.T, name, version string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for entry, body := range map[string]string{
		"package.xml": `<package><modelerProject><module name="` + name + `"/>` +
			`<extensions><extension name="agent-editor"/></extensions></modelerProject></package>`,
		"manifest.json":                        `{"package":{"name":"` + name + `","version":"` + version + `","module-type":"add-on"}}`,
		"extensions/agent-editor/main.js":      "// " + version,
		"extensions/agent-editor/agentEdit.js": "// edit " + version,
	} {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestInstallPackageFiles_WritesAddOnModulePackage is ako/mxcli#2205: Agent
// Editor 2.7.0's AgentEditorCommons package also carries
// modules/AgentEditor.mxmodule, the protected add-on module whose Studio Pro
// extension defines the agenteditor.agent document type. Studio Pro keeps it
// as that file in the app's modules/ folder — there is no unit for it in the
// .mpr — so an install that writes only the model and the files package.xml
// lists leaves Studio Pro reporting "unknown type 'agenteditor.agent'", while
// mx check and mxbuild stay green.
func TestInstallPackageFiles_WritesAddOnModulePackage(t *testing.T) {
	addon := addOnModuleBytes(t, "AgentEditor", "2.7.0")
	mpk := buildMPK(t, map[string]string{
		"package.xml":   agentEditorCommonsPackageXML,
		"project.mpr":   "model",
		"manifest.json": `{"name":"AgentEditorCommons"}`,
		"themesource/agenteditorcommons/settings.json": "{}",
		"modules/AgentEditor.mxmodule":                 addon,
	})
	proj := t.TempDir()

	written, _, err := InstallPackageFiles(mpk, proj)
	if err != nil {
		t.Fatalf("InstallPackageFiles: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(proj, "modules", "AgentEditor.mxmodule"))
	if err != nil {
		t.Fatalf("the add-on module package was not installed: %v", err)
	}
	if !bytes.Equal(got, []byte(addon)) {
		t.Errorf("modules/AgentEditor.mxmodule is not byte-identical to the package's copy")
	}
	if got := AddOnModulePackages(written); len(got) != 1 || got[0] != filepath.Join("modules", "AgentEditor.mxmodule") {
		t.Errorf("AddOnModulePackages(written) = %v, want [modules/AgentEditor.mxmodule]; written = %v", got, written)
	}

	// Control: package metadata is still left out, so the add-on is not
	// arriving because the declared-files filter stopped applying.
	if _, err := os.Stat(filepath.Join(proj, "manifest.json")); err == nil {
		t.Errorf("manifest.json written into the app root")
	}
	if _, err := os.Stat(filepath.Join(proj, "themesource", "agenteditorcommons", "settings.json")); err != nil {
		t.Errorf("declared file not installed: %v", err)
	}
}

// An update installs the new package's files over the old ones, so the
// add-on's version follows the module's: the copy in modules/ is replaced.
func TestInstallPackageFiles_UpdateReplacesAddOnModulePackage(t *testing.T) {
	proj := t.TempDir()
	older := addOnModuleBytes(t, "AgentEditor", "2.6.0")
	newer := addOnModuleBytes(t, "AgentEditor", "2.7.0")
	if older == newer {
		t.Fatal("fixture: the two versions must differ")
	}
	for _, body := range []string{older, newer} {
		mpk := buildMPK(t, map[string]string{
			"package.xml":                  agentEditorCommonsPackageXML,
			"project.mpr":                  "model",
			"modules/AgentEditor.mxmodule": body,
		})
		if _, _, err := InstallPackageFiles(mpk, proj); err != nil {
			t.Fatalf("InstallPackageFiles: %v", err)
		}
	}
	got, err := os.ReadFile(filepath.Join(proj, "modules", "AgentEditor.mxmodule"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(newer)) {
		t.Errorf("update left the old add-on module package in place")
	}
}

// Only a direct child of modules/ is an add-on module package. A traversal
// spelled to look like one must not reach outside the project, and a nested
// .mxmodule the package does not declare is not installed on the strength of
// its extension alone.
func TestInstallPackageFiles_AddOnModulePathSafety(t *testing.T) {
	for _, name := range []string{
		"modules/../../evil.mxmodule",
		"modules/../../../evil.mxmodule",
		`modules\..\..\evil.mxmodule`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			proj := filepath.Join(root, "project")
			if err := os.MkdirAll(proj, 0o755); err != nil {
				t.Fatal(err)
			}
			mpk := buildMPK(t, map[string]string{
				"package.xml": agentEditorCommonsPackageXML,
				name:          "PWNED",
			})
			_, _, _ = InstallPackageFiles(mpk, proj)

			var escaped []string
			_ = filepath.Walk(root, func(p string, fi os.FileInfo, e error) error {
				if e != nil || fi.IsDir() {
					return nil
				}
				if rel, _ := filepath.Rel(proj, p); len(rel) >= 2 && rel[:2] == ".." {
					escaped = append(escaped, p)
				}
				return nil
			})
			if len(escaped) > 0 {
				t.Errorf("ESCAPED: %v", escaped)
			}
		})
	}

	t.Run("nested .mxmodule is not an add-on", func(t *testing.T) {
		proj := t.TempDir()
		mpk := buildMPK(t, map[string]string{
			"package.xml":                  agentEditorCommonsPackageXML,
			"modules/sub/Nested.mxmodule":  "x",
			"modules/AgentEditor.mxmodule": "addon",
		})
		if _, _, err := InstallPackageFiles(mpk, proj); err != nil {
			t.Fatalf("InstallPackageFiles: %v", err)
		}
		if _, err := os.Stat(filepath.Join(proj, "modules", "sub", "Nested.mxmodule")); err == nil {
			t.Errorf("an undeclared nested .mxmodule was installed")
		}
		// Control: the direct child is.
		if _, err := os.Stat(filepath.Join(proj, "modules", "AgentEditor.mxmodule")); err != nil {
			t.Errorf("control: add-on not installed: %v", err)
		}
	})
}
