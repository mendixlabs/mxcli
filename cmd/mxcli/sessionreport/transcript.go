// SPDX-License-Identifier: Apache-2.0

// Package sessionreport reads Claude Code session transcripts (JSON Lines) and
// reports where an agent session's tool calls and tokens went.
//
// It is the second instrument of lever 6 in
// docs/11-proposals/PROPOSAL_agent_loop_efficiency.md. `mxcli diag loop-report`
// counts mxcli processes from mxcli's own logs; it cannot see the agent's other
// tool calls, how large their results were, or why a call happened. The
// transcript can: every model call, every tool call and every tool result is in
// it. Since cost ≈ model calls × conversation size, the two numbers this
// package cares about most are the call count and the re-read cost of each
// result — its size multiplied by the number of model calls that re-read it.
//
// The report never prints prompt or tool content beyond short command, path and
// error snippets, so its output can be shared without sharing the conversation.
package sessionreport

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Usage is the token accounting of one model call, as the API reports it.
type Usage struct {
	Input      int64 `json:"input_tokens"`
	CacheRead  int64 `json:"cache_read_input_tokens"`
	CacheWrite int64 `json:"cache_creation_input_tokens"`
	Output     int64 `json:"output_tokens"`
}

func (u *Usage) add(o Usage) {
	u.Input += o.Input
	u.CacheRead += o.CacheRead
	u.CacheWrite += o.CacheWrite
	u.Output += o.Output
}

// Call is one tool call and its result. Raw content is kept only as far as the
// classifiers need it and is never rendered beyond Snippet / ErrorLine.
type Call struct {
	ID   string
	Tool string
	// Turn is the index (0-based, within the conversation) of the model call
	// that issued this tool call. Its result is first read by model call Turn+1.
	Turn int
	Time time.Time

	Command string // Bash: the command
	Path    string // Read/Edit/Write/Grep/Glob: the file or directory
	Pattern string // Grep/Glob: the pattern
	Name    string // Skill: the skill; Agent: the description

	HasResult    bool
	ResultChars  int
	ResultImages int
	// ResultTokens estimates what the result adds to the conversation:
	// characters / 4 for text, plus an image estimate (see imageTokens).
	ResultTokens int
	Failed       bool
	ErrorLine    string

	// resultText is the result's text, used by the failure and error-line
	// heuristics only.
	resultText string
}

// Conversation is one model context: the main session, or one subagent.
// Re-read cost is computed per conversation, because a subagent's results are
// re-read by the subagent's own calls and never by the main session's.
type Conversation struct {
	Key        string // "main", or the subagent's agent id
	ModelCalls int
	Usage      Usage
	Calls      []*Call
	// Boundaries are model-call indices at which the conversation was
	// compacted: a result issued before a boundary is not re-read after it.
	Boundaries []int
	First      time.Time
	Last       time.Time
	// Active is wall time with idle gaps (over idleGap between two records)
	// left out: a session resumed after a night is not a 12-hour session.
	Active time.Duration
}

// idleGap is the longest pause between two records still counted as work.
const idleGap = 15 * time.Minute

// Session is one transcript file and the subagent transcripts beside it.
type Session struct {
	ID            string
	Source        string
	Conversations []*Conversation
}

// rawRecord is one JSON Lines entry. Only the fields read here are declared.
type rawRecord struct {
	Type        string          `json:"type"`
	Subtype     string          `json:"subtype"`
	IsSidechain bool            `json:"isSidechain"`
	AgentID     string          `json:"agentId"`
	SessionID   string          `json:"sessionId"`
	UUID        string          `json:"uuid"`
	RequestID   string          `json:"requestId"`
	Timestamp   time.Time       `json:"timestamp"`
	Message     json.RawMessage `json:"message"`
}

