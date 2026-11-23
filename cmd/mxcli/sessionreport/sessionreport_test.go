// SPDX-License-Identifier: Apache-2.0

package sessionreport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fixture builds a hand-made transcript in Claude Code's JSON Lines shape:
// one record per content block, the message id and usage repeated on each.
type fixture struct {
	lines []string
	t     time.Time
	n     int
}

func newFixture() *fixture {
	return &fixture{t: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
}

func (f *fixture) add(rec map[string]any) {
	f.t = f.t.Add(10 * time.Second)
	rec["timestamp"] = f.t.Format(time.RFC3339Nano)
	rec["sessionId"] = "11111111-2222-3333-4444-555555555555"
	if _, ok := rec["isSidechain"]; !ok {
		rec["isSidechain"] = false
	}
	b, _ := json.Marshal(rec)
	f.lines = append(f.lines, string(b))
}

// model writes one model call that issues the given tool calls. Each block is
// its own record, as Claude Code writes them.
func (f *fixture) model(cacheRead int, uses ...map[string]any) {
	f.n++
	id := fmt.Sprintf("msg_%d", f.n)
	usage := map[string]any{"input_tokens": 1, "cache_read_input_tokens": cacheRead, "cache_creation_input_tokens": 100, "output_tokens": 10}
	blocks := []map[string]any{{"type": "thinking", "thinking": "SECRET_THOUGHT"}}
	blocks = append(blocks, uses...)
	for _, b := range blocks {
		f.add(map[string]any{"type": "assistant", "message": map[string]any{
			"id": id, "role": "assistant", "content": []any{b}, "usage": usage,
		}})
	}
}

func use(id, tool string, input map[string]any) map[string]any {
	return map[string]any{"type": "tool_use", "id": id, "name": tool, "input": input}
}

func bash(id, cmd string) map[string]any { return use(id, "Bash", map[string]any{"command": cmd}) }

func (f *fixture) result(id string, content any, isErr bool) {
	f.add(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "tool_result", "tool_use_id": id, "content": content, "is_error": isErr},
	}}})
}

func (f *fixture) compact() {
	f.add(map[string]any{"type": "system", "subtype": "compact_boundary"})
}

