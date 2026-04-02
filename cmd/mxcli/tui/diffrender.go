package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// RenderPlainUnifiedDiff generates a standard unified diff string (no ANSI colors).
// This format is directly understood by LLMs and tools like patch/git.
func RenderPlainUnifiedDiff(result *DiffResult, oldTitle, newTitle string) string {
	if result == nil || len(result.Lines) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n", oldTitle))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", newTitle))

	// Generate hunks with context
	lines := result.Lines
	total := len(lines)
	const contextLines = 3

	// Find hunk boundaries: groups of changes with context
	type hunkRange struct{ start, end int }
	var hunks []hunkRange

	i := 0
	for i < total {
		// Skip equal lines until we find a change
		if lines[i].Type == DiffEqual {
			i++
			continue
		}
		// Found a change — expand to include context
		start := max(0, i-contextLines)
		// Find end of this change group (including bridged gaps)
		for i < total {
			if lines[i].Type != DiffEqual {
				i++
				continue
			}
			// Count consecutive equal lines
			eqStart := i
			for i < total && lines[i].Type == DiffEqual {
				i++
			}
			eqCount := i - eqStart
			if i >= total || eqCount > contextLines*2 {
				// Gap too large or end of file — close hunk
				end := min(total, eqStart+contextLines)
				hunks = append(hunks, hunkRange{start, end})
				break
			}
			// Small gap — bridge and continue
		}
		if len(hunks) == 0 || hunks[len(hunks)-1].end < i {
			end := min(total, i+contextLines)
			hunks = append(hunks, hunkRange{start, end})
		}
	}

	// If no hunks (all equal), nothing to output
	if len(hunks) == 0 {
		return sb.String() + "@@ no differences @@\n"
	}

	for _, h := range hunks {
		// Count old/new lines in this hunk
		oldStart, newStart := 0, 0
		oldCount, newCount := 0, 0
		for j := h.start; j < h.end; j++ {
			dl := lines[j]
			if j == h.start {
				oldStart = max(1, dl.OldLineNo)
				newStart = max(1, dl.NewLineNo)
				if dl.Type == DiffInsert {
					oldStart = max(1, dl.NewLineNo) // approximate
				}
				if dl.Type == DiffDelete {
					newStart = max(1, dl.OldLineNo)
				}
			}
			switch dl.Type {
			case DiffEqual:
				oldCount++
				newCount++
			case DiffDelete:
				oldCount++
			case DiffInsert:
				newCount++
			}
		}

		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))
		for j := h.start; j < h.end; j++ {
			dl := lines[j]
			switch dl.Type {
			case DiffEqual:
				sb.WriteString(" " + dl.Content + "\n")
			case DiffDelete:
				sb.WriteString("-" + dl.Content + "\n")
			case DiffInsert:
				sb.WriteString("+" + dl.Content + "\n")
			}
		}
	}

	return sb.String()
}

// DiffRenderedLine holds the sticky prefix (gutter + line numbers) and scrollable content separately.
type DiffRenderedLine struct {
	Prefix  string // gutter char + line numbers (sticky, never scrolled)
	Content string // actual code/text content (horizontally scrollable)
}

// RenderUnifiedDiff renders a DiffResult as unified diff lines with prefix/content split.
func RenderUnifiedDiff(result *DiffResult, lang string) []DiffRenderedLine {
	if result == nil || len(result.Lines) == 0 {
		return nil
	}

	gutterCharSt := lipgloss.NewStyle()
	lineNoSt := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	maxLineNo := 0
	for _, dl := range result.Lines {
		if dl.OldLineNo > maxLineNo {
			maxLineNo = dl.OldLineNo
		}
		if dl.NewLineNo > maxLineNo {
			maxLineNo = dl.NewLineNo
		}
	}
	lineNoW := max(3, len(fmt.Sprintf("%d", maxLineNo)))

	rendered := make([]DiffRenderedLine, 0, len(result.Lines))
	for _, dl := range result.Lines {
		var gutter, oldNo, newNo, content string

		switch dl.Type {
		case DiffEqual:
			gutter = gutterCharSt.Foreground(DiffEqualGutter).Render("│")
			oldNo = lineNoSt.Render(fmt.Sprintf("%*d", lineNoW, dl.OldLineNo))
			newNo = lineNoSt.Render(fmt.Sprintf("%*d", lineNoW, dl.NewLineNo))
			content = highlightLine(dl.Content, lang)

		case DiffInsert:
			gutter = gutterCharSt.Foreground(DiffGutterAddedFg).Render("+")
			oldNo = lineNoSt.Render(strings.Repeat(" ", lineNoW))
			newNo = lipgloss.NewStyle().Foreground(DiffGutterAddedFg).Render(fmt.Sprintf("%*d", lineNoW, dl.NewLineNo))
			content = renderSegments(dl.Segments, DiffInsert)

		case DiffDelete:
			gutter = gutterCharSt.Foreground(DiffGutterRemovedFg).Render("-")
			oldNo = lipgloss.NewStyle().Foreground(DiffGutterRemovedFg).Render(fmt.Sprintf("%*d", lineNoW, dl.OldLineNo))
			newNo = lineNoSt.Render(strings.Repeat(" ", lineNoW))
			content = renderSegments(dl.Segments, DiffDelete)
		}

		prefix := gutter + " " + oldNo + " " + newNo + " "
		rendered = append(rendered, DiffRenderedLine{Prefix: prefix, Content: content})
	}
	return rendered
}

