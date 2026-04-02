package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Layout constants for column width calculations.
const (
	minTwoColumnWidth = 80
	parentMaxPct      = 30
	currentMaxPct     = 35
	previewMinPct     = 25
	twoColCurrentPct  = 50
	minParentWidth    = 8
	minCurrentWidth   = 15
)

// previewDebounceDelay is the delay before requesting a preview for a leaf node,
// preventing subprocess flooding during rapid cursor movement.
const previewDebounceDelay = 150 * time.Millisecond

// MillerFocus indicates which pane has keyboard focus.
type MillerFocus int

const (
	MillerFocusParent MillerFocus = iota
	MillerFocusCurrent
)

// PreviewPane holds the right-column state: either child items or leaf content.
type PreviewPane struct {
	childColumn  *Column
	content      string
	imagePaths   []string // source image file paths for lazy rendering
	contentLines []string // split content for scrolling
	highlighted  string
	mode         PreviewMode
	loading      bool
	scrollOffset int
}

// navEntry stores one level of the navigation stack for drill-in / go-back.
type navEntry struct {
	parentItems   []ColumnItem
	currentItems  []ColumnItem
	parentTitle   string
	currentTitle  string
	parentNode    *TreeNode // the node whose children are shown in current
	parentCursor  int
	currentCursor int
}

// previewDebounceMsg fires after the debounce delay for leaf-node previews.
type previewDebounceMsg struct {
	node    *TreeNode
	counter int
}

// animTickMsg is kept for backward compatibility (forwarded in app.go).
type animTickMsg struct{}

// MillerView coordinates three columns: parent, current, and preview.
type MillerView struct {
	parent  Column
	current Column
	preview PreviewPane

	previewEngine *PreviewEngine
	rootNodes     []*TreeNode
	currentParent *TreeNode // node whose children fill the current column

	focus    MillerFocus
	navStack []navEntry

	width           int
	height          int
	zenMode         bool
	debounceCounter int
}

// NewMillerView creates a MillerView wired to the given preview engine.
func NewMillerView(previewEngine *PreviewEngine) MillerView {
	return MillerView{
		parent:        NewColumn("Parent"),
		current:       NewColumn("Current"),
		previewEngine: previewEngine,
		preview:       PreviewPane{mode: PreviewMDL},
		focus:         MillerFocusCurrent,
	}
}

// SetRootNodes loads the top-level tree nodes and resets navigation.
func (m *MillerView) SetRootNodes(nodes []*TreeNode) {
	m.rootNodes = nodes
	m.currentParent = nil
	m.navStack = nil
	m.focus = MillerFocusCurrent

	m.parent.SetItems(nil)
	m.parent.SetTitle("")

	items := treeNodesToItems(nodes)
	m.current.SetItems(items)
	m.current.SetTitle("Project")
	m.current.SetFocused(true)
	m.parent.SetFocused(false)

	m.clearPreview()
}

// SetSize updates dimensions and recalculates column widths.
func (m *MillerView) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.relayout()
}

// --- Update ---

// Update routes messages to the focused column or handles navigation.
func (m MillerView) Update(msg tea.Msg) (MillerView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case CursorChangedMsg:
		Trace("miller: CursorChanged node=%q type=%q children=%d", msg.Node.Label, msg.Node.Type, len(msg.Node.Children))
		return m.handleCursorChanged(msg)

	case PreviewReadyMsg:
		Trace("miller: PreviewReady key=%q highlight=%q len=%d", msg.NodeKey, msg.HighlightType, len(msg.Content))
		m.preview.loading = false
		m.preview.content = msg.Content
		m.preview.imagePaths = msg.ImagePaths
		m.preview.contentLines = strings.Split(msg.Content, "\n")
		m.preview.highlighted = msg.HighlightType
		m.preview.childColumn = nil
		m.preview.scrollOffset = 0
		return m, nil

	case PreviewLoadingMsg:
		m.preview.loading = true
		return m, nil

	case previewDebounceMsg:
		// Ignore if superseded by a newer cursor move
		if msg.counter != m.debounceCounter {
			return m, nil
		}
		node := msg.node
		if node != nil && node.QualifiedName != "" && node.Type != "" {
			cmd := m.previewEngine.RequestPreview(node.Type, node.QualifiedName, m.preview.mode)
			return m, cmd
		}
		return m, nil

	case animTickMsg:
		return m, nil // no-op, animation removed

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	// Forward to focused column
	return m.forwardToFocused(msg)
}

