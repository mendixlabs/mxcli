// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SecuritySettings is the part of project security the report reads, copied
// out of sdk/security so the report's JSON does not change shape when that
// type does.
type SecuritySettings struct {
	Known             bool       `json:"known"`
	Level             string     `json:"level"` // CheckNothing | CheckFormsAndMicroflows | CheckEverything
	StrictMode        bool       `json:"strictMode"`
	DemoUsersEnabled  bool       `json:"demoUsersEnabled"`
	DemoUsers         []DemoUser `json:"demoUsers,omitempty"`
	GuestAccess       bool       `json:"guestAccess"`
	GuestRole         string     `json:"guestRole,omitempty"`
	AdminUser         string     `json:"adminUser,omitempty"`
	AdminRole         string     `json:"adminRole,omitempty"`
	PasswordMinLength int        `json:"passwordMinLength"`
	// AdminRoles are user roles that can manage all roles: what a demo user
	// holding one can do is everything.
	AdminRoles []string `json:"adminRoles,omitempty"`
}

// DemoUser is a demo account defined in the model.
type DemoUser struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

// LintFinding is one lint violation, flattened.
type LintFinding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Where    string `json:"where"`
}

// SecurityInput is everything the security report reads besides the catalog.
type SecurityInput struct {
	Scope    ScopeOptions
	Settings SecuritySettings
	Lint     []LintFinding
}

// SecurityData is the collected security review.
type SecurityData struct {
	Scope    Scope            `json:"scope"`
	Settings SecuritySettings `json:"settings"`
	Findings []Finding        `json:"findings"`
	Controls []string         `json:"controls"`
	Roles    []string         `json:"roles"`
	Grid     []GridRow        `json:"grid"`
	XPaths   []XPathRule      `json:"xpaths"`
	Flows    []CallableFlow   `json:"flows"`
	// SourceIndexed says whether microflow bodies were read; without them no
	// flow can be seen to check its caller.
	SourceIndexed bool       `json:"sourceIndexed"`
	LintRules     []LintRule `json:"lintRules"`
}

// Finding is one security finding.
type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"` // high | medium | low | info
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Fix      string `json:"fix"`
}

// GridRow is one entity's access per user role.
type GridRow struct {
	Entity string            `json:"entity"`
	Access map[string]string `json:"access"` // user role → "R all", "RW own", …
}

// XPathRule is a distinct access-rule constraint with its classification.
type XPathRule struct {
	Entity string   `json:"entity"`
	XPath  string   `json:"xpath"`
	Class  string   `json:"class"`
	Roles  []string `json:"roles"`
}

// CallableFlow is a microflow a user role may call that takes a persistent
// object the client supplies.
type CallableFlow struct {
	Name              string   `json:"name"`
	Roles             []string `json:"roles"`
	Params            []string `json:"params"`
	ApplyEntityAccess bool     `json:"applyEntityAccess"`
	ChecksCaller      bool     `json:"checksCaller"`
	TakesUser         bool     `json:"takesUser"`
	Writes            bool     `json:"writes"`
	Risk              string   `json:"risk"` // high | medium | low | ok
	Triage            string   `json:"triage"`
}

// LintRule is one security lint rule's findings, grouped.
type LintRule struct {
	Rule     string   `json:"rule"`
	Severity string   `json:"severity"`
	Count    int      `json:"count"`
	Examples []string `json:"examples"`
	Hint     string   `json:"hint,omitempty"`
}

// XPath access classes, narrowest first.
const (
	AccessOwn      = "own rows"
	AccessViaUser  = "via user"
	AccessFiltered = "filtered"
	AccessAll      = "all"
)

var accessRank = map[string]int{AccessOwn: 0, AccessViaUser: 1, AccessFiltered: 2, AccessAll: 3}

var ownRows = regexp.MustCompile(`(?i)^\[\s*(System\.owner|id)\s*=\s*'\[%CurrentUser%\]'\s*\]$`)

// ClassifyXPath sorts an access rule's constraint into how much it lets a
// role see.
func ClassifyXPath(x string) string {
	x = strings.TrimSpace(x)
	switch {
	case x == "":
		return AccessAll
	case ownRows.MatchString(x):
		return AccessOwn
	case strings.Contains(strings.ToLower(x), "%currentuser%"):
		return AccessViaUser
	default:
		return AccessFiltered
	}
}

