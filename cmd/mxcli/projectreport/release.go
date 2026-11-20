// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"sort"
	"strings"
)

// The release report (ako/mxcli#2424) says what deploying this model does to
// the data already in production. Nothing else does: the model stays valid, mx
// check is clean and the build passes, and the data goes on the next deploy.
//
// The runtime identifies a table by its entity's GUID and a column by its
// attribute's GUID (mendixsystem$entity.id, mendixsystem$attribute.id; measured
// on Mendix 11.12.1 against PostgreSQL, CLAUDE.md "A GUID Is the Database's
// Identity"). So the report pairs the two models exactly as the runtime will:
// by GUID, never by name.
//
//   - Same GUID, other name or module: the runtime renames the table or column,
//     and the rows are kept.
//   - Same name, other GUID: the runtime sees one entity removed and another
//     added, drops the table and creates an empty one. For an attribute the
//     column is dropped and recreated, filled with the model default (#1119).
//   - GUID only in the deployed model: dropped.
//
// A type change keeps the GUID, and what the runtime does with the column then
// — convert it, or drop and recreate it — has not been measured. The report
// says so ("verify") rather than guess.

// ReleaseModel is the part of a model that has data behind it.
type ReleaseModel struct {
	Entities     []RelEntity `json:"entities"`
	Associations []RelAssoc  `json:"associations"`
}

// RelEntity is an entity, stored or not: a change of persistence matters too.
type RelEntity struct {
	GUID        string `json:"guid"`
	ID          string `json:"id"`
	Name        string `json:"name"` // Module.Entity
	Module      string `json:"module"`
	Marketplace bool   `json:"marketplace,omitempty"`
	// Stored: persistable (itself or through its generalization) and not an
	// external entity. Only a stored entity has a table.
	Stored         bool      `json:"stored"`
	Generalization string    `json:"generalization,omitempty"`
	Attributes     []RelAttr `json:"attributes"`
}

// RelAttr is an attribute. A calculated one has no column.
type RelAttr struct {
	GUID       string `json:"guid"`
	Name       string `json:"name"`
	Type       string `json:"type"` // String, Integer, Enumeration, …
	Length     int    `json:"length,omitempty"`
	Calculated bool   `json:"calculated,omitempty"`
}

// RelAssoc is an association (or a cross-module association). From and To are
// the GUIDs of the entities at its ends.
type RelAssoc struct {
	GUID          string `json:"guid"`
	Name          string `json:"name"`
	Module        string `json:"module"`
	From          string `json:"from"`
	To            string `json:"to"`
	FromName      string `json:"fromName"`
	ToName        string `json:"toName"`
	Type          string `json:"type"`          // Reference | ReferenceSet
	StorageFormat string `json:"storageFormat"` // Table | Column
}

// Release effects, in the order the page lists them.
const (
	EffectDestroys = "destroys"
	EffectVerify   = "verify"
	EffectMigrates = "migrates"
	EffectAdds     = "adds"
)

// ReleaseChange is one thing the deploy does to stored data.
type ReleaseChange struct {
	Effect   string `json:"effect"`
	Kind     string `json:"kind"` // entity | attribute | association
	Name     string `json:"name"` // as it will be after the deploy (as it was, if removed)
	Was      string `json:"was,omitempty"`
	Module   string `json:"module"`
	What     string `json:"what"`
	Evidence string `json:"evidence"`
	// Marketplace is set when the element is in a Marketplace module: such a
	// change usually comes with a module update.
	Marketplace bool `json:"marketplace,omitempty"`
}

// Key is the change's annotation key: release:<kind>:<name>.
func (c ReleaseChange) Key() string { return "release:" + c.Kind + ":" + c.Name }

// ReleaseData is the collected release report.
type ReleaseData struct {
	Against string          `json:"against"`
	Current string          `json:"current"`
	Tables  int             `json:"tables"` // stored entities in the deployed model
	Changes []ReleaseChange `json:"changes"`
}

// Count returns the changes with the given effect.
func (d *ReleaseData) Count(effect string) int {
	n := 0
	for _, c := range d.Changes {
		if c.Effect == effect {
			n++
		}
	}
	return n
}

