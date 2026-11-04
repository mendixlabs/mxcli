// SPDX-License-Identifier: Apache-2.0

// shard.go - rendering a shard to Markdown and reading it back.
//
// The format is chosen for the reader, not the parser: a Mendix developer meets
// these files in a pull request diff, so the anchors are visible prose rather
// than metadata hidden in an HTML comment. There is exactly one copy of each
// fact in the file — a visible line that is also the parsed one — because a
// human-readable copy beside a machine-readable copy is two things that drift.
package brain

import (
	"fmt"
	"regexp"
	"strings"
)

// shardMarker identifies a file mxcli wrote. `brain init` refuses a docs/brain/
// whose README does not carry it, so an existing folder of someone else's notes
// is never adopted by accident.
const shardMarker = "<!-- mxcli-brain -->"

// metaLine matches an entry's one metadata line. The separator is a middle dot
// so that a title or body containing a hyphen cannot be mistaken for it.
var metaLine = regexp.MustCompile("^Anchors: (.*?) · id `([0-9a-f]{6})` · (\\d{4}-\\d{2}-\\d{2})( · OPEN)?\\s*$")

// anchorRef matches one backticked anchor inside the metadata line.
var anchorRef = regexp.MustCompile("`(@[A-Za-z_][A-Za-z0-9_.]*)`")

// RenderShard writes a whole shard. Entries are emitted in the order given;
// promotion appends, so that is chronological, and a diff shows one added block.
func RenderShard(shard string, entries []Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", shardTitle(shard))
	b.WriteString(shardMarker)
	b.WriteString("\n\n")
	b.WriteString(shardPreamble(shard))
	for _, e := range entries {
		b.WriteString("\n")
		b.WriteString(renderEntry(e))
	}
	return b.String()
}

func renderEntry(e Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", e.Title)
	open := ""
	if e.Open {
		open = " · OPEN"
	}
	fmt.Fprintf(&b, "Anchors: %s · id `%s` · %s%s\n", renderAnchors(e.Anchors), e.ID, e.Date, open)
	if e.Source != "" {
		// Its own line, directly after the metadata line (ako/mxcli#1951).
		// Not ON that line: older builds match it with an exact regex and
		// would report every entry carrying a source as malformed. Not
		// before it: older builds discard lines between title and metadata
		// on their next rewrite. Here, an older build reads it as the first
		// line of the body and keeps it.
		fmt.Fprintf(&b, "Source: `%s`\n", e.Source)
	}
	if e.Body != "" {
		fmt.Fprintf(&b, "\n%s\n", e.Body)
	}
	return b.String()
}

func renderAnchors(anchors []string) string {
	if len(anchors) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(anchors))
	for _, a := range anchors {
		if !strings.HasPrefix(a, "@") {
			a = "@" + a
		}
		parts = append(parts, "`"+a+"`")
	}
	return strings.Join(parts, ", ")
}

// ParseShard reads entries back out of a rendered shard. A block that does not
// carry a well-formed metadata line is *reported*, not skipped: silently
// dropping it would make `check` claim a clean shard while an entry sits in it
// unchecked.
//
// The kind is taken from the shard, never from the entry's own text. A
// requirement is a requirement because it lives under plan/, so there is no
// second copy of that fact in the file to drift from the first.
func ParseShard(shard, content string) (entries []Entry, malformed []string, err error) {
	for _, blk := range splitEntries(content) {
		e, ok := parseEntry(blk)
		if !ok {
			malformed = append(malformed, firstLine(blk))
			continue
		}
		if IsPlanShard(shard) {
			e.Kind, e.Slice = KindRequirement, SliceOf(shard)
		}
		entries = append(entries, e)
	}
	return entries, malformed, nil
}

func splitEntries(content string) []string {
	lines := strings.Split(content, "\n")
	var blocks []string
	var cur []string
	in := false
	for _, ln := range lines {
		if strings.HasPrefix(ln, "## ") {
			if in {
				blocks = append(blocks, strings.Join(cur, "\n"))
			}
			in, cur = true, []string{ln}
			continue
		}
		if in {
			cur = append(cur, ln)
		}
	}
	if in {
		blocks = append(blocks, strings.Join(cur, "\n"))
	}
	return blocks
}

