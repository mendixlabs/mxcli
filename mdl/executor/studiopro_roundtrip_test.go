// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// TestStudioProDocumentsRoundTrip is the round trip users perform on a document
// they did not write:
//
//	describe X   ->   exec that output unchanged
//
// It must write nothing. Every other round-trip test starts from MDL, so the
// document it compares was authored by mxcli and holds only what MDL can say;
// the losses live in what Studio Pro stores and MDL does not mention. The
// documents here are Studio Pro-authored: they ship in Mendix's Blank app
// template, which is what `mx create-project` (TestMain) produces, and they are
// byte-identical to the PedApp units the round-trip audit behind ako/mxcli#705
// measured.
//
// Each case is held to several checks, because "the document is not identical"
// is too coarse to track. A page can lose a dozen unrelated things at once, and
// one of them being fixed must be visible even while the others remain:
//
//   - exec:        the describe output re-executes at all.
//   - document:    the stored unit is canonically unchanged (`canon.Equal`, the
//     form the write path elides on — ADR-0008 — so a rebuild's fresh `$ID`s are
//     not a difference) and a second describe prints the same MDL. This is the
//     whole law; the checks below are slices of it.
//   - header:      the unit's top-level scalar properties are unchanged.
//   - texts:       every stored translation survives, as a (language, text)
//     multiset — order is not a loss, dropping one is.
//   - annotations: every annotation connector survives.
//   - source:      for a Java action, the parts of its .java file Studio Pro's
//     regeneration retains — imports, user code, extra code — are unchanged.
//     No BSON comparison sees these, and losing them breaks the build.
//
// A container document (the domain model for an association, the module
// security for a module role) is compared whole, which is stricter than the
// element alone: a rewrite of one member may not disturb its siblings either.
//
// studioProKnownLossy is what is broken TODAY, keyed "<case>|<check>". The test
// fails in both directions: a check that starts passing must be struck off, and
// one that starts failing must be explained.
var studioProKnownLossy = map[string]string{
	// Empty. The last entry, page Administration.Account_Overview|document,
	// passes since its tab container's null default tab is carried
	// (ako/mxcli#1897).
}

type studioProCase struct {
	kind   string // describe keyword(s)
	module string
	name   string
	// storage type and name of the unit to compare; unitName "" selects the
	// module's single unit of that type (domain model, module security).
	unitType string
	unitName string
}

func (c studioProCase) key() string { return c.kind + " " + c.module + "." + c.name }

var studioProCases = []studioProCase{
	{"page", "FeedbackModule", "ShareFeedback", "Forms$Page", "ShareFeedback"},
	{"page", "Administration", "Account_Overview", "Forms$Page", "Account_Overview"},
	{"nanoflow", "FeedbackModule", "ACT_Feedback_UploadImage", "Microflows$Nanoflow", "ACT_Feedback_UploadImage"},
	// Stores ReturnVariableName "Feedback"; the audited one above stores "".
	{"nanoflow", "FeedbackModule", "SUB_Feedback_GetOrCreate", "Microflows$Nanoflow", "SUB_Feedback_GetOrCreate"},
	{"java action", "FeedbackModule", "XSS_Sanitizer", "JavaActions$JavaAction", "XSS_Sanitizer"},
	{"snippet", "FeedbackModule", "_ReadMe", "Forms$Snippet", "_ReadMe"},
	{"association", "Administration", "AccountPasswordData_Account", "DomainModels$DomainModel", ""},
	{"module role", "Administration", "Administrator", "Security$ModuleSecurity", ""},
}

var studioProChecks = []string{"exec", "document", "header", "texts", "annotations", "source"}

func TestStudioProDocumentsRoundTrip(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()
	copyJavaSource(t, env)

	for _, c := range studioProCases {
		t.Run(c.key(), func(t *testing.T) {
			checkStudioProRoundTrip(t, env, c)
		})
	}

	valid := map[string]bool{}
	for _, c := range studioProCases {
		for _, chk := range studioProChecks {
			valid[c.key()+"|"+chk] = true
		}
	}
	for k := range studioProKnownLossy {
		if !valid[k] {
			t.Errorf("studioProKnownLossy lists %q, which is not a case|check — stale entry, remove it", k)
		}
	}
}

