// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeWidgetMPK writes a one-widget .mpk into projectDir/widgets.
func writeWidgetMPK(t *testing.T, projectDir, file, widgetID string, pluggable bool) string {
	t.Helper()
	plugin := ""
	if pluggable {
		plugin = ` pluginWidget="true"`
	}
	entries := []struct{ name, body string }{
		{"package.xml", `<package><clientModule name="W" version="1.0.0"><widgetFiles><widgetFile path="W.xml"/></widgetFiles></clientModule></package>`},
		{"W.xml", `<widget id="` + widgetID + `"` + plugin + `><name>W</name><properties><propertyGroup caption="General"><property key="caption" type="string" required="false"><caption>Caption</caption><description/></property></propertyGroup></properties></widget>`},
	}
	dir := filepath.Join(projectDir, "widgets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, file)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// defsByWidgetID reads every .def.json in the project and maps widget ID to
// its mdlName.
func defsByWidgetID(t *testing.T, projectDir string) map[string]string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(projectDir, ".mxcli", "widgets", "*.def.json"))
	out := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var def WidgetDefinition
		if err := json.Unmarshal(data, &def); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out[def.WidgetID] = strings.ToUpper(def.MDLName)
	}
	return out
}

// #1848: KeyboardShortcut.mpk (a legacy custom widget) and
// itvisors.KeyboardShortcut.mpk (its pluggable successor) both derive
// KEYBOARDSHORTCUT. They wrote one file in turn, so one widget had no
// definition and every refresh reported "2 refreshed" and regenerated the
// widget docs.
func TestRefreshWidgetDefinitions_CollidingNamesKeepBoth(t *testing.T) {
	withWidgetStateFingerprint(t, "")
	dir := t.TempDir()
	const legacy = "KeyboardShortcut.widget.KeyboardShortcut"
	const pluggable = "itvisors.keyboardshortcut.KeyboardShortcut"
	writeWidgetMPK(t, dir, "KeyboardShortcut.mpk", legacy, false)
	writeWidgetMPK(t, dir, "itvisors.KeyboardShortcut.mpk", pluggable, true)
	project := filepath.Join(dir, "App.mpr")

	first, err := RefreshWidgetDefinitions(project, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Extracted != 2 {
		t.Errorf("first refresh: %+v, want 2 extracted", first)
	}

	defs := defsByWidgetID(t, dir)
	if defs[pluggable] != "KEYBOARDSHORTCUT" {
		t.Errorf("pluggable widget should keep the bare keyword, got %q (defs %v)", defs[pluggable], defs)
	}
	if name := defs[legacy]; name == "" || name == "KEYBOARDSHORTCUT" {
		t.Errorf("legacy widget needs a definition under its own name, got %q (defs %v)", name, defs)
	}

	second, err := RefreshWidgetDefinitions(project, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Extracted != 0 || second.Refreshed != 0 {
		t.Errorf("second refresh rewrote definitions: %+v", second)
	}
}

// #1848: a refresh after a model edit re-read every .mpk although widgets/
// had not changed; on Windows each read is also a Defender scan.
func TestRefreshWidgetDefinitions_SkipsUnchangedWidgets(t *testing.T) {
	withWidgetStateFingerprint(t, "build-1")
	dir := t.TempDir()
	mpkPath := writeWidgetMPK(t, dir, "Badge.mpk", "com.example.badge.Badge", true)
	project := filepath.Join(dir, "App.mpr")

	if _, err := RefreshWidgetDefinitions(project, false, nil); err != nil {
		t.Fatal(err)
	}
	stats, err := RefreshWidgetDefinitions(project, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Unchanged {
		t.Fatalf("second refresh of an unchanged widgets/ folder did the work again: %+v", stats)
	}

	// Controls: each change below must make the refresh look again.
	t.Run("changed mpk", func(t *testing.T) {
		later := time.Now().Add(time.Minute)
		if err := os.Chtimes(mpkPath, later, later); err != nil {
			t.Fatal(err)
		}
		if stats, _ := RefreshWidgetDefinitions(project, false, nil); stats.Unchanged {
			t.Error("a touched .mpk was not looked at")
		}
	})
	t.Run("deleted def", func(t *testing.T) {
		if err := os.Remove(filepath.Join(dir, ".mxcli", "widgets", "badge.def.json")); err != nil {
			t.Fatal(err)
		}
		stats, _ := RefreshWidgetDefinitions(project, false, nil)
		if stats.Unchanged || stats.Extracted != 1 {
			t.Errorf("a deleted definition was not regenerated: %+v", stats)
		}
	})
	t.Run("another mxcli build", func(t *testing.T) {
		withWidgetStateFingerprint(t, "build-2")
		if stats, _ := RefreshWidgetDefinitions(project, false, nil); stats.Unchanged {
			t.Error("another mxcli build reused the previous build's definitions")
		}
	})
	t.Run("force", func(t *testing.T) {
		if stats, _ := RefreshWidgetDefinitions(project, true, nil); stats.Unchanged {
			t.Error("force skipped the refresh")
		}
	})
}

func withWidgetStateFingerprint(t *testing.T, fp string) {
	t.Helper()
	prev := widgetStateFingerprint
	widgetStateFingerprint = func() string { return fp }
	t.Cleanup(func() { widgetStateFingerprint = prev })
}

// ako/mxcli#2155: a package copied into widgets/ AFTER the project's
// definitions were extracted was never extracted. RefreshStaleWidgetDefinitions
// — the one refresh exec's page builder and check's registry both go through —
// only looked at the generatorVersion stamps of the definitions already there,
// so a new .mpk beside current definitions passed check (its package is
// installed) and exec refused it with "installed but not extracted yet (run
// 'mxcli widget init')".
func TestRefreshStaleWidgetDefinitions_ExtractsPackageAddedLater(t *testing.T) {
	withWidgetStateFingerprint(t, "build-1")
	dir := t.TempDir()
	project := filepath.Join(dir, "App.mpr")
	const first = "com.example.badge.Badge"
	const added = "com.example.chatrichtext.ChatRichText"

	writeWidgetMPK(t, dir, "Badge.mpk", first, true)
	if _, err := RefreshWidgetDefinitions(project, false, nil); err != nil { // `widget init`
		t.Fatal(err)
	}
	// Nothing changed since: no refresh, and the existing widget resolves.
	if refreshed, err := RefreshStaleWidgetDefinitions(project); err != nil || refreshed {
		t.Fatalf("unchanged widgets/ refreshed=%v err=%v, want no work", refreshed, err)
	}

	writeWidgetMPK(t, dir, "ChatRichText.mpk", added, true)

	registry, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := LoadProjectWidgetDefinitions(registry, project); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.GetByWidgetID(first); !ok {
		t.Errorf("control: %s (extracted by widget init) does not resolve", first)
	}
	if _, ok := registry.GetByWidgetID(added); !ok {
		t.Errorf("%s was installed after widget init and still has no definition; defs: %v",
			added, defsByWidgetID(t, dir))
	}
	// And once extracted, the next load does no work again.
	if refreshed, err := RefreshStaleWidgetDefinitions(project); err != nil || refreshed {
		t.Errorf("second load after the extraction refreshed=%v err=%v, want no work", refreshed, err)
	}
}
