// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// mainPlaceholderName is the name mxbuild requires exactly one of. It is not a
// choice mxcli makes — see validateLayoutPlaceholders.
const mainPlaceholderName = "Main"

// validateLayoutPlaceholders (MDL081, MDL082) enforces the placeholder rule
// mxbuild applies to every layout.
//
// # This is a rule, not a convention
//
// mxcli's own docs said otherwise — "Which placeholder is 'main' is a naming
// convention (22 of 22 name one `Main`; a page binds by qualified name anyway)"
// — and the write-time guard was written to match that belief: it accepted any
// placeholder under any name. The belief is wrong. Measured on Mendix 11.12.1,
// against a layout NO PAGE USES, so none of it depends on a page binding:
//
//	placeholder Content            → [error] [CE0848] "No placeholder with the
//	                                 name 'Main' found. There should be exactly one."
//	placeholder Main               → 0 errors
//	placeholder Main + Content     → 0 errors
//	placeholder Main + Main        → [CE0849] "Multiple placeholders with name
//	                                 'Main' found. There can be only one."
//	                                 + [CE0495] "Duplicate name 'Main'."
//	placeholder Main + Side + Side → [CE0495] "Duplicate name 'Side'."
//
// So the rule has two independent halves, and CE0495 shows the second is not
// about Main at all: placeholder names must be unique, whatever they are.
//
// # Why check has to carry it
//
// Nothing between the author and mxbuild resolves a placeholder name. The
// reported script (mendixlabs/mxcli#1063) passed `mxcli check -p --references`
// AND `mxcli exec`, reported "Created layout", and failed CE0848 a whole build
// later — with no `DROP LAYOUT` to undo it. The layout is fully described by the
// statement, so this needs no project and runs in the project-free pass, which
// is what puts it in CI.
//
// # An error, not a warning
//
// Unlike MDL078, nothing here is a snapshot of a Mendix asset that a future
// version might extend: CE0848 and CE0495 are structural and were measured, so
// a flagged layout cannot build today. `exec` refuses it rather than writing a
// document whose only remedy is a verb that had to be added alongside this.
func validateLayoutPlaceholders(stmt ast.Statement) []linter.Violation {
	layout, ok := stmt.(*ast.CreateLayoutStmt)
	if !ok {
		// A page's `placeholder X { … }` FILLS a layout's slot rather than
		// declaring one, so none of this applies to it.
		return nil
	}

	names := layoutPlaceholderNames(layout.Widgets, nil)
	qn := layout.Name.String()
	loc := linter.Location{
		Module:       layout.Name.Module,
		DocumentType: "layout",
		DocumentName: layout.Name.Name,
	}

	issue, mains, dups := types.CheckLayoutPlaceholderNames(names)

	var out []linter.Violation

	// Reported before the Main rule: a braced placeholder is not counted above
	// (the visitor drops it), so leading with "declares no placeholder named
	// Main" would contradict a script that plainly says `placeholder Main { }`.
	if len(layout.BracedPlaceholders) > 0 {
		out = append(out, linter.Violation{
			RuleID:   "MDL083",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf(
				"layout '%s' writes %s with a `{ … }` body. In a layout a placeholder is "+
					"DECLARED and carries nothing; the braced form is the page-side spelling "+
					"that FILLS one, and it is dropped here — leaving the layout with no "+
					"placeholder at all",
				qn, quoteList(layout.BracedPlaceholders)),
			Suggestion: fmt.Sprintf(
				"drop the braces: `placeholder %s`. Widgets go in the region around it, "+
					"not inside it.", layout.BracedPlaceholders[0]),
		})
	}

	switch issue {
	case types.LayoutPlaceholderNoMain:
		out = append(out, linter.Violation{
			RuleID:   "MDL081",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf(
				"layout '%s' declares %s, so mxbuild fails it with CE0848 "+
					"(\"No placeholder with the name 'Main' found. There should be exactly one.\")",
				qn, describePlaceholders(names)),
			Suggestion: "name the placeholder that holds page content `placeholder Main`. " +
				"Extra placeholders under other names are fine — only the Main one is required.",
		})
	case types.LayoutPlaceholderManyMains:
		out = append(out, linter.Violation{
			RuleID:   "MDL081",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf(
				"layout '%s' declares %d placeholders named 'Main', so mxbuild fails it with CE0849 "+
					"(\"Multiple placeholders with name 'Main' found. There can be only one.\")",
				qn, mains),
			Suggestion: "keep one `placeholder Main` and rename the others — a page binds to " +
				"each by name as Module.Layout.<Name>, so the names are its API.",
		})
	case types.LayoutPlaceholderDuplicateName:
		out = append(out, linter.Violation{
			RuleID:   "MDL082",
			Severity: linter.SeverityError,
			Location: loc,
			Message: fmt.Sprintf(
				"layout '%s' declares more than one placeholder named %s, so mxbuild fails it "+
					"with CE0495 (\"Duplicate name '%s'.\")",
				qn, quoteList(dups), dups[0]),
			Suggestion: "give each placeholder a distinct name: a page binds to one as " +
				"Module.Layout.<Name>, which cannot pick between two that share it.",
		})
	}
	return out
}

// layoutPlaceholderNames collects placeholder names anywhere in the tree, in
// document order. A real layout nests them inside a scroll container's regions,
// so a walk that stopped at the top level would see none of them.
func layoutPlaceholderNames(widgets []*ast.WidgetV3, acc []string) []string {
	for _, w := range widgets {
		if w == nil {
			continue
		}
		if strings.EqualFold(w.Type, "placeholder") {
			acc = append(acc, w.Name)
		}
		acc = layoutPlaceholderNames(w.Children, acc)
	}
	return acc
}

// describePlaceholders renders what the layout does declare, so the message
// distinguishes "you named it something else" from "you declared none" — two
// different mistakes behind the same CE0848.
func describePlaceholders(names []string) string {
	if len(names) == 0 {
		return "no placeholders at all"
	}
	return "only " + quoteList(names)
}
