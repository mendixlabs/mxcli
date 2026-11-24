// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/linter/rules"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Lint a Mendix project for issues",
	Long: `Run linting rules against a Mendix project to find potential issues.

Built-in rules check for:
  - Naming conventions (MPR001) - entities, microflows, pages, enumerations
  - Empty microflows (MPR002) - microflows with no activities
  - Domain model size (MPR003) - max persistent entities per domain model
  - Empty validation feedback (MPR004) - validation feedback with empty message
  - Unconfigured images (MPR005) - IMAGE widgets with no source configured
  - Empty containers (MPR006) - layout containers with no children
  - Navigation page security (MPR007) - pages in navigation need allowed roles, and every user role must be able to open its home page
  - Gallery selection listener (MPR009) - DataView 'DataSource: selection X' needs gallery 'ItemSelectionMode: toggle' (Studio Pro CE3637)
  - React-unsupported widgets (MPR012) - static/dynamic image, drop-down, legacy data grid, reference selector are CE0582 under the React client
  - Entity access rules (SEC001) - persistent entities need access rules
  - Password policy (SEC002) - password minimum length should be 8+
  - Demo users (SEC003) - demo users should be off at Production security
  - Unreadable page data (SEC010) - a data widget's entity must be readable by the roles that open its page (at Production it renders empty)
  - XPath through an unreadable association (SEC011) - a page data source's XPath may cross only associations the roles opening the page can read (at Production the page fails)
  - Reachable page not readable (SEC012) - every page a user role can reach shows only what the role may read (mxbuild CE2729)
  - Security level ignores rules (SEC013) - Off ignores access rules, grants and demo users; Prototype ignores entity XPath constraints
  - Un-describable branch structure (MDL-FLOW01) - decision branches that do not nest, so DESCRIBE cannot render them faithfully

Bundled Starlark rules (in .claude/lint-rules/):
  Security:
  - Guest access enabled (SEC004) - review anonymous user entity access
  - Strict mode disabled (SEC005) - XPath constraint enforcement off
  - PII attributes exposed (SEC006) - PII-sounding attributes need access rules
  - Anonymous unconstrained READ (SEC007) - DIVD-2022-00019 pattern detection
  - PII unconstrained READ (SEC008) - PII entities readable without row scoping
  - Missing member restrictions (SEC009) - large entities without attribute-level access
  Architecture:
  - Cross-module data access (ARCH001) - pages should use same-module entities
  - Data changes through microflows (ARCH002) - enforce microflow-based writes
  - Entity business key (ARCH003) - persistent entities need a unique key
  Quality:
  - McCabe complexity (QUAL001) - microflow cyclomatic complexity threshold
  - Missing documentation (QUAL002) - every document type (modules, entities,
      pages, microflows, workflows, Java/JavaScript actions and their parameters,
      REST services, mappings, constants, ...). Attributes and associations are
      off by default; see the check_* options.
  - Long microflows (QUAL003) - microflows with too many activities
  - Orphaned elements (QUAL004) - unreferenced elements in the project
  Design:
  - Entity attribute count (DESIGN001) - entities with too many attributes
  Convention (Best Practices):
  - Boolean naming (CONV001) - boolean attributes should start with Is/Has/Can/etc.
  - No default values (CONV002) - avoid entity attribute defaults, use microflows
  - Page naming suffix (CONV003) - pages should end with _NewEdit, _View, etc.
  - Enumeration prefix (CONV004) - enumerations should start with ENUM_
  - Snippet prefix (CONV005) - snippets should start with SNIPPET_
  - No create/delete rights (CONV006) - use microflows for create/delete
  - XPath on all access (CONV007) - entity access should have XPath constraints
  - Module role mapping (CONV008) - each module role maps to one user role
  - Max microflow objects (CONV009) - microflows should have <= 15 activities
  - ACT_ microflow content (CONV010) - ACT_ microflows should only contain UI actions
  - No commit in loop (CONV011) - commit actions should not be inside loops
  - Exclusive split caption (CONV012) - exclusive splits need captions
  - Error handling on calls (CONV013) - external calls need custom error handling
  - No continue error handling (CONV014) - avoid silently swallowing errors
  - No validation rules (CONV015) - use microflows instead of entity validation rules
  - No event handlers (CONV016) - avoid entity event handlers
  - No calculated attributes (CONV017) - avoid calculated attributes
  - Created object never persisted (CONV020) - a created object that is never committed, returned or passed on
  - Create/change then commit (CONV021) - a create/change directly followed by a commit of the same object; set Commit on it instead

Custom Starlark rules in .claude/lint-rules/*.star are loaded automatically.

Examples:
  mxcli lint -p app.mpr
  mxcli lint -p app.mpr --format json
  mxcli lint -p app.mpr --format sarif > results.sarif
  mxcli lint -p app.mpr --list-rules
  mxcli lint -p app.mpr -r MPR001
  mxcli lint -p app.mpr -r MPR001 -r SEC001
  mxcli lint -p app.mpr -r MPR001,SEC001
  mxcli lint -p app.mpr -m MyModule
  mxcli lint -p app.mpr -m MyModule -m AnotherModule
  mxcli lint -p app.mpr -d Sales.ACT_Order
  mxcli lint -p app.mpr -d Sales.ACT_Order -r CONV011
  mxcli lint -p app.mpr --baseline lint-baseline.json --update-baseline
  mxcli lint -p app.mpr --baseline lint-baseline.json

--baseline reports only the findings that are not in a recorded baseline, so the
result on a mature app is about the change just made, not the backlog; the exit
code follows the new findings alone. --update-baseline records the current
findings there instead (commit the file). A finding is matched on its rule and
its document, not its message, so rewording a rule does not make its findings
new. A run scoped with --modules, --documents or --rules sees only part of the
project: it reports new findings but not how many were fixed, and cannot update
the baseline.

A baseline also records which rules ran and a fingerprint of the lint
configuration. A later run in which a rule the baseline holds findings for did not
run (the project's .claude/lint-rules/ is missing, the rule is disabled, or it
failed) reports those findings as "not checked", never as fixed, and says which
rules; a changed configuration is reported too. Run lint where the rules and the
config are installed, or re-record with --update-baseline if the change is
intended. mxcli verify turns either into an error (exit 2) instead of a pass.
--update-baseline refuses (exit 2) while a rule cannot run: the baseline would hold
none of its findings. It also refuses when the run has errors the baseline does not
already hold, since recording them turns a failing gate into a passing one without
fixing anything; --accept-errors records them on purpose.

--documents scopes a run to named documents, so a structural question about one
microflow costs a fraction of a project-wide lint and needs no baseline diff to
see what is new. It narrows the document iterators and implies the documents'
modules, and findings that are not about those documents are not reported.
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		format := resolveFormat(cmd, "text")
		useColor, _ := cmd.Flags().GetBool("color")
		listRules, _ := cmd.Flags().GetBool("list-rules")
		excludeModules, _ := cmd.Flags().GetStringSlice("exclude")
		onlyRules, _ := cmd.Flags().GetStringSlice("rules")
		moduleFilter, _ := cmd.Flags().GetStringSlice("modules")
		documentFilter, _ := cmd.Flags().GetStringSlice("documents")
		baselinePath, _ := cmd.Flags().GetString("baseline")
		updateBaseline, _ := cmd.Flags().GetBool("update-baseline")
		acceptErrors, _ := cmd.Flags().GetBool("accept-errors")
		scoped := len(onlyRules) > 0 || len(moduleFilter) > 0 || len(documentFilter) > 0
		if updateBaseline && baselinePath == "" {
			fmt.Fprintln(os.Stderr, "Error: --update-baseline needs --baseline <file> to write to")
			os.Exit(1)
		}
		// A scoped run sees part of the project; recording it would drop every
		// other finding from the baseline, and the next full run would report
		// them all as new.
		if updateBaseline && scoped {
			fmt.Fprintln(os.Stderr, "Error: --update-baseline records the whole project; drop --modules, --documents and --rules")
			os.Exit(1)
		}
		// Loaded before connecting, for the same reason as the --documents check
		// below: a typo in the path should not wait for a catalog build.
		var baseline *linter.Baseline
		if baselinePath != "" && !updateBaseline {
			b, err := linter.LoadBaseline(baselinePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			baseline = b
		}
		// Checked before connecting: a malformed filter is the caller's typo, and
		// making them wait for a catalog build to hear about it is the slow half
		// of the problem --documents exists to fix.
		for _, d := range documentFilter {
			if !strings.Contains(d, ".") {
				fmt.Fprintf(os.Stderr, "Error: --documents takes QUALIFIED names; %q names no module.\n", d)
				fmt.Fprintln(os.Stderr, "  Use Module.Document, e.g. Sales.ACT_Order. A bare name matches nothing,")
				fmt.Fprintln(os.Stderr, "  and a scoped lint that matches nothing reports a clean document.")
				os.Exit(1)
			}
		}

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Create executor and connect. With a machine-readable format, stdout
		// carries only the payload and every progress line goes to stderr —
		// otherwise "Connected to:" and the catalog lines precede the JSON and
		// nothing can parse stdout.
		exec, logger := newLoggedExecutorTo("subcommand", progressSink(format))
		defer logger.Close()
		defer exec.Close()

		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				os.Exit(1)
			}
		}

		projectDir := filepath.Dir(projectPath)

		// Build the rule set first — rules construct independently of the catalog,
		// so we can compute the catalog depth they need BEFORE building it. A rule
		// that needs the refs (full) or graph_* (communities) tables then gets them
		// automatically instead of silently returning empty results (issue #721).
		lintRules := projectLintRules(projectDir, os.Stderr)
		lintRulesDir := linter.FindLintRulesDir(projectDir)

		// Build catalog at the depth the rules need (fast / full / communities).
		catalogMode := linter.RequiredCatalogMode(lintRules)
		refreshProg, _ := visitor.Build(catalogRefreshCommand(catalogMode))
		for _, stmt := range refreshProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error building catalog: %v\n", err)
				os.Exit(1)
			}
		}

		// Get catalog from executor
		cat := exec.Catalog()
		if cat == nil {
			fmt.Fprintln(os.Stderr, "Error: catalog not built")
			os.Exit(1)
		}

		// Create lint context
		ctx := linter.NewLintContext(cat, exec.Backend())
		ctx.SetExcludedModules(excludeModules)
		if len(moduleFilter) > 0 {
			ctx.SetIncludedModules(moduleFilter)
		}
		// --documents narrows the document iterators AND implies their modules,
		// so rules this package does not narrow still skip the other modules
		// before their per-document read (ako/mxcli#681).
		if len(documentFilter) > 0 {
			ctx.SetIncludedDocuments(documentFilter)
		}

		// Safety net: if a rule needs data the catalog still lacks (e.g. a project
		// with no cross-references, or a build that couldn't populate the graph),
		// warn instead of silently under-reporting.
		if !ctx.SatisfiesCatalogMode(catalogMode) {
			fmt.Fprintf(os.Stderr, "Warning: some rules need '%s' catalog data that is unavailable; their results may be incomplete\n", catalogMode)
		}

		// Create linter and register the rules built above.
		lint := linter.New(ctx)
		for _, rule := range lintRules {
			lint.AddRule(rule)
		}

		// Load lint config file and apply (excludedModules, rule severity/enabled overrides).
		// Config ExcludeModules merges with --exclude flag values.
		cfg, configPath := applyLintConfig(lint, projectDir, os.Stderr)
		if cfg != nil {
			if len(cfg.ExcludeModules) > 0 {
				merged := append(excludeModules, cfg.ExcludeModules...)
				ctx.SetExcludedModules(merged)
				// An exclude always wins over --modules (LintContext.IsExcluded
				// checks the exclude set first), so asking for a module the
				// config excludes yields zero findings with no explanation.
				// `mxcli init` now ships a config excluding System, which makes
				// `lint -m System` exactly that trap — say so rather than
				// returning a silent empty result.
				if shadowed := intersect(moduleFilter, cfg.ExcludeModules); len(shadowed) > 0 {
					fmt.Fprintf(os.Stderr,
						"Warning: --modules names %s, but %s excluded by %s — no findings will be reported for %s. Remove it from excludeModules to lint it.\n",
						strings.Join(shadowed, ", "),
						pluralIsAre(len(shadowed)),
						configPath,
						pluralItThem(len(shadowed)))
				}
			}
		}

		// If --rules is specified, disable every rule not in the allowlist.
		//
		// Validate the ids first. The allowlist works by disabling everything it
		// does not name, so an id matching no rule disabled every rule and the
		// run reported "No issues found." — indistinguishable from a clean
		// project, and the exact failure #904 described. That is also how a rule
		// which failed to load reads: silently absent, then silently clean.
		if len(onlyRules) > 0 {
			if unknown := linter.UnknownRuleIDs(onlyRules, lint.Rules()); len(unknown) > 0 {
				fmt.Fprintf(os.Stderr, "Error: unknown rule id(s): %s\n", strings.Join(unknown, ", "))
				if lintRulesDir == "" {
					fmt.Fprintln(os.Stderr, "  No .claude/lint-rules/ directory was found, so no Starlark rules are loaded.")
					fmt.Fprintln(os.Stderr, "  Run 'mxcli init' to create one, or check that it is inside the repository.")
				} else {
					fmt.Fprintf(os.Stderr, "  Starlark rules were loaded from %s.\n", lintRulesDir)
				}
				fmt.Fprintln(os.Stderr, "  Run 'mxcli lint -p <project> --list-rules' to see the rules that are available.")
				os.Exit(1)
			}
			for _, rule := range lint.Rules() {
				allowed := false
				for _, id := range onlyRules {
					if linter.MatchesRuleID(id, rule.ID()) {
						allowed = true
						break
					}
				}
				if !allowed {
					lint.ConfigureRule(rule.ID(), linter.RuleConfig{Enabled: false})
				}
			}
		}

		// List rules mode
		if listRules {
			fmt.Println("Available lint rules:")
			fmt.Println()
			for _, rule := range lint.Rules() {
				fmt.Printf("  %s (%s) - %s\n", rule.ID(), rule.Name(), rule.Description())
				fmt.Printf("      Category: %s, Severity: %s\n", rule.Category(), rule.DefaultSeverity())
				fmt.Println()
			}
			return
		}

		// Run linting
		violations, err := lint.Run(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running linter: %v\n", err)
			os.Exit(1)
		}

		// A rule that reports without walking a document iterator (a project
		// setting, a security policy) is unaffected by the SQL narrowing above,
		// so a scoped run would still carry its findings. Drop anything that is
		// not one of the named documents: `lint -d A.B` reporting something about
		// C is the same class of lie as a gate that passes what it did not read.
		if len(documentFilter) > 0 {
			violations = keepDocumentViolations(violations, documentFilter)
		}

		// Findings a decision in the project brain accepts are left out of
		// what lint reports as open, and of the exit code; an acceptance whose
		// rule no longer fires there is stale (ako/mxcli#2304). Applied before
		// the baseline, so a baseline records only what is not accepted.
		acceptances := loadBrainAcceptances(projectDir)
		violations = acceptances.apply(violations)

		// Baseline: record the findings, or keep only the new ones. On stderr,
		// so the summary cannot corrupt --format json/sarif.
		// Which rules ran, and under which configuration: a baseline records
		// both, because a finding whose rule did not run is "not checked", not
		// "fixed" (ako/mxcli#2454, #2455).
		ranRules, failedRules := lint.RulesRun()
		configFingerprint := cfg.Fingerprint(excludeModules)
		var baselineReport *linter.BaselineReport
		if updateBaseline {
			// A rule that ran into an error reports nothing, so recording now would
			// replace the baseline with one that holds none of its findings and does
			// not list it as a rule that ran: the next run that can run it would
			// report every one as new, and a re-record is what the failing run's own
			// message suggests. Refuse instead of writing a worse baseline.
			if len(failedRules) > 0 {
				fmt.Fprintf(os.Stderr, "Error: the baseline was not recorded: %s could not run, so it would hold none of their findings.\n", shortList(failedRules, 8))
				for _, v := range violations {
					if v.RuleFailure {
						fmt.Fprintf(os.Stderr, "  %s\n", v.Message)
					}
				}
				fmt.Fprintln(os.Stderr, "  A rule that ran into an error usually runs on a second attempt; run the command again, and report the rule if it keeps failing.")
				os.Exit(2)
			}
			// Re-recording is how a failing gate becomes a passing one without fixing
			// anything, and an agent that wants green will do it. Errors the baseline
			// does not already hold are accepted only on request.
			existing, _ := linter.LoadBaseline(baselinePath) // none, or one that cannot be read: a first record
			toAccept := existing.ErrorsToAccept(violations, linter.RunInfo{Known: true, Ran: ranRules, Failed: failedRules, Config: configFingerprint})
			if len(toAccept) > 0 && !acceptErrors {
				fmt.Fprintf(os.Stderr, "Error: the baseline was not recorded: it would accept %d error(s) it does not hold.\n", len(toAccept))
				for i, v := range toAccept {
					if i == 5 {
						fmt.Fprintf(os.Stderr, "  ... and %d more\n", len(toAccept)-5)
						break
					}
					fmt.Fprintf(os.Stderr, "  %s %s: %s\n", v.RuleID, v.Location.QualifiedName(), v.Message)
				}
				fmt.Fprintln(os.Stderr, "  Fix them, or record them on purpose with --accept-errors.")
				os.Exit(2)
			}
			if len(toAccept) > 0 {
				fmt.Fprintf(os.Stderr, "Accepted %d error(s) into the baseline (--accept-errors); they will not fail the gate again.\n", len(toAccept))
			}
			if err := linter.WriteBaselineWith(baselinePath, violations, linter.BaselineMeta{Rules: ranRules, Config: configFingerprint}); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing lint baseline: %v\n", err)
				os.Exit(1)
			}
			recorded := 0
			for _, v := range violations {
				if !v.RuleFailure {
					recorded++
				}
			}
			fmt.Fprintf(os.Stderr, "Recorded %d finding(s) from %d rule(s) in the lint baseline %s\n", recorded, len(ranRules), baselinePath)
		} else if baseline != nil {
			allBySeverity := severityCounts(violations)
			cmp := baseline.CompareRun(violations, linter.RunInfo{Known: true, Ran: ranRules, Failed: failedRules, Config: configFingerprint, Scoped: scoped})
			violations = cmp.New
			newFindings := 0
			for _, v := range cmp.New {
				if !v.RuleFailure {
					newFindings++
				}
			}
			switch {
			case scoped:
				fmt.Fprintf(os.Stderr, "Lint baseline %s: %d new, %d already known (a scoped run: fixed findings are not counted)\n",
					baselinePath, newFindings, cmp.Unchanged)
			case cmp.NotChecked > 0:
				fmt.Fprintf(os.Stderr, "Lint baseline %s: %d new, %d already known, %d fixed, %d not checked\n",
					baselinePath, newFindings, cmp.Unchanged, cmp.Fixed, cmp.NotChecked)
			default:
				fmt.Fprintf(os.Stderr, "Lint baseline %s: %d new, %d already known, %d fixed\n",
					baselinePath, newFindings, cmp.Unchanged, cmp.Fixed)
			}
			if !scoped {
				if len(cmp.RulesNotRun) > 0 {
					fmt.Fprintf(os.Stderr, "Warning: %d finding(s) of %s are NOT checked: the baseline holds them but the rule(s) did not run (not installed, disabled, or failed). They are not fixed.\n",
						cmp.NotChecked, shortList(cmp.RulesNotRun, 8))
				}
				if cmp.ConfigChanged {
					fmt.Fprintln(os.Stderr, "Warning: the lint configuration differs from the one the baseline was recorded under, so what the rules report may differ; re-record with --update-baseline if the change is intended.")
				}
				if len(cmp.NewRules) > 0 {
					fmt.Fprintf(os.Stderr, "Note: %s ran and are not in the baseline's recorded rules; all their findings are reported as new.\n", shortList(cmp.NewRules, 8))
				}
			}
			baselineReport = &linter.BaselineReport{
				Path: baselinePath, New: newFindings, Unchanged: cmp.Unchanged, Fixed: cmp.Fixed, NotChecked: cmp.NotChecked,
				UnchangedBySeverity: knownBySeverity(allBySeverity, severityCounts(cmp.New)),
				RulesNotRun:         cmp.RulesNotRun, RuleFailures: cmp.RuleFailures, NewRules: cmp.NewRules, ConfigChanged: cmp.ConfigChanged,
				Scoped: scoped,
			}
		}

		// Output results
		outputFormat := linter.OutputFormat(format)
		formatter := linter.GetFormatter(outputFormat, useColor)
		if jf, ok := formatter.(*linter.JSONFormatter); ok {
			jf.Baseline = baselineReport
		}
		if err := formatter.Format(violations, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}

		// A failed catalog query means some iterator yielded nothing and the
		// findings above are incomplete. Reported on stderr so it survives
		// `| jq` on the results, and treated as a failure: silently passing CI
		// on a lint run that could not read the model is the worst outcome.
		queryErrs := lint.QueryErrors()
		for _, qe := range queryErrs {
			fmt.Fprintf(os.Stderr,
				"Error: lint could not read the catalog for %s: %v\n", qe.Iterator, qe.Err)
		}
		if len(queryErrs) > 0 {
			fmt.Fprintf(os.Stderr,
				"Results are INCOMPLETE (%d failed %s). The cached catalog is usually the cause; "+
					"delete %s and re-run.\n",
				len(queryErrs), map[bool]string{true: "query", false: "queries"}[len(queryErrs) == 1],
				filepath.Join(projectDir, ".mxcli", "catalog.db"))
			os.Exit(1)
		}

		// A7: the gap that motivates curation is printed by a command that
		// already runs. A queue nobody promotes is the brain's version of the
		// on-demand digest that went three months without a run — and a report
		// only `brain check` prints is a report nothing demands. One file read,
		// silent when there is no store or no queue, and on stderr so it cannot
		// corrupt --format json/sarif.
		reportBrainGap(projectDir)
		showAccepted, _ := cmd.Flags().GetBool("show-accepted")
		var stale []brain.Acceptance
		if !scoped {
			// A rule ran when lint has it and it is enabled: RuleEnabled alone
			// is true for an id lint has never heard of, which is brain
			// check's to report, not a stale acceptance.
			has := map[string]bool{}
			for _, r := range lint.Rules() {
				has[strings.ToUpper(r.ID())] = true
			}
			ran := func(rule string) bool { return has[strings.ToUpper(rule)] && lint.RuleEnabled(rule) }
			stale = acceptances.stale(ran, func(m string) bool { return !ctx.IsExcluded(m) })
		}
		acceptances.report(os.Stderr, showAccepted, stale)

		// Exit with error if there are errors — the new ones, against a
		// baseline. Recording a baseline accepts what it records.
		summary := linter.Summarize(violations)
		if summary.Errors > 0 && !updateBaseline {
			os.Exit(1)
		}
	},
}

// severityCounts counts findings about the project by severity name; a rule
// failure is about the rule and is not counted.
func severityCounts(vs []linter.Violation) map[string]int {
	out := map[string]int{}
	for _, v := range vs {
		if !v.RuleFailure {
			out[v.Severity.String()]++
		}
	}
	return out
}

// knownBySeverity is what the run found minus what is new: the findings the
// baseline already had, by severity.
func knownBySeverity(all, fresh map[string]int) map[string]int {
	out := map[string]int{}
	for sev, n := range all {
		if k := n - fresh[sev]; k > 0 {
			out[sev] = k
		}
	}
	return out
}

// shortList joins ids, keeping the first n: a baseline can miss dozens of rules
// (a checkout without the project's custom rules), and the JSON output has them all.
func shortList(ids []string, n int) string {
	if len(ids) <= n {
		return strings.Join(ids, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(ids[:n], ", "), len(ids)-n)
}

// catalogRefreshCommand maps the required catalog depth to the MDL refresh
// statement that builds it (COMMUNITIES implies FULL implies fast).
func catalogRefreshCommand(mode linter.CatalogMode) string {
	switch mode {
	case linter.CatalogCommunities:
		return "REFRESH CATALOG COMMUNITIES"
	case linter.CatalogFull:
		return "REFRESH CATALOG FULL"
	default:
		return "REFRESH CATALOG"
	}
}

// intersect returns the values of want that appear in have, preserving want's
// order and dropping duplicates.
func intersect(want, have []string) []string {
	if len(want) == 0 || len(have) == 0 {
		return nil
	}
	inHave := make(map[string]bool, len(have))
	for _, h := range have {
		inHave[h] = true
	}
	seen := make(map[string]bool, len(want))
	var out []string
	for _, w := range want {
		if inHave[w] && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

func pluralIsAre(n int) string {
	if n == 1 {
		return "it is"
	}
	return "they are"
}

func pluralItThem(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// reportBrainGap prints one line when the project brain has staged entries that
// nobody has promoted. It never fails lint and never touches the model: the
// brain is opt-in, so a project without a store hears nothing at all.
func reportBrainGap(projectDir string) {
	if !brain.NewStore(projectDir).Exists() {
		return
	}
	staged, err := brain.NewQueue(projectDir).Load()
	if err != nil {
		return
	}
	// An unanswered question is the same shape of gap as an unpromoted entry:
	// something a person has to act on, that no other command would mention.
	open, err := brain.NewStore(projectDir).OpenQuestions()
	if err != nil {
		return
	}
	if len(staged) == 0 && len(open) == 0 {
		return
	}
	var parts []string
	if len(staged) > 0 {
		parts = append(parts, fmt.Sprintf("%d staged %s not yet promoted ('mxcli brain staged')",
			len(staged), plural(len(staged), "entry", "entries")))
	}
	if len(open) > 0 {
		parts = append(parts, fmt.Sprintf("%d open %s ('mxcli brain check')",
			len(open), plural(len(open), "question", "questions")))
	}
	fmt.Fprintf(os.Stderr, "\nProject brain: %s.\n", strings.Join(parts, "; "))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// builtinLintRules is the built-in rule set, in one place because more than one
// caller needs to know what it contains. `--list-rules` prints it, and the
// generated CLAUDE.md is asserted NOT to restate it — a check that needs the
// real list rather than a second copy of it, since a second copy is the drift
// it exists to prevent.
func builtinLintRules() []linter.Rule {
	return []linter.Rule{
		rules.NewNamingConventionRule(),
		rules.NewEmptyMicroflowRule(),
		rules.NewDomainModelSizeRule(),
		rules.NewValidationFeedbackRule(),
		rules.NewImageSourceRule(),
		rules.NewReactUnsupportedWidgetRule(),
		rules.NewSnippetCallAppearanceRule(),
		rules.NewAgentWithoutSelectedModelRule(),
		rules.NewConsumedMCPServiceWithoutEndpointRule(),
		rules.NewAgentDocumentDanglingReferenceRule(),
		rules.NewEmptyContainerRule(),
		rules.NewGallerySelectionListenerRule(),
		rules.NewDataViewLayoutGridRule(),
		rules.NewRequiredCaptionDefaultLanguageRule(), // QUAL006 - CE4899, reads the stored units
		rules.NewPageNavigationSecurityRule(),
		rules.NewNoEntityAccessRulesRule(),
		rules.NewWeakPasswordPolicyRule(),
		rules.NewDemoUsersActiveRule(),
		rules.NewPageEntityReadAccessRule(),      // SEC010 - full catalog (widgets, permissions)
		rules.NewXPathAssociationAccessRule(),    // SEC011 - full catalog (xpath_expressions, permissions)
		rules.NewReachablePageReadAccessRule(),   // SEC012 - full catalog (refs, navigation, permissions)
		rules.NewSecurityLevelIgnoresRulesRule(), // SEC013 - security level Off/Prototype ignores the project's rules
		rules.NewOverlappingActivitiesRule(),     // MPR008 - requires BSON inspection
		rules.NewLoopChildContainmentRule(),      // MPR011 - requires BSON inspection
		rules.NewOverlappingEntitiesRule(),       // MPR016 - reads entity geometry from the domain model
		rules.NewFixedGuardAttributeRule(),       // MPR017 - a Boolean guard with one reachable value
		rules.NewModelNameInLiteralRule(),        // MPR018 - a model element named in a string literal
		rules.NewUnindexedConstraintRule(),       // MPR019 - an XPath-filtered attribute with no leading index
		rules.NewDomainModelNotLaidOutRule(),     // MPR020 - linked entities still on the auto-placement grid
		rules.NewAssociationCrossesEntityRule(),  // MPR021 - a diagram line through an entity it does not connect
		rules.NewNoCommitInLoopRule(),            // CONV011-CONV014 - require BSON inspection
		rules.NewExclusiveSplitCaptionRule(),
		rules.NewErrorHandlingOnCallsRule(),
		rules.NewNoContinueErrorHandlingRule(),
		rules.NewCreatedObjectNeverPersistedRule(), // CONV020 - create without commit, never persisted or passed on
		rules.NewCreateChangeThenCommitRule(),      // CONV021 - create/change then a commit of the same object
		rules.NewIrreducibleFlowGraphRule(),        // MDL-FLOW01 - graph structure vs MDL nesting
		rules.NewImportRangeObjectMappingRule(),    // MDL-MAP04 - First over an object-rooted mapping in the model
	}
}
