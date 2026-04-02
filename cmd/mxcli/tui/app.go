package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// chromeHeight is the vertical space consumed by tab bar (1) + hint bar (1) + status bar (1).
const chromeHeight = 3

// handledNoop is a pre-allocated no-op Msg to avoid per-call goroutine allocation.
var handledNoop tea.Msg = struct{}{}

// handledCmd is returned by handleBrowserAppKeys to signal that a key was
// consumed without producing a follow-up message.  Using a shared variable
// avoids allocating a new closure on every handled keystroke.
var handledCmd tea.Cmd = func() tea.Msg { return handledNoop }

// compareFlashClearMsg is sent 1 s after a clipboard copy in compare view.
type compareFlashClearMsg struct{}

// App is the root Bubble Tea model for the yazi-style TUI.
type App struct {
	tabs      []Tab
	activeTab int
	nextTabID int

	width     int
	height    int
	mxcliPath string

	views    ViewStack
	showHelp bool
	picker   *PickerModel // non-nil when cross-project picker is open

	// Check error navigation state (]e / [e)
	checkNavActive    bool
	checkNavIndex     int
	checkNavLocations []CheckNavLocation
	pendingKey        rune // ']' or '[' waiting for 'e', 0 if none

	tabBar        TabBar
	hintBar       HintBar
	statusBar     StatusBar
	previewEngine *PreviewEngine

	watcher      *Watcher
	checkErrors  []CheckError // nil = no check run yet, empty = pass
	checkRunning bool

	pendingSession *TUISession // session to restore after tree loads

	agentListener    *AgentListener
	agentAutoProceed bool                 // skip human confirmation for agent ops (set before tea.NewProgram)
	agentPending     *agentPendingOp      // non-nil when waiting for user confirmation
	agentCheckCh     chan<- AgentResponse // non-nil when agent check is in-flight
	agentCheckReqID  int                  // request ID for pending agent check
	agentExecCtx     *agentExecContext    // non-nil when agent-initiated exec/delete/create is in progress
}

// agentPendingOp tracks an in-flight agent operation awaiting user confirmation.
type agentPendingOp struct {
	RequestID  int
	Output     string
	Success    bool
	ResponseCh chan<- AgentResponse
}

// agentExecContext tracks an agent-initiated operation routed through UI views.
// The agent's exec/delete/create_module actions push the same views a human
// would use (ExecView/ConfirmView/InputView). This context links the UI flow
// back to the agent response channel.
type agentExecContext struct {
	RequestID  int
	ResponseCh chan<- AgentResponse
}

// NewApp creates the root App model.
func NewApp(mxcliPath, projectPath string) App {
	initTrace()
	Trace("app: NewApp mxcli=%q project=%q", mxcliPath, projectPath)

	engine := NewPreviewEngine(mxcliPath, projectPath)
	tab := NewTab(1, projectPath, engine, nil)

	browserView := NewBrowserView(&tab, mxcliPath, engine)

	app := App{
		mxcliPath:     mxcliPath,
		nextTabID:     2,
		views:         NewViewStack(browserView),
		tabBar:        NewTabBar(nil),
		statusBar:     NewStatusBar(),
		hintBar:       NewHintBar(ListBrowsingHints),
		previewEngine: engine,
	}
	app.tabs = []Tab{tab}
	app.syncTabBar()
	return app
}

// StartWatcher begins watching MPR files for external changes.
// Call after tea.NewProgram is created but before p.Run().
func (a *App) StartWatcher(prog *tea.Program) {
	tab := a.activeTabPtr()
	if tab == nil {
		return
	}
	mprPath := tab.ProjectPath
	contentsDir := ""
	dir := filepath.Dir(mprPath)
	candidate := filepath.Join(dir, "mprcontents")
	if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
		contentsDir = candidate
	}
	w, err := NewWatcher(mprPath, contentsDir, prog)
	if err != nil {
		Trace("app: failed to start watcher: %v", err)
		return
	}
	a.watcher = w
	Trace("app: watcher started for %s (contentsDir=%q)", mprPath, contentsDir)
}

// SetAgentAutoProceed configures whether agent operations skip human confirmation.
// Must be called BEFORE tea.NewProgram so the value is captured in the model copy.
func (a *App) SetAgentAutoProceed(autoProceed bool) {
	a.agentAutoProceed = autoProceed
}

// StartAgentListener begins listening on a Unix socket for agent commands.
// Call after tea.NewProgram is created, like StartWatcher.
func (a *App) StartAgentListener(prog *tea.Program, socketPath string, autoProceed bool) error {
	listener, err := NewAgentListener(socketPath, prog.Send, autoProceed)
	if err != nil {
		return err
	}
	a.agentListener = listener
	Trace("app: agent listener started on %s (autoProceed=%v)", socketPath, autoProceed)
	return nil
}

// CloseAgentListener stops the agent listener if running.
func (a *App) CloseAgentListener() {
	if a.agentListener != nil {
		a.agentListener.Close()
	}
}

func (a *App) activeTabPtr() *Tab {
	if a.activeTab >= 0 && a.activeTab < len(a.tabs) {
		return &a.tabs[a.activeTab]
	}
	return nil
}

func (a *App) activeTabProjectPath() string {
	tab := a.activeTabPtr()
	if tab != nil {
		return tab.ProjectPath
	}
	return ""
}

func (a *App) syncTabBar() {
	infos := make([]TabInfo, len(a.tabs))
	for i, t := range a.tabs {
		infos[i] = TabInfo{ID: t.ID, Label: t.Label, Active: i == a.activeTab}
	}
	a.tabBar.SetTabs(infos)
}

func (a *App) syncBrowserView() {
	tab := a.activeTabPtr()
	if tab == nil {
		return
	}
	bv := NewBrowserView(tab, a.mxcliPath, a.previewEngine)
	bv.allNodes = tab.AllNodes
	// Ensure miller has current dimensions so scroll calculations in
	// Update() work correctly (Render operates on a value copy).
	if a.height > 0 {
		contentH := max(5, a.height-chromeHeight-1) // -1 for LLM anchor line
		bv.miller.SetSize(a.width, contentH)
	}
	a.views.SetBase(bv)
}

// --- Init ---