var (
	// A bare "Key" is not enough: LocalStorageKey and SortKey are names, not
	// secrets. The key has to say what kind it is.
	secretName  = regexp.MustCompile(`(?i)(secret|passw|pwd|token|credential|(api|access|private|encryption|signing|master|crypt|aes|hmac|client)_?key)`)
	testishName = regexp.MustCompile(`(?i)(mock|test|demo|debug|fake|stub|seed|bypass|skip|dev(mode)?$|sandbox)`)
	hexKey      = regexp.MustCompile(`^(?i)[0-9a-f]{32}$|^(?i)[0-9a-f]{48}$|^(?i)[0-9a-f]{64}$`)
	pemBlock    = regexp.MustCompile(`-----BEGIN [A-Z ]*(PRIVATE KEY|CERTIFICATE)-----`)
	callerCheck = regexp.MustCompile(`(?i)\$currentUser\b|\[%CurrentUser%\]|%CurrentUser%`)
)

// LooksLikeKeyMaterial reports whether a string value looks like a key: base64
// of 16, 24 or 32 bytes, a 128/192/256-bit hex string, or a PEM block.
func LooksLikeKeyMaterial(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if hexKey.MatchString(v) || pemBlock.MatchString(v) {
		return true
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(v); err == nil {
			switch len(b) {
			case 16, 24, 32, 64:
				return true
			}
		}
	}
	if _, err := hex.DecodeString(v); err == nil && len(v) >= 32 {
		return true
	}
	return false
}

// CollectSecurity reviews access, client-callable flows and unsafe defaults.
func CollectSecurity(q Querier, in SecurityInput) (*SecurityData, error) {
	scope, err := LoadScope(q, in.Scope)
	if err != nil {
		return nil, err
	}
	d := &SecurityData{Scope: scope, Settings: in.Settings}
	app := setOf(scope.App)

	// Module role → user roles.
	rm, err := rows(q, `SELECT UserRoleName, ModuleRoleName FROM role_mappings ORDER BY UserRoleName`)
	if err != nil {
		return nil, err
	}
	userRolesOf := map[string][]string{}
	roleSet := map[string]bool{}
	for _, r := range rm {
		userRolesOf[r["ModuleRoleName"]] = appendUnique(userRolesOf[r["ModuleRoleName"]], r["UserRoleName"])
		roleSet[r["UserRoleName"]] = true
	}
	ur, err := rows(q, `SELECT Name FROM user_roles ORDER BY Name`)
	if err != nil {
		return nil, err
	}
	for _, r := range ur {
		roleSet[r["Name"]] = true
	}
	for r := range roleSet {
		d.Roles = append(d.Roles, r)
	}
	sort.Strings(d.Roles)

	// Entities.
	ents, err := rows(q, `SELECT QualifiedName, ModuleName, COALESCE(EntityType, '') AS EntityType,
		COALESCE(Generalization, '') AS Generalization FROM entities ORDER BY QualifiedName`)
	if err != nil {
		return nil, err
	}
	gen := map[string]string{}
	persistent := map[string]bool{}
	for _, e := range ents {
		gen[e["QualifiedName"]] = e["Generalization"]
		if strings.EqualFold(e["EntityType"], "PERSISTENT") {
			persistent[e["QualifiedName"]] = true
		}
	}
	isUser := func(ent string) bool {
		for i := 0; ent != "" && i < 20; i++ {
			if ent == "System.User" {
				return true
			}
			ent = gen[ent]
		}
		return false
	}
	// System.User and its specializations are persistent even when the
	// catalog has no row for the System module's entities.
	isPersistent := func(ent string) bool { return persistent[ent] || isUser(ent) }

	// Entity access per user role.
	perms, err := rows(q, `SELECT ModuleRoleName, ElementName, AccessType, COALESCE(XPathConstraint, '') AS XPath
		FROM permissions WHERE ElementType = 'ENTITY' AND AccessType IN ('READ', 'WRITE', 'CREATE', 'DELETE')`)
	if err != nil {
		return nil, err
	}
	type acc struct {
		read  string
		write bool
	}
	grid := map[string]map[string]*acc{} // entity → user role → access
	readClass := map[string]map[string]string{}
	xp := map[[2]string]*XPathRule{}
	for _, p := range perms {
		ent := p["ElementName"]
		if !app[moduleOf(ent)] || !persistent[ent] {
			continue
		}
		cls := ClassifyXPath(p["XPath"])
		for _, role := range userRolesOf[p["ModuleRoleName"]] {
			if grid[ent] == nil {
				grid[ent] = map[string]*acc{}
				readClass[ent] = map[string]string{}
			}
			a := grid[ent][role]
			if a == nil {
				a = &acc{}
				grid[ent][role] = a
			}
			switch p["AccessType"] {
			case "READ":
				if a.read == "" || accessRank[cls] > accessRank[a.read] {
					a.read = cls
					readClass[ent][role] = cls
				}
			default:
				a.write = true
			}
		}
		if p["XPath"] != "" {
			k := [2]string{ent, p["XPath"]}
			r := xp[k]
			if r == nil {
				r = &XPathRule{Entity: ent, XPath: p["XPath"], Class: cls}
				xp[k] = r
			}
			r.Roles = appendUnique(r.Roles, p["ModuleRoleName"])
		}
	}
	var gridEnts []string
	for e := range grid {
		gridEnts = append(gridEnts, e)
	}
	sort.Strings(gridEnts)
	for _, e := range gridEnts {
		row := GridRow{Entity: e, Access: map[string]string{}}
		for role, a := range grid[e] {
			cell := "R"
			if a.read == "" {
				cell = "W"
			} else if a.write {
				cell = "RW"
			}
			if a.read != "" {
				cell += " " + a.read
			}
			row.Access[role] = cell
		}
		d.Grid = append(d.Grid, row)
	}
	for _, r := range xp {
		sort.Strings(r.Roles)
		d.XPaths = append(d.XPaths, *r)
	}
	sort.Slice(d.XPaths, func(i, j int) bool {
		if d.XPaths[i].Entity != d.XPaths[j].Entity {
			return d.XPaths[i].Entity < d.XPaths[j].Entity
		}
		return d.XPaths[i].XPath < d.XPaths[j].XPath
	})

	if err := collectCallableFlows(q, d, app, userRolesOf, isPersistent, isUser, readClass); err != nil {
		return nil, err
	}
	if err := collectUnsafeDefaults(q, d); err != nil {
		return nil, err
	}
	settingsFindings(d)
	d.LintRules = groupLint(in.Lint)
	d.Controls = controls(d)
	sort.SliceStable(d.Findings, func(i, j int) bool { return sevRank(d.Findings[i].Severity) > sevRank(d.Findings[j].Severity) })
	return d, nil
}