// CompareReleases lists what deploying next does to data stored under deployed.
func CompareReleases(deployed, next *ReleaseModel) *ReleaseData {
	d := &ReleaseData{Changes: []ReleaseChange{}}
	oldByGUID, newByGUID := entityIndex(deployed), entityIndex(next)
	newByName := map[string]*RelEntity{}
	for i := range next.Entities {
		newByName[next.Entities[i].Name] = &next.Entities[i]
	}
	paired := map[string]bool{} // new entity GUIDs accounted for
	add := func(c ReleaseChange) { d.Changes = append(d.Changes, c) }

	for i := range deployed.Entities {
		o := &deployed.Entities[i]
		if !o.Stored {
			continue
		}
		d.Tables++
		n := newByGUID[o.GUID]
		switch {
		case n != nil && n.Stored:
			paired[n.GUID] = true
			if n.Name != o.Name {
				what := "renamed: the runtime renames the table, and its rows are kept"
				if n.Module != o.Module {
					what = "moved to module " + n.Module + ": the runtime renames the table, and its rows are kept"
				}
				add(ReleaseChange{Effect: EffectMigrates, Kind: "entity", Name: n.Name, Was: o.Name, Module: n.Module, Marketplace: n.Marketplace,
					What: what, Evidence: "same GUID " + short(o.GUID)})
			}
			if n.Generalization != o.Generalization {
				add(ReleaseChange{Effect: EffectVerify, Kind: "entity", Name: n.Name, Module: n.Module, Marketplace: n.Marketplace,
					What:     fmt.Sprintf("generalization changed from %s to %s: the inheritance tables change; not measured, verify on a copy of production", orNone(o.Generalization), orNone(n.Generalization)),
					Evidence: "same GUID " + short(o.GUID)})
			}
			compareAttributes(o, n, add)
		case n != nil:
			paired[n.GUID] = true
			add(ReleaseChange{Effect: EffectDestroys, Kind: "entity", Name: n.Name, Was: wasIf(o.Name, n.Name), Module: n.Module, Marketplace: n.Marketplace,
				What: "no longer persistent (or now external): the table and all its rows are dropped", Evidence: "same GUID " + short(o.GUID)})
		default:
			if same := newByName[o.Name]; same != nil && same.Stored && oldByGUID[same.GUID] == nil {
				paired[same.GUID] = true
				add(ReleaseChange{Effect: EffectDestroys, Kind: "entity", Name: o.Name, Module: o.Module, Marketplace: o.Marketplace || same.Marketplace,
					What:     "GUID changed under the same name: the runtime treats it as another entity, drops the table with all its rows and creates an empty one",
					Evidence: "GUID " + short(o.GUID) + " → " + short(same.GUID)})
				continue
			}
			add(ReleaseChange{Effect: EffectDestroys, Kind: "entity", Name: o.Name, Module: o.Module, Marketplace: o.Marketplace,
				What: "removed: the table and all its rows are dropped", Evidence: "GUID " + short(o.GUID) + " is not in the new model"})
		}
	}
	for i := range next.Entities {
		n := &next.Entities[i]
		if !n.Stored || paired[n.GUID] {
			continue
		}
		what := "new entity: a new, empty table"
		if o := oldByGUID[n.GUID]; o != nil {
			what = "now persistent: a new, empty table"
		}
		add(ReleaseChange{Effect: EffectAdds, Kind: "entity", Name: n.Name, Module: n.Module, Marketplace: n.Marketplace, What: what, Evidence: "GUID " + short(n.GUID)})
	}
	compareAssociations(deployed, next, oldByGUID, newByGUID, add)

	rank := map[string]int{EffectDestroys: 0, EffectVerify: 1, EffectMigrates: 2, EffectAdds: 3}
	sort.SliceStable(d.Changes, func(i, j int) bool {
		a, b := d.Changes[i], d.Changes[j]
		if rank[a.Effect] != rank[b.Effect] {
			return rank[a.Effect] < rank[b.Effect]
		}
		return a.Name < b.Name
	})
	return d
}

func entityIndex(m *ReleaseModel) map[string]*RelEntity {
	out := map[string]*RelEntity{}
	for i := range m.Entities {
		if g := m.Entities[i].GUID; g != "" {
			out[g] = &m.Entities[i]
		}
	}
	return out
}