func (m MillerView) handleKey(msg tea.KeyMsg) (MillerView, tea.Cmd) {
	// In filter mode, delegate entirely to focused column
	if m.focusedColumn().IsFilterActive() {
		return m.forwardToFocused(msg)
	}

	switch msg.String() {
	case "l", "right", "enter":
		return m.drillIn()
	case "h", "left":
		return m.goBack()
	case "tab":
		return m.togglePreviewMode()
	case "z":
		m.zenMode = !m.zenMode
		m.relayout()
		return m, nil
	}

	// Forward navigation keys to focused column
	return m.forwardToFocused(msg)
}

func (m MillerView) forwardToFocused(msg tea.Msg) (MillerView, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case MillerFocusParent:
		m.parent, cmd = m.parent.Update(msg)
	default:
		m.current, cmd = m.current.Update(msg)
	}
	return m, cmd
}

func (m MillerView) handleCursorChanged(msg CursorChangedMsg) (MillerView, tea.Cmd) {
	node := msg.Node
	if node == nil {
		m.clearPreview()
		return m, nil
	}

	// Nodes with children: show child column immediately (no subprocess, no debounce)
	if len(node.Children) > 0 {
		col := NewColumn(node.Label)
		col.SetItems(treeNodesToItems(node.Children))
		m.preview.childColumn = &col
		m.preview.content = ""
		m.preview.imagePaths = nil
		m.preview.contentLines = nil
		m.preview.loading = false
		m.preview.scrollOffset = 0
		m.relayout()
		return m, nil
	}

	// Leaf node: debounce preview request to avoid flooding subprocesses
	m.preview.childColumn = nil
	m.preview.scrollOffset = 0
	m.debounceCounter++
	counter := m.debounceCounter

	if node.QualifiedName != "" && node.Type != "" {
		return m, tea.Tick(previewDebounceDelay, func(t time.Time) tea.Msg {
			return previewDebounceMsg{node: node, counter: counter}
		})
	}

	m.preview.content = ""
	m.preview.imagePaths = nil
	m.preview.contentLines = nil
	m.preview.loading = false
	return m, nil
}

func (m MillerView) drillIn() (MillerView, tea.Cmd) {
	selected := m.current.SelectedNode()
	if selected == nil || len(selected.Children) == 0 {
		Trace("miller: drillIn no-op (nil or no children)")
		return m, nil
	}
	Trace("miller: drillIn into %q (%d children)", selected.Label, len(selected.Children))

	// Use actual item indices (not filtered-list cursor positions) so that
	// SetItems (which clears the filter) restores the cursor to the right item.
	actualCurrentIdx := m.current.selectedIndex()
	if actualCurrentIdx < 0 {
		actualCurrentIdx = m.current.cursor
	}
	actualParentIdx := m.parent.selectedIndex()
	if actualParentIdx < 0 {
		actualParentIdx = m.parent.cursor
	}

	// Save current state including cursor positions for goBack restore
	entry := navEntry{
		parentItems:   cloneItems(m.parent.items),
		currentItems:  cloneItems(m.current.items),
		parentTitle:   m.parent.Title(),
		currentTitle:  m.current.Title(),
		parentNode:    m.currentParent,
		parentCursor:  actualParentIdx,
		currentCursor: actualCurrentIdx,
	}
	m.navStack = append(m.navStack, entry)

	// Shift: current → parent, children → current.
	// Use actual item index (not filtered cursor) so parent highlights the
	// correct item after SetItems clears the filter.
	m.parent.SetItems(cloneItems(m.current.items))
	m.parent.SetTitle(m.current.Title())
	m.parent.SetCursor(actualCurrentIdx)
	m.currentParent = selected

	items := treeNodesToItems(selected.Children)
	m.current.SetItems(items)
	m.current.SetTitle(selected.Label)

	m.clearPreview()
	m.focus = MillerFocusCurrent
	m.updateFocusStyles()
	m.relayout()

	// Trigger preview for first item in new current column
	if node := m.current.SelectedNode(); node != nil {
		if len(node.Children) > 0 {
			col := NewColumn(node.Label)
			col.SetItems(treeNodesToItems(node.Children))
			m.preview.childColumn = &col
			m.relayout()
		} else if node.QualifiedName != "" && node.Type != "" {
			return m, m.previewEngine.RequestPreview(node.Type, node.QualifiedName, m.preview.mode)
		}
	}

	return m, nil
}

