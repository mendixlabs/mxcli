// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// The configuration report lists what a deployment has to supply, read from
// the model, and what the model fixes so that no environment can change it
// (ako/mxcli#2426). It does not read the deployment portal or a pipeline:
// the actual production values are out of its reach, and it says so.

// ConfigurationInput is what the command passes in besides the catalog.
type ConfigurationInput struct {
	Scope ScopeOptions
	// The project settings' lifecycle microflows; project settings are not in
	// the catalog.
	AfterStartup, BeforeShutdown, HealthCheck string
}

// ConfigurationData is the collected configuration report.
type ConfigurationData struct {
	Scope         Scope              `json:"scope"`
	SourceIndexed bool               `json:"sourceIndexed"`
	Constants     []ConfigConstant   `json:"constants"`
	Endpoints     []ConfigEndpoint   `json:"endpoints"`
	Credentials   []ConfigCredential `json:"credentials"`
	Scheduled     []ConfigScheduled  `json:"scheduled"`
	Lifecycle     []ConfigLifecycle  `json:"lifecycle"`
	Published     []ConfigPublished  `json:"published"`
}

// What a constant needs from the deployment.
const (
	NeedMustSet  = "must be set" // no default, or a placeholder
	NeedOverride = "wrong unless overridden"
	NeedSecret   = "secret in the model" // report security has the finding
	NeedDefault  = "default applies"
)

// ConfigConstant is one constant with what the deployment must do about it.
type ConfigConstant struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Default string   `json:"default"` // masked when it looks like a secret
	Masked  bool     `json:"masked,omitempty"`
	Exposed bool     `json:"exposedToClient,omitempty"`
	Configs []string `json:"configs,omitempty"` // run configurations that set it
	UsedBy  []string `json:"usedBy,omitempty"`
	Need    string   `json:"need"`
	Reason  string   `json:"reason,omitempty"`
}

// ConfigEndpoint is a place the app calls out: a REST client's base URL or
// a REST call action's location.
type ConfigEndpoint struct {
	Where    string `json:"where"`
	Kind     string `json:"kind"`
	Value    string `json:"value,omitempty"`
	Constant string `json:"constant,omitempty"`
	// Fixed: the location is a literal, so no environment can change it.
	Fixed bool `json:"fixed"`
	// Host classifies a fixed location: localhost, private, test or external.
	Host string `json:"host,omitempty"`
}

// ConfigCredential is a credential written into the model as a literal.
type ConfigCredential struct {
	Where string `json:"where"`
	What  string `json:"what"`
}

// ConfigScheduled is a scheduled event: enabled or disabled per environment.
type ConfigScheduled struct {
	Name      string `json:"name"`
	Microflow string `json:"microflow"`
	Interval  string `json:"interval"`
	Enabled   bool   `json:"enabledInModel"`
}

// ConfigLifecycle is a microflow the runtime runs by itself: after startup,
// before shutdown, as health check.
type ConfigLifecycle struct {
	Role      string   `json:"role"`
	Microflow string   `json:"microflow"`
	Reaches   int      `json:"reaches"` // microflows in its call closure, itself included
	Writes    []string `json:"writes,omitempty"`
	Calls     []string `json:"callsOut,omitempty"` // endpoints its closure calls
}

// ConfigPublished is a published service and how its callers authenticate.
type ConfigPublished struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	Auth       string `json:"auth"`
	Operations int    `json:"operations"`
}

var (
	constantUse = regexp.MustCompile(`@([A-Za-z_]\w*\.[A-Za-z_]\w*)`)
	// call rest service get 'url' with ({1} = expr, …)
	restCall = regexp.MustCompile(`call rest service (?:get|post|put|patch|delete) '((?:[^']|'')*)'(?: with \(([^)]*)\))?`)
	// Authentication: basic (Username: expr, Password: expr)
	basicAuth   = regexp.MustCompile(`Authentication: basic \(Username: (.*?), Password: (.*?)\)`)
	testHost    = regexp.MustCompile(`(?i)(^|[.-])(test|tst|staging|stage|acc|acceptance|dev|sandbox|mock|demo|qa|uat)([.-]|\d|$)|example\.(com|org|net)$`)
	literalText = regexp.MustCompile(`^'(?:[^']|'')+'$`)
)

