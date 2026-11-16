// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"
)

// ReportFormatter formats a report for output.
type ReportFormatter interface {
	FormatReport(report *Report, w io.Writer) error
}

// GetReportFormatter returns a formatter for the given format.
func GetReportFormatter(format string) ReportFormatter {
	switch format {
	case "json":
		return &JSONReportFormatter{}
	case "html":
		return &HTMLReportFormatter{}
	default:
		return &MarkdownReportFormatter{}
	}
}

// --- Markdown Formatter ---

// MarkdownReportFormatter outputs the report as Markdown.
type MarkdownReportFormatter struct{}

func (f *MarkdownReportFormatter) FormatReport(report *Report, w io.Writer) error {
	fmt.Fprintf(w, "# Mendix Best Practices Report\n\n")
	fmt.Fprintf(w, "**Project:** %s  \n", report.ProjectName)
	fmt.Fprintf(w, "**Date:** %s  \n", report.Date)
	if len(report.Modules) > 0 {
		fmt.Fprintf(w, "**Modules:** %s (findings outside these modules are not scored)  \n", strings.Join(report.Modules, ", "))
	}
	fmt.Fprintf(w, "**Overall Score:** %s %.0f/100\n\n", scoreBar(report.OverallScore), report.OverallScore)

	// Summary
	fmt.Fprintf(w, "## Summary\n\n")
	fmt.Fprintf(w, "| Metric | Count |\n")
	fmt.Fprintf(w, "|--------|-------|\n")
	fmt.Fprintf(w, "| Errors | %d |\n", report.Summary.Errors)
	fmt.Fprintf(w, "| Warnings | %d |\n", report.Summary.Warnings)
	fmt.Fprintf(w, "| Info | %d |\n", report.Summary.Infos)
	fmt.Fprintf(w, "| **Total** | **%d** |\n", report.Summary.Total)
	if report.Accepted > 0 {
		fmt.Fprintf(w, "| Accepted (brain decisions, not scored) | %d |\n", report.Accepted)
	}
	fmt.Fprintln(w)

	// Category scores
	fmt.Fprintf(w, "## Category Scores\n\n")
	fmt.Fprintf(w, "| Category | Score | Errors | Warnings | Info |\n")
	fmt.Fprintf(w, "|----------|-------|--------|----------|------|\n")
	for _, cat := range report.Categories {
		fmt.Fprintf(w, "| %s | %s %.0f | %d | %d | %d |\n",
			cat.Name, scoreBar(cat.Score), cat.Score,
			cat.Errors, cat.Warnings, cat.Infos)
	}
	fmt.Fprintln(w)

	// Top recommendations per category
	fmt.Fprintf(w, "## Recommendations\n\n")
	for _, cat := range report.Categories {
		if len(cat.TopActions) == 0 {
			continue
		}
		fmt.Fprintf(w, "### %s\n\n", cat.Name)
		for _, action := range cat.TopActions {
			fmt.Fprintf(w, "- %s\n", action)
		}
		fmt.Fprintln(w)
	}

	// Detailed violations
	if len(report.Violations) > 0 {
		fmt.Fprintf(w, "## Detailed Findings\n\n")
		fmt.Fprintf(w, "| Rule | Severity | Location | Message |\n")
		fmt.Fprintf(w, "|------|----------|----------|--------|\n")
		for _, v := range report.Violations {
			loc := v.Location.QualifiedName()
			if loc == "" {
				loc = v.Location.DocumentName
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n",
				v.RuleID, v.Severity, loc, v.Message)
		}
		fmt.Fprintln(w)
	}

	if len(report.RuleFailures) > 0 {
		fmt.Fprintf(w, "## Rules That Could Not Run\n\n")
		fmt.Fprintf(w, "Not counted in the score or the summary: these are problems with the lint tooling, not the project.\n\n")
		fmt.Fprintf(w, "| Rule | Severity | Problem |\n")
		fmt.Fprintf(w, "|------|----------|---------|\n")
		for _, v := range report.RuleFailures {
			fmt.Fprintf(w, "| %s | %s | %s |\n", v.RuleID, v.Severity, v.Message)
		}
		fmt.Fprintln(w)
	}

	return nil
}