func (m MillerView) goBack() (MillerView, tea.Cmd) {
	depth := len(m.navStack)
	Trace("miller: goBack depth=%d", depth)
	if depth == 0 {
		return m, nil
	}

	entry := m.navStack[depth-1]
	m.navStack = m.navStack[:depth-1]

	// Restore: parent items → parent, saved current → current.
	// SetItems resets cursor to 0, so restore saved positions after.
	m.parent.SetItems(entry.parentItems)
	m.parent.SetTitle(entry.parentTitle)
	m.parent.SetCursor(entry.parentCursor)
	m.current.SetItems(entry.currentItems)
	m.current.SetTitle(entry.currentTitle)
	m.current.SetCursor(entry.currentCursor)
	m.currentParent = entry.parentNode

	m.clearPreview()
	m.focus = MillerFocusCurrent
	m.updateFocusStyles()
	m.relayout()

	// Trigger preview for selected item in restored current column
	if node := m.current.SelectedNode(); node != nil {
		if len(node.Children) > 0 {
			col := NewColumn(node.Label)
			col.SetItems(treeNodesToItems(node.Children))
			m.preview.childColumn = &col
			m.relayout()
		} else if node.QualifiedName != "" && node.Type != "" {
			return m, m.previewEngine.RequestPreview(node.Type, node.QualifiedName, m.preview.mode)
		}
	}

	return m, nil
}

// goBackToDepth navigates back until the navStack has targetDepth entries.
func (m MillerView) goBackToDepth(targetDepth int) (MillerView, tea.Cmd) {
	for len(m.navStack) > targetDepth {
		m, _ = m.goBack()
	}
	// Trigger preview for current selection
	if node := m.current.SelectedNode(); node != nil {
		if len(node.Children) > 0 {
			col := NewColumn(node.Label)
			col.SetItems(treeNodesToItems(node.Children))
			m.preview.childColumn = &col
			m.relayout()
		} else if node.QualifiedName != "" && node.Type != "" {
			return m, m.previewEngine.RequestPreview(node.Type, node.QualifiedName, m.preview.mode)
		}
	}
	return m, nil
}

func (m MillerView) togglePreviewMode() (MillerView, tea.Cmd) {
	if m.preview.mode == PreviewMDL {
		m.preview.mode = PreviewNDSL
	} else {
		m.preview.mode = PreviewMDL
	}

	// Re-request for current selection
	node := m.current.SelectedNode()
	if node != nil && node.QualifiedName != "" && node.Type != "" && len(node.Children) == 0 {
		cmd := m.previewEngine.RequestPreview(node.Type, node.QualifiedName, m.preview.mode)
		return m, cmd
	}
	return m, nil
}

// --- View ---

