package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mendixlabs/mxcli/mdl/formatter"
)

// overlayContentMsg carries reloaded content for an OverlayView after Tab switch.
type overlayContentMsg struct {
	Title   string
	Content string
}

// OverlayViewOpts holds optional configuration for an OverlayView.
type OverlayViewOpts struct {
	QName           string
	NodeType        string
	IsNDSL          bool
	Switchable      bool
	MxcliPath       string
	ProjectPath     string
	HideLineNumbers bool
	Refreshable     bool               // show "r" hint and allow re-triggering via RefreshMsg
	RefreshMsg      tea.Msg            // message to send when "r" is pressed
	CheckAnchors    string             // pre-rendered LLM anchor text for check overlays
	CheckFilter     string             // severity filter: "all", "error", "warning", "deprecation"
	CheckErrors     []CheckError       // stored errors for re-rendering with different filter
	CheckNavLocs    []CheckNavLocation // navigable document locations for selection
}

// OverlayView wraps an Overlay to satisfy the View interface,
// adding BSON/MDL switching and self-contained content reload.
type OverlayView struct {
	overlay      Overlay
	qname        string
	nodeType     string
	isNDSL       bool
	switchable   bool
	mxcliPath    string
	projectPath  string
	refreshable  bool
	refreshMsg   tea.Msg
	checkAnchors string             // LLM-structured anchor text, replaces generic anchor when set
	checkFilter  string             // severity filter for check overlays: "all", "error", "warning", "deprecation"
	checkErrors  []CheckError       // stored check errors for re-rendering with different filter
	checkNavLocs []CheckNavLocation // navigable document locations
	selectedIdx  int                // cursor index into checkNavLocs (-1 = none)
	pendingKey   rune               // ']' or '[' waiting for 'e', 0 if none
}

// NewOverlayView creates an OverlayView with the given title, content, dimensions, and options.
func NewOverlayView(title, content string, width, height int, opts OverlayViewOpts) OverlayView {
	checkFilter := opts.CheckFilter
	if checkFilter == "" {
		checkFilter = "all"
	}
	selectedIdx := -1
	if len(opts.CheckNavLocs) > 0 {
		selectedIdx = 0
	}
	ov := OverlayView{
		qname:        opts.QName,
		nodeType:     opts.NodeType,
		isNDSL:       opts.IsNDSL,
		switchable:   opts.Switchable,
		mxcliPath:    opts.MxcliPath,
		projectPath:  opts.ProjectPath,
		refreshable:  opts.Refreshable,
		refreshMsg:   opts.RefreshMsg,
		checkAnchors: opts.CheckAnchors,
		checkFilter:  checkFilter,
		checkErrors:  opts.CheckErrors,
		checkNavLocs: opts.CheckNavLocs,
		selectedIdx:  selectedIdx,
	}
	ov.overlay = NewOverlay()
	ov.overlay.switchable = opts.Switchable
	ov.overlay.refreshable = opts.Refreshable
	ov.overlay.Show(title, content, width, height)
	if opts.HideLineNumbers {
		ov.overlay.content.hideLineNumbers = true
	}
	return ov
}

