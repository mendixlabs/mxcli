// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"fmt"
	"html"
	"io"
	"sort"
	"strings"
)

// ConsistencyCheck is the result of Mendix's own consistency check (`mx check`)
// as the scored report shows it (ako/mxcli#2423).
//
// The lint rules measure conventions; they do not say whether the project
// builds. A project with consistency errors cannot be deployed, yet it could
// score well, so the report states the check's verdict above the score. The
// check's findings are not scored: one CE error is a project that does not
// build, which no category weight can express, and folding it into the score
// would let a few lint fixes hide it.
type ConsistencyCheck struct {
	// Ran is false when the check was not run; NotRun then says why and how
	// to run it. A report never shows a check it did not run as clean.
	Ran    bool   `json:"ran"`
	NotRun string `json:"notRun,omitempty"`
	// Source names what the result came from: the mx binary, or a results
	// file passed with --mx-check-results.
	Source       string `json:"source,omitempty"`
	Errors       int    `json:"errors"`
	Warnings     int    `json:"warnings"`
	Deprecations int    `json:"deprecations"`
	// Incomplete is set when the parsed findings disagree with the error
	// count mx printed, or mx did not print it: the list may be missing
	// findings, so the verdict cannot be "builds".
	Incomplete string               `json:"incomplete,omitempty"`
	Findings   []ConsistencyFinding `json:"findings,omitempty"`
}

// ConsistencyFinding is one `mx check` diagnostic.
type ConsistencyFinding struct {
	Severity string `json:"severity"` // error | warning | deprecation
	Code     string `json:"code"`     // CE0117, CW…, CD…
	Message  string `json:"message"`
	// Module, Document ("Microflow 'ACT_X'") and Element ("End event") say
	// where; a project-level finding has none.
	Module   string `json:"module,omitempty"`
	Document string `json:"document,omitempty"`
	Element  string `json:"element,omitempty"`
}

// where is the finding's document, "Module: Microflow 'ACT_X'", or "" for a
// project-level finding.
func (f ConsistencyFinding) where() string {
	switch {
	case f.Module != "" && f.Document != "":
		return f.Module + ": " + f.Document
	case f.Document != "":
		return f.Document
	default:
		return f.Module
	}
}

// Verdict is the one-line answer to "does it build?".
func (c *ConsistencyCheck) Verdict() string {
	switch {
	case c == nil || !c.Ran:
		return "not checked"
	case c.Errors > 0:
		return fmt.Sprintf("no — %d consistency error(s)", c.Errors)
	case c.Incomplete != "":
		return "unknown — the check's output could not be read in full"
	default:
		return "yes"
	}
}

// Builds reports whether the check ran, read completely, and found no error.
func (c *ConsistencyCheck) Builds() bool {
	return c != nil && c.Ran && c.Errors == 0 && c.Incomplete == ""
}

// consistencyLead is the sentence under the verdict.
func (c *ConsistencyCheck) consistencyLead() string {
	switch {
	case c == nil || !c.Ran:
		reason := "it was not run"
		if c != nil && c.NotRun != "" {
			reason = c.NotRun
		}
		return "Mendix's consistency check (`mx check`) was not run, so this report does not say whether the project builds: " + reason
	case c.Errors > 0:
		return "The project does not build: Mendix's consistency check (`mx check`) found errors, and a project with errors cannot be deployed. The score covers the lint rules only."
	case c.Incomplete != "":
		return "The consistency check ran, but " + c.Incomplete + "."
	default:
		return "Mendix's consistency check (`mx check`) found no errors."
	}
}

func (c *ConsistencyCheck) counts() string {
	return fmt.Sprintf("%d error(s), %d warning(s), %d deprecation(s)", c.Errors, c.Warnings, c.Deprecations)
}

// ConsistencyGroup is the findings of one code: a project with a stale theme or
// widget set has hundreds of the same one, which a row each would bury.
type ConsistencyGroup struct {
	Severity  string
	Code      string
	Count     int
	Message   string   // the first finding's; they differ only in detail
	Locations []string // the first few distinct documents
	More      int      // distinct locations not listed
}

// maxGroupLocations is how many locations a group lists in markdown and HTML;
// JSON carries every finding.
const maxGroupLocations = 5

