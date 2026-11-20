// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The gate list is stated in three places, each of which is the only one some
// reader ever sees:
//
//   - the generated CLAUDE.md, re-read into EVERY session in the project, so it
//     is what an agent does by default without being asked;
//   - the `bootstrap-app` skill, which provisions the project and hands over;
//   - docs-site's bootstrap-prompt page, which is what a human reads before
//     pasting the seed prompt.
//
// They drifted, and the drift was invisible: `mxcli test` appeared in the
// skill only as a ports aside ("avoid 8081/8091/6544") and on no gate list at
// all, so testing was reachable only by a user asking for it by name — while
// `check` was named in 41 skills and ran almost every time. A gate stated in
// two places out of three is a gate that runs when someone remembers.
//
// These tests hold the three to the gate lists: the generated file to the list its
// project gets (classicGates, or verifyGates once the project has verify
// baselines), the skill and the docs page to every gate of either.

// bootstrapSkill returns the bootstrap-app SKILL.md as it SHIPS (from the
// embedded tree), not as it sits in .claude/. The embedded copy is what a user
// gets, and reading it also catches a skill edit that was never synced.
func bootstrapSkill(t *testing.T) string {
	t.Helper()
	const embedded = "skills/bootstrap-app/SKILL.md"
	b, err := skillsFS.ReadFile(embedded)
	if err != nil {
		t.Fatalf("cannot read embedded %s: %v\nRun `make sync-skills` (or `make build`) to mirror "+
			".claude/skills/mendix/ into cmd/mxcli/skills/.", embedded, err)
	}
	return string(b)
}

func TestEveryGateIsARealCommand(t *testing.T) {
	for _, g := range allProjectGates() {
		if _, _, err := rootCmd.Find(strings.Fields(g.Command)); err != nil {
			t.Errorf("gate %q names `mxcli %s`, which is not a registered command: %v",
				g.Ref, g.Command, err)
		}
	}
}

// The generated CLAUDE.md is the default behaviour: a gate missing here is a
// gate an agent never runs unless the user names it.
func TestGeneratedClaudeMDPublishesEveryGate(t *testing.T) {
	for name, c := range map[string]struct {
		md    string
		gates []projectGate
	}{
		"a project without verify baselines": {generateClaudeMD("Demo", "Demo.mpr"), classicGates},
		"a project with verify baselines":    {generateClaudeMDIn(projectWithBaseline(t), "Demo", "Demo.mpr"), verifyGates},
	} {
		for _, g := range c.gates {
			if !strings.Contains(c.md, "./"+g.Ref) {
				t.Errorf("the CLAUDE.md of %s does not name the %q gate; it is then run only when "+
					"the user asks for it by name", name, g.Ref)
			}
		}
	}
}

// projectWithBaseline is a project directory that has recorded the lint baseline
// `mxcli verify` needs, which is what puts it on the verify gate list.
func projectWithBaseline(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, verifyBaselineDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, verifyBaselineDir, "lint.json"), []byte(`{"version":1,"findings":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// gateBlock is the fenced gate list of a generated CLAUDE.md, so a test asks what
// the gates are and not whether a command is mentioned anywhere (`lint` is also
// named in the table of where to ask the tool).
func gateBlock(t *testing.T, md string) string {
	t.Helper()
	i := strings.Index(md, "## The gates, in order")
	if i < 0 {
		t.Fatal("no gate section")
	}
	rest := md[i:]
	start := strings.Index(rest, "```bash\n")
	end := strings.Index(rest[start+8:], "```")
	if start < 0 || end < 0 {
		t.Fatal("no fenced gate block")
	}
	return rest[start+8 : start+8+end]
}

// The gate list follows the project, not the binary. Without baselines `verify`
// reports every finding the project already had as new, and on a project with
// errors it fails on the whole backlog; the session-start bootstrap regenerates
// this file every session, so a binary upgrade must not move a project onto it.
func TestGateListFollowsTheVerifyBaseline(t *testing.T) {
	without := gateBlock(t, generateClaudeMD("Demo", "Demo.mpr"))
	with := gateBlock(t, generateClaudeMDIn(projectWithBaseline(t), "Demo", "Demo.mpr"))
	for _, want := range []string{"./mxcli lint -p Demo.mpr", "./mxcli docker check -p Demo.mpr"} {
		if !strings.Contains(without, want) {
			t.Errorf("a project without baselines lacks %q:\n%s", want, without)
		}
	}
	if strings.Contains(without, "mxcli verify") {
		t.Errorf("a project without baselines is told to run verify:\n%s", without)
	}
	if !strings.Contains(with, "./mxcli verify -p Demo.mpr") {
		t.Errorf("a project with baselines is not told to run verify:\n%s", with)
	}
	for _, not := range []string{"./mxcli lint -p", "./mxcli docker check"} {
		if strings.Contains(with, not) {
			t.Errorf("a project with baselines still runs %q, which verify runs:\n%s", not, with)
		}
	}
}

