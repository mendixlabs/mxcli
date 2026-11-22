// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// A pluggable widget package declares the platform it renders on
// (supportedPlatform: Web, Native or All; absent means Web). check never
// compared it with the page's: measured on PedApp (11.13.0) with Native Mobile
// Resources 12.6.0, `combobox` and `datagrid` on a NativePhone page and
// `nativeslider` / `nativegallery` on an Atlas_Default page all passed check
// and exec. A web widget on a native page is a widget the native client does
// not render, and the reverse the same for the browser.
const nativeSliderXML = `<?xml version="1.0" encoding="utf-8" ?>
<widget id="com.mendix.widget.native.slider.Slider" pluginWidget="true" supportedPlatform="Native" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Slider</name>
  <properties>
    <propertyGroup caption="General">
      <propertyGroup caption="General">
        <property key="valueAttribute" type="attribute"><caption>Value</caption><description/>
          <attributeTypes><attributeType name="Decimal"/></attributeTypes></property>
      </propertyGroup>
    </propertyGroup>
  </properties>
</widget>`

// writeNativeWidgetProject writes a project directory holding the native
// slider's package and returns the .mpr path.
func writeNativeWidgetProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "widgets", "com.mendix.widget.native.Slider.mpk"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for entry, body := range map[string]string{
		"package.xml": `<package><clientModule name="Slider" version="1.0.0"><widgetFiles><widgetFile path="Slider.xml"/></widgetFiles></clientModule></package>`,
		"Slider.xml":  nativeSliderXML,
	} {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	mpk.ClearCache()
	t.Cleanup(mpk.ClearCache)
	return filepath.Join(dir, "App.mpr")
}

func platformRegistry() *WidgetRegistry {
	reg := LoadWidgetRegistry("")
	def := &WidgetDefinition{WidgetID: "com.mendix.widget.native.slider.Slider", MDLName: "nativeslider", WidgetKind: "pluggable",
		PropertyMappings: []PropertyMapping{{PropertyKey: "valueAttribute", Source: "Attribute", Operation: "attribute"}}}
	reg.byMDLName["NATIVESLIDER"] = def
	reg.byWidgetID[def.WidgetID] = def
	return reg
}

func platformViolations(t *testing.T, mprPath, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var got []string
	for _, v := range validateWidgetPlatforms(prog, platformRegistry(), mprPath, nil) {
		if v.RuleID != widgetPlatformRule {
			t.Errorf("rule %s, want %s", v.RuleID, widgetPlatformRule)
		}
		got = append(got, v.Message)
	}
	sort.Strings(got)
	return got
}

func TestWidgetPlatform_WebWidgetOnNativePage(t *testing.T) {
	mpr := writeNativeWidgetProject(t)
	got := platformViolations(t, mpr, `create layout M.NL (LayoutType: Default) { placeholder Main };
create page M.P (Title: 'N', Layout: M.NL) {
  container c {
    combobox cb1
  }
  image img1 (Image: 'M.Images.logo')
  nativeslider s1
};`)
	joined := strings.Join(got, "\n")
	if len(got) != 2 || !strings.Contains(joined, "cb1") || !strings.Contains(joined, "img1") {
		t.Fatalf("want the web combobox (nested in a container) and the web image reported, got:\n%s", joined)
	}
	if strings.Contains(joined, "s1") {
		t.Errorf("the native slider belongs on a native page:\n%s", joined)
	}
}

func TestWidgetPlatform_NativeWidgetOnWebPage(t *testing.T) {
	mpr := writeNativeWidgetProject(t)
	got := platformViolations(t, mpr, `create layout M.WL (LayoutType: Responsive) { placeholder Main };
create page M.P (Title: 'W', Layout: M.WL) {
  nativeslider s1
  combobox cb1
};`)
	joined := strings.Join(got, "\n")
	if len(got) != 1 || !strings.Contains(joined, "s1") {
		t.Fatalf("want only the native slider reported, got:\n%s", joined)
	}
}

// A layout whose platform nothing can tell — neither the script nor a project —
// is not judged: guessing web would report every native widget on a native page.
func TestWidgetPlatform_UnknownLayoutIsNotJudged(t *testing.T) {
	mpr := writeNativeWidgetProject(t)
	got := platformViolations(t, mpr, `create page M.P (Title: 'X', Layout: Atlas_Core.NativePhone_Default) {
  nativeslider s1
  combobox cb1
};`)
	if len(got) != 0 {
		t.Fatalf("a page on an unresolved layout must not be judged, got:\n%s", strings.Join(got, "\n"))
	}
}