// View renders the three columns side by side with dim separators.
func (m *MillerView) View() string {
	if m.zenMode {
		return m.viewZen()
	}

	parentW, currentW, previewW := m.columnWidths()

	// Build separator: exactly m.height lines of │
	sepLines := make([]string, m.height)
	sepChar := SeparatorStyle.Render(SeparatorChar)
	for i := range sepLines {
		sepLines[i] = sepChar
	}
	sep := strings.Join(sepLines, "\n")

	var parts []string

	// Parent column (hidden when too narrow)
	if parentW > 0 {
		m.parent.SetSize(parentW, m.height)
		parts = append(parts, m.parent.View(), sep)
	}

	// Current column
	m.current.SetSize(currentW, m.height)
	parts = append(parts, m.current.View(), sep)

	// Preview column
	previewContent := m.renderPreview(previewW)
	parts = append(parts, previewContent)

	rendered := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	// Clamp to exactly m.height lines to prevent overflow
	outLines := strings.Split(rendered, "\n")
	if len(outLines) > m.height {
		outLines = outLines[:m.height]
	}
	return strings.Join(outLines, "\n")
}

func (m *MillerView) viewZen() string {
	// Build separator
	sepLines := make([]string, m.height)
	sepChar := SeparatorStyle.Render(SeparatorChar)
	for i := range sepLines {
		sepLines[i] = sepChar
	}
	sep := strings.Join(sepLines, "\n")

	// Allocate widths: current ~40%, preview ~60% (1 sep char)
	usable := m.width - 1
	currentW := usable * 40 / 100
	if currentW < minCurrentWidth {
		currentW = minCurrentWidth
	}
	previewW := usable - currentW

	var col *Column
	if m.focus == MillerFocusParent {
		col = &m.parent
	} else {
		col = &m.current
	}
	col.SetSize(currentW, m.height)

	previewContent := m.renderPreview(previewW)
	rendered := lipgloss.JoinHorizontal(lipgloss.Top, col.View(), sep, previewContent)

	outLines := strings.Split(rendered, "\n")
	if len(outLines) > m.height {
		outLines = outLines[:m.height]
	}
	return strings.Join(outLines, "\n")
}

func (m MillerView) renderPreview(previewWidth int) string {
	if m.preview.loading {
		return LoadingStyle.
			Width(previewWidth).
			Height(m.height).
			Render("Loading…")
	}

	if m.preview.childColumn != nil {
		m.preview.childColumn.SetFocused(false)
		m.preview.childColumn.SetSize(previewWidth, m.height)
		return m.preview.childColumn.View()
	}

	if m.preview.content != "" {
		// Mode label (always visible, not scrolled)
		modeLabel := "MDL"
		if m.preview.mode == PreviewNDSL {
			modeLabel = "NDSL"
		}
		if len(m.preview.imagePaths) > 0 {
			modeLabel += "  🖼 click path to view"
		}

		contentHeight := m.height - 1 // reserve 1 line for header
		if contentHeight < 1 {
			contentHeight = 1
		}
		srcLines := m.preview.contentLines
		totalSrc := len(srcLines)

		// Line numbers gutter
		gutterW := len(fmt.Sprintf("%d", totalSrc))
		gutterTotal := gutterW + 1 // digits + space
		contentW := previewWidth - gutterTotal
		if contentW < 10 {
			contentW = 10
		}

		// Wrap all source lines into visual lines
		type visualLine struct {
			text   string
			lineNo int // original line number (0 = continuation)
		}
		var vlines []visualLine
		for i, line := range srcLines {
			wrapped := wrapVisual(line, contentW)
			for j, wl := range wrapped {
				no := 0
				if j == 0 {
					no = i + 1
				}
				vlines = append(vlines, visualLine{text: wl, lineNo: no})
			}
		}
		totalVis := len(vlines)

		// Apply scroll offset (on visual lines)
		start := m.preview.scrollOffset
		if start > totalVis {
			start = totalVis
		}
		end := start + contentHeight
		if end > totalVis {
			end = totalVis
		}
		visible := vlines[start:end]

		// Scroll indicator in header
		if totalVis > contentHeight {
			pct := 100 * end / totalVis
			if pct > 100 {
				pct = 100
			}
			modeLabel += " " + PositionStyle.Render(fmt.Sprintf("%d%%", pct))
		}

		// Build output
		var out strings.Builder
		out.WriteString(AccentStyle.Render(modeLabel))
		for _, vl := range visible {
			out.WriteByte('\n')
			if vl.lineNo > 0 {
				out.WriteString(PositionStyle.Render(fmt.Sprintf("%*d ", gutterW, vl.lineNo)))
			} else {
				out.WriteString(strings.Repeat(" ", gutterTotal)) // continuation indent
			}
			out.WriteString(vl.text)
		}
		// Pad remaining lines to fill height
		for i := len(visible); i < contentHeight; i++ {
			out.WriteByte('\n')
		}

		return lipgloss.NewStyle().
			Width(previewWidth).
			MaxHeight(m.height).
			Render(out.String())
	}

	return lipgloss.NewStyle().
		Width(previewWidth).
		Height(m.height).
		Render(LoadingStyle.Render("No preview"))
}

