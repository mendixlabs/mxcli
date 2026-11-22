// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"encoding/json"
	"fmt"
	"testing"
)

// ako/mxcli#2518: a widget property's shipped default text reaches the build.
// The object-list items already carried it (#891); a top-level property did
// not, so a visible text template could only ever be written empty.
func TestTopLevelPropertyCarriesDefaultTranslations(t *testing.T) {
	n := 0
	_, _, ids, _, _, err := GetTemplateFullBSON("com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter", func() string {
		n++
		return fmt.Sprintf("%032x", n)
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := ids["emptyOptionCaption"].DefaultTranslations
	if len(got) != 2 || got[0].LanguageCode != "en_US" || got[0].Text != "None" || got[1].Text != "Niets" {
		t.Errorf("emptyOptionCaption DefaultTranslations = %+v, want en_US None / nl_NL Niets", got)
	}
}

// The embedded drop-down filter is a freshly dropped widget at its default
// configuration (filterable and multiSelect off), where emptyOptionCaption is
// visible, so it holds the shipped text — what `mx update-widgets` writes there.
// It held an empty template, which update-widgets rewrites and mx check reports
// as CE0463. (An empty template is not wrong everywhere: the text and number
// filters' screenReaderInputCaption ships text too, and mx check accepts both
// Studio Pro's empty translation and mxcli's empty template there.)
func TestDropdownFilterTemplate_EmptyOptionCaptionHoldsShippedText(t *testing.T) {
	data, err := templateFS.ReadFile("templates/mendix-11.6/datagrid-dropdown-filter.json")
	if err != nil {
		t.Fatal(err)
	}
	var tmpl struct {
		Type   map[string]any `json:"type"`
		Object map[string]any `json:"object"`
	}
	if err := json.Unmarshal(data, &tmpl); err != nil {
		t.Fatal(err)
	}
	var ptID string
	walkJSON(tmpl.Type, func(m map[string]any) {
		if m["$Type"] == "CustomWidgets$WidgetPropertyType" && m["PropertyKey"] == "emptyOptionCaption" {
			ptID, _ = m["$ID"].(string)
		}
	})
	var texts []string
	found := false
	walkJSON(tmpl.Object, func(m map[string]any) {
		if m["$Type"] != "CustomWidgets$WidgetProperty" || m["TypePointer"] != ptID {
			return
		}
		found = true
		val, _ := m["Value"].(map[string]any)
		tt, _ := val["TextTemplate"].(map[string]any)
		tpl, _ := tt["Template"].(map[string]any)
		items, _ := tpl["Items"].([]any)
		for _, it := range items {
			if tr, ok := it.(map[string]any); ok {
				texts = append(texts, fmt.Sprint(tr["LanguageCode"], "=", tr["Text"]))
			}
		}
	})
	if !found {
		t.Fatal("no emptyOptionCaption property in the template object")
	}
	if len(texts) != 2 || texts[0] != "en_US=None" || texts[1] != "nl_NL=Niets" {
		t.Errorf("emptyOptionCaption template = %v, want [en_US=None nl_NL=Niets]", texts)
	}
}

func walkJSON(v any, fn func(map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		fn(x)
		for _, c := range x {
			walkJSON(c, fn)
		}
	case []any:
		for _, c := range x {
			walkJSON(c, fn)
		}
	}
}
