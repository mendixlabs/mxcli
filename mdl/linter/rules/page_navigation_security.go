// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// PageNavigationSecurityRule checks that pages used in navigation have at least
// one allowed module role. Studio Pro reports this as CE0557 but mx check does not.
type PageNavigationSecurityRule struct{}

// NewPageNavigationSecurityRule creates a new page navigation security rule.
func NewPageNavigationSecurityRule() *PageNavigationSecurityRule {
	return &PageNavigationSecurityRule{}
}

func (r *PageNavigationSecurityRule) ID() string                       { return "MPR007" }
func (r *PageNavigationSecurityRule) Name() string                     { return "PageNavigationSecurity" }
func (r *PageNavigationSecurityRule) Category() string                 { return "security" }
func (r *PageNavigationSecurityRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *PageNavigationSecurityRule) Description() string {
	return "Checks that pages used in navigation have at least one allowed role (CE0557)"
}

// navUsage describes how a page is used in navigation.
type navUsage struct {
	Profile string
	Context string // "home page", "menu item 'Caption'", "login page", etc.
}

// Check finds pages referenced in navigation profiles and verifies they have allowed roles.
func (r *PageNavigationSecurityRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	nav, err := reader.GetNavigation()
	if err != nil || nav == nil {
		return nil
	}

	// Collect all pages used in navigation, with usage context
	navPages := make(map[string][]navUsage) // qualifiedName → usages

	for _, profile := range nav.Profiles {
		pName := profile.Kind
		if pName == "" {
			pName = profile.Name
		}

		if profile.HomePage != nil && profile.HomePage.Page != "" {
			navPages[profile.HomePage.Page] = append(navPages[profile.HomePage.Page],
				navUsage{Profile: pName, Context: "home page"})
		}

		for _, rbh := range profile.RoleBasedHomePages {
			if rbh.Page != "" {
				navPages[rbh.Page] = append(navPages[rbh.Page],
					navUsage{Profile: pName, Context: fmt.Sprintf("role-based home page for %s", rbh.UserRole)})
			}
		}

		if profile.LoginPage != "" {
			navPages[profile.LoginPage] = append(navPages[profile.LoginPage],
				navUsage{Profile: pName, Context: "login page"})
		}

		if profile.NotFoundPage != "" {
			navPages[profile.NotFoundPage] = append(navPages[profile.NotFoundPage],
				navUsage{Profile: pName, Context: "not-found page"})
		}

		collectMenuPages(profile.MenuItems, pName, navPages)
	}

	if len(navPages) == 0 {
		return nil
	}

	// Build map of qualified page name → AllowedRoles count
	pageRoleCounts := buildPageRoleCountMap(reader)

	var violations []linter.Violation
	for pageName, usages := range navPages {
		moduleName := moduleFromQualified(pageName)
		if ctx.IsExcluded(moduleName) {
			continue
		}

		roleCount, found := pageRoleCounts[pageName]
		if !found {
			continue // page not found in project (may be from marketplace module)
		}

		if roleCount > 0 {
			continue // has at least one allowed role
		}

		usage := usages[0]
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message: fmt.Sprintf("Page '%s' is used as %s in %s navigation but has no allowed roles (CE0557)",
				pageName, usage.Context, usage.Profile),
			Location: linter.Location{
				Module:       moduleName,
				DocumentType: "page",
				DocumentName: docNameFromQualified(pageName),
			},
			Suggestion: fmt.Sprintf("Grant view access: GRANT VIEW ON PAGE %s TO %s.<RoleName>", pageName, moduleName),
		})
	}

	return violations
}

// collectMenuPages recursively collects pages from navigation menu items.
func collectMenuPages(items []*types.NavMenuItem, profileName string, navPages map[string][]navUsage) {
	for _, item := range items {
		if item.Page != "" {
			navPages[item.Page] = append(navPages[item.Page],
				navUsage{Profile: profileName, Context: fmt.Sprintf("menu item '%s'", item.Caption)})
		}
		collectMenuPages(item.Items, profileName, navPages)
	}
}

// buildPageRoleCountMap builds a map of qualified page name → number of allowed roles.
func buildPageRoleCountMap(reader linter.LintReader) map[string]int {
	result := make(map[string]int)

	pages, err := reader.ListPages()
	if err != nil {
		return result
	}

	// Build hierarchy to resolve container → module
	modules, err := reader.ListModules()
	if err != nil {
		return result
	}
	folders, err := reader.ListFolders()
	if err != nil {
		return result
	}

	// Build container → module name map
	moduleByID := make(map[string]string) // module ID → module name
	for _, m := range modules {
		moduleByID[string(m.ID)] = m.Name
	}
	folderParent := make(map[string]string) // folder ID → parent ID
	for _, f := range folders {
		folderParent[string(f.ID)] = string(f.ContainerID)
	}

	resolveModule := func(containerID string) string {
		id := containerID
		for i := 0; i < 20; i++ { // max depth guard
			if name, ok := moduleByID[id]; ok {
				return name
			}
			parent, ok := folderParent[id]
			if !ok {
				return ""
			}
			id = parent
		}
		return ""
	}

	for _, pg := range pages {
		moduleName := resolveModule(string(pg.ContainerID))
		if moduleName == "" {
			continue
		}
		qualifiedName := moduleName + "." + pg.Name
		result[qualifiedName] = len(pg.AllowedRoles)
	}

	return result
}

// moduleFromQualified extracts module name from "Module.Name".
func moduleFromQualified(qualifiedName string) string {
	if idx := strings.Index(qualifiedName, "."); idx > 0 {
		return qualifiedName[:idx]
	}
	return qualifiedName
}