func compareAttributes(o, n *RelEntity, add func(ReleaseChange)) {
	oldBy, newBy, newByName := map[string]*RelAttr{}, map[string]*RelAttr{}, map[string]*RelAttr{}
	for i := range o.Attributes {
		oldBy[o.Attributes[i].GUID] = &o.Attributes[i]
	}
	for i := range n.Attributes {
		newBy[n.Attributes[i].GUID] = &n.Attributes[i]
		newByName[n.Attributes[i].Name] = &n.Attributes[i]
	}
	paired := map[string]bool{}
	ch := func(effect, name, was, what, evidence string) {
		add(ReleaseChange{Effect: effect, Kind: "attribute", Name: n.Name + "." + name, Was: was, Module: n.Module, Marketplace: n.Marketplace, What: what, Evidence: evidence})
	}
	for i := range o.Attributes {
		oa := &o.Attributes[i]
		if oa.Calculated {
			continue // no column
		}
		na := newBy[oa.GUID]
		switch {
		case na != nil && !na.Calculated:
			paired[na.GUID] = true
			if na.Name != oa.Name {
				ch(EffectMigrates, na.Name, o.Name+"."+oa.Name, "renamed: the runtime renames the column, and its values are kept", "same GUID "+short(oa.GUID))
			}
			switch {
			case na.Type != oa.Type:
				ch(EffectVerify, na.Name, "", fmt.Sprintf("type changed from %s to %s: whether the runtime converts the column or drops and recreates it is not measured; verify on a copy of production",
					typeLabel(oa), typeLabel(na)), "same GUID "+short(oa.GUID))
			case na.Type == "String" && shorter(na.Length, oa.Length):
				ch(EffectVerify, na.Name, "", fmt.Sprintf("length lowered from %s to %s: stored values longer than that may be cut or refused; verify on a copy of production",
					lengthLabel(oa.Length), lengthLabel(na.Length)), "same GUID "+short(oa.GUID))
			}
		case na != nil:
			paired[na.GUID] = true
			ch(EffectDestroys, na.Name, "", "now calculated: the column and its values are dropped", "same GUID "+short(oa.GUID))
		default:
			if same := newByName[oa.Name]; same != nil && !same.Calculated && oldBy[same.GUID] == nil {
				paired[same.GUID] = true
				ch(EffectDestroys, oa.Name, "", "GUID changed under the same name: the column is dropped and recreated, and every row gets the model default (ako/mxcli#1119)",
					"GUID "+short(oa.GUID)+" → "+short(same.GUID))
				continue
			}
			ch(EffectDestroys, oa.Name, "", "removed: the column and its values are dropped", "GUID "+short(oa.GUID)+" is not in the new model")
		}
	}
	for i := range n.Attributes {
		na := &n.Attributes[i]
		if na.Calculated || paired[na.GUID] {
			continue
		}
		ch(EffectAdds, na.Name, "", "new column ("+typeLabel(na)+"): existing rows get the model default", "GUID "+short(na.GUID))
	}
}

