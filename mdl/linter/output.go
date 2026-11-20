// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// OutputFormat specifies the output format for lint results.
type OutputFormat string

const (
	OutputFormatText  OutputFormat = "text"
	OutputFormatJSON  OutputFormat = "json"
	OutputFormatSARIF OutputFormat = "sarif"
)

// Formatter formats violations for output.
type Formatter interface {
	Format(violations []Violation, w io.Writer) error
}

// GetFormatter returns a formatter for the given format.
func GetFormatter(format OutputFormat, useColor bool) Formatter {
	switch format {
	case OutputFormatJSON:
		return &JSONFormatter{}
	case OutputFormatSARIF:
		return &SARIFFormatter{}
	default:
		return &TextFormatter{UseColor: useColor}
	}
}

// TextFormatter outputs violations in human-readable text format.
type TextFormatter struct {
	UseColor bool
	// OmittedInfos counts info-level violations the caller left out of the
	// list it passes (exec's preflight counts its notes instead of printing
	// them). The summary line includes them, marked as not shown, so it does
	// not claim "0 info" for a script that has some.
	OmittedInfos int
	// NoSummary leaves the "N issues: …" line out, for a caller that formats
	// several batches and prints one summary over all of them (WriteSummary).
	NoSummary bool
}

// Format outputs violations grouped by module.
func (f *TextFormatter) Format(violations []Violation, w io.Writer) error {
	if len(violations) == 0 {
		fmt.Fprintln(w, "No issues found.")
		return nil
	}

	// Group by module
	byModule := make(map[string][]Violation)
	for _, v := range violations {
		module := v.Location.Module
		if module == "" {
			module = "(no module)"
		}
		byModule[module] = append(byModule[module], v)
	}

	// Sort module names
	modules := make([]string, 0, len(byModule))
	for m := range byModule {
		modules = append(modules, m)
	}
	sort.Strings(modules)

	// Output each module
	for _, module := range modules {
		modViolations := byModule[module]

		// Module header
		fmt.Fprintln(w, f.colorize(module, colorCyan))
		fmt.Fprintln(w, strings.Repeat("-", len(module)))

		// Sort violations by document name
		sort.Slice(modViolations, func(i, j int) bool {
			return modViolations[i].Location.DocumentName < modViolations[j].Location.DocumentName
		})

		// Output violations
		for _, v := range modViolations {
			symbol := f.severitySymbol(v.Severity)
			ruleID := fmt.Sprintf("[%s]", v.RuleID)

			// Main message
			fmt.Fprintf(w, "  %s %s %s\n", symbol, v.Message, f.colorize(ruleID, colorDim))

			// Location
			location := v.Location.QualifiedName()
			fmt.Fprintf(w, "      at %s\n", f.colorize(location, colorDim))

			// Suggestion
			if v.Suggestion != "" {
				fmt.Fprintf(w, "      %s %s\n", f.colorize("→", colorGreen), v.Suggestion)
			}

			fmt.Fprintln(w)
		}
	}

	if f.NoSummary {
		return nil
	}
	WriteSummary(w, violations, f.OmittedInfos)
	return nil
}

// WriteSummary prints the "N issues: …" line for violations, counting
// omittedInfos info notes that were left out of the list as not shown.
func WriteSummary(w io.Writer, violations []Violation, omittedInfos int) {
	summary := Summarize(violations)
	if omittedInfos > 0 {
		fmt.Fprintf(w, "%d issues: %d errors, %d warnings, %d info (not shown)\n",
			summary.Total+omittedInfos, summary.Errors, summary.Warnings, summary.Infos+omittedInfos)
		return
	}
	fmt.Fprintf(w, "%d issues: %d errors, %d warnings, %d info\n",
		summary.Total, summary.Errors, summary.Warnings, summary.Infos)
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
)

func (f *TextFormatter) colorize(s string, color string) string {
	if !f.UseColor {
		return s
	}
	return color + s + colorReset
}

func (f *TextFormatter) severitySymbol(s Severity) string {
	switch s {
	case SeverityError:
		return f.colorize("✗", colorRed)
	case SeverityWarning:
		return f.colorize("⚠", colorYellow)
	case SeverityInfo:
		return f.colorize("ℹ", colorBlue)
	case SeverityHint:
		return f.colorize("💡", colorDim)
	default:
		return "?"
	}
}

// JSONFormatter outputs violations as JSON.
type JSONFormatter struct {
	// Baseline, when set, is added to the output as "baseline": how the run
	// compares with the baseline it was given.
	Baseline *BaselineReport
}

