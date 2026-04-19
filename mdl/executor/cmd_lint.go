// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/linter/rules"
)

// execLint executes a LINT statement.
func execLint(ctx *ExecContext, s *ast.LintStmt) error {
	e := ctx.executor
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Handle SHOW LINT RULES
	if s.ShowRules {
		return showLintRules(ctx)
	}

	// Ensure catalog is built
	if ctx.Catalog == nil {
		fmt.Fprintln(ctx.Output, "Building catalog for linting...")
		if err := buildCatalog(ctx, false); err != nil {
			return mdlerrors.NewBackend("build catalog", err)
		}
	}

	// Create lint context
	lintCtx := linter.NewLintContext(ctx.Catalog)
	lintCtx.SetReader(e.Reader())

	// Load configuration
	projectDir := filepath.Dir(ctx.MprPath)
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

	// Create linter and register built-in rules
	lint := linter.New(lintCtx)
	lint.AddRule(rules.NewNamingConventionRule())
	lint.AddRule(rules.NewEmptyMicroflowRule())
	lint.AddRule(rules.NewDomainModelSizeRule())
	lint.AddRule(rules.NewValidationFeedbackRule())
	lint.AddRule(rules.NewImageSourceRule())
	lint.AddRule(rules.NewMissingTranslationsRule())

	// Load custom Starlark rules
	rulesDir := filepath.Join(projectDir, ".claude", "lint-rules")
	starlarkRules, err := linter.LoadStarlarkRulesFromDir(rulesDir)
	if err != nil {
		fmt.Fprintf(ctx.Output, "Warning: failed to load custom rules: %v\n", err)
	}
	for _, rule := range starlarkRules {
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

// showLintRules displays available lint rules.
func showLintRules(ctx *ExecContext) error {
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
