// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The dependencies report (ako/mxcli#2425): what the app stands on, and
// whether that is current and safe — the Mendix version's release line, the
// Marketplace modules, the pluggable widgets, and the Java libraries with their
// known vulnerabilities.
//
// The network sources are optional. What could not be checked — no
// Marketplace token, --offline, a jar without Maven coordinates — is said, per
// item, and never shown as "up to date" or "no vulnerabilities".

// DependencyData is the collected dependencies report.
type DependencyData struct {
	MendixVersion string `json:"mendixVersion"`
	// Line is LTS, MTS, monthly or unknown (sdk/versions registry);
	// LineName the minor it names ("10.24").
	Line     string `json:"line"`
	LineName string `json:"lineName"`

	Modules   []DepModule  `json:"modules"`
	Widgets   []DepWidget  `json:"widgets"`
	Libraries []DepLibrary `json:"libraries"`

	// MarketplaceChecked is false when the Marketplace was not asked;
	// MarketplaceNote says why (no token, --offline, an error).
	MarketplaceChecked bool   `json:"marketplaceChecked"`
	MarketplaceNote    string `json:"marketplaceNote,omitempty"`
	// VulnChecked is false when the advisory database was not asked.
	VulnChecked bool   `json:"vulnChecked"`
	VulnNote    string `json:"vulnNote,omitempty"`
	VulnSource  string `json:"vulnSource,omitempty"`
}

// Dependency statuses.
const (
	DepUpToDate      = "up to date"
	DepNewer         = "newer available"
	DepNotIdentified = "not identified"
	DepNotChecked    = "not checked"
)

// DepModule is a Marketplace module.
type DepModule struct {
	Name      string `json:"name"`
	Installed string `json:"installed"`
	// VersionID is the Marketplace version UUID the module records
	// (AppStoreGuid): what identifies the content, not the version number.
	VersionID string `json:"versionId,omitempty"`
	ContentID int    `json:"contentId,omitempty"`
	// Latest is the newest published version; LatestCompatible the newest
	// whose minimum Mendix version is at most the project's.
	Latest           string `json:"latest,omitempty"`
	LatestCompatible string `json:"latestCompatible,omitempty"`
	Status           string `json:"status"`
	Note             string `json:"note,omitempty"`
}

// DepWidget is a pluggable widget package in widgets/.
type DepWidget struct {
	Package string `json:"package"` // file name
	Name    string `json:"name"`
	Version string `json:"version"`
	Latest  string `json:"latest,omitempty"`
	Status  string `json:"status"`
	Note    string `json:"note,omitempty"`
}

// DepLibrary is a jar in userlib/ or vendorlib/.
type DepLibrary struct {
	File    string `json:"file"` // relative to the project
	Group   string `json:"group,omitempty"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	// Coordinates says where group, name and version came from:
	// "pom.properties" (inside the jar) or "file name" (no group).
	Coordinates string    `json:"coordinates"`
	Checked     bool      `json:"checked"`
	Note        string    `json:"note,omitempty"`
	Vulns       []DepVuln `json:"vulns,omitempty"`
}

// DepVuln is one advisory against a library version.
type DepVuln struct {
	ID       string   `json:"id"`
	Aliases  []string `json:"aliases,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Severity string   `json:"severity,omitempty"` // LOW | MODERATE | HIGH | CRITICAL, or ""
	Fixed    []string `json:"fixed,omitempty"`
}

// Blocking reports whether a vulnerability blocks a release: high or critical.
func (v DepVuln) Blocking() bool {
	s := strings.ToUpper(v.Severity)
	return s == "HIGH" || s == "CRITICAL"
}

// Coordinate is "group:name:version" as far as it is known.
func (l DepLibrary) Coordinate() string {
	s := l.Name
	if l.Group != "" {
		s = l.Group + ":" + s
	}
	if l.Version != "" {
		s += ":" + l.Version
	}
	return s
}

// ---------------------------------------------------------------- render

var DependenciesMethod = []string{
	"The Mendix version is the project's; its release line (LTS, MTS or a monthly release) comes from mxcli's version registry, which records the lines but no support dates.",
	"Marketplace modules are the modules that record a Marketplace version (FromAppStore). A module is identified by the version UUID it records (AppStoreGuid), not by its name or version number, which collide across content; the newest compatible version is the newest whose minimum Mendix version is at most the project's. This needs a Marketplace token (mxcli auth login).",
	"Pluggable widgets are the packages in widgets/, with the name and version their package.xml declares; the newest published version comes from the Marketplace catalog by name.",
	"Java libraries are the jars in userlib/ and vendorlib/. Maven coordinates come from the jar's own META-INF/maven/…/pom.properties; a jar without them is named after its file and cannot be looked up. Known vulnerabilities come from the OSV database (osv.dev), queried by Maven coordinates.",
}

