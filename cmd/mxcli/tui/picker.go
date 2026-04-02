// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const pickerMaxVisible = 8

// pathCandidate is a filesystem entry shown during path completion.
type pathCandidate struct {
	fullPath    string
	name        string
	isDir       bool
	isMPR       bool
	isMendixDir bool // directory that contains at least one .mpr file
	enriched    bool // whether isMendixDir has been checked
}

func (c pathCandidate) icon() string {
	switch {
	case c.isMPR:
		return "⬡"
	case c.isMendixDir:
		return "⬡"
	case c.isDir:
		return "📁"
	default:
		return "·"
	}
}

// dirContainsMPR returns true if dir has at least one .mpr file (non-recursive).
func dirContainsMPR(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mpr") {
			return true
		}
	}
	return false
}

// dirSingleMPR returns the single .mpr path if the directory contains exactly one .mpr file.
func dirSingleMPR(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var found string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mpr") {
			if found != "" {
				return "" // more than one
			}
			found = filepath.Join(dir, e.Name())
		}
	}
	return found
}

// listPathCandidates lists filesystem entries matching the current input prefix.
func listPathCandidates(input string) []pathCandidate {
	dir := input
	prefix := ""

	if !strings.HasSuffix(input, string(os.PathSeparator)) {
		dir = filepath.Dir(input)
		prefix = strings.ToLower(filepath.Base(input))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var candidates []pathCandidate
	for _, e := range entries {
		if prefix != "" && !strings.HasPrefix(strings.ToLower(e.Name()), prefix) {
			continue
		}
		full := filepath.Join(dir, e.Name())
		isMPR := !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mpr")
		candidates = append(candidates, pathCandidate{
			fullPath: full,
			name:     e.Name(),
			isDir:    e.IsDir(),
			isMPR:    isMPR,
		})
	}
	return candidates
}

// PickerDoneMsg is sent when the picker is embedded in App and user selects a project.
type PickerDoneMsg struct {
	Path string // empty if cancelled
}

// PickerModel lets the user select from recent projects or type a new path.
type PickerModel struct {
	history             []string
	cursor              int
	historyScrollOffset int

	input            textinput.Model
	inputMode        bool
	pathCandidates   []pathCandidate
	pathCursor       int
	pathScrollOffset int

	chosen   string
	done     bool
	embedded bool // when true, send PickerDoneMsg instead of tea.Quit
	width    int
	height   int
}

// NewPickerModel creates the picker model with loaded history.
func NewPickerModel() PickerModel {
	ti := textinput.New()
	ti.Placeholder = "/path/to/App.mpr"
	ti.Prompt = "  Path: "
	ti.CharLimit = 500

	return PickerModel{
		history: LoadHistory(),
		input:   ti,
	}
}

// NewEmbeddedPicker creates a picker for use within App (sends PickerDoneMsg, not tea.Quit).
func NewEmbeddedPicker() PickerModel {
	p := NewPickerModel()
	p.embedded = true
	return p
}

// Chosen returns the selected project path (empty if cancelled).
func (m PickerModel) Chosen() string {
	return m.chosen
}

// doneCmd returns the appropriate tea.Cmd for picker completion.
func (m PickerModel) doneCmd() tea.Cmd {
	if m.embedded {
		path := m.chosen
		return func() tea.Msg { return PickerDoneMsg{Path: path} }
	}
	return tea.Quit
}

func (m PickerModel) Init() tea.Cmd {
	return nil
}

// enrichVisible populates isMendixDir only for candidates within the visible window.
func (m *PickerModel) enrichVisible() {
	end := m.pathScrollOffset + pickerMaxVisible
	if end > len(m.pathCandidates) {
		end = len(m.pathCandidates)
	}
	for i := m.pathScrollOffset; i < end; i++ {
		c := &m.pathCandidates[i]
		if c.isDir && !c.enriched {
			c.isMendixDir = dirContainsMPR(c.fullPath)
			c.enriched = true
		}
	}
}

func (m *PickerModel) refreshCandidates() {
	val := strings.TrimSpace(m.input.Value())
	if val == "" {
		m.pathCandidates = nil
		m.pathCursor = 0
		m.pathScrollOffset = 0
		return
	}
	m.pathCandidates = listPathCandidates(val)
	if m.pathCursor >= len(m.pathCandidates) {
		m.pathCursor = 0
		m.pathScrollOffset = 0
	}
	m.enrichVisible()
}

func (m *PickerModel) pathCursorDown() {
	if len(m.pathCandidates) == 0 {
		return
	}
	m.pathCursor++
	if m.pathCursor >= len(m.pathCandidates) {
		m.pathCursor = 0
		m.pathScrollOffset = 0
	} else if m.pathCursor >= m.pathScrollOffset+pickerMaxVisible {
		m.pathScrollOffset = m.pathCursor - pickerMaxVisible + 1
	}
	m.enrichVisible()
}

func (m *PickerModel) pathCursorUp() {
	if len(m.pathCandidates) == 0 {
		return
	}
	m.pathCursor--
	if m.pathCursor < 0 {
		m.pathCursor = len(m.pathCandidates) - 1
		m.pathScrollOffset = max(0, m.pathCursor-pickerMaxVisible+1)
	} else if m.pathCursor < m.pathScrollOffset {
		m.pathScrollOffset = m.pathCursor
	}
	m.enrichVisible()
}

func (m *PickerModel) applyCandidate() bool {
	if len(m.pathCandidates) == 0 {
		return false
	}
	c := m.pathCandidates[m.pathCursor]
	if c.isDir {
		// If the directory has exactly one .mpr, open it directly
		if single := dirSingleMPR(c.fullPath); single != "" {
			m.chosen = single
			m.done = true
			return true
		}
		m.input.SetValue(c.fullPath + string(os.PathSeparator))
	} else {
		m.input.SetValue(c.fullPath)
	}
	m.input.CursorEnd()
	m.pathCursor = 0
	m.pathScrollOffset = 0
	m.refreshCandidates()
	return false
}

func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.inputMode {
			switch msg.String() {
			case "esc":
				m.inputMode = false
				m.pathCandidates = nil
				m.pathCursor = 0
				m.pathScrollOffset = 0
				m.input.Blur()
				return m, nil

			case "tab":
				if len(m.pathCandidates) > 0 {
					if done := m.applyCandidate(); done {
						return m, m.doneCmd()
					}
				}
				return m, nil

			case "up":
				m.pathCursorUp()
				return m, nil

			case "down":
				m.pathCursorDown()
				return m, nil

			case "enter":
				if len(m.pathCandidates) > 0 {
					if done := m.applyCandidate(); done {
						return m, m.doneCmd()
					}
					// If it was a dir without unique mpr, stay in input mode
					if len(m.pathCandidates) > 0 {
						return m, nil
					}
				}
				val := strings.TrimSpace(m.input.Value())
				if val != "" {
					m.chosen = val
					m.done = true
					return m, m.doneCmd()
				}

			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				m.pathCursor = 0
				m.pathScrollOffset = 0
				m.refreshCandidates()
				return m, cmd
			}
		} else {
			switch msg.String() {
			case "ctrl+c", "q":
				m.done = true
				return m, m.doneCmd()

			case "j", "down":
				if m.cursor < len(m.history)-1 {
					m.cursor++
					if m.cursor >= m.historyScrollOffset+pickerMaxVisible {
						m.historyScrollOffset = m.cursor - pickerMaxVisible + 1
					}
				}

			case "k", "up":
				if m.cursor > 0 {
					m.cursor--
					if m.cursor < m.historyScrollOffset {
						m.historyScrollOffset = m.cursor
					}
				}

			case "enter":
				if len(m.history) > 0 {
					m.chosen = m.history[m.cursor]
					m.done = true
					return m, m.doneCmd()
				}

			case "n":
				m.inputMode = true
				m.input.SetValue("")
				m.pathCandidates = nil
				m.pathCursor = 0
				m.pathScrollOffset = 0
				m.input.Focus()
				return m, nil
			}
		}
	}
	return m, nil
}

