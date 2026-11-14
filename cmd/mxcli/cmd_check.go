// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/scriptdiff"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check <file|-> [file...]",
	Short: "Check an MDL script for errors without executing it",
	Long: `Check an MDL script file for syntax errors and optionally validate references.

Without a project it checks syntax and the semantic rules that need no model.
With a project it also resolves every reference — modules, entities, pages,
microflows and icons — against that project; --references is implied by a
project and is kept only for compatibility.

The project is the one passed with -p or, without -p, the single .mpr in the
current directory, auto-detected as for every mxcli command (the "Using
project:" line says which). Run from a project directory, a plain
"mxcli check script.mdl" therefore checks references too. Pass --no-project to
check syntax and the no-model rules only, wherever it is run.

It also resolves MEMBER names, not just the entity they belong to: an attribute
named in a create/change activity is looked up on that entity and its
generalizations, so a typo is reported here rather than as CE1613 at the far end
of a build. This needs the target's entity to be known, which it is for a create
(the entity is in the statement) and for a change on a parameter, a database
retrieve, an association retrieve, or a loop over one of those. A variable bound
by some other activity is left unchecked rather than guessed at.

The same applies inside widgets: a page's XPath constraint has every step
resolved against the entity it filters, and a template parameter (ContentParams
/ CaptionParams) rooted in a variable is reported with no project at all.

Expression kinds are checked where the position declares one: a bare word as a
create/change member's value (Mendix expressions have no bare identifiers), and
a log message's template parameter, which must be a String — Mendix does not
coerce there, so wrap a non-String one in toString(...).

Reference validation is smart: it automatically skips references to objects
that are created within the script itself. For example, if your script creates
a module "MyModule" and then creates entities in it, no error will be reported
for the module reference.

Creation ORDER is checked separately, and without a project. The executor
resolves most references when it writes the referring document, so naming
something a later statement creates fails partway through "mxcli exec" — with
earlier statements already written, since exec is not transactional. Those are
reported as MDL-ORDER01 (and MDL-PAGE01 for a widget's page reference).

Given a project it also type-checks the expressions in the script's microflows
and nanoflows: comparing an enumeration attribute to a string literal (in a
create/change member, or in a condition such as: if $obj/Status = 'Open'),
operand and argument type mismatches, and the like. Attribute paths resolve
through associations too, so $Order/Sales.Order_Customer/Name is typed.
These need the project to answer what an attribute's type is and which values an
enumeration has, which is why they need -p. They report under exprcheck's own
E0xx codes, and — like every other check here — only an error severity fails the
run.

Given a project it also predicts what exec would refuse. A "create or modify"
of a microflow or nanoflow the project holds is applied as a patch of the stored
flow; a change the patch cannot make (inside a loop body, a redrawn connector, a
return in an inserted fragment, ...) is refused by exec under "mdl 1;", and
check reports it as an MDL-V1-REBUILD error with exec's message. Under mdl 0
exec rebuilds the whole flow instead, and check reports the MDL-V1-REBUILD
warning exec prints. An "alter microflow|nanoflow" exec would refuse is MDL090.

Given a project it also applies exec's version gates: a statement the project's
Mendix version cannot store (create view entity below 10.18, send email below
11.13, ...) is MDL-VERSION01, with the message exec refuses it with. See
"show features" for the floors.
This is the verdict "mxcli diff" reports as "Refused:", computed by the same
code. A statement on a flow an earlier statement of the script changes, or
whose flow does not build until an earlier statement has run, is not predicted.

Several files are checked as one script set, run in the order given, as
"mxcli exec" would run them one after the other. With a project, each file is
resolved against a scratch copy of it that the files before it have been
applied to, so a name an earlier file creates resolves in a later one; a file
that does not pass is left out of the copy, as exec's pre-flight would refuse
it and write nothing. The project itself is not changed. A test file
(.test.mdl, .test.md) can be one of the files: it creates nothing itself, so
when its tests call a microflow a setup script creates, check the two together,
the setup script first — checked alone, the test file reports the microflow as
not found. The set is also first
read as a whole for two "create or modify" statements declaring the same flow — a placeholder
("stub") followed by the real flow. That is reported as an MDL-STUB01 warning
naming both statements: run in order, the stub replaces the real flow before
the real statement restores it, on every run, and with the two under different
language headers the real statement can be refused while the stub's rebuild
goes through. Since #843 a self-recursive flow is created in one statement, so
the stub can be dropped. One file alone is not read this way.

Output includes structured rule IDs (MDL prefix for reference and script rules,
E0xx for expression type rules) for each validation issue.

A deprecated MDL spelling — an alias left over from consolidating MDL onto one
canonical form, such as "create or replace" for "create or modify" or "show" for
"list" — is reported as an MDL-DEPRnnn warning naming the canonical form. Pass
--deprecations=error to fail the run on one instead, e.g. in CI over docs.

A headerless script is mdl 0, and every construct in it that means something
else, or is refused, under "mdl 1;" is reported as an MDL-V1-* warning
(MDL-V1-SLASH, MDL-V1-SEMI, MDL-V1-LIMIT1, MDL-V1-QUOTEDEXPR, the MDL-V1-REBUILD
warning form, ...). Pass --v1=error to fail the run on one. The two flags gate
different things: a script can pass --deprecations=error and still not parse
under mdl 1, so --deprecations=error --v1=error together is the full mdl 1
readiness gate. "mxcli fmt --upgrade --header" is the fix for both. A script
already headed "mdl 1;" reports no MDL-V1-* warning.

Use --post-migration to scan an existing project (independent of the script)
for legacy native widgets that have pluggable replacements — Studio Pro does
not auto-migrate these on a Mendix major-version upgrade.

Examples:
  # Check syntax only, even in a directory holding an .mpr
  mxcli check script.mdl --no-project

  # Check syntax, resolve references, and type-check expressions
  mxcli check script.mdl -p app.mpr

  # The same, against the .mpr auto-detected in the current directory
  mxcli check script.mdl

  # Scan the project for legacy native widgets after a Mendix upgrade
  mxcli check script.mdl -p app.mpr --post-migration

  # Output as JSON or SARIF: one document on stdout covering every phase
  # (progress goes to stderr); the exit code still says whether it passed
  mxcli check script.mdl --format json
  mxcli check script.mdl -p app.mpr --format sarif > results.sarif

  # Gate a headerless script on being ready for "mdl 1;"
  mxcli check script.mdl -p app.mpr --deprecations=error --v1=error

  # Check a script set, run in this order
  mxcli check 10-domain.mdl 20-flows.mdl 30-pages.mdl -p app.mpr

  # Check a test file against what its setup script creates
  mxcli check setup.mdl tests/orders.test.mdl -p app.mpr

  # Read the script from stdin
  cat script.mdl | mxcli check -
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkNoProjectConflicts(cmd); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		if code := runCheckFiles(cmd, args); code != 0 {
			os.Exit(code)
		}
	},
}

// checkNoProjectConflicts refuses --no-project together with a flag that asks
// for a project, rather than letting one of the two silently win. -p is only
// "changed" when the user gave it: auto-detection does not run under
// --no-project, so it cannot set it.
func checkNoProjectConflicts(cmd *cobra.Command) error {
	if !skipsProjectDiscovery(cmd) {
		return nil
	}
	for _, name := range []string{"project", "references", "post-migration"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s asks for a project and --%s for none; pass one or the other", name, noProjectFlag)
		}
	}
	return nil
}

// runCheckFile checks one script and returns the exit code: 0 when it passed.
func runCheckFile(cmd *cobra.Command, filePath string) int {
	return runCheckFileAgainst(cmd, filePath, "")
}

// runCheckFileAgainst is runCheckFile resolving against the project at
// against instead of --project — a script set's scratch copy, holding what
// the files before this one write — while still naming --project in its
// report. "" means --project itself.
func runCheckFileAgainst(cmd *cobra.Command, filePath, against string) int {
	projectPath, _ := cmd.Flags().GetString("project")
	shownProject := projectPath
	if against != "" && projectPath != "" {
		projectPath = against
	}
	// A project makes reference resolution possible, so it runs. It used to
	// need --references as well, which meant `mxcli check script.mdl -p
	// app.mpr` printed an unqualified "Check passed!" having resolved
	// nothing — icons, entity and page references all silently unchecked.
	// Someone who hands the command a project has said what they want; the
	// flag stays accepted so existing invocations and scripts keep working.
	checkRefs, _ := cmd.Flags().GetBool("references")
	checkRefs = checkRefs || projectPath != ""
	postMigration, _ := cmd.Flags().GetBool("post-migration")
	depPolicy := deprecationPolicy(cmd)
	v1 := v1Policy(cmd)
	format := resolveFormat(cmd, "text")
	isStructured := format != "" && format != "text"

	outputFormat := linter.OutputFormat(format)
	formatter := linter.GetFormatter(outputFormat, !isStructured)
	// The text report runs in tiers (semantic checks, references, project
	// verdicts, legacy widgets) and each printed its own "N issues" line, so
	// a warning in one tier and an error in another read as two separate
	// counts with a "✓" between them, and neither was the total. The tiers
	// now print their violations and finish prints one summary over all.
	var printed []linter.Violation
	if tf, ok := formatter.(*linter.TextFormatter); ok {
		tf.NoSummary = true
	}
	report := func(vs []linter.Violation) {
		formatter.Format(vs, os.Stderr)
		printed = append(printed, vs...)
	}

	// In a structured format the payload is ONE document on stdout, emitted
	// once at the end (or at the first failing phase). Each phase used to
	// format its own violations to stderr, so `check --format json` put
	// nothing parseable on stdout — only the executor's "Connected to:"
	// chatter — and a run reaching several phases wrote several documents.
	var structured []linter.Violation
	finish := func(code int) int {
		if isStructured {
			formatter.Format(structured, os.Stdout)
		} else if len(printed) > 0 {
			fmt.Fprintln(os.Stderr)
			linter.WriteSummary(os.Stderr, printed, 0)
		}
		return code
	}

	// Read the script (a path, or "-" for stdin)
	content, err := readMDLSource(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		return 1
	}

	// Parse the script
	if !isStructured {
		fmt.Printf("Checking syntax: %s\n", mdlSourceLabel(filePath))
	}

	// A .test.mdl / .test.md file is not top-level MDL: each block is a
	// microflow body. Render it as the microflows it becomes, on the source's
	// own lines, so every rule below applies to what the author actually wrote
	// (mendixlabs/mxcli#1103).
	source := string(content)
	var testProblems []linter.Violation
	if testrunner.IsTestFile(filePath) {
		checked, terr := testrunner.CheckSource(source, filePath)
		if terr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", terr)
			return 1
		}
		source = checked.MDL
		// A rendering with nothing in it means the file declares no @test
		// block. That is not #618's "the parser could not begin reading it"
		// — the parser was handed an empty rendering, not the author's text
		// — so it gets its own message rather than one quoting a line that
		// was never parsed.
		if strings.TrimSpace(source) == "" && strings.TrimSpace(string(content)) != "" {
			fmt.Fprintln(os.Stderr, noTestsDeclaredError(mdlSourceLabel(filePath)))
			return 1
		}
		for _, p := range checked.Problems {
			testProblems = append(testProblems, linter.Violation{
				RuleID:   "MDL-TEST01",
				Severity: linter.SeverityError,
				Message:  fmt.Sprintf("test %q: %s", p.Test, p.Message),
				Location: linter.Location{DocumentType: "test", DocumentName: p.Test},
			})
		}
	}

	prog, errs := visitor.Build(source)
	if len(errs) > 0 {
		if isStructured {
			var parseViolations []linter.Violation
			for _, parseErr := range errs {
				parseViolations = append(parseViolations, linter.Violation{
					RuleID:   "MDL-SYNTAX",
					Severity: linter.SeverityError,
					Message:  parseErr.Error(),
				})
			}
			structured = append(structured, parseViolations...)
			return finish(1)
		} else {
			fmt.Fprintf(os.Stderr, "Syntax errors found:\n")
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "  - %v\n", err)
			}
			// Hint: if script contains IMPORT/QUERY with single $ but not $$, suggest dollar-quoting
			src := source
			if (strings.Contains(src, "IMPORT") || strings.Contains(src, "import")) &&
				(strings.Contains(src, "QUERY") || strings.Contains(src, "query")) &&
				strings.Contains(src, "$") && !strings.Contains(src, "$$") {
				fmt.Fprintf(os.Stderr, "\nHint: SQL queries in IMPORT statements should use dollar-quoting ($$...$$) instead of single quotes.\n")
				fmt.Fprintf(os.Stderr, "  Example: IMPORT FROM alias QUERY $$SELECT * FROM table$$ INTO Module.Entity MAP (...)\n")
			}
		}
		return 1
	}
	// Zero statements from non-empty input is not an empty script: the parser
	// never got into the file. Both gates refuse it (ako/mxcli#618).
	//
	// `source`, not `content`: for a test file they differ, and the message
	// names a line from whichever text the parser was actually given.
	if line, bad := unparsableInput(source, len(prog.Statements)); bad {
		fmt.Fprintln(os.Stderr, unparsableInputError(filePath, line))
		return 1
	}
	if !isStructured {
		fmt.Printf("✓ Syntax OK (%d statements)\n", len(prog.Statements))
	}

	// Every semantic check lives in executor.ValidateProgram, so `mxcli exec`
	// refuses exactly what `mxcli check` reports. Adding a check there gives
	// both commands it at once.
	violations := append(testProblems, executor.ValidateProgram(prog, projectPath)...)

	// With a project, connect before reporting: two semantic rules read the
	// stored model, so `check -p` reports what `exec -p` would (ako/mxcli#943).
	//   - MDL067 is dropped for a flow already stored the way the script
	//     commits — the filter exec's preflight applies.
	//   - MDL-WORKFLOW10 reads a called microflow the script does not create.
	// A test endpoint left in the project by a run that never removed it would
	// be committed with the script's changes; say so while it can still be undone.
	if projectPath != "" && !isStructured {
		if msg := testrunner.LeftoverEndpointWarning(projectPath); msg != "" {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)
		}
	}

	var exec *executor.Executor
	if projectPath != "" {
		e, logger := newLoggedExecutorTo("check", progressSink(format))
		defer logger.Close()
		defer e.Close()
		// check cannot know how the script will run, so it accepts a call to
		// a flow created further down, as plain `exec` (which rolls back)
		// does; exec refuses it in the modes that cannot (#1405).
		e.SetForwardFlowCalls(true)
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := e.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				return 1
			}
		}
		exec = e
		// check writes nothing, and its tiers each list the project's flows
		// and pages: one span over all of them, so a unit is read once per
		// run rather than once per tier (ako/mxcli#2160). The tiers' own
		// spans nest inside it. Released before the deferred Close.
		defer e.CacheUnitReads()()
		if b := exec.Backend(); b != nil {
			violations = executor.DropSettledCommitNotes(violations, prog, executor.NewStoredCommitEvents(b))
			// MDL004 on a void microflow's end-event value is a warning for a flow
			// already stored that way: re-applying it changes nothing.
			violations = executor.SettleStoredVoidReturnValues(violations, prog, executor.NewStoredVoidReturns(b))
			violations = append(violations, executor.StoredTaskClaimViolations(prog, b)...)
			violations = append(violations, executor.StoredTaskPageOutcomeViolations(prog, b)...)
			// MDL-VERSION01: a statement the project's Mendix version cannot
			// store. The gates are the ones exec's handlers refuse with, from
			// one table (executor.statementVersionGates), so check -p no
			// longer passes a script exec refuses for the version (#1434).
			violations = append(violations, exec.CheckVersionGates(prog)...)
		}
	}
	violations = executor.ApplyDeprecationPolicy(violations, depPolicy)
	violations = executor.ApplyV1Policy(violations, v1)

	if isStructured {
		// Always emit structured output (even when clean)
		structured = append(structured, violations...)
	} else if len(violations) > 0 {
		fmt.Fprintln(os.Stderr)
		report(violations)
	}

	// A semantic error fails the run, but does not hide the reference tier:
	// an unresolved name is a separate error mxbuild reports alongside it, and
	// stopping here made `check --references` look as if it had accepted a
	// call to a parameter the action does not have (CE1613, ako/mxcli#953).
	// The catalog-backed tier after it still waits for a clean script.
	semanticErrors := len(violations) > 0 && linter.Summarize(violations).Errors > 0
	if semanticErrors && !checkRefs {
		return finish(1)
	}

	// If reference checking requested
	if checkRefs {
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for reference checking")
			return 1
		}

		if !isStructured {
			fmt.Printf("\nValidating references against: %s\n", shownProject)
			fmt.Printf("(Note: References to objects created within the script are skipped)\n")
		}
		// exec was connected above, before the semantic report.

		// A test file's microflows live in the runner's MxTest module, which
		// the runner creates first and the project does not have
		// (ako/mxcli#677).
		refProg := prog
		if testrunner.IsTestFile(filePath) {
			refProg = testrunner.WithRunnerModule(prog)
		}

		// Validate the program (considers objects defined within the script)
		validationErrors, refWarnings := exec.ValidateProgramWithWarnings(refProg)

		// Check for project conflicts: plain CREATE where the document already exists
		validationErrors = append(validationErrors, exec.CheckProjectConflicts(refProg)...)

		// Unresolved references in EXCLUDED documents: reported, never
		// failing the run — Mendix does not validate excluded documents.
		// In structured mode they join the error list (one document, not two)
		// or are emitted on their own when there is nothing else.
		var warnViolations []linter.Violation
		for _, w := range refWarnings {
			warnViolations = append(warnViolations, linter.Violation{
				RuleID:   "MDL-REF",
				Severity: linter.SeverityWarning,
				Message:  w,
			})
		}
		if len(refWarnings) > 0 && !isStructured {
			fmt.Fprintf(os.Stderr, "Reference warnings:\n")
			for _, w := range refWarnings {
				fmt.Fprintf(os.Stderr, "  %s\n", w)
			}
			// Printed here, not through report, but part of the total the
			// closing summary gives (ako/mxcli#1928).
			printed = append(printed, warnViolations...)
		} else if len(warnViolations) > 0 && len(validationErrors) == 0 {
			structured = append(structured, warnViolations...)
		}

		if len(validationErrors) > 0 {
			if isStructured {
				refViolations := warnViolations
				for _, err := range validationErrors {
					refViolations = append(refViolations, linter.Violation{
						RuleID:   "MDL-REF",
						Severity: linter.SeverityError,
						Message:  err.Error(),
					})
				}
				structured = append(structured, refViolations...)
			} else {
				fmt.Fprintf(os.Stderr, "Reference errors:\n")
				for _, err := range validationErrors {
					fmt.Fprintf(os.Stderr, "  %v\n", err)
				}
				fmt.Fprintf(os.Stderr, "\n✗ %d reference error(s) found\n", len(validationErrors))
				// The closing summary is the run's total; it read "0 errors"
				// after this line, because these are printed here rather than
				// through report (ako/mxcli#1928).
				for _, err := range validationErrors {
					printed = append(printed, linter.Violation{
						RuleID:   "MDL-REF",
						Severity: linter.SeverityError,
						Message:  err.Error(),
					})
				}
				if hint := testFileMissingTargetHint(filePath, validationErrors); hint != "" {
					fmt.Fprint(os.Stderr, hint)
				}
			}
			// The flow verdicts still run (ako/mxcli#2014): they are what exec
			// would refuse, and stopping here hid them behind the reference
			// error — fixed, re-checked, and only then the next problem, or an
			// `exec --no-check` that ran far and rolled back. The rest of the
			// project tier stays skipped: a name that does not resolve mostly
			// makes the type checks repeat the reference error.
			if verdicts := executor.ApplyV1Policy(exec.CheckFlowVerdictsReport(prog).Violations, v1); len(verdicts) > 0 {
				if isStructured {
					structured = append(structured, verdicts...)
				} else {
					fmt.Fprintln(os.Stderr)
					report(verdicts)
				}
			}
			return finish(1)
		}
		if !isStructured {
			fmt.Printf("✓ All references valid\n")
		}
		if semanticErrors {
			return finish(1)
		}

		// The catalog-backed tier: the checks whose answers only exist once a
		// project is connected. It runs after the reference check because a
		// script naming things that do not exist has a more basic problem
		// than a mistyped operand — and because building the catalog for a
		// run that already failed is wasted work.
		//
		// Like every other violation this command emits, only an error
		// severity fails the run. Warnings and hints are advice, and a
		// checker whose first outing turns advice into a broken build is a
		// checker people turn off.
		//
		// MDL087 is what this script REMOVES from the project, which nothing
		// reported until now (ako/mxcli#562). `create or modify entity`
		// rebuilds the entity from the statement, so a member the script does
		// not restate is deleted — and the loss only becomes visible slices
		// later, as a CE1613 on whatever still binds it. exec prints the same
		// list, but only as it applies the statement; by then it is gone.
		//
		// Expression type checking is the other half: the rules that need an
		// attribute's type, an enumeration's cases or a microflow's return
		// type. The scope-local tier already ran in the unconditional pass.
		//
		// The flow verdicts are what exec would refuse (ako/mxcli#876): a
		// `create or modify` of a stored flow whose change the splice cannot
		// make is refused under mdl 1 and rebuilt with MDL-V1-REBUILD under
		// mdl 0, and an alter whose patch fails is refused under both. They
		// run the verdict exec and diff run, so the three agree.
		projectViolations := exec.CheckEntityMemberDrops(prog)
		// MDL-ODATA10: a call external action argument whose stored Associations
		// to include a create or modify keeps because the statement does not
		// write the clause (ako/mxcli#1914) — the omitted-clause warning exec
		// prints, said before the write.
		projectViolations = append(projectViolations, exec.CheckCarriedIncludedAssociations(prog)...)
		projectViolations = append(projectViolations, exec.TypeCheckProgram(prog)...)
		flowReport := exec.CheckFlowVerdictsReport(prog)
		projectViolations = append(projectViolations, flowReport.Violations...)
		// MDL-SEC21 (MxBuild CE0106): a flow this script leaves with no allowed
		// role while a page, snippet, nanoflow, menu or navigation uses it. The
		// reported shape was drop + create in separate runs, which loses the
		// roles that `create or modify` keeps.
		projectViolations = append(projectViolations, exec.CheckFlowAccess(prog)...)
		// MDL-SEC22 (MxBuild CE0114): a microflow that does not apply entity
		// access calling a microflow or rule that does (mendixlabs/mxcli#1500).
		// A consistency rule between two documents, not a reference, so the
		// reference check above cannot see it.
		projectViolations = append(projectViolations, exec.CheckEntityAccessCalls(prog)...)
		// MDL-I18N01 (MxBuild CE4899): a script that changes the default
		// language leaves every required caption written in the old one empty
		// in the new one (ako/mxcli#944).
		projectViolations = append(projectViolations, exec.CheckDefaultLanguageCaptions(prog)...)
		// --v1=error: the MDL-V1-REBUILD warning form (a headerless script's
		// flow change exec rebuilds) fails the run like any other MDL-V1-*.
		projectViolations = executor.ApplyV1Policy(projectViolations, v1)

		if len(projectViolations) > 0 {
			if isStructured {
				structured = append(structured, projectViolations...)
			} else {
				fmt.Fprintln(os.Stderr)
				report(projectViolations)
			}
			if linter.Summarize(projectViolations).Errors > 0 {
				return finish(1)
			}
		} else if !isStructured {
			flows := "no flow change exec would refuse"
			if len(flowReport.Unpredicted) > 0 {
				flows = "no predicted flow change exec would refuse"
			}
			fmt.Printf("✓ Expression types OK, no unstated member drops, %s, no used flow left without access, no entity-access call mismatch\n", flows)
		}
		// A flow change whose verdict depends on what earlier statements
		// create is not predicted; say so rather than let the line above
		// read as covering it.
		if !isStructured && len(flowReport.Unpredicted) > 0 {
			fmt.Fprint(os.Stderr, executor.FormatUnpredictedFlows(flowReport.Unpredicted))
		}
	}

	// Post-migration scan: walk the project for native widgets that
	// have pluggable replacements (Studio Pro does not auto-migrate
	// these on a Mendix major-version upgrade).
	if postMigration {
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for --post-migration")
			return 1
		}
		if !isStructured {
			fmt.Printf("\nScanning project for legacy native widgets: %s\n", shownProject)
		}
		legacyViolations, err := scanLegacyWidgets(projectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning project: %v\n", err)
			return 1
		}
		if isStructured {
			structured = append(structured, legacyViolations...)
		} else if len(legacyViolations) > 0 {
			fmt.Fprintln(os.Stderr)
			report(legacyViolations)
			fmt.Fprintf(os.Stderr, "\n✗ %d legacy widget(s) found\n", len(legacyViolations))
		} else {
			fmt.Printf("✓ No legacy native widgets found\n")
		}
		if len(legacyViolations) > 0 {
			summary := linter.Summarize(legacyViolations)
			if summary.Errors > 0 {
				return finish(1)
			}
		}
	}

	finish(0)
	if !isStructured {
		fmt.Println("\nCheck passed!")
		// Qualify the verdict when nothing was resolved against a model. A
		// bare "Check passed!" reads as more than it is: without a project
		// no icon, entity, page or microflow name in the script has been
		// looked up, and those are exactly what this command gets reached
		// for. Saying so beats leaving the reader to infer it.
		if !checkRefs {
			fmt.Println("  (no project given — icon, entity, page and microflow references were")
			fmt.Println("   not resolved; re-run with -p <project.mpr> for full coverage)")
		}
	}
	return 0
}