func (a App) Init() tea.Cmd {
	tab := a.activeTabPtr()
	if tab == nil {
		return nil
	}
	tabID := tab.ID
	mxcliPath := a.mxcliPath
	projectPath := tab.ProjectPath
	return func() tea.Msg {
		out, err := runMxcli(mxcliPath, "project-tree", "-p", projectPath)
		if err != nil {
			return LoadTreeMsg{TabID: tabID, Err: err}
		}
		nodes, parseErr := ParseTree(out)
		return LoadTreeMsg{TabID: tabID, Nodes: nodes, Err: parseErr}
	}
}

// --- Update ---

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// --- ViewStack navigation ---
	case PushViewMsg:
		a.views.Push(msg.View)
		return a, nil
	case PopViewMsg:
		a.views.Pop()
		// If human cancelled a view while an agent operation was pending, reject it
		if ctx := a.agentExecCtx; ctx != nil {
			ctx.ResponseCh <- AgentResponse{
				ID: ctx.RequestID, OK: false,
				Error: "cancelled by user",
			}
			a.agentExecCtx = nil
		}
		return a, nil

	case PaletteExecMsg:
		a.views.Pop()
		if msg.Key != "" {
			return a, a.dispatchPaletteKey(msg.Key)
		}
		return a, nil

	// --- View creation messages ---
	case OpenOverlayMsg:
		ov := NewOverlayView(msg.Title, msg.Content, a.width, a.height, msg.Opts)
		a.views.Push(ov)
		return a, nil

	case OpenImageOverlayMsg:
		w, h := a.width, a.height
		paths := msg.Paths
		title := msg.Title
		return a, func() tea.Msg {
			innerW := w - 4
			innerH := h - 4
			if innerW < 20 {
				innerW = 20
			}
			if innerH < 5 {
				innerH = 5
			}
			perImg := innerH / len(paths)
			if perImg < 1 {
				perImg = 1
			}
			content := renderImagesWithSize(paths, innerW, perImg)
			if content == "" {
				content = "(no image rendered — set MXCLI_IMAGE_PROTOCOL or install chafa)"
			}
			return OpenOverlayMsg{Title: title, Content: content}
		}

	case JumpToNodeMsg:
		// Pop the jumper view first
		a.views.Pop()
		// Navigate browser to the target node
		if bv, ok := a.views.Base().(BrowserView); ok {
			cmd := bv.navigateToNode(msg.QName)
			a.views.SetBase(bv)
			if tab := a.activeTabPtr(); tab != nil {
				tab.Miller = bv.miller
				tab.UpdateLabel()
				a.syncTabBar()
			}
			return a, cmd
		}
		return a, nil

	case NavigateToDocMsg:
		// Close overlay, navigate tree to document, enter check nav mode
		a.views.Pop()
		qname := docNameToQualifiedName(msg.ModuleName, msg.DocumentName)
		if bv, ok := a.views.Base().(BrowserView); ok {
			cmd := bv.navigateToNode(qname)
			a.views.SetBase(bv)
			if tab := a.activeTabPtr(); tab != nil {
				tab.Miller = bv.miller
				tab.UpdateLabel()
				a.syncTabBar()
			}
			// Enter check nav mode
			a.checkNavActive = true
			a.checkNavIndex = msg.NavIndex
			a.checkNavLocations = extractCheckNavLocations(filterCheckErrors(a.checkErrors, "all"))
			return a, cmd
		}
		return a, nil

	case DiffOpenMsg:
		dv := NewDiffView(msg, a.width, a.height)
		a.views.Push(dv)
		return a, nil

	case PickerDoneMsg:
		Trace("app: PickerDoneMsg path=%q", msg.Path)
		a.picker = nil
		if msg.Path != "" {
			SaveHistory(msg.Path)
			engine := NewPreviewEngine(a.mxcliPath, msg.Path)
			newTab := NewTab(a.nextTabID, msg.Path, engine, nil)
			a.nextTabID++
			a.tabs = append(a.tabs, newTab)
			a.activeTab = len(a.tabs) - 1
			a.syncBrowserView()
			a.syncTabBar()
			tabID := newTab.ID
			mxcliPath := a.mxcliPath
			projectPath := msg.Path
			return a, func() tea.Msg {
				out, err := runMxcli(mxcliPath, "project-tree", "-p", projectPath)
				if err != nil {
					return LoadTreeMsg{TabID: tabID, Err: err}
				}
				nodes, parseErr := ParseTree(out)
				return LoadTreeMsg{TabID: tabID, Nodes: nodes, Err: parseErr}
			}
		}
		return a, nil

	case CompareLoadMsg:
		if cv, ok := a.views.Active().(CompareView); ok {
			cv.SetContent(msg.Side, msg.Title, msg.NodeType, msg.Content)
			a.views.SetActive(cv)
		}
		return a, nil

	case ComparePickMsg:
		if cv, ok := a.views.Active().(CompareView); ok {
			cv.SetLoading(msg.Side)
			a.views.SetActive(cv)
			return a, cv.loadForCompare(msg.QName, msg.NodeType, msg.Side, msg.Kind)
		}
		return a, nil

	case CompareReloadMsg:
		if cv, ok := a.views.Active().(CompareView); ok {
			var cmds []tea.Cmd
			if cv.left.qname != "" {
				cv.SetLoading(CompareFocusLeft)
				cmds = append(cmds, cv.loadForCompare(cv.left.qname, cv.left.nodeType, CompareFocusLeft, msg.Kind))
			}
			if cv.right.qname != "" {
				cv.SetLoading(CompareFocusRight)
				cmds = append(cmds, cv.loadForCompare(cv.right.qname, cv.right.nodeType, CompareFocusRight, msg.Kind))
			}
			a.views.SetActive(cv)
			return a, tea.Batch(cmds...)
		}
		return a, nil

	case overlayFlashClearMsg:
		// Forward to active view (Overlay handles this internally)
		updated, cmd := a.views.Active().Update(msg)
		a.views.SetActive(updated)
		return a, cmd

	case compareFlashClearMsg:
		if cv, ok := a.views.Active().(CompareView); ok {
			cv.copiedFlash = false
			a.views.SetActive(cv)
		}
		return a, nil

	case overlayContentMsg:
		updated, cmd := a.views.Active().Update(msg)
		a.views.SetActive(updated)
		return a, cmd

	case CmdResultMsg:
		content := msg.Output
		if msg.Err != nil {
			content = "-- Error:\n" + msg.Output
		}
		ov := NewOverlayView("Result", DetectAndHighlight(content), a.width, a.height, OverlayViewOpts{})
		a.views.Push(ov)
		return a, nil

	case discardDoneMsg:
		// Clear preview cache so stale data isn't shown after discard
		a.previewEngine.ClearCache()
		return a, func() tea.Msg {
			return execShowResultMsg{Content: msg.Output, Success: msg.Success}
		}

	case execShowResultMsg:
		// Pop the ExecView (or ConfirmView)
		a.views.Pop()
		// Show result in overlay
		content := DetectAndHighlight(msg.Content)
		title := "Exec Result"
		if a.agentExecCtx != nil {
			title = "Agent Exec Result"
			if !msg.Success {
				title = "Agent Exec Error"
			}
		}
		ov := NewOverlayView(title, content, a.width, a.height, OverlayViewOpts{})
		a.views.Push(ov)
		// If execution succeeded, suppress watcher (self-modification) and refresh tree
		if msg.Success {
			if a.watcher != nil {
				a.watcher.Suppress(2 * time.Second)
			}
		}
		// Handle agent response if this was agent-initiated
		if ctx := a.agentExecCtx; ctx != nil {
			if a.agentAutoProceed {
				resp := AgentResponse{
					ID: ctx.RequestID, OK: msg.Success,
					Result: msg.Content, Mode: "overlay:exec-result",
				}
				if changes := agentParseChanges(msg.Content); len(changes) > 0 {
					resp.Changes, _ = json.Marshal(changes)
				}
				ctx.ResponseCh <- resp
				a.agentExecCtx = nil
				if msg.Success {
					return a, a.Init()
				}
				return a, nil
			}
			// Store pending op for user confirmation (q/esc in overlay)
			a.agentPending = &agentPendingOp{
				RequestID: ctx.RequestID, Output: msg.Content,
				Success: msg.Success, ResponseCh: ctx.ResponseCh,
			}
			a.agentExecCtx = nil
			return a, nil
		}
		if msg.Success {
			return a, a.Init()
		}
		return a, nil

	case OpenExecWithContentMsg:
		ev := NewExecViewWithContent(a.mxcliPath, a.activeTabProjectPath(), a.width, a.height, msg.Content)
		a.views.Push(ev)
		return a, nil

	// --- Agent channel messages ---
	case AgentExecMsg:
		Trace("app: AgentExecMsg id=%d mdl=%q", msg.RequestID, msg.MDL)
		// Route through ExecView — same UI path as human pressing 'x'
		a.agentExecCtx = &agentExecContext{
			RequestID:  msg.RequestID,
			ResponseCh: msg.ResponseCh,
		}
		ev := NewExecViewWithContent(a.mxcliPath, a.activeTabProjectPath(), a.width, a.height, msg.MDL)
		a.views.Push(ev)
		if a.agentAutoProceed {
			return a, func() tea.Msg { return AgentAutoExecMsg{} }
		}
		return a, nil

	case AgentStateMsg:
		Trace("app: AgentStateMsg id=%d", msg.RequestID)
		stateInfo := agentBuildState(a)
		stateJSON, _ := json.Marshal(stateInfo)
		msg.ResponseCh <- AgentResponse{
			ID: msg.RequestID, OK: true,
			Result: string(stateJSON),
			Mode:   "state",
		}
		return a, nil

	case AgentCheckMsg:
		Trace("app: AgentCheckMsg id=%d", msg.RequestID)
		if a.agentCheckCh != nil {
			msg.ResponseCh <- AgentResponse{
				ID: msg.RequestID, OK: false,
				Error: "check already in progress",
			}
			return a, nil
		}
		a.agentCheckCh = msg.ResponseCh
		a.agentCheckReqID = msg.RequestID
		projectPath := a.activeTabProjectPath()
		return a, runMxCheck(projectPath)

	case AgentNavigateMsg:
		Trace("app: AgentNavigateMsg id=%d target=%q", msg.RequestID, msg.Target)
		target := msg.Target
		if bv, ok := a.views.Base().(BrowserView); ok {
			qname := target
			if idx := strings.Index(target, ":"); idx >= 0 {
				qname = target[idx+1:]
			}
			cmd := bv.navigateToNode(qname)
			a.views.SetBase(bv)
			if tab := a.activeTabPtr(); tab != nil {
				tab.Miller = bv.miller
				tab.UpdateLabel()
				a.syncTabBar()
			}
			msg.ResponseCh <- AgentResponse{
				ID: msg.RequestID, OK: true,
				Result: fmt.Sprintf("navigated to %s", qname), Mode: "browser",
			}
			return a, cmd
		}
		msg.ResponseCh <- AgentResponse{
			ID: msg.RequestID, OK: false, Error: "not in browser mode",
		}
		return a, nil

	case AgentDeleteMsg:
		Trace("app: AgentDeleteMsg id=%d target=%q", msg.RequestID, msg.Target)
		nodeType, qname := parseTarget(msg.Target)
		dropCmd := buildDropCmd(nodeType, qname)
		if dropCmd == "" {
			msg.ResponseCh <- AgentResponse{
				ID: msg.RequestID, OK: false,
				Error: fmt.Sprintf("unsupported delete type: %q", nodeType),
			}
			return a, nil
		}
		// Route through ConfirmView — same UI path as human pressing 'D'
		a.agentExecCtx = &agentExecContext{
			RequestID:  msg.RequestID,
			ResponseCh: msg.ResponseCh,
		}
		message := buildDeleteMessage(nodeType, qname)
		cv := NewConfirmView("Delete", message, dropCmd, a.mxcliPath, a.activeTabProjectPath())
		a.views.Push(cv)
		if a.agentAutoProceed {
			// Auto-confirm (like pressing 'y')
			return a, func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
			}
		}
		return a, nil

	case AgentCreateModuleMsg:
		Trace("app: AgentCreateModuleMsg id=%d name=%q", msg.RequestID, msg.Name)
		// Route through InputView — same UI path as human pressing 'C'
		a.agentExecCtx = &agentExecContext{
			RequestID:  msg.RequestID,
			ResponseCh: msg.ResponseCh,
		}
		mxcliPath := a.mxcliPath
		projectPath := a.activeTabProjectPath()
		iv := NewInputView("Create Module", "Module name: ", func(name string) tea.Cmd {
			return func() tea.Msg {
				out, err := runMxcli(mxcliPath, "-p", projectPath, "-c", "CREATE MODULE "+name)
				return execShowResultMsg{Content: out, Success: err == nil}
			}
		})
		iv.input.SetValue(msg.Name)
		a.views.Push(iv)
		if a.agentAutoProceed {
			// Auto-submit (like pressing Enter)
			return a, func() tea.Msg {
				return tea.KeyMsg{Type: tea.KeyEnter}
			}
		}
		return a, nil

	case tea.KeyMsg:
		Trace("app: key=%q picker=%v mode=%v help=%v", msg.String(), a.picker != nil, a.views.Active().Mode(), a.showHelp)
		if msg.String() == "ctrl+c" {
			if a.watcher != nil {
				a.watcher.Close()
			}
			a.CloseAgentListener()
			return a, tea.Quit
		}

		// Agent confirmation: when overlay is dismissed while agent op is pending
		if a.agentPending != nil && a.views.Active().Mode() == ModeOverlay &&
			(msg.String() == "q" || msg.String() == "esc") {
			pending := a.agentPending
			a.agentPending = nil
			a.views.Pop()
			resp := AgentResponse{
				ID: pending.RequestID, OK: pending.Success,
				Result: pending.Output, Mode: "overlay:exec-result",
			}
			if changes := agentParseChanges(pending.Output); len(changes) > 0 {
				resp.Changes, _ = json.Marshal(changes)
			}
			pending.ResponseCh <- resp
			if pending.Success {
				return a, a.Init()
			}
			return a, nil
		}

		// Picker modal (not a View — special case)
		if a.picker != nil {
			result, cmd := a.picker.Update(msg)
			p := result.(PickerModel)
			a.picker = &p
			return a, cmd
		}

		// Help toggle (global, only in Browser mode)
		if a.showHelp {
			a.showHelp = false
			return a, nil
		}
		if msg.String() == "?" && a.views.Active().Mode() == ModeBrowser {
			a.showHelp = !a.showHelp
			return a, nil
		}

		// Tab management and app-level keys (only in Browser mode)
		if a.views.Active().Mode() == ModeBrowser {
			if cmd := a.handleBrowserAppKeys(msg); cmd != nil {
				return a, cmd
			}
		}

		// Delegate to active view
		updated, cmd := a.views.Active().Update(msg)
		a.views.SetActive(updated)

		// Sync tab label if browser view
		if a.views.Active().Mode() == ModeBrowser {
			tab := a.activeTabPtr()
			if tab != nil {
				if bv, ok := a.views.Active().(BrowserView); ok {
					tab.Miller = bv.miller
					tab.UpdateLabel()
					a.syncTabBar()
				}
			}
		}
		return a, cmd

	case tea.MouseMsg:
		Trace("app: mouse x=%d y=%d btn=%v action=%v", msg.X, msg.Y, msg.Button, msg.Action)
		if a.picker != nil {
			return a, nil
		}

		// Tab bar clicks (row 1, after LLM anchor line) — only when in browser mode
		if msg.Y == 1 && a.views.Active().Mode() == ModeBrowser &&
			msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if clickMsg := a.tabBar.HandleClick(msg.X); clickMsg != nil {
				if tc, ok := clickMsg.(TabClickMsg); ok {
					a.switchToTabByID(tc.ID)
					return a, nil
				}
			}
		}

		// Status bar clicks (last line) — breadcrumb navigation
		if msg.Y == a.height-1 && a.views.Active().Mode() == ModeBrowser &&
			msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if depth, ok := a.statusBar.HitTest(msg.X); ok {
				if bv, ok := a.views.Active().(BrowserView); ok {
					var cmd tea.Cmd
					bv.miller, cmd = bv.miller.goBackToDepth(depth)
					a.views.SetActive(bv)
					if tab := a.activeTabPtr(); tab != nil {
						tab.Miller = bv.miller
						tab.UpdateLabel()
						a.syncTabBar()
					}
					return a, cmd
				}
			}
		}

		// Offset Y by -2 (LLM anchor line + tab bar) when in browser mode
		if a.views.Active().Mode() == ModeBrowser {
			offsetMsg := tea.MouseMsg{
				X: msg.X, Y: msg.Y - 2,
				Button: msg.Button, Action: msg.Action,
			}
			updated, cmd := a.views.Active().Update(offsetMsg)
			a.views.SetActive(updated)
			return a, cmd
		}

		// Forward to active view
		updated, cmd := a.views.Active().Update(msg)
		a.views.SetActive(updated)
		return a, cmd

	case tea.WindowSizeMsg:
		Trace("app: resize %dx%d", msg.Width, msg.Height)
		a.width = msg.Width
		a.height = msg.Height
		// Propagate content dimensions to the browser view so that
		// subsequent Update() calls use correct column heights for
		// scroll calculations (Render operates on a copy and cannot
		// persist dimensions back).
		if bv, ok := a.views.Active().(BrowserView); ok {
			contentH := a.height - chromeHeight - 1 // -1 for LLM anchor line
			if contentH < 5 {
				contentH = 5
			}
			bv.miller.SetSize(a.width, contentH)
			a.views.SetActive(bv)
		}
		return a, nil

	case LoadTreeMsg:
		Trace("app: LoadTreeMsg tabID=%d err=%v nodes=%d", msg.TabID, msg.Err, len(msg.Nodes))
		if msg.Err == nil && msg.Nodes != nil {
			tab := a.findTabByID(msg.TabID)
			if tab != nil {
				tab.AllNodes = msg.Nodes
				tab.Miller.SetRootNodes(msg.Nodes)
				a.syncTabBar()
				// Update browser view if it's the base
				if bv, ok := a.views.Base().(BrowserView); ok {
					bv.allNodes = msg.Nodes
					bv.miller = tab.Miller
					if a.height > 0 {
						contentH := max(5, a.height-chromeHeight-1) // -1 for LLM anchor line
						bv.miller.SetSize(a.width, contentH)
					}
					a.views.SetBase(bv)
				}
			}

			// Apply pending session restore after tree is loaded
			if a.pendingSession != nil {
				applySessionRestore(&a)
			}
		}
		return a, nil

	case MprChangedMsg:
		Trace("app: MprChangedMsg — refreshing tree and running mx check")
		a.previewEngine.ClearCache()
		projectPath := a.activeTabProjectPath()
		return a, tea.Batch(a.Init(), runMxCheck(projectPath))

	case MxCheckRerunMsg:
		Trace("app: manual mx check rerun requested")
		projectPath := a.activeTabProjectPath()
		return a, runMxCheck(projectPath)

	case MxCheckStartMsg:
		a.checkRunning = true
		// Update check overlay content if it's currently visible
		if ov, ok := a.views.Active().(OverlayView); ok && ov.refreshable {
			ov.overlay.Show("mx check", CheckRunningStyle.Render("⟳ Running mx check..."), ov.overlay.width, ov.overlay.height)
			a.views.SetActive(ov)
		}
		return a, nil

	case MxCheckResultMsg:
		a.checkRunning = false
		if msg.Err != nil {
			Trace("app: mx check error: %v", msg.Err)
			a.checkErrors = nil
		} else {
			a.checkErrors = msg.Errors
			Trace("app: mx check done: %d diagnostics", len(msg.Errors))
		}
		// Respond to pending agent check request
		if a.agentCheckCh != nil {
			if msg.Err != nil {
				a.agentCheckCh <- AgentResponse{
					ID: a.agentCheckReqID, OK: false,
					Result: msg.Err.Error(), Mode: "check",
				}
			} else {
				result := renderCheckResultsPlain(msg.Errors)
				a.agentCheckCh <- AgentResponse{
					ID: a.agentCheckReqID, OK: true,
					Result: result, Mode: "check",
				}
			}
			a.agentCheckCh = nil
			a.agentCheckReqID = 0
		}
		// Update check overlay content if it's currently visible
		if ov, ok := a.views.Active().(OverlayView); ok && ov.refreshable {
			ov.checkErrors = a.checkErrors
			filtered := filterCheckErrors(a.checkErrors, ov.checkFilter)
			ov.checkNavLocs = extractCheckNavLocations(filtered)
			if ov.selectedIdx >= len(ov.checkNavLocs) {
				ov.selectedIdx = max(0, len(ov.checkNavLocs)-1)
			}
			if len(ov.checkNavLocs) == 0 {
				ov.selectedIdx = -1
			}
			title := renderCheckFilterTitle(a.checkErrors, ov.checkFilter)
			content := renderCheckResults(a.checkErrors, ov.checkFilter)
			ov.overlay.Show(title, content, ov.overlay.width, ov.overlay.height)
			a.views.SetActive(ov)
		}
		return a, nil

	case PreviewReadyMsg, PreviewLoadingMsg, CursorChangedMsg, animTickMsg, previewDebounceMsg:
		if a.views.Active().Mode() == ModeBrowser {
			updated, cmd := a.views.Active().Update(msg)
			a.views.SetActive(updated)
			// Sync miller back to tab
			if bv, ok := updated.(BrowserView); ok {
				tab := a.activeTabPtr()
				if tab != nil {
					tab.Miller = bv.miller
				}
			}
			return a, cmd
		}
		return a, nil

	default:
		// Forward everything else to active view
		updated, cmd := a.views.Active().Update(msg)
		a.views.SetActive(updated)
		return a, cmd
	}
}

