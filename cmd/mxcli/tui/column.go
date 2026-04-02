package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const mouseScrollStep = 3

// TreeNode mirrors cmd/mxcli.TreeNode for JSON parsing.
type TreeNode struct {
	Label         string      `json:"label"`
	Type          string      `json:"type"`
	QualifiedName string      `json:"qualifiedName,omitempty"`
	Children      []*TreeNode `json:"children,omitempty"`
}

// ColumnItem represents a single row in a Column.
type ColumnItem struct {
	Label         string
	Icon          string
	Type          string // Mendix node type
	QualifiedName string
	HasChildren   bool
	Node          *TreeNode
}

// FilterState manages the inline filter for a Column.
type FilterState struct {
	active        bool
	input         textinput.Model
	query         string
	matches       []int  // indices into items that match the query
	historyCursor int    // -1 = editing new query, 0..n = browsing history
	draft         string // user's typed text before browsing history
}

// CursorChangedMsg is emitted when the cursor moves to a different item.
type CursorChangedMsg struct {
	Node *TreeNode
}

// Column is a generic scrollable list with filtering, mouse support, and
// a visual scrollbar. It is the building block for Miller columns.
// Borders and separators are NOT rendered by Column — the parent (miller.go)
// handles those.
type Column struct {
	items        []ColumnItem
	cursor       int
	scrollOffset int
	filter       FilterState
	width        int
	height       int
	title        string
	focused      bool
}

// NewColumn creates a Column with the given title.
func NewColumn(title string) Column {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.CharLimit = 200
	return Column{
		title:  title,
		filter: FilterState{input: ti},
	}
}

// SetItems replaces the column items and resets cursor/scroll/filter state.
func (c *Column) SetItems(items []ColumnItem) {
	c.items = items
	c.cursor = 0
	c.scrollOffset = 0
	c.filter.query = ""
	c.filter.input.SetValue("")
	c.filter.input.Blur()
	c.filter.active = false
	c.rebuildFiltered()
}

// SetSize updates the column dimensions.
func (c *Column) SetSize(w, h int) {
	c.width = w
	c.height = h
}

// SetFocused sets the visual focus state.
func (c *Column) SetFocused(focused bool) {
	c.focused = focused
}

// SelectedItem returns the currently selected item, or nil if empty.
func (c Column) SelectedItem() *ColumnItem {
	idx := c.selectedIndex()
	if idx < 0 {
		return nil
	}
	return &c.items[idx]
}

// SelectedNode returns the TreeNode for the selected item, or nil.
func (c Column) SelectedNode() *TreeNode {
	item := c.SelectedItem()
	if item == nil {
		return nil
	}
	return item.Node
}

// IsFilterActive returns true if the filter input is currently focused.
func (c Column) IsFilterActive() bool {
	return c.filter.active
}

// Title returns the column title.
func (c Column) Title() string {
	return c.title
}

// SetTitle updates the column title.
func (c *Column) SetTitle(title string) {
	c.title = title
}

// ItemCount returns the number of visible (filtered) items.
func (c Column) ItemCount() int {
	return len(c.filter.matches)
}

// --- Update ---

// Update handles keyboard and mouse messages, returning a command if the
// cursor moved to a new item.
func (c Column) Update(msg tea.Msg) (Column, tea.Cmd) {
	prevCursor := c.cursor

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if c.filter.active {
			return c.updateFilter(msg)
		}
		c.handleKey(msg)

	case tea.MouseMsg:
		c.handleMouse(msg)
	}

	if c.cursor != prevCursor {
		node := c.SelectedNode()
		if node != nil {
			return c, func() tea.Msg { return CursorChangedMsg{Node: node} }
		}
	}
	return c, nil
}

func (c *Column) handleKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "j", "down":
		c.moveCursorDown()
	case "k", "up":
		c.moveCursorUp()
	case "/":
		c.activateFilter()
	case "G":
		c.moveCursorToEnd()
	case "g":
		c.moveCursorToStart()
	}
}

func (c Column) updateFilter(msg tea.KeyMsg) (Column, tea.Cmd) {
	switch msg.String() {
	case "esc":
		c.deactivateFilter()
		return c, nil
	case "enter":
		// Lock filter results, exit input mode
		AddFilterHistoryEntry(c.filter.query)
		c.filter.active = false
		c.filter.input.Blur()
		return c, nil
	case "up":
		// Browse filter history when input is focused
		entries := LoadFilterHistory()
		if len(entries) > 0 && c.filter.historyCursor < len(entries)-1 {
			if c.filter.historyCursor == -1 {
				c.filter.draft = c.filter.input.Value()
			}
			c.filter.historyCursor++
			c.filter.input.SetValue(entries[c.filter.historyCursor])
			c.filter.query = entries[c.filter.historyCursor]
			c.rebuildFiltered()
			return c, nil
		}
		// Fall through to cursor movement if no more history
		c.moveCursorUp()
		return c, nil
	case "down":
		if c.filter.historyCursor > 0 {
			c.filter.historyCursor--
			entries := LoadFilterHistory()
			c.filter.input.SetValue(entries[c.filter.historyCursor])
			c.filter.query = entries[c.filter.historyCursor]
			c.rebuildFiltered()
			return c, nil
		} else if c.filter.historyCursor == 0 {
			c.filter.historyCursor = -1
			c.filter.input.SetValue(c.filter.draft)
			c.filter.query = c.filter.draft
			c.rebuildFiltered()
			return c, nil
		}
		c.moveCursorDown()
		return c, nil
	default:
		// Reset history browsing on any text input
		c.filter.historyCursor = -1
		var cmd tea.Cmd
		c.filter.input, cmd = c.filter.input.Update(msg)
		c.filter.query = c.filter.input.Value()
		c.rebuildFiltered()
		return c, cmd
	}
}

