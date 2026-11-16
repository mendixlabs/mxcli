// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2325: `git merge --plan` resolves per unit, so "take": "theirs" on
// a domain model replaced the whole unit: Shop.OursOnly, which no one else
// touched, was gone after the merge. The property report listed it under Yours
// and nothing said the choice would drop it.

const oursDiff2325 = `{"unitDifferences":[{"id":"` + tDM + `","type":"DomainModels$DomainModel","change":"Modified","changeDetails":[` +
	`{"change":"Added","changeDescription":"Entity 'Shop.OursOnly'"},` +
	`{"change":"Modified","changeDescription":"Attribute 'Shop.Customer.Name'","changeDetails":[{"property":"New type > Length","baseValue":"200","mineValue":"150"}]}]}]}`

const theirsDiff2325 = `{"unitDifferences":[{"id":"` + tDM + `","type":"DomainModels$DomainModel","change":"Modified","changeDetails":[` +
	`{"change":"Added","changeDescription":"Entity 'Shop.TheirsOnly'"},` +
	`{"change":"Modified","changeDescription":"Attribute 'Shop.Customer.Name'","changeDetails":[{"property":"New type > Length","baseValue":"200","mineValue":"300"}]}]}]}`

func diffSides2325(t *testing.T) (diffSide, diffSide) {
	t.Helper()
	dir := t.TempDir()
	o, th := filepath.Join(dir, "ours.json"), filepath.Join(dir, "theirs.json")
	if err := os.WriteFile(o, []byte(oursDiff2325), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(th, []byte(theirsDiff2325), 0o644); err != nil {
		t.Fatal(err)
	}
	return diffSide{path: o}, diffSide{path: th}
}

func TestWholeUnitDropReport_NamesTheNonConflictingChanges_Issue2325(t *testing.T) {
	ours, theirs := diffSides2325(t)
	got := wholeUnitDropReport(ours, theirs, []string{tDM}, nil)
	for _, want := range []string{
		`"take": "theirs" also drops these changes of yours`, "Entity 'Shop.OursOnly'",
		`"take": "ours" also drops these changes of theirs`, "Entity 'Shop.TheirsOnly'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
	// The conflicting attribute is the choice itself, not a loss to warn of.
	if strings.Contains(got, "Shop.Customer.Name") {
		t.Errorf("report lists the conflicting attribute as dropped:\n%s", got)
	}
}

// With a plan, only the side each unit's choice drops is reported.
func TestWholeUnitDropReport_PlanNamesWhatItsChoiceDrops_Issue2325(t *testing.T) {
	ours, theirs := diffSides2325(t)
	got := wholeUnitDropReport(ours, theirs, []string{tDM}, map[string]string{tDM: "theirs"})
	if !strings.Contains(got, "Warning") || !strings.Contains(got, "Entity 'Shop.OursOnly'") {
		t.Errorf("plan warning lacks the dropped entity:\n%s", got)
	}
	if strings.Contains(got, "TheirsOnly") {
		t.Errorf("plan warning names what the choice keeps:\n%s", got)
	}
}

// CONTROL: when the two sides changed only the same element, the choice drops
// nothing else and nothing is reported.
func TestWholeUnitDropReport_NothingElseChangedSaysNothing_Issue2325(t *testing.T) {
	dir := t.TempDir()
	one := `{"unitDifferences":[{"id":"` + tDM + `","type":"DomainModels$DomainModel","changeDetails":[{"changeDescription":"Attribute 'Shop.Customer.Name'"}]}]}`
	o, th := filepath.Join(dir, "o.json"), filepath.Join(dir, "t.json")
	_ = os.WriteFile(o, []byte(one), 0o644)
	_ = os.WriteFile(th, []byte(one), 0o644)
	if got := wholeUnitDropReport(diffSide{path: o}, diffSide{path: th}, []string{tDM}, nil); got != "" {
		t.Errorf("report = %q, want none", got)
	}
}

// End to end through the conflict stage: the report printed before a plan is
// written says what each choice drops.
func TestGitMerge_ConflictReportSaysWhatATakeDrops_Issue2325(t *testing.T) {
	dir, mprPath := modelConflictRepo(t)
	doctorEnv(t, dir)
	mx := mxRunners{
		merge: func(base, mine, theirs string, shas [3]string) (int, string, error) {
			return 2, `[{"ConflictResolutionDetails":"ManualResolutionRequired","UnitId":"` + tDM + `"}]`, nil
		},
		diff: func(base, mine, outJSON string) error {
			body := oursDiff2325
			if strings.Contains(mine, "theirs") {
				body = theirsDiff2325
			}
			return os.WriteFile(outJSON, []byte(body), 0o644)
		},
	}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "side", "", "", "", false, mx, &out); err == nil {
		t.Fatal("a manual conflict must end with the decision error")
	}
	if !strings.Contains(out.String(), "Entity 'Shop.OursOnly'") || !strings.Contains(out.String(), "also drops") {
		t.Errorf("conflict report does not say what taking theirs drops:\n%s", out.String())
	}
	gitInE(t, dir, "merge", "--abort")
}

// And when the plan is applied: the warning names what that plan's choice
// drops, before the merge is committed.
func TestGitMerge_PlanWarnsWhatItDrops_Issue2325(t *testing.T) {
	dir, mprPath := twoUnitConflictRepo(t)
	doctorEnv(t, dir)
	result := baseUnits(t)
	result[tDM] = tUnit{tMod, unitDoc(t, "DomainModels$DomainModel", "", mainSideKey)}
	result[tMf] = tUnit{tMod, unitDoc(t, "Microflows$Microflow", "ACT_Save", mainSideKey)}
	planPath := writePlanFor(t, dir, map[string]string{tDM: "theirs", tMf: "ours"})
	mx := mxRunners{
		merge: engineDecides(t, result, []string{tDM, tMf}, nil),
		diff: func(base, mine, outJSON string) error {
			body := oursDiff2325
			if strings.Contains(mine, "theirs") {
				body = theirsDiff2325
			}
			return os.WriteFile(outJSON, []byte(body), 0o644)
		},
	}
	var out bytes.Buffer
	if err := mergeRun(mprPath, "side", "", "", planPath, false, mx, &out); err != nil {
		t.Fatalf("mergeRun --plan: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "Warning: the plan takes each unit whole") || !strings.Contains(got, "Entity 'Shop.OursOnly'") {
		t.Errorf("plan run does not warn what taking theirs drops:\n%s", got)
	}
	if i, j := strings.Index(got, "Warning: the plan"), strings.Index(got, "committed"); j >= 0 && i > j {
		t.Errorf("the warning comes after the commit:\n%s", got)
	}
	assertPlanMergeCommitted(t, dir, mprPath)
}
