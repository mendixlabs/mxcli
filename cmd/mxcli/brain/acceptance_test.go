// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"
	"testing"
	"time"
)

func TestAcceptanceLinesParseAndRoundTrip(t *testing.T) {
	line, err := AcceptanceLine("sec001", "@Dossier.Beperking", "read only through a logged data source")
	if err != nil {
		t.Fatal(err)
	}
	if line != "Accepts lint `SEC001` on `@Dossier.Beperking`: read only through a logged data source" {
		t.Errorf("line = %q", line)
	}
	e := Entry{ID: "d1", Title: "No access rules on Beperking", Body: "Context.\n" + line + "\n- Accepts lint `MDL-FLOW01` on `Sales`: legacy"}
	as := Acceptances(e)
	if len(as) != 2 || as[0].Rule != "SEC001" || as[0].Element != "Dossier.Beperking" || as[1].Rule != "MDL-FLOW01" || as[1].Element != "Sales" {
		t.Errorf("parsed = %+v", as)
	}
	if _, err := AcceptanceLine("not a rule", "@Sales", ""); err == nil {
		t.Error("a malformed rule id is refused")
	}
	if l, _ := AcceptanceLine("SEC003", "project", ""); l != "Accepts lint `SEC003` on `project`" {
		t.Errorf("project line = %q", l)
	}
}

func TestAddAcceptancesFollowsTheAnchors(t *testing.T) {
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	d, _ := NewEntry("Pages bind Core.Medewerker directly", []string{"@Core.Medewerker", "@Rooster"}, now)
	d, err := AddAcceptances(d, []string{"ARCH001:the shell owns the layout, by design"})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := AddAcceptances(d, []string{"ARCH001:the shell owns the layout, by design"})
	if strings.Count(d.Body, "Accepts lint") != 2 || again.Body != d.Body {
		t.Errorf("body = %q", d.Body)
	}
	p, _ := NewEntry("Demo users stay on in the demo app", nil, now)
	if p, err = AddAcceptances(p, []string{"SEC003"}); err != nil || !strings.Contains(p.Body, "on `project`") {
		t.Errorf("an unanchored decision accepts project-wide: %q %v", p.Body, err)
	}
	q, _ := NewQuestion("Accept SEC001?", []string{"@Core.X"}, "", now)
	if _, err := AddAcceptances(q, []string{"SEC001"}); err == nil {
		t.Error("an open question cannot accept a finding")
	}
}

func TestAcceptanceCovers(t *testing.T) {
	doc := Acceptance{Rule: "SEC001", Element: "Dossier.Beperking"}
	member := Acceptance{Rule: "SEC001", Element: "Dossier.Beperking.Reden"}
	mod := Acceptance{Rule: "SEC001", Element: "Dossier"}
	proj := Acceptance{Rule: "SEC013", Element: "project"}
	for _, c := range []struct {
		a                      Acceptance
		rule, module, document string
		want                   bool
	}{
		{doc, "SEC001", "Dossier", "Beperking", true},
		{doc, "sec001", "Dossier", "Dossier.Beperking", true}, // qualified, any case
		{doc, "SEC001", "Dossier", "Other", false},
		{doc, "SEC002", "Dossier", "Beperking", false},
		{member, "SEC001", "Dossier", "Beperking", true},
		{mod, "SEC001", "Dossier", "Anything", true},
		{mod, "SEC001", "Other", "Anything", false},
		{proj, "SEC013", "", "ProjectSecurity", true},
		{proj, "SEC013", "Dossier", "X", false},
		{doc, "SEC001", "", "Beperking", false},
	} {
		if got := c.a.Covers(c.rule, c.module, c.document); got != c.want {
			t.Errorf("%+v covers %s %s.%s = %v, want %v", c.a, c.rule, c.module, c.document, got, c.want)
		}
	}
}

// brain check refuses an acceptance on an open question, and one whose
// element is not in the model.
func TestCheckValidatesAcceptances(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	good, _ := NewEntry("Beperking has no access rules\nAccepts lint `SEC001` on `@Core.ACT_Read`: logged", []string{"@Core.ACT_Read"}, now)
	gone, _ := NewEntry("Old\nAccepts lint `SEC001` on `@Core.MissingThing`: gone", []string{"@Core.ACT_Read"}, now)
	q, _ := NewQuestion("Should we?\nAccepts lint `SEC001` on `@Core.ACT_Read`", []string{"@Core.ACT_Read"}, "", now)
	proj, _ := NewEntry("Demo app\nAccepts lint `SEC013` on `project`: demo only", nil, now)
	for _, e := range []Entry{good, gone, q} {
		if err := s.Promote(e, "Core"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Promote(proj, ProjectShard); err != nil {
		t.Fatal(err)
	}
	shards, _ := s.ListShards()
	rep, err := Check(s, kindResolver{}, shards)
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	for _, b := range rep.BadAcceptances {
		problems = append(problems, b.EntryID+":"+b.Problem)
	}
	joined := strings.Join(problems, "\n")
	if len(rep.BadAcceptances) != 2 || !strings.Contains(joined, gone.ID+":@Core.MissingThing is not in the model") ||
		!strings.Contains(joined, q.ID+":accepting a lint finding is a decision") {
		t.Errorf("bad acceptances:\n%s", joined)
	}
	if !rep.Failed() {
		t.Error("a bad acceptance fails the check")
	}
	res, _ := s.List(ListFilter{})
	for _, e := range res.Entries {
		if e.ID == proj.ID && (len(e.Accepts) != 1 || e.Accepts[0].Element != "project" || e.Accepts[0].Reason != "demo only") {
			t.Errorf("list accepts = %+v", e.Accepts)
		}
	}
}