func (c *Column) handleMouse(msg tea.MouseMsg) {
	if msg.Action != tea.MouseActionPress {
		return
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		c.scrollUp(mouseScrollStep)
	case tea.MouseButtonWheelDown:
		c.scrollDown(mouseScrollStep)
	case tea.MouseButtonLeft:
		topOffset := c.headerLines()
		clicked := c.scrollOffset + (msg.Y - topOffset)
		if clicked >= 0 && clicked < len(c.filter.matches) {
			c.cursor = clicked
		}
	}
}

// --- View ---

// View renders the column content: title, optional filter bar, items, and scrollbar.
func (c Column) View() string {
	var sb strings.Builder

	// Title — use accent style when focused
	titleStyle := ColumnTitleStyle
	if c.focused {
		titleStyle = FocusedTitleStyle
	}
	sb.WriteString(titleStyle.Render(c.title))
	sb.WriteString("\n")

	// Filter bar
	filterLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	if c.filter.active {
		sb.WriteString(c.filter.input.View())
		sb.WriteString("\n")
	} else if c.filter.query != "" {
		sb.WriteString(filterLabelStyle.Render("Filter: " + c.filter.query))
		sb.WriteString("\n")
	}

	total := len(c.filter.matches)
	maxVis := c.maxVisible()

	// Reserve 1 column for scrollbar when needed
	contentWidth := c.width
	showScrollbar := total > maxVis
	if showScrollbar {
		contentWidth--
	}
	// Reserve 1 column for focus edge indicator
	if c.focused {
		contentWidth--
	}
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Scrollbar thumb position
	var thumbStart, thumbEnd int
	if showScrollbar {
		trackHeight := maxVis
		if total <= maxVis {
			thumbStart = 0
			thumbEnd = trackHeight
		} else {
			thumbSize := max(1, trackHeight*maxVis/total)
			maxOffset := total - maxVis
			thumbStart = c.scrollOffset * (trackHeight - thumbSize) / maxOffset
			thumbEnd = thumbStart + thumbSize
		}
	}

	scrollThumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	scrollTrackStyle := lipgloss.NewStyle().Faint(true)
	edgeChar := FocusedEdgeStyle.Render(FocusedEdgeChar)

	for vi := range maxVis {
		idx := c.scrollOffset + vi
		var line string
		if idx < total {
			itemIdx := c.filter.matches[idx]
			item := c.items[itemIdx]

			icon := item.Icon
			if icon == "" {
				icon = IconFor(item.Type)
			}
			label := icon + "  " + item.Label

			// Directory indicator
			if item.HasChildren {
				label += " ▶"
			}

			// Truncate to fit (rune-aware to avoid breaking multi-byte characters)
			if lipgloss.Width(label) > contentWidth-2 {
				label = runewidth.Truncate(label, contentWidth-2, "")
			}

			if idx == c.cursor {
				line = SelectedItemStyle.Render(label)
			} else if item.HasChildren {
				line = DirectoryStyle.Render(label)
			} else {
				line = LeafStyle.Render(label)
			}
		}

		// Focus edge indicator
		if c.focused {
			line = edgeChar + line
		}

		// Pad to contentWidth (plus edge char width)
		targetWidth := contentWidth
		if c.focused {
			targetWidth++ // account for the 1-char edge prefix
		}
		lineWidth := lipgloss.Width(line)
		if lineWidth < targetWidth {
			line += strings.Repeat(" ", targetWidth-lineWidth)
		}

		// Scrollbar
		if showScrollbar {
			if vi >= thumbStart && vi < thumbEnd {
				line += scrollThumbStyle.Render("█")
			} else {
				line += scrollTrackStyle.Render("│")
			}
		}

		sb.WriteString(line)
		if vi < maxVis-1 {
			sb.WriteString("\n")
		}
	}

	result := sb.String()
	if !c.focused {
		result = lipgloss.NewStyle().Faint(true).Render(result)
	}
	return result
}

// --- Helpers ---

func (c Column) selectedIndex() int {
	if len(c.filter.matches) == 0 || c.cursor >= len(c.filter.matches) {
		return -1
	}
	return c.filter.matches[c.cursor]
}

