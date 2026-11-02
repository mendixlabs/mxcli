// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ReachablePageReadAccessRule (SEC012) predicts mxbuild's CE2729: a page a user
// role can reach shows an attribute, or lists an entity, that the role may not
// read (ako/mxcli#936).
//
//	CE2729 "No read access to attribute 'Title' in entity 'Probe.Ticket' for user
//	        role 'ProbeUser' (with role 'User' in module 'Probe')."
//	CE2729 "No access to entity 'Probe.Ticket2' for user role 'ProbeUser' …"
//
// `check --references`, `exec` and `lint` all passed such projects; only a
// build reported it. It is a question about the whole project, worked out per
// user role, so it belongs here and not in a script check.
//
// # When mxbuild reports it (measured on 11.15.0, Production security)
//
// Only for a page the user role can REACH and VIEW. Reached:
//   - from navigation: a menu item; the role's home page — its role-based one,
//     else the profile's — which is checked even when the role may not view it;
//   - from a reached page or snippet: a show-page action, a snippet call, a
//     microflow or nanoflow it calls that the role may execute;
//   - from a reached microflow: a show page, and any sub-microflow it calls
//     (the sub-microflow's own allowed roles do not matter).
//
// Not reported: a page nothing reaches, a page reached only through a
// microflow the role may not execute, and a page in the menu the role may not
// view. An inherited attribute counts against the specialization's rules, and
// a role with no read rule at all gets both the entity and the attribute error.
//
// A page with a URL and nothing linking to it was reported in fresh 11.13.0 and
// 11.15.0 apps, and was NOT in JTSBootLogboek (11.13.0), for any user role. Until
// what decides that is known, a URL does not make a page reachable here: mxbuild
// still reports those, and a lint finding mxbuild does not is worse than one it
// reports alone.
//
// # What is left out
//
// Attributes of System entities (their access comes from the platform, see
// SEC010), associations a widget binds, layouts' own menus, and pages in
// Marketplace modules. When a page lists several entities an attribute could
// belong to, it is reported only if the role reads it on none of them.
type ReachablePageReadAccessRule struct{}

// NewReachablePageReadAccessRule creates the SEC012 rule.
func NewReachablePageReadAccessRule() *ReachablePageReadAccessRule {
	return &ReachablePageReadAccessRule{}
}

func (r *ReachablePageReadAccessRule) ID() string                       { return "SEC012" }
func (r *ReachablePageReadAccessRule) Name() string                     { return "ReachablePageNotReadable" }
func (r *ReachablePageReadAccessRule) Category() string                 { return "security" }
func (r *ReachablePageReadAccessRule) DefaultSeverity() linter.Severity { return linter.SeverityError }

func (r *ReachablePageReadAccessRule) Description() string {
	return "Checks that every page a user role can reach shows only attributes and entities the role may read (mxbuild CE2729)"
}

// RequiredCatalogMode: refs (member, show_page, action, call) and permissions
// are written by REFRESH CATALOG FULL.
func (r *ReachablePageReadAccessRule) RequiredCatalogMode() linter.CatalogMode {
	return linter.CatalogFull
}

type reachEdge struct{ kind, targetType, target string }

// reachModel is the project as the walk needs it.
type reachModel struct {
	edges        map[string][]reachEdge         // source document -> refs out of it
	snippets     map[string]bool                // qualified names of snippets
	module       map[string]string              // page/snippet -> module
	market       map[string]bool                // module -> is a Marketplace module
	views        map[string]map[string]bool     // page -> module roles that may view it
	executes     map[string]map[string]bool     // microflow/nanoflow -> module roles that may execute it
	gen          map[string]string              // entity -> generalization
	reads        map[string]map[string]bool     // entity -> module roles with a READ rule
	members      map[string]map[string]bool     // entity+"\x00"+member -> module roles reading it
	listed       map[string]map[string]bool     // entity -> module roles whose rule lists members
	defaultRO    map[string]map[string]bool     // entity -> module roles whose rule's default reads
	entries      []string                       // menu pages and microflows
	profileHomes map[string]string              // navigation profile -> its home page
	roleHomes    map[string]map[string][]string // profile -> user role -> role-based home pages/microflows
}

