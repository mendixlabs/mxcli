// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// ped_check_errors has two input shapes, and the server does not say which by
// version alone (serverInfo.version was frozen at 1.0.0 through 11.14, and
// reads "11.15.0-rc.4" on 11.15):
//
//   - up to 11.14: `documents: [{documentType, documentName}]` (required), schema
//     additionalProperties:false, so anything else is rejected outright;
//   - 11.15+: `filters: {documentType, documentNamePrefix, severities}` plus
//     `pagination: {checkId, offset, size}`, schema additionalProperties-permissive.
//
// The second is the trap: 11.15 ACCEPTS the old `documents` argument and ignores
// it, so the check runs project-wide (measured: an empty enumeration elsewhere in
// the module failed an entity create; a nonexistent document came back "No
// errors found."). The shape is chosen from the tool's advertised inputSchema
// (SupportsToolArg), never from the version string.
//
// 11.15 answers with a listing rather than free text:
//
//	Listing problems 1-2 (out of 2). Check ID: 6
//	'MyFirstModule.Zz_BrokenEnum' (Enumerations$Enumeration):
//	- [error] Missing enumeration values. (at locations: -)
//	'MyFirstModule' (DomainModels$DomainModel):
//	- [error] Duplicate name ...
//
// `documentNamePrefix` is a PREFIX ('M.Colors' also matches 'M.ColorsOld'), the
// first response holds at most 100 problems, and a page request must repeat the
// filters (a page without them lists the whole project's errors). So the
// listing is always scoped here by its unit headers — exact name and type — and
// every page is fetched. That also covers the fallback: when the tools/list
// probe cannot answer, the old form is sent, and an 11.15 server's unscoped
// listing is still narrowed to the checked document.

// checkErrorsPageSize is the largest page ped_check_errors serves (schema max).
const checkErrorsPageSize = 100

// checkErrorsMaxRefetch bounds re-fetches after a stale page window (the model
// changed between pages), so a model that never settles cannot loop forever.
const checkErrorsMaxRefetch = 3

var (
	checkListingHeader = regexp.MustCompile(`^Listing problems (\d+)-(\d+) \(out of (\d+)\)\. Check ID: (\d+)`)
	checkUnitHeader    = regexp.MustCompile(`^'(.*)' \(([A-Za-z]+\$[A-Za-z]+)\):\s*$`)
)

// checkDocumentNow asks for the current verdict without waiting for the error
// list to settle — which is why only pedCheckDocument should call it.
func (b *Backend) checkDocumentNow(docType, docName string) error {
	var base map[string]any
	if b.client.SupportsToolArg("ped_check_errors", "filters") {
		base = map[string]any{"filters": map[string]any{
			"documentType":       docType,
			"documentNamePrefix": docName,
		}}
	} else {
		base = map[string]any{"documents": []map[string]any{
			{"documentType": docType, "documentName": docName},
		}}
	}

	for attempt := 0; ; attempt++ {
		problems, stale, err := b.collectCheckErrors(base, docName, func(text string) []string {
			return scopeCheckListing(text, docType, docName)
		})
		if err != nil {
			return err
		}
		if stale {
			if attempt < checkErrorsMaxRefetch {
				continue
			}
			return fmt.Errorf("validation of %s did not settle: Studio Pro's error list kept changing between pages", docName)
		}
		if len(problems) > 0 {
			return fmt.Errorf("validation failed for %s: %s", docName, strings.Join(problems, "\n"))
		}
		return nil
	}
}

// collectCheckErrors runs one complete check (first response plus every page) and
// returns the problem lines scope keeps from each listing page. docName names what
// was checked in errors. stale reports that a page window went stale and the
// whole check must be asked again.
func (b *Backend) collectCheckErrors(base map[string]any, docName string, scope func(string) []string) (problems []string, stale bool, err error) {
	res, err := b.client.CallTool("ped_check_errors", base)
	if err != nil {
		return nil, false, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return nil, false, fmt.Errorf("validation failed for %s: %s", docName, text)
	}
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "No errors found") {
		return nil, false, nil
	}
	m := checkListingHeader.FindStringSubmatch(trimmed)
	if m == nil {
		// The pre-11.15 free-text answer (or anything unrecognised): any text
		// other than the clean verdict is the document's error. An answer that
		// cannot be read must never pass as clean.
		return []string{text}, false, nil
	}
	to, _ := strconv.Atoi(m[2])
	total, _ := strconv.Atoi(m[3])
	checkID, _ := strconv.Atoi(m[4])
	problems = scope(trimmed)

	for offset := to; offset < total; {
		args := make(map[string]any, len(base)+1)
		for k, v := range base {
			args[k] = v
		}
		args["pagination"] = map[string]any{"checkId": checkID, "offset": offset, "size": checkErrorsPageSize}
		res, err := b.client.CallTool("ped_check_errors", args)
		if err != nil {
			return nil, false, err
		}
		page := strings.TrimSpace(pedStripReminder(res.Text))
		if res.IsError {
			return nil, false, fmt.Errorf("validation failed for %s: %s", docName, page)
		}
		pm := checkListingHeader.FindStringSubmatch(page)
		if pm == nil && strings.Contains(page, "stale") {
			return nil, true, nil
		}
		if pm == nil {
			// "No errors found in the requested page window." — the listing
			// shrank under us; whatever was listed is all there is.
			break
		}
		next, _ := strconv.Atoi(pm[2])
		problems = append(problems, scope(page)...)
		if next <= offset {
			break // no progress; never loop
		}
		offset = next
	}
	return problems, false, nil
}

// scopeCheckListing returns the problem lines of an 11.15 listing that belong to
// the checked document, matched on its exact name and type. A problem line
// belongs to the unit header above it; lines of other units are dropped.
func scopeCheckListing(text, docType, docName string) []string {
	var out []string
	mine := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if checkListingHeader.MatchString(line) {
			continue
		}
		if h := checkUnitHeader.FindStringSubmatch(line); h != nil {
			mine = h[1] == docName && h[2] == docType
			continue
		}
		if strings.HasPrefix(line, "Project-level ") {
			mine = false
			continue
		}
		if mine && strings.TrimSpace(line) != "" {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

// CheckProject runs ped_check_errors over the whole project (--mcp-check) and
// returns its listing: each unit header followed by that unit's problems. Only
// the 11.15+ shape can ask project-wide — the older `documents` argument is
// required and names the documents to check.
func (b *Backend) CheckProject() ([]string, error) {
	if b.client == nil || !b.client.SupportsToolArg("ped_check_errors", "filters") {
		return nil, fmt.Errorf("a project-wide check needs Studio Pro 11.15+ (ped_check_errors filters)")
	}
	for attempt := 0; ; attempt++ {
		problems, stale, err := b.collectCheckErrors(map[string]any{}, "the project", projectCheckListing)
		if err != nil {
			return nil, err
		}
		if !stale {
			return problems, nil
		}
		if attempt >= checkErrorsMaxRefetch {
			return nil, fmt.Errorf("the project check did not settle: Studio Pro's error list kept changing between pages")
		}
	}
}

// projectCheckListing keeps every line of an 11.15 listing but its page header.
func projectCheckListing(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if checkListingHeader.MatchString(line) || strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// writeCheckReport prints a project check's listing, or that it is clean.
func writeCheckReport(w io.Writer, listing []string) {
	if len(listing) == 0 {
		fmt.Fprintln(w, "model check: no errors found")
		return
	}
	fmt.Fprintln(w, "model check:")
	for _, line := range listing {
		fmt.Fprintf(w, "  %s\n", line)
	}
}
