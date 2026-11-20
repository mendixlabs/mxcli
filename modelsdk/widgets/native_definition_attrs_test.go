// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// Three properties of the definition mxcli generated from a package differed
// from what mx update-widgets writes, each one CE0463 on every instance
// (mx check 11.13.0, Native Mobile Resources 12.6.0):
//
//	Signature    imageSource      type="image" allowUpload="true"   AllowUpload false, want true
//	Bottom sheet triggerAttribute onChange="triggerAttributeChange" OnChangeProperty "", want the key
//	Maps         markers/…        <enumerationValue>Based on latitude and longitude
//	                                 </enumerationValue>            caption trimmed, want it verbatim
//
// The parser already read onChange; the generator wrote "" regardless.
// allowUpload was not read at all. And the parser trimmed an option's caption,
// which Mendix stores as written — trailing newline and indentation included.
func TestGenerateFromMPK_DefinitionAttrsAsDeclared(t *testing.T) {
	ResetPlaceholderCounter()
	const latlng = "Based on latitude and longitude\n                                        "
	def := &mpk.WidgetDefinition{
		ID: "com.example.W", Name: "W", Version: "1.0.0",
		Properties: []mpk.PropertyDef{
			{Key: "imageSource", Type: "image", Required: true, AllowUpload: true},
			{Key: "plainImage", Type: "image"},
			{Key: "triggerAttribute", Type: "attribute", OnChange: "triggerAttributeChange", AllowedTypes: []string{"Boolean"}},
			{Key: "triggerAttributeChange", Type: "action"},
			{Key: "locationType", Type: "enumeration", DefaultValue: "address", EnumValues: []mpk.EnumValue{
				{Key: "address", Caption: "Based on address", RawCaption: "Based on address"},
				{Key: "latlng", Caption: "Based on latitude and longitude", RawCaption: latlng},
			}},
		},
	}
	vts := valueTypesByKey(t, GenerateFromMPK(def))

	if got := vts["imageSource"]["AllowUpload"]; got != true {
		t.Errorf("imageSource AllowUpload = %v, want true (allowUpload=\"true\")", got)
	}
	if got := vts["plainImage"]["AllowUpload"]; got != false {
		t.Errorf("control: plainImage AllowUpload = %v, want false", got)
	}
	if got := vts["triggerAttribute"]["OnChangeProperty"]; got != "triggerAttributeChange" {
		t.Errorf("triggerAttribute OnChangeProperty = %q, want triggerAttributeChange", got)
	}
	if got := vts["triggerAttributeChange"]["OnChangeProperty"]; got != "" {
		t.Errorf("control: an action's OnChangeProperty = %q, want empty", got)
	}
	enum, _ := vts["locationType"]["EnumerationValues"].([]any)
	captions := map[string]any{}
	for _, e := range enum {
		if m, ok := e.(map[string]any); ok {
			captions[m["_Key"].(string)] = m["Caption"]
		}
	}
	if captions["latlng"] != latlng {
		t.Errorf("latlng caption = %q, want it as written in the package, %q", captions["latlng"], latlng)
	}
	if captions["address"] != "Based on address" {
		t.Errorf("control: address caption = %q", captions["address"])
	}
}

func valueTypesByKey(t *testing.T, tmpl *WidgetTemplate) map[string]map[string]any {
	t.Helper()
	pts := tmpl.Type["ObjectType"].(map[string]any)["PropertyTypes"].([]any)
	out := map[string]map[string]any{}
	for _, pt := range pts[1:] {
		m := pt.(map[string]any)
		if vt, ok := m["ValueType"].(map[string]any); ok {
			out[m["PropertyKey"].(string)] = vt
		}
	}
	return out
}
