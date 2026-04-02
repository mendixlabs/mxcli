package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ContentView is a scrollable content viewer with line numbers, scrollbar,
// vim navigation, search, and mouse support.
type ContentView struct {
	lines           []string
	yOffset         int
	width           int
	height          int
	gutterW         int
	hideLineNumbers bool

	// Search state
	searching   bool
	searchInput textinput.Model
	searchQuery string // locked-in query (after Enter)
	matchLines  []int  // line indices that match
	matchIdx    int    // current match index in matchLines
}

func NewContentView(width, height int) ContentView {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.CharLimit = 200
	return ContentView{width: width, height: height, searchInput: ti}
}

func (v *ContentView) SetContent(content string) {
	v.lines = strings.Split(content, "\n")
	v.yOffset = 0
	v.gutterW = max(4, len(fmt.Sprintf("%d", len(v.lines)))+1)
	v.clearSearch()
}

func (v *ContentView) SetSize(w, h int) { v.width = w; v.height = h }
func (v *ContentView) GotoTop()         { v.yOffset = 0 }
func (v ContentView) TotalLines() int   { return len(v.lines) }
func (v ContentView) YOffset() int      { return v.yOffset }
func (v ContentView) IsSearching() bool { return v.searching }

// PlainText returns the content as plain text with ANSI codes stripped.
func (v ContentView) PlainText() string {
	var sb strings.Builder
	for i, line := range v.lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(stripAnsi(line))
	}
	return sb.String()
}

func (v ContentView) ScrollPercent() float64 {
	m := v.maxOffset()
	if m <= 0 {
		return 1.0
	}
	return float64(v.yOffset) / float64(m)
}

func (v *ContentView) SetYOffset(y int) { v.yOffset = clamp(y, 0, v.maxOffset()) }

func (v ContentView) maxOffset() int { return max(0, len(v.lines)-v.height) }

func clamp(val, lo, hi int) int {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

// SearchInfo returns a summary like "3/12" (current match / total matches) or "".
func (v ContentView) SearchInfo() string {
	if v.searchQuery == "" || len(v.matchLines) == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", v.matchIdx+1, len(v.matchLines))
}

// --- Search ---

func (v *ContentView) startSearch() {
	v.searching = true
	v.searchInput.SetValue(v.searchQuery)
	v.searchInput.Focus()
}

func (v *ContentView) clearSearch() {
	v.searching = false
	v.searchQuery = ""
	v.matchLines = nil
	v.matchIdx = 0
	v.searchInput.Blur()
}

func (v *ContentView) commitSearch() {
	v.searching = false
	v.searchInput.Blur()
	v.searchQuery = strings.TrimSpace(v.searchInput.Value())
	v.buildMatchLines()
	if len(v.matchLines) > 0 {
		v.matchIdx = 0
		v.scrollToMatch()
	}
}

func (v *ContentView) buildMatchLines() {
	v.matchLines = nil
	if v.searchQuery == "" {
		return
	}
	q := strings.ToLower(v.searchQuery)
	for i, line := range v.lines {
		// Strip ANSI for matching
		if strings.Contains(strings.ToLower(stripAnsi(line)), q) {
			v.matchLines = append(v.matchLines, i)
		}
	}
}

func (v *ContentView) nextMatch() {
	if len(v.matchLines) == 0 {
		return
	}
	v.matchIdx = (v.matchIdx + 1) % len(v.matchLines)
	v.scrollToMatch()
}

func (v *ContentView) prevMatch() {
	if len(v.matchLines) == 0 {
		return
	}
	v.matchIdx--
	if v.matchIdx < 0 {
		v.matchIdx = len(v.matchLines) - 1
	}
	v.scrollToMatch()
}

func (v *ContentView) scrollToMatch() {
	if v.matchIdx >= len(v.matchLines) {
		return
	}
	target := v.matchLines[v.matchIdx]
	// Center the match in the viewport
	v.yOffset = clamp(target-v.height/2, 0, v.maxOffset())
}

// --- Update ---

func (v ContentView) Update(msg tea.Msg) (ContentView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if v.searching {
			return v.updateSearch(msg)
		}
		return v.updateNormal(msg)

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				v.yOffset = clamp(v.yOffset-mouseScrollStep, 0, v.maxOffset())
			case tea.MouseButtonWheelDown:
				v.yOffset = clamp(v.yOffset+mouseScrollStep, 0, v.maxOffset())
			}
		}
	}
	return v, nil
}

func (v ContentView) updateSearch(msg tea.KeyMsg) (ContentView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		v.searching = false
		v.searchInput.Blur()
		return v, nil
	case "enter":
		v.commitSearch()
		return v, nil
	default:
		var cmd tea.Cmd
		v.searchInput, cmd = v.searchInput.Update(msg)
		// Live search: update matches as user types
		v.searchQuery = strings.TrimSpace(v.searchInput.Value())
		v.buildMatchLines()
		if len(v.matchLines) > 0 {
			v.matchIdx = 0
			v.scrollToMatch()
		}
		return v, cmd
	}
}