var DependenciesLimits = []string{
	"Local changes to a Marketplace module are not measured here: `mxcli marketplace diff <content-id>` compares a module with the package it was installed from.",
	"Support end dates are not known to mxcli; check the Mendix support policy for the line the project is on.",
	"Vulnerabilities in JavaScript dependencies of widgets, in the Mendix runtime itself, and in jars that are not in userlib/ or vendorlib/ are not checked.",
	"A vulnerability is listed for the library version; whether the app reaches the vulnerable code is not judged.",
}

// RenderDependencies builds the dependencies page.
func RenderDependencies(env *Envelope, ann *Annotations) (Doc, error) {
	var d DependencyData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	doc := Doc{Title: "Dependencies", Subtitle: "The Mendix version, Marketplace modules, widgets and Java libraries the app stands on"}

	var blocking, vulnLibs, behind int
	for _, l := range d.Libraries {
		if len(l.Vulns) > 0 {
			vulnLibs++
		}
		for _, v := range l.Vulns {
			if v.Blocking() {
				if n, ok := ann.Get("vuln:" + v.ID); !ok || (n.Status != "accepted" && n.Status != "intended") {
					blocking++
				}
			}
		}
	}
	for _, m := range d.Modules {
		if m.Status == DepNewer {
			behind++
		}
	}
	doc.Stats = []Stat{
		{Label: "Mendix", Value: d.MendixVersion, Note: lineNote(d), Status: statusIf(false, d.Line == "monthly")},
		{Label: "Marketplace modules", Value: fmt.Sprint(len(d.Modules)), Note: checkedNote(d.MarketplaceChecked, fmt.Sprintf("%d behind", behind)), Status: statusIf(false, !d.MarketplaceChecked || behind > 0)},
		{Label: "widget packages", Value: fmt.Sprint(len(d.Widgets))},
		{Label: "Java libraries", Value: fmt.Sprint(len(d.Libraries)), Note: checkedNote(d.VulnChecked, fmt.Sprintf("%d with known vulnerabilities", vulnLibs))},
		{Label: "high/critical vulnerabilities", Value: countOrDash(d.VulnChecked, blocking), Status: vulnStatus(d.VulnChecked, blocking)},
	}

	var head []Block
	switch {
	case blocking > 0:
		head = append(head, Callout{Kind: "warn", Text: fmt.Sprintf("%d high or critical vulnerability(ies) in the Java libraries. Upgrade the library to a fixed version, or annotate `vuln:<id>` with status accepted and why it does not apply.", blocking)})
	case d.VulnChecked:
		head = append(head, Callout{Kind: "info", Text: "No high or critical vulnerability is known for the Java libraries that have Maven coordinates."})
	}
	if !d.VulnChecked {
		head = append(head, Callout{Kind: "warn", Text: "Java libraries were not checked for vulnerabilities: " + d.VulnNote})
	}
	if !d.MarketplaceChecked {
		head = append(head, Callout{Kind: "warn", Text: "Marketplace versions were not checked: " + d.MarketplaceNote})
	}
	if d.Line == "monthly" {
		head = append(head, Callout{Kind: "warn", Text: "Mendix " + d.MendixVersion + " is a monthly release, not on an LTS or MTS line: it is supported only until the next release. Plan the move to an LTS or MTS version."})
	}
	if len(head) > 0 {
		doc.Sections = append(doc.Sections, Section{ID: "verdict", Title: "Verdict", Blocks: head})
	}

	// Java libraries.
	lt := Table{Columns: []string{"Library", "File", "Vulnerabilities", "Upgrade to at least", "Status"}}
	libs := append([]DepLibrary(nil), d.Libraries...)
	sort.SliceStable(libs, func(i, j int) bool { return worst(libs[i]) > worst(libs[j]) })
	for _, l := range libs {
		vulns, fixed := "", ""
		cls := ""
		switch {
		case !l.Checked:
			vulns = "not checked: " + l.Note
		case len(l.Vulns) == 0:
			vulns = "none known"
		default:
			var parts []string
			for _, v := range l.Vulns {
				p := v.ID
				if v.Severity != "" {
					p += " (" + strings.ToLower(v.Severity) + ")"
				}
				if v.Summary != "" {
					p += ": " + v.Summary
				}
				parts = append(parts, p)
				if v.Blocking() {
					cls = "bad"
				} else if cls == "" {
					cls = "warn"
				}
			}
			vulns = strings.Join(parts, "; ")
			fixed = upgradeTo(l)
		}
		status := ""
		for _, v := range l.Vulns {
			if n, ok := ann.Get("vuln:" + v.ID); ok {
				status = joinNote(v.ID+" "+n.Status, n.Note, "")
			}
		}
		lt.Rows = append(lt.Rows, []string{"`" + l.Coordinate() + "`", l.File, vulns, fixed, status})
		lt.Classes = append(lt.Classes, []string{"", "", cls, "", ""})
	}
	libBlocks := []Block{lt}
	if len(d.Libraries) == 0 {
		libBlocks = []Block{Para{Text: "No jars in userlib/ or vendorlib/."}}
	}
	doc.Sections = append(doc.Sections, Section{ID: "libraries", Title: "Java libraries", Intro: srcIntro(d), Blocks: libBlocks})

	// Marketplace modules.
	mt := Table{Columns: []string{"Module", "Installed", "Newest compatible", "Newest", "Status"}}
	for _, m := range d.Modules {
		st := m.Status
		if m.Note != "" {
			st += ": " + m.Note
		}
		if n, ok := ann.Get("module:" + m.Name); ok {
			st = joinNote(n.Status, n.Note, st)
		}
		mt.Rows = append(mt.Rows, []string{m.Name, m.Installed, m.LatestCompatible, m.Latest, st})
		cls := ""
		if m.Status == DepNewer {
			cls = "warn"
		}
		mt.Classes = append(mt.Classes, []string{"", "", "", "", cls})
	}
	modBlocks := []Block{mt}
	if len(d.Modules) == 0 {
		modBlocks = []Block{Para{Text: "No module records a Marketplace version."}}
	}
	doc.Sections = append(doc.Sections, Section{ID: "modules", Title: "Marketplace modules",
		Intro:  "Before updating a module, check it was not changed locally: `mxcli marketplace diff <content-id>`. An update replaces the module and drops local changes.",
		Blocks: modBlocks})

	// Widgets.
	wt := Table{Columns: []string{"Widget", "Package", "Version", "Newest", "Status"}}
	for _, w := range d.Widgets {
		st := w.Status
		if w.Note != "" {
			st += ": " + w.Note
		}
		wt.Rows = append(wt.Rows, []string{w.Name, w.Package, w.Version, w.Latest, st})
		cls := ""
		if w.Status == DepNewer {
			cls = "warn"
		}
		wt.Classes = append(wt.Classes, []string{"", "", "", "", cls})
	}
	widBlocks := []Block{wt}
	if len(d.Widgets) == 0 {
		widBlocks = []Block{Para{Text: "No widget packages in widgets/."}}
	}
	doc.Sections = append(doc.Sections, Section{ID: "widgets", Title: "Pluggable widgets",
		Intro:  "A widget package newer than the pages that use it makes CE0463 (\"the definition of this widget has changed\"): after updating one, `mxcli report` shows it under Builds, and `mxcli fix widgets` updates the pages.",
		Blocks: widBlocks})
	return Frame(env, ann, doc), nil
}

