// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// --- SEC013: SecurityLevelIgnoresRules ---

// SecurityLevelIgnoresRulesRule flags security the project wrote that its
// security level does not apply (ako/mxcli#2027). At Off the runtime applies
// no entity access, no page or microflow grant and creates no demo user; at
// Prototype it guards pages and microflows but not entity access, so an XPath
// constraint on an access rule has no effect. A blank app ships at Off, every
// other gate passes, and SEC010–SEC012 only run at Production.
type SecurityLevelIgnoresRulesRule struct{}

func NewSecurityLevelIgnoresRulesRule() *SecurityLevelIgnoresRulesRule {
	return &SecurityLevelIgnoresRulesRule{}
}

func (r *SecurityLevelIgnoresRulesRule) ID() string       { return "SEC013" }
func (r *SecurityLevelIgnoresRulesRule) Name() string     { return "SecurityLevelIgnoresRules" }
func (r *SecurityLevelIgnoresRulesRule) Category() string { return "security" }
func (r *SecurityLevelIgnoresRulesRule) DefaultSeverity() linter.Severity {
	return linter.SeverityWarning
}

func (r *SecurityLevelIgnoresRulesRule) Description() string {
	return "Checks that the security level applies the access rules, grants and demo users the project defines"
}

// RequiredCatalogMode: permissions are written by REFRESH CATALOG FULL.
func (r *SecurityLevelIgnoresRulesRule) RequiredCatalogMode() linter.CatalogMode {
	return linter.CatalogFull
}

const sec013Suggestion = "ALTER APP SECURITY ( SecurityLevel: PRODUCTION )"

func (r *SecurityLevelIgnoresRulesRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}
	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil {
		return nil
	}
	switch ps.SecurityLevel {
	case security.SecurityLevelOff:
		return r.checkOff(ctx, ps)
	case security.SecurityLevelPrototype:
		return r.checkPrototype(ctx)
	}
	return nil
}

// ownModules is the set of the project's own modules: not a marketplace
// module (whose rules nobody here wrote) and not excluded from the lint run.
func ownModules(ctx *linter.LintContext) map[string]bool {
	own := map[string]bool{}
	rows, err := ctx.Query(`SELECT Name FROM modules WHERE COALESCE(Source, '') = '' AND Name <> 'System'`)
	if err != nil {
		return own
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil && !ctx.IsExcluded(name) {
			own[name] = true
		}
	}
	return own
}

func (r *SecurityLevelIgnoresRulesRule) checkOff(ctx *linter.LintContext, ps *security.ProjectSecurity) []linter.Violation {
	own := ownModules(ctx)
	entities, docs := map[string]bool{}, map[string]bool{}
	for p := range ctx.Permissions() {
		if !own[p.ModuleName] {
			continue
		}
		switch p.ElementType {
		case "ENTITY":
			entities[p.ElementName] = true
		case "PAGE", "MICROFLOW", "NANOFLOW":
			docs[p.ElementType+" "+p.ElementName] = true
		}
	}
	pageCount, flowCount := 0, 0
	for d := range docs {
		if strings.HasPrefix(d, "PAGE ") {
			pageCount++
		} else {
			flowCount++
		}
	}
	var ignored []string
	if n := len(entities); n > 0 {
		ignored = append(ignored, countOf(n, "entity", "entities")+" with access rules")
	}
	if pageCount > 0 {
		ignored = append(ignored, countOf(pageCount, "page", "pages")+" granted to roles")
	}
	if flowCount > 0 {
		ignored = append(ignored, countOf(flowCount, "microflow or nanoflow", "microflows or nanoflows")+" granted to roles")
	}
	if n := len(ps.DemoUsers); n > 0 {
		ignored = append(ignored, countOf(n, "demo user", "demo users"))
	}
	if len(ignored) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message: fmt.Sprintf("Security level is Off, so the runtime ignores the project's %s: every user can open every page and read every row, and no demo user is created",
			strings.Join(ignored, ", ")),
		Location:   linter.Location{DocumentType: "security", DocumentName: "ProjectSecurity"},
		Suggestion: sec013Suggestion + " — Production is the only level that applies entity access",
	}}
}

func (r *SecurityLevelIgnoresRulesRule) checkPrototype(ctx *linter.LintContext) []linter.Violation {
	own := ownModules(ctx)
	constrained := map[string]bool{}
	for p := range ctx.Permissions() {
		if p.ElementType == "ENTITY" && p.IsConstrained && own[p.ModuleName] {
			constrained[p.ElementName] = true
		}
	}
	if len(constrained) == 0 {
		return nil
	}
	names := make([]string, 0, len(constrained))
	for n := range constrained {
		names = append(names, n)
	}
	sort.Strings(names)
	shown := names
	if len(shown) > 5 {
		shown = append(append([]string{}, names[:5]...), fmt.Sprintf("and %d more", len(names)-5))
	}
	return []linter.Violation{{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message: fmt.Sprintf("Security level is Prototype, which does not apply entity access: the XPath constraints on %s have no effect, so every user with page access reads every row (%s)",
			countOf(len(names), "entity's access rules", "entities' access rules"), strings.Join(shown, ", ")),
		Location:   linter.Location{DocumentType: "security", DocumentName: "ProjectSecurity"},
		Suggestion: sec013Suggestion,
	}}
}

func countOf(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