// SideBySideRenderedLine holds prefix and content for one pane in side-by-side view.
type SideBySideRenderedLine struct {
	Prefix  string // line number (sticky)
	Content string // code content (scrollable)
	Blank   bool   // true if this is a blank filler line
}

// RenderSideBySideDiff renders a DiffResult as two columns with prefix/content split.
func RenderSideBySideDiff(result *DiffResult, lang string) (left, right []SideBySideRenderedLine) {
	if result == nil || len(result.Lines) == 0 {
		return nil, nil
	}

	lineNoSt := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	maxLineNo := 0
	for _, dl := range result.Lines {
		if dl.OldLineNo > maxLineNo {
			maxLineNo = dl.OldLineNo
		}
		if dl.NewLineNo > maxLineNo {
			maxLineNo = dl.NewLineNo
		}
	}
	lineNoW := max(3, len(fmt.Sprintf("%d", maxLineNo)))
	blankPrefix := strings.Repeat(" ", lineNoW) + " "

	for _, dl := range result.Lines {
		switch dl.Type {
		case DiffEqual:
			highlighted := highlightLine(dl.Content, lang)
			oldNo := lineNoSt.Render(fmt.Sprintf("%*d", lineNoW, dl.OldLineNo)) + " "
			newNo := lineNoSt.Render(fmt.Sprintf("%*d", lineNoW, dl.NewLineNo)) + " "
			left = append(left, SideBySideRenderedLine{Prefix: oldNo, Content: highlighted})
			right = append(right, SideBySideRenderedLine{Prefix: newNo, Content: highlighted})

		case DiffDelete:
			content := renderSegments(dl.Segments, DiffDelete)
			oldNo := lipgloss.NewStyle().Foreground(DiffGutterRemovedFg).Render(fmt.Sprintf("%*d", lineNoW, dl.OldLineNo)) + " "
			left = append(left, SideBySideRenderedLine{Prefix: oldNo, Content: content})
			right = append(right, SideBySideRenderedLine{Prefix: blankPrefix, Blank: true})

		case DiffInsert:
			content := renderSegments(dl.Segments, DiffInsert)
			newNo := lipgloss.NewStyle().Foreground(DiffGutterAddedFg).Render(fmt.Sprintf("%*d", lineNoW, dl.NewLineNo)) + " "
			left = append(left, SideBySideRenderedLine{Prefix: blankPrefix, Blank: true})
			right = append(right, SideBySideRenderedLine{Prefix: newNo, Content: content})
		}
	}
	return left, right
}

// renderSegments renders word-level diff segments with appropriate styling.
func renderSegments(segments []DiffSegment, lineType DiffLineType) string {
	if len(segments) == 0 {
		return ""
	}

	var normalFg, changedFg, changedBg lipgloss.TerminalColor
	switch lineType {
	case DiffInsert:
		normalFg = DiffAddedFg
		changedFg = DiffAddedChangedFg
		changedBg = DiffAddedChangedBg
	case DiffDelete:
		normalFg = DiffRemovedFg
		changedFg = DiffRemovedChangedFg
		changedBg = DiffRemovedChangedBg
	default:
		var sb strings.Builder
		for _, seg := range segments {
			sb.WriteString(seg.Text)
		}
		return sb.String()
	}

	normalSt := lipgloss.NewStyle().Foreground(normalFg)
	changedSt := lipgloss.NewStyle().Foreground(changedFg).Background(changedBg)

	var sb strings.Builder
	for _, seg := range segments {
		if seg.Changed {
			sb.WriteString(changedSt.Render(seg.Text))
		} else {
			sb.WriteString(normalSt.Render(seg.Text))
		}
	}
	return sb.String()
}

// highlightLine applies syntax highlighting based on language.
func highlightLine(content, lang string) string {
	switch strings.ToLower(lang) {
	case "sql", "mdl":
		return HighlightMDL(content)
	case "ndsl":
		return HighlightNDSL(content)
	case "":
		return DetectAndHighlight(content)
	default:
		return content
	}
}

// hslice returns a horizontal slice of an ANSI-colored string,
// skipping the first `skip` visual columns and returning up to `take` visual columns.
// Only CSI sequences (ESC [ ... letter) are handled; OSC/DCS/hyperlink escapes are not
// parsed. This is safe for the diff renderer which only emits lipgloss SGR sequences.
func hslice(s string, skip, take int) string {
	if skip == 0 {
		return truncateToWidth(s, take)
	}

	var result strings.Builder
	visW := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			if visW >= skip {
				result.WriteRune(r)
			}
			continue
		}
		if inEsc {
			if visW >= skip {
				result.WriteRune(r)
			}
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		rw := runewidth.RuneWidth(r)
		visW += rw
		if visW <= skip {
			continue
		}
		if visW-skip > take {
			break
		}
		result.WriteRune(r)
	}
	return result.String()
}

// truncateToWidth truncates a (possibly ANSI-colored) string to fit maxW visual columns.
func truncateToWidth(s string, maxW int) string {
	if lipgloss.Width(s) <= maxW {
		return s
	}

	var result strings.Builder
	visW := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			result.WriteRune(r)
			continue
		}
		if inEsc {
			result.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		rw := runewidth.RuneWidth(r)
		if visW+rw > maxW {
			break
		}
		visW += rw
		result.WriteRune(r)
	}
	return result.String()
}