// handleBrowserAppKeys handles keys that App intercepts when in Browser mode.
// Returns a non-nil tea.Cmd if the key was handled, nil if the key should
// be forwarded to the active view.
func (a *App) handleBrowserAppKeys(msg tea.KeyMsg) tea.Cmd {
	tab := a.activeTabPtr()

	// Handle two-key sequence: ]e / [e (check error navigation)
	if a.pendingKey != 0 {
		pending := a.pendingKey
		a.pendingKey = 0
		if msg.String() == "e" && len(a.checkErrors) > 0 {
			// Lazily initialize check nav state if not already active
			if !a.checkNavActive {
				a.checkNavActive = true
				a.checkNavLocations = extractCheckNavLocations(filterCheckErrors(a.checkErrors, "all"))
				a.checkNavIndex = -1 // will be incremented to 0 for ], or wrapped to last for [
			}
			if pending == ']' {
				a.checkNavIndex++
				if a.checkNavIndex >= len(a.checkNavLocations) {
					a.checkNavIndex = 0 // wrap around
				}
			} else {
				a.checkNavIndex--
				if a.checkNavIndex < 0 {
					a.checkNavIndex = len(a.checkNavLocations) - 1 // wrap around
				}
			}
			loc := a.checkNavLocations[a.checkNavIndex]
			qname := docNameToQualifiedName(loc.ModuleName, loc.DocumentName)
			if bv, ok := a.views.Base().(BrowserView); ok {
				cmd := bv.navigateToNode(qname)
				a.views.SetBase(bv)
				if tab := a.activeTabPtr(); tab != nil {
					tab.Miller = bv.miller
					tab.UpdateLabel()
					a.syncTabBar()
				}
				return cmd
			}
			return handledCmd
		}
		// Not 'e' — fall through to normal handling for the pending key
		// Re-process the pending key's original action
		if pending == ']' {
			if a.activeTab < len(a.tabs)-1 {
				a.activeTab++
				a.syncBrowserView()
				a.syncTabBar()
			}
		} else if pending == '[' {
			if a.activeTab > 0 {
				a.activeTab--
				a.syncBrowserView()
				a.syncTabBar()
			}
		}
		// Now process the current key normally (fall through)
	}

	// Non-nav keys exit check nav mode (preserve for ]/[/! which are nav-related)
	if a.checkNavActive {
		key := msg.String()
		if key != "]" && key != "[" && key != "!" && key != "\\!" {
			a.checkNavActive = false
		}
	}

	switch msg.String() {
	case "q":
		// Save session state before quitting
		if session := ExtractSession(a); session != nil {
			_ = SaveSession(session)
		}
		if a.watcher != nil {
			a.watcher.Close()
		}
		for i := range a.tabs {
			a.tabs[i].Miller.previewEngine.Cancel()
		}
		CloseTrace()
		return tea.Quit

	case "t":
		if tab != nil {
			newTab := tab.CloneTab(a.nextTabID, a.previewEngine)
			a.nextTabID++
			a.tabs = append(a.tabs, newTab)
			a.activeTab = len(a.tabs) - 1
			a.syncBrowserView()
			a.syncTabBar()
		}
		return handledCmd

	case "T":
		p := NewEmbeddedPicker()
		p.width = a.width
		p.height = a.height
		a.picker = &p
		return handledCmd

	case "W":
		if len(a.tabs) > 1 {
			a.tabs[a.activeTab].Miller.previewEngine.Cancel()
			a.tabs = append(a.tabs[:a.activeTab], a.tabs[a.activeTab+1:]...)
			if a.activeTab >= len(a.tabs) {
				a.activeTab = len(a.tabs) - 1
			}
			a.syncBrowserView()
			a.syncTabBar()
		}
		return handledCmd

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(msg.String()[0]-'0') - 1
		if idx >= 0 && idx < len(a.tabs) {
			a.activeTab = idx
			a.syncBrowserView()
			a.syncTabBar()
		}
		return handledCmd

	case "[":
		if len(a.checkErrors) > 0 {
			a.pendingKey = '['
			return handledCmd
		}
		if a.activeTab > 0 {
			a.activeTab--
			a.syncBrowserView()
			a.syncTabBar()
		}
		return handledCmd

	case "]":
		if len(a.checkErrors) > 0 {
			a.pendingKey = ']'
			return handledCmd
		}
		if a.activeTab < len(a.tabs)-1 {
			a.activeTab++
			a.syncBrowserView()
			a.syncTabBar()
		}
		return handledCmd

	case "r":
		return a.Init()

	case "R":
		projectPath := a.activeTabProjectPath()
		if projectPath == "" {
			return handledCmd
		}
		cv := NewDiscardConfirmView(projectPath, a.mxcliPath)
		a.views.Push(cv)
		return handledCmd

	case " ":
		if tab != nil {
			items := flattenQualifiedNames(tab.AllNodes)
			jumper := NewJumperView(items, a.width, a.height)
			a.views.Push(jumper)
		}
		return handledCmd

	case "x":
		ev := NewExecView(a.mxcliPath, a.activeTabProjectPath(), a.width, a.height)
		a.views.Push(ev)
		return handledCmd

	case "C":
		mxcliPath := a.mxcliPath
		projectPath := a.activeTabProjectPath()
		iv := NewInputView("Create Module", "Module name: ", func(name string) tea.Cmd {
			return func() tea.Msg {
				out, err := runMxcli(mxcliPath, "-p", projectPath, "-c", "CREATE MODULE "+name)
				return execShowResultMsg{Content: out, Success: err == nil}
			}
		})
		a.views.Push(iv)
		return handledCmd

	case "!", "\\!": // some terminals send "\\!" for shifted-1; accept both forms
		filter := "all"
		title := renderCheckFilterTitle(a.checkErrors, filter)
		content := renderCheckResults(a.checkErrors, filter)
		navLocs := extractCheckNavLocations(a.checkErrors)
		ov := NewOverlayView(title, content, a.width, a.height, OverlayViewOpts{
			HideLineNumbers: true,
			Refreshable:     true,
			RefreshMsg:      MxCheckRerunMsg{},
			CheckFilter:     filter,
			CheckErrors:     a.checkErrors,
			CheckNavLocs:    navLocs,
		})
		a.views.Push(ov)
		return handledCmd

	case ":":
		cp := NewCommandPaletteView(a.width, a.height)
		a.views.Push(cp)
		return handledCmd

	case "c":
		cv := NewCompareView()
		cv.mxcliPath = a.mxcliPath
		cv.projectPath = a.activeTabProjectPath()
		cv.Show(CompareNDSLMDL, a.width, a.height)
		if tab != nil {
			cv.SetItems(flattenQualifiedNames(tab.AllNodes))
			if node := tab.Miller.SelectedNode(); node != nil && node.QualifiedName != "" {
				cv.SetLoading(CompareFocusLeft)
				cv.SetLoading(CompareFocusRight)
				a.views.Push(cv)
				return tea.Batch(
					cv.loadBsonNDSL(node.QualifiedName, node.Type, CompareFocusLeft),
					cv.loadMDL(node.QualifiedName, node.Type, CompareFocusRight),
				)
			}
		}
		a.views.Push(cv)
		return handledCmd
	}

	return nil
}