// TestStudioProRoundTripControl proves the comparison can see a change. Without
// it, "nothing was written" and "the comparison never looked" are the same green
// run — the trap that let PR #125 ship.
func TestStudioProRoundTripControl(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	c := studioProCases[0] // page FeedbackModule.ShareFeedback
	_, before := studioProUnit(t, env.projectPath, c)
	// One deliberate edit, through the path that is known to carry everything
	// else (ALTER PAGE edits the stored document in place).
	if err := env.executeMDL(`alter page FeedbackModule.ShareFeedback {
  set Caption = 'Round-trip control' on actionButton1
};`); err != nil {
		t.Fatalf("control edit: %v", err)
	}
	_, after := studioProUnit(t, env.projectPath, c)
	equal, err := canon.Equal(before, after)
	if err != nil {
		t.Fatalf("canonical compare: %v", err)
	}
	if equal {
		t.Fatalf("an edited caption left the page canonically unchanged — the comparison cannot detect a write")
	}
	if lost, _ := lostTranslations(before, after); len(lost) == 0 {
		t.Fatalf("an edited caption lost no translation — the texts check cannot detect a change")
	}
}

func checkStudioProRoundTrip(t *testing.T, env *testEnv, c studioProCase) {
	t.Helper()
	describeCmd := "describe " + c.kind + " " + c.module + "." + c.name

	unitID, before := studioProUnit(t, env.projectPath, c)
	sourceBefore := javaSourceSections(env.projectPath, c)
	described, err := env.describeMDL(describeCmd)
	if err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if strings.TrimSpace(described) == "" {
		t.Fatalf("describe produced no output")
	}

	execErr := env.executeMDL(described)
	verdict(t, c, "exec", execErr == nil, func() string {
		return fmt.Sprintf("re-executing DESCRIBE output failed: %v\n--- output ---\n%s", execErr, described)
	})
	if execErr != nil {
		return
	}

	_, after := studioProUnit(t, env.projectPath, c)
	// MXCLI_ROUNDTRIP_DUMP=<dir> keeps both documents for a closer look: the
	// path diff below is positional, so a reordered list reads as noise.
	if dir := os.Getenv("MXCLI_ROUNDTRIP_DUMP"); dir != "" {
		base := filepath.Join(dir, strings.ReplaceAll(c.key(), " ", "_"))
		_ = os.WriteFile(base+".before.bson", before, 0o644)
		_ = os.WriteFile(base+".after.bson", after, 0o644)
	}

	equal, err := canon.Equal(before, after)
	if err != nil {
		t.Fatalf("canonical compare failed (unit %s): %v", unitID, err)
	}
	redescribed, err := env.describeMDL(describeCmd)
	if err != nil {
		t.Fatalf("second describe failed: %v", err)
	}
	verdict(t, c, "document", equal && redescribed == described, func() string {
		var b strings.Builder
		if !equal {
			fmt.Fprintf(&b, "DESCRIBE output rebuilt a different document (unit %s):\n  %s\n",
				unitID, strings.Join(bsonPathDiff(before, after, 40), "\n  "))
		}
		if redescribed != described {
			fmt.Fprintf(&b, "a second DESCRIBE differs from the first:\n--- second ---\n%s\n", redescribed)
		}
		fmt.Fprintf(&b, "--- DESCRIBE output ---\n%s", described)
		return b.String()
	})

	headerDiffs := headerDiff(before, after)
	verdict(t, c, "header", len(headerDiffs) == 0, func() string {
		return "top-level properties changed:\n  " + strings.Join(headerDiffs, "\n  ")
	})

	lost, gained := lostTranslations(before, after)
	verdict(t, c, "texts", len(lost) == 0, func() string {
		return fmt.Sprintf("translations lost:\n  %s\ngained:\n  %s",
			strings.Join(lost, "\n  "), strings.Join(gained, "\n  "))
	})

	nb, na := countType(before, "Microflows$AnnotationFlow"), countType(after, "Microflows$AnnotationFlow")
	verdict(t, c, "annotations", nb == na, func() string {
		return fmt.Sprintf("annotation connectors: %d -> %d", nb, na)
	})

	sourceAfter := javaSourceSections(env.projectPath, c)
	verdict(t, c, "source", sourceBefore == sourceAfter, func() string {
		return fmt.Sprintf("the .java file's retained sections changed:\n--- before ---\n%s\n--- after ---\n%s",
			sourceBefore, sourceAfter)
	})
}

