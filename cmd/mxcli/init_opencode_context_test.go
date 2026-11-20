// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1822 (mendixlabs/mxcli#1683): `init --tool opencode` wrote an
// opencode.json whose `instructions` globbed both skill trees. OpenCode
// inlines every instruction file's body into the system prompt, so a fresh
// session started with 150 files, 2.7 MB — about 73% of a 1M window — before
// the user typed anything. OpenCode discovers .opencode/skills/ by itself and
// loads a skill's body on demand; the instructions are AGENTS.md alone.
func TestOpenCodeConfig_InstructionsAreAgentsMdOnly(t *testing.T) {
	var cfg struct {
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(generateOpenCodeConfig("App", "App.mpr")), &cfg); err != nil {
		t.Fatalf("opencode.json is not JSON: %v", err)
	}
	if len(cfg.Instructions) != 1 || cfg.Instructions[0] != "AGENTS.md" {
		t.Errorf("instructions = %v, want only AGENTS.md: a skill glob inlines every skill body into every prompt", cfg.Instructions)
	}
}

// The same report's second symptom: the .opencode/skills/ copies were given a
// frontmatter of their own, described by the skill's first heading
// ("Agents"), in front of the skill's own frontmatter. OpenCode reads the
// first, so the "Use when…" description that decides whether a skill is
// invoked was replaced by a title. A skill that has frontmatter keeps its
// description, in one frontmatter block.
func TestWrapSkillContent_KeepsTheSkillsOwnDescription(t *testing.T) {
	src := "---\nname: agents\ndescription: \"Author agents in MDL. Use when adding GenAI features.\"\n---\n# Agents\n\nbody\n"
	out := string(wrapSkillContent("agents", []byte(src)))
	if n := strings.Count(out, "\n---\n") + boolInt(strings.HasPrefix(out, "---\n")); n != 2 {
		t.Errorf("want exactly one frontmatter block (2 delimiters), got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "Use when adding GenAI features.") {
		t.Errorf("the skill's own description was lost:\n%s", out)
	}
	if !strings.Contains(out, "compatibility: opencode") || !strings.Contains(out, "# Agents\n\nbody\n") {
		t.Errorf("want the opencode marker and the body:\n%s", out)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// End to end on the shipped skills: every .opencode/skills/ copy carries the
// description of the skill it copies.
func TestInitOpenCode_SkillsKeepTheirDescriptions(t *testing.T) {
	dir := t.TempDir()
	runInit(t, []string{"opencode"}, dir)
	body, err := os.ReadFile(filepath.Join(dir, ".opencode", "skills", "agents", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(strings.TrimPrefix(string(body), "---\n"), "\n---\n")
	if !strings.Contains(head, "Use when") {
		t.Errorf(".opencode/skills/agents/SKILL.md frontmatter lacks the skill's \"Use when\" description:\n%s", head)
	}
}
