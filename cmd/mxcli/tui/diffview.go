package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DiffViewMode determines the diff display layout.
type DiffViewMode int

const (
	DiffViewUnified DiffViewMode = iota
	DiffViewSideBySide
	DiffViewPlainDiff // standard unified diff text (LLM-friendly)
)

// horizontalScrollStep is the number of columns to shift per horizontal scroll event.
const horizontalScrollStep = 8

// DiffOpenMsg requests opening a diff view.
type DiffOpenMsg struct {
	OldText  string
	NewText  string
	Language string // "sql", "go", "ndsl", "" (auto-detect)
	Title    string
}

// DiffView is a Bubble Tea component for interactive diff viewing.
type DiffView struct {
	// Input
	oldText  string
	newText  string
	language string
	title    string

	// Computed
	result     *DiffResult
	unified    []DiffRenderedLine       // pre-rendered unified lines
	sideLeft   []SideBySideRenderedLine // pre-rendered side-by-side left
	sideRight  []SideBySideRenderedLine // pre-rendered side-by-side right
	plainLines []string                 // standard unified diff text lines (LLM-friendly)
	hunkStarts []int                    // line indices where hunks begin

	// View state
	viewMode DiffViewMode
	yOffset  int
	xOffset  int // horizontal scroll offset (content only, line numbers stay fixed)
	width    int
	height   int

	// Search
	searching   bool
	searchInput textinput.Model
	searchQuery string
	matchLines  []int
	matchIdx    int

	// Two-key sequence state (e.g. ]c / [c for Vim hunk navigation)
	pendingKey rune // ']' or '[' waiting for 'c', 0 if none

	// Mode-specific hunk starts
	plainHunkStarts []int // hunk header indices in plainLines
}

// NewDiffView creates a DiffView from a DiffOpenMsg.
func NewDiffView(msg DiffOpenMsg, width, height int) DiffView {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.CharLimit = 200

	dv := DiffView{
		oldText:     msg.OldText,
		newText:     msg.NewText,
		language:    msg.Language,
		title:       msg.Title,
		width:       width,
		height:      height,
		searchInput: ti,
	}

	dv.result = ComputeDiff(msg.OldText, msg.NewText)
	dv.renderAll()
	dv.computeHunkStarts()

	return dv
}

func (dv *DiffView) renderAll() {
	dv.unified = RenderUnifiedDiff(dv.result, dv.language)
	dv.sideLeft, dv.sideRight = RenderSideBySideDiff(dv.result, dv.language)
	plain := RenderPlainUnifiedDiff(dv.result, "old", "new")
	dv.plainLines = strings.Split(plain, "\n")
	if len(dv.plainLines) > 0 && dv.plainLines[len(dv.plainLines)-1] == "" {
		dv.plainLines = dv.plainLines[:len(dv.plainLines)-1]
	}
}

func (dv *DiffView) computeHunkStarts() {
	dv.hunkStarts = nil
	dv.plainHunkStarts = nil
	if dv.result == nil {
		return
	}
	// Unified and Side-by-Side: 1:1 mapping with result.Lines
	for i, dl := range dv.result.Lines {
		if dl.Type == DiffEqual {
			continue
		}
		if i == 0 || dv.result.Lines[i-1].Type == DiffEqual {
			dv.hunkStarts = append(dv.hunkStarts, i)
		}
	}
	// Plain Diff: find @@ hunk header lines
	for i, line := range dv.plainLines {
		if strings.HasPrefix(line, "@@") {
			dv.plainHunkStarts = append(dv.plainHunkStarts, i)
		}
	}
}

// SetSize updates dimensions.
func (dv *DiffView) SetSize(w, h int) {
	dv.width = w
	dv.height = h
}

func (dv DiffView) totalLines() int {
	switch dv.viewMode {
	case DiffViewSideBySide:
		return len(dv.sideLeft)
	case DiffViewPlainDiff:
		return len(dv.plainLines)
	default:
		return len(dv.unified)
	}
}

func (dv DiffView) contentHeight() int {
	h := dv.height - 2 // title bar + hint bar
	if dv.searching {
		h--
	}
	return max(5, h)
}

func (dv DiffView) maxOffset() int {
	return max(0, dv.totalLines()-dv.contentHeight())
}

// --- Update ---