// Update handles input and internal messages.
func (ov OverlayView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case overlayContentMsg:
		ov.overlay.Show(msg.Title, msg.Content, ov.overlay.width, ov.overlay.height)
		return ov, nil

	case tea.KeyMsg:
		if ov.overlay.content.IsSearching() {
			var cmd tea.Cmd
			ov.overlay, cmd = ov.overlay.Update(msg)
			return ov, cmd
		}
		switch msg.String() {
		case "esc", "q":
			return ov, func() tea.Msg { return PopViewMsg{} }
		case "enter":
			// Navigate to selected document in check overlay
			if ov.selectedIdx >= 0 && ov.selectedIdx < len(ov.checkNavLocs) {
				loc := ov.checkNavLocs[ov.selectedIdx]
				idx := ov.selectedIdx
				return ov, func() tea.Msg {
					return NavigateToDocMsg{
						ModuleName:   loc.ModuleName,
						DocumentName: loc.DocumentName,
						NavIndex:     idx,
					}
				}
			}
		case "r":
			if ov.refreshable && ov.refreshMsg != nil {
				refreshMsg := ov.refreshMsg
				return ov, func() tea.Msg { return refreshMsg }
			}
		case "tab":
			if ov.switchable && ov.qname != "" {
				ov.isNDSL = !ov.isNDSL
				return ov, ov.reloadContent()
			}
			// When refreshable (check overlay) and not switchable, Tab cycles severity filter
			if ov.refreshable && !ov.switchable && ov.checkErrors != nil {
				ov.checkFilter = nextCheckFilter(ov.checkFilter)
				// Recompute nav locations for new filter
				filtered := filterCheckErrors(ov.checkErrors, ov.checkFilter)
				ov.checkNavLocs = extractCheckNavLocations(filtered)
				if ov.selectedIdx >= len(ov.checkNavLocs) {
					ov.selectedIdx = max(0, len(ov.checkNavLocs)-1)
				}
				if len(ov.checkNavLocs) == 0 {
					ov.selectedIdx = -1
				}
				title := renderCheckFilterTitle(ov.checkErrors, ov.checkFilter)
				content := renderCheckResults(ov.checkErrors, ov.checkFilter)
				ov.overlay.Show(title, content, ov.overlay.width, ov.overlay.height)
				return ov, nil
			}
		}

		// 'f' formats MDL content (non-NDSL, non-check overlays only)
		if msg.String() == "f" && !ov.isNDSL && len(ov.checkNavLocs) == 0 {
			raw := ov.overlay.content.PlainText()
			formatted := formatter.Format(raw)
			highlighted := DetectAndHighlight(formatted)
			ov.overlay.Show(ov.overlay.title, highlighted, ov.overlay.width, ov.overlay.height)
			return ov, nil
		}

		// 'e' opens ExecView with overlay content (non-check overlays only)
		if msg.String() == "e" && len(ov.checkNavLocs) == 0 && ov.pendingKey == 0 {
			if ov.qname != "" && !ov.isNDSL {
				raw := ov.overlay.content.PlainText()
				return ov, func() tea.Msg { return OpenExecWithContentMsg{Content: raw} }
			}
		}

		// j/k move cursor, ]e/[e jump between errors in check overlay
		if len(ov.checkNavLocs) > 0 {
			switch msg.String() {
			case "j", "down":
				if ov.selectedIdx < len(ov.checkNavLocs)-1 {
					ov.selectedIdx++
				}
			case "k", "up":
				if ov.selectedIdx > 0 {
					ov.selectedIdx--
				}
			case "]":
				ov.pendingKey = ']'
				return ov, nil
			case "[":
				ov.pendingKey = '['
				return ov, nil
			case "e":
				if ov.pendingKey != 0 {
					if ov.pendingKey == ']' {
						ov.selectedIdx++
						if ov.selectedIdx >= len(ov.checkNavLocs) {
							ov.selectedIdx = 0
						}
					} else {
						ov.selectedIdx--
						if ov.selectedIdx < 0 {
							ov.selectedIdx = len(ov.checkNavLocs) - 1
						}
					}
					ov.pendingKey = 0
					return ov, nil
				}
			}
			// Clear pending if a non-e key was pressed after ]/[
			if msg.String() != "]" && msg.String() != "[" && msg.String() != "e" {
				ov.pendingKey = 0
			}
		}
	}

	var cmd tea.Cmd
	ov.overlay, cmd = ov.overlay.Update(msg)
	return ov, cmd
}