// runCheckFiles checks each script of a run and returns the worst exit code.
// One file is checked exactly as before. Several files are a script set run
// in order (ako/mxcli#905): before the files are checked one by one, the set
// is read as a whole for what no single file shows — a flow two of its
// `create or modify` statements declare, the stub-then-real pattern
// (MDL-STUB01, a warning).
func runCheckFiles(cmd *cobra.Command, files []string) int {
	if len(files) == 1 {
		return runCheckFile(cmd, files[0])
	}
	for _, f := range files {
		if f == stdinPath {
			fmt.Fprintln(os.Stderr, "Error: '-' (stdin) checks one script; it cannot be one of several files")
			return 1
		}
	}
	if format := resolveFormat(cmd, "text"); format != "" && format != "text" {
		fmt.Fprintf(os.Stderr, "Error: --format %s writes one document for one script; check the files one at a time\n", format)
		return 1
	}
	if v := stubThenRealViolations(findFlowRedeclarations(parseScriptSet(files))); len(v) > 0 {
		fmt.Printf("Checking the script set: %d files\n", len(files))
		linter.GetFormatter(linter.OutputFormat("text"), true).Format(v, os.Stderr)
		fmt.Fprintln(os.Stderr)
	}
	// Against a project, each file is resolved against a scratch copy that the
	// files before it have been applied to — what exec, run file by file in
	// this order, leaves for it (mendixlabs/mxcli#1355). Checked against the
	// project itself, every reference to an earlier file's entity, page or
	// module was reported as missing, so a valid set failed its pre-flight.
	var scratch *scriptdiff.Scratch
	if projectPath, _ := cmd.Flags().GetString("project"); projectPath != "" {
		sc, err := scriptdiff.NewScratch(projectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: copy the project to check the script set against: %v\n", err)
			return 1
		}
		defer sc.Close()
		scratch = sc
	}
	worst := 0
	var notApplied, failed []string
	for i, f := range files {
		if i > 0 {
			fmt.Println()
		}
		against := ""
		if scratch != nil {
			against = scratch.Mpr
			if i > 0 {
				fmt.Printf("Checking %s against a scratch copy of the project holding what the %d file(s) before it write (the project itself is not changed)\n", f, i)
				for _, n := range notApplied {
					fmt.Printf("  (without %s)\n", n)
				}
			}
		}
		code := runCheckFileAgainst(cmd, f, against)
		if code > worst {
			worst = code
		}
		if code != 0 {
			failed = append(failed, f)
		}
		if scratch != nil && i < len(files)-1 {
			if why := applyToScratch(scratch, f, code); why != "" {
				notApplied = append(notApplied, why)
			}
		}
	}
	// Each file ends on its own verdict, so without this the run's last line
	// was the last file's "Check passed!" even after an earlier file failed
	// (mendixlabs/mxcli#1431). Close on the verdict for the whole set.
	fmt.Println()
	if len(failed) > 0 {
		fmt.Printf("%d of %d files failed: %s\n", len(failed), len(files), strings.Join(failed, ", "))
	} else {
		fmt.Printf("All %d files passed\n", len(files))
	}
	return worst
}

