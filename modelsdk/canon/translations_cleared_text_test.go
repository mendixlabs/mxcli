// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"testing"
)

// A text the statement clears is written as one empty translation in the
// authoring language, ("en_US", ""), so the carry treats that language as
// written and leaves it empty. Rebuilt with no translation at all — as a data
// grid column header was — it looked like a text MDL could not express, and the
// stored "Triage" was carried straight back: `create or replace` reported
// "Unchanged page" and the caption stayed.
func TestCarryTranslations_ClearedTextStaysCleared(t *testing.T) {
	stored := textDoc(t, 10, "P", tr(11, "en_US", "Triage"))
	rebuilt := textDoc(t, 20, "P", tr(21, "en_US", ""))

	got := translationsOf(t, CarryTranslations(rebuilt, stored))
	if got["en_US"] != "" || len(got) != 1 {
		t.Errorf("translations = %v, want only en_US \"\"", got)
	}
}

// The other half: Studio Pro stores an empty pluggable caption with no
// translation at all. A rebuild that writes ("en_US", "") for it must not
// become a write, or every describe -> exec of such a page churns. The empty
// translation is dropped where the stored text has none in that language.
func TestCarryTranslations_EmptyTranslationOverAnEmptyStoredTextIsDropped(t *testing.T) {
	stored := textDoc(t, 10, "P")
	rebuilt := textDoc(t, 20, "P", tr(21, "en_US", ""))

	got := CarryTranslations(rebuilt, stored)
	if n := len(translationsOf(t, got)); n != 0 {
		t.Errorf("translations = %v, want none, as stored", translationsOf(t, got))
	}
	same, err := Equal(got, stored)
	if err != nil || !same {
		t.Errorf("the result is not canonically equal to the stored text (err %v): the rewrite would churn", err)
	}
}

// Control for the drop: a non-empty translation, or an empty one the stored
// text also has, is kept as written.
func TestCarryTranslations_OnlyAnUnmatchedEmptyTranslationIsDropped(t *testing.T) {
	stored := textDoc(t, 10, "P", tr(11, "en_US", ""))
	rebuilt := textDoc(t, 20, "P", tr(21, "en_US", ""))
	if got := translationsOf(t, CarryTranslations(rebuilt, stored)); len(got) != 1 {
		t.Errorf("stored en_US \"\" too: translations = %v, want it kept", got)
	}

	stored = textDoc(t, 10, "P")
	rebuilt = textDoc(t, 20, "P", tr(21, "en_US", "Title"))
	out := CarryTranslations(rebuilt, stored)
	if got := translationsOf(t, out); got["en_US"] != "Title" {
		t.Errorf("translations = %v, want en_US \"Title\" kept", got)
	}
	if !bytes.Equal(out, rebuilt) && len(translationsOf(t, out)) != 1 {
		t.Errorf("a written text was changed: %v", translationsOf(t, out))
	}
}
