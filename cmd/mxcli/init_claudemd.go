// SPDX-License-Identifier: Apache-2.0

// init_claudemd.go - CLAUDE.md generation for Mendix projects
package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"strings"
)

// yamlSingleQuote wraps s in YAML single quotes and escapes any internal
// single quotes by doubling them, so the result is safe to embed in a YAML
// value without further quoting.
func yamlSingleQuote(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "'", "''")
	return "'" + s + "'"
}

// wrapSkillContent gives a skill the frontmatter OpenCode reads: its name, its
// description, and the opencode marker. A skill that already has frontmatter
// (the Agent Skills layout every bundled skill uses) keeps its own description
// and is given ONE block: prepending a second one, described by the skill's
// first heading, made OpenCode read "Agents" where the skill says "… Use when
// adding GenAI features", and the description is what decides whether a skill
// is invoked at all (ako/mxcli#1822).
func wrapSkillContent(skillName string, content []byte) []byte {
	description, body := splitSkillFrontmatter(content)
	if description == "" {
		description = extractSkillDescription(body)
	}
	frontmatter := fmt.Sprintf("---\nname: %s\ndescription: %s\ncompatibility: opencode\n---\n\n", yamlSingleQuote(skillName), yamlSingleQuote(description))
	return append([]byte(frontmatter), body...)
}

// splitSkillFrontmatter returns the description a skill's leading YAML
// frontmatter declares ("" when it has none, or none readable) and the content
// after that frontmatter. Content without frontmatter is returned whole.
func splitSkillFrontmatter(content []byte) (description string, body []byte) {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return "", content
	}
	head, rest, ok := strings.Cut(text[len("---\n"):], "\n---\n")
	if !ok {
		return "", content
	}
	var fm struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(head), &fm); err != nil {
		return "", content
	}
	return strings.TrimSpace(fm.Description), []byte(strings.TrimLeft(rest, "\n"))
}

// extractSkillDescription returns a one-line description for the skill by
// finding the first top-level markdown heading (# ...) and stripping a leading
// "Skill: " prefix if present.  Falls back to "MDL skill" if no heading is
// found.
func extractSkillDescription(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			desc := strings.TrimPrefix(line, "# ")
			desc = strings.TrimPrefix(desc, "Skill: ")
			return strings.TrimSpace(desc)
		}
	}
	return "MDL skill"
}

// projectGate is one command in the ordered gate list the generated CLAUDE.md
// publishes as a project's definition of done.
//
// It exists as data rather than as literal markdown lines because the same
// sequence is stated in three places that must agree: this file (re-read into
// every session, so it IS the default behaviour), the `bootstrap-app` skill
// (which sets the project up and hands over), and the bootstrap-prompt docs
// page that describes the skill. When they were three hand-kept copies,
// `mxcli test` was named in the skill's ports note and on no gate list at all,
// so testing was reachable only by a user asking for it by name.
// TestBootstrapProcedureNamesEveryGate holds the three together.
type projectGate struct {
	// Cmd is the command as written, without the leading "./mxcli ".
	// A %s is substituted with the project's .mpr path.
	Cmd string
	// Note is the trailing comment explaining what the gate buys and roughly
	// what it costs.
	Note string
	// Ref is the substring that must also appear in the bootstrap skill and on
	// the docs page for this gate to count as stated there.
	Ref string
	// Command is the cobra command path ("docker check" for a subcommand),
	// checked to exist so a rename breaks a test rather than every project's
	// onboarding.
	Command string
}

// projectGates is the ordered, cheapest-first gate list. Adding one here adds
// it to every generated CLAUDE.md, and requires naming it in the bootstrap
// skill and docs page too.
var projectGates = []projectGate{
	{"check script.mdl -p %s", "syntax + references, no apply (~2s)", "mxcli check", "check"},
	{"exec script.mdl -p %s", "apply", "mxcli exec", "exec"},
	{"lint -p %s", "rules (~3s)", "mxcli lint", "lint"},
	{"report -p %s", "scored quality report", "mxcli report", "report"},
	{"docker check -p %s", "mxbuild, the slow one (~25s)", "mxcli docker check", "docker check"},
	{"test tests/ -p %s --local", "microflow tests (~30s cold, ~2s warm)", "mxcli test", "test"},
	// --detach because an agent's tool call must return: without it the run was
	// started with nohup and then managed by sleep/grep/pkill loops, a quarter
	// of all tool calls in measured sessions. `run wait` is what follows an exec.
	{"run --local --watch --detach -p %s", "the app, backgrounded; then `run wait`/`status`/`stop`", "mxcli run --local", "run"},
}