// CollectConfiguration reads what the deployment must supply.
func CollectConfiguration(q Querier, in ConfigurationInput) (*ConfigurationData, error) {
	scope, err := LoadScope(q, in.Scope)
	if err != nil {
		return nil, err
	}
	d := &ConfigurationData{Scope: scope}
	app := setOf(scope.App)
	appList := sqlList(scope.App)

	src, err := rows(q, `SELECT QualifiedName, ObjectType, SourceText FROM source`)
	if err != nil {
		return nil, err
	}
	d.SourceIndexed = len(src) > 0

	// Where each constant is used: @Module.Name in any document's source, a
	// REST client's base URL, and refs (an MCP endpoint, a model key).
	used := map[string][]string{}
	use := func(c, by string) {
		if !containsStr(used[c], by) {
			used[c] = append(used[c], by)
		}
	}
	for _, s := range src {
		for _, m := range constantUse.FindAllStringSubmatch(s["SourceText"], -1) {
			use(m[1], s["QualifiedName"])
		}
	}
	cr, err := rows(q, `SELECT SourceName, TargetName FROM refs WHERE TargetType = 'CONSTANT'`)
	if err != nil {
		return nil, err
	}
	for _, r := range cr {
		use(r["TargetName"], r["SourceName"])
	}
	clients, err := rows(q, `SELECT QualifiedName, ModuleName, COALESCE(BaseUrl, '') AS BaseUrl,
		COALESCE(BaseUrlConstant, '') AS BaseUrlConstant FROM consumed_rest_services ORDER BY QualifiedName`)
	if err != nil {
		return nil, err
	}
	for _, c := range clients {
		if c["BaseUrlConstant"] != "" {
			use(c["BaseUrlConstant"], c["QualifiedName"]+" (base URL)")
		}
	}

	// Constants.
	cs, err := rows(q, `SELECT QualifiedName, COALESCE(DataType, '') AS DataType, COALESCE(DefaultValue, '') AS DefaultValue,
		COALESCE(ExposedToClient, 0) AS ExposedToClient FROM constants WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return nil, err
	}
	vals, err := rows(q, `SELECT ConstantName, ConfigurationName FROM constant_values`)
	if err != nil {
		return nil, err
	}
	cfg := map[string][]string{}
	for _, v := range vals {
		cfg[v["ConstantName"]] = appendUnique(cfg[v["ConstantName"]], v["ConfigurationName"])
	}
	for _, c := range cs {
		name, def := c["QualifiedName"], c["DefaultValue"]
		cc := ConfigConstant{Name: name, Type: c["DataType"], Default: def, Configs: cfg[name], UsedBy: used[name],
			Exposed: c["ExposedToClient"] == "1" || strings.EqualFold(c["ExposedToClient"], "true")}
		sort.Strings(cc.UsedBy)
		isString := strings.EqualFold(cc.Type, "string") || cc.Type == ""
		switch {
		case isString && def != "" && (LooksLikeKeyMaterial(def) || (looksSecretNamed(shortName(name)) && !placeholder(def))):
			cc.Need, cc.Reason = NeedSecret, "the default looks like a secret; see the security report"
			cc.Default, cc.Masked = fmt.Sprintf("•••• (%d characters)", len(def)), true
		case strings.TrimSpace(def) == "" || placeholder(def):
			cc.Need, cc.Reason = NeedMustSet, "no usable default"
		default:
			if h := hostClass(def); h == "localhost" || h == "private" || h == "test" {
				cc.Need, cc.Reason = NeedOverride, "the default points at "+hostPhrase(h)
			} else {
				cc.Need = NeedDefault
			}
		}
		if len(cc.UsedBy) == 0 {
			cc.Reason = joinReason(cc.Reason, "no use found in the model (a Java action or a page can still read it)")
		}
		d.Constants = append(d.Constants, cc)
	}

	// Endpoints: REST clients, then REST call actions.
	for _, c := range clients {
		if !app[c["ModuleName"]] {
			continue
		}
		e := ConfigEndpoint{Where: c["QualifiedName"], Kind: "REST client base URL"}
		switch {
		case c["BaseUrlConstant"] != "":
			e.Constant = c["BaseUrlConstant"]
		case c["BaseUrl"] != "":
			e.Value, e.Fixed, e.Host = c["BaseUrl"], true, hostClass(c["BaseUrl"])
		default:
			continue
		}
		d.Endpoints = append(d.Endpoints, e)
	}
	callsOut := map[string][]string{}
	for _, s := range src {
		if (s["ObjectType"] != "MICROFLOW" && s["ObjectType"] != "NANOFLOW") || !app[moduleOf(s["QualifiedName"])] {
			continue
		}
		text := s["SourceText"]
		for _, m := range restCall.FindAllStringSubmatch(text, -1) {
			loc := strings.ReplaceAll(m[1], "''", "'")
			e := ConfigEndpoint{Where: s["QualifiedName"], Kind: "REST call action", Value: loc}
			if c := constantUse.FindStringSubmatch(m[2]); c != nil {
				e.Constant = c[1]
			}
			// A URL built from an expression is stored as the template '{1}'
			// with the expression as its parameter: show the expression.
			if loc == "{1}" && strings.HasPrefix(strings.TrimSpace(m[2]), "{1} = ") {
				e.Value = strings.TrimPrefix(strings.TrimSpace(m[2]), "{1} = ")
			}
			if strings.Contains(loc, "://") || hostClass(loc) != "" {
				if !strings.HasPrefix(loc, "{") {
					e.Fixed, e.Host = true, hostClass(loc)
				}
			}
			d.Endpoints = append(d.Endpoints, e)
			callsOut[s["QualifiedName"]] = appendUnique(callsOut[s["QualifiedName"]], nonEmpty(e.Constant, loc))
		}
		for _, m := range basicAuth.FindAllStringSubmatch(text, -1) {
			if literalText.MatchString(strings.TrimSpace(m[2])) {
				d.Credentials = append(d.Credentials, ConfigCredential{Where: s["QualifiedName"], What: "a REST call's basic-authentication password is a literal"})
			} else if literalText.MatchString(strings.TrimSpace(m[1])) && !strings.Contains(m[2], "@") {
				d.Credentials = append(d.Credentials, ConfigCredential{Where: s["QualifiedName"], What: "a REST call's basic-authentication user name is a literal"})
			}
		}
	}
	for _, c := range clients {
		callsOut[c["QualifiedName"]] = appendUnique(callsOut[c["QualifiedName"]], nonEmpty(c["BaseUrlConstant"], c["BaseUrl"]))
	}

	// Scheduled events.
	se, err := rows(q, `SELECT QualifiedName, COALESCE(Microflow, '') AS Microflow, COALESCE(RepeatDescription, '') AS Interval,
		COALESCE(Enabled, 0) AS Enabled FROM scheduled_events WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return nil, err
	}
	for _, r := range se {
		d.Scheduled = append(d.Scheduled, ConfigScheduled{Name: r["QualifiedName"], Microflow: r["Microflow"], Interval: r["Interval"],
			Enabled: r["Enabled"] == "1" || strings.EqualFold(r["Enabled"], "true")})
	}

	// Lifecycle microflows and what they reach.
	g, err := LoadCallGraph(q)
	if err != nil {
		return nil, err
	}
	wr, err := rows(q, `SELECT DISTINCT SourceName, TargetName FROM refs WHERE SourceType = 'MICROFLOW' AND TargetType = 'ENTITY'
		AND RefKind IN ('change', 'delete', 'commit', 'create')`)
	if err != nil {
		return nil, err
	}
	writes := map[string][]string{}
	for _, w := range wr {
		writes[w["SourceName"]] = append(writes[w["SourceName"]], w["TargetName"])
	}
	for _, l := range []struct{ role, mf string }{{"after startup", in.AfterStartup}, {"before shutdown", in.BeforeShutdown}, {"health check", in.HealthCheck}} {
		if l.mf == "" {
			continue
		}
		cl := g.Closure([]string{l.mf})
		lc := ConfigLifecycle{Role: l.role, Microflow: l.mf, Reaches: len(cl)}
		for _, m := range cl {
			for _, e := range writes[m] {
				lc.Writes = appendUnique(lc.Writes, e)
			}
			for _, c := range callsOut[m] {
				lc.Calls = appendUnique(lc.Calls, c)
			}
		}
		sort.Strings(lc.Writes)
		d.Lifecycle = append(d.Lifecycle, lc)
	}

	// Published services.
	pr, err := rows(q, `SELECT QualifiedName, COALESCE(Path, '') AS Path, COALESCE(AuthenticationTypes, '') AS Auth,
		COALESCE(OperationCount, 0) AS N FROM published_rest_services WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return nil, err
	}
	for _, r := range pr {
		d.Published = append(d.Published, ConfigPublished{Name: r["QualifiedName"], Kind: "REST", Path: r["Path"], Auth: r["Auth"], Operations: atoi(r["N"])})
	}
	po, err := rows(q, `SELECT QualifiedName, COALESCE(Path, '') AS Path, COALESCE(AuthenticationTypes, '') AS Auth,
		COALESCE(EntitySetCount, 0) AS N FROM published_odata_services WHERE ModuleName IN (`+appList+`) ORDER BY QualifiedName`)
	if err != nil {
		return nil, err
	}
	for _, r := range po {
		d.Published = append(d.Published, ConfigPublished{Name: r["QualifiedName"], Kind: "OData", Path: r["Path"], Auth: r["Auth"], Operations: atoi(r["N"])})
	}
	return d, nil
}

// hostClass says what kind of host a location names: localhost, private (an
// RFC 1918 or link-local address), test (a test, staging or example name),
// external, or "" when it names none.
func hostClass(loc string) string {
	s := strings.TrimSpace(loc)
	if s == "" {
		return ""
	}
	host := ""
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		host = u.Hostname()
	} else if !strings.ContainsAny(s, " /{}") && strings.Contains(s, ".") {
		host = strings.Split(s, ":")[0]
	}
	if host == "" {
		if strings.Contains(strings.ToLower(s), "localhost") {
			return "localhost"
		}
		return ""
	}
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return "localhost"
	}
	if ip := net.ParseIP(h); ip != nil {
		switch {
		case ip.IsLoopback() || ip.IsUnspecified():
			return "localhost"
		case ip.IsPrivate() || ip.IsLinkLocalUnicast():
			return "private"
		}
		return "external"
	}
	if testHost.MatchString(h) {
		return "test"
	}
	return "external"
}

func joinReason(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

// hostPhrase names a host class in a sentence.
func hostPhrase(h string) string {
	switch h {
	case "localhost":
		return "localhost"
	case "private":
		return "a private address"
	case "test":
		return "a test host"
	}
	return "an external host"
}

// ---------------------------------------------------------------- render

var ConfigurationMethod = []string{
	"Read from the model through the mxcli catalog: constants and their run-configuration values, consumed REST services (`CATALOG.CONSUMED_REST_SERVICES`, base URL literal or constant), scheduled events, published REST and OData services, and microflow source (`CATALOG.SOURCE`) for constant uses (`@Module.Name`), REST call locations and literal credentials.",
	"A constant must be set when it has no default or a placeholder one; it is wrong unless overridden when its default points at localhost, a private address or a test host (test, staging, acc, dev, example.com, …); a default that looks like a secret is the security report's finding and is masked here.",
	"A location is fixed in the model when it is a literal URL: no environment can change it. One built from a constant is configurable.",
	"The lifecycle microflows are the project settings' after-startup, before-shutdown and health-check microflows, with what their call closure writes and calls.",
}

var ConfigurationLimits = []string{
	"This page does not read the deployment portal, a pipeline or an environment's constant values: it lists what they must supply, not what they do supply.",
	"Scheduled events are switched on or off per environment in the cloud portal (or the runtime's settings on premises); the model's own flag only applies locally.",
	"Constants read by Java actions, JavaScript actions or widgets are not seen as used. Consumed OData services, web service calls and database connections take their locations from settings this page does not read yet.",
	"Web-service users and the accounts a published service's callers need are created in each environment; the page lists the services, not the accounts.",
}

// RenderConfiguration builds the page.
func RenderConfiguration(env *Envelope, ann *Annotations) (Doc, error) {
	var d ConfigurationData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	need := map[string]int{}
	for _, c := range d.Constants {
		need[c.Need]++
	}
	fixedBad := 0
	for _, e := range d.Endpoints {
		if e.Fixed && e.Host != "external" {
			fixedBad++
		}
	}
	doc := Doc{Title: "Configuration", Subtitle: "What each environment must supply, and what the model fixes"}
	doc.Lede = fmt.Sprintf("%d constants to set per environment, %d scheduled events to switch on or off, %d locations fixed in the model.",
		need[NeedMustSet]+need[NeedOverride], len(d.Scheduled), countFixed(d.Endpoints))
	doc.Stats = []Stat{
		{Label: "constants to set", Value: fmt.Sprint(need[NeedMustSet] + need[NeedOverride]), Note: fmt.Sprintf("of %d", len(d.Constants)), Status: statusIf(false, need[NeedOverride] > 0)},
		{Label: "fixed local or test locations", Value: fmt.Sprint(fixedBad), Status: statusIf(fixedBad > 0, false)},
		{Label: "literal credentials", Value: fmt.Sprint(len(d.Credentials)), Status: statusIf(len(d.Credentials) > 0, false)},
		{Label: "scheduled events", Value: fmt.Sprint(len(d.Scheduled))},
		{Label: "published services", Value: fmt.Sprint(len(d.Published))},
	}

	// Checklist.
	cl := Table{Columns: []string{"Item", "Why", "Status", "Key"}}
	item := func(key, text, why string) {
		status := "open"
		if n, ok := ann.Get(key); ok {
			status = joinNote(nonEmpty(n.Status, "open"), n.Note, "")
		}
		cl.Rows = append(cl.Rows, []string{text, why, status, "`" + key + "`"})
		cl.Classes = append(cl.Classes, []string{"", "", statusClass(strings.SplitN(status, ":", 2)[0]), ""})
	}
	for _, c := range d.Constants {
		switch c.Need {
		case NeedMustSet, NeedOverride:
			why := c.Need
			if len(c.UsedBy) > 0 {
				why += fmt.Sprintf("; used by %d document(s)", len(c.UsedBy))
			}
			item("constant:"+c.Name, "Set `"+c.Name+"`", joinReason(why, c.Reason))
		case NeedSecret:
			item("constant:"+c.Name, "Set `"+c.Name+"` and rotate the committed value", c.Reason)
		}
	}
	for _, s := range d.Scheduled {
		item("scheduled:"+s.Name, "Enable or disable `"+s.Name+"`", nonEmpty(s.Interval, "scheduled")+", runs `"+s.Microflow+"`")
	}
	for _, p := range d.Published {
		if p.Auth != "" && !strings.EqualFold(p.Auth, "None") {
			item("published:"+p.Name, "Set up callers of `"+p.Name+"`", p.Kind+" at `"+p.Path+"`, authenticated by "+p.Auth)
		}
	}
	blocks := []Block{}
	if len(cl.Rows) > 0 {
		blocks = append(blocks, cl)
	} else {
		blocks = append(blocks, Para{Text: "Nothing in the model needs a value per environment."})
	}
	doc.Sections = append(doc.Sections, Section{ID: "checklist", Title: "Checklist for the administrator",
		Intro:  "One item per thing an environment must supply. Sign items off with `--annotations` (`constant:<Name>`, `scheduled:<Name>`, `published:<Name>`, status `done` and a note naming the environment).",
		Blocks: blocks})

	// Fix in the model.
	var fix []Block
	ft := Table{Columns: []string{"Where", "What", "Location", "Host"}}
	for _, e := range d.Endpoints {
		if e.Fixed && e.Host != "external" {
			ft.Rows = append(ft.Rows, []string{"`" + e.Where + "`", e.Kind, "`" + e.Value + "`", hostPhrase(e.Host)})
			ft.Classes = append(ft.Classes, []string{"", "", "", "bad"})
		}
	}
	for _, c := range d.Credentials {
		ft.Rows = append(ft.Rows, []string{"`" + c.Where + "`", c.What, "", ""})
		ft.Classes = append(ft.Classes, []string{"", "", "", "bad"})
	}
	if len(ft.Rows) > 0 {
		fix = append(fix, ft)
	} else {
		fix = append(fix, Para{Text: "No location is fixed to localhost, a private address or a test host, and no credential is a literal."})
	}
	doc.Sections = append(doc.Sections, Section{ID: "fix", Title: "Fixed in the model",
		Intro:  "No environment setting can change these: they are literals in the model. Move each to a constant and set it per environment.",
		Blocks: fix})

	// Constants.
	ct := Table{Columns: []string{"Constant", "Type", "Default", "Exposed", "Run configurations", "Used by", "Needs"}}
	for _, c := range d.Constants {
		def := "`" + c.Default + "`"
		switch {
		case c.Masked:
			def = c.Default
		case c.Default == "":
			def = "(empty)"
		}
		exp := ""
		if c.Exposed {
			exp = "yes"
		}
		cls := ""
		switch c.Need {
		case NeedMustSet:
			cls = "info"
		case NeedOverride:
			cls = "warn"
		case NeedSecret:
			cls = "bad"
		}
		ct.Rows = append(ct.Rows, []string{"`" + c.Name + "`", c.Type, def, exp, strings.Join(c.Configs, ", "),
			cappedList(c.UsedBy, 4), joinReason(c.Need, c.Reason)})
		ct.Classes = append(ct.Classes, []string{"", "", "", "", "", "", cls})
	}
	cb := []Block{ct}
	if !d.SourceIndexed {
		cb = append([]Block{Callout{Kind: "warn", Text: "Microflow and page source was not indexed, so no constant can be seen as used and no REST call location was read. Run without `--no-source`."}}, cb...)
	}
	doc.Sections = append(doc.Sections, Section{ID: "constants", Title: "Constants",
		Intro: "Every constant in the app's modules, with its committed default and where the model uses it.", Blocks: cb})

	// Endpoints.
	if len(d.Endpoints) > 0 {
		et := Table{Columns: []string{"Where", "What", "Location", "Configured by"}}
		for _, e := range d.Endpoints {
			by := "a constant: `" + e.Constant + "`"
			cls := "ok"
			switch {
			case e.Constant != "":
			case e.Fixed:
				by, cls = "fixed in the model ("+hostPhrase(e.Host)+")", "warn"
				if e.Host != "external" {
					cls = "bad"
				}
			default:
				by, cls = "an expression", ""
			}
			loc := ""
			if e.Value != "" {
				loc = "`" + e.Value + "`"
			}
			et.Rows = append(et.Rows, []string{"`" + e.Where + "`", e.Kind, loc, by})
			et.Classes = append(et.Classes, []string{"", "", "", cls})
		}
		doc.Sections = append(doc.Sections, Section{ID: "endpoints", Title: "Outbound locations",
			Intro: "Where the app calls out, and whether an environment can change where.", Blocks: []Block{et}})
	}

	// Scheduled events and lifecycle.
	var rb []Block
	if len(d.Scheduled) > 0 {
		st := Table{Caption: "Scheduled events", Columns: []string{"Event", "Interval", "Runs", "Enabled in the model"}}
		for _, s := range d.Scheduled {
			st.Rows = append(st.Rows, []string{"`" + s.Name + "`", s.Interval, "`" + s.Microflow + "`", onOff(s.Enabled)})
		}
		rb = append(rb, st)
	}
	if len(d.Lifecycle) > 0 {
		lt := Table{Caption: "Lifecycle microflows", Columns: []string{"Runs", "Microflow", "Reaches", "Writes", "Calls out to"}}
		for _, l := range d.Lifecycle {
			lt.Rows = append(lt.Rows, []string{l.Role, "`" + l.Microflow + "`", fmt.Sprintf("%d microflows", l.Reaches),
				cappedList(l.Writes, 8), cappedList(l.Calls, 8)})
		}
		rb = append(rb, lt)
	}
	if len(rb) > 0 {
		doc.Sections = append(doc.Sections, Section{ID: "runtime", Title: "What runs by itself",
			Intro:  "Scheduled events are switched on or off per environment; the lifecycle microflows run on every start, stop and health check. What they write and call is what a new environment does before anyone logs in.",
			Blocks: rb})
	}

	if len(d.Published) > 0 {
		pt := Table{Columns: []string{"Service", "Kind", "Path", "Operations", "Authentication"}}
		for _, p := range d.Published {
			auth, cls := nonEmpty(p.Auth, "none"), ""
			if strings.EqualFold(auth, "none") {
				cls = "warn"
			}
			pt.Rows = append(pt.Rows, []string{"`" + p.Name + "`", p.Kind, "`" + p.Path + "`", fmt.Sprint(p.Operations), auth})
			pt.Classes = append(pt.Classes, []string{"", "", "", "num", cls})
		}
		doc.Sections = append(doc.Sections, Section{ID: "published", Title: "Published services",
			Intro: "Each environment needs the accounts or clients these services authenticate; one without authentication is open to anyone who can reach it.", Blocks: []Block{pt}})
	}
	return Frame(env, ann, doc), nil
}

func countFixed(es []ConfigEndpoint) int {
	n := 0
	for _, e := range es {
		if e.Fixed {
			n++
		}
	}
	return n
}
