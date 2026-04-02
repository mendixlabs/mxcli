package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// CheckError represents a single mx check diagnostic.
type CheckError struct {
	Severity     string // "ERROR", "WARNING", or "DEPRECATION"
	Code         string // e.g. "CE0001"
	Message      string
	DocumentName string // e.g. "Page 'P_ComboBox_Enum'" (from JSON output)
	ElementName  string // e.g. "Property 'Association' of combo box 'cmbPriority'"
	ModuleName   string // e.g. "MyFirstModule"
	ElementID    string // unique element identifier for deduplication
}

// CheckGroup represents errors grouped by error code.
type CheckGroup struct {
	Code     string
	Severity string
	Message  string
	Items    []CheckGroupItem
}

// CheckGroupItem represents a deduplicated location within a group.
type CheckGroupItem struct {
	DocLocation string // formatted as "Module.DocName (Type)"
	ElementName string
	ElementID   string
	Count       int // occurrences of the same element-id
}

// CheckNavLocation represents a unique document location for error navigation.
type CheckNavLocation struct {
	ModuleName   string
	DocumentName string // raw document name from mx check (e.g. "Page 'P_ComboBox'")
	Code         string // error code (e.g. "CE1613")
	Message      string
}

// NavigateToDocMsg requests navigation to a document in the tree.
type NavigateToDocMsg struct {
	ModuleName   string
	DocumentName string // raw document name (e.g. "Page 'P_ComboBox'")
	NavIndex     int    // index into checkNavLocations for ]e/[e navigation
}

// extractCheckNavLocations builds a flat list of unique document locations from check errors.
// Each unique (ModuleName, DocumentName) pair appears once, with the code/message from the first occurrence.
func extractCheckNavLocations(errors []CheckError) []CheckNavLocation {
	type locKey struct{ mod, doc string }
	seen := map[locKey]bool{}
	var locations []CheckNavLocation
	for _, e := range errors {
		if e.DocumentName == "" {
			continue
		}
		key := locKey{e.ModuleName, e.DocumentName}
		if seen[key] {
			continue
		}
		seen[key] = true
		locations = append(locations, CheckNavLocation{
			ModuleName:   e.ModuleName,
			DocumentName: e.DocumentName,
			Code:         e.Code,
			Message:      e.Message,
		})
	}
	return locations
}

// docNameToQualifiedName converts a module name and raw document name
// (e.g. "Page 'P_ComboBox'") to a qualified name (e.g. "MyModule.P_ComboBox").
func docNameToQualifiedName(moduleName, documentName string) string {
	// documentName format: "Type 'Name'" — extract the name part
	if idx := strings.Index(documentName, " '"); idx > 0 {
		docName := strings.TrimSuffix(documentName[idx+2:], "'")
		if moduleName != "" {
			return moduleName + "." + docName
		}
		return docName
	}
	if moduleName != "" {
		return moduleName + "." + documentName
	}
	return documentName
}

// MxCheckResultMsg carries the result of an async mx check run.
type MxCheckResultMsg struct {
	Errors []CheckError
	Err    error
}

// MxCheckStartMsg signals that a check run has started.
type MxCheckStartMsg struct{}

// MxCheckRerunMsg requests a manual re-run of mx check (e.g. from overlay "r" key).
type MxCheckRerunMsg struct{}

// mxCheckJSON mirrors the JSON structure produced by `mx check -j -w -d`.
type mxCheckJSON struct {
	Errors       []mxCheckEntry `json:"errors"`
	Warnings     []mxCheckEntry `json:"warnings"`
	Deprecations []mxCheckEntry `json:"deprecations"`
}

type mxCheckEntry struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Locations []mxCheckLocation `json:"locations"`
}

type mxCheckLocation struct {
	ModuleName   string `json:"module-name"`
	DocumentName string `json:"document-name"`
	ElementName  string `json:"element-name"`
	ElementID    string `json:"element-id"`
	UnitID       string `json:"unit-id"`
}