func (v ContentView) updateNormal(msg tea.KeyMsg) (ContentView, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		v.yOffset = clamp(v.yOffset+1, 0, v.maxOffset())
	case "k", "up":
		v.yOffset = clamp(v.yOffset-1, 0, v.maxOffset())
	case "d", "ctrl+d":
		v.yOffset = clamp(v.yOffset+v.height/2, 0, v.maxOffset())
	case "u", "ctrl+u":
		v.yOffset = clamp(v.yOffset-v.height/2, 0, v.maxOffset())
	case "f", "pgdown":
		v.yOffset = clamp(v.yOffset+v.height, 0, v.maxOffset())
	case "b", "pgup":
		v.yOffset = clamp(v.yOffset-v.height, 0, v.maxOffset())
	case "g", "home":
		v.yOffset = 0
	case "G", "end":
		v.yOffset = v.maxOffset()
	case "/":
		v.startSearch()
	case "n":
		v.nextMatch()
	case "N":
		v.prevMatch()
	}
	return v, nil
}

// --- View ---

func (v ContentView) View() string {
	if len(v.lines) == 0 {
		return ""
	}

	lineNumSt := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	matchLineNumSt := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	currentMatchNumSt := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	trackSt := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	thumbSt := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	matchSt := lipgloss.NewStyle().Background(lipgloss.Color("58"))
	currentMatchSt := lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("0"))

	total := len(v.lines)
	showScrollbar := total > v.height
	effectiveGutterW := v.gutterW
	if v.hideLineNumbers {
		effectiveGutterW = 0
	}
	contentW := v.width - effectiveGutterW - 1
	if showScrollbar {
		contentW--
	}
	contentW = max(10, contentW)

	// Scrollbar geometry
	var thumbStart, thumbEnd int
	if showScrollbar {
		trackH := v.height
		thumbSize := max(1, trackH*v.height/total)
		if m := v.maxOffset(); m > 0 {
			thumbStart = v.yOffset * (trackH - thumbSize) / m
		}
		thumbEnd = thumbStart + thumbSize
	}

	// Current match line (for highlighting)
	currentMatchLine := -1
	if v.searchQuery != "" && len(v.matchLines) > 0 && v.matchIdx < len(v.matchLines) {
		currentMatchLine = v.matchLines[v.matchIdx]
	}

	// Match line set for fast lookup
	matchSet := make(map[int]bool, len(v.matchLines))
	for _, ml := range v.matchLines {
		matchSet[ml] = true
	}

	viewH := v.height
	if v.searching {
		viewH-- // reserve last line for search input
	}

	var sb strings.Builder
	for vi := range viewH {
		lineIdx := v.yOffset + vi
		var line string
		if lineIdx < total {
			var gutter string
			if v.hideLineNumbers {
				gutter = ""
			} else {
				num := fmt.Sprintf("%*d", v.gutterW-1, lineIdx+1)

				// Style line number based on match status
				if lineIdx == currentMatchLine {
					gutter = currentMatchNumSt.Render(num) + " "
				} else if matchSet[lineIdx] {
					gutter = matchLineNumSt.Render(num) + " "
				} else {
					gutter = lineNumSt.Render(num) + " "
				}
			}

			content := v.lines[lineIdx]

			// Highlight search matches within the line
			if v.searchQuery != "" && matchSet[lineIdx] {
				if lineIdx == currentMatchLine {
					content = highlightMatches(content, v.searchQuery, currentMatchSt)
				} else {
					content = highlightMatches(content, v.searchQuery, matchSt)
				}
			}

			// Truncate
			if lipgloss.Width(content) > contentW {
				runes := []rune(content)
				if len(runes) > contentW {
					content = string(runes[:contentW])
				}
			}

			// Pad
			if pad := contentW - lipgloss.Width(content); pad > 0 {
				content += strings.Repeat(" ", pad)
			}

			line = gutter + content
		} else {
			line = strings.Repeat(" ", effectiveGutterW+contentW)
		}

		// Scrollbar
		if showScrollbar {
			if vi >= thumbStart && vi < thumbEnd {
				line += thumbSt.Render("█")
			} else {
				line += trackSt.Render("│")
			}
		}

		sb.WriteString(line)
		if vi < viewH-1 || v.searching {
			sb.WriteString("\n")
		}
	}

	// Search input bar
	if v.searching {
		matchInfo := ""
		if q := strings.TrimSpace(v.searchInput.Value()); q != "" {
			matchInfo = fmt.Sprintf(" (%d matches)", len(v.matchLines))
		}
		sb.WriteString(v.searchInput.View() + lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(matchInfo))
	}

	return sb.String()
}

// highlightMatches highlights all occurrences of query in the line (case-insensitive).
// Works with ANSI-colored text by matching on stripped text and applying style around matches.
func highlightMatches(line, query string, style lipgloss.Style) string {
	plain := stripAnsi(line)
	lowerPlain := strings.ToLower(plain)
	lowerQuery := strings.ToLower(query)

	// If line has no ANSI, do simple replacement
	if plain == line {
		var result strings.Builder
		remaining := line
		lowerRemaining := lowerPlain
		for {
			idx := strings.Index(lowerRemaining, lowerQuery)
			if idx < 0 {
				result.WriteString(remaining)
				break
			}
			result.WriteString(remaining[:idx])
			result.WriteString(style.Render(remaining[idx : idx+len(query)]))
			remaining = remaining[idx+len(query):]
			lowerRemaining = lowerRemaining[idx+len(query):]
		}
		return result.String()
	}

	// For ANSI text, find match positions in plain text and highlight
	// by wrapping the entire line with a marker. Simple approach: just return
	// the line as-is with ANSI — the line number coloring indicates matches.
	return line
}
