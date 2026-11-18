// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2383 (mendixlabs/mxcli#1890): Marketplace texts carry CRLF
// ("{1} days\r\n", Workflow Commons' confirmation questions). A file whose
// CRs were lost on the way — an editor, an agent, git — named every one of
// them as unknown ("No text has … as its source"), and the confirmation
// questions stayed CE4899. A source matches whatever its line endings.
func TestApply_MatchesASourceRegardlessOfLineEndings_Issue2383(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "{1} days\r\n"))),
		unit(t, text(tr("en_US", "Sure?\r\n\r\nThis withdraws it."))),
	)
	dict := Dictionary{"{1} days\n": "{1} Tage\n", "Sure?\n\nThis withdraws it.": "Sicher?\n\nDies zieht es zurück."}
	stats, err := Apply(p, "en_US", "de_CH", dict, ModeMerge, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Set != 2 || len(stats.Unmatched) != 0 {
		t.Fatalf("stats = %+v, want 2 set and nothing unmatched", stats)
	}
	// The translation takes the source's line breaks, as Studio Pro stores them.
	if got := langsOf(t, p.units["A"])[0]["de_CH"]; got != "{1} Tage\r\n" {
		t.Errorf("translation = %q, want %q", got, "{1} Tage\r\n")
	}
	if got := langsOf(t, p.units["B"])[0]["de_CH"]; got != "Sicher?\r\n\r\nDies zieht es zurück." {
		t.Errorf("translation = %q", got)
	}

	// CONTROL: the exact spelling still matches, and a different text does not.
	p = newProject(t, unit(t, text(tr("en_US", "{1} days\r\n"))))
	stats, _ = Apply(p, "en_US", "de_CH", Dictionary{"{1} days\r\n": "{1} Tage\r\n", "{1} weeks\n": "x"}, ModeMerge, nil)
	if stats.Set != 1 || len(stats.Unmatched) != 1 || stats.Unmatched[0] != "{1} weeks\n" {
		t.Errorf("stats = %+v, want 1 set and {1} weeks unmatched", stats)
	}
}

func TestOutOfScope_MatchesRegardlessOfLineEndings_Issue2383(t *testing.T) {
	p := newProject(t, unit(t, text(tr("en_US", "{1} days\r\n"))))
	missed, err := OutOfScope(p, "en_US", func(model.ID) bool { return false }, Dictionary{"{1} days\n": "{1} Tage\n"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missed) != 1 || missed[0] != "{1} days\n" {
		t.Errorf("missed = %q, want the file's own key", missed)
	}
}