type rawMessage struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Usage   *Usage          `json:"usage"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Text      string          `json:"text"`
	Source    *struct {
		Data string `json:"data"`
	} `json:"source"`
}

type convBuilder struct {
	conv   *Conversation
	msgs   map[string]int   // message id -> model call index
	usages map[string]Usage // message id -> last usage seen
	calls  map[string]*Call // tool_use id -> call
	order  []string         // message ids in order, for summing usage
}

// ParseFile reads one transcript. When the file is <dir>/<session>.jsonl and
// <dir>/<session>/subagents/*.jsonl exist, those are read too unless
// withSubagents is false: Claude Code writes each subagent's conversation
// there rather than into the main file.
func ParseFile(path string, withSubagents bool) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := &Session{Source: path}
	builders := map[string]*convBuilder{}
	var order []string
	if err := parseInto(f, s, builders, &order); err != nil {
		return nil, err
	}
	if withSubagents {
		dir := strings.TrimSuffix(path, filepath.Ext(path))
		subs, _ := filepath.Glob(filepath.Join(dir, "subagents", "*.jsonl"))
		sort.Strings(subs)
		for _, sp := range subs {
			sf, err := os.Open(sp)
			if err != nil {
				continue
			}
			_ = parseInto(sf, s, builders, &order)
			sf.Close()
		}
	}
	finish(s, builders, order)
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return s, nil
}

// Parse reads one transcript from r (no subagent discovery).
func Parse(r io.Reader, source string) (*Session, error) {
	s := &Session{Source: source}
	builders := map[string]*convBuilder{}
	var order []string
	if err := parseInto(r, s, builders, &order); err != nil {
		return nil, err
	}
	finish(s, builders, order)
	if s.ID == "" {
		s.ID = source
	}
	return s, nil
}

func parseInto(r io.Reader, s *Session, builders map[string]*convBuilder, order *[]string) error {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			handleLine(line, s, builders, order)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func handleLine(line []byte, s *Session, builders map[string]*convBuilder, order *[]string) {
	var rec rawRecord
	if json.Unmarshal(line, &rec) != nil {
		return // a truncated final line is normal
	}
	if s.ID == "" && rec.SessionID != "" {
		s.ID = rec.SessionID
	}
	key := "main"
	if rec.IsSidechain {
		key = "sidechain"
		if rec.AgentID != "" {
			key = rec.AgentID
		}
	}
	b := builders[key]
	if b == nil {
		b = &convBuilder{
			conv:   &Conversation{Key: key},
			msgs:   map[string]int{},
			usages: map[string]Usage{},
			calls:  map[string]*Call{},
		}
		builders[key] = b
		*order = append(*order, key)
	}
	c := b.conv
	if !rec.Timestamp.IsZero() {
		if c.First.IsZero() || rec.Timestamp.Before(c.First) {
			c.First = rec.Timestamp
		}
		if rec.Timestamp.After(c.Last) {
			if !c.Last.IsZero() {
				if gap := rec.Timestamp.Sub(c.Last); gap <= idleGap {
					c.Active += gap
				}
			}
			c.Last = rec.Timestamp
		}
	}
	if rec.Type == "system" && rec.Subtype == "compact_boundary" {
		c.Boundaries = append(c.Boundaries, c.ModelCalls)
		return
	}
	if rec.Type != "assistant" && rec.Type != "user" {
		return
	}
	var msg rawMessage
	if json.Unmarshal(rec.Message, &msg) != nil {
		return
	}
	var blocks []rawBlock
	if len(msg.Content) > 0 && msg.Content[0] == '[' {
		_ = json.Unmarshal(msg.Content, &blocks)
	}

	if rec.Type == "assistant" {
		// One API response is written as several records, one per content
		// block, each repeating the message id and usage. A model call is a
		// distinct message id, not a record.
		id := msg.ID
		if id == "" {
			id = rec.RequestID
		}
		if id == "" {
			id = rec.UUID
		}
		turn, seen := b.msgs[id]
		if !seen {
			turn = c.ModelCalls
			b.msgs[id] = turn
			b.order = append(b.order, id)
			c.ModelCalls++
		}
		if msg.Usage != nil {
			b.usages[id] = *msg.Usage
		}
		for _, bl := range blocks {
			if bl.Type != "tool_use" {
				continue
			}
			call := &Call{ID: bl.ID, Tool: bl.Name, Turn: turn, Time: rec.Timestamp}
			fillInput(call, bl.Input)
			b.calls[bl.ID] = call
			c.Calls = append(c.Calls, call)
		}
		return
	}

	for _, bl := range blocks {
		if bl.Type != "tool_result" {
			continue
		}
		call := b.calls[bl.ToolUseID]
		if call == nil {
			continue
		}
		text, images := resultContent(bl.Content)
		call.HasResult = true
		call.ResultChars = len(text)
		call.ResultImages = len(images)
		call.ResultTokens = (len(text) + 3) / 4
		for _, img := range images {
			call.ResultTokens += imageTokens(img)
		}
		call.resultText = text
		call.Failed, call.ErrorLine = detectFailure(call.Tool, call.Command, bl.IsError, text)
	}
}

func fillInput(c *Call, raw json.RawMessage) {
	var in map[string]any
	if json.Unmarshal(raw, &in) != nil {
		return
	}
	str := func(k string) string {
		v, _ := in[k].(string)
		return v
	}
	c.Command = str("command")
	c.Path = str("file_path")
	if c.Path == "" {
		c.Path = str("notebook_path")
	}
	if c.Path == "" {
		c.Path = str("path")
	}
	c.Pattern = str("pattern")
	c.Name = str("skill")
	if c.Name == "" {
		c.Name = str("description")
	}
}

// resultContent flattens a tool_result's content: a string, or a list of
// text and image blocks. Images are returned as their base64 data.
func resultContent(raw json.RawMessage) (string, []string) {
	if len(raw) == 0 {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s, nil
	}
	var blocks []rawBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil
	}
	var sb strings.Builder
	var images []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(b.Text)
		case "image":
			data := ""
			if b.Source != nil {
				data = b.Source.Data
			}
			images = append(images, data)
		}
	}
	return sb.String(), images
}

// imageTokenCap is the most an image is counted for: the API downscales an
// image whose long edge exceeds 1568 px, which puts a full-size image at about
// 1,600 tokens. An image whose size cannot be read is counted at the cap.
const imageTokenCap = 1600

// imageTokens estimates an image's input cost with Anthropic's published rule
// of thumb, width × height / 750, reading the dimensions from a PNG header.
// Any other format, or a header that does not decode, counts at the cap.
func imageTokens(b64 string) int {
	head := b64
	if len(head) > 44 {
		head = head[:44] // 33 bytes: signature + IHDR width/height
	}
	raw, err := base64.StdEncoding.DecodeString(head[:len(head)/4*4])
	if err != nil || len(raw) < 24 || string(raw[1:4]) != "PNG" {
		return imageTokenCap
	}
	w := int(binary.BigEndian.Uint32(raw[16:20]))
	h := int(binary.BigEndian.Uint32(raw[20:24]))
	t := w * h / 750
	if t <= 0 || t > imageTokenCap {
		return imageTokenCap
	}
	return t
}

func finish(s *Session, builders map[string]*convBuilder, order []string) {
	for _, key := range order {
		b := builders[key]
		for _, id := range b.order {
			b.conv.Usage.add(b.usages[id])
		}
		// A file with records but no model call (a bare prompt, a snapshot)
		// is not a conversation worth reporting.
		if b.conv.ModelCalls == 0 && len(b.conv.Calls) == 0 {
			continue
		}
		s.Conversations = append(s.Conversations, b.conv)
	}
}

// readersAfter is the number of model calls that re-read a result issued at
// model call turn: every later call up to the next compaction.
func (c *Conversation) readersAfter(turn int) int {
	end := c.ModelCalls
	for _, b := range c.Boundaries {
		if b > turn {
			end = b
			break
		}
	}
	n := end - (turn + 1)
	if n < 0 {
		return 0
	}
	return n
}