// What counts as having recorded baselines: the lint baseline file, in the
// directory next to the project. An empty directory, a directory standing where
// the file should be, or the old git-ignored location are not it.
func TestWhatPutsAProjectOnTheVerifyGateList(t *testing.T) {
	mk := func(rel string, dir bool) string {
		d := t.TempDir()
		p := filepath.Join(d, rel)
		if dir {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}
	for name, c := range map[string]struct {
		dir  string
		want bool
	}{
		"the lint baseline":            {mk("verify-baseline/lint.json", false), true},
		"only the mx check baseline":   {mk("verify-baseline/mxcheck.json", false), false},
		"an empty baseline directory":  {mk("verify-baseline", true), false},
		"a directory named lint.json":  {mk("verify-baseline/lint.json", true), false},
		"the old git-ignored location": {mk(".mxcli/verify/lint.json", false), false},
		"no project directory":         {"", false},
	} {
		if got := hasVerifyBaseline(c.dir); got != c.want {
			t.Errorf("%s: hasVerifyBaseline = %v, want %v", name, got, c.want)
		}
	}
}

// The path that reaches existing projects is the sync the bootstrap hook runs
// every session. Recording a baseline moves the project to the verify list at the
// next sync, and removing it moves it back; an upgrade alone does neither.
func TestSyncMovesAProjectOnAndOffTheVerifyGateList(t *testing.T) {
	dir := newSyncProject(t)
	path := filepath.Join(dir, "CLAUDE.md")
	mustWrite(t, path, wrapMxcliSection("# Mendix Project: Demo\n\nOld guidance.\n"))
	syncAndRead := func() string {
		t.Helper()
		if _, err := syncProjectTooling(dir); err != nil {
			t.Fatal(err)
		}
		return gateBlock(t, mustRead(t, path))
	}
	if got := syncAndRead(); !strings.Contains(got, "docker check") || strings.Contains(got, "verify") {
		t.Fatalf("a synced project without baselines:\n%s", got)
	}
	mustWrite(t, filepath.Join(dir, verifyBaselineDir, "lint.json"), `{"version":1,"findings":[]}`)
	if got := syncAndRead(); !strings.Contains(got, "./mxcli verify -p") || strings.Contains(got, "docker check") {
		t.Fatalf("a synced project with baselines:\n%s", got)
	}
	if err := os.Remove(filepath.Join(dir, verifyBaselineDir, "lint.json")); err != nil {
		t.Fatal(err)
	}
	if got := syncAndRead(); !strings.Contains(got, "docker check") || strings.Contains(got, "verify") {
		t.Fatalf("a synced project whose baseline was removed:\n%s", got)
	}
}

// The skill provisions the project and is the last thing read before the work
// starts, so it has to name the same gates the project's CLAUDE.md will.
func TestBootstrapSkillNamesEveryGate(t *testing.T) {
	skill := bootstrapSkill(t)
	for _, g := range allProjectGates() {
		if !strings.Contains(skill, g.Ref) {
			t.Errorf("the bootstrap-app skill does not name the %q gate, but the CLAUDE.md it "+
				"provisions does. Update .claude/skills/mendix/bootstrap-app/SKILL.md and re-run "+
				"`make sync-skills`.", g.Ref)
		}
	}
}

// The docs page describes the skill to a human deciding whether to trust it. A
// step the page omits is a step nobody reviews.
func TestBootstrapPromptDocNamesEveryGate(t *testing.T) {
	const path = "../../docs-site/src/tools/bootstrap-prompt.md"
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	doc := string(b)
	for _, g := range allProjectGates() {
		if !strings.Contains(doc, g.Ref) {
			t.Errorf("%s does not name the %q gate, but the procedure it documents runs it", path, g.Ref)
		}
	}
}

// The three descriptions of the procedure also have to agree about the quality
// report and the plan, which are the parts a coding agent skips first: they
// produce no error when omitted, so nothing else notices.
func TestBootstrapAndDefaultBehaviourAgreeOnQualityAndPlan(t *testing.T) {
	md := generateClaudeMD("Demo", "Demo.mpr")
	skill := bootstrapSkill(t)

	for _, want := range []struct{ substr, why string }{
		{"brain capture", "capturing requirements as they arrive is what keeps `brain plan` a real progress report"},
		{"FINDINGS.md", "the bootstrap creates it; without a rule to append to it, it is written once and then stops being true"},
	} {
		if !strings.Contains(md, want.substr) {
			t.Errorf("generated CLAUDE.md does not mention %q — %s", want.substr, want.why)
		}
		if !strings.Contains(skill, want.substr) {
			t.Errorf("bootstrap-app skill does not mention %q — %s", want.substr, want.why)
		}
	}

	// The baseline is only meaningful taken before any of the user's own work
	// is in the project, so the skill has to run `report` during provisioning,
	// not merely list it as something that exists.
	if !strings.Contains(skill, "quality baseline") {
		t.Error("bootstrap-app skill no longer takes a quality baseline; the `report` scores it " +
			"leaves behind are then uncomparable, because every later figure is the user's work " +
			"plus the template's with nothing to subtract")
	}
}

// Two modelling choices are first-class in Mendix and get reinvented in
// microflows by default, because the microflow version passes `check`, builds,
// and is flagged by nothing: a business process with human steps (a WORKFLOW)
// and an aggregation (a VIEW ENTITY). Neither is something a command can
// prompt for — `lint` cannot know that a Status attribute is standing in for a
// state machine — so if the instruction is not in all three descriptions of
// the procedure, it reaches an agent only when the user already knows to ask,
// which is exactly when it is least needed.
func TestModellingDefaultsAreStatedEverywhere(t *testing.T) {
	sources := map[string]string{
		"the generated CLAUDE.md": generateClaudeMD("Demo", "Demo.mpr"),
		"the bootstrap-app skill": bootstrapSkill(t),
	}
	const docPath = "../../docs-site/src/tools/bootstrap-prompt.md"
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", docPath, err)
	}
	sources[docPath] = string(doc)

	for _, want := range []struct{ phrase, why string }{
		{"workflow", "a process with human steps belongs in a workflow, not a status attribute plus microflows"},
		{"view entity", "an aggregation belongs in a view entity, not a microflow that retrieves every row to produce one number"},
	} {
		for name, body := range sources {
			if !strings.Contains(strings.ToLower(body), want.phrase) {
				t.Errorf("%s does not mention %q — %s", name, want.phrase, want.why)
			}
		}
	}
}