func (dv DiffView) updateInternal(msg tea.Msg) (DiffView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if dv.searching {
			return dv.updateSearch(msg)
		}
		return dv.updateNormal(msg)

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				dv.scroll(-3)
			case tea.MouseButtonWheelDown:
				dv.scroll(3)
			case tea.MouseButtonWheelLeft:
				dv.xOffset = max(0, dv.xOffset-horizontalScrollStep)
			case tea.MouseButtonWheelRight:
				dv.xOffset += horizontalScrollStep
			}
		}

	case tea.WindowSizeMsg:
		dv.SetSize(msg.Width, msg.Height)
	}
	return dv, nil
}

func (dv DiffView) updateSearch(msg tea.KeyMsg) (DiffView, tea.Cmd) {
	switch msg.String() {
	case "esc":
		dv.searching = false
		dv.searchInput.Blur()
		return dv, nil
	case "enter":
		dv.commitSearch()
		return dv, nil
	default:
		var cmd tea.Cmd
		dv.searchInput, cmd = dv.searchInput.Update(msg)
		dv.searchQuery = strings.TrimSpace(dv.searchInput.Value())
		dv.buildMatchLines()
		if len(dv.matchLines) > 0 {
			dv.matchIdx = 0
			dv.scrollToMatch()
		}
		return dv, cmd
	}
}

func (dv DiffView) updateNormal(msg tea.KeyMsg) (DiffView, tea.Cmd) {
	key := msg.String()

	// Handle two-key sequence: ]c / [c (Vim hunk navigation)
	if dv.pendingKey != 0 {
		pending := dv.pendingKey
		dv.pendingKey = 0
		if key == "c" {
			if pending == ']' {
				dv.nextHunk()
			} else {
				dv.prevHunk()
			}
			return dv, nil
		}
		// Not 'c' — discard pending and fall through to handle this key normally
	}

	switch key {
	case "q", "esc":
		return dv, func() tea.Msg { return PopViewMsg{} }

	// Vertical scroll
	case "j", "down":
		dv.scroll(1)
	case "k", "up":
		dv.scroll(-1)
	case "d", "ctrl+d":
		dv.scroll(dv.contentHeight() / 2)
	case "u", "ctrl+u":
		dv.scroll(-dv.contentHeight() / 2)
	case "f", "pgdown":
		dv.scroll(dv.contentHeight())
	case "b", "pgup":
		dv.scroll(-dv.contentHeight())
	case "g", "home":
		dv.yOffset = 0
	case "G", "end":
		dv.yOffset = dv.maxOffset()

	// Horizontal scroll
	case "h", "left":
		dv.xOffset = max(0, dv.xOffset-horizontalScrollStep)
	case "l", "right":
		dv.xOffset += horizontalScrollStep

	// View mode toggle: Unified → Side-by-Side → Plain Diff → Unified
	case "tab":
		switch dv.viewMode {
		case DiffViewUnified:
			dv.viewMode = DiffViewSideBySide
		case DiffViewSideBySide:
			dv.viewMode = DiffViewPlainDiff
		case DiffViewPlainDiff:
			dv.viewMode = DiffViewUnified
		}
		dv.yOffset = 0
		dv.xOffset = 0
		// Rebuild search matches for the new mode (indices differ between modes)
		dv.buildMatchLines()

	// Yank unified diff to clipboard
	case "y":
		plain := RenderPlainUnifiedDiff(dv.result, "old", "new")
		_ = writeClipboard(plain)

	// Search
	case "/":
		dv.searching = true
		dv.searchInput.SetValue(dv.searchQuery)
		dv.searchInput.Focus()
	case "n":
		dv.nextMatch()
	case "N":
		dv.prevMatch()

	// Hunk navigation: first key of ]c / [c sequence
	case "]":
		dv.pendingKey = ']'
	case "[":
		dv.pendingKey = '['
	}

	return dv, nil
}

func (dv *DiffView) scroll(delta int) {
	dv.yOffset = clamp(dv.yOffset+delta, 0, dv.maxOffset())
}

// --- Hunk navigation ---

// activeHunkStarts returns the hunk start indices appropriate for the current view mode.
func (dv *DiffView) activeHunkStarts() []int {
	if dv.viewMode == DiffViewPlainDiff {
		return dv.plainHunkStarts
	}
	return dv.hunkStarts
}