// renderProjectGates writes the gate list as aligned shell lines, comments in
// one column.
//
// The single longest command (the detached run) overhangs the column instead of
// setting it: every byte of padding is paid on every gate line in every
// session, and aligning to that one line pushed all seven comments right.
func renderProjectGates(mprPath string) string {
	lines := make([]string, len(projectGates))
	longest, width := 0, 0
	for i, g := range projectGates {
		lines[i] = "./mxcli " + fmt.Sprintf(g.Cmd, mprPath)
		switch n := len(lines[i]); {
		case n > longest:
			width, longest = longest, n
		case n > width:
			width = n
		}
	}
	var sb strings.Builder
	for i, g := range projectGates {
		sb.WriteString(lines[i])
		if pad := width - len(lines[i]); pad > 0 {
			sb.WriteString(strings.Repeat(" ", pad))
		}
		sb.WriteString("   # " + g.Note + "\n")
	}
	return sb.String()
}

// projectDisplayName is the name generated docs give the project: the app's,
// taken from the .mpr file name, falling back to the folder name when there is
// no .mpr. The folder is the repository's name in a cloud session ("seed",
// "init-bootstrap"), which says nothing about the app.
func projectDisplayName(dir, mprFile string) string {
	if name := strings.TrimSuffix(filepath.Base(mprFile), ".mpr"); mprFile != "" && name != "" {
		return name
	}
	return filepath.Base(dir)
}

