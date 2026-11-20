// SPDX-License-Identifier: Apache-2.0

// Package linter provides an extensible linting framework for Mendix projects.
package linter

import (
	"context"
	"sort"
	"strings"
)

// Severity indicates how serious a violation is.
type Severity int

const (
	SeverityHint Severity = iota
	SeverityInfo
	SeverityWarning
	SeverityError
)

// String returns the string representation of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityHint:
		return "hint"
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// Symbol returns the symbol used in text output.
func (s Severity) Symbol() string {
	switch s {
	case SeverityHint:
		return "💡"
	case SeverityInfo:
		return "ℹ"
	case SeverityWarning:
		return "⚠"
	case SeverityError:
		return "✗"
	default:
		return "?"
	}
}

// Violation represents a single lint violation.
type Violation struct {
	RuleID     string
	Severity   Severity
	Message    string
	Location   Location
	Suggestion string
	// RuleFailure marks a finding about the rule rather than the project: the
	// rule itself failed to run. It says nothing about the model, so the
	// report keeps it out of the score and lists it separately
	// (ako/mxcli#952), and a configured severity override does not apply.
	RuleFailure bool
}

// Location identifies where a violation occurred.
type Location struct {
	Module       string // e.g., "Sales"
	DocumentType string // "entity", "microflow", "page"
	DocumentName string // e.g., "Customer"
	DocumentID   string // UUID
}

// QualifiedName returns the full qualified name of the location.
//
// DocumentName is meant to be the document's own name, but most bundled
// Starlark rules pass the qualified one (`document_name=mf.qualified_name`), and
// so does missing_translations. Prefixing the module again printed every such
// finding as `at M.M.NAV_Home` (ako/mxcli#1484). This is the one place a
// location is turned into a name — text, Markdown, SARIF and `lint -d` all come
// through here — so both spellings are accepted here rather than in each rule,
// which would leave a project's stale rule copies and every user-written rule
// still repeating the module. A dot cannot occur in a Mendix document name, so
// "<module>." at the front is always the module and never part of the name.
func (l Location) QualifiedName() string {
	if l.Module == "" {
		return l.DocumentName
	}
	if strings.HasPrefix(l.DocumentName, l.Module+".") {
		return l.DocumentName
	}
	// A finding about the module itself (CONV018) names the module as its
	// document; "Sales.Sales" would read as a document called Sales.
	if strings.EqualFold(l.DocumentType, "module") && l.DocumentName == l.Module {
		return l.Module
	}
	return l.Module + "." + l.DocumentName
}

// Rule is the interface that all lint rules must implement.
type Rule interface {
	ID() string
	Name() string
	Description() string
	DefaultSeverity() Severity
	Category() string
	Check(ctx *LintContext) []Violation
}

// Configurable is an optional interface for rules that accept options from the lint config file.
// Rules implementing this interface receive their options map before Check is called.
type Configurable interface {
	Configure(options map[string]any)
}

// RuleConfig holds configuration for a specific rule.
type RuleConfig struct {
	Enabled  bool
	Severity Severity
	Options  map[string]any
}

// Linter orchestrates running lint rules against a project.
type Linter struct {
	ctx        *LintContext
	rules      []Rule
	configs    map[string]RuleConfig
	maxWorkers int

	// ran and failed are the rules the last Run executed, and those of them that
	// reported they could not run. A rule that is not enabled is in neither.
	ran    []string
	failed map[string]bool
}

// New creates a new Linter with the given context.
func New(ctx *LintContext) *Linter {
	return &Linter{
		ctx:        ctx,
		rules:      []Rule{},
		configs:    make(map[string]RuleConfig),
		maxWorkers: 4,
	}
}

// AddRule adds a rule to the linter.
func (l *Linter) AddRule(rule Rule) {
	l.rules = append(l.rules, rule)
}

// ConfigureRule sets configuration for a specific rule.
func (l *Linter) ConfigureRule(ruleID string, config RuleConfig) {
	l.configs[ruleID] = config
}

// SetMaxWorkers sets the maximum number of parallel workers.
func (l *Linter) SetMaxWorkers(n int) {
	if n > 0 {
		l.maxWorkers = n
	}
}

// Rules returns all registered rules.
func (l *Linter) Rules() []Rule {
	return l.rules
}

// Run executes all enabled rules and returns the violations found.
func (l *Linter) Run(ctx context.Context) ([]Violation, error) {
	var allViolations []Violation
	l.ran, l.failed = nil, map[string]bool{}

	// Run rules sequentially to avoid SQLite concurrency issues with in-memory db
	for _, rule := range l.rules {
		// Check if rule is enabled
		if config, ok := l.configs[rule.ID()]; ok && !config.Enabled {
			continue
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return allViolations, ctx.Err()
		default:
		}

		// Pass options to rules that support configuration.
		if config, ok := l.configs[rule.ID()]; ok {
			if len(config.Options) > 0 {
				if c, ok := rule.(Configurable); ok {
					c.Configure(config.Options)
				}
			}
		}

		// Run the rule
		violations := rule.Check(l.ctx)

		// Apply configured severity if different from default. A rule
		// failure keeps its own: configuring QUAL004 as "warning" says how
		// much its findings matter, not how much its crashing does.
		if config, ok := l.configs[rule.ID()]; ok {
			for i := range violations {
				if !violations[i].RuleFailure {
					violations[i].Severity = config.Severity
				}
			}
		}

		// Which rules ran, and which could not: a baseline has to know, because
		// a rule that did not run reports nothing, which reads as every one of
		// its findings being fixed (ako/mxcli#2454, #2455).
		l.ran = append(l.ran, rule.ID())
		for _, v := range violations {
			if v.RuleFailure {
				l.failed[rule.ID()] = true
			}
		}

		// Collect results
		allViolations = append(allViolations, violations...)
	}

	return allViolations, nil
}

// RulesRun returns the IDs of the rules the last Run executed to completion, and
// separately those that were enabled and executed but reported that they could
// not run. Both are sorted. A rule that was disabled, or filtered out, is in
// neither.
func (l *Linter) RulesRun() (ran, failed []string) {
	for _, id := range l.ran {
		if l.failed[id] {
			failed = append(failed, id)
		} else {
			ran = append(ran, id)
		}
	}
	sort.Strings(ran)
	sort.Strings(failed)
	return ran, failed
}

// QueryErrors returns the catalog queries that failed during the run.
//
// A non-empty result means the findings are INCOMPLETE: some iterator could not
// read the catalog and yielded nothing, which is indistinguishable from a clean
// project in the violation list alone. Callers should report these rather than
// present the run as successful.
func (l *Linter) QueryErrors() []QueryError {
	if l.ctx == nil {
		return nil
	}
	return l.ctx.QueryErrors()
}

// Summary holds counts of violations by severity.
type Summary struct {
	Errors   int
	Warnings int
	Infos    int
	Hints    int
	Total    int
}

// Summarize counts violations by severity.
func Summarize(violations []Violation) Summary {
	var s Summary
	for _, v := range violations {
		switch v.Severity {
		case SeverityError:
			s.Errors++
		case SeverityWarning:
			s.Warnings++
		case SeverityInfo:
			s.Infos++
		case SeverityHint:
			s.Hints++
		}
	}
	s.Total = len(violations)
	return s
}

// RuleEnabled reports whether a rule will run. A rule with no configuration runs
// by default, so this answers the same question Run asks rather than reporting
// whether a configuration exists.
func (l *Linter) RuleEnabled(ruleID string) bool {
	if config, ok := l.configs[ruleID]; ok {
		return config.Enabled
	}
	return true
}