// compareAssociations pairs associations by GUID. An association between two
// stored entities stores references; one whose end is not stored stores nothing.
func compareAssociations(deployed, next *ReleaseModel, oldEnt, newEnt map[string]*RelEntity, add func(ReleaseChange)) {
	stored := func(a *RelAssoc, ents map[string]*RelEntity) bool {
		f, t := ents[a.From], ents[a.To]
		return f != nil && t != nil && f.Stored && t.Stored
	}
	gone := func(guid string) bool { // the end entity's table is dropped anyway
		n := newEnt[guid]
		return n == nil || !n.Stored
	}
	newBy, newByName := map[string]*RelAssoc{}, map[string]*RelAssoc{}
	oldBy := map[string]*RelAssoc{}
	for i := range next.Associations {
		newBy[next.Associations[i].GUID] = &next.Associations[i]
		newByName[next.Associations[i].Name] = &next.Associations[i]
	}
	for i := range deployed.Associations {
		oldBy[deployed.Associations[i].GUID] = &deployed.Associations[i]
	}
	paired := map[string]bool{}
	ch := func(effect string, a *RelAssoc, was, what, evidence string) {
		mp := false
		if e := newEnt[a.From]; e != nil {
			mp = e.Marketplace
		} else if e := oldEnt[a.From]; e != nil {
			mp = e.Marketplace
		}
		add(ReleaseChange{Effect: effect, Kind: "association", Name: a.Name, Was: was, Module: a.Module, Marketplace: mp, What: what, Evidence: evidence})
	}
	for i := range deployed.Associations {
		o := &deployed.Associations[i]
		if !stored(o, oldEnt) {
			continue
		}
		n := newBy[o.GUID]
		switch {
		case n != nil && stored(n, newEnt):
			paired[n.GUID] = true
			if n.Name != o.Name {
				ch(EffectMigrates, n, o.Name, "renamed: the references are kept", "same GUID "+short(o.GUID))
			}
			if n.From != o.From || n.To != o.To {
				ch(EffectVerify, n, "", fmt.Sprintf("now connects %s → %s (was %s → %s): not measured; verify on a copy of production", n.FromName, n.ToName, o.FromName, o.ToName), "same GUID "+short(o.GUID))
			}
			if n.Type != o.Type || n.StorageFormat != o.StorageFormat {
				ch(EffectVerify, n, "", fmt.Sprintf("changed from %s to %s: the references are migrated or dropped, which is not measured; verify on a copy of production",
					assocShape(o), assocShape(n)), "same GUID "+short(o.GUID))
			}
		case gone(o.From) || gone(o.To):
			// Its table goes with the entity's, which is listed already.
		case n != nil:
			paired[n.GUID] = true
			ch(EffectDestroys, n, "", "an end is no longer persistent: the references are dropped", "same GUID "+short(o.GUID))
		default:
			if same := newByName[o.Name]; same != nil && stored(same, newEnt) && oldBy[same.GUID] == nil {
				paired[same.GUID] = true
				ch(EffectDestroys, o, "", "GUID changed under the same name: the references are dropped and the association starts empty",
					"GUID "+short(o.GUID)+" → "+short(same.GUID))
				continue
			}
			ch(EffectDestroys, o, "", "removed: the references are dropped", "GUID "+short(o.GUID)+" is not in the new model")
		}
	}
	for i := range next.Associations {
		n := &next.Associations[i]
		if paired[n.GUID] || !stored(n, newEnt) {
			continue
		}
		ch(EffectAdds, n, "", "new association ("+assocShape(n)+"): it starts empty", "GUID "+short(n.GUID))
	}
}

func assocShape(a *RelAssoc) string {
	s := a.Type
	if a.StorageFormat != "" {
		s += ", stored as " + strings.ToLower(a.StorageFormat)
	}
	return s
}

func typeLabel(a *RelAttr) string {
	if a.Type == "String" {
		return "String(" + lengthLabel(a.Length) + ")"
	}
	return a.Type
}

// lengthLabel prints a String length; 0 is Mendix's "unlimited".
func lengthLabel(n int) string {
	if n == 0 {
		return "unlimited"
	}
	return fmt.Sprint(n)
}

// shorter reports whether length a is less than b, where 0 is unlimited.
func shorter(a, b int) bool {
	switch {
	case a == 0:
		return false
	case b == 0:
		return true
	default:
		return a < b
	}
}

