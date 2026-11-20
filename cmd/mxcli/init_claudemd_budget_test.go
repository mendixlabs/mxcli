// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"strings"
	"testing"
)

// The generated CLAUDE.md is re-read into EVERY context this project starts,
// so its size is a per-session tax rather than a one-off. Measured on a real
// build (ako/CapTrackV4, 15 slices): 6,918 tokens × 15 sub-agent contexts =
// ~104k tokens of re-read reference.
//
// The budget is expressed in bytes because that is what the generator produces
// and what a test can measure without a tokenizer; ~4 bytes/token is the usual
// ratio for English prose with code fences, so 6,000 bytes is roughly the
// 1,500-token target.
const claudeMDBudgetBytes = 6000

func TestGeneratedClaudeMDStaysWithinItsContextBudget(t *testing.T) {
	// Both gate lists: a project with verify baselines gets the other one.
	for name, md := range map[string]string{
		"without verify baselines": generateClaudeMD("Demo", "Demo.mpr"),
		"with verify baselines":    generateClaudeMDIn(projectWithBaseline(t), "Demo", "Demo.mpr"),
	} {
		if len(md) > claudeMDBudgetBytes {
			t.Errorf("generated CLAUDE.md %s is %d bytes (~%d tokens), over the %d-byte budget.\n"+
				"It is re-read into every context this project starts, so anything mxcli can\n"+
				"answer on demand belongs behind a command, not in here.",
				name, len(md), len(md)/4, claudeMDBudgetBytes)
		}
	}
}

// A table of rule IDs, MDL commands or skill names in the generated file is a
// transcription of something mxcli answers authoritatively, and it drifts
// silently: nothing fails when a rule is added, so the copy in every project's
// CLAUDE.md quietly stops matching the tool.
//
// This is not hypothetical here. #906 was the same failure in the skill table,
// which had drifted to 12 of 68 before anyone noticed. At the commit this test
// was written the lint tables were wrong on both axes at once: 10 of the 14
// registered built-in rules were listed (MPR008-011 missing, MPR008 being
// precisely the rule whose guidance projects argue with), and the Starlark
// count read "27" against 31 shipped files.
func TestGeneratedClaudeMDDoesNotTranscribeWhatMxcliAnswers(t *testing.T) {
	md := generateClaudeMD("Demo", "Demo.mpr")

	// Every built-in rule ID the linter registers. If the doc names one, it is
	// keeping a list it does not own.
	for _, r := range builtinLintRules() {
		if strings.Contains(md, r.ID()) {
			t.Errorf("generated CLAUDE.md names lint rule %s. The rule list belongs to "+
				"`mxcli lint --list-rules`; a copy here goes stale the next time a rule is added.", r.ID())
		}
	}

	// A markdown table row whose first cell is an MDL statement is the command
	// reference being restated. `mxcli syntax [topic]` owns that, and has a
	// --json mode built for exactly this consumer.
	cmdRow := regexp.MustCompile("(?m)^\\| `(SHOW|DESCRIBE|CREATE|ALTER|DROP|GRANT|REVOKE|MOVE|REFRESH|SQL) ")
	if m := cmdRow.FindAllString(md, -1); len(m) > 0 {
		t.Errorf("generated CLAUDE.md restates %d MDL command table rows (e.g. %q). "+
			"Point at `mxcli syntax <topic>` instead.", len(m), strings.TrimSpace(m[0]))
	}
}

// Deleting the tables is only safe if what replaces them actually resolves.
// These are the commands the trimmed file sends a reader to, so a rename or
// removal of one has to break this test rather than a project's onboarding.
func TestGeneratedClaudeMDPointsAtCommandsThatExist(t *testing.T) {
	md := generateClaudeMD("Demo", "Demo.mpr")
	for _, want := range []string{
		"mxcli syntax",        // the MDL command + syntax reference
		"--list-rules",        // the lint rule list
		".ai-context/skills/", // the skill index, routed by frontmatter
	} {
		if !strings.Contains(md, want) {
			t.Errorf("generated CLAUDE.md does not mention %q; the content it replaced is then simply gone", want)
		}
	}
	for _, name := range []string{"syntax", "lint", "check", "exec"} {
		if _, _, err := rootCmd.Find([]string{name}); err != nil {
			t.Errorf("CLAUDE.md sends readers to `mxcli %s`, which is not a registered command: %v", name, err)
		}
	}
}