func generateClaudeMD(projectName, mprFile string) string {
	mprPath := mprFile
	if mprPath == "" {
		mprPath = "<project>.mpr"
	}

	bt := "`"    // backtick helper
	bt3 := "```" // triple backtick helper

	var sb strings.Builder
	w := func(s string) { sb.WriteString(s) }

	// This file is re-read into EVERY context started in this project, so its
	// size is a per-session tax, not a one-off. It used to carry the MDL
	// command reference, the lint rule table and the skill table — ~4,800 of
	// its ~6,900 tokens — all of which mxcli answers itself, and all of which
	// had drifted (10 of 14 built-in rules listed; "27" Starlark rules against
	// 31 shipped; no layouts, rules, queues or scheduled events at all). #906
	// was the same failure in the skill table, which reached 12 of 68 before
	// anyone noticed.
	//
	// So the rule for anything added here is the brain's own: if a command can
	// answer it, name the command instead. What stays is what no command can
	// say — where things are, which gates to run, and how to behave.

	w("# Mendix Project: " + projectName + "\n\n")
	w("Built with mxcli and MDL (Mendix Definition Language).\n\n")

	// ── Project Brain ───────────────────────────────────────────────
	// The brain's project.md is documented as loaded every session, and this
	// is the only thing that makes that true: routing to it through the skill
	// alone does not, because a skill is triggered by symptom.
	w("## Project Brain — read this first\n\n")
	w("If " + bt + "docs/brain/" + bt + " exists, read " + bt + "docs/brain/project.md" + bt + " before doing anything\n")
	w("else. It holds the decisions this project has already made — things no command can\n")
	w("tell you, and that are cheap to contradict by accident.\n\n")
	w("Then, depending on what you are doing:\n\n")
	w("- **Building in a module** — also read " + bt + "docs/brain/modules/<Module>.md" + bt + " for the\n")
	w("  modules you are about to touch. Not the whole directory; only those.\n")
	w("- **Working a slice** — " + bt + "./mxcli brain brief --slice <name> -p " + mprPath + bt + " emits\n")
	w("  exactly that pack (project + the slice's modules + its plan) as one read.\n")
	w("- **Planning, or picking work up** — run " + bt + "./mxcli brain plan -p " + mprPath + bt + ".\n")
	w("  It reports what is built from the model itself, so it cannot be out of date.\n\n")
	w("Record what you learn with " + bt + "./mxcli brain capture" + bt + ". Read\n")
	w(bt + ".ai-context/skills/project-brain/SKILL.md" + bt + " for what belongs there and what does not —\n")
	w("the short version is that anything mxcli can answer must never be written down.\n\n")

	// ── Communication Style ─────────────────────────────────────────
	w("## Communication Style\n\n")
	w("- **Never show raw MDL in chat.** Describe changes in plain language as a numbered list.\n")
	w("- After the user approves, write the MDL to a script file, validate it, and execute it silently.\n")
	w("- Show MDL only if the user asks to see the script.\n")
	w("- Report results as plain language, not as a diff.\n\n")

	// ── Running mxcli ───────────────────────────────────────────────
	w("## Running mxcli\n\n")
	w("The binary is in the **root of this project**, not on " + bt + "PATH" + bt + " — always " + bt + "./mxcli" + bt + ".\n\n")
	w(bt3 + "bash\n")
	w("./mxcli -p " + mprPath + " -c \"DESCRIBE STRUCTURE\"   # one command\n")
	w("./mxcli exec script.mdl -p " + mprPath + "        # a script\n")
	w(bt3 + "\n\n")
	w("Scripts live in " + bt + "mdlsource/" + bt + ", one file per concern.\n")
	// The language header (decision 4 on ako/mxcli#714): a script written now
	// is mdl 1, and a headerless one is the older mdl 0, whose statements can
	// mean something else. Mixing the two in one file is the failure to
	// prevent, so the upgrade comes before the edit.
	w("**Each starts with " + bt + "mdl 1;" + bt + ".** A headerless one is " + bt + "mdl 0" + bt + ": upgrade it first,\n")
	w(bt + "./mxcli fmt --upgrade --header -p " + mprPath + " -w <file>" + bt + ", then\n")
	w(bt + "check --deprecations=error --v1=error" + bt + " it; never mix them in a file.\n")
	w("Exec a change twice: the second run must write nothing (unchanged).\n\n")

	// ── The gates ───────────────────────────────────────────────────
	// Ordered cheapest-first on purpose: each one is only worth paying for
	// once the one above it is clean.
	//
	// The list is projectGates rather than literal lines because the same set
	// has to appear in the bootstrap procedure (the skill) and on the docs
	// page that describes it. Three hand-kept copies is how `test` came to be
	// in two of them and absent from the one that is re-read every session.
	w("## The gates, in order\n\n")
	w("Run them cheapest-first; each is only worth paying for once the one above is clean.\n")
	w("**They are the definition of done, not a menu** — a change is finished when they have\n")
	w("all been run and you have said what each one reported.\n\n")
	w("**Once per change, not per edit** — a change being a coherent unit of work, not a\n")
	w("statement and not a file write. Iterate with " + bt + "exec" + bt + ", then run the gates once over the\n")
	w("result: the whole list after every edit proves nothing the one run at the end does not.\n\n")
	w(bt3 + "bash\n")
	w(renderProjectGates(mprPath))
	w(bt3 + "\n\n")
	w("**" + bt + "lint" + bt + " printing no errors is not a pass** — read the warning count, and read\n")
	w(bt + "report" + bt + "'s score. A green " + bt + "check" + bt + " proves nothing about behaviour or about how\n")
	w("a page renders: logic needs " + bt + "test" + bt + ", and anything visual needs the app actually running.\n\n")
	w("Set " + bt + "mx" + bt + " up once with " + bt + "./mxcli setup mxbuild -p " + mprPath + bt + ". To call it directly,\n")
	w("name the version — " + bt + "~/.mxcli/mxbuild/<version>/modeler/mx" + bt + " — because a " + bt + "*" + bt + " glob\n")
	w("breaks the moment two are cached.\n\n")

	// ── The reference is the tool ───────────────────────────────────
	w("## Syntax, rules and skills — ask the tool, not this file\n\n")
	w("These change every release, and a copy that disagrees with the tool is worse than none.\n\n")
	w("| To find out | Run |\n")
	w("|---|---|\n")
	w("| How to write any MDL — **before any skill** | " + bt + "./mxcli syntax" + bt + " → " + bt + "./mxcli syntax <topic> [sub]" + bt + " |\n")
	w("| Which lint rules exist | " + bt + "./mxcli lint -p " + mprPath + " --list-rules" + bt + " |\n")
	w("| What a command takes | " + bt + "./mxcli help <command>" + bt + " |\n")
	w("| What this project contains | " + bt + "./mxcli -p " + mprPath + " -c \"DESCRIBE STRUCTURE\"" + bt + " |\n")
	w("| Why it was built this way | " + bt + "docs/brain/" + bt + " (above) |\n")
	w("\n")
	w("**Skills** are in " + bt + ".ai-context/skills/<name>/SKILL.md" + bt + " (and " + bt + ".claude/skills/" + bt + ", which is\n")
	w("the path Claude Code scans). Each one's frontmatter " + bt + "description" + bt + " says when to reach for\n")
	w("it — that IS the index, so list the directory rather than looking for a table.\n\n")
	// Measured on the BENCH-001 baseline (2026-10-10): reading the skills was
	// ~68% of the session's re-read cost against ~31% of its calls, and one
	// `cat write-microflows/SKILL.md` (7.1k tokens, re-read 43 times) cost more
	// than every mxcli invocation in the costliest ten put together. The same
	// answers cost 0.1-0.6k through `syntax`. Both paths existed; the line here
	// said only "read the matching skill", so it read whole files.
	//
	// The routing itself rides the table row above rather than a paragraph of
	// its own: this file is re-read every session, so prose about saving tokens
	// that costs 500 bytes is self-defeating. What is left is the one thing the
	// table cannot say.
	w("**Read a skill's section, not the file** — each opens with a line-numbered index.\n\n")

	// ── The non-derivable conventions ───────────────────────────────
	// Short, and each one is here precisely because no command reports it.
	w("## Conventions no command will tell you\n\n")
	w("- **Quote every identifier** in MDL — " + bt + "Module.\"Customer\"" + bt + ", " + bt + "\"Status\": String(50)" + bt + ".\n")
	w("  Quotes are stripped, so it is safe and sidesteps every parser keyword.\n")
	w("  It does **not** exempt names Mendix itself reserves (" + bt + "Type" + bt + ", " + bt + "ID" + bt + ", " + bt + "CreatedDate" + bt + ") —\n")
	w("  those are rejected quoted or not.\n")
	w("- **A business process with human steps is a " + bt + "WORKFLOW" + bt + "**, not a status attribute\n")
	w("  plus microflows — you get the user-task inbox, assignment, timers and a definition\n")
	w("  the business can read. " + bt + "mxcli syntax workflow" + bt + ", skill " + bt + "write-workflows" + bt + ".\n")
	w("- **An aggregation is a " + bt + "VIEW ENTITY" + bt + "** (OQL, Mendix 10.18+) — not a microflow that\n")
	w("  retrieves rows and counts them. The database does the work instead of pulling every\n")
	w("  object into memory. " + bt + "mxcli syntax view-entity" + bt + ", skill " + bt + "write-oql-queries" + bt + ".\n")
	w("- **A " + bt + "/** ... */" + bt + " comment before a statement sets that element's documentation.**\n")
	w("- **" + bt + "@Position(x, y)" + bt + " is optional** — mxcli places microflow activities, and\n")
	w("  " + bt + "./mxcli layout" + bt + " arranges the domain model.\n\n")

	// ── Finishing a change ──────────────────────────────────────────
	// The bootstrap created README.md, FINDINGS.md and the plan; without this
	// they are written once and then quietly stop being true, and the
	// bootstrap procedure and the steady state disagree about what a finished
	// change looks like.
	w("## Finishing a change\n\n")
	w("Bootstrap set these up; they are only worth having if every change keeps them.\n\n")
	w("1. **Run the gates above, in order**, and say what each reported — not \"it builds\".\n")
	w("2. **Capture requirements as they arrive** — " + bt + "./mxcli brain capture \"<requirement>\"\n")
	w("   --slice <slice> -a @Mod.Thing" + bt + ". Never tick anything off: " + bt + "brain plan" + bt + " derives what\n")
	w("   is built from the model, so finishing the work is what moves the number.\n")
	w("3. **Append to " + bt + "FINDINGS.md" + bt + "** — anything surprising or broken, and how you verified it.\n")
	w("4. **Commit.** An idle session is reaped and its container goes with it; whatever is\n")
	w("   uncommitted is gone, including the model.\n\n")

	return sb.String()
}
