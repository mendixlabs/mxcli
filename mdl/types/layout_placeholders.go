// SPDX-License-Identifier: Apache-2.0

package types

import "sort"

// MainPlaceholderName is the name mxbuild requires exactly one of on every
// layout. It is not a convention mxcli picked — see LayoutPlaceholderProblem.
const MainPlaceholderName = "Main"

// LayoutPlaceholderIssue identifies which half of the placeholder rule a layout
// breaks. The two are independent: a layout can satisfy the Main rule and still
// repeat another name.
type LayoutPlaceholderIssue int

const (
	// LayoutPlaceholdersOK means the names build.
	LayoutPlaceholdersOK LayoutPlaceholderIssue = iota
	// LayoutPlaceholderNoMain → CE0848.
	LayoutPlaceholderNoMain
	// LayoutPlaceholderManyMains → CE0849 (and CE0495).
	LayoutPlaceholderManyMains
	// LayoutPlaceholderDuplicateName → CE0495, on a name other than Main.
	LayoutPlaceholderDuplicateName
)

// CheckLayoutPlaceholderNames applies the rule mxbuild enforces on every layout,
// measured on Mendix 11.12.1 against a layout NO PAGE USES — so none of it
// depends on a page binding to the layout:
//
//	"Content"                 → CE0848 "No placeholder with the name 'Main'
//	                            found. There should be exactly one."
//	"Main"                    → 0 errors
//	"Main", "Content"         → 0 errors — extra placeholders are fine
//	"Main", "Main"            → CE0849 + CE0495
//	"Main", "Side", "Side"    → CE0495 "Duplicate name 'Side'."
//
// It lives here, on the bare names, because two callers in different currencies
// have to agree: `mxcli check` reads the names off the AST, and the writer reads
// them off the semantic model. Two copies of a three-line rule is how a resolver
// drifts — the same mistake that let three icon vocabularies disagree in
// mendixlabs/mxcli#1059.
//
// mains is how many are named Main; dups lists the repeated names, sorted.
func CheckLayoutPlaceholderNames(names []string) (issue LayoutPlaceholderIssue, mains int, dups []string) {
	seen := map[string]int{}
	for _, n := range names {
		seen[n]++
		if n == MainPlaceholderName {
			mains++
		}
	}
	for n, c := range seen {
		if c > 1 {
			dups = append(dups, n)
		}
	}
	sort.Strings(dups)

	switch {
	case mains == 0:
		return LayoutPlaceholderNoMain, mains, dups
	case mains > 1:
		return LayoutPlaceholderManyMains, mains, dups
	case len(dups) > 0:
		return LayoutPlaceholderDuplicateName, mains, dups
	}
	return LayoutPlaceholdersOK, mains, dups
}
