// SPDX-License-Identifier: Apache-2.0

package mpk

import "testing"

// `allowUpload` is read from an image/file property, and an enumeration
// option's caption is kept as written: Mendix stores it verbatim, and a
// trimmed copy is CE0463 (Native Mobile Resources 12.6.0's Maps). The trimmed
// caption stays for display.
func TestParse_ReadsAllowUploadAndRawEnumCaption(t *testing.T) {
	const widgetXML = `<?xml version="1.0" encoding="utf-8"?>
<widget id="com.example.W" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>W</name>
  <properties>
    <propertyGroup caption="General">
      <property key="imageSource" type="image" required="true" allowUpload="true">
        <caption>Image</caption><description/>
      </property>
      <property key="plainImage" type="image" required="false">
        <caption>Plain</caption><description/>
      </property>
      <property key="locationType" type="enumeration" defaultValue="address">
        <caption>Location</caption><description/>
        <enumerationValues>
          <enumerationValue key="address">Based on address</enumerationValue>
          <enumerationValue key="latlng">Based on latitude and longitude
          </enumerationValue>
        </enumerationValues>
      </property>
    </propertyGroup>
  </properties>
</widget>`

	def := parseWidgetXML(t, widgetXML)
	byKey := map[string]PropertyDef{}
	for _, p := range def.Properties {
		byKey[p.Key] = p
	}
	if !byKey["imageSource"].AllowUpload {
		t.Error("imageSource: allowUpload=\"true\" not read")
	}
	if byKey["plainImage"].AllowUpload {
		t.Error("control: plainImage declares no allowUpload")
	}
	var latlng EnumValue
	for _, ev := range byKey["locationType"].EnumValues {
		if ev.Key == "latlng" {
			latlng = ev
		}
	}
	if want := "Based on latitude and longitude\n          "; latlng.RawCaption != want {
		t.Errorf("RawCaption = %q, want %q", latlng.RawCaption, want)
	}
	if latlng.Caption != "Based on latitude and longitude" {
		t.Errorf("Caption = %q, want the trimmed text for display", latlng.Caption)
	}
}
