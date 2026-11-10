// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"reflect"
	"testing"
)

// ako/mxcli#1303: a widgets() row had no caption or tooltip, so a rule could
// not tell which button's caption was "Delete" -- describe page prints it, the
// row did not. The keys are Studio Pro's, measured on CitrusGrove: a button's
// caption is CaptionTemplate (a Forms$ClientTemplate around a Texts$Text), a
// label's is Caption (a Texts$Text), a tooltip is a Texts$Text.
func rawText(langText ...string) map[string]any {
	items := []any{int32(3)}
	for i := 0; i < len(langText); i += 2 {
		items = append(items, map[string]any{"$Type": "Texts$Translation", "LanguageCode": langText[i], "Text": langText[i+1]})
	}
	return map[string]any{"$Type": "Texts$Text", "Items": items}
}

func clientTemplate(t map[string]any) map[string]any {
	return map[string]any{"$Type": "Forms$ClientTemplate", "Template": t, "Parameters": []any{int32(2)}}
}

func TestWidgetCaptionAndTooltip_Issue1303(t *testing.T) {
	page := wid("dv", "dvSale", "Forms$DataView", map[string]any{
		"Widgets": bsonList(
			wid("b1", "btnDelete", "Forms$ActionButton", map[string]any{
				"CaptionTemplate": clientTemplate(rawText("en_US", "Delete sale", "nl_NL", "Verkoop verwijderen")),
				"Tooltip": rawText("en_US", "Deletes it", "nl_NL", "Verwijdert hem"),
			}),
			wid("b2", "btnRemove", "Forms$ActionButton", map[string]any{
				"CaptionTemplate": clientTemplate(rawText("en_US", "Remove")),
				"Tooltip": rawText(), // a tooltip never set: no translations
			}),
			wid("t1", "text1", "Forms$Label", map[string]any{
				"Caption": rawText("en_US", "Total"),
			}),
			// control: an input's label is not its caption
			wid("tb", "textBox1", "Forms$TextBox", map[string]any{
				"LabelTemplate": clientTemplate(rawText("en_US", "Amount")),
			}),
		),
	})
	rows := extractWidgetsRecursive(page)
	for _, c := range []struct {
		name             string
		caption, tooltip map[string]string
	}{
		{"btnDelete", map[string]string{"en_US": "Delete sale", "nl_NL": "Verkoop verwijderen"},
			map[string]string{"en_US": "Deletes it", "nl_NL": "Verwijdert hem"}},
		{"btnRemove", map[string]string{"en_US": "Remove"}, nil},
		{"text1", map[string]string{"en_US": "Total"}, nil},
		{"textBox1", nil, nil},
	} {
		w := widgetByName(rows, c.name)
		if w == nil {
			t.Fatalf("%s not indexed", c.name)
		}
		if !reflect.DeepEqual(w.CaptionTexts, c.caption) || !reflect.DeepEqual(w.TooltipTexts, c.tooltip) {
			t.Errorf("%s: caption %v tooltip %v, want %v %v", c.name, w.CaptionTexts, w.TooltipTexts, c.caption, c.tooltip)
		}
	}
	// The row holds the project's default language, as pages.title does.
	if got := pickTranslation(widgetByName(rows, "btnDelete").CaptionTexts, "nl_NL"); got != "Verkoop verwijderen" {
		t.Errorf("nl_NL caption = %q", got)
	}
}
