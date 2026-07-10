// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"math"
	"sort"
	"unicode"
)

// normalizationBaseline is the reference "checkable universe" size a category's
// penalty is scaled against. A category with exactly this many catalog elements
// scores as if unnormalized; fewer elements amplify the penalty per violation,
// more elements dilute it — so the same violation count doesn't punish a small
// project as hard as it rewards a large one.
const normalizationBaseline = 50

// Report represents a complete lint report with scoring.
type Report struct {
	ProjectName  string          `json:"projectName"`
	Date         string          `json:"date"`
	OverallScore float64         `json:"overallScore"`
	Categories   []CategoryScore `json:"categories"`
	Violations   []Violation     `json:"-"`
	Summary      Summary         `json:"summary"`
}

// CategoryScore tracks the score for a lint category.
type CategoryScore struct {
	Name            string   `json:"name"`
	Score           float64  `json:"score"`           // 0-100
	Total           int      `json:"total"`           // violations found (errors+warnings+infos)
	ElementsChecked int      `json:"elementsChecked"` // catalog elements this category evaluates
	Errors          int      `json:"errors"`
	Warnings        int      `json:"warnings"`
	Infos           int      `json:"infos"`
	TopActions      []string `json:"topActions"`
}

// categoryWeight defines the weight for each category in overall score.
// Categories are discovered dynamically from registered rules' Category()
// (see CategoriesFromRules); this table only supplies the relative importance
// of the categories mxcli's built-in and Starlark rules currently use. An
// unlisted category (e.g. a custom Starlark rule with a novel CATEGORY)
// falls back to the default weight applied below.
var categoryWeight = map[string]float64{
	"Security":     0.25,
	"Correctness":  0.20,
	"Quality":      0.20,
	"Architecture": 0.15,
	"Performance":  0.15,
	"Naming":       0.10,
	"Design":       0.10,
	"Complexity":   0.10,
	"Other":        0.05,
}

// defaultCategoryWeight is used for any category not listed in categoryWeight.
const defaultCategoryWeight = 0.05

// CategoriesFromRules derives a rule ID → category name mapping from a set of
// registered lint rules, so the report's category breakdown always reflects
// whatever rules actually ran instead of a hand-maintained list that can
// drift out of sync (see rules.Rule.Category()).
func CategoriesFromRules(rules []Rule) map[string]string {
	m := make(map[string]string, len(rules))
	for _, r := range rules {
		m[r.ID()] = titleCaseCategory(r.Category())
	}
	return m
}

// titleCaseCategory upper-cases the first rune of a rule's lowercase
// Category() (e.g. "security" -> "Security") for display. An empty category
// maps to "Other".
func titleCaseCategory(cat string) string {
	if cat == "" {
		return "Other"
	}
	r := []rune(cat)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// BuildReport creates a Report from a list of violations. rules is the set of
// lint rules that were run — used to discover categories (via Category()) and
// group violations, so the report reflects exactly the rules that executed.
// elementCounts maps each category name to the number of catalog elements
// that make up its "checkable universe", used to normalize violation density
// into a score (see normalizationBaseline).
func BuildReport(projectName, date string, violations []Violation, rules []Rule, elementCounts map[string]int) *Report {
	report := &Report{
		ProjectName: projectName,
		Date:        date,
		Violations:  violations,
		Summary:     Summarize(violations),
	}

	ruleCategories := CategoriesFromRules(rules)

	// Every category with a registered rule is reported, even with zero
	// violations, so the score breakdown doesn't silently omit a clean area.
	catSet := make(map[string]bool)
	for _, cat := range ruleCategories {
		catSet[cat] = true
	}

	// Group violations by category
	catViolations := make(map[string][]Violation)
	for _, v := range violations {
		cat := resolveCategory(v.RuleID, ruleCategories)
		catViolations[cat] = append(catViolations[cat], v)
		catSet[cat] = true
	}

	var allCats []string
	for cat := range catSet {
		allCats = append(allCats, cat)
	}
	sort.Strings(allCats)

	// Build category scores
	var categories []CategoryScore
	for _, catName := range allCats {
		vols := catViolations[catName]
		cs := buildCategoryScore(catName, vols, elementCounts[catName])
		categories = append(categories, cs)
	}

	report.Categories = categories

	// Compute overall score as weighted average
	var totalWeight float64
	var weightedScore float64
	for _, cs := range categories {
		w := categoryWeight[cs.Name]
		if w == 0 {
			w = defaultCategoryWeight
		}
		totalWeight += w
		weightedScore += cs.Score * w
	}
	if totalWeight > 0 {
		report.OverallScore = math.Round(weightedScore/totalWeight*10) / 10
	} else {
		report.OverallScore = 100
	}

	return report
}

func resolveCategory(ruleID string, ruleCategories map[string]string) string {
	if cat, ok := ruleCategories[ruleID]; ok {
		return cat
	}
	return "Other"
}

func buildCategoryScore(name string, violations []Violation, elementsChecked int) CategoryScore {
	cs := CategoryScore{Name: name, ElementsChecked: elementsChecked}

	for _, v := range violations {
		switch v.Severity {
		case SeverityError:
			cs.Errors++
		case SeverityWarning:
			cs.Warnings++
		case SeverityInfo:
			cs.Infos++
		}
	}

	cs.Total = cs.Errors + cs.Warnings + cs.Infos

	// Scoring: Error=-5, Warning=-1, Info=-0.2 //adjusted from original: Error=-10, Warning=-3, Info=-1
	penalty := float64(cs.Errors)*5 + float64(cs.Warnings)*1 + float64(cs.Infos)*0.2
	if elementsChecked > 0 {
		penalty = penalty / float64(elementsChecked) * normalizationBaseline
	}
	// if elementsChecked == 0, fall back to the flat penalty (nothing to normalize against)
	
	cs.Score = math.Max(0, 100-penalty)

	// Build top actions from most frequent violation messages (deduplicated by rule)
	ruleMessages := make(map[string]string)
	ruleCounts := make(map[string]int)
	for _, v := range violations {
		ruleCounts[v.RuleID]++
		if _, ok := ruleMessages[v.RuleID]; !ok && v.Suggestion != "" {
			ruleMessages[v.RuleID] = v.Suggestion
		}
	}

	// Sort by count descending
	type ruleCount struct {
		ruleID string
		count  int
	}
	var sorted []ruleCount
	for id, c := range ruleCounts {
		sorted = append(sorted, ruleCount{id, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	for i, rc := range sorted {
		if i >= 5 {
			break
		}
		if msg, ok := ruleMessages[rc.ruleID]; ok {
			cs.TopActions = append(cs.TopActions, msg)
		}
	}

	return cs
}