// Render returns the overlay's rendered string at the given dimensions with an LLM anchor prefix.
// OverlayView uses a value receiver: dimensions are set on the local copy
// so that View() picks them up within this call. The original is unaffected.
func (ov OverlayView) Render(width, height int) string {
	ov.overlay.width = width
	ov.overlay.height = height
	rendered := ov.overlay.View()

	// Build LLM anchor: compute check-specific structured anchors when check errors
	// are available, otherwise fall back to generic overlay anchor.
	var anchor string
	if len(ov.checkErrors) > 0 {
		groups := groupCheckErrors(ov.checkErrors)
		anchor = renderCheckAnchors(groups, ov.checkErrors)
	} else if ov.checkAnchors != "" {
		anchor = ov.checkAnchors
	} else {
		info := ov.StatusInfo()
		anchor = fmt.Sprintf("[mxcli:overlay] %s  %s", ov.overlay.title, info.Mode)
	}
	anchorStyle := lipgloss.NewStyle().Foreground(MutedColor).Faint(true)
	anchorStr := anchorStyle.Render(anchor)

	if idx := strings.IndexByte(rendered, '\n'); idx >= 0 {
		rendered = anchorStr + rendered[idx:]
	} else {
		rendered = anchorStr
	}
	return rendered
}

// Hints returns context-sensitive key hints for this overlay.
func (ov OverlayView) Hints() []Hint {
	hints := []Hint{
		{Key: "j/k", Label: "scroll"},
		{Key: "/", Label: "search"},
		{Key: "y", Label: "copy"},
	}
	if len(ov.checkNavLocs) > 0 {
		hints = append(hints, Hint{Key: "Enter", Label: "go to"})
	}
	if !ov.isNDSL && len(ov.checkNavLocs) == 0 {
		hints = append(hints, Hint{Key: "f", Label: "format"})
	}
	if ov.switchable {
		hints = append(hints, Hint{Key: "Tab", Label: "mdl/ndsl"})
	} else if ov.refreshable && ov.checkErrors != nil {
		hints = append(hints, Hint{Key: "Tab", Label: "filter"})
	}
	if ov.refreshable {
		hints = append(hints, Hint{Key: "r", Label: "rerun"})
	}
	hints = append(hints, Hint{Key: "q", Label: "close"})
	return hints
}

// StatusInfo returns display data for the status bar.
func (ov OverlayView) StatusInfo() StatusInfo {
	modeLabel := "MDL"
	if ov.isNDSL {
		modeLabel = "NDSL"
	}
	position := fmt.Sprintf("L%d/%d", ov.overlay.content.YOffset()+1, ov.overlay.content.TotalLines())
	return StatusInfo{
		Breadcrumb: []string{ov.overlay.title},
		Position:   position,
		Mode:       modeLabel,
	}
}

// Mode returns ModeOverlay.
func (ov OverlayView) Mode() ViewMode {
	return ModeOverlay
}

// reloadContent returns a tea.Cmd that fetches new content based on isNDSL state.
func (ov OverlayView) reloadContent() tea.Cmd {
	if ov.isNDSL {
		return ov.runBsonReload()
	}
	return ov.runMDLReload()
}

func (ov OverlayView) runBsonReload() tea.Cmd {
	bsonType := inferBsonType(ov.nodeType)
	if bsonType == "" {
		return nil
	}
	mxcliPath := ov.mxcliPath
	projectPath := ov.projectPath
	qname := ov.qname
	return func() tea.Msg {
		args := []string{"bson", "dump", "-p", projectPath, "--format", "ndsl",
			"--type", bsonType, "--object", qname}
		out, err := runMxcli(mxcliPath, args...)
		out = StripBanner(out)
		title := fmt.Sprintf("BSON: %s", qname)
		if err != nil {
			return overlayContentMsg{Title: title, Content: "Error: " + out}
		}
		return overlayContentMsg{Title: title, Content: HighlightNDSL(out)}
	}
}

func (ov OverlayView) runMDLReload() tea.Cmd {
	mxcliPath := ov.mxcliPath
	projectPath := ov.projectPath
	qname := ov.qname
	nodeType := ov.nodeType
	return func() tea.Msg {
		out, err := runMxcli(mxcliPath, "-p", projectPath, "-c", buildDescribeCmd(nodeType, qname))
		out = StripBanner(out)
		title := fmt.Sprintf("MDL: %s", qname)
		if err != nil {
			return overlayContentMsg{Title: title, Content: "Error: " + out}
		}
		return overlayContentMsg{Title: title, Content: DetectAndHighlight(out)}
	}
}
