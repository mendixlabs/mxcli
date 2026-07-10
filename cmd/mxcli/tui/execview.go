package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/JordtenBulte-OLC/mxcli/mdl/formatter"
)

// ExecDoneMsg carries the result of MDL execution.
type ExecDoneMsg struct {
	Output string
	Err    error
}

// execShowResultMsg signals App to show exec result and optionally refresh tree.
type execShowResultMsg struct {
	Content string
	Success bool
}

// OpenExecWithContentMsg requests opening ExecView pre-filled with content.
type OpenExecWithContentMsg struct {
	Content string
}

// execFileLoadedMsg carries the content of a loaded MDL file.
type execFileLoadedMsg struct {
	Path    string
	Content string
	Err     error
}

const execPickerMaxVisible = 10

// ExecView provides a textarea for entering/pasting MDL scripts and executing them.
// It has three modes: editor mode (textarea), file picker mode (path input with completion),
// and preview mode (read-only syntax-highlighted ContentView).
type ExecView struct {
	textarea    textarea.Model
	mxcliPath   string
	projectPath string
	width       int
	height      int
	executing   bool
	flash       string
	loadedPath  string // path of the currently loaded file (for status display)

	// Preview mode (Ctrl+P toggle)
	previewing     bool
	previewContent ContentView

	// File picker state (inline, not a separate View)
	picking        bool
	pathInput      textinput.Model
	pathCandidates []mdlCandidate
	pathCursor     int
	pathScroll     int
}

// mdlCandidate is a filesystem entry shown in the MDL file picker.
type mdlCandidate struct {
	fullPath string
	name     string
	isDir    bool
	isMDL    bool
}

func (c mdlCandidate) icon() string {
	if c.isMDL {
		return "📄"
	}
	if c.isDir {
		return "📁"
	}
	return "·"
}

// NewExecView creates an ExecView with a textarea for MDL input.
func NewExecView(mxcliPath, projectPath string, width, height int) ExecView {
	ta := textarea.New()
	ta.Placeholder = "Paste or type MDL script here...\n\nCtrl+E to execute, Ctrl+O to open file, Esc to close"
	ta.ShowLineNumbers = true
	ta.Focus()
	ta.SetWidth(width - 4)
	ta.SetHeight(height - 6)

	pi := textinput.New()
	pi.Placeholder = "/path/to/script.mdl"
	pi.Prompt = "  File: "
	pi.CharLimit = 500

	return ExecView{
		textarea:    ta,
		pathInput:   pi,
		mxcliPath:   mxcliPath,
		projectPath: projectPath,
		width:       width,
		height:      height,
	}
}

// NewExecViewWithContent creates an ExecView pre-filled with content.
func NewExecViewWithContent(mxcliPath, projectPath string, width, height int, content string) ExecView {
	ev := NewExecView(mxcliPath, projectPath, width, height)
	ev.textarea.SetValue(content)
	return ev
}

// Mode returns ModeExec.
func (ev ExecView) Mode() ViewMode {
	return ModeExec
}

// Hints returns context-sensitive hints.
func (ev ExecView) Hints() []Hint {
	if ev.picking {
		return []Hint{
			{Key: "Tab", Label: "complete"},
			{Key: "Enter", Label: "open"},
			{Key: "Esc", Label: "back"},
		}
	}
	if ev.previewing {
		return []Hint{
			{Key: "Ctrl+P", Label: "edit"},
			{Key: "Ctrl+E", Label: "execute"},
			{Key: "Ctrl+F", Label: "format"},
			{Key: "j/k", Label: "scroll"},
			{Key: "/", Label: "search"},
			{Key: "Esc", Label: "close"},
		}
	}
	return ExecViewHints
}

// StatusInfo returns display data for the status bar.
func (ev ExecView) StatusInfo() StatusInfo {
	if ev.picking {
		return StatusInfo{
			Breadcrumb: []string{"Execute MDL", "Open File"},
			Mode:       "Exec",
		}
	}
	if ev.previewing {
		extra := ""
		if ev.loadedPath != "" {
			extra = filepath.Base(ev.loadedPath)
		}
		return StatusInfo{
			Breadcrumb: []string{"Execute MDL", "Preview"},
			Position:   fmt.Sprintf("L%d/%d", ev.previewContent.YOffset()+1, ev.previewContent.TotalLines()),
			Mode:       "Preview",
			Extra:      extra,
		}
	}
	lines := strings.Count(ev.textarea.Value(), "\n") + 1
	extra := ""
	if ev.loadedPath != "" {
		extra = filepath.Base(ev.loadedPath)
	}
	return StatusInfo{
		Breadcrumb: []string{"Execute MDL"},
		Position:   fmt.Sprintf("L%d", lines),
		Mode:       "Exec",
		Extra:      extra,
	}
}

