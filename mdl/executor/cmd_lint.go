// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter/rules"
)

// execLint executes a LINT statement.
func execLint(ctx *ExecContext, s *ast.LintStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Handle SHOW LINT RULES
	if s.ShowRules {
		return listLintRules(ctx)
	}

	projectDir := filepath.Dir(ctx.MprPath)

	// Build the rule set first so we can size the catalog to what the rules need
	// (issue #721): a rule using the refs (full) or graph_* (communities) tables
	// gets them instead of silently returning empty.
	lintRules := []linter.Rule{
		rules.NewNamingConventionRule(),
		rules.NewEmptyMicroflowRule(),
		rules.NewDomainModelSizeRule(),
		rules.NewValidationFeedbackRule(),
		rules.NewImageSourceRule(),
		rules.NewMissingTranslationsRule(),
		rules.NewGallerySelectionListenerRule(),
		rules.NewDataViewLayoutGridRule(),
	}
	rulesDir := filepath.Join(projectDir, ".claude", "lint-rules")
	starlarkRules, err := linter.LoadStarlarkRulesFromDir(rulesDir)
	if err != nil {
		fmt.Fprintf(ctx.Output, "Warning: failed to load custom rules: %v\n", err)
	}
	for _, rule := range starlarkRules {
		lintRules = append(lintRules, rule)
	}
	catalogMode := linter.RequiredCatalogMode(lintRules)
	needFull := catalogMode >= linter.CatalogFull
	needCommunities := catalogMode >= linter.CatalogCommunities

	// Ensure the catalog exists and is deep enough.
	if ctx.Catalog == nil {
		fmt.Fprintln(ctx.Output, "Building catalog for linting...")
		if err := buildCatalog(ctx, needFull, false, needCommunities, 0); err != nil {
			return mdlerrors.NewBackend("build catalog", err)
		}
	}

	// Create lint context
	lintCtx := linter.NewLintContext(ctx.Catalog, ctx.Backend)

	// A reused catalog (e.g. a prior fast REFRESH CATALOG in the same REPL session)
	// may be too shallow — rebuild it at the required depth.
	if (needFull || needCommunities) && !lintCtx.SatisfiesCatalogMode(catalogMode) {
		if err := buildCatalog(ctx, needFull, false, needCommunities, 0); err == nil {
			lintCtx = linter.NewLintContext(ctx.Catalog, ctx.Backend)
		}
	}
	if !lintCtx.SatisfiesCatalogMode(catalogMode) {
		fmt.Fprintf(ctx.Output, "Warning: some rules need '%s' catalog data that is unavailable; their results may be incomplete\n", catalogMode)
	}

	// Load configuration
	configPath := linter.FindConfigFile(projectDir)
	config, err := linter.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(ctx.Output, "Warning: failed to load lint config: %v\n", err)
		config = linter.DefaultConfig()
	}

	// Set excluded modules from config
	if len(config.ExcludeModules) > 0 {
		lintCtx.SetExcludedModules(config.ExcludeModules)
	}

	// Create linter and register the rules built above.
	lint := linter.New(lintCtx)
	for _, rule := range lintRules {
		lint.AddRule(rule)
	}

	// Apply configuration
	config.ApplyConfig(lint)

	// Handle module filtering
	if s.Target != nil && s.ModuleOnly {
		// Only lint specific module - set all others as excluded
		lintCtx.SetExcludedModules(nil) // Clear any existing exclusions
		// This is a simplified approach - ideally we'd filter in the linter
		fmt.Fprintf(ctx.Output, "Linting module: %s\n", s.Target.Module)
	}

	// Run linting
	violations, err := lint.Run(context.Background())
	if err != nil {
		return mdlerrors.NewBackend("lint", err)
	}

	// Filter violations if targeting specific module
	if s.Target != nil && s.ModuleOnly {
		filtered := make([]linter.Violation, 0)
		for _, v := range violations {
			if v.Location.Module == s.Target.Module {
				filtered = append(filtered, v)
			}
		}
		violations = filtered
	}

	// Output results
	var format linter.OutputFormat
	switch s.Format {
	case ast.LintFormatJSON:
		format = linter.OutputFormatJSON
	case ast.LintFormatSARIF:
		format = linter.OutputFormatSARIF
	default:
		format = linter.OutputFormatText
	}

	formatter := linter.GetFormatter(format, false)
	return formatter.Format(violations, ctx.Output)
}

// listLintRules displays available lint rules.
func listLintRules(ctx *ExecContext) error {
	fmt.Fprintln(ctx.Output, "Built-in rules:")
	fmt.Fprintln(ctx.Output)

	// Create a temporary linter with built-in rules
	lint := linter.New(nil)
	lint.AddRule(rules.NewNamingConventionRule())
	lint.AddRule(rules.NewEmptyMicroflowRule())
	lint.AddRule(rules.NewDomainModelSizeRule())
	lint.AddRule(rules.NewValidationFeedbackRule())
	lint.AddRule(rules.NewImageSourceRule())
	lint.AddRule(rules.NewMissingTranslationsRule())
	lint.AddRule(rules.NewGallerySelectionListenerRule())
	lint.AddRule(rules.NewDataViewLayoutGridRule())

	for _, rule := range lint.Rules() {
		fmt.Fprintf(ctx.Output, "  %s (%s)\n", rule.ID(), rule.Name())
		fmt.Fprintf(ctx.Output, "    %s\n", rule.Description())
		fmt.Fprintf(ctx.Output, "    Category: %s, Default Severity: %s\n", rule.Category(), rule.DefaultSeverity())
		fmt.Fprintln(ctx.Output)
	}

	// Show custom Starlark rules if connected
	if ctx.MprPath != "" {
		projectDir := filepath.Dir(ctx.MprPath)
		rulesDir := filepath.Join(projectDir, ".claude", "lint-rules")
		starlarkRules, err := linter.LoadStarlarkRulesFromDir(rulesDir)
		if err == nil && len(starlarkRules) > 0 {
			fmt.Fprintln(ctx.Output, "Custom rules (from .claude/lint-rules/):")
			fmt.Fprintln(ctx.Output)
			for _, rule := range starlarkRules {
				fmt.Fprintf(ctx.Output, "  %s (%s)\n", rule.ID(), rule.Name())
				fmt.Fprintf(ctx.Output, "    %s\n", rule.Description())
				fmt.Fprintf(ctx.Output, "    Category: %s, Default Severity: %s\n", rule.Category(), rule.DefaultSeverity())
				fmt.Fprintln(ctx.Output)
			}
		}
	}

	return nil
}

// --- Executor method wrappers for backward compatibility ---