// Check reports, per page or snippet a user role reaches, each attribute or
// entity the role may not read.
func (r *ReachablePageReadAccessRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}
	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil || ps.SecurityLevel != security.SecurityLevelProduction || len(ps.UserRoles) == 0 {
		return nil
	}
	m := loadReachModel(ctx)

	type finding struct {
		doc, entity, member string // member "" for the entity-level error
		roles               []string
	}
	found := map[string]*finding{}
	var order []string
	note := func(doc, entity, member, userRole string) {
		k := doc + "\x00" + entity + "\x00" + member
		f, ok := found[k]
		if !ok {
			f = &finding{doc: doc, entity: entity, member: member}
			found[k] = f
			order = append(order, k)
		}
		f.roles = append(f.roles, userRole)
	}

	for _, ur := range ps.UserRoles {
		roles := map[string]bool{}
		for _, mr := range ur.ModuleRoles {
			roles[mr] = true
		}
		for _, doc := range m.reach(ur.Name, roles) {
			if mod := m.module[doc]; m.market[mod] || ctx.IsExcluded(mod) {
				continue
			}
			var entities []string
			for _, e := range m.edges[doc] {
				if e.kind == "datasource" && e.targetType == "ENTITY" {
					entities = append(entities, e.target)
				}
			}
			blindEntity := map[string]bool{}
			for _, e := range entities {
				if !strings.HasPrefix(e, "System.") && !m.readsEntity(e, roles) && !blindEntity[e] {
					blindEntity[e] = true
					note(doc, e, "", ur.Name)
				}
			}
			for _, e := range m.edges[doc] {
				if e.kind != "member" || e.targetType != "ATTRIBUTE" || strings.HasPrefix(e.target, "System.") {
					continue
				}
				i := strings.LastIndex(e.target, ".")
				if i <= 0 {
					continue
				}
				declaring := e.target[:i]
				candidates := m.holders(declaring, entities)
				readable := false
				for _, c := range candidates {
					if m.readsMember(c, e.target, roles) {
						readable = true
						break
					}
				}
				if !readable {
					note(doc, candidates[0], e.target, ur.Name)
				}
			}
		}
	}

	var violations []linter.Violation
	for _, k := range order {
		f := found[k]
		sort.Strings(f.roles)
		kind := "page"
		if m.snippets[f.doc] {
			kind = "snippet"
		}
		v := linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Location: linter.Location{
				Module:       m.module[f.doc],
				DocumentType: kind,
				DocumentName: docNameFromQualified(f.doc),
			},
		}
		who := fmt.Sprintf("user role%s %s", plural(f.roles), quoteList(f.roles))
		if f.member == "" {
			v.Message = fmt.Sprintf("%s %s lists %s, which %s can reach but may not read at all — mxbuild reports CE2729 \"No access to entity '%s'\"",
				kind, f.doc, f.entity, who, f.entity)
			v.Suggestion = fmt.Sprintf("GRANT READ on %s to a module role those user roles hold, or keep the page out of their reach", f.entity)
		} else {
			attr := f.member[strings.LastIndex(f.member, ".")+1:]
			v.Message = fmt.Sprintf("%s %s shows %s, which %s can reach but may not read on %s — mxbuild reports CE2729 \"No read access to attribute '%s' in entity '%s'\"",
				kind, f.doc, f.member, who, f.entity, attr, f.entity)
			v.Suggestion = fmt.Sprintf("Add %s to the READ member list of %s's access rule for a module role those user roles hold (repeat that rule's XPath so the grant merges into it), or keep the page out of their reach",
				attr, f.entity)
		}
		violations = append(violations, v)
	}
	return violations
}