// Render returns the ExecView rendered string.
// Value receiver is intentional: SetWidth/SetHeight mutate only this copy,
// keeping the authoritative dimensions in Update (same pattern as DiffView).
func (ev ExecView) Render(width, height int) string {
	if ev.picking {
		return ev.renderPicker(width, height)
	}
	if ev.previewing {
		return ev.renderPreview(width, height)
	}

	ev.textarea.SetWidth(width - 4)
	ev.textarea.SetHeight(height - 6)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(AccentColor).Padding(0, 1)
	title := titleStyle.Render("Execute MDL")

	statusLine := ""
	if ev.executing {
		statusLine = lipgloss.NewStyle().Foreground(AccentColor).Render("  Executing...")
	} else if ev.flash != "" {
		statusLine = lipgloss.NewStyle().Foreground(MutedColor).Render("  " + ev.flash)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		ev.textarea.View(),
		statusLine,
	)

	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (ev ExecView) renderPreview(width, height int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(AccentColor).Padding(0, 1)
	modeStyle := lipgloss.NewStyle().Foreground(MutedColor).Italic(true)

	titleLine := lipgloss.JoinHorizontal(lipgloss.Top,
		titleStyle.Render("Execute MDL"),
		modeStyle.Render(" — PREVIEW (Ctrl+P to edit)"),
	)

	previewH := height - 6
	ev.previewContent.SetSize(width-4, previewH)

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		ev.previewContent.View(),
	)

	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (ev ExecView) renderPicker(width, height int) string {
	dimStyle := lipgloss.NewStyle().Foreground(MutedColor)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	mdlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(AccentColor)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Open MDL File") + "\n\n")
	sb.WriteString(ev.pathInput.View() + "\n\n")

	if len(ev.pathCandidates) == 0 {
		sb.WriteString(dimStyle.Render("Type a path, use Tab to complete") + "\n")
	} else {
		end := ev.pathScroll + execPickerMaxVisible
		if end > len(ev.pathCandidates) {
			end = len(ev.pathCandidates)
		}
		if ev.pathScroll > 0 {
			sb.WriteString(dimStyle.Render("  ↑ more above") + "\n")
		}
		for i := ev.pathScroll; i < end; i++ {
			c := ev.pathCandidates[i]
			suffix := ""
			if c.isDir {
				suffix = "/"
			}
			label := c.icon() + "  " + c.name + suffix
			if i == ev.pathCursor {
				if c.isMDL {
					sb.WriteString(mdlStyle.Render("> "+label) + "\n")
				} else {
					sb.WriteString(selectedStyle.Render("> "+label) + "\n")
				}
			} else {
				sb.WriteString(normalStyle.Render("  "+label) + "\n")
			}
		}
		if end < len(ev.pathCandidates) {
			sb.WriteString(dimStyle.Render("  ↓ more below") + "\n")
		}
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("%d items", len(ev.pathCandidates))) + "\n")
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(AccentColor).
		Padding(1, 2).
		Width(min(70, width-4))

	content := boxStyle.Render(sb.String())
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// Update handles input and internal messages.
func (ev ExecView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case ExecDoneMsg:
		ev.executing = false
		content := msg.Output
		if msg.Err != nil {
			content = "-- Error:\n" + msg.Output
		}
		return ev, func() tea.Msg {
			return execShowResultMsg{Content: content, Success: msg.Err == nil}
		}

	case execFileLoadedMsg:
		ev.picking = false
		ev.textarea.Focus()
		if msg.Err != nil {
			ev.flash = fmt.Sprintf("Error: %v", msg.Err)
			return ev, nil
		}
		if msg.Content != "" {
			ev.textarea.SetValue(msg.Content)
			ev.loadedPath = msg.Path
			ev.flash = fmt.Sprintf("Loaded: %s", filepath.Base(msg.Path))
		}
		return ev, nil

	case tea.WindowSizeMsg:
		ev.width = msg.Width
		ev.height = msg.Height
		ev.textarea.SetWidth(msg.Width - 4)
		ev.textarea.SetHeight(msg.Height - 6)
		return ev, nil

	case AgentAutoExecMsg:
		mdlText := strings.TrimSpace(ev.textarea.Value())
		if mdlText == "" {
			return ev, nil
		}
		ev.executing = true
		return ev, ev.executeMDL(mdlText)

	case tea.KeyMsg:
		if ev.executing {
			return ev, nil
		}
		if ev.picking {
			return ev.updatePicker(msg)
		}
		if ev.previewing {
			return ev.updatePreview(msg)
		}
		return ev.updateEditor(msg)
	}

	var cmd tea.Cmd
	ev.textarea, cmd = ev.textarea.Update(msg)
	return ev, cmd
}