func (a *App) findTabByID(id int) *Tab {
	for i := range a.tabs {
		if a.tabs[i].ID == id {
			return &a.tabs[i]
		}
	}
	return nil
}

func (a *App) switchToTabByID(id int) {
	for i, t := range a.tabs {
		if t.ID == id {
			a.activeTab = i
			a.syncBrowserView()
			a.syncTabBar()
			return
		}
	}
}

// SetPendingSession stores a session to be restored after the project tree loads.
func (a *App) SetPendingSession(session *TUISession) {
	a.pendingSession = session
}

// applySessionRestore applies the pending session state to the loaded app.
// Called after LoadTreeMsg delivers nodes so navigation paths can be resolved.
// Takes *App because it's called from Update (value receiver) via &a.
func applySessionRestore(a *App) {
	session := a.pendingSession
	if session == nil {
		return
	}
	a.pendingSession = nil

	if len(session.Tabs) == 0 {
		return
	}

	// Restore the first tab's navigation (multi-tab restore: only the
	// primary tab is restored since additional tabs need separate
	// project-tree loads which are not wired yet).
	ts := session.Tabs[0]
	tab := a.activeTabPtr()
	if tab == nil || len(tab.AllNodes) == 0 {
		return
	}

	// Navigate to the selected node if available
	if ts.SelectedNode != "" {
		if bv, ok := a.views.Base().(BrowserView); ok {
			bv.allNodes = tab.AllNodes
			bv.navigateToNode(ts.SelectedNode)
			// Set preview mode after navigation (navigateToNode resets miller)
			setPreviewMode(&bv.miller, ts.PreviewMode)
			tab.Miller = bv.miller
			tab.UpdateLabel()
			a.views.SetBase(bv)
			a.syncTabBar()
			Trace("app: session restored — navigated to %q", ts.SelectedNode)
			return
		}
	}

	// Fallback: navigate the miller path breadcrumb
	if len(ts.MillerPath) > 0 {
		restoreMillerPath(a, tab, ts.MillerPath)
	}

	// Set preview mode (for path-based or no-navigation restore)
	setPreviewMode(&tab.Miller, ts.PreviewMode)
}

