// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2327 (mendixlabs/mxcli#1870): `create or modify` of a microflow
// whose show message text changed edits the activity in place, and the edit
// is written from the statement — one language. The splice saves as a patch,
// which canon does not carry translations onto, so the message kept only the
// language MDL wrote: its nl_NL text was deleted. Studio Pro keeps the other
// languages when the default text is edited, stale as they then are.

func showMessage(act, text string, langs ...[2]string) bson.D {
	var items []bson.D
	for i, l := range langs {
		id := ""
		if act != "" {
			id = act + "t" + string(rune('0'+i))
		}
		items = append(items, el(id, "Texts$Translation", kv("LanguageCode", l[0]), kv("Text", l[1])))
	}
	textID, tplID := "", ""
	if act != "" {
		textID, tplID = act+"x", act+"p"
	}
	return el(act, "Microflows$ShowMessageAction",
		kv("Blocking", true),
		kv("ErrorHandlingType", "Rollback"),
		kv("Template", el(tplID, "Microflows$TextTemplate",
			kv("Parameters", list(2)),
			kv("Text", el(textID, "Texts$Text", kv("Items", list(3, items...)))))),
		kv("Type", "Information"))
}

func messageTranslations(a bson.D) map[string]string {
	out := map[string]string{}
	text := dDoc(dDoc(dDoc(a, "Action"), "Template"), "Text")
	for _, it := range arrayElements(dGet(text, "Items")) {
		if d, ok := it.(bson.D); ok {
			out[dString(d, "LanguageCode")] = dString(d, "Text")
		}
	}
	return out
}

func TestSplice_EditKeepsOtherTranslations(t *testing.T) {
	a := editA(t,
		showMessage("act", "", [2]string{"en_US", "The password has been updated."},
			[2]string{"nl_NL", "Het wachtwoord is gewijzigd."}),
		showMessage("", "", [2]string{"en_US", "The password has been updated!"}),
		"")
	got := messageTranslations(a)
	if got["en_US"] != "The password has been updated!" {
		t.Errorf("en_US = %q, want the edited text", got["en_US"])
	}
	if got["nl_NL"] != "Het wachtwoord is gewijzigd." {
		t.Errorf("nl_NL = %q, want the stored translation kept (got %v)", got["nl_NL"], got)
	}
}

// Control: the language the statement writes is the statement's, so an edit of
// it lands even when the stored text had only that language.
func TestSplice_EditOfTheOnlyLanguageLands(t *testing.T) {
	a := editA(t,
		showMessage("act", "", [2]string{"en_US", "Saved."}),
		showMessage("", "", [2]string{"en_US", "Saved!"}),
		"")
	got := messageTranslations(a)
	if len(got) != 1 || got["en_US"] != "Saved!" {
		t.Errorf("translations = %v, want only en_US Saved!", got)
	}
}
