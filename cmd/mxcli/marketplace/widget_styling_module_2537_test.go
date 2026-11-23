// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2537 (mendixlabs/mxcli#1928): installing Excel Importer 11.2.2
// over Data Widgets 3.5.0 moved Datagrid and three of its filters to 3.8.1 —
// the module bundles them (measured: Datagrid, DatagridDropdownFilter,
// DatagridNumberFilter, DatagridTextFilter, all 3.8.1) — while the Data
// Widgets module and its themesource/datawidgets styling stayed at 3.5.0.
// Datagrid 3.8.1 needs the styling of its own release, so every Data grid 2
// collapsed into one column, with no error. A bundled copy now never moves a
// widget past the module that styles it; the skip is reported.
func withStylingModuleVersion(t *testing.T, versions map[string]string) {
	t.Helper()
	prev := stylingModuleVersion
	stylingModuleVersion = func(projectDir, module string) string { return versions[module] }
	t.Cleanup(func() { stylingModuleVersion = prev })
}

func projectWithDatagrid(t *testing.T, version string) (string, string) {
	t.Helper()
	proj := t.TempDir()
	widget := filepath.Join(proj, "widgets", "com.mendix.widget.web.Datagrid.mpk")
	if err := os.MkdirAll(filepath.Dir(widget), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(widget, widgetMpk(t, version), 0o644); err != nil {
		t.Fatal(err)
	}
	return proj, widget
}

const excelImporterPackageXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/"><modelerProject><module name="ExcelImporter"/></modelerProject></package>`

const dataWidgetsPackageXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/"><modelerProject><module name="DataWidgets"/></modelerProject></package>`

func TestBundledWidgetNotMovedPastItsStylingModule_2537(t *testing.T) {
	withStylingModuleVersion(t, map[string]string{"DataWidgets": "3.5.0"})
	proj, widget := projectWithDatagrid(t, "3.5.0")
	pkg := modulePackage(t, map[string][]byte{
		"package.xml": []byte(excelImporterPackageXML),
		"widgets/com.mendix.widget.web.Datagrid.mpk": widgetMpk(t, "3.8.1"),
	})
	_, skipped, err := InstallPackageFiles(pkg, proj)
	if err != nil {
		t.Fatal(err)
	}
	if got := widgetVersionOnDisk(widget); got != "3.5.0" {
		t.Errorf("Datagrid on disk %s, want 3.5.0: it moved past Data Widgets 3.5.0's styling", got)
	}
	if len(skipped) != 1 || skipped[0].Kept != "3.5.0" || skipped[0].Offered != "3.8.1" ||
		!strings.Contains(skipped[0].Reason, "DataWidgets") {
		t.Errorf("skip not reported with its reason: %+v", skipped)
	}
}

// Controls: installing Data Widgets itself moves its widgets; a bundled copy
// up to the styling module's version is installed; with no Data Widgets
// module in the project there is no styling to keep in step with.
func TestBundledWidgetStylingControls_2537(t *testing.T) {
	for _, tc := range []struct {
		name, pkgXML, module, ships, want string
	}{
		{"the styling module's own update", dataWidgetsPackageXML, "3.5.0", "3.8.1", "3.8.1"},
		{"up to the styling module's version", excelImporterPackageXML, "3.8.1", "3.8.1", "3.8.1"},
		{"no styling module in the project", excelImporterPackageXML, "", "3.8.1", "3.8.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withStylingModuleVersion(t, map[string]string{"DataWidgets": tc.module})
			proj, widget := projectWithDatagrid(t, "3.5.0")
			pkg := modulePackage(t, map[string][]byte{
				"package.xml": []byte(tc.pkgXML),
				"widgets/com.mendix.widget.web.Datagrid.mpk": widgetMpk(t, tc.ships),
			})
			if _, skipped, err := InstallPackageFiles(pkg, proj); err != nil {
				t.Fatal(err)
			} else if len(skipped) != 0 {
				t.Errorf("skipped %+v", skipped)
			}
			if got := widgetVersionOnDisk(widget); got != tc.want {
				t.Errorf("on disk %s, want %s", got, tc.want)
			}
		})
	}
}