// javaSourceSections renders the parts of a Java action's source file that
// Studio Pro's regeneration retains, whitespace-normalised per line (a rewrite
// may re-indent; the build does not care). "" for any other case.
func javaSourceSections(project string, c studioProCase) string {
	if c.unitType != "JavaActions$JavaAction" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(project), "javasource",
		strings.ToLower(c.module), "actions", c.name+".java"))
	if err != nil {
		return "<no source file>"
	}
	src := string(raw)
	imports, extra := javaactions.RetainedSections(src)
	user := ""
	if i := strings.Index(src, "// BEGIN USER CODE"); i >= 0 {
		if j := strings.Index(src[i:], "// END USER CODE"); j >= 0 {
			user = src[i : i+j]
		}
	}
	norm := func(s string) string {
		var out []string
		for l := range strings.SplitSeq(s, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
		return strings.Join(out, "\n")
	}
	return "imports:\n" + strings.Join(imports, "\n") + "\nuser:\n" + norm(user) + "\nextra:\n" + norm(extra)
}

// verdict reports one check against the ledger, failing in both directions.
func verdict(t *testing.T, c studioProCase, check string, ok bool, detail func() string) {
	t.Helper()
	key := c.key() + "|" + check
	reason, lossy := studioProKnownLossy[key]
	switch {
	case ok && lossy:
		t.Errorf("%s: now passes — remove it from studioProKnownLossy (was: %s)", check, reason)
	case !ok && !lossy:
		t.Errorf("%s: %s", check, detail())
	case !ok:
		t.Logf("%s: still lossy, as expected (%s)", check, reason)
	}
}

// copyJavaSource adds the project's javasource tree to the test copy. A Java
// action's DESCRIBE reads its body from there; without it the output carries a
// placeholder, which is not the round trip a user performs.
func copyJavaSource(t *testing.T, env *testEnv) {
	t.Helper()
	src := filepath.Join(sharedSourceProject, "javasource")
	if _, err := os.Stat(src); err != nil {
		return
	}
	if err := copyDir(src, filepath.Join(filepath.Dir(env.projectPath), "javasource")); err != nil {
		t.Fatalf("copy javasource: %v", err)
	}
}

// studioProUnit returns the unit id and stored bytes of the case's unit.
func studioProUnit(t *testing.T, project string, c studioProCase) (string, []byte) {
	t.Helper()
	moduleIDs := map[string]string{} // unit id -> module name
	for name, id := range moduleUnitIDs(t, project) {
		moduleIDs[id] = name
	}

	r, err := mmpr.Open(project)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	defer r.Close()
	parents, err := r.BuildContainerParent()
	if err != nil {
		t.Fatalf("container parents: %v", err)
	}
	units, err := r.ListUnitsByType(c.unitType)
	if err != nil {
		t.Fatalf("list %s: %v", c.unitType, err)
	}
	for _, u := range units {
		if mmpr.ResolveModuleName(u.ContainerID, moduleIDs, parents) != c.module {
			continue
		}
		b, err := r.GetRawUnitBytes(u.ID)
		if err != nil {
			t.Fatalf("read unit %s: %v", u.ID, err)
		}
		if _, n := documentTypeAndName(b); c.unitName == "" || n == c.unitName {
			return u.ID, b
		}
	}
	t.Fatalf("no %s %q in module %s", c.unitType, c.unitName, c.module)
	return "", nil
}