// reach returns the pages and snippets userRole reaches, in walk order.
//
// A home page is checked whether or not the role may view it, and is walked
// onward only if it may (measured on JTSBootLogboek, 11.13.0: a user role
// holding no access to the profile's home page got CE2729 on that page's
// widgets and on nothing it links to). A profile's own home page applies to the
// user roles that have no role-based home page in that profile.
func (m *reachModel) reach(userRole string, roles map[string]bool) []string {
	held := keys(roles)
	seen := map[string]bool{}
	var docs []string
	type item struct {
		name  string
		entry bool // from navigation: a microflow needs execute rights
		home  bool // a home page: checked even when the role may not view it
	}
	var queue []item
	for _, h := range m.homesFor(userRole) {
		queue = append(queue, item{name: h, entry: true, home: true})
	}
	for _, e := range m.entries {
		queue = append(queue, item{name: e, entry: true})
	}
	for i := 0; i < len(queue); i++ {
		it := queue[i]
		n := it.name
		if seen[n] {
			continue
		}
		page := m.module[n] != "" && !m.snippets[n]
		viewable := page && reachAny(m.views[n], held)
		switch {
		case page && !viewable && !it.home:
			continue // a page the role may not view is not reached
		case m.module[n] == "" && it.entry && !reachAny(m.executes[n], held):
			continue // an entry microflow the role may not execute
		}
		seen[n] = true
		if m.module[n] != "" {
			docs = append(docs, n)
		}
		if page && !viewable {
			continue // checked, but its links lead nowhere for this role
		}
		for _, e := range m.edges[n] {
			switch {
			case e.kind == "show_page" && e.targetType == "PAGE":
				queue = append(queue, item{name: e.target})
			case (e.kind == "action" || e.kind == "datasource") && (e.targetType == "MICROFLOW" || e.targetType == "NANOFLOW"):
				if reachAny(m.executes[e.target], held) {
					queue = append(queue, item{name: e.target})
				}
			case e.kind == "call" && (e.targetType == "MICROFLOW" || e.targetType == "NANOFLOW"):
				// A sub-microflow runs whatever its own allowed roles say
				// (measured). A microflow called from a nanoflow is a call from
				// the client, which needs execute rights.
				if e.targetType == "MICROFLOW" && isNanoflow(m, n) && !reachAny(m.executes[e.target], held) {
					continue
				}
				queue = append(queue, item{name: e.target})
			}
		}
	}
	return docs
}

// homesFor returns userRole's home pages: its role-based home in a profile
// that has one for it, else that profile's own home page.
func (m *reachModel) homesFor(userRole string) []string {
	var out []string
	for profile, home := range m.profileHomes {
		if h, ok := m.roleHomes[profile][userRole]; ok {
			out = append(out, h...)
		} else if home != "" {
			out = append(out, home)
		}
	}
	sort.Strings(out)
	return out
}

func isNanoflow(m *reachModel, n string) bool { return m.executes[n]["\x00nanoflow"] }

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (m *reachModel) chain(entity string) []string {
	var c []string
	seen := map[string]bool{}
	for e := entity; e != "" && !seen[e]; e = m.gen[e] {
		seen[e] = true
		c = append(c, e)
	}
	return c
}

func (m *reachModel) readsEntity(entity string, roles map[string]bool) bool {
	for _, e := range m.chain(entity) {
		for r := range roles {
			if m.reads[e][r] {
				return true
			}
		}
	}
	return false
}

// readsMember reports whether roles read member on entity: through a rule on
// the entity or a generalization that lists it, or one that lists no members
// and reads by default (the catalog expands only the entity's own attributes
// for such a rule).
func (m *reachModel) readsMember(entity, member string, roles map[string]bool) bool {
	for _, e := range m.chain(entity) {
		for r := range roles {
			if m.members[e+"\x00"+member][r] {
				return true
			}
			if m.reads[e][r] && !m.listed[e][r] && m.defaultRO[e][r] {
				return true
			}
		}
	}
	return false
}

// holders returns the entities on the page an attribute of declaring can be
// shown through: those listed there that are declaring or specialize it. When
// none is (the object comes in over an association, a parameter, a microflow),
// declaring and every specialization of it are candidates — a page over
// Teamcenter.TcBOMLine shows Teamcenter.Node's attributes through
// TcBOMLine's rules, and Node has none of its own (FactoryManagement, where
// mxbuild reports no CE2729).
func (m *reachModel) holders(declaring string, entities []string) []string {
	var out []string
	for _, e := range entities {
		for _, c := range m.chain(e) {
			if c == declaring {
				out = append(out, e)
				break
			}
		}
	}
	if len(out) == 0 {
		specs := m.specializations(declaring)
		if len(specs) == 1 {
			// Named after the one entity it can be shown through, as mxbuild
			// names it (Administration.Account.FullName shown through
			// JTSBootLogboek.Bemanning is reported "in entity Bemanning").
			return []string{specs[0], declaring}
		}
		out = append([]string{declaring}, specs...)
	}
	return out
}

func (m *reachModel) specializations(entity string) []string {
	var out []string
	seen := map[string]bool{entity: true}
	queue := []string{entity}
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		for child, parent := range m.gen {
			if parent == e && !seen[child] {
				seen[child] = true
				out = append(out, child)
				queue = append(queue, child)
			}
		}
	}
	sort.Strings(out)
	return out
}