// --- Layout helpers ---

// columnWidths returns (parent, current, preview) widths.
// Separator chars are accounted for (1 char each).
func (m MillerView) columnWidths() (int, int, int) {
	available := m.width

	// Below minTwoColumnWidth: hide parent column (2-column mode)
	if available < minTwoColumnWidth {
		sepWidth := 1
		usable := available - sepWidth
		// Content-aware split: current gets what it needs, rest to preview
		idealCur := m.current.IdealWidth()
		currentW := min(idealCur, usable*twoColCurrentPct/100) // cap at 50%
		if currentW < minCurrentWidth {
			currentW = minCurrentWidth
		}
		previewW := usable - currentW
		return 0, currentW, previewW
	}

	// 3-column mode: content-aware widths
	sepWidth := 2
	usable := available - sepWidth

	// Calculate ideal widths
	idealParent := m.parent.IdealWidth()
	idealCurrent := m.current.IdealWidth()

	// Parent: fit content, cap at parentMaxPct% of usable
	maxParent := usable * parentMaxPct / 100
	parentW := min(idealParent, maxParent)
	if parentW < minParentWidth {
		parentW = minParentWidth
	}

	// Current: fit content, cap at currentMaxPct% of usable
	maxCurrent := usable * currentMaxPct / 100
	currentW := min(idealCurrent, maxCurrent)
	if currentW < minCurrentWidth {
		currentW = minCurrentWidth
	}

	// Preview: everything else (at least previewMinPct%)
	previewW := usable - parentW - currentW
	minPreview := usable * previewMinPct / 100
	if previewW < minPreview {
		// Shrink parent and current proportionally to give preview enough space
		excess := minPreview - previewW
		parentShrink := excess * parentW / (parentW + currentW)
		currentShrink := excess - parentShrink
		parentW -= parentShrink
		currentW -= currentShrink
		previewW = minPreview
	}

	Trace("miller: columnWidths usable=%d ideal(p=%d,c=%d) result(p=%d,c=%d,pv=%d)",
		usable, idealParent, idealCurrent, parentW, currentW, previewW)
	return parentW, currentW, previewW
}

// mouseZone identifies which column area a mouse event targets.
type mouseZone int

const (
	zoneParent mouseZone = iota
	zoneCurrent
	zonePreview
)