// worst ranks a library by its worst advisory, for ordering.
func worst(l DepLibrary) int {
	w := 0
	for _, v := range l.Vulns {
		r := map[string]int{"LOW": 1, "MODERATE": 2, "MEDIUM": 2, "HIGH": 3, "CRITICAL": 4}[strings.ToUpper(v.Severity)]
		if r == 0 {
			r = 1
		}
		if r > w {
			w = r
		}
	}
	return w
}

func lineNote(d DependencyData) string {
	switch d.Line {
	case "LTS", "MTS":
		return d.Line + " line " + d.LineName
	case "monthly":
		return "monthly release"
	default:
		return "release line not known to mxcli"
	}
}

func checkedNote(checked bool, s string) string {
	if !checked {
		return "not checked"
	}
	return s
}

func countOrDash(checked bool, n int) string {
	if !checked {
		return "—"
	}
	return fmt.Sprint(n)
}

func srcIntro(d DependencyData) string {
	s := "Jars in userlib/ and vendorlib/, worst first."
	if d.VulnSource != "" {
		s += " Advisories from " + d.VulnSource + "."
	}
	return s + " Annotate `vuln:<id>` with status accepted, and why, for one that does not apply."
}

// upgradeTo is the version that fixes every known advisory of the library, as
// far as the advisories say: for each, the lowest fixed version above the
// installed one, and the highest of those. Advisories that name no such fix
// are listed after it.
func upgradeTo(l DepLibrary) string {
	target := ""
	var unfixed []string
	for _, v := range l.Vulns {
		lowest := ""
		for _, f := range v.Fixed {
			if l.Version != "" && compareVersions(f, l.Version) <= 0 {
				continue
			}
			if lowest == "" || compareVersions(f, lowest) < 0 {
				lowest = f
			}
		}
		if lowest == "" {
			unfixed = append(unfixed, v.ID)
			continue
		}
		if target == "" || compareVersions(lowest, target) > 0 {
			target = lowest
		}
	}
	if len(unfixed) > 0 {
		note := "no fixed version known for " + strings.Join(unfixed, ", ")
		if target == "" {
			return note
		}
		return target + " (" + note + ")"
	}
	return target
}

// compareVersions compares dotted versions numerically where it can
// ("2.10.5.1" > "2.9.10"); a non-numeric part compares as text.
func compareVersions(a, b string) int {
	as, bs := strings.FieldsFunc(a, versionSep), strings.FieldsFunc(b, versionSep)
	for i := 0; i < len(as) || i < len(bs); i++ {
		x, y := "0", "0"
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		switch {
		case xerr == nil && yerr == nil:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		case x != y:
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionSep(r rune) bool { return r == '.' || r == '-' }