// runMxCheck returns a tea.Cmd that runs mx check asynchronously.
// Uses `-j` for JSON output to get document-level location information.
func runMxCheck(projectPath string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return MxCheckStartMsg{} },
		func() tea.Msg {
			mxPath, err := docker.ResolveMx("")
			if err != nil {
				Trace("checker: mx not found: %v", err)
				return MxCheckResultMsg{Err: err}
			}

			jsonFile, err := os.CreateTemp("", "mx-check-*.json")
			if err != nil {
				return MxCheckResultMsg{Err: err}
			}
			jsonPath := jsonFile.Name()
			jsonFile.Close()
			defer os.Remove(jsonPath)

			Trace("checker: running %s check %s -j %s -w -d", mxPath, projectPath, jsonPath)
			cmd := exec.Command(mxPath, "check", projectPath, "-j", jsonPath, "-w", "-d")
			_, runErr := cmd.CombinedOutput()

			checkErrors, parseErr := parseCheckJSON(jsonPath)
			if parseErr != nil {
				Trace("checker: JSON parse error: %v", parseErr)
				// If JSON parsing fails, return the run error
				if runErr != nil {
					return MxCheckResultMsg{Err: runErr}
				}
				return MxCheckResultMsg{Err: parseErr}
			}

			Trace("checker: done, %d diagnostics", len(checkErrors))

			// mx check returns non-zero exit code when there are errors,
			// but we still want to show the parsed errors.
			if runErr != nil && len(checkErrors) == 0 {
				return MxCheckResultMsg{Err: runErr}
			}
			return MxCheckResultMsg{Errors: checkErrors}
		},
	)
}

// parseCheckJSON reads the JSON file produced by `mx check -j` and converts to CheckError slice.
func parseCheckJSON(jsonPath string) ([]CheckError, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var result mxCheckJSON
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	var checkErrors []CheckError
	for _, entry := range result.Errors {
		checkErrors = append(checkErrors, entryToCheckError(entry, "ERROR"))
	}
	for _, entry := range result.Warnings {
		checkErrors = append(checkErrors, entryToCheckError(entry, "WARNING"))
	}
	for _, entry := range result.Deprecations {
		checkErrors = append(checkErrors, entryToCheckError(entry, "DEPRECATION"))
	}
	return checkErrors, nil
}

func entryToCheckError(entry mxCheckEntry, severity string) CheckError {
	ce := CheckError{
		Severity: severity,
		Code:     entry.Code,
		Message:  entry.Message,
	}
	if len(entry.Locations) > 0 {
		loc := entry.Locations[0]
		ce.ModuleName = loc.ModuleName
		ce.DocumentName = loc.DocumentName
		ce.ElementName = loc.ElementName
		ce.ElementID = loc.ElementID
	}
	return ce
}

// groupCheckErrors groups errors by Code and deduplicates by element-id within each group.
func groupCheckErrors(errors []CheckError) []CheckGroup {
	// Preserve insertion order of codes
	var codeOrder []string
	groupByCode := make(map[string]*CheckGroup)

	for _, e := range errors {
		g, exists := groupByCode[e.Code]
		if !exists {
			g = &CheckGroup{
				Code:     e.Code,
				Severity: e.Severity,
				Message:  e.Message,
			}
			groupByCode[e.Code] = g
			codeOrder = append(codeOrder, e.Code)
		}

		docLoc := formatDocLocation(e.ModuleName, e.DocumentName)

		// Deduplicate by element-id within the group
		dedupKey := e.ElementID
		if dedupKey == "" {
			// No element-id: use doc location + element name as fallback key
			dedupKey = docLoc + "|" + e.ElementName
		}

		found := false
		for i := range g.Items {
			itemKey := g.Items[i].ElementID
			if itemKey == "" {
				itemKey = g.Items[i].DocLocation + "|" + g.Items[i].ElementName
			}
			if itemKey == dedupKey {
				g.Items[i].Count++
				found = true
				break
			}
		}
		if !found {
			g.Items = append(g.Items, CheckGroupItem{
				DocLocation: docLoc,
				ElementName: e.ElementName,
				ElementID:   e.ElementID,
				Count:       1,
			})
		}
	}

	groups := make([]CheckGroup, 0, len(codeOrder))
	for _, code := range codeOrder {
		groups = append(groups, *groupByCode[code])
	}
	return groups
}