func (dv *DiffView) nextHunk() {
	starts := dv.activeHunkStarts()
	if len(starts) == 0 {
		return
	}
	for _, hs := range starts {
		if hs > dv.yOffset {
			dv.yOffset = clamp(hs, 0, dv.maxOffset())
			return
		}
	}
	dv.yOffset = clamp(starts[0], 0, dv.maxOffset())
}

func (dv *DiffView) prevHunk() {
	starts := dv.activeHunkStarts()
	if len(starts) == 0 {
		return
	}
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] < dv.yOffset {
			dv.yOffset = clamp(starts[i], 0, dv.maxOffset())
			return
		}
	}
	dv.yOffset = clamp(starts[len(starts)-1], 0, dv.maxOffset())
}

// --- Search ---

func (dv *DiffView) commitSearch() {
	dv.searching = false
	dv.searchInput.Blur()
	dv.searchQuery = strings.TrimSpace(dv.searchInput.Value())
	dv.buildMatchLines()
	if len(dv.matchLines) > 0 {
		dv.matchIdx = 0
		dv.scrollToMatch()
	}
}

func (dv *DiffView) buildMatchLines() {
	dv.matchLines = nil
	if dv.searchQuery == "" {
		return
	}
	q := strings.ToLower(dv.searchQuery)
	if dv.viewMode == DiffViewPlainDiff {
		// In PlainDiff mode, search against plainLines (which have different indices)
		for i, line := range dv.plainLines {
			if strings.Contains(strings.ToLower(line), q) {
				dv.matchLines = append(dv.matchLines, i)
			}
		}
	} else if dv.result != nil {
		// Unified and Side-by-Side: search DiffResult lines (1:1 index mapping)
		for i, dl := range dv.result.Lines {
			if strings.Contains(strings.ToLower(dl.Content), q) {
				dv.matchLines = append(dv.matchLines, i)
			}
		}
	}
}

func (dv *DiffView) nextMatch() {
	if len(dv.matchLines) == 0 {
		return
	}
	dv.matchIdx = (dv.matchIdx + 1) % len(dv.matchLines)
	dv.scrollToMatch()
}

func (dv *DiffView) prevMatch() {
	if len(dv.matchLines) == 0 {
		return
	}
	dv.matchIdx--
	if dv.matchIdx < 0 {
		dv.matchIdx = len(dv.matchLines) - 1
	}
	dv.scrollToMatch()
}

func (dv *DiffView) scrollToMatch() {
	if dv.matchIdx >= len(dv.matchLines) {
		return
	}
	target := dv.matchLines[dv.matchIdx]
	dv.yOffset = clamp(target-dv.contentHeight()/2, 0, dv.maxOffset())
}

func (dv DiffView) searchInfo() string {
	if dv.searchQuery == "" || len(dv.matchLines) == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", dv.matchIdx+1, len(dv.matchLines))
}

// --- View ---