func (ev ExecView) updateEditor(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if ev.flash != "" {
			ev.flash = ""
			return ev, nil
		}
		return ev, func() tea.Msg { return PopViewMsg{} }

	case "ctrl+e":
		mdlText := strings.TrimSpace(ev.textarea.Value())
		if mdlText == "" {
			ev.flash = "Nothing to execute"
			return ev, nil
		}
		ev.executing = true
		return ev, ev.executeMDL(mdlText)

	case "ctrl+f":
		content := ev.textarea.Value()
		if strings.TrimSpace(content) == "" {
			ev.flash = "Nothing to format"
			return ev, nil
		}
		formatted := formatter.Format(content)
		ev.textarea.SetValue(formatted)
		ev.flash = "Formatted"
		return ev, nil

	case "ctrl+o":
		ev.picking = true
		ev.textarea.Blur()
		ev.pathInput.SetValue("")
		ev.pathCandidates = nil
		ev.pathCursor = 0
		ev.pathScroll = 0
		// Start from working directory
		cwd, _ := os.Getwd()
		ev.pathInput.SetValue(cwd + string(os.PathSeparator))
		ev.pathInput.CursorEnd()
		ev.pathInput.Focus()
		ev.refreshMDLCandidates()
		return ev, nil

	case "ctrl+p":
		content := ev.textarea.Value()
		highlighted := HighlightMDL(content)
		ev.previewContent = NewContentView(ev.width-4, ev.height-6)
		ev.previewContent.SetContent(highlighted)
		ev.previewing = true
		ev.textarea.Blur()
		return ev, nil
	}

	var cmd tea.Cmd
	ev.textarea, cmd = ev.textarea.Update(msg)
	return ev, cmd
}

func (ev ExecView) updatePreview(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "ctrl+p":
		ev.previewing = false
		ev.textarea.Focus()
		return ev, nil

	case "esc":
		return ev, func() tea.Msg { return PopViewMsg{} }

	case "ctrl+e":
		mdlText := strings.TrimSpace(ev.textarea.Value())
		if mdlText == "" {
			ev.flash = "Nothing to execute"
			ev.previewing = false
			ev.textarea.Focus()
			return ev, nil
		}
		ev.executing = true
		ev.previewing = false
		ev.textarea.Focus()
		return ev, ev.executeMDL(mdlText)

	case "ctrl+f":
		content := ev.textarea.Value()
		if strings.TrimSpace(content) == "" {
			return ev, nil
		}
		formatted := formatter.Format(content)
		ev.textarea.SetValue(formatted)
		highlighted := HighlightMDL(formatted)
		ev.previewContent = NewContentView(ev.width-4, ev.height-6)
		ev.previewContent.SetContent(highlighted)
		ev.flash = "Formatted"
		return ev, nil

	default:
		var cmd tea.Cmd
		ev.previewContent, cmd = ev.previewContent.Update(msg)
		return ev, cmd
	}
}

func (ev ExecView) updatePicker(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "esc":
		ev.picking = false
		ev.textarea.Focus()
		return ev, nil

	case "up":
		ev.pickerCursorUp()
		return ev, nil

	case "down":
		ev.pickerCursorDown()
		return ev, nil

	case "tab":
		if len(ev.pathCandidates) > 0 {
			ev.applyMDLCandidate()
		}
		return ev, nil

	case "enter":
		if len(ev.pathCandidates) > 0 {
			c := ev.pathCandidates[ev.pathCursor]
			if c.isMDL {
				// Load the file
				return ev, ev.loadFile(c.fullPath)
			}
			// Directory: drill in
			ev.applyMDLCandidate()
			return ev, nil
		}
		// Try loading whatever path is in the input
		val := strings.TrimSpace(ev.pathInput.Value())
		if val != "" && strings.HasSuffix(strings.ToLower(val), ".mdl") {
			return ev, ev.loadFile(val)
		}
		return ev, nil

	default:
		var cmd tea.Cmd
		ev.pathInput, cmd = ev.pathInput.Update(msg)
		ev.pathCursor = 0
		ev.pathScroll = 0
		ev.refreshMDLCandidates()
		return ev, cmd
	}
}

