// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// mendixlabs/mxcli#1632, end to end from the .mpk: the Marketplace Calendar lists
// toolbarItems' sub-properties directly under <properties>. The generated Type
// wrote that object property with ObjectType nil, where mxbuild writes a
// WidgetObjectType holding the sub-property types — CE0463 on every Calendar.
func TestGenerateFromMPK_ObjectPropertyWithDirectSubProperties(t *testing.T) {
	const pkgXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/">
  <clientModule name="Cal" version="1.0.0" xmlns="http://www.mendix.com/clientModule/1.0/">
    <widgetFiles><widgetFile path="Cal.xml"/></widgetFiles>
  </clientModule>
</package>`
	const widgetXML = `<?xml version="1.0" encoding="utf-8"?>
<widget id="com.example.Cal" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Cal</name>
  <properties>
    <propertyGroup caption="General">
      <property key="toolbarItems" type="object" isList="true" required="false">
        <caption>Custom top bar views</caption><description/>
        <properties>
          <property key="itemType" type="enumeration" defaultValue="month">
            <caption>Item</caption>
            <category>Appearance</category>
            <description/>
            <enumerationValues>
              <enumerationValue key="day">Day</enumerationValue>
              <enumerationValue key="month">Month</enumerationValue>
            </enumerationValues>
          </property>
          <property key="caption" type="textTemplate" required="false">
            <caption>Caption</caption><description/>
          </property>
        </properties>
      </property>
    </propertyGroup>
  </properties>
</widget>`

	path := filepath.Join(t.TempDir(), "Cal.mpk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{"package.xml": pkgXML, "Cal.xml": widgetXML} {
		w, err := zw.Create(name)
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
	f.Close()

	def, err := mpk.ParseMPK(path)
	if err != nil {
		t.Fatal(err)
	}
	ResetPlaceholderCounter()
	tmpl := GenerateFromMPK(def)

	var toolbar map[string]any
	for _, pt := range tmpl.Type["ObjectType"].(map[string]any)["PropertyTypes"].([]any) {
		if m, ok := pt.(map[string]any); ok && m["PropertyKey"] == "toolbarItems" {
			toolbar = m
		}
	}
	if toolbar == nil {
		t.Fatal("toolbarItems PropertyType missing")
	}
	objType, _ := toolbar["ValueType"].(map[string]any)["ObjectType"].(map[string]any)
	if objType == nil {
		t.Fatal("toolbarItems ValueType.ObjectType is nil; mxbuild writes a WidgetObjectType with its sub-property types")
	}
	var keys, cats []string
	for _, pt := range objType["PropertyTypes"].([]any) {
		if m, ok := pt.(map[string]any); ok {
			keys = append(keys, m["PropertyKey"].(string))
			cats = append(cats, m["Category"].(string))
		}
	}
	if len(keys) != 2 || keys[0] != "itemType" || keys[1] != "caption" {
		t.Fatalf("sub-property types = %v, want [itemType caption]", keys)
	}
	// mxbuild writes Category "Appearance" for the Calendar's itemType, from its
	// <category> element. (Every sub-property of the real Calendar declares one.)
	if cats[0] != "Appearance" {
		t.Errorf("itemType Category = %q, want %q", cats[0], "Appearance")
	}
}