// The gate list had a completeness rule ("**They are the definition of done,
// not a menu** — a change is finished when they have all been run") sitting one
// line below an escalation rule ("each is only worth paying for once the one
// above is clean"). They contradict each other, and the bolded one wins.
//
// Applied to a list containing `docker check` (~25s), `test` (~30s cold) and
// `run --local`, the bolded reading mandates ~55s of gates and 5+ tool calls per
// change — and "a change" was never defined, so in practice it became each edit.
// That is not a hypothetical reading: it is what a session-cost comparison
// measured (523 model calls vs 123 for the same class of app elsewhere, 5-8 tool
// calls per change), and the report's own line — "I tested every admin flow ...
// most of those checks included screenshots" — is this instruction being
// followed, not an agent being careless. See
// docs/11-proposals/PROPOSAL_agent_loop_efficiency.md and ako/mxcli#608.
//
// The fix is the UNIT, not the list: every gate stays (the three-copy tests
// above exist because `test` fell off this list once), and what changes is that
// the gates are done-criteria for a coherent unit of work rather than for each
// edit. Iterate with `exec`, then run the gates once over the result.
//
// This is held in all three places for the same reason the gate list itself is:
// stated in two of the three, it is guidance that applies when someone remembers.
func TestGateBatchingUnitIsStatedEverywhere(t *testing.T) {
	const marker = "not per edit"

	sources := map[string]string{
		"the generated CLAUDE.md": generateClaudeMD("Demo", "Demo.mpr"),
		"the bootstrap-app skill": bootstrapSkill(t),
	}
	const docPath = "../../docs-site/src/tools/bootstrap-prompt.md"
	b, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", docPath, err)
	}
	sources[docPath] = string(b)

	for name, body := range sources {
		if !strings.Contains(strings.ToLower(body), marker) {
			t.Errorf("%s does not say the gates run once per change and %q — without the unit, "+
				"\"definition of done\" reads as ~55s of gates after every edit, which is the "+
				"dominant cost in an agent session", name, marker)
		}
	}
}