// scoreBar returns a Unicode progress bar for a score (0-100).
func scoreBar(score float64) string {
	filled := int(score / 10)
	empty := 10 - filled
	if filled < 0 {
		filled = 0
	}
	if empty < 0 {
		empty = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}

// --- JSON Formatter ---

// JSONReportFormatter outputs the report as JSON.
type JSONReportFormatter struct{}

// JSONReport is the JSON output structure for a report.
type JSONReport struct {
	ProjectName  string          `json:"projectName"`
	Date         string          `json:"date"`
	Modules      []string        `json:"modules,omitempty"`
	OverallScore float64         `json:"overallScore"`
	Accepted     int             `json:"accepted,omitempty"`
	Summary      JSONSummary     `json:"summary"`
	Categories   []CategoryScore `json:"categories"`
	Violations   []JSONViolation `json:"violations"`
	// RuleFailures are rules that could not run; excluded from the score and
	// the summary (ako/mxcli#952).
	RuleFailures []JSONViolation `json:"ruleFailures,omitempty"`
}

// JSONSummary is the summary in JSON format.
type JSONSummary struct {
	Total    int `json:"total"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

func (f *JSONReportFormatter) FormatReport(report *Report, w io.Writer) error {
	jr := JSONReport{
		ProjectName:  report.ProjectName,
		Date:         report.Date,
		Modules:      report.Modules,
		OverallScore: report.OverallScore,
		Accepted:     report.Accepted,
		Summary: JSONSummary{
			Total:    report.Summary.Total,
			Errors:   report.Summary.Errors,
			Warnings: report.Summary.Warnings,
			Infos:    report.Summary.Infos,
		},
		Categories: report.Categories,
	}

	for _, v := range report.Violations {
		jr.Violations = append(jr.Violations, JSONViolation{
			RuleID:       v.RuleID,
			Severity:     v.Severity.String(),
			Message:      v.Message,
			Module:       v.Location.Module,
			Document:     v.Location.DocumentName,
			DocumentType: v.Location.DocumentType,
			DocumentID:   v.Location.DocumentID,
			Suggestion:   v.Suggestion,
		})
	}

	for _, v := range report.RuleFailures {
		jr.RuleFailures = append(jr.RuleFailures, JSONViolation{
			RuleID:     v.RuleID,
			Severity:   v.Severity.String(),
			Message:    v.Message,
			Suggestion: v.Suggestion,
		})
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jr)
}

// --- HTML Formatter ---

// HTMLReportFormatter outputs the report as standalone HTML.
type HTMLReportFormatter struct{}

func (f *HTMLReportFormatter) FormatReport(report *Report, w io.Writer) error {
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Mendix Best Practices Report - %s</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; margin: 2rem; color: #333; max-width: 1200px; margin: 2rem auto; }
  h1 { color: #0a3d8f; border-bottom: 2px solid #0a3d8f; padding-bottom: 0.5rem; }
  h2 { color: #1a5bb5; margin-top: 2rem; }
  table { border-collapse: collapse; width: 100%%; margin-bottom: 1rem; }
  th, td { border: 1px solid #ddd; padding: 8px 12px; text-align: left; }
  th { background-color: #f4f6f8; font-weight: 600; }
  tr:nth-child(even) { background-color: #f9f9f9; }
  .score-bar { display: inline-block; width: 120px; height: 16px; background: #eee; border-radius: 4px; overflow: hidden; vertical-align: middle; }
  .score-fill { height: 100%%; border-radius: 4px; }
  .score-good { background: #28a745; }
  .score-ok { background: #ffc107; }
  .score-bad { background: #dc3545; }
  .overall { font-size: 2rem; font-weight: bold; margin: 1rem 0; }
  .severity-error { color: #dc3545; font-weight: bold; }
  .severity-warning { color: #ffc107; }
  .severity-info { color: #17a2b8; }
  .meta { color: #666; margin-bottom: 0.25rem; }
  .recommendation { background: #f8f9fa; padding: 0.5rem 1rem; margin: 0.25rem 0; border-left: 3px solid #0a3d8f; }
</style>
</head>
<body>
`, report.ProjectName)

	fmt.Fprintf(w, "<h1>Mendix Best Practices Report</h1>\n")
	fmt.Fprintf(w, "<p class='meta'><strong>Project:</strong> %s</p>\n", report.ProjectName)
	fmt.Fprintf(w, "<p class='meta'><strong>Date:</strong> %s</p>\n", report.Date)
	if len(report.Modules) > 0 {
		fmt.Fprintf(w, "<p class='meta'><strong>Modules:</strong> %s (findings outside these modules are not scored)</p>\n",
			html.EscapeString(strings.Join(report.Modules, ", ")))
	}

	// Overall score
	scoreClass := "score-good"
	if report.OverallScore < 50 {
		scoreClass = "score-bad"
	} else if report.OverallScore < 75 {
		scoreClass = "score-ok"
	}
	fmt.Fprintf(w, "<p class='overall'>Overall Score: <span class='score-bar'><span class='score-fill %s' style='width: %.0f%%'></span></span> %.0f/100</p>\n",
		scoreClass, report.OverallScore, report.OverallScore)

	// Summary
	fmt.Fprintf(w, "<h2>Summary</h2>\n<table>\n")
	fmt.Fprintf(w, "<tr><th>Metric</th><th>Count</th></tr>\n")
	fmt.Fprintf(w, "<tr><td class='severity-error'>Errors</td><td>%d</td></tr>\n", report.Summary.Errors)
	fmt.Fprintf(w, "<tr><td class='severity-warning'>Warnings</td><td>%d</td></tr>\n", report.Summary.Warnings)
	fmt.Fprintf(w, "<tr><td class='severity-info'>Info</td><td>%d</td></tr>\n", report.Summary.Infos)
	fmt.Fprintf(w, "<tr><td><strong>Total</strong></td><td><strong>%d</strong></td></tr>\n", report.Summary.Total)
	if report.Accepted > 0 {
		fmt.Fprintf(w, "<tr><td>Accepted (brain decisions, not scored)</td><td>%d</td></tr>\n", report.Accepted)
	}
	fmt.Fprintf(w, "</table>\n")

	// Category scores
	fmt.Fprintf(w, "<h2>Category Scores</h2>\n<table>\n")
	fmt.Fprintf(w, "<tr><th>Category</th><th>Score</th><th>Errors</th><th>Warnings</th><th>Info</th></tr>\n")
	for _, cat := range report.Categories {
		sc := "score-good"
		if cat.Score < 50 {
			sc = "score-bad"
		} else if cat.Score < 75 {
			sc = "score-ok"
		}
		fmt.Fprintf(w, "<tr><td>%s</td><td><span class='score-bar'><span class='score-fill %s' style='width: %.0f%%'></span></span> %.0f</td><td>%d</td><td>%d</td><td>%d</td></tr>\n",
			cat.Name, sc, cat.Score, cat.Score, cat.Errors, cat.Warnings, cat.Infos)
	}
	fmt.Fprintf(w, "</table>\n")

	// Recommendations
	hasRecs := false
	for _, cat := range report.Categories {
		if len(cat.TopActions) > 0 {
			hasRecs = true
			break
		}
	}
	if hasRecs {
		fmt.Fprintf(w, "<h2>Recommendations</h2>\n")
		for _, cat := range report.Categories {
			if len(cat.TopActions) == 0 {
				continue
			}
			fmt.Fprintf(w, "<h3>%s</h3>\n", cat.Name)
			for _, action := range cat.TopActions {
				fmt.Fprintf(w, "<div class='recommendation'>%s</div>\n", action)
			}
		}
	}

	// Violations table
	if len(report.Violations) > 0 {
		fmt.Fprintf(w, "<h2>Detailed Findings</h2>\n<table>\n")
		fmt.Fprintf(w, "<tr><th>Rule</th><th>Severity</th><th>Location</th><th>Message</th></tr>\n")
		for _, v := range report.Violations {
			sevClass := "severity-info"
			switch v.Severity {
			case SeverityError:
				sevClass = "severity-error"
			case SeverityWarning:
				sevClass = "severity-warning"
			}
			loc := v.Location.QualifiedName()
			if loc == "" {
				loc = v.Location.DocumentName
			}
			fmt.Fprintf(w, "<tr><td>%s</td><td class='%s'>%s</td><td>%s</td><td>%s</td></tr>\n",
				v.RuleID, sevClass, v.Severity, loc, v.Message)
		}
		fmt.Fprintf(w, "</table>\n")
	}

	if len(report.RuleFailures) > 0 {
		fmt.Fprintf(w, "<h2>Rules That Could Not Run</h2>\n")
		fmt.Fprintf(w, "<p>Not counted in the score or the summary: these are problems with the lint tooling, not the project.</p>\n<table>\n")
		fmt.Fprintf(w, "<tr><th>Rule</th><th>Severity</th><th>Problem</th></tr>\n")
		for _, v := range report.RuleFailures {
			fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				html.EscapeString(v.RuleID), v.Severity, html.EscapeString(v.Message))
		}
		fmt.Fprintf(w, "</table>\n")
	}

	fmt.Fprintf(w, "</body>\n</html>\n")
	return nil
}