// applyToScratch runs one checked file of a script set against the scratch
// copy, so the files after it resolve against what it writes. It returns ""
// when the whole file was applied, and otherwise what was left out and why.
//
// A file that did not pass is not applied: exec's pre-flight would refuse it
// and write nothing, so the files after it are checked as exec would run them
// — without it. A test file is not applied either: running it runs its tests.
func applyToScratch(scratch *scriptdiff.Scratch, file string, checkCode int) string {
	if checkCode != 0 {
		return file + ", which did not pass: exec would refuse it and write nothing"
	}
	if testrunner.IsTestFile(file) {
		return file + ", a test file: it creates nothing for the files after it"
	}
	data, err := readMDLSource(file)
	if err != nil {
		return fmt.Sprintf("%s: %v", file, err)
	}
	prog, errs := visitor.Build(string(data))
	if len(errs) > 0 {
		return fmt.Sprintf("%s: %v", file, errs[0])
	}
	opts := scriptdiff.Options{
		// The file engine whatever --mcp says: the file is executed for real,
		// and only the scratch copy may receive it.
		NewBackend: func() backend.FullBackend { return modelsdkbackend.New() },
	}
	if abs, err := filepath.Abs(file); err == nil {
		opts.ScriptDir = filepath.Dir(abs)
	}
	if _, err := scratch.Apply(prog, opts); err != nil {
		return fmt.Sprintf("what %s writes after: %v", file, err)
	}
	return ""
}