func (dv DiffView) View() string {
	titleSt := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimSt := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	keySt := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	activeSt := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	addSt := lipgloss.NewStyle().Foreground(DiffAddedFg).Bold(true)
	delSt := lipgloss.NewStyle().Foreground(DiffRemovedFg).Bold(true)

	// Title bar
	var modeLabel string
	switch dv.viewMode {
	case DiffViewUnified:
		modeLabel = "Unified"
	case DiffViewSideBySide:
		modeLabel = "Side-by-Side"
	case DiffViewPlainDiff:
		modeLabel = "Plain Diff (LLM)"
	}
	stats := ""
	if dv.result != nil {
		stats = addSt.Render(fmt.Sprintf("+%d", dv.result.Stats.Additions)) + " " +
			delSt.Render(fmt.Sprintf("-%d", dv.result.Stats.Deletions))
	}
	pct := fmt.Sprintf("%d%%", dv.scrollPercent())
	xInfo := ""
	if dv.xOffset > 0 {
		xInfo = dimSt.Render(fmt.Sprintf(" col:%d", dv.xOffset))
	}
	titleBar := titleSt.Render(dv.title) + "  " + stats + "  " +
		dimSt.Render("["+modeLabel+"]") + "  " + dimSt.Render(pct) + xInfo

	// Content
	viewH := dv.contentHeight()
	var content string
	switch dv.viewMode {
	case DiffViewSideBySide:
		content = dv.renderSideBySide(viewH)
	case DiffViewPlainDiff:
		content = dv.renderPlainDiff(viewH)
	default:
		content = dv.renderUnified(viewH)
	}

	// Hint bar
	var hints []string
	hints = append(hints, keySt.Render("j/k")+" "+dimSt.Render("vert"))
	hints = append(hints, keySt.Render("h/l")+" "+dimSt.Render("horiz"))
	hints = append(hints, keySt.Render("Tab")+" "+dimSt.Render("mode"))
	hints = append(hints, keySt.Render("]c/[c")+" "+dimSt.Render("hunk"))
	hints = append(hints, keySt.Render("/")+" "+dimSt.Render("search"))
	if si := dv.searchInfo(); si != "" {
		hints = append(hints, keySt.Render("n/N")+" "+activeSt.Render(si))
	}
	hints = append(hints, keySt.Render("q")+" "+dimSt.Render("close"))
	hintLine := " " + strings.Join(hints, "  ")

	var sb strings.Builder
	sb.WriteString(titleBar)
	sb.WriteString("\n")
	sb.WriteString(content)
	sb.WriteString("\n")

	if dv.searching {
		matchInfo := ""
		if q := strings.TrimSpace(dv.searchInput.Value()); q != "" {
			matchInfo = fmt.Sprintf(" (%d matches)", len(dv.matchLines))
		}
		sb.WriteString(dv.searchInput.View() + dimSt.Render(matchInfo))
	} else {
		sb.WriteString(hintLine)
	}

	return sb.String()
}

