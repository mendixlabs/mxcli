// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"
	"testing"
)

// ako/mxcli#2524: --accepts, --changes and --partly-meets had no batch
// spelling, so a decision accepting a lint finding could not be captured with
// --from-json at all.

// The issue's item, verbatim: the field name the reporter reached for must parse.
func TestParseBatchTakesAcceptsChangesAndPartlyMeets(t *testing.T) {
	got, err := ParseBatch(strings.NewReader(`[
		{"text":"Kept deliberately simple: ACT_ microflows do their own object changes.",
		 "anchors":["@FieldService.ACT_Triage_Complete"],
		 "accepts":["CONV010:one-happy-path demo app"]},
		{"text":"Approval is per team", "changes":["a1b2c3:per team, not per person"],
		 "partly_meets":["d4e5f6"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Accepts[0] != "CONV010:one-happy-path demo app" ||
		got[1].Changes[0] != "a1b2c3:per team, not per person" || got[1].PartlyMeets[0] != "d4e5f6" {
		t.Errorf("parsed: %+v", got)
	}
	// Unknown fields are still refused: the brain's JSON is snake_case, so the
	// camelCase spelling is a typo, not an alias.
	if _, err := ParseBatch(strings.NewReader(`[{"text":"A","partlyMeets":["d4e5f6"]}]`)); err == nil {
		t.Error("partlyMeets was accepted; it must be refused like any other unknown field")
	}
}

// A batch item must produce exactly what the flags produce for the same input,
// or the two spellings of one decision diverge.
func TestBatchItemWritesTheSameLinesAsTheFlags(t *testing.T) {
	it := BatchItem{
		Text:    "Kept deliberately simple: ACT_ microflows do their own object changes.",
		Anchors: []string{"@FieldService.ACT_Triage_Complete", "@FieldService.ACT_Triage_Start"},
		Accepts: []string{"CONV010:one-happy-path demo app"},
		Changes: []string{"a1b2c3:per team"},
	}
	got, err := BuildBatch([]BatchItem{it}, day, BatchChecks{})
	if err != nil {
		t.Fatal(err)
	}

	want, err := NewEntry(it.Text, it.Anchors, day)
	if err != nil {
		t.Fatal(err)
	}
	if want, err = AddVerdicts(want, VerdictChanges, it.Changes); err != nil {
		t.Fatal(err)
	}
	if want, err = AddAcceptances(want, it.Accepts); err != nil {
		t.Fatal(err)
	}
	if got[0].Body != want.Body {
		t.Errorf("batch body:\n%s\nflag body:\n%s", got[0].Body, want.Body)
	}
	// And the readers see them: one acceptance per anchor, one verdict.
	if n := len(Acceptances(got[0])); n != 2 {
		t.Errorf("acceptances = %d, want 2 (one per anchor)", n)
	}
	if v := Verdicts(got[0]); len(v) != 1 || v[0].RequirementID != "a1b2c3" || v[0].Reason != "per team" {
		t.Errorf("verdicts = %+v", v)
	}
	// Control: an item without them builds the plain entry.
	plain, err := BuildBatch([]BatchItem{{Text: it.Text, Anchors: it.Anchors}}, day, BatchChecks{})
	if err != nil || plain[0].Body != "" {
		t.Errorf("plain item: body %q, %v", plain[0].Body, err)
	}
}

// Validated per item, like the rest of the batch: the first bad one fails the
// whole batch, named by index, with the reason the flag form gives.
func TestBatchRefusesAnInvalidAcceptanceOrVerdict(t *testing.T) {
	known := map[string]bool{"a1b2c3": true}
	good := BatchItem{Text: "Fine", Anchors: []string{"@Sales.Order"}}
	for _, tc := range []struct {
		name string
		bad  BatchItem
		want string
	}{
		{"accepts on a requirement", BatchItem{Text: "R", Slice: "01-x", Anchors: []string{"@Sales.ACT_X"}, Accepts: []string{"SEC001"}}, "is a decision"},
		{"accepts on an open question", BatchItem{Text: "Q", Open: true, Accepts: []string{"SEC001"}}, "is a decision"},
		{"not a rule id", BatchItem{Text: "D", Anchors: []string{"@Sales.Order"}, Accepts: []string{"sec 001:x"}}, "not a lint rule id"},
		{"verdict on a requirement", BatchItem{Text: "R", Slice: "01-x", Anchors: []string{"@Sales.ACT_X"}, Changes: []string{"a1b2c3"}}, "not on a requirement"},
		{"not a requirement id", BatchItem{Text: "D", Changes: []string{"nope"}}, "not a requirement id"},
		{"unknown requirement", BatchItem{Text: "D", PartlyMeets: []string{"ffffff:x"}}, "no requirement ffffff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildBatch([]BatchItem{good, tc.bad}, day, BatchChecks{KnownRequirements: known})
			if err == nil || !strings.Contains(err.Error(), "items[1]") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want items[1] and %q", err, tc.want)
			}
		})
	}

	// Control: the known requirement passes, and with no plan to check
	// against (no store yet) the id is not checked — as the flags behave.
	ok := BatchItem{Text: "D", Changes: []string{"a1b2c3"}, PartlyMeets: []string{"ffffff"}}
	if _, err := BuildBatch([]BatchItem{{Text: "D", Changes: []string{"a1b2c3"}}}, day, BatchChecks{KnownRequirements: known}); err != nil {
		t.Errorf("a verdict on a known requirement was refused: %v", err)
	}
	if _, err := BuildBatch([]BatchItem{ok}, day, BatchChecks{}); err != nil {
		t.Errorf("with no plan to check against, ids are not checked: %v", err)
	}
}

// A requirement queued earlier in the SAME batch is not in the plan yet, so a
// verdict cannot name it — as `capture --changes` cannot name a staged one.
// The message says what to do.
func TestBatchVerdictCannotNameARequirementOnlyStaged(t *testing.T) {
	req := BatchItem{Text: "Orders must be approvable", Slice: "01-approvals", Anchors: []string{"@Sales.ACT_Approve"}}
	built, err := BuildBatch([]BatchItem{req}, day, BatchChecks{})
	if err != nil {
		t.Fatal(err)
	}
	dec := BatchItem{Text: "Approval is per team", Changes: []string{built[0].ID}}
	_, err = BuildBatch([]BatchItem{req, dec}, day, BatchChecks{KnownRequirements: map[string]bool{}})
	if err == nil || !strings.Contains(err.Error(), "items[1]") || !strings.Contains(err.Error(), "promote") {
		t.Errorf("err = %v, want items[1] refused with a pointer to promoting the requirement first", err)
	}
}
