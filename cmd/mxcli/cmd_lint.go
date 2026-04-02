// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
  - Navigation page security (MPR007) - pages in navigation need allowed roles
  - Entity access rules (SEC001) - persistent entities need access rules
  - Password policy (SEC002) - password minimum length should be 8+
  - Demo users (SEC003) - demo users should be off at Production security

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
  - Missing documentation (QUAL002) - entities/microflows need documentation
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

Custom Starlark rules in .claude/lint-rules/*.star are loaded automatically.

Examples:
  mxcli lint -p app.mpr
  mxcli lint -p app.mpr --format json
  mxcli lint -p app.mpr --format sarif > results.sarif
  mxcli lint -p app.mpr --list-rules
`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		format, _ := cmd.Flags().GetString("format")
		useColor, _ := cmd.Flags().GetBool("color")
		listRules, _ := cmd.Flags().GetBool("list-rules")
		excludeModules, _ := cmd.Flags().GetStringSlice("exclude")

		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		// Create executor and connect
		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()

		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", projectPath))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				os.Exit(1)
			}
		}

		// Build catalog (required for linting)
		refreshCmd := "REFRESH CATALOG"
		refreshProg, _ := visitor.Build(refreshCmd)
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
		ctx := linter.NewLintContext(cat)
		ctx.SetExcludedModules(excludeModules)

		// Set reader so rules that inspect raw BSON (MPR004, MPR005) work
		if reader := exec.Reader(); reader != nil {
			ctx.SetReader(reader)
		}

		// Create linter and register rules
		lint := linter.New(ctx)
		lint.AddRule(rules.NewNamingConventionRule())
		lint.AddRule(rules.NewEmptyMicroflowRule())
		lint.AddRule(rules.NewDomainModelSizeRule())
		lint.AddRule(rules.NewValidationFeedbackRule())
		lint.AddRule(rules.NewImageSourceRule())
		lint.AddRule(rules.NewEmptyContainerRule())
		lint.AddRule(rules.NewPageNavigationSecurityRule())
		lint.AddRule(rules.NewNoEntityAccessRulesRule())
		lint.AddRule(rules.NewWeakPasswordPolicyRule())
		lint.AddRule(rules.NewDemoUsersActiveRule())

		// MPR008 - requires BSON inspection
		lint.AddRule(rules.NewOverlappingActivitiesRule())

		// Convention rules (CONV011-CONV014) - require BSON inspection
		lint.AddRule(rules.NewNoCommitInLoopRule())
		lint.AddRule(rules.NewExclusiveSplitCaptionRule())
		lint.AddRule(rules.NewErrorHandlingOnCallsRule())
		lint.AddRule(rules.NewNoContinueErrorHandlingRule())

		// Load custom Starlark rules from project's .claude/lint-rules/
		projectDir := filepath.Dir(projectPath)
		lintRulesDir := filepath.Join(projectDir, ".claude", "lint-rules")
		if starlarkRules, err := linter.LoadStarlarkRulesFromDir(lintRulesDir); err == nil {
			for _, rule := range starlarkRules {
				lint.AddRule(rule)
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

		// Output results
		outputFormat := linter.OutputFormat(format)
		formatter := linter.GetFormatter(outputFormat, useColor)
		if err := formatter.Format(violations, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}

		// Exit with error if there are errors
		summary := linter.Summarize(violations)
		if summary.Errors > 0 {
			os.Exit(1)
		}
	},
}