// setPreviewMode sets the miller preview mode from a string value.
func setPreviewMode(miller *MillerView, mode string) {
	if mode == "NDSL" {
		miller.preview.mode = PreviewNDSL
	} else {
		miller.preview.mode = PreviewMDL
	}
}

// restoreMillerPath drills the miller view through a breadcrumb path.
func restoreMillerPath(a *App, tab *Tab, millerPath []string) {
	bv, ok := a.views.Base().(BrowserView)
	if !ok {
		return
	}
	bv.allNodes = tab.AllNodes
	bv.miller.SetRootNodes(tab.AllNodes)

	for _, segment := range millerPath {
		found := false
		for j, item := range bv.miller.current.items {
			if item.Label == segment {
				bv.miller.current.SetCursor(j)
				if item.Node != nil && len(item.Node.Children) > 0 {
					bv.miller, _ = bv.miller.drillIn()
				}
				found = true
				break
			}
		}
		if !found {
			Trace("app: session restore — path segment %q not found, stopping", segment)
			break
		}
	}

	tab.Miller = bv.miller
	tab.UpdateLabel()
	a.views.SetBase(bv)
	a.syncTabBar()
	Trace("app: session restored via miller path %v", millerPath)
}

// --- View ---

func (a App) View() string {
	if a.width == 0 {
		return "mxcli tui — loading...\n\nPress q to quit"
	}

	if a.picker != nil {
		return a.picker.View()
	}

	active := a.views.Active()

	// For non-browser views, delegate rendering entirely
	if active.Mode() != ModeBrowser {
		contentH := a.height
		content := active.Render(a.width, contentH)

		if a.showHelp {
			helpView := renderHelp(a.width, a.height)
			content = lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, helpView,
				lipgloss.WithWhitespaceBackground(lipgloss.Color("0")))
		}

		return content
	}

	// Browser mode: App renders chrome (tab bar, hint bar, status bar)

	// Tab bar (line 1) with mode badge + context summary on the right
	tabLine := a.tabBar.View(a.width)
	tab := a.activeTabPtr()
	if tab != nil {
		modeBadge := AccentStyle.Render(active.Mode().String())
		summary := renderContextSummary(tab.AllNodes)
		rightSide := modeBadge
		if summary != "" {
			rightSide += BreadcrumbDimStyle.Render(" │ ") + BreadcrumbDimStyle.Render(summary)
		}
		rightWidth := lipgloss.Width(rightSide) + 1 // 1 char right padding
		tabWidth := lipgloss.Width(tabLine)
		if tabWidth+rightWidth <= a.width {
			// Replace trailing spaces with gap + right side
			trimmed := strings.TrimRight(tabLine, " ")
			trimmedWidth := lipgloss.Width(trimmed)
			gap := a.width - trimmedWidth - rightWidth
			if gap < 2 {
				gap = 2
			}
			tabLine = trimmed + strings.Repeat(" ", gap) + rightSide + " "
		}
	}

	// Content area (chromeHeight + 1 for the LLM anchor line)
	contentH := a.height - chromeHeight - 1
	if contentH < 5 {
		contentH = 5
	}
	content := active.Render(a.width, contentH)

	// Hint bar — declarative from active view
	a.hintBar.SetHints(active.Hints())
	hintLine := a.hintBar.View(a.width)

	// Status bar — declarative from active view
	info := active.StatusInfo()
	a.statusBar.SetBreadcrumb(info.Breadcrumb)
	a.statusBar.SetPosition(info.Position)
	a.statusBar.SetMode(info.Mode)
	if a.checkNavActive && len(a.checkNavLocations) > 0 {
		loc := a.checkNavLocations[a.checkNavIndex]
		navInfo := fmt.Sprintf("[%d/%d] %s: %s  ]e next  [e prev",
			a.checkNavIndex+1, len(a.checkNavLocations),
			loc.Code, docNameToQualifiedName(loc.ModuleName, loc.DocumentName))
		a.statusBar.SetCheckBadge(CheckWarnStyle.Render(navInfo))
	} else {
		a.statusBar.SetCheckBadge(formatCheckBadge(a.checkErrors, a.checkRunning))
	}
	// Agent activity badge
	if a.agentExecCtx != nil {
		a.statusBar.SetAgentBadge(AgentBadgeStyle.Render("⚡agent"))
	} else {
		a.statusBar.SetAgentBadge("")
	}
	viewModeNames := a.collectViewModeNames()
	a.statusBar.SetViewDepth(a.views.Depth(), viewModeNames)
	statusLine := StatusBarStyle.Width(a.width).Render(a.statusBar.View(a.width))

	// LLM anchor: machine-readable command list (Faint, not visible to users in practice)
	anchorStyle := lipgloss.NewStyle().Foreground(MutedColor).Faint(true)
	anchorLine := anchorStyle.Render("[mxcli:commands] h:back l:open Space:jump /:filter b:bson c:compare d:diagram z:zen Tab:toggle x:exec r:refresh y:copy !:check ]e:next-error [e:prev-error t:tab T:new-tab W:close-tab 1-9:switch ?:help ::palette")

	rendered := anchorLine + "\n" + tabLine + "\n" + content + "\n" + hintLine + "\n" + statusLine

	if a.showHelp {
		helpView := renderHelp(a.width, a.height)
		rendered = lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, helpView,
			lipgloss.WithWhitespaceBackground(lipgloss.Color("0")))
	}

	return rendered
}