// Groups returns the findings grouped by severity and code: errors first, then
// warnings and deprecations, each by count.
func (c *ConsistencyCheck) Groups() []ConsistencyGroup {
	at := map[string]int{}
	seen := map[string]map[string]bool{}
	var out []ConsistencyGroup
	for _, f := range c.Findings {
		k := f.Severity + "\x00" + f.Code
		i, ok := at[k]
		if !ok {
			out = append(out, ConsistencyGroup{Severity: f.Severity, Code: f.Code, Message: f.Message})
			i = len(out) - 1
			at[k] = i
			seen[k] = map[string]bool{}
		}
		g := &out[i]
		g.Count++
		if loc := f.where(); loc != "" && !seen[k][loc] {
			seen[k][loc] = true
			if len(g.Locations) < maxGroupLocations {
				g.Locations = append(g.Locations, loc)
			} else {
				g.More++
			}
		}
	}
	rank := map[string]int{"error": 0, "warning": 1, "deprecation": 2}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Severity] != rank[out[j].Severity] {
			return rank[out[i].Severity] < rank[out[j].Severity]
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func (g ConsistencyGroup) where() string {
	s := strings.Join(g.Locations, "; ")
	if g.More > 0 {
		s += fmt.Sprintf("; and %d more", g.More)
	}
	return s
}

// hints says what to do about the codes whose cause is usually not the model's
// logic.
func (c *ConsistencyCheck) hints() []string {
	var out []string
	for _, g := range c.Groups() {
		if g.Code == "CE0463" {
			out = append(out, "CE0463 is usually a widget whose definition changed (a widget package upgrade): `mxcli docker check` labels which ones `mxcli fix widgets` clears.")
		}
	}
	return out
}

// wholeProjectNote says that the check is not narrowed by --modules: mx checks
// the project, and a project that does not build does not build in any module.
const wholeProjectNote = "The check covers the whole project, not only the selected modules."

func writeConsistencyMarkdown(w io.Writer, c *ConsistencyCheck, modules []string) {
	if c == nil {
		return
	}
	fmt.Fprintf(w, "## Consistency Check\n\n")
	fmt.Fprintf(w, "%s\n\n", c.consistencyLead())
	if c.Ran && len(modules) > 0 {
		fmt.Fprintf(w, "%s\n\n", wholeProjectNote)
	}
	if !c.Ran {
		return
	}
	if c.Source != "" {
		fmt.Fprintf(w, "Source: %s. %s.\n\n", c.Source, c.counts())
	} else {
		fmt.Fprintf(w, "%s.\n\n", c.counts())
	}
	if len(c.Findings) == 0 {
		return
	}
	fmt.Fprintf(w, "| Severity | Code | Count | Message | Where |\n")
	fmt.Fprintf(w, "|----------|------|-------|---------|-------|\n")
	for _, g := range c.Groups() {
		fmt.Fprintf(w, "| %s | %s | %d | %s | %s |\n", g.Severity, g.Code, g.Count, mdCell(g.Message), mdCell(g.where()))
	}
	fmt.Fprintln(w)
	for _, h := range c.hints() {
		fmt.Fprintf(w, "%s\n\n", h)
	}
	fmt.Fprintf(w, "Every finding, with its element, is in `--format json`; `mxcli docker check -p <app.mpr>` prints them with mx's own wording.\n\n")
}

func writeConsistencyHTML(w io.Writer, c *ConsistencyCheck, modules []string) {
	if c == nil {
		return
	}
	fmt.Fprintf(w, "<h2>Consistency Check</h2>\n")
	cls := "recommendation"
	if c.Errors > 0 {
		cls = "recommendation severity-error"
	}
	fmt.Fprintf(w, "<p class='%s'>%s</p>\n", cls, html.EscapeString(strings.ReplaceAll(c.consistencyLead(), "`", "")))
	if c.Ran && len(modules) > 0 {
		fmt.Fprintf(w, "<p class='meta'>%s</p>\n", wholeProjectNote)
	}
	if !c.Ran {
		return
	}
	src := ""
	if c.Source != "" {
		src = "Source: " + c.Source + ". "
	}
	fmt.Fprintf(w, "<p class='meta'>%s%s.</p>\n", html.EscapeString(src), html.EscapeString(c.counts()))
	if len(c.Findings) == 0 {
		return
	}
	fmt.Fprintf(w, "<table>\n<tr><th>Severity</th><th>Code</th><th>Count</th><th>Message</th><th>Where</th></tr>\n")
	for _, g := range c.Groups() {
		sev := "severity-info"
		switch g.Severity {
		case "error":
			sev = "severity-error"
		case "warning":
			sev = "severity-warning"
		}
		fmt.Fprintf(w, "<tr><td class='%s'>%s</td><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>\n",
			sev, html.EscapeString(g.Severity), html.EscapeString(g.Code), g.Count, html.EscapeString(g.Message), html.EscapeString(g.where()))
	}
	fmt.Fprintf(w, "</table>\n")
	for _, h := range c.hints() {
		fmt.Fprintf(w, "<p>%s</p>\n", html.EscapeString(strings.ReplaceAll(h, "`", "")))
	}
	fmt.Fprintf(w, "<p class='meta'>Every finding is in --format json; mxcli docker check prints them with mx's own wording.</p>\n")
}

// mdCell keeps a value inside one markdown table cell.
func mdCell(s string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(s)
}