func collectCallableFlows(q Querier, d *SecurityData, app map[string]bool, userRolesOf map[string][]string,
	isPersistent, isUser func(string) bool, readClass map[string]map[string]string) error {
	grants, err := rows(q, `SELECT ModuleRoleName, ElementName FROM permissions
		WHERE ElementType = 'MICROFLOW' AND AccessType = 'EXECUTE'`)
	if err != nil {
		return err
	}
	roles := map[string][]string{}
	for _, g := range grants {
		for _, ur := range userRolesOf[g["ModuleRoleName"]] {
			roles[g["ElementName"]] = appendUnique(roles[g["ElementName"]], ur)
		}
	}
	params, err := rows(q, `SELECT MicroflowQualifiedName, Name, ParameterType FROM microflow_parameters ORDER BY MicroflowQualifiedName, Ordinal`)
	if err != nil {
		return err
	}
	pOf := map[string][]map[string]string{}
	for _, p := range params {
		pOf[p["MicroflowQualifiedName"]] = append(pOf[p["MicroflowQualifiedName"]], p)
	}
	mfs, err := rows(q, `SELECT QualifiedName, ApplyEntityAccess FROM microflows WHERE MicroflowType = 'MICROFLOW'`)
	if err != nil {
		return err
	}
	applyEA := map[string]bool{}
	for _, m := range mfs {
		applyEA[m["QualifiedName"]] = m["ApplyEntityAccess"] == "1" || strings.EqualFold(m["ApplyEntityAccess"], "true")
	}
	src, err := rows(q, `SELECT QualifiedName, SourceText FROM source WHERE ObjectType = 'MICROFLOW'`)
	if err != nil {
		return err
	}
	d.SourceIndexed = len(src) > 0
	body := map[string]string{}
	for _, s := range src {
		body[s["QualifiedName"]] = s["SourceText"]
	}
	wr, err := rows(q, `SELECT DISTINCT SourceName FROM refs WHERE SourceType = 'MICROFLOW' AND RefKind IN ('change', 'delete', 'commit', 'create')`)
	if err != nil {
		return err
	}
	writes := map[string]bool{}
	for _, w := range wr {
		writes[w["SourceName"]] = true
	}
	var names []string
	for n := range roles {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if !app[moduleOf(name)] {
			continue
		}
		f := CallableFlow{Name: name, Roles: roles[name], ApplyEntityAccess: applyEA[name], Writes: writes[name]}
		sort.Strings(f.Roles)
		var paramEnts []string
		for _, p := range pOf[name] {
			t := p["ParameterType"]
			ent := ""
			if i := strings.IndexByte(t, ':'); i >= 0 && (strings.HasPrefix(t, "Object") || strings.HasPrefix(t, "List")) {
				ent = t[i+1:]
			}
			if ent == "" || !isPersistent(ent) {
				continue
			}
			f.Params = append(f.Params, "$"+p["Name"]+": "+t)
			paramEnts = append(paramEnts, ent)
			if isUser(ent) {
				f.TakesUser = true
			}
		}
		if len(f.Params) == 0 {
			continue
		}
		f.ChecksCaller = callerCheck.MatchString(body[name])
		widest := ""
		for _, ent := range paramEnts {
			for _, r := range f.Roles {
				if c := readClass[ent][r]; c != "" && (widest == "" || accessRank[c] > accessRank[widest]) {
					widest = c
				}
			}
		}
		switch {
		case f.TakesUser && !f.ChecksCaller:
			f.Risk, f.Triage = "high", "acts for a user the page supplies: an object parameter is a plain id, so any user the caller may read can be substituted. Use $currentUser in the body instead"
		case f.ChecksCaller:
			f.Risk, f.Triage = "ok", "checks the caller in its own body ($currentUser / [%CurrentUser%])"
		case !f.ApplyEntityAccess && f.Writes:
			f.Risk, f.Triage = "high", "changes an object the client chose, with entity access off: no access rule stops a substituted id"
		case !f.ApplyEntityAccess:
			f.Risk, f.Triage = "medium", "reads an object the client chose, with entity access off"
		case widest == AccessAll:
			f.Risk, f.Triage = "medium", "relies on access rules, but the granted roles may read every object of the parameter's entity, so any id can be passed"
		default:
			f.Risk, f.Triage = "low", "relies on access rules"
			if widest != "" {
				f.Triage += " (granted roles read " + widest + ")"
			}
		}
		d.Flows = append(d.Flows, f)
		if f.Risk == "high" {
			d.Findings = append(d.Findings, Finding{ID: "flow:" + name, Severity: "high", Title: "Caller-trusting flow `" + name + "`",
				Detail: strings.ToUpper(f.Triage[:1]) + f.Triage[1:] + ". Callable by " + strings.Join(f.Roles, ", ") + ".",
				Fix:    "Derive the acting user or object in the microflow (`$currentUser`, a retrieve constrained by `[%CurrentUser%]`), or turn on entity access and constrain the parameter's entity."})
		}
	}
	return nil
}