// SetCursor moves the cursor to the given index, clamping to valid range.
func (c *Column) SetCursor(idx int) {
	total := len(c.filter.matches)
	if total == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= total {
		idx = total - 1
	}
	c.cursor = idx
	// Adjust scroll to keep cursor visible
	maxVis := c.maxVisible()
	if c.cursor < c.scrollOffset {
		c.scrollOffset = c.cursor
	}
	if c.cursor >= c.scrollOffset+maxVis {
		c.scrollOffset = c.cursor - maxVis + 1
	}
}

// IdealWidth returns the minimum width needed to show all items without truncation.
// Uses visual width (lipgloss.Width) to handle multibyte icons correctly.
func (c Column) IdealWidth() int {
	maxW := lipgloss.Width(c.title)
	for _, item := range c.items {
		icon := item.Icon
		if icon == "" {
			icon = IconFor(item.Type)
		}
		label := icon + "  " + item.Label
		if item.HasChildren {
			label += " ▶"
		}
		w := lipgloss.Width(label)
		if w > maxW {
			maxW = w
		}
	}
	return maxW + 2 // +2 for padding
}

// HitTestIndex returns the item index at the given Y coordinate, or -1.
func (c Column) HitTestIndex(y int) int {
	topOffset := c.headerLines()
	idx := c.scrollOffset + (y - topOffset)
	if idx >= 0 && idx < len(c.filter.matches) {
		return idx
	}
	return -1
}

func (c Column) headerLines() int {
	lines := 1 // title
	if c.filter.active || c.filter.query != "" {
		lines++
	}
	return lines
}

func (c Column) maxVisible() int {
	visible := c.height - c.headerLines()
	if visible < 1 {
		return 1
	}
	return visible
}

func (c *Column) rebuildFiltered() {
	c.filter.matches = c.filter.matches[:0]
	query := strings.ToLower(strings.TrimSpace(c.filter.query))
	for i, item := range c.items {
		if query == "" || strings.Contains(strings.ToLower(item.Label), query) {
			c.filter.matches = append(c.filter.matches, i)
		}
	}
	if c.cursor >= len(c.filter.matches) {
		c.cursor = max(0, len(c.filter.matches)-1)
	}
	c.clampScroll()
}

func (c *Column) clampScroll() {
	maxVis := c.maxVisible()
	total := len(c.filter.matches)
	if c.scrollOffset > total-maxVis {
		c.scrollOffset = max(0, total-maxVis)
	}
	if c.scrollOffset < 0 {
		c.scrollOffset = 0
	}
}

func (c *Column) moveCursorDown() {
	total := len(c.filter.matches)
	if total == 0 {
		return
	}
	c.cursor++
	if c.cursor >= total {
		c.cursor = 0
		c.scrollOffset = 0
		return
	}
	maxVis := c.maxVisible()
	if c.cursor >= c.scrollOffset+maxVis {
		c.scrollOffset = c.cursor - maxVis + 1
	}
}

func (c *Column) moveCursorUp() {
	total := len(c.filter.matches)
	if total == 0 {
		return
	}
	c.cursor--
	if c.cursor < 0 {
		c.cursor = total - 1
		c.scrollOffset = max(0, c.cursor-c.maxVisible()+1)
		return
	}
	if c.cursor < c.scrollOffset {
		c.scrollOffset = c.cursor
	}
}

func (c *Column) moveCursorToStart() {
	c.cursor = 0
	c.scrollOffset = 0
}

func (c *Column) moveCursorToEnd() {
	total := len(c.filter.matches)
	if total > 0 {
		c.cursor = total - 1
		c.scrollOffset = max(0, total-c.maxVisible())
	}
}

func (c *Column) scrollUp(n int) {
	c.scrollOffset -= n
	if c.scrollOffset < 0 {
		c.scrollOffset = 0
	}
	if c.cursor >= c.scrollOffset+c.maxVisible() {
		c.cursor = c.scrollOffset + c.maxVisible() - 1
	}
}

func (c *Column) scrollDown(n int) {
	total := len(c.filter.matches)
	maxVis := c.maxVisible()
	c.scrollOffset += n
	if c.scrollOffset > total-maxVis {
		c.scrollOffset = max(0, total-maxVis)
	}
	if c.cursor < c.scrollOffset {
		c.cursor = c.scrollOffset
	}
}

func (c *Column) activateFilter() {
	c.filter.active = true
	c.filter.input.SetValue("")
	c.filter.query = ""
	c.filter.historyCursor = -1
	c.filter.draft = ""
	c.filter.input.Focus()
	c.rebuildFiltered()
}

func (c *Column) deactivateFilter() {
	c.filter.active = false
	c.filter.query = ""
	c.filter.historyCursor = -1
	c.filter.draft = ""
	c.filter.input.SetValue("")
	c.filter.input.Blur()
	c.rebuildFiltered()
}
