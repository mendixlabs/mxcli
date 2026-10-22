// SPDX-License-Identifier: Apache-2.0

package mpk

import "testing"

// mendixlabs/mxcli#1632: an object property may list its sub-properties directly
// under <properties>, with no <propertyGroup> around them, and give each one its
// category as a <category> element. The Marketplace Calendar's toolbarItems does.
// Only grouped sub-properties were read, so the object came back with no Children,
// its generated ObjectType was nil, and every Calendar failed `mx check` with
// CE0463 until `update-widgets` rewrote the Type.
func TestParse_ObjectPropertyWithDirectSubProperties(t *testing.T) {
	const widgetXML = `<?xml version="1.0" encoding="utf-8"?>
<widget id="com.example.Cal" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Cal</name>
  <properties>
    <propertyGroup caption="General">
      <propertyGroup caption="Toolbar">
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
        <property key="lines" type="object" isList="true">
          <caption>Series</caption><description/>
          <properties>
            <propertyGroup caption="General">
              <property key="dataSet" type="enumeration" defaultValue="static">
                <caption>Data set</caption><description/>
                <enumerationValues><enumerationValue key="static">Single</enumerationValue></enumerationValues>
              </property>
            </propertyGroup>
          </properties>
        </property>
      </propertyGroup>
    </propertyGroup>
  </properties>
</widget>`

	def := parseWidgetXML(t, widgetXML)
	children := map[string][]PropertyDef{}
	for _, p := range def.Properties {
		children[p.Key] = p.Children
	}

	type want struct{ key, category string }
	check := func(prop string, wants []want) {
		t.Helper()
		got := children[prop]
		if len(got) != len(wants) {
			t.Fatalf("%s: %d sub-properties, want %d (%v)", prop, len(got), len(wants), got)
		}
		for i, w := range wants {
			if got[i].Key != w.key || got[i].Category != w.category {
				t.Errorf("%s[%d] = {%q, category %q}, want {%q, category %q}",
					prop, i, got[i].Key, got[i].Category, w.key, w.category)
			}
		}
	}
	// Direct: read in document order, the category from the property's own
	// <category> element, none when it declares none.
	check("toolbarItems", []want{{"itemType", "Appearance"}, {"caption", ""}})
	if got := children["toolbarItems"]; len(got) == 2 {
		if len(got[0].EnumValues) != 2 || got[1].Required {
			t.Errorf("direct sub-properties lost their attributes: %+v", got)
		}
	}
	// Control: grouped sub-properties keep the group caption as their category.
	check("lines", []want{{"dataSet", "General"}})
}
