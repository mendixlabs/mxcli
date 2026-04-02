package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mendixlabs/mxcli/mdl/formatter"
)

// AgentListener accepts agent connections on a Unix socket and converts
// JSON requests into tea.Msg values injected into the bubbletea program.
type AgentListener struct {
	socketPath  string
	listener    net.Listener
	sendMsg     func(tea.Msg)
	autoProceed bool
	mu          sync.Mutex
	closed      bool
	wg          sync.WaitGroup
}

// NewAgentListener creates and starts a Unix socket listener.
// sendMsg is called to inject messages into the bubbletea event loop
// (typically tea.Program.Send).
func NewAgentListener(socketPath string, sendMsg func(tea.Msg), autoProceed bool) (*AgentListener, error) {
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	// Restrict socket access to current user only
	if err := os.Chmod(socketPath, 0600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	al := &AgentListener{
		socketPath:  socketPath,
		listener:    ln,
		sendMsg:     sendMsg,
		autoProceed: autoProceed,
	}

	al.wg.Add(1)
	go al.acceptLoop()
	return al, nil
}

// Close stops the listener and removes the socket file.
func (al *AgentListener) Close() {
	al.mu.Lock()
	if al.closed {
		al.mu.Unlock()
		return
	}
	al.closed = true
	al.mu.Unlock()

	al.listener.Close()
	al.wg.Wait()
	os.Remove(al.socketPath)
}

// AutoProceed returns whether human confirmation is skipped.
func (al *AgentListener) AutoProceed() bool {
	return al.autoProceed
}

func (al *AgentListener) acceptLoop() {
	defer al.wg.Done()
	for {
		conn, err := al.listener.Accept()
		if err != nil {
			return
		}
		al.wg.Add(1)
		go al.handleConnection(conn)
	}
}

func (al *AgentListener) handleConnection(conn net.Conn) {
	defer al.wg.Done()
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	encoder := json.NewEncoder(conn)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req AgentRequest
		if err := json.Unmarshal(line, &req); err != nil {
			encoder.Encode(AgentResponse{OK: false, Error: "invalid json: " + err.Error()})
			continue
		}

		if err := req.Validate(); err != nil {
			encoder.Encode(AgentResponse{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}

		// Synchronous actions: handled directly in the listener goroutine
		// without bubbletea round-trip (pure computation or read-only).
		if resp, ok := al.handleSyncAction(req); ok {
			encoder.Encode(resp)
			continue
		}

		responseCh := make(chan AgentResponse, 1)

		switch req.Action {
		case "exec":
			al.sendMsg(AgentExecMsg{RequestID: req.ID, MDL: req.MDL, ResponseCh: responseCh})
		case "check":
			al.sendMsg(AgentCheckMsg{RequestID: req.ID, ResponseCh: responseCh})
		case "state":
			al.sendMsg(AgentStateMsg{RequestID: req.ID, ResponseCh: responseCh})
		case "navigate":
			al.sendMsg(AgentNavigateMsg{RequestID: req.ID, Target: req.Target, ResponseCh: responseCh})
		case "delete":
			al.sendMsg(AgentDeleteMsg{RequestID: req.ID, Target: req.Target, ResponseCh: responseCh})
		case "create_module":
			al.sendMsg(AgentCreateModuleMsg{RequestID: req.ID, Name: req.Name, ResponseCh: responseCh})
		case "list":
			mdl, err := buildListCmd(req.Target)
			if err != nil {
				encoder.Encode(AgentResponse{ID: req.ID, OK: false, Error: err.Error()})
				continue
			}
			al.sendMsg(AgentExecMsg{RequestID: req.ID, MDL: mdl, ResponseCh: responseCh})
		case "describe":
			mdl, err := buildAgentDescribeCmd(req.Target)
			if err != nil {
				encoder.Encode(AgentResponse{ID: req.ID, OK: false, Error: err.Error()})
				continue
			}
			al.sendMsg(AgentExecMsg{RequestID: req.ID, MDL: mdl, ResponseCh: responseCh})
		default:
			encoder.Encode(AgentResponse{ID: req.ID, OK: false, Error: fmt.Sprintf("unknown action: %q", req.Action)})
			continue
		}

		select {
		case resp := <-responseCh:
			encoder.Encode(resp)
		case <-time.After(120 * time.Second):
			encoder.Encode(AgentResponse{ID: req.ID, OK: false, Error: "timeout waiting for TUI response"})
		}
	}
}

// handleSyncAction handles actions that can be resolved synchronously
// without going through the bubbletea event loop.
// Returns (response, true) if handled, (zero, false) if not.
func (al *AgentListener) handleSyncAction(req AgentRequest) (AgentResponse, bool) {
	switch req.Action {
	case "format":
		formatted := formatter.Format(req.MDL)
		return AgentResponse{ID: req.ID, OK: true, Result: formatted}, true
	}
	return AgentResponse{}, false
}