// testFileMissingTargetHint is the line under a test file's "not found"
// reference errors that names the way to check it (ako/mxcli#1514).
//
// A test file creates nothing of its own — applyToScratch does not even run
// it — so whatever its tests call has to be in the project already or come
// from a script. Check resolves a later file against what the files before it
// write, but a lone "microflow not found" gave no sign that this applies to
// test files; the user had to know the script-set form existed.
func testFileMissingTargetHint(filePath string, errs []error) string {
	if !testrunner.IsTestFile(filePath) {
		return ""
	}
	for _, err := range errs {
		if strings.Contains(err.Error(), "not found") {
			return fmt.Sprintf("  Hint: a test file creates nothing itself, so what its tests use must already be in\n"+
				"  the project. If a script creates it, check the two together, that script first —\n"+
				"  each file is resolved against what the files before it write:\n"+
				"    mxcli check <setup script> %s -p <project>\n", mdlSourceLabel(filePath))
		}
	}
	return ""
}

// parseScriptSet reads and parses each file for the set-level checks. A file
// that cannot be read or parsed is left out (Prog nil): its own check reports
// that.
func parseScriptSet(files []string) []setScript {
	var out []setScript
	for _, f := range files {
		sc := setScript{Path: f}
		if data, err := readMDLSource(f); err == nil && !testrunner.IsTestFile(f) {
			sc.Source = string(data)
			if prog, errs := visitor.Build(sc.Source); len(errs) == 0 {
				sc.Prog = prog
			}
		}
		out = append(out, sc)
	}
	return out
}
