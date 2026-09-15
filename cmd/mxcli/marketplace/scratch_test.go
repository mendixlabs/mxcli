// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"strings"
	"testing"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/backend"
)

func openFixture(mprPath string) (backend.FullBackend, error) { return modelsdk.Open(mprPath) }

// Version UUIDs read from the live marketplace API, cross-checked against what
// a blank Mendix 11.12.1 project records for the module each one installs. They
// are stable identifiers of a published release, so they do not rot.
const (
	adminV432    = "2059615c-c6f1-4103-aedb-14820c077a1c" // Administration 4.3.2, content 23513
	adminV450    = "d6d35b0e-c44b-4a28-987c-77aefcd027f5" // Administration 4.5.0
	dataWidgets  = "225ac9cf-f84a-4559-bc18-a6a217414c5e" // DataWidgets 3.5.0, content 116540
	notPublished = "00000000-0000-0000-0000-000000000000"
)

// TestModuleForVersionIDs_IdentifiesTheModuleAndVersion is the whole point of
// matching on the GUID: the marketplace listing is called "Administration
// module" and the module it installs is called "Administration", so the name
// cannot be inferred from the listing. The project knows, and says so exactly.
func TestModuleForVersionIDs_IdentifiesTheModuleAndVersion(t *testing.T) {
	name, versionID, err := ModuleForVersionIDs(copyFixture(t), []string{adminV450, adminV432})
	if err != nil {
		t.Fatalf("ModuleForVersionIDs: %v", err)
	}
	if name != "Administration" {
		t.Errorf("module = %q, want Administration", name)
	}
	// The *installed* version must be identified, not merely some version of the
	// content: comparing against the latest release would report the author's
	// changes as the user's.
	if versionID != adminV432 {
		t.Errorf("versionID = %q, want the installed 4.3.2 (%s)", versionID, adminV432)
	}
}

// TestModuleForVersionIDs_IgnoresOtherContent checks the match is scoped to the
// content asked about. A blank project ships eight marketplace modules; only one
// of them is this one.
func TestModuleForVersionIDs_IgnoresOtherContent(t *testing.T) {
	name, _, err := ModuleForVersionIDs(copyFixture(t), []string{dataWidgets})
	if err != nil {
		t.Fatalf("ModuleForVersionIDs: %v", err)
	}
	if name != "DataWidgets" {
		t.Errorf("module = %q, want DataWidgets", name)
	}
}

// TestModuleForVersionIDs_VersionNumbersWouldBeAmbiguous records why this
// matches on the version UUID rather than the version number, because the
// simpler thing looks like it works right up until it silently doesn't.
//
// A blank 11.12.1 project has Atlas_Web_Content at 4.1.0, and Administration's
// content has *also* published a 4.1.0. Matching on the number picks both, and
// picking either one produces a report about the wrong module that is
// indistinguishable from a real one. The GUIDs do not collide.
func TestModuleForVersionIDs_VersionNumbersWouldBeAmbiguous(t *testing.T) {
	mpr := copyFixture(t)

	byNumber := map[string][]string{}
	for _, m := range listFixtureModules(t, mpr) {
		if m.version != "" {
			byNumber[m.version] = append(byNumber[m.version], m.name)
		}
	}
	if len(byNumber["4.1.0"]) == 0 {
		t.Skip("the fixture no longer carries a 4.1.0 module; the collision cannot be shown here")
	}

	// The GUID for Administration 4.3.2 selects exactly one module even though a
	// version *number* from the same content matches a different module.
	name, _, err := ModuleForVersionIDs(mpr, []string{adminV432})
	if err != nil {
		t.Fatalf("ModuleForVersionIDs: %v", err)
	}
	if name != "Administration" {
		t.Errorf("GUID matching picked %q; version-number matching would have been ambiguous with %v",
			name, byNumber["4.1.0"])
	}
}

// TestModuleForVersionIDs_ReportsNoMatch — a project that never installed this
// content must be told so, not handed a guess.
func TestModuleForVersionIDs_ReportsNoMatch(t *testing.T) {
	_, _, err := ModuleForVersionIDs(copyFixture(t), []string{notPublished})
	if err == nil {
		t.Fatal("a project with none of this content's versions must be an error")
	}
	if !strings.Contains(err.Error(), "no module") {
		t.Errorf("the error should say no module matched; got: %v", err)
	}
}

// TestInstalledModule_RefusesAModuleWithNoRecordedVersion — a hand-imported
// module has nothing to compare against, and saying so is more useful than
// comparing it to whatever the latest release happens to be.
func TestInstalledModule_RefusesAModuleWithNoRecordedVersion(t *testing.T) {
	mpr := copyFixture(t)
	var local string
	for _, m := range listFixtureModules(t, mpr) {
		if m.version == "" {
			local = m.name
			break
		}
	}
	if local == "" {
		t.Skip("the fixture has no non-marketplace module")
	}

	_, mendixVersion, err := InstalledModule(mpr, local)
	if err == nil {
		t.Fatalf("module %q records no marketplace version, so this must be refused", local)
	}
	if !strings.Contains(err.Error(), "no marketplace version") {
		t.Errorf("the error should explain there is nothing to compare against; got: %v", err)
	}
	// The Mendix version is still reported, because the caller uses it to build
	// the reference project and the failure is about the module, not the project.
	if mendixVersion == "" {
		t.Error("the project's Mendix version should be reported even on failure")
	}
}

// TestInstalledModule_ReadsTheRecordedVersion is the happy path the command
// depends on for its headline line.
func TestInstalledModule_ReadsTheRecordedVersion(t *testing.T) {
	version, mendixVersion, err := InstalledModule(copyFixture(t), "Administration")
	if err != nil {
		t.Fatalf("InstalledModule: %v", err)
	}
	if version == "" {
		t.Error("Administration is a marketplace module; its version must be reported")
	}
	if mendixVersion == "" {
		t.Error("the project's Mendix version must be reported: the reference is built at it")
	}
}

type fixtureModule struct{ name, version string }

func listFixtureModules(t *testing.T, mprPath string) []fixtureModule {
	t.Helper()
	reader, err := openFixture(mprPath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer reader.Disconnect()

	mods, err := reader.ListModules()
	if err != nil {
		t.Fatalf("list modules: %v", err)
	}
	out := make([]fixtureModule, 0, len(mods))
	for _, m := range mods {
		out = append(out, fixtureModule{m.Name, m.AppStoreVersion})
	}
	return out
}
