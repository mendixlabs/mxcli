// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1951: which sentence of which document an entry came from.

func writeDoc(t *testing.T, base, rel string) {
	t.Helper()
	p := filepath.Join(base, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("# PRD\n\nA planner can create a visit.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSourceRoundTripsThroughAShard(t *testing.T) {
	req := mustRequirement(t, "A planner can create a visit\n\nWith a window.", "01-scheduling", "@Scheduling.Visit")
	req.Source = "docs/sample-prd.md#L18-21"

	text := RenderShard(req.Shard(), []Entry{req})
	// Its own line, IMMEDIATELY after the metadata line: never on it (an
	// older parser's exact regex would call every such entry malformed and
	// fail `check`), never before it (an older parser drops those lines on
	// its next rewrite). After it, an older build reads it as body and keeps it.
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(ln, "Anchors: ") {
			if strings.Contains(ln, "sample-prd") {
				t.Errorf("source rendered on the metadata line: %q", ln)
			}
			if i+1 >= len(lines) || lines[i+1] != "Source: `docs/sample-prd.md#L18-21`" {
				t.Errorf("line after metadata = %q, want the Source line", lines[i+1])
			}
		}
	}

	got, malformed, err := ParseShard(req.Shard(), text)
	if err != nil || len(malformed) != 0 || len(got) != 1 {
		t.Fatalf("parse: %v %v %+v", err, malformed, got)
	}
	if got[0].Source != req.Source || got[0].Body != "With a window." {
		t.Errorf("round trip: source %q, body %q", got[0].Source, got[0].Body)
	}

	// Control: an entry without a source renders exactly as before, so
	// existing shards are byte-for-byte unchanged by this feature.
	req.Source = ""
	if strings.Contains(RenderShard(req.Shard(), []Entry{req}), "Source:") {
		t.Error("an entry with no source must not render a Source line")
	}
}

// An older mxcli reads the Source line as the first line of the body, and on
// its next rewrite renders that body after a blank line. The new parser must
// take the line back out of the body in that shape too, or one round trip
// through an old build turns provenance into prose.
func TestSourceSurvivesARewriteByAnOlderBuild(t *testing.T) {
	const oldRewrite = "## A planner can create a visit\n\n" +
		"Anchors: `@Scheduling.Visit` · id `abc123` · 2026-09-03\n\n" +
		"Source: `docs/sample-prd.md#L18-21`\n\nWith a window.\n"
	got, malformed, err := ParseShard(PlanShard("01-scheduling"), oldRewrite)
	if err != nil || len(malformed) != 0 || len(got) != 1 {
		t.Fatalf("parse: %v %v %+v", err, malformed, got)
	}
	if got[0].Source != "docs/sample-prd.md#L18-21" || got[0].Body != "With a window." {
		t.Errorf("source %q, body %q", got[0].Source, got[0].Body)
	}

	// Control: a body that merely MENTIONS a source further down is prose,
	// not provenance — only the first line after the metadata is read.
	const prose = "## X\n\nAnchors: none · id `abc124` · 2026-09-03\n\nSee below.\n\nSource: `docs/other.md`\n"
	got, _, _ = ParseShard(ProjectShard, prose)
	if len(got) != 1 || got[0].Source != "" || !strings.Contains(got[0].Body, "Source: `docs/other.md`") {
		t.Errorf("a later Source line was taken as provenance: %+v", got)
	}
}

// The same requirement found in two places in a document is still one fact.
func TestContentKeyIgnoresSource(t *testing.T) {
	a := mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	b := a
	b.Source = "docs/sample-prd.md#L18"
	if a.ContentKey() != b.ContentKey() {
		t.Error("source changed the content key")
	}
}

func TestNormalizeSource(t *testing.T) {
	base := t.TempDir()
	writeDoc(t, base, "docs/sample-prd.md")

	for _, tc := range []struct{ in, want string }{
		{filepath.Join(base, "docs", "sample-prd.md"), "docs/sample-prd.md"},
		{filepath.Join(base, "docs", "sample-prd.md") + "#L18-21", "docs/sample-prd.md#L18-21"},
		{"docs/sample-prd.md#Goals", "docs/sample-prd.md#Goals"}, // relative to base
	} {
		got, err := NormalizeSource(base, tc.in)
		if err != nil || got != tc.want {
			t.Errorf("NormalizeSource(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{
		filepath.Join(base, "docs", "missing.md"),             // provenance nobody has
		filepath.Join(filepath.Dir(base), "outside.md"),       // not in the repository
		filepath.Join(base, "docs", "sample-prd.md") + "#a`b", // would break the line
		"",
	} {
		if bad == filepath.Join(filepath.Dir(base), "outside.md") {
			writeDoc(t, filepath.Dir(base), "outside.md")
		}
		if got, err := NormalizeSource(base, bad); err == nil {
			t.Errorf("NormalizeSource(%q) = %q, want an error", bad, got)
		}
	}
}

// A source file that has gone is a reference to nothing — the same defect as an
// anchor that names nothing, and it fails the check the same way.
func TestCheckFailsWhenASourceFileIsGone(t *testing.T) {
	base := t.TempDir()
	s := NewStore(base)
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	writeDoc(t, base, "docs/sample-prd.md")
	req := mustRequirement(t, "A planner can create a visit", "01-scheduling", "@Scheduling.Visit")
	req.Source = "docs/sample-prd.md#L18-21"
	dec := mustEntry(t, "Visits are committed by the planner", "@Scheduling.Visit")
	dec.Source = "docs/sample-prd.md#L30"
	promoteAll(t, s, req)
	if err := s.Promote(dec, "Scheduling"); err != nil {
		t.Fatal(err)
	}
	r := stubResolver{"@Scheduling.Visit": {State: Resolved, Module: "Scheduling"}}
	shards := []string{"Scheduling", req.Shard()}

	rep, err := Check(s, r, shards)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() || len(rep.MissingSources) != 0 {
		t.Fatalf("control: the source exists, so nothing fails: %+v", rep)
	}

	if err := os.Remove(filepath.Join(base, "docs", "sample-prd.md")); err != nil {
		t.Fatal(err)
	}
	rep, err = Check(s, r, shards)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Failed() || len(rep.MissingSources) != 2 {
		t.Errorf("a missing source must fail, once per entry: %+v", rep.MissingSources)
	}
}
