// SPDX-License-Identifier: Apache-2.0

// milestone.go - which milestone of the specification a slice belongs to.
//
// Milestones lived in a separate docs/brain/plan/traceability.json, the one
// input the traceability report could not derive. It was a second file about
// the same slices, which nothing kept in step with them: rename a slice and the
// file named one that no longer existed. A milestone is now part of the slice:
//
//   - a key in the slice file's frontmatter, which the store already carries
//     across every rewrite —
//
//     ---
//     milestone: "M1 — accounts"
//     ---
//
//   - and, for a requirement placed apart from its slice, a line in its body:
//
//     Milestone: `M2 — approvals`
//
// Milestones are ordered by where they first appear, walking the slices in name
// order — the same order `brain plan` sequences a roadmap in.
package brain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Milestone is one milestone and what it holds.
type Milestone struct {
	Name         string   `json:"name"`
	Slices       []string `json:"slices,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

var (
	frontmatterMilestone = regexp.MustCompile(`(?m)^milestone:[ \t]*(.*?)[ \t]*$`)
	bodyMilestone        = regexp.MustCompile("(?im)^\\s*milestone:\\s*`([^`]+)`\\s*$")
)

// milestoneOfFrontmatter reads the milestone key out of a frontmatter block.
func milestoneOfFrontmatter(fm string) string {
	m := frontmatterMilestone.FindStringSubmatch(fm)
	if m == nil {
		return ""
	}
	v := m[1]
	switch {
	case strings.HasPrefix(v, `"`):
		var s string
		if json.Unmarshal([]byte(v), &s) == nil {
			return strings.TrimSpace(s)
		}
	case strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") && len(v) >= 2:
		return strings.TrimSpace(strings.ReplaceAll(v[1:len(v)-1], "''", "'"))
	}
	return strings.TrimSpace(v)
}

// withFrontmatterMilestone sets (or, with name "", removes) the milestone key
// in a frontmatter block, leaving every other line as it was. A block that
// ends up empty is dropped.
func withFrontmatterMilestone(fm, name string) string {
	line := "milestone: " + yamlQuote(name)
	if fm == "" {
		if name == "" {
			return ""
		}
		return "---\n" + line + "\n---\n"
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(fm, "---\n"), "---\n")
	inner = strings.TrimSuffix(inner, "---")
	if frontmatterMilestone.MatchString(inner) {
		if name == "" {
			inner = frontmatterMilestone.ReplaceAllString(inner, "")
			inner = strings.ReplaceAll(inner, "\n\n", "\n")
			inner = strings.TrimLeft(inner, "\n")
		} else {
			inner = frontmatterMilestone.ReplaceAllLiteralString(inner, line)
		}
	} else if name != "" {
		if inner != "" && !strings.HasSuffix(inner, "\n") {
			inner += "\n"
		}
		inner += line + "\n"
	}
	if strings.TrimSpace(inner) == "" {
		return ""
	}
	if !strings.HasSuffix(inner, "\n") {
		inner += "\n"
	}
	return "---\n" + inner + "---\n"
}

// yamlQuote writes a JSON string, which is a valid YAML double-quoted scalar,
// without escaping & < > the way json.Marshal does for HTML.
func yamlQuote(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// RequirementMilestone is the milestone a requirement's own body places it in,
// or "".
func RequirementMilestone(e Entry) string {
	if m := bodyMilestone.FindStringSubmatch(e.Body); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// withRequirementMilestone sets (or with "" removes) a requirement's milestone
// line.
func withRequirementMilestone(e Entry, name string) Entry {
	body := strings.TrimSpace(bodyMilestone.ReplaceAllString(e.Body, ""))
	body = regexp.MustCompile(`\n{3,}`).ReplaceAllString(body, "\n\n")
	if name != "" {
		line := "Milestone: `" + name + "`"
		if body == "" {
			body = line
		} else {
			body += "\n\n" + line
		}
	}
	e.Body = body
	return e
}

// SliceMilestone is the milestone a slice's frontmatter names, or "".
func (s *Store) SliceMilestone(slice string) string {
	return milestoneOfFrontmatter(s.Frontmatter(PlanShard(slice)))
}

// SetSliceMilestone puts a slice in a milestone; name "" takes it out. The
// slice must exist: a slice file is only written with requirements in it.
func (s *Store) SetSliceMilestone(slice, name string) error {
	if strings.ContainsAny(name, "\n`") {
		return fmt.Errorf("a milestone name is one line without backticks: %q", name)
	}
	shard := PlanShard(slice)
	entries, _, err := s.LoadShard(shard)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("no slice %q: capture a requirement in it first ('mxcli brain capture … --slice %s')", slice, slice)
	}
	return s.saveShardWith(shard, entries, withFrontmatterMilestone(s.Frontmatter(shard), strings.TrimSpace(name)))
}

// SetRequirementMilestone places one requirement in a milestone apart from its
// slice; name "" puts it back with its slice.
func (s *Store) SetRequirementMilestone(id, name string) error {
	if strings.ContainsAny(name, "\n`") {
		return fmt.Errorf("a milestone name is one line without backticks: %q", name)
	}
	e, shard, err := s.Find(id)
	if err != nil {
		return err
	}
	if shard == "" {
		return fmt.Errorf("no entry %s", id)
	}
	if e.EntryKind() != KindRequirement || e.Open {
		return fmt.Errorf("%s is not a requirement: only requirements belong to a milestone", id)
	}
	return s.Replace(shard, withRequirementMilestone(e, strings.TrimSpace(name)))
}

// Milestones reads every milestone from the plan, in order of first
// appearance. A requirement whose body names a milestone is listed under
// Requirements of that milestone and not counted with its slice's. Slices in
// no milestone are not listed; nil means the plan names no milestone at all.
func (s *Store) Milestones() ([]Milestone, error) {
	slices, err := s.ListSlices()
	if err != nil {
		return nil, err
	}
	var out []Milestone
	at := map[string]int{}
	get := func(name string) *Milestone {
		i, ok := at[name]
		if !ok {
			out = append(out, Milestone{Name: name})
			i = len(out) - 1
			at[name] = i
		}
		return &out[i]
	}
	for _, sh := range slices {
		slice := SliceOf(sh)
		if name := s.SliceMilestone(slice); name != "" {
			m := get(name)
			m.Slices = append(m.Slices, slice)
		}
		entries, _, err := s.LoadShard(sh)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Open {
				continue
			}
			if name := RequirementMilestone(e); name != "" {
				m := get(name)
				m.Requirements = append(m.Requirements, e.ID)
			}
		}
	}
	return out, nil
}

// ImportMilestones writes a traceability.json's milestones into the plan: each
// named slice gets the milestone in its frontmatter, each named requirement a
// milestone line. It returns what it could not place (a slice or requirement
// the plan does not have), and changes nothing when the file names a slice
// twice, which the new form cannot express.
func (s *Store) ImportMilestones(ms []Milestone) (skipped []string, err error) {
	seen := map[string]string{}
	for _, m := range ms {
		for _, sl := range m.Slices {
			if prev, ok := seen[sl]; ok && prev != m.Name {
				return nil, fmt.Errorf("slice %q is in two milestones (%q and %q); a slice belongs to one", sl, prev, m.Name)
			}
			seen[sl] = m.Name
		}
	}
	for _, m := range ms {
		for _, sl := range m.Slices {
			if err := s.SetSliceMilestone(sl, m.Name); err != nil {
				skipped = append(skipped, fmt.Sprintf("milestone %q: %v", m.Name, err))
			}
		}
		for _, id := range m.Requirements {
			if err := s.SetRequirementMilestone(id, m.Name); err != nil {
				skipped = append(skipped, fmt.Sprintf("milestone %q: %v", m.Name, err))
			}
		}
	}
	return skipped, nil
}