// countBySeverity returns error, warning, and deprecation counts.
func countBySeverity(errors []CheckError) (errorCount, warningCount, deprecationCount int) {
	for _, e := range errors {
		switch e.Severity {
		case "ERROR":
			errorCount++
		case "WARNING":
			warningCount++
		case "DEPRECATION":
			deprecationCount++
		}
	}
	return
}

// renderCheckFilterTitle returns the overlay title with filter indicator.
// Examples: "mx check  [All: 8E 2W 1D]" or "mx check  [Errors: 8]"
func renderCheckFilterTitle(errors []CheckError, filter string) string {
	if errors == nil || len(errors) == 0 {
		return "mx check"
	}
	ec, wc, dc := countBySeverity(errors)
	var indicator string
	switch filter {
	case "error":
		indicator = "[Errors: " + itoa(ec) + "]"
	case "warning":
		indicator = "[Warnings: " + itoa(wc) + "]"
	case "deprecation":
		indicator = "[Deprecations: " + itoa(dc) + "]"
	default: // "all"
		var parts []string
		if ec > 0 {
			parts = append(parts, itoa(ec)+"E")
		}
		if wc > 0 {
			parts = append(parts, itoa(wc)+"W")
		}
		if dc > 0 {
			parts = append(parts, itoa(dc)+"D")
		}
		indicator = "[All: " + strings.Join(parts, " ") + "]"
	}
	return "mx check  " + indicator
}

// nextCheckFilter cycles the filter: all → error → warning → deprecation → all.
func nextCheckFilter(current string) string {
	switch current {
	case "all":
		return "error"
	case "error":
		return "warning"
	case "warning":
		return "deprecation"
	case "deprecation":
		return "all"
	default:
		return "all"
	}
}

// severityMatchesFilter checks if a severity matches a filter value.
func severityMatchesFilter(severity, filter string) bool {
	switch filter {
	case "error":
		return severity == "ERROR"
	case "warning":
		return severity == "WARNING"
	case "deprecation":
		return severity == "DEPRECATION"
	default:
		return true
	}
}