// BaselineReport is how a run compares with a baseline, for the JSON output. A
// reader that gates on lint (verify) needs more than the new findings: that rules
// did not run, or that the configuration changed, means a short list of new
// findings is not a clean one.
type BaselineReport struct {
	Path          string   `json:"path"`
	New           int      `json:"new"`
	Unchanged     int      `json:"unchanged"`
	Fixed         int      `json:"fixed"`
	NotChecked    int      `json:"notChecked"`
	RulesNotRun   []string `json:"rulesNotRun,omitempty"`
	RuleFailures  []string `json:"ruleFailures,omitempty"`
	NewRules      []string `json:"newRules,omitempty"`
	ConfigChanged bool     `json:"configChanged,omitempty"`
	Scoped        bool     `json:"scoped,omitempty"`
}

// JSONViolation is the JSON representation of a violation.
type JSONViolation struct {
	RuleID       string `json:"ruleId"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Module       string `json:"module"`
	Document     string `json:"document"`
	DocumentType string `json:"documentType"`
	DocumentID   string `json:"documentId,omitempty"`
	Suggestion   string `json:"suggestion,omitempty"`
	// RuleFailure marks a finding about the rule (it could not run), not the project.
	RuleFailure bool `json:"ruleFailure,omitempty"`
}

// JSONOutput is the JSON output structure.
type JSONOutput struct {
	Violations []JSONViolation `json:"violations"`
	Baseline   *BaselineReport `json:"baseline,omitempty"`
	Summary    struct {
		Total    int `json:"total"`
		Errors   int `json:"errors"`
		Warnings int `json:"warnings"`
		Infos    int `json:"infos"`
		Hints    int `json:"hints"`
	} `json:"summary"`
}

// Format outputs violations as JSON.
func (f *JSONFormatter) Format(violations []Violation, w io.Writer) error {
	output := JSONOutput{Baseline: f.Baseline}

	for _, v := range violations {
		output.Violations = append(output.Violations, JSONViolation{
			RuleID:       v.RuleID,
			Severity:     v.Severity.String(),
			Message:      v.Message,
			Module:       v.Location.Module,
			Document:     v.Location.DocumentName,
			DocumentType: v.Location.DocumentType,
			DocumentID:   v.Location.DocumentID,
			Suggestion:   v.Suggestion,
			RuleFailure:  v.RuleFailure,
		})
	}

	summary := Summarize(violations)
	output.Summary.Total = summary.Total
	output.Summary.Errors = summary.Errors
	output.Summary.Warnings = summary.Warnings
	output.Summary.Infos = summary.Infos
	output.Summary.Hints = summary.Hints

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// SARIFFormatter outputs violations in SARIF format (for CI/GitHub integration).
type SARIFFormatter struct{}

// Format outputs violations in SARIF format.
func (f *SARIFFormatter) Format(violations []Violation, w io.Writer) error {
	// SARIF 2.1.0 format
	sarif := map[string]any{
		"$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		"version": "2.1.0",
		"runs": []map[string]any{
			{
				"tool": map[string]any{
					"driver": map[string]any{
						"name":    "mxcli-lint",
						"version": "0.1.0",
						"rules":   f.buildRules(violations),
					},
				},
				"results": f.buildResults(violations),
			},
		},
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(sarif)
}

func (f *SARIFFormatter) buildRules(violations []Violation) []map[string]any {
	// Collect unique rules
	ruleMap := make(map[string]bool)
	var rules []map[string]any

	for _, v := range violations {
		if ruleMap[v.RuleID] {
			continue
		}
		ruleMap[v.RuleID] = true

		rules = append(rules, map[string]any{
			"id": v.RuleID,
			"shortDescription": map[string]string{
				"text": v.RuleID,
			},
		})
	}

	return rules
}

func (f *SARIFFormatter) buildResults(violations []Violation) []map[string]any {
	var results []map[string]any

	for _, v := range violations {
		level := "warning"
		switch v.Severity {
		case SeverityError:
			level = "error"
		case SeverityWarning:
			level = "warning"
		case SeverityInfo, SeverityHint:
			level = "note"
		}

		result := map[string]any{
			"ruleId": v.RuleID,
			"level":  level,
			"message": map[string]string{
				"text": v.Message,
			},
			"locations": []map[string]any{
				{
					"physicalLocation": map[string]any{
						"artifactLocation": map[string]string{
							"uri": v.Location.QualifiedName(),
						},
					},
				},
			},
		}

		results = append(results, result)
	}

	return results
}