func parseEntry(block string) (Entry, bool) {
	lines := strings.Split(block, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "## ") {
		return Entry{}, false
	}
	e := Entry{Title: strings.TrimSpace(strings.TrimPrefix(lines[0], "## "))}
	metaAt := -1
	for i := 1; i < len(lines); i++ {
		if m := metaLine.FindStringSubmatch(strings.TrimSpace(lines[i])); m != nil {
			for _, a := range anchorRef.FindAllStringSubmatch(m[1], -1) {
				e.Anchors = append(e.Anchors, a[1])
			}
			e.ID, e.Date = m[2], m[3]
			e.Open = m[4] != ""
			metaAt = i
			break
		}
	}
	if metaAt < 0 {
		return Entry{}, false
	}
	rest := lines[metaAt+1:]
	// The first non-blank line after the metadata may be the source. An
	// older build that rewrote the shard put a blank line before it, having
	// taken it for body text, so blank lines are skipped — but only the FIRST
	// line is read: a body that mentions a source further down is prose.
	for i, ln := range rest {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if m := sourceLine.FindStringSubmatch(strings.TrimSpace(ln)); m != nil {
			e.Source = m[1]
			rest = rest[i+1:]
		}
		break
	}
	e.Body = strings.TrimSpace(strings.Join(rest, "\n"))
	return e, true
}

// sourceLine matches an entry's provenance line.
var sourceLine = regexp.MustCompile("^Source: `([^`]+)`$")

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func shardTitle(shard string) string {
	switch {
	case shard == ProjectShard:
		return "Project"
	case IsPlanShard(shard):
		return "Slice: " + SliceOf(shard)
	default:
		return shard
	}
}

func shardPreamble(shard string) string {
	if IsPlanShard(shard) {
		return "Requirements for this slice. Anchors point FORWARD, at what the slice\n" +
			"will build — an anchor that does not resolve yet means not built, not\n" +
			"stale. `mxcli brain plan` counts them against the model.\n"
	}
	if shard == ProjectShard {
		return "Decisions that are not about one module. This file is loaded every\n" +
			"session, so it carries the tightest cap — see `mxcli brain show`.\n"
	}
	return fmt.Sprintf("Decisions anchored to the %s module. Loaded when %s is in play,\n"+
		"not otherwise.\n", shard, shard)
}

// HasMarker reports whether a file was written by mxcli.
func HasMarker(content string) bool { return strings.Contains(content, shardMarker) }

// CountLines is the size measure caps are expressed in, because lines are what
// an agent pays for when the shard is loaded into context.
func CountLines(content string) int {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

// extractFrontmatter returns the YAML frontmatter block at the top of a shard,
// fences included, or "" when there is none.
//
// A shard is re-rendered from the entries parsed out of it, so everything else
// in the file is discarded on the next write. That is deliberate for the parts
// mxcli owns — the title and the preamble are regenerated so they cannot drift
// from the shard's identity — but it also ate frontmatter, which mxcli does not
// own and which is where every markdown tool in the ecosystem keeps its
// per-file metadata: Foam and Obsidian tags, a docs site's nav weight, a
// linter's per-file config.
//
// The recognition is deliberately strict, because the failure of a loose rule
// is not a missed block but a swallowed document. Only an opening fence on the
// very first line, closed by a later fence, counts. An unterminated `---` is
// left alone: treating it as frontmatter would carry the entire file forward as
// opaque text and then write the entries out again beneath it.
func extractFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}
	rest := content[len("---\n"):]
	// The closing fence is a line of exactly "---". Scanning line by line
	// rather than with an index search so a "---" inside a YAML value cannot
	// close the block early.
	offset := len("---\n")
	for len(rest) > 0 {
		line, tail, found := strings.Cut(rest, "\n")
		if !found {
			return "" // ran off the end: no closing fence, so not frontmatter
		}
		offset += len(line) + 1
		if strings.TrimRight(line, " \t") == "---" {
			return content[:offset]
		}
		rest = tail
	}
	return ""
}

// RenderShardWithFrontmatter renders a shard beneath a preserved frontmatter
// block. An empty block renders exactly as before, so a shard nobody has
// annotated is byte-identical and existing projects see no diff.
func RenderShardWithFrontmatter(shard string, entries []Entry, frontmatter string) string {
	if frontmatter == "" {
		return RenderShard(shard, entries)
	}
	if !strings.HasSuffix(frontmatter, "\n") {
		frontmatter += "\n"
	}
	return frontmatter + "\n" + RenderShard(shard, entries)
}