func loadReachModel(ctx *linter.LintContext) *reachModel {
	m := &reachModel{
		edges: map[string][]reachEdge{}, snippets: map[string]bool{}, module: map[string]string{},
		market: map[string]bool{}, views: map[string]map[string]bool{}, executes: map[string]map[string]bool{},
		gen: map[string]string{}, reads: map[string]map[string]bool{}, members: map[string]map[string]bool{},
		listed: map[string]map[string]bool{}, defaultRO: map[string]map[string]bool{}, roleHomes: map[string]map[string][]string{}, profileHomes: map[string]string{},
	}
	query := func(q string, scan func(cols []string)) {
		rows, err := ctx.Query(q)
		if err != nil {
			return
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		strs := make([]string, len(cols))
		for i := range vals {
			vals[i] = new(any)
		}
		for rows.Next() {
			if rows.Scan(vals...) != nil {
				continue
			}
			for i, v := range vals {
				strs[i] = ""
				if x := *(v.(*any)); x != nil {
					strs[i] = fmt.Sprint(x)
				}
			}
			scan(strs)
		}
	}
	query(`SELECT Name, COALESCE(Source, '') FROM modules`, func(c []string) { m.market[c[0]] = c[1] != "" })
	query(`SELECT QualifiedName, ModuleName FROM pages`, func(c []string) {
		m.module[c[0]] = c[1]
		if m.views[c[0]] == nil {
			m.views[c[0]] = map[string]bool{}
		}
	})
	query(`SELECT QualifiedName, ModuleName FROM snippets`, func(c []string) {
		m.module[c[0]] = c[1]
		m.snippets[c[0]] = true
	})
	query(`SELECT QualifiedName FROM nanoflows`, func(c []string) {
		if m.executes[c[0]] == nil {
			m.executes[c[0]] = map[string]bool{}
		}
		m.executes[c[0]]["\x00nanoflow"] = true
	})
	query(`SELECT QualifiedName, COALESCE(Generalization, '') FROM entities`, func(c []string) {
		if c[1] != "" {
			m.gen[c[0]] = c[1]
		}
	})
	query(`SELECT SourceName, RefKind, TargetType, TargetName FROM refs
		WHERE RefKind IN ('show_page', 'action', 'call', 'member', 'datasource')`, func(c []string) {
		m.edges[c[0]] = append(m.edges[c[0]], reachEdge{kind: c[1], targetType: c[2], target: c[3]})
	})
	query(`SELECT COALESCE(TargetPage, ''), COALESCE(TargetMicroflow, '') FROM navigation_menu_items`, func(c []string) {
		for _, t := range c {
			if t != "" {
				m.entries = append(m.entries, t)
			}
		}
	})
	query(`SELECT ProfileName, COALESCE(HomePage, '') FROM navigation_profiles`, func(c []string) {
		m.profileHomes[c[0]] = c[1]
	})
	query(`SELECT ProfileName, UserRole, COALESCE(Page, ''), COALESCE(Microflow, '') FROM navigation_role_homes`, func(c []string) {
		if m.roleHomes[c[0]] == nil {
			m.roleHomes[c[0]] = map[string][]string{}
		}
		for _, t := range c[2:] {
			if t != "" {
				m.roleHomes[c[0]][c[1]] = append(m.roleHomes[c[0]][c[1]], t)
			}
		}
	})
	for p := range ctx.Permissions() {
		switch {
		case p.ElementType == "PAGE" && p.AccessType == "VIEW":
			reachGrant(m.views, p.ElementName, p.ModuleRoleName)
		case (p.ElementType == "MICROFLOW" || p.ElementType == "NANOFLOW") && p.AccessType == "EXECUTE":
			reachGrant(m.executes, p.ElementName, p.ModuleRoleName)
		case p.ElementType == "ENTITY" && p.AccessType == "READ" && p.MemberName == "":
			reachGrant(m.reads, p.ElementName, p.ModuleRoleName)
			if p.DefaultMemberAccessRights == "ReadOnly" || p.DefaultMemberAccessRights == "ReadWrite" {
				reachGrant(m.defaultRO, p.ElementName, p.ModuleRoleName)
			}
		case p.ElementType == "ENTITY" && p.AccessType == "MEMBER_READ" && p.MemberName != "":
			reachGrant(m.members, p.ElementName+"\x00"+p.MemberName, p.ModuleRoleName)
			reachGrant(m.listed, p.ElementName, p.ModuleRoleName)
		}
	}
	return m
}

func reachGrant(m map[string]map[string]bool, key, role string) {
	if m[key] == nil {
		m[key] = map[string]bool{}
	}
	m[key][role] = true
}

func reachAny(granted map[string]bool, roles []string) bool {
	for _, r := range roles {
		if granted[r] {
			return true
		}
	}
	return false
}