func (m MillerView) handleMouse(msg tea.MouseMsg) (MillerView, tea.Cmd) {
	parentW, currentW, _ := m.columnWidths()

	x := msg.X
	var zone mouseZone
	var localX int

	if parentW > 0 {
		if x < parentW {
			zone = zoneParent
			localX = x
		} else if x < parentW+1+currentW {
			zone = zoneCurrent
			localX = x - parentW - 1
		} else {
			zone = zonePreview
			localX = x - parentW - 1 - currentW - 1
		}
	} else {
		if x < currentW {
			zone = zoneCurrent
			localX = x
		} else {
			zone = zonePreview
			localX = x - currentW - 1
		}
	}

	// Scroll wheel: forward to the targeted column
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		switch zone {
		case zoneParent:
			localMsg := tea.MouseMsg{X: localX, Y: msg.Y, Button: msg.Button, Action: msg.Action}
			m.parent, _ = m.parent.Update(localMsg)
		case zoneCurrent:
			localMsg := tea.MouseMsg{X: localX, Y: msg.Y, Button: msg.Button, Action: msg.Action}
			m.current, _ = m.current.Update(localMsg)
		case zonePreview:
			if m.preview.childColumn != nil {
				localMsg := tea.MouseMsg{X: localX, Y: msg.Y, Button: msg.Button, Action: msg.Action}
				*m.preview.childColumn, _ = m.preview.childColumn.Update(localMsg)
			} else if m.preview.content != "" {
				return m.scrollPreviewContent(msg)
			}
		}
		return m, nil
	}

	// Left click: clicked column becomes center, others shift
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		Trace("miller: click zone=%d x=%d y=%d", zone, msg.X, msg.Y)
		switch zone {
		case zoneParent:
			// Click parent item → go back, then select the clicked item
			clickedIdx := m.parent.HitTestIndex(msg.Y)
			m, _ = m.goBack()
			if clickedIdx >= 0 {
				m.current.SetCursor(clickedIdx)
			}
			// Trigger preview update for newly selected item
			if node := m.current.SelectedNode(); node != nil {
				return m, func() tea.Msg { return CursorChangedMsg{Node: node} }
			}
			return m, nil

		case zoneCurrent:
			// Click current → select item, then drill in if it has children
			clickedIdx := m.current.HitTestIndex(msg.Y)
			if clickedIdx >= 0 {
				m.current.SetCursor(clickedIdx)
				if node := m.current.SelectedNode(); node != nil && len(node.Children) > 0 {
					return m.drillIn()
				}
				// Leaf node: trigger preview update
				if node := m.current.SelectedNode(); node != nil {
					return m, func() tea.Msg { return CursorChangedMsg{Node: node} }
				}
			}
			return m, nil

		case zonePreview:
			// If imagecollection, click a FROM FILE line → open that image in overlay
			if m.preview.childColumn == nil && len(m.preview.imagePaths) > 0 {
				// Y=0 tabbar, Y=1 MDL header, Y=2+ content (0-indexed visual lines)
				clickedVLine := msg.Y - 2
				path := findImagePathAtClick(m.preview.contentLines, m.preview.imagePaths,
					clickedVLine, m.preview.scrollOffset)
				if path != "" {
					return m, func() tea.Msg {
						return OpenImageOverlayMsg{Title: "Image Preview", Paths: []string{path}}
					}
				}
				return m, nil
			}
			// Click preview child item → drill in, then select the clicked item
			if m.preview.childColumn != nil {
				clickedIdx := m.preview.childColumn.HitTestIndex(msg.Y)
				m, _ = m.drillIn()
				if clickedIdx >= 0 {
					m.current.SetCursor(clickedIdx)
				}
				// Trigger preview update for newly selected item
				if node := m.current.SelectedNode(); node != nil {
					return m, func() tea.Msg { return CursorChangedMsg{Node: node} }
				}
			}
			return m, nil
		}
	}

	return m, nil
}

// previewVisualLineCount returns the total number of visual lines after wrapping.
func (m MillerView) previewVisualLineCount() int {
	_, _, previewW := m.columnWidths()
	totalSrc := len(m.preview.contentLines)
	gutterW := len(fmt.Sprintf("%d", totalSrc))
	contentW := previewW - gutterW - 1
	if contentW < 10 {
		contentW = 10
	}
	count := 0
	for _, line := range m.preview.contentLines {
		count += len(wrapVisual(line, contentW))
	}
	return count
}

func (m MillerView) scrollPreviewContent(msg tea.MouseMsg) (MillerView, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	contentHeight := m.height - 1
	total := m.previewVisualLineCount()
	maxScroll := max(0, total-contentHeight)

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.preview.scrollOffset -= mouseScrollStep
		if m.preview.scrollOffset < 0 {
			m.preview.scrollOffset = 0
		}
	case tea.MouseButtonWheelDown:
		m.preview.scrollOffset += mouseScrollStep
		if m.preview.scrollOffset > maxScroll {
			m.preview.scrollOffset = maxScroll
		}
	}
	return m, nil
}