func short(guid string) string {
	if len(guid) > 8 {
		return guid[:8]
	}
	return guid
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func wasIf(was, now string) string {
	if was == now {
		return ""
	}
	return was
}

// ---------------------------------------------------------------- render

var ReleaseMethod = []string{
	"Both models are read from their stored form: each persistent entity, attribute and association with its GUID. External entities and calculated attributes have no table or column and are left out.",
	"The two are paired by GUID, the way the runtime pairs the model with the database (mendixsystem$entity.id and mendixsystem$attribute.id hold the GUID; measured on Mendix 11.12.1 with PostgreSQL). Names are used only to say that an element kept its name but not its GUID.",
	"Destroys: a table or column dropped — removed, no longer persistent, now calculated, or a GUID changed under the same name. Measured: an entity whose GUID changed loses its table; an attribute whose GUID changed gets its column dropped and recreated with the model default (#1119).",
	"Migrates: the same GUID under another name or module; measured: the runtime renames the table and keeps the rows.",
	"Verify: a change the runtime handles in a way that has not been measured — an attribute type, a shorter string, an association's type, storage or ends, a generalization.",
}

var ReleaseLimits = []string{
	"It compares two models, not a model with a database: --against must be what is deployed. A database that has drifted from its model is not seen.",
	"Enumeration values removed or renamed, and changed default values, are not compared: rows keep values the new model may not know.",
	"Indexes, validation rules and access rules have no stored data behind them and are not compared.",
	"Type conversions are listed as verify because what the runtime does with them has not been measured; try the deploy on a copy of production before trusting one.",
}

// RenderRelease builds the release page.
func RenderRelease(env *Envelope, ann *Annotations) (Doc, error) {
	var d ReleaseData
	if err := env.Decode(&d); err != nil {
		return Doc{}, err
	}
	doc := Doc{Title: "Release data impact", Subtitle: "What deploying this model does to the data in production"}
	destroys := d.Count(EffectDestroys)
	accepted := 0
	for _, c := range d.Changes {
		if c.Effect == EffectDestroys {
			if n, ok := ann.Get(c.Key()); ok && (n.Status == "accepted" || n.Status == "intended") {
				accepted++
			}
		}
	}
	verdict := "No stored data is destroyed."
	switch {
	case destroys > accepted:
		verdict = fmt.Sprintf("This release destroys stored data in %d place(s)", destroys-accepted)
		if accepted > 0 {
			verdict += fmt.Sprintf(" (and %d more that are accepted)", accepted)
		}
		verdict += "."
	case destroys > 0:
		verdict = fmt.Sprintf("This release destroys stored data in %d place(s), all accepted.", destroys)
	}
	doc.Stats = []Stat{
		{Label: "destroys", Value: fmt.Sprint(destroys), Status: statusIf(destroys > accepted, destroys > 0)},
		{Label: "verify", Value: fmt.Sprint(d.Count(EffectVerify)), Status: statusIf(false, d.Count(EffectVerify) > 0)},
		{Label: "migrates", Value: fmt.Sprint(d.Count(EffectMigrates))},
		{Label: "adds", Value: fmt.Sprint(d.Count(EffectAdds))},
		{Label: "tables deployed", Value: fmt.Sprint(d.Tables)},
	}
	kind := "info"
	if destroys > accepted {
		kind = "warn"
	}
	head := []Block{Callout{Kind: kind, Text: verdict + " Compared with " + d.Against + "."}}
	doc.Sections = append(doc.Sections, Section{ID: "verdict", Title: "Verdict", Blocks: head})

	sections := []struct{ effect, id, title, intro string }{
		{EffectDestroys, "destroys", "Destroys data", "Each of these drops a table or a column, with its data, on the next deploy. Annotate `release:<kind>:<name>` with status accepted when it is intended — and say how the data is kept (an export, a migration flow)."},
		{EffectVerify, "verify", "Verify on a copy of production", "The runtime handles these in a way that has not been measured. Deploy to a copy of production first."},
		{EffectMigrates, "migrates", "Migrates, rows kept", "Renamed or moved under the same GUID: the runtime renames the table or column."},
		{EffectAdds, "adds", "Adds", "New tables, columns and associations. Existing rows get the model default."},
	}
	for _, s := range sections {
		t := Table{Columns: []string{"Element", "Kind", "What happens", "Evidence", "Status"}}
		for _, c := range d.Changes {
			if c.Effect != s.effect {
				continue
			}
			name := "`" + c.Name + "`"
			if c.Was != "" {
				name += " (was `" + c.Was + "`)"
			}
			if c.Marketplace {
				name += " — Marketplace module"
			}
			status := ""
			if n, ok := ann.Get(c.Key()); ok {
				status = joinNote(n.Status, n.Note, "")
			}
			t.Rows = append(t.Rows, []string{name, c.Kind, c.What, c.Evidence, status})
			cls := ""
			if c.Effect == EffectDestroys && !(strings.HasPrefix(status, "accepted") || strings.HasPrefix(status, "intended")) {
				cls = "bad"
			}
			t.Classes = append(t.Classes, []string{cls, "", "", "", ""})
		}
		var blocks []Block
		if len(t.Rows) == 0 {
			blocks = []Block{Para{Text: "None."}}
		} else {
			blocks = []Block{t}
		}
		doc.Sections = append(doc.Sections, Section{ID: s.id, Title: s.title, Intro: s.intro, Blocks: blocks})
	}
	return Frame(env, ann, doc), nil
}