func collectUnsafeDefaults(q Querier, d *SecurityData) error {
	cs, err := rows(q, `SELECT QualifiedName, ModuleName, COALESCE(DataType, '') AS DataType, COALESCE(DefaultValue, '') AS DefaultValue,
		COALESCE(ExposedToClient, 0) AS ExposedToClient FROM constants WHERE ModuleName != 'System' ORDER BY QualifiedName`)
	if err != nil {
		return err
	}
	vals, err := rows(q, `SELECT ConstantName, ConfigurationName, COALESCE(Value, '') AS Value FROM constant_values`)
	if err != nil {
		return err
	}
	cfg := map[string][][2]string{}
	for _, v := range vals {
		cfg[v["ConstantName"]] = append(cfg[v["ConstantName"]], [2]string{v["ConfigurationName"], v["Value"]})
	}
	for _, c := range cs {
		name, val := c["QualifiedName"], c["DefaultValue"]
		short := shortName(name)
		exposed := c["ExposedToClient"] == "1" || strings.EqualFold(c["ExposedToClient"], "true")
		dt := strings.ToLower(c["DataType"])
		if dt == "string" || dt == "" {
			key := LooksLikeKeyMaterial(val)
			named := secretName.MatchString(short)
			switch {
			case key:
				sev := "high"
				d.Findings = append(d.Findings, Finding{ID: "constant-secret:" + name, Severity: sev,
					Title:  "Constant `" + name + "` defaults to what looks like key material",
					Detail: "The default value is committed with the model (and in git history). Any environment that does not override it runs with this key." + exposedNote(exposed),
					Fix:    "Make the default empty or an obvious placeholder, set the real value per environment, and rotate the committed key — it is in the history."})
			case named && val != "" && !placeholder(val):
				d.Findings = append(d.Findings, Finding{ID: "constant-secret:" + name, Severity: "medium",
					Title:  "Constant `" + name + "` is named like a secret and has a default",
					Detail: "A default for a password, token or key travels with the model to every environment that forgets the override." + exposedNote(exposed),
					Fix:    "Leave the default empty and set it per environment."})
			}
			for _, cv := range cfg[name] {
				if LooksLikeKeyMaterial(cv[1]) || (named && cv[1] != "" && !placeholder(cv[1])) {
					d.Findings = append(d.Findings, Finding{ID: "constant-config:" + name + ":" + cv[0], Severity: "medium",
						Title:  "Configuration `" + cv[0] + "` sets secret-like constant `" + name + "`",
						Detail: "Run configurations are part of the project and are committed with it.",
						Fix:    "Keep secrets out of committed configurations; use environment variables or the deployment's constant settings."})
				}
			}
		}
		if (dt == "boolean") && strings.EqualFold(strings.TrimSpace(val), "true") && testishName.MatchString(short) {
			d.Findings = append(d.Findings, Finding{ID: "constant-testmode:" + name, Severity: "medium",
				Title:  "Constant `" + name + "` switches on test behaviour by default",
				Detail: "It is `true` unless an environment overrides it, so an environment that forgets the override runs with mock data or relaxed checks.",
				Fix:    "Default it to `false` and switch it on in the environments that need it."})
		}
	}
	return nil
}

