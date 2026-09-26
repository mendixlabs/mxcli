// SPDX-License-Identifier: Apache-2.0

package mpk

import (
	"os"
	"testing"
)

// An action property may declare the variables its action receives
// (`<actionVariables>`). They are part of the widget DEFINITION — Studio Pro
// stores them on the ValueType even when no action is configured — so dropping
// them at parse time left nothing for the generator to write, and every page
// carrying such a widget was CE0463 (mendixlabs/mxcli#1200: Signature 2.1.0,
// Calendar 2.6.0).
func TestParse_ReadsActionVariables(t *testing.T) {
	const widgetXML = `<?xml version="1.0" encoding="utf-8"?>
<widget id="com.example.W" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>W</name>
  <properties>
    <propertyGroup caption="General">
      <property key="onSign" type="action" required="false">
        <caption>On sign</caption><description/>
        <actionVariables>
          <actionVariable key="signature" caption="Signature" type="String" />
          <actionVariable key="signedAt" caption="Signed at" type="DateTime" />
        </actionVariables>
      </property>
      <property key="plainAction" type="action" required="false">
        <caption>Plain</caption><description/>
      </property>
      <property key="items" type="object" isList="true">
        <caption>Items</caption><description/>
        <properties>
          <propertyGroup caption="Item">
            <property key="onPick" type="action" required="false">
              <caption>On pick</caption><description/>
              <actionVariables>
                <actionVariable key="index" caption="Index" type="Integer" />
              </actionVariables>
            </property>
          </propertyGroup>
        </properties>
      </property>
    </propertyGroup>
  </properties>
</widget>`

	def := parseWidgetXML(t, widgetXML)
	byKey := map[string]PropertyDef{}
	for _, p := range def.Properties {
		byKey[p.Key] = p
	}

	got := byKey["onSign"].ActionVariables
	want := []ActionVariable{
		{Key: "signature", Type: "String", Caption: "Signature"},
		{Key: "signedAt", Type: "DateTime", Caption: "Signed at"},
	}
	if len(got) != len(want) {
		t.Fatalf("onSign ActionVariables = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("onSign ActionVariables[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if av := byKey["plainAction"].ActionVariables; len(av) != 0 {
		t.Errorf("plainAction declares none, parsed %+v", av)
	}

	var nested []ActionVariable
	for _, c := range byKey["items"].Children {
		if c.Key == "onPick" {
			nested = c.ActionVariables
		}
	}
	if len(nested) != 1 || nested[0] != (ActionVariable{Key: "index", Type: "Integer", Caption: "Index"}) {
		t.Errorf("nested onPick ActionVariables = %+v, want one {index Integer Index}", nested)
	}
}

// The real package: Combobox 2.5.0's onChangeFilterInputEvent declares one.
func TestParse_ComboboxFixtureActionVariable(t *testing.T) {
	const path = "../../../testdata/expr-checker/widgets/com.mendix.widget.web.Combobox.mpk"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	def, err := ParseMPKForWidget(path, "com.mendix.widget.web.combobox.Combobox")
	if err != nil || def == nil {
		t.Fatalf("parse: %v", err)
	}
	for _, p := range def.Properties {
		if p.Key == "onChangeFilterInputEvent" {
			want := ActionVariable{Key: "filterInput", Type: "String", Caption: "Filter Input"}
			if len(p.ActionVariables) != 1 || p.ActionVariables[0] != want {
				t.Errorf("onChangeFilterInputEvent ActionVariables = %+v, want [%+v]", p.ActionVariables, want)
			}
			return
		}
	}
	t.Fatal("onChangeFilterInputEvent not parsed")
}