// headerDiff lists the top-level scalar properties that differ. Sub-documents,
// lists and binaries are left to the other checks.
func headerDiff(a, b []byte) []string {
	scalars := func(raw []byte) map[string]string {
		out := map[string]string{}
		var doc bson.D
		if bson.Unmarshal(raw, &doc) != nil {
			return out
		}
		for _, e := range doc {
			switch e.Value.(type) {
			case bson.D, bson.A, bson.Binary:
				continue
			}
			out[e.Key] = fmt.Sprintf("%#v", e.Value)
		}
		return out
	}
	sa, sb := scalars(a), scalars(b)
	keys := map[string]bool{}
	for k := range sa {
		keys[k] = true
	}
	for k := range sb {
		keys[k] = true
	}
	var out []string
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		va, oka := sa[k]
		vb, okb := sb[k]
		if oka == okb && va == vb {
			continue
		}
		if !oka {
			va = "<absent>"
		}
		if !okb {
			vb = "<absent>"
		}
		out = append(out, fmt.Sprintf("%s: %s -> %s", k, va, vb))
	}
	return out
}

// lostTranslations compares every Texts$Translation in the two documents as a
// (language, text) multiset and returns the pairs only the first holds, and only
// the second.
func lostTranslations(a, b []byte) (lost, gained []string) {
	pairs := func(raw []byte) map[string]int {
		out := map[string]int{}
		walkBSON(raw, func(d bson.D) {
			if docString(d, "$Type") != "Texts$Translation" {
				return
			}
			out[fmt.Sprintf("%s %q", docString(d, "LanguageCode"), docString(d, "Text"))]++
		})
		return out
	}
	pa, pb := pairs(a), pairs(b)
	for _, k := range slices.Sorted(maps.Keys(pa)) {
		for range pa[k] - pb[k] {
			lost = append(lost, k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(pb)) {
		for range pb[k] - pa[k] {
			gained = append(gained, k)
		}
	}
	return lost, gained
}

func countType(raw []byte, typeName string) int {
	n := 0
	walkBSON(raw, func(d bson.D) {
		if docString(d, "$Type") == typeName {
			n++
		}
	})
	return n
}

func walkBSON(raw []byte, visit func(bson.D)) {
	var doc bson.D
	if bson.Unmarshal(raw, &doc) != nil {
		return
	}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			visit(x)
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
}

func docString(d bson.D, key string) string {
	for _, e := range d {
		if e.Key == key {
			s, _ := e.Value.(string)
			return s
		}
	}
	return ""
}

// bsonPathDiff lists the leaf paths that differ between two documents, with
// `$ID`s and binary pointers left out — they are what canon normalises, so
// listing them would bury the real difference. Diagnostic only: it is
// positional, so a reordered list reads as a run of changes.
func bsonPathDiff(a, b []byte, limit int) []string {
	fa, fb := map[string]string{}, map[string]string{}
	flattenBSON(a, fa)
	flattenBSON(b, fb)
	keys := map[string]bool{}
	for k := range fa {
		keys[k] = true
	}
	for k := range fb {
		keys[k] = true
	}
	var out []string
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		va, oka := fa[k]
		vb, okb := fb[k]
		if oka && okb && va == vb {
			continue
		}
		if !oka {
			va = "<absent>"
		}
		if !okb {
			vb = "<absent>"
		}
		out = append(out, fmt.Sprintf("%s: %s -> %s", k, va, vb))
		if len(out) == limit {
			out = append(out, "…")
			break
		}
	}
	return out
}

func flattenBSON(raw []byte, out map[string]string) {
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return
	}
	var walk func(p string, v any)
	walk = func(p string, v any) {
		switch x := v.(type) {
		case bson.D:
			for _, e := range x {
				if e.Key == "$ID" {
					continue
				}
				walk(p+"/"+e.Key, e.Value)
			}
		case bson.A:
			for i, e := range x {
				walk(fmt.Sprintf("%s[%d]", p, i), e)
			}
		case bson.Binary:
			// pointers: canon compares them structurally
		default:
			s := fmt.Sprintf("%q", fmt.Sprint(x))
			if len(s) > 80 {
				s = s[:80] + "…"
			}
			out[p] = s
		}
	}
	walk("", doc)
}