// refreshMDLCandidates lists filesystem entries, showing directories and .mdl files.
func (ev *ExecView) refreshMDLCandidates() {
	val := strings.TrimSpace(ev.pathInput.Value())
	if val == "" {
		ev.pathCandidates = nil
		return
	}

	dir := val
	prefix := ""
	if !strings.HasSuffix(val, string(os.PathSeparator)) {
		dir = filepath.Dir(val)
		prefix = strings.ToLower(filepath.Base(val))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		ev.pathCandidates = nil
		return
	}

	var candidates []mdlCandidate
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // skip hidden files
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), prefix) {
			continue
		}
		full := filepath.Join(dir, name)
		isMDL := !e.IsDir() && strings.HasSuffix(strings.ToLower(name), ".mdl")
		// Show directories and .mdl files only
		if !e.IsDir() && !isMDL {
			continue
		}
		candidates = append(candidates, mdlCandidate{
			fullPath: full,
			name:     name,
			isDir:    e.IsDir(),
			isMDL:    isMDL,
		})
	}
	ev.pathCandidates = candidates
	if ev.pathCursor >= len(candidates) {
		ev.pathCursor = 0
		ev.pathScroll = 0
	}
}

func (ev *ExecView) pickerCursorDown() {
	if len(ev.pathCandidates) == 0 {
		return
	}
	ev.pathCursor++
	if ev.pathCursor >= len(ev.pathCandidates) {
		ev.pathCursor = 0
		ev.pathScroll = 0
	} else if ev.pathCursor >= ev.pathScroll+execPickerMaxVisible {
		ev.pathScroll = ev.pathCursor - execPickerMaxVisible + 1
	}
}

func (ev *ExecView) pickerCursorUp() {
	if len(ev.pathCandidates) == 0 {
		return
	}
	ev.pathCursor--
	if ev.pathCursor < 0 {
		ev.pathCursor = len(ev.pathCandidates) - 1
		ev.pathScroll = max(0, ev.pathCursor-execPickerMaxVisible+1)
	} else if ev.pathCursor < ev.pathScroll {
		ev.pathScroll = ev.pathCursor
	}
}

func (ev *ExecView) applyMDLCandidate() {
	if len(ev.pathCandidates) == 0 {
		return
	}
	c := ev.pathCandidates[ev.pathCursor]
	if c.isDir {
		ev.pathInput.SetValue(c.fullPath + string(os.PathSeparator))
	} else {
		ev.pathInput.SetValue(c.fullPath)
	}
	ev.pathInput.CursorEnd()
	ev.pathCursor = 0
	ev.pathScroll = 0
	ev.refreshMDLCandidates()
}

// loadFile reads a file and returns execFileLoadedMsg.
func (ev ExecView) loadFile(path string) tea.Cmd {
	return func() tea.Msg {
		content, err := os.ReadFile(path)
		if err != nil {
			return execFileLoadedMsg{Err: fmt.Errorf("read %s: %w", path, err)}
		}
		return execFileLoadedMsg{Path: path, Content: string(content)}
	}
}

// executeMDL writes MDL to a temp file and runs `mxcli exec`.
func (ev ExecView) executeMDL(mdlText string) tea.Cmd {
	mxcliPath := ev.mxcliPath
	projectPath := ev.projectPath
	return func() tea.Msg {
		tmpFile, err := os.CreateTemp("", "mxcli-exec-*.mdl")
		if err != nil {
			return ExecDoneMsg{Output: fmt.Sprintf("Failed to create temp file: %v", err), Err: err}
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.WriteString(mdlText); err != nil {
			tmpFile.Close()
			return ExecDoneMsg{Output: fmt.Sprintf("Failed to write temp file: %v", err), Err: err}
		}
		tmpFile.Close()

		args := []string{"exec"}
		if projectPath != "" {
			args = append(args, "-p", projectPath)
		}
		args = append(args, tmpPath)
		out, err := runMxcli(mxcliPath, args...)
		return ExecDoneMsg{Output: out, Err: err}
	}
}