// filterCheckErrors returns only errors matching the given severity filter.
func filterCheckErrors(errors []CheckError, filter string) []CheckError {
	if filter == "all" {
		return errors
	}
	var filtered []CheckError
	for _, e := range errors {
		if severityMatchesFilter(e.Severity, filter) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// renderCheckResults formats check errors for display in an overlay.
// Errors are grouped by code and deduplicated by element-id.
// The filter parameter controls which severities to show: "all", "error", "warning", "deprecation".
func renderCheckResults(errors []CheckError, filter string) string {
	if errors == nil {
		return "No check has been run yet. Changes to the project will trigger an automatic check."
	}
	if len(errors) == 0 {
		return CheckPassStyle.Render("✓ Project check passed — no errors or warnings")
	}

	var sb strings.Builder
	ec, wc, dc := countBySeverity(errors)

	// Summary header
	sb.WriteString(CheckHeaderStyle.Render("mx check Results"))
	sb.WriteString("\n")
	var summaryParts []string
	if ec > 0 {
		summaryParts = append(summaryParts, CheckErrorStyle.Render("● "+itoa(ec)+" errors"))
	}
	if wc > 0 {
		summaryParts = append(summaryParts, CheckWarnStyle.Render("● "+itoa(wc)+" warnings"))
	}
	if dc > 0 {
		summaryParts = append(summaryParts, CheckDeprecStyle.Render("● "+itoa(dc)+" deprecations"))
	}
	sb.WriteString(strings.Join(summaryParts, "  "))
	sb.WriteString("\n\n")

	// Grouped detail lines
	groups := groupCheckErrors(errors)
	for _, g := range groups {
		// Apply filter: skip groups that don't match
		if filter != "all" && !severityMatchesFilter(g.Severity, filter) {
			continue
		}

		var label string
		switch g.Severity {
		case "ERROR":
			label = CheckErrorStyle.Render(g.Code)
		case "WARNING":
			label = CheckWarnStyle.Render(g.Code)
		case "DEPRECATION":
			label = CheckDeprecStyle.Render(g.Code)
		default:
			label = g.Code
		}
		sb.WriteString(label + " — " + g.Message + "\n")

		for _, item := range g.Items {
			countSuffix := ""
			if item.Count > 1 {
				countSuffix = " (x" + itoa(item.Count) + ")"
			}
			sb.WriteString("  " + item.DocLocation + countSuffix + "\n")
			if item.ElementName != "" {
				sb.WriteString("    > " + CheckLocStyle.Render(item.ElementName) + "\n")
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderCheckResultsPlain produces a plain-text summary of check errors for agent responses.
func renderCheckResultsPlain(errors []CheckError) string {
	if len(errors) == 0 {
		return "Project check passed — no errors or warnings"
	}
	var sb strings.Builder
	ec, wc, dc := countBySeverity(errors)
	sb.WriteString(fmt.Sprintf("%d errors, %d warnings, %d deprecations\n\n", ec, wc, dc))
	groups := groupCheckErrors(errors)
	for _, g := range groups {
		sb.WriteString(g.Code + " [" + g.Severity + "] " + g.Message + "\n")
		for _, item := range g.Items {
			countSuffix := ""
			if item.Count > 1 {
				countSuffix = fmt.Sprintf(" (x%d)", item.Count)
			}
			sb.WriteString("  " + item.DocLocation + countSuffix + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// formatDocLocation converts mx JSON document-name (e.g. "Page 'P_ComboBox'")
// into a qualified name like "MyModule.P_ComboBox (Page)".
func formatDocLocation(moduleName, documentName string) string {
	// documentName format: "Type 'Name'" — extract type and name
	if idx := strings.Index(documentName, " '"); idx > 0 {
		docType := documentName[:idx]
		docName := strings.TrimSuffix(documentName[idx+2:], "'")
		qname := docName
		if moduleName != "" {
			qname = moduleName + "." + docName
		}
		return qname + " (" + docType + ")"
	}
	// Fallback: just prefix with module
	if moduleName != "" {
		return moduleName + "." + documentName
	}
	return documentName
}

// formatCheckBadge returns a compact badge string for the status bar.
func formatCheckBadge(errors []CheckError, running bool) string {
	if running {
		return CheckRunningStyle.Render("⟳ checking")
	}
	if errors == nil {
		return "" // no check has run yet
	}
	if len(errors) == 0 {
		return CheckPassStyle.Render("✓")
	}
	ec, wc, dc := countBySeverity(errors)
	var parts []string
	if ec > 0 {
		parts = append(parts, CheckErrorStyle.Render("✗ "+itoa(ec)+"E"))
	}
	if wc > 0 {
		parts = append(parts, CheckWarnStyle.Render(itoa(wc)+"W"))
	}
	if dc > 0 {
		parts = append(parts, CheckDeprecStyle.Render(itoa(dc)+"D"))
	}
	return strings.Join(parts, " ")
}

// renderCheckAnchors builds LLM-friendly structured anchor lines for check results.
// Each line is a key=value record that LLMs can parse from screenshots or clipboard.
func renderCheckAnchors(groups []CheckGroup, errors []CheckError) string {
	ec, wc, dc := countBySeverity(errors)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[mxcli:check] errors=%d warnings=%d deprecations=%d", ec, wc, dc))

	for _, g := range groups {
		for _, item := range g.Items {
			// Extract doc name and type from DocLocation format "Module.Name (Type)"
			docName := item.DocLocation
			docType := ""
			if idx := strings.LastIndex(item.DocLocation, " ("); idx > 0 {
				docName = item.DocLocation[:idx]
				docType = strings.TrimSuffix(item.DocLocation[idx+2:], ")")
			}

			line := fmt.Sprintf("\n[mxcli:check:%s] severity=%s count=%d doc=%s",
				g.Code, g.Severity, item.Count, docName)
			if docType != "" {
				line += " type=" + docType
			}
			if item.ElementName != "" {
				line += " element=" + item.ElementName
			}
			sb.WriteString(line)
		}
	}
	return sb.String()
}