// --- Load helpers ---

func buildDiagramHTML(elkJSON, nodeType, qualifiedName string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>%s %s</title>
<script src="https://cdn.jsdelivr.net/npm/elkjs@0.9.3/lib/elk.bundled.js"></script>
<style>body{margin:0;background:#1e1e2e;color:#cdd6f4;font-family:monospace}svg{width:100vw;height:100vh}</style>
</head><body><div id="diagram"></div><script>
const elkData = %s;
const ELK = new ELKConstructor();
ELK.layout(elkData).then(graph=>{
  const svg=document.createElementNS("http://www.w3.org/2000/svg","svg");
  document.getElementById("diagram").appendChild(svg);
});
</script></body></html>`, nodeType, qualifiedName, elkJSON)
}

// CmdResultMsg carries output from any mxcli command.
type CmdResultMsg struct {
	Output string
	Err    error
}

// irregularPlurals maps singular type names to their correct plural forms
// for types where simply appending "s" produces incorrect English.
var irregularPlurals = map[string]string{
	"Index": "indexes",
}

// renderContextSummary counts top-level node types and returns a compact summary.
func renderContextSummary(nodes []*TreeNode) string {
	if len(nodes) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, n := range nodes {
		counts[n.Type]++
	}
	// Display in a predictable order
	order := []struct {
		key    string
		plural string
	}{
		{"Module", "modules"},
		{"Entity", "entities"},
		{"Microflow", "microflows"},
		{"Page", "pages"},
		{"Nanoflow", "nanoflows"},
		{"Enumeration", "enumerations"},
	}
	var parts []string
	used := map[string]bool{}
	for _, o := range order {
		if c, ok := counts[o.key]; ok {
			parts = append(parts, fmt.Sprintf("%d %s", c, o.plural))
			used[o.key] = true
		}
	}
	// Add remaining types not in the predefined order
	for k, c := range counts {
		if !used[k] {
			plural, ok := irregularPlurals[k]
			if !ok {
				plural = strings.ToLower(k) + "s"
			}
			parts = append(parts, fmt.Sprintf("%d %s", c, plural))
		}
	}
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ", ")
}

// collectViewModeNames returns the mode names for all views in the stack.
func (a App) collectViewModeNames() []string {
	return a.views.ModeNames()
}

// dispatchPaletteKey converts a palette command key string into a synthetic
// tea.KeyMsg and re-dispatches it through Update.
func (a App) dispatchPaletteKey(key string) tea.Cmd {
	var keyMsg tea.KeyMsg
	switch key {
	case " ":
		keyMsg = tea.KeyMsg{Type: tea.KeySpace}
	case "Tab":
		keyMsg = tea.KeyMsg{Type: tea.KeyTab}
	default:
		keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	return func() tea.Msg { return keyMsg }
}

// inferBsonType maps tree node types to valid bson object types.
func inferBsonType(nodeType string) string {
	switch strings.ToLower(nodeType) {
	case "page", "microflow", "nanoflow", "workflow",
		"enumeration", "snippet", "layout", "entity", "association",
		"imagecollection", "javaaction", "javascriptaction", "constant":
		return strings.ToLower(nodeType)
	default:
		return ""
	}
}

// agentStateInfo is the structured state returned by the "state" action.
type agentStateInfo struct {
	Mode         string         `json:"mode"`
	Project      string         `json:"project"`
	SelectedNode *agentNodeInfo `json:"selectedNode,omitempty"`
	PreviewMode  string         `json:"previewMode,omitempty"`
	CheckErrors  int            `json:"checkErrors"`
	CheckRunning bool           `json:"checkRunning"`
}

// agentNodeInfo describes the currently selected tree node.
type agentNodeInfo struct {
	Type          string `json:"type"`
	QualifiedName string `json:"qualifiedName"`
}

// agentBuildState extracts rich TUI state for the agent.
func agentBuildState(a App) agentStateInfo {
	info := agentStateInfo{
		Mode:         a.views.Active().Mode().String(),
		Project:      a.activeTabProjectPath(),
		CheckErrors:  len(a.checkErrors),
		CheckRunning: a.checkRunning,
	}
	if bv, ok := a.views.Base().(BrowserView); ok {
		info.PreviewMode = "MDL"
		if bv.miller.preview.mode == PreviewNDSL {
			info.PreviewMode = "NDSL"
		}
		if node := bv.miller.SelectedNode(); node != nil {
			qname := node.QualifiedName
			if qname == "" {
				qname = node.Label
			}
			info.SelectedNode = &agentNodeInfo{
				Type:          node.Type,
				QualifiedName: qname,
			}
		}
	}
	return info
}

// agentExecChanges is a structured summary of exec output changes.
type agentExecChange struct {
	Action string `json:"action"` // "created", "modified", "dropped"
	Target string `json:"target"` // e.g. "entity Module.Entity"
}

// agentChangePattern matches exec output lines like "Created entity MyModule.Customer".
// Requires a known Mendix type keyword after the verb to avoid matching log noise
// such as "Removed trailing whitespace".
var agentChangePattern = regexp.MustCompile(`(?im)^(Created|Modified|Dropped|Deleted|Added|Removed)\s+(entity|association|attribute|enumeration|microflow|nanoflow|page|layout|snippet|module|folder|constant|workflow|image collection|java action|user role|module role|demo user|business event service)\s+(.+)$`)

// agentParseChanges extracts structured changes from exec output.
func agentParseChanges(output string) []agentExecChange {
	matches := agentChangePattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}
	changes := make([]agentExecChange, 0, len(matches))
	for _, m := range matches {
		changes = append(changes, agentExecChange{
			Action: strings.ToLower(m[1]),
			Target: strings.ToLower(m[2]) + " " + strings.TrimSpace(m[3]),
		})
	}
	return changes
}

// Shared by BrowserView and CompareView to avoid duplicate implementations.
func loadBsonNDSL(mxcliPath, projectPath, qname, nodeType string, side CompareFocus) tea.Cmd {
	return func() tea.Msg {
		bsonType := inferBsonType(nodeType)
		if bsonType == "" {
			return CompareLoadMsg{Side: side, Title: qname, NodeType: nodeType,
				Content: fmt.Sprintf("Error: type %q not supported for BSON dump", nodeType),
				Err:     fmt.Errorf("unsupported type")}
		}
		args := []string{"bson", "dump", "-p", projectPath, "--format", "ndsl",
			"--type", bsonType, "--object", qname}
		out, err := runMxcli(mxcliPath, args...)
		out = StripBanner(out)
		if err != nil {
			return CompareLoadMsg{Side: side, Title: qname, NodeType: nodeType, Content: "Error: " + out, Err: err}
		}
		return CompareLoadMsg{Side: side, Title: qname, NodeType: nodeType, Content: HighlightNDSL(out)}
	}
}
