// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// MissingTranslationsRule checks for elements that have translations in some
// languages but not all languages used in the project.
type MissingTranslationsRule struct{}

// NewMissingTranslationsRule creates a new missing translations rule.
func NewMissingTranslationsRule() *MissingTranslationsRule {
	return &MissingTranslationsRule{}
}

func (r *MissingTranslationsRule) ID() string                       { return "QUAL005" }
func (r *MissingTranslationsRule) Name() string                     { return "MissingTranslations" }
func (r *MissingTranslationsRule) Category() string                 { return "quality" }
func (r *MissingTranslationsRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *MissingTranslationsRule) Description() string {
	return "Checks for translatable strings that are missing translations in one or more project languages"
}

// Check runs the missing translations check.
// Requires REFRESH CATALOG FULL to populate the strings table.
func (r *MissingTranslationsRule) Check(ctx *linter.LintContext) []linter.Violation {
	db := ctx.CatalogDB()
	if db == nil {
		return nil
	}

	// Step 1: Find all languages used in the project
	langRows, err := db.Query(`
		SELECT DISTINCT Language FROM strings WHERE Language != '' ORDER BY Language
	`)
	if err != nil {
		return nil // strings table may not exist (no REFRESH CATALOG FULL)
	}
	defer langRows.Close()

	var languages []string
	for langRows.Next() {
		var lang string
		if err := langRows.Scan(&lang); err == nil && lang != "" {
			languages = append(languages, lang)
		}
	}

	// Need at least 2 languages to detect missing translations
	if len(languages) < 2 {
		return nil
	}

	// Step 2: Find elements that have translations in some languages but not all.
	// Group by (QualifiedName, StringContext) — each group should have all languages.
	rows, err := db.Query(`
		SELECT QualifiedName, ObjectType, StringContext, Language, StringValue
		FROM strings
		WHERE Language != ''
		ORDER BY QualifiedName, StringContext, Language
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	// Build a map: (QualifiedName, StringContext) -> set of languages present
	type elementKey struct {
		QualifiedName string
		StringContext string
	}
	type elementInfo struct {
		ObjectType string
		Languages  map[string]bool
		Example    string // one translation value for context in the message
	}

	elements := make(map[elementKey]*elementInfo)
	for rows.Next() {
		var qn, objType, sctx, lang, value string
		if err := rows.Scan(&qn, &objType, &sctx, &lang, &value); err != nil {
			continue
		}
		key := elementKey{qn, sctx}
		info, ok := elements[key]
		if !ok {
			info = &elementInfo{ObjectType: objType, Languages: make(map[string]bool)}
			elements[key] = info
		}
		info.Languages[lang] = true
		if info.Example == "" {
			info.Example = value
		}
	}

	// Step 3: Check each element for missing languages
	var violations []linter.Violation

	langSet := make(map[string]bool, len(languages))
	for _, l := range languages {
		langSet[l] = true
	}

	for key, info := range elements {
		// Skip elements that only have one language (may be intentionally single-language)
		if len(info.Languages) < 1 {
			continue
		}

		for _, lang := range languages {
			if !info.Languages[lang] {
				// Extract module from qualified name
				module := ""
				for i, c := range key.QualifiedName {
					if c == '.' {
						module = key.QualifiedName[:i]
						break
					}
				}

				example := info.Example
				if len(example) > 50 {
					example = example[:50] + "..."
				}

				violations = append(violations, linter.Violation{
					RuleID:   r.ID(),
					Severity: r.DefaultSeverity(),
					Message: fmt.Sprintf("Missing '%s' translation for %s %s (%s: %q)",
						lang, info.ObjectType, key.QualifiedName, key.StringContext, example),
					Location: linter.Location{
						Module:       module,
						DocumentType: info.ObjectType,
						DocumentName: key.QualifiedName,
					},
					Suggestion: fmt.Sprintf("Add '%s' translation for this %s", lang, key.StringContext),
				})
			}
		}
	}

	return violations
}