func (m PickerModel) View() string {
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	highlightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	mprStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Select Mendix Project") + "\n\n")

	if !m.inputMode {
		if len(m.history) == 0 {
			sb.WriteString(dimStyle.Render("No recent projects.") + "\n\n")
		} else {
			sb.WriteString(dimStyle.Render("Recent projects:") + "\n")
			end := m.historyScrollOffset + pickerMaxVisible
			if end > len(m.history) {
				end = len(m.history)
			}
			if m.historyScrollOffset > 0 {
				sb.WriteString(dimStyle.Render("  ↑ more above") + "\n")
			}
			for i := m.historyScrollOffset; i < end; i++ {
				path := m.history[i]
				if i == m.cursor {
					sb.WriteString(selectedStyle.Render("> ⬡  "+path) + "\n")
				} else {
					sb.WriteString(normalStyle.Render("  ⬡  "+path) + "\n")
				}
			}
			if end < len(m.history) {
				sb.WriteString(dimStyle.Render("  ↓ more below") + "\n")
			}
			sb.WriteString("\n")
		}
		hint := "[n] new path  [q] quit"
		if len(m.history) > 0 {
			hint = "[j/k] navigate  [Enter] open  [n] new path  [q] quit"
		}
		sb.WriteString(dimStyle.Render(hint) + "\n")
	} else {
		sb.WriteString(m.input.View() + "\n")

		if len(m.pathCandidates) == 0 {
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render("[Tab] complete  [Enter] confirm  [Esc] back") + "\n")
		} else {
			sb.WriteString("\n")
			end := m.pathScrollOffset + pickerMaxVisible
			if end > len(m.pathCandidates) {
				end = len(m.pathCandidates)
			}
			if m.pathScrollOffset > 0 {
				sb.WriteString(dimStyle.Render("  ↑ more above") + "\n")
			}
			for i := m.pathScrollOffset; i < end; i++ {
				c := m.pathCandidates[i]
				icon := c.icon()
				suffix := ""
				if c.isDir {
					suffix = "/"
				}
				label := icon + "  " + c.name + suffix
				if i == m.pathCursor {
					if c.isMPR || c.isMendixDir {
						sb.WriteString(mprStyle.Render("> "+label) + "\n")
					} else {
						sb.WriteString(highlightStyle.Render("> "+label) + "\n")
					}
				} else {
					sb.WriteString(normalStyle.Render("  "+label) + "\n")
				}
			}
			if end < len(m.pathCandidates) {
				sb.WriteString(dimStyle.Render("  ↓ more below") + "\n")
			}
			sb.WriteString("\n")
			sb.WriteString(dimStyle.Render("[↑↓] navigate  [Tab/Enter] open  [Esc] back") + "\n")
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, 2).
		Width(70)

	content := boxStyle.Render(sb.String())

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	return content
}