func (dv DiffView) renderUnified(viewH int) string {
	lines := dv.unified
	total := len(lines)
	thumbStart, thumbEnd, scrollW := scrollbarGeometry(total, viewH, dv.yOffset)

	// Prefix is fixed (sticky line numbers); content gets remaining width.
	prefixW := 0
	if len(lines) > 0 {
		prefixW = lipgloss.Width(lines[0].Prefix)
	}
	contentW := max(10, dv.width-prefixW-scrollW)

	var sb strings.Builder
	for vi := range viewH {
		lineIdx := dv.yOffset + vi
		var line string
		if lineIdx < total {
			rl := lines[lineIdx]
			content := hslice(rl.Content, dv.xOffset, contentW)
			if pad := contentW - lipgloss.Width(content); pad > 0 {
				content += strings.Repeat(" ", pad)
			}
			line = rl.Prefix + content
		} else {
			line = strings.Repeat(" ", dv.width-scrollW)
		}
		if scrollW > 0 {
			line += scrollbarChar(vi, thumbStart, thumbEnd)
		}
		sb.WriteString(line)
		if vi < viewH-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (dv DiffView) renderSideBySide(viewH int) string {
	dividerSt := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	total := len(dv.sideLeft)
	thumbStart, thumbEnd, scrollW := scrollbarGeometry(total, viewH, dv.yOffset)

	const dividerW = 3 // " │ "
	paneTotal := (dv.width - dividerW - scrollW) / 2

	prefixW := 0
	if len(dv.sideLeft) > 0 {
		prefixW = lipgloss.Width(dv.sideLeft[0].Prefix)
	}
	contentW := max(5, paneTotal-prefixW)

	var sb strings.Builder
	for vi := range viewH {
		lineIdx := dv.yOffset + vi

		var leftStr, rightStr string
		if lineIdx < total {
			ll := dv.sideLeft[lineIdx]
			leftContent := hslice(ll.Content, dv.xOffset, contentW)
			if pad := contentW - lipgloss.Width(leftContent); pad > 0 {
				leftContent += strings.Repeat(" ", pad)
			}
			leftStr = ll.Prefix + leftContent
		} else {
			leftStr = strings.Repeat(" ", paneTotal)
		}

		if lineIdx < len(dv.sideRight) {
			rl := dv.sideRight[lineIdx]
			rightContent := hslice(rl.Content, dv.xOffset, contentW)
			if pad := contentW - lipgloss.Width(rightContent); pad > 0 {
				rightContent += strings.Repeat(" ", pad)
			}
			rightStr = rl.Prefix + rightContent
		} else {
			rightStr = strings.Repeat(" ", paneTotal)
		}

		line := leftStr + dividerSt.Render(" │ ") + rightStr
		if scrollW > 0 {
			line += scrollbarChar(vi, thumbStart, thumbEnd)
		}
		sb.WriteString(line)
		if vi < viewH-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (dv DiffView) renderPlainDiff(viewH int) string {
	lines := dv.plainLines
	total := len(lines)
	thumbStart, thumbEnd, scrollW := scrollbarGeometry(total, viewH, dv.yOffset)
	contentW := dv.width - scrollW

	var sb strings.Builder
	for vi := range viewH {
		lineIdx := dv.yOffset + vi
		var line string
		if lineIdx < total {
			line = hslice(lines[lineIdx], dv.xOffset, contentW)
		}
		// Pad to fill width (use visual width for multi-byte safety)
		if pad := contentW - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		if scrollW > 0 {
			line += scrollbarChar(vi, thumbStart, thumbEnd)
		}
		sb.WriteString(line)
		if vi < viewH-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// scrollbarGeometry computes thumb position for a scrollbar given content/viewport metrics.
// Returns (thumbStart, thumbEnd, scrollW); scrollW is 0 when no scrollbar is needed.
func scrollbarGeometry(total, viewH, yOffset int) (thumbStart, thumbEnd, scrollW int) {
	if total <= viewH {
		return 0, 0, 0
	}
	maxOff := total - viewH
	thumbSize := max(1, viewH*viewH/total)
	if maxOff > 0 {
		thumbStart = yOffset * (viewH - thumbSize) / maxOff
	}
	return thumbStart, thumbStart + thumbSize, 1
}

var (
	scrollTrackSt = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	scrollThumbSt = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
)

// scrollbarChar returns the rendered scrollbar character for row vi.
func scrollbarChar(vi, thumbStart, thumbEnd int) string {
	if vi >= thumbStart && vi < thumbEnd {
		return scrollThumbSt.Render("█")
	}
	return scrollTrackSt.Render("│")
}

func (dv DiffView) scrollPercent() int {
	m := dv.maxOffset()
	if m <= 0 {
		return 100
	}
	return int(float64(dv.yOffset) / float64(m) * 100)
}

// --- View interface ---

// Update satisfies the View interface.
func (dv DiffView) Update(msg tea.Msg) (View, tea.Cmd) {
	updated, cmd := dv.updateInternal(msg)
	return updated, cmd
}

// Render satisfies the View interface, with an LLM anchor prefix.
// DiffView uses a value receiver: width/height are set on the local copy
// so that View() picks them up within this call. The original is unaffected.
func (dv DiffView) Render(width, height int) string {
	dv.width = width
	dv.height = height
	rendered := dv.View()

	// Embed LLM anchor as muted prefix on the first line
	info := dv.StatusInfo()
	anchor := fmt.Sprintf("[mxcli:diff] %s  %s", info.Mode, info.Extra)
	anchorStr := lipgloss.NewStyle().Foreground(MutedColor).Faint(true).Render(anchor)

	if idx := strings.IndexByte(rendered, '\n'); idx >= 0 {
		rendered = anchorStr + rendered[idx:]
	} else {
		rendered = anchorStr
	}
	return rendered
}

// Hints satisfies the View interface.
func (dv DiffView) Hints() []Hint {
	return DiffViewHints
}

// StatusInfo satisfies the View interface.
func (dv DiffView) StatusInfo() StatusInfo {
	var modeLabel string
	switch dv.viewMode {
	case DiffViewUnified:
		modeLabel = "Unified"
	case DiffViewSideBySide:
		modeLabel = "Side-by-Side"
	case DiffViewPlainDiff:
		modeLabel = "Plain Diff"
	}
	extra := ""
	if dv.result != nil {
		extra = fmt.Sprintf("+%d -%d", dv.result.Stats.Additions, dv.result.Stats.Deletions)
	}
	return StatusInfo{
		Breadcrumb: []string{dv.title},
		Position:   fmt.Sprintf("%d%%", dv.scrollPercent()),
		Mode:       modeLabel,
		Extra:      extra,
	}
}

// Mode satisfies the View interface.
func (dv DiffView) Mode() ViewMode {
	return ModeDiff
}