func exposedNote(exposed bool) string {
	if exposed {
		return " It is also exposed to the client, so every browser receives it."
	}
	return ""
}

func placeholder(v string) bool {
	l := strings.ToLower(strings.TrimSpace(v))
	for _, p := range []string{"changeme", "change-me", "todo", "xxx", "<", "placeholder", "override", "secret", "password", "none", "dummy"} {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

func settingsFindings(d *SecurityData) {
	s := d.Settings
	if !s.Known {
		return
	}
	switch s.Level {
	case "CheckNothing":
		d.Findings = append(d.Findings, Finding{ID: "setting:security-off", Severity: "high", Title: "Security is off",
			Detail: "No access rule, page or microflow permission is checked.", Fix: "Set security to Production and grant access per role."})
	case "CheckFormsAndMicroflows":
		d.Findings = append(d.Findings, Finding{ID: "setting:security-prototype", Severity: "high", Title: "Security is at Prototype level",
			Detail: "Entity access rules are not enforced: every role reads and writes every object through the API.", Fix: "Set security to Production."})
	}
	admin := setOf(s.AdminRoles)
	if s.DemoUsersEnabled && len(s.DemoUsers) > 0 {
		var admins, others []string
		for _, u := range s.DemoUsers {
			isAdmin := false
			for _, r := range u.Roles {
				if admin[r] {
					isAdmin = true
				}
			}
			if isAdmin {
				admins = append(admins, u.Name)
			} else {
				others = append(others, u.Name)
			}
		}
		sev := "medium"
		if len(admins) > 0 {
			sev = "high"
		}
		detail := fmt.Sprintf("%d demo user(s) are enabled", len(s.DemoUsers))
		if len(admins) > 0 {
			detail += ", including " + strings.Join(codeList(admins), ", ") + " with a role that manages all roles"
		}
		detail += ". Two runtime facts decide the fix: dropping a demo user from the model does NOT delete its account from an existing database, and the runtime does NOT reactivate a demo account that was deactivated."
		d.Findings = append(d.Findings, Finding{ID: "setting:demo-users", Severity: sev, Title: "Demo users are enabled",
			Detail: detail, Fix: "Disable demo users for any environment with real data, and deactivate (or delete) the existing demo accounts in each database."})
	}
	if s.GuestAccess {
		d.Findings = append(d.Findings, Finding{ID: "setting:guest-access", Severity: "info", Title: "Anonymous (guest) access is on",
			Detail: "Anonymous users get role `" + s.GuestRole + "`. Everything that role may read and call is public.",
			Fix:    "Check the anonymous role's grid column and callable flows below."})
	}
	if s.Level == "CheckEverything" && !s.StrictMode {
		d.Findings = append(d.Findings, Finding{ID: "setting:strict-mode-off", Severity: "low", Title: "Strict mode is off",
			Detail: "Without strict mode the client API (`mx.data.get` and friends) may retrieve any object the role can read, not only what pages show.",
			Fix:    "Turn strict mode on unless a widget or JavaScript action needs the client API."})
	}
	if s.PasswordMinLength > 0 && s.PasswordMinLength < 8 {
		d.Findings = append(d.Findings, Finding{ID: "setting:password-length", Severity: "low", Title: "Short minimum password length",
			Detail: fmt.Sprintf("The password policy allows %d characters.", s.PasswordMinLength), Fix: "Require at least 12 characters, or use single sign-on."})
	}
}

var lintHints = map[string]string{
	"CONV007": "Fires on every unconstrained rule, including the intended ones (an admin role, reference data). Judge each entity, do not count them.",
	"SEC010":  "Fires when a page's role may not read a listed entity. Often intended when access is row-level by design; real when a role sees an empty page.",
	"SEC003":  "Demo users: see the settings finding for the runtime facts.",
}

func groupLint(ls []LintFinding) []LintRule {
	by := map[string]*LintRule{}
	for _, l := range ls {
		r := by[l.Rule]
		if r == nil {
			r = &LintRule{Rule: l.Rule, Severity: l.Severity, Hint: lintHints[l.Rule]}
			by[l.Rule] = r
		}
		r.Count++
		if len(r.Examples) < 3 {
			ex := l.Message
			if l.Where != "" {
				ex = l.Where + ": " + ex
			}
			r.Examples = append(r.Examples, ex)
		}
	}
	var out []LintRule
	for _, r := range by {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rule < out[j].Rule })
	return out
}

func controls(d *SecurityData) []string {
	var out []string
	s := d.Settings
	if s.Known && s.Level == "CheckEverything" {
		out = append(out, "Security is at Production level: entity access rules are enforced.")
	}
	if s.Known && s.StrictMode {
		out = append(out, "Strict mode is on: the client API is limited to what pages request.")
	}
	if s.Known && !s.DemoUsersEnabled {
		out = append(out, "Demo users are disabled.")
	}
	narrow := 0
	for _, g := range d.Grid {
		all := false
		for _, v := range g.Access {
			if strings.HasSuffix(v, " "+AccessAll) {
				all = true
			}
		}
		if !all {
			narrow++
		}
	}
	if len(d.Grid) > 0 {
		out = append(out, fmt.Sprintf("%d of %d persistent app entities have an XPath constraint on every role's read access.", narrow, len(d.Grid)))
	}
	checks := 0
	for _, f := range d.Flows {
		if f.ChecksCaller {
			checks++
		}
	}
	if checks > 0 {
		out = append(out, fmt.Sprintf("%d client-callable flow(s) check the caller in their own body.", checks))
	}
	return out
}

func sevRank(s string) int {
	return map[string]int{"high": 4, "medium": 3, "low": 2, "info": 1}[s]
}

func appendUnique(xs []string, s string) []string {
	if containsStr(xs, s) {
		return xs
	}
	return append(xs, s)
}

// ---------------------------------------------------------------- render

var SecurityMethod = []string{
	"A review of the model, read from the mxcli catalog (`refresh catalog full` and `source`) and the project security settings. Nothing ran against the app.",
	"Who can read what: each user role's module roles (`CATALOG.ROLE_MAPPINGS`) joined to entity access rules (`CATALOG.PERMISSIONS`). A rule's XPath is classified as own rows (`[System.owner = '[%CurrentUser%]']`), via user (any other path to the current user), filtered (a constraint not about the user) or all (no constraint); a cell shows the widest.",
	"Client-callable flows: microflows granted to a user role that take a persistent object or list. A flow checks its caller when its body (`CATALOG.SOURCE`) uses `$currentUser` or `[%CurrentUser%]`.",
	"Unsafe defaults: string constants whose default (or a committed configuration value) looks like key material — base64 of 16/24/32/64 bytes, a 128/192/256-bit hex string, a PEM block — or that are named like a secret; boolean constants named like test switches that default to `true`; the security level, demo users, guest access, strict mode and password policy.",
	"Lint security rules are grouped per rule with a verdict slot, so the score is explained rather than alarming.",
}

var SecurityLimits = []string{
	"This is a model review, not a penetration test. Hosting (HTTPS, headers, TLS), Marketplace module vulnerabilities, rate limiting and the deployment's own constant values are not covered.",
	"Object parameters versus association values: an object parameter is a plain id, so a caller can substitute any object it may read; a forged association value is rejected by the runtime (\"Hash is invalid\"), and with strict mode `mx.data.get` is refused. Only the first is flagged here — confirm a flagged flow before fixing it.",
	"A caller check is detected by name (`$currentUser`); a check through a sub-microflow or a Java action is not seen, and such a flow is shown as trusting its caller.",
	"Access through Java actions, OQL datasets, published services' own authentication and nanoflows is not analysed.",
	"Demo accounts: dropping a demo user from the model does not delete its account from an existing database, and the runtime does not reactivate a deactivated demo account. Check each environment's database.",
}

// RenderSecurity builds the security page.
func RenderSecurity(env *Envelope, ann *Annotations) (Doc, error) {
	var d SecurityData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	open := map[string]int{}
	for _, f := range d.Findings {
		st := "open"
		if n, ok := ann.Peek("finding:" + f.ID); ok && n.Status != "" {
			st = n.Status
		}
		if st == "open" {
			open[f.Severity]++
		}
	}
	risky := 0
	for _, f := range d.Flows {
		if f.Risk == "high" || f.Risk == "medium" {
			risky++
		}
	}
	doc := Doc{Title: "Security review", Subtitle: "What protects the data, what does not, and what to fix"}
	doc.Stats = []Stat{
		{Label: "open high findings", Value: fmt.Sprint(open["high"])},
		{Label: "open medium findings", Value: fmt.Sprint(open["medium"])},
		{Label: "client-callable flows", Value: fmt.Sprint(len(d.Flows)), Note: fmt.Sprintf("%d to triage", risky)},
		{Label: "security level", Value: levelName(d.Settings)},
	}

	// Findings.
	ft := Table{Columns: []string{"Severity", "Finding", "Fix", "Status", "Key"}}
	for _, f := range d.Findings {
		key := "finding:" + f.ID
		status := "open"
		if n, ok := ann.Get(key); ok {
			status = joinNote(nonEmpty(n.Status, "open"), n.Note, "")
		}
		ft.Rows = append(ft.Rows, []string{f.Severity, f.Title + " — " + f.Detail, f.Fix, status, "`" + key + "`"})
		stCls := statusClass(strings.SplitN(status, ":", 2)[0])
		if strings.HasPrefix(status, "accepted") || strings.HasPrefix(status, "intended") {
			stCls = "info"
		}
		ft.Classes = append(ft.Classes, []string{sevClass(f.Severity), "", "", stCls, ""})
	}
	fb := []Block{ft}
	if len(d.Controls) > 0 {
		fb = append(fb, Para{Text: "Controls that hold:"}, Bullets{Items: d.Controls})
	}
	doc.Sections = append(doc.Sections, Section{ID: "findings", Title: "Findings",
		Intro: "Most severe first. Record a status per finding with `--annotations` (`open`, `fixed`, `accepted`, with a note); a fixed finding that comes back is shown open again.", Blocks: fb})

	// Grid.
	gt := Table{Columns: append([]string{"Entity"}, d.Roles...)}
	for _, g := range d.Grid {
		row := []string{"`" + g.Entity + "`"}
		cls := []string{""}
		for _, r := range d.Roles {
			v := g.Access[r]
			row = append(row, v)
			c := ""
			switch {
			case strings.HasSuffix(v, AccessAll):
				c = "warn"
			case strings.HasSuffix(v, AccessOwn) || strings.HasSuffix(v, AccessViaUser):
				c = "ok"
			case v != "":
				c = "info"
			}
			cls = append(cls, c)
		}
		gt.Rows = append(gt.Rows, row)
		gt.Classes = append(gt.Classes, cls)
	}
	xt := Table{Caption: "Access-rule constraints", Columns: []string{"Entity", "XPath", "Class", "Module roles"}}
	for _, x := range d.XPaths {
		xt.Rows = append(xt.Rows, []string{"`" + x.Entity + "`", "`" + x.XPath + "`", x.Class, strings.Join(x.Roles, ", ")})
	}
	doc.Sections = append(doc.Sections, Section{ID: "grid", Title: "Who can read what",
		Intro:  "User role × persistent app entity. R read, W write; then the widest read: own rows, via user, filtered, or all (no constraint). `all` is right for reference data and admin roles and wrong almost anywhere else.",
		Blocks: []Block{gt, xt}})

	// Flows.
	flt := Table{Columns: []string{"Microflow", "Roles", "Object parameters", "Entity access", "Risk", "Triage", "Verdict"}}
	for _, f := range d.Flows {
		key := "flow:" + f.Name
		verdict := ""
		if n, ok := ann.Get(key); ok {
			verdict = joinNote(n.Status, n.Note, "")
		} else if f.Risk != "ok" && f.Risk != "low" {
			verdict = "`" + key + "`"
		}
		ea := "off"
		if f.ApplyEntityAccess {
			ea = "on"
		}
		flt.Rows = append(flt.Rows, []string{"`" + f.Name + "`", strings.Join(f.Roles, ", "), strings.Join(codeList(f.Params), ", "), ea, f.Risk, f.Triage, verdict})
		flt.Classes = append(flt.Classes, []string{"", "", "", "", sevClass(f.Risk), "", ""})
	}
	fbl := []Block{flt}
	if !d.SourceIndexed {
		fbl = append([]Block{Callout{Kind: "warn", Text: "Microflow bodies were not indexed, so no flow can be seen to check its caller. Run without `--no-source`."}}, fbl...)
	}
	doc.Sections = append(doc.Sections, Section{ID: "flows", Title: "Client-callable flows",
		Intro:  "Microflows a user role may call that take a persistent object. The page passes the object as a plain id, so the question is what stops a caller substituting another one: a check in the flow's own body, or access rules on the parameter's entity.",
		Blocks: fbl})

	// Lint.
	lt := Table{Columns: []string{"Rule", "Severity", "Findings", "Examples", "Verdict"}}
	for _, r := range d.LintRules {
		key := "rule:" + r.Rule
		verdict := nonEmpty(r.Hint, "")
		if n, ok := ann.Get(key); ok {
			verdict = joinNote(n.Status, n.Note, "")
		} else {
			verdict = joinNote("unreviewed", "", verdict)
		}
		lt.Rows = append(lt.Rows, []string{r.Rule, r.Severity, fmt.Sprint(r.Count), strings.Join(r.Examples, " · "), verdict})
		vc := ""
		if strings.HasPrefix(verdict, "real") {
			vc = "bad"
		} else if strings.HasPrefix(verdict, "intended") || strings.HasPrefix(verdict, "accepted") {
			vc = "ok"
		}
		lt.Classes = append(lt.Classes, []string{"", "", "num", "", vc})
	}
	doc.Sections = append(doc.Sections, Section{ID: "lint", Title: "Lint security rules",
		Intro:  "Each security rule with a verdict: `intended` (the design wants it), `accepted` (a known risk), or `real` (to fix). Annotate `rule:<ID>`. The scored report counts these findings; this table explains them.",
		Blocks: []Block{lt}})
	return Frame(env, ann, doc), nil
}

func levelName(s SecuritySettings) string {
	if !s.Known {
		return "unknown"
	}
	switch s.Level {
	case "CheckEverything":
		return "Production"
	case "CheckFormsAndMicroflows":
		return "Prototype"
	case "CheckNothing":
		return "Off"
	}
	return s.Level
}

func sevClass(s string) string {
	switch s {
	case "high":
		return "bad"
	case "medium":
		return "warn"
	case "low", "info":
		return "info"
	case "ok":
		return "ok"
	}
	return ""
}