// wrapVisual wraps a string (possibly containing ANSI codes) into lines of at most maxWidth visible characters.
// Returns at least one line (empty string if input is empty).
func wrapVisual(s string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{""}
	}
	if s == "" {
		return []string{""}
	}

	var result []string
	var cur strings.Builder
	visW := 0
	inEsc := false

	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			cur.WriteRune(r)
			continue
		}
		if inEsc {
			cur.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if visW >= maxWidth {
			// End current line with reset, start new line
			cur.WriteString("\x1b[0m")
			result = append(result, cur.String())
			cur.Reset()
			visW = 0
		}
		cur.WriteRune(r)
		visW++
	}
	cur.WriteString("\x1b[0m")
	result = append(result, cur.String())
	return result
}

func (m *MillerView) relayout() {
	parentW, currentW, previewW := m.columnWidths()
	if parentW > 0 {
		m.parent.SetSize(parentW, m.height)
	}
	m.current.SetSize(currentW, m.height)
	if m.preview.childColumn != nil {
		m.preview.childColumn.SetSize(previewW, m.height)
	}
}

func (m *MillerView) updateFocusStyles() {
	m.parent.SetFocused(m.focus == MillerFocusParent)
	m.current.SetFocused(m.focus == MillerFocusCurrent)
}

func (m *MillerView) clearPreview() {
	m.preview.childColumn = nil
	m.preview.content = ""
	m.preview.imagePaths = nil
	m.preview.contentLines = nil
	m.preview.loading = false
	m.preview.scrollOffset = 0
}

func (m *MillerView) focusedColumn() *Column {
	if m.focus == MillerFocusParent {
		return &m.parent
	}
	return &m.current
}

// Breadcrumb returns the current navigation path as a slice of labels.
func (m MillerView) Breadcrumb() []string {
	crumbs := make([]string, 0, len(m.navStack)+1)
	for _, entry := range m.navStack {
		crumbs = append(crumbs, entry.currentTitle)
	}
	if m.currentParent != nil {
		crumbs = append(crumbs, m.currentParent.Label)
	}
	return crumbs
}

// SelectedNode returns the TreeNode under the cursor in the current column.
func (m MillerView) SelectedNode() *TreeNode {
	return m.current.SelectedNode()
}

// --- Utility ---

func treeNodesToItems(nodes []*TreeNode) []ColumnItem {
	items := make([]ColumnItem, len(nodes))
	for i, n := range nodes {
		items[i] = ColumnItem{
			Label:         n.Label,
			Icon:          IconFor(n.Type),
			Type:          n.Type,
			QualifiedName: n.QualifiedName,
			HasChildren:   len(n.Children) > 0,
			Node:          n,
		}
	}
	return items
}

func cloneItems(items []ColumnItem) []ColumnItem {
	cloned := make([]ColumnItem, len(items))
	copy(cloned, items)
	return cloned
}

// findImagePathAtClick maps a clicked visual line (0-indexed relative to content area)
// plus the current scroll offset to an image file path in imagePaths.
// Returns "" if no FROM FILE line is found near the click.
func findImagePathAtClick(contentLines, imagePaths []string, clickedVLine, scrollOffset int) string {
	// Approximate source line: each long FROM FILE line typically wraps to ~2 visual lines.
	// Search a window of ±3 source lines around the estimate to handle wrapping.
	approx := clickedVLine + scrollOffset
	for delta := 0; delta <= 3; delta++ {
		for _, sign := range []int{0, 1, -1} {
			srcIdx := approx + sign*delta
			if srcIdx < 0 || srcIdx >= len(contentLines) {
				continue
			}
			plain := stripAnsi(contentLines[srcIdx])
			i := strings.Index(plain, "FROM FILE '")
			if i == -1 {
				continue
			}
			rest := plain[i+len("FROM FILE '"):]
			end := strings.Index(rest, "'")
			if end == -1 {
				continue
			}
			foundPath := rest[:end]
			for _, p := range imagePaths {
				if p == foundPath {
					return p
				}
			}
		}
	}
	return ""
}
