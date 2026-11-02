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
		if cfg, configPath := applyLintConfig(lint, projectDir, os.Stderr); cfg != nil {
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

		// Baseline: record the findings, or keep only the new ones. On stderr,
		// so the summary cannot corrupt --format json/sarif.
		if updateBaseline {
			if err := linter.WriteBaseline(baselinePath, violations); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing lint baseline: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Recorded %d finding(s) in the lint baseline %s\n", len(violations), baselinePath)
		} else if baseline != nil {
			cmp := baseline.Compare(violations)
			violations = cmp.New
			if scoped {
				fmt.Fprintf(os.Stderr, "Lint baseline %s: %d new, %d already known (a scoped run: fixed findings are not counted)\n",
					baselinePath, len(cmp.New), cmp.Unchanged)
			} else {
				fmt.Fprintf(os.Stderr, "Lint baseline %s: %d new, %d already known, %d fixed\n",
					baselinePath, len(cmp.New), cmp.Unchanged, cmp.Fixed)
			}
		}

		// Output results
		outputFormat := linter.OutputFormat(format)
		formatter := linter.GetFormatter(outputFormat, useColor)
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

		// Exit with error if there are errors — the new ones, against a
		// baseline. Recording a baseline accepts what it records.
		summary := linter.Summarize(violations)
		if summary.Errors > 0 && !updateBaseline {
			os.Exit(1)
		}
	},
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
		rules.NewEmptyContainerRule(),
		rules.NewGallerySelectionListenerRule(),
		rules.NewDataViewLayoutGridRule(),
		rules.NewRequiredCaptionDefaultLanguageRule(), // QUAL006 - CE4899, reads the stored units
		rules.NewPageNavigationSecurityRule(),
		rules.NewNoEntityAccessRulesRule(),
		rules.NewWeakPasswordPolicyRule(),
		rules.NewDemoUsersActiveRule(),
		rules.NewPageEntityReadAccessRule(),    // SEC010 - full catalog (widgets, permissions)
		rules.NewXPathAssociationAccessRule(),  // SEC011 - full catalog (xpath_expressions, permissions)
		rules.NewReachablePageReadAccessRule(), // SEC012 - full catalog (refs, navigation, permissions)
		rules.NewOverlappingActivitiesRule(),   // MPR008 - requires BSON inspection
		rules.NewLoopChildContainmentRule(),    // MPR011 - requires BSON inspection
		rules.NewNoCommitInLoopRule(),          // CONV011-CONV014 - require BSON inspection
		rules.NewExclusiveSplitCaptionRule(),
		rules.NewErrorHandlingOnCallsRule(),
		rules.NewNoContinueErrorHandlingRule(),
		rules.NewCreatedObjectNeverPersistedRule(), // CONV020 - create without commit, never persisted or passed on
		rules.NewCreateChangeThenCommitRule(),      // CONV021 - create/change then a commit of the same object
		rules.NewIrreducibleFlowGraphRule(),        // MDL-FLOW01 - graph structure vs MDL nesting
		rules.NewImportRangeObjectMappingRule(),    // MDL-MAP04 - First over an object-rooted mapping in the model
	}
}