func (f *fixture) session(t *testing.T) *Session {
	t.Helper()
	s, err := Parse(strings.NewReader(strings.Join(f.lines, "\n")+"\n"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func analyze(t *testing.T, f *fixture) Report {
	t.Helper()
	return Analyze([]*Session{f.session(t)}, Options{})
}

func TestModelCallIsAMessageNotARecord(t *testing.T) {
	f := newFixture()
	f.model(1000, bash("t1", "ls"), bash("t2", "pwd")) // 3 records, 1 call
	f.result("t1", "a", false)
	f.result("t2", "b", false)
	f.model(2000)
	s := f.session(t)
	c := s.Conversations[0]
	if c.ModelCalls != 2 {
		t.Fatalf("model calls = %d, want 2 (records repeat the message id)", c.ModelCalls)
	}
	if c.Usage.CacheRead != 3000 {
		t.Fatalf("cache read = %d, want 3000 (usage counted once per message)", c.Usage.CacheRead)
	}
	if len(c.Calls) != 2 || c.Calls[0].Turn != 0 || c.Calls[1].Turn != 0 {
		t.Fatalf("calls = %+v", c.Calls)
	}
}

func TestReReadCostCountsLaterCallsUntilCompaction(t *testing.T) {
	f := newFixture()
	f.model(0, bash("big", "cat big.txt"))
	f.result("big", strings.Repeat("x", 4000), false) // 1000 tokens
	f.model(0, bash("small", "echo hi"))
	f.result("small", strings.Repeat("y", 400), false) // 100 tokens
	f.model(0)
	f.model(0)
	f.compact()
	f.model(0) // after compaction: re-reads neither
	r := analyze(t, f)
	if r.ModelCalls != 5 {
		t.Fatalf("model calls = %d", r.ModelCalls)
	}
	// big: issued at call 0, re-read by calls 1..3 (call 4 is past the boundary).
	if r.Top[0].What != "cat big.txt" || r.Top[0].Readers != 3 || r.Top[0].Cost != 3000 {
		t.Fatalf("top = %+v", r.Top[0])
	}
	if r.Top[1].Readers != 2 || r.Top[1].Cost != 200 {
		t.Fatalf("second = %+v", r.Top[1])
	}
	if r.Compactions != 1 {
		t.Fatalf("compactions = %d", r.Compactions)
	}
}

func TestImageResultsCountByPNGSize(t *testing.T) {
	// A 100x75 PNG header: 100*75/750 = 10 tokens.
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 100, 0, 0, 0, 75, 8, 6, 0, 0, 0}
	if got := imageTokens(b64(png)); got != 10 {
		t.Fatalf("png tokens = %d, want 10", got)
	}
	if got := imageTokens("not-an-image"); got != imageTokenCap {
		t.Fatalf("unknown image = %d, want the cap", got)
	}
}

func TestSplitShell(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`cd /a && ./bin/mxcli exec x.mdl -p app.mpr 2>&1 | tail -5`, []string{"cd /a", "./bin/mxcli exec x.mdl -p app.mpr 2>&1", "tail -5"}},
		{`mxcli -c "show entities; describe entity A.B"`, []string{`mxcli -c "show entities; describe entity A.B"`}},
		{"cat > s.mdl <<'EOF'\ncreate entity A.B;\nrm -rf /\nEOF\nmxcli exec s.mdl", []string{"cat > s.mdl <<'EOF'", "mxcli exec s.mdl"}},
		{`a; b || c & d`, []string{"a", "b", "c", "d"}},
		{"ls # a comment; not a command", []string{"ls"}},
	}
	for _, c := range cases {
		if got := splitShell(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitShell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseBashMxcliVerbs(t *testing.T) {
	segs := ParseBash(`cd app && MXCLI_QUIET=1 ./bin/mxcli exec a.mdl -p app.mpr && mxcli docker check -p app.mpr && mxcli -p app.mpr -c "describe entity M.E"`, nil)
	var verbs []string
	for _, s := range segs {
		if s.Bucket == BucketMxcli {
			verbs = append(verbs, s.Verb)
		}
	}
	want := []string{"exec", "docker check", "-c describe"}
	if !reflect.DeepEqual(verbs, want) {
		t.Fatalf("verbs = %q, want %q", verbs, want)
	}
	if segs[1].Scripts[0] != "a.mdl" {
		t.Fatalf("scripts = %q", segs[1].Scripts)
	}
}

func TestCategories(t *testing.T) {
	cases := []struct {
		tool  string
		input map[string]any
		want  string
	}{
		{"Bash", map[string]any{"command": "mxcli exec a.mdl -p app.mpr && mxcli docker check -p app.mpr"}, CatApply},
		{"Bash", map[string]any{"command": "mxcli check a.mdl -p app.mpr 2>&1 | tail -20"}, CatValidate},
		{"Bash", map[string]any{"command": "mxcli lint -p app.mpr"}, CatValidate},
		{"Bash", map[string]any{"command": "mxcli syntax page"}, CatOrientation},
		{"Bash", map[string]any{"command": `mxcli -p app.mpr -c "describe page M.P"`}, CatOrientation},
		{"Bash", map[string]any{"command": `mxcli -p app.mpr -c "create module M"`}, CatApply},
		{"Bash", map[string]any{"command": "mxcli test tests/ -p app.mpr --local"}, CatVerify},
		{"Bash", map[string]any{"command": "mxcli playwright verify flows.yaml"}, CatVerify},
		{"Bash", map[string]any{"command": "playwright-cli screenshot --filename x.png"}, CatVerify},
		{"Bash", map[string]any{"command": "cat > a.mdl <<'EOF'\ncreate entity A.B;\nEOF"}, CatWrite},
		{"Bash", map[string]any{"command": "tail -50 deployment/log/m2ee.log | grep ERROR"}, CatDiagnosis},
		{"Bash", map[string]any{"command": "grep -rn Order src/"}, CatOrientation},
		{"Bash", map[string]any{"command": "git status"}, CatOther},
		{"Read", map[string]any{"file_path": "/p/.claude/skills/mendix/write-microflows/SKILL.md"}, CatOrientation},
		{"Read", map[string]any{"file_path": "/home/u/.mxcli/logs/mxcli-2026-10-01.log"}, CatDiagnosis},
		{"Edit", map[string]any{"file_path": "/p/a.mdl"}, CatWrite},
		{"Write", map[string]any{"file_path": "/p/a.mdl"}, CatWrite},
		{"Skill", map[string]any{"skill": "mendix:create-crud"}, CatOrientation},
		{"Agent", map[string]any{"description": "probe"}, CatDelegate},
		{"mcp__playwright__browser_take_screenshot", map[string]any{}, CatVerify},
	}
	for _, c := range cases {
		call := &Call{Tool: c.tool}
		raw, _ := json.Marshal(c.input)
		fillInput(call, raw)
		if got, _ := categorize(call, nil); got != c.want {
			t.Errorf("%s %v: category %s, want %s", c.tool, c.input, got, c.want)
		}
	}
}

func TestDetectFailure(t *testing.T) {
	cases := []struct {
		tool    string
		isErr   bool
		text    string
		failed  bool
		errLine string
	}{
		{"Bash", true, "Exit code 1\nWARNING: This is a vibe-coded PoC\nParse error: line 3:5 extraneous input '.'", true, "Parse error: line 3:5 extraneous input '.'"},
		{"Bash", false, "checking...\nError: entity not found: Shop.Ordr\ndone", true, "Error: entity not found: Shop.Ordr"},
		{"Bash", false, "  - line 4:2 mismatched input ':' expecting '}'", true, "- line 4:2 mismatched input ':' expecting '}'"},
		{"Bash", false, "x.go:12: Error: not a failure, a grep hit", false, ""},
		{"Bash", true, "Exit code 2", true, "Exit code 2"},
		{"Read", false, "Error: this is file content", false, ""},
		{"Edit", true, "<tool_use_error>String to replace not found in file.</tool_use_error>", true, "String to replace not found in file."},
	}
	for _, c := range cases {
		failed, line := detectFailure(c.tool, "", c.isErr, c.text)
		if failed != c.failed || line != c.errLine {
			t.Errorf("detectFailure(%s, %v, %q) = %v %q, want %v %q", c.tool, c.isErr, c.text, failed, line, c.failed, c.errLine)
		}
	}
}

func TestRetryChain(t *testing.T) {
	f := newFixture()
	f.model(0, bash("w", "cat > app.mdl <<'EOF'\ncreate entity A.B (Name: String);\nEOF"))
	f.result("w", "", false)
	f.model(0, bash("e1", "mxcli exec app.mdl -p app.mpr"))
	f.result("e1", "Exit code 1\nParse error: line 1:20 mismatched input 'Name' expecting {IDENTIFIER}", true)
	f.model(0, use("r", "Read", map[string]any{"file_path": "/p/other.md"})) // unrelated
	f.result("r", "doc", false)
	f.model(0, use("ed", "Edit", map[string]any{"file_path": "/p/app.mdl"}))
	f.result("ed", "ok", false)
	f.model(0, bash("e2", "mxcli exec app.mdl -p app.mpr"))
	f.result("e2", "Exit code 1\nParse error: line 1:9 mismatched input 'Title' expecting {IDENTIFIER}", true)
	f.model(0, use("ed2", "Edit", map[string]any{"file_path": "/p/app.mdl"}))
	f.result("ed2", "ok", false)
	f.model(0, bash("e3", "mxcli exec app.mdl -p app.mpr"))
	f.result("e3", "Created entity A.B", false)
	f.model(0)
	r := analyze(t, f)
	if len(r.Chains) != 1 {
		t.Fatalf("chains = %+v", r.Chains)
	}
	ch := r.Chains[0]
	if ch.Length != 4 || !ch.Resolved || ch.What != "mxcli exec app.mdl -p app.mpr" {
		t.Fatalf("chain = %+v", ch)
	}
	if r.RetryCalls != 4 || r.Failures != 2 {
		t.Fatalf("retry calls %d failures %d", r.RetryCalls, r.Failures)
	}
	fam := r.ErrorFamilies[0]
	if fam.Error != "Parse error: line N:N mismatched input 'Name' expecting {IDENTIFIER}" || fam.Retries != 4 || fam.Failures != 2 {
		t.Fatalf("family = %+v", fam)
	}
	cats := map[string]int{}
	for _, c := range r.ByCategory {
		cats[c.Name] = c.N
	}
	if cats[CatRetry] != 4 || cats[CatApply] != 1 || cats[CatWrite] != 1 || cats[CatOrientation] != 1 {
		t.Fatalf("categories = %+v", r.ByCategory)
	}
}

func TestChainClosesWhenAbandoned(t *testing.T) {
	f := newFixture()
	f.model(0, bash("e1", "mxcli exec app.mdl"))
	f.result("e1", "Exit code 1\nError: boom", true)
	for i := 0; i < chainIdleLimit+1; i++ {
		id := fmt.Sprintf("x%d", i)
		f.model(0, bash(id, fmt.Sprintf("ls dir%d", i)))
		f.result(id, "", false)
	}
	f.model(0, bash("e2", "mxcli exec app.mdl")) // new work, not a retry
	f.result("e2", "ok", false)
	r := analyze(t, f)
	if len(r.Chains) != 0 || r.RetryCalls != 0 || r.Failures != 1 {
		t.Fatalf("chains %+v retry %d failures %d", r.Chains, r.RetryCalls, r.Failures)
	}
}

func TestLookups(t *testing.T) {
	f := newFixture()
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("r%d", i)
		f.model(0, use(id, "Read", map[string]any{"file_path": "/home/u/app/.claude/skills/mendix/write-microflows/SKILL.md"}))
		f.result(id, "skill text", false)
	}
	f.model(0, bash("s1", "./mxcli syntax page 2>&1"), bash("s2", "./mxcli syntax page"), bash("c", "cat CLAUDE.md"))
	f.result("s1", "...", false)
	f.result("s2", "...", false)
	f.result("c", "...", false)
	r := analyze(t, f)
	if r.FileReads != 4 || r.DocLookups != 6 {
		t.Fatalf("reads %d lookups %d", r.FileReads, r.DocLookups)
	}
	want := []Count{{"skill write-microflows", 3}, {"mxcli syntax page", 2}}
	if !reflect.DeepEqual(r.RepeatedLookups, want) {
		t.Fatalf("repeated lookups = %+v", r.RepeatedLookups)
	}
	if len(r.RepeatedReads) != 1 || r.RepeatedReads[0].N != 3 || r.RepeatedReads[0].Name != "…/write-microflows/SKILL.md" {
		t.Fatalf("repeated reads = %+v", r.RepeatedReads)
	}
}

func TestBashBreakdown(t *testing.T) {
	f := newFixture()
	f.model(0,
		bash("a", "mxcli exec a.mdl -p x.mpr && mxcli docker check -p x.mpr"),
		bash("b", "git status"),
		bash("c", "make build"),
		bash("d", "npx playwright test"),
		bash("e", "cd /p && ls"),
	)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		f.result(id, "", false)
	}
	r := analyze(t, f)
	got := map[string]int{}
	for _, c := range r.Bash.ByBucket {
		got[c.Name] = c.N
	}
	want := map[string]int{"mxcli": 1, "git": 1, "build": 1, "playwright": 1, "other": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buckets = %v", got)
	}
	if r.Bash.MxcliInvocations != 2 || r.Bash.ChainedCalls != 1 {
		t.Fatalf("mxcli %d chained %d", r.Bash.MxcliInvocations, r.Bash.ChainedCalls)
	}
}

// The report is meant to be shareable: no prompt, thinking or result text may
// reach it — only counts and short command/path/error snippets.
func TestReportCarriesNoContent(t *testing.T) {
	f := newFixture()
	f.add(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": "SECRET_PROMPT build me an app"}})
	f.model(0, bash("a", "mxcli -p app.mpr -c 'describe entity Shop.Order'"))
	f.result("a", "SECRET_RESULT "+strings.Repeat("z", 5000), false)
	f.model(0, use("w", "Write", map[string]any{"file_path": "/home/alice/projects/acme/app.mdl", "content": "SECRET_FILE_BODY"}))
	f.result("w", "ok", false)
	f.model(0, bash("t", "TOKEN=sk-ant-abcdefghijklmnopqrstuvwxyz0123456789 curl https://api"))
	f.result("t", "Exit code 1\nError: unauthorized SECRET_TAIL", true)
	f.model(0)
	r := analyze(t, f)
	var buf bytes.Buffer
	Render(&buf, r)
	js, _ := json.Marshal(r)
	for _, out := range []string{buf.String(), string(js)} {
		for _, secret := range []string{"SECRET_PROMPT", "SECRET_RESULT", "SECRET_FILE_BODY", "SECRET_THOUGHT", "abcdefghijklmnopqrstuvwxyz0123456789", "/home/alice"} {
			if strings.Contains(out, secret) {
				t.Errorf("report leaks %q:\n%s", secret, out)
			}
		}
	}
	if !strings.Contains(buf.String(), "describe entity Shop.Order") {
		t.Errorf("short command snippet missing:\n%s", buf.String())
	}
}

func TestNormalizeError(t *testing.T) {
	cases := map[string]string{
		"Parse error: line 12:4 extraneous input '.' expecting x":              "Parse error: line N:N extraneous input '.' expecting x",
		"open /home/u/proj/a/b/c.mdl: no such file":                            "open c.mdl: no such file",
		"isolated in the worktree agent-aa2d8f7ec45568 and agent-a1b2c3d4e5f6": "isolated in the worktree agent-# and agent-#",
	}
	for in, want := range cases {
		if got := NormalizeError(in); got != want {
			t.Errorf("NormalizeError(%q) = %q, want %q", in, got, want)
		}
	}
}

// ParseFile reads the subagent transcripts Claude Code writes beside the main
// one, and keeps their cost separate: a subagent's results are re-read by the
// subagent's calls, not the main session's.
func TestParseFileIncludesSubagents(t *testing.T) {
	path := filepath.Join("testdata", "session1.jsonl")
	s, err := ParseFile(path, true)
	if err != nil {
		t.Fatal(err)
	}
	r := Analyze([]*Session{s}, Options{})
	if r.Subagents != 1 || r.ModelCalls != 5 || r.MainModelCalls != 3 {
		t.Fatalf("subagents %d model calls %d main %d", r.Subagents, r.ModelCalls, r.MainModelCalls)
	}
	if r.ToolCalls != 4 {
		t.Fatalf("tool calls = %d", r.ToolCalls)
	}
	s2, _ := ParseFile(path, false)
	if r2 := Analyze([]*Session{s2}, Options{}); r2.Subagents != 0 || r2.ModelCalls != 3 {
		t.Fatalf("without subagents: %d %d", r2.Subagents, r2.ModelCalls)
	}
	// Combined over two copies doubles the counts.
	c := Analyze([]*Session{s, s}, Options{})
	if c.Sessions != 2 || c.ModelCalls != 10 || c.Label != "combined" {
		t.Fatalf("combined = %+v", c)
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
