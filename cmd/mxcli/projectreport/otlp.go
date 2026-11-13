// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Span is one finished OpenTelemetry span, flattened. It is what the collector
// writes (one JSON object per line) and what the tests, performance and
// traceability reports read.
type Span struct {
	TraceID  string            `json:"traceId"`
	SpanID   string            `json:"spanId"`
	ParentID string            `json:"parentId,omitempty"`
	Name     string            `json:"name"`
	Kind     int               `json:"kind,omitempty"`
	Start    int64             `json:"start"` // unix nanoseconds
	End      int64             `json:"end"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Service  string            `json:"service,omitempty"`
}

// DurationMs is the span's duration in milliseconds.
func (s Span) DurationMs() float64 { return float64(s.End-s.Start) / 1e6 }

// microflowSpan matches the runtime's microflow spans ("Microflow Mod.Name").
var microflowSpan = regexp.MustCompile(`^(?i:microflow)\s+([A-Za-z_]\w*\.[A-Za-z_]\w*)`)

// Microflow returns the qualified microflow name a span executes, or "".
func (s Span) Microflow() string {
	if m := microflowSpan.FindStringSubmatch(s.Name); m != nil {
		return m[1]
	}
	for _, k := range []string{"mx.microflow.name", "mendix.microflow.name", "microflow.name", "microflow"} {
		if v := s.Attrs[k]; v != "" && strings.Contains(v, ".") {
			return v
		}
	}
	return ""
}

// IsDB reports whether the span is a database call.
func (s Span) IsDB() bool {
	if s.Attrs["db.system"] != "" || s.Attrs["db.statement"] != "" || s.Attrs["db.query.text"] != "" {
		return true
	}
	n := strings.ToUpper(s.Name)
	return strings.HasPrefix(n, "SELECT") || strings.HasPrefix(n, "INSERT") || strings.HasPrefix(n, "UPDATE") || strings.HasPrefix(n, "DELETE")
}

// Statement is the SQL text of a database span, or its name.
func (s Span) Statement() string {
	for _, k := range []string{"db.statement", "db.query.text"} {
		if v := s.Attrs[k]; v != "" {
			return v
		}
	}
	return s.Name
}

// ---------------------------------------------------------------- reading

// ReadSpans reads spans from files or directories: the collector's JSONL
// (one Span per line), OTLP/JSON exports ({"resourceSpans": …}), or OTLP
// protobuf files (.pb). A directory is read for every file in it.
func ReadSpans(paths []string) ([]Span, error) {
	var out []Span
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		files := []string{p}
		if fi.IsDir() {
			files = nil
			entries, err := os.ReadDir(p)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if !e.IsDir() {
					files = append(files, filepath.Join(p, e.Name()))
				}
			}
			sort.Strings(files)
		}
		for _, f := range files {
			spans, err := readSpanFile(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			out = append(out, spans...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}

func readSpanFile(path string) ([]Span, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".pb") || strings.HasSuffix(path, ".protobuf") {
		return DecodeOTLPProto(b)
	}
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed[:min(len(trimmed), 200)], "resourceSpans") && !strings.Contains(trimmed, "\n{") {
		return DecodeOTLPJSON(b)
	}
	var out []Span
	sc := bufio.NewScanner(strings.NewReader(trimmed))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.Contains(line[:min(len(line), 40)], "resourceSpans") {
			spans, err := DecodeOTLPJSON([]byte(line))
			if err != nil {
				return nil, err
			}
			out = append(out, spans...)
			continue
		}
		var s Span
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

// DecodeOTLPJSON decodes an OTLP/JSON ExportTraceServiceRequest.
func DecodeOTLPJSON(b []byte) ([]Span, error) {
	type kv struct {
		Key   string         `json:"key"`
		Value map[string]any `json:"value"`
	}
	var req struct {
		ResourceSpans []struct {
			Resource struct {
				Attributes []kv `json:"attributes"`
			} `json:"resource"`
			ScopeSpans []struct {
				Spans []struct {
					TraceID      string `json:"traceId"`
					SpanID       string `json:"spanId"`
					ParentSpanID string `json:"parentSpanId"`
					Name         string `json:"name"`
					Kind         int    `json:"kind"`
					Start        any    `json:"startTimeUnixNano"`
					End          any    `json:"endTimeUnixNano"`
					Attributes   []kv   `json:"attributes"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return nil, err
	}
	val := func(m map[string]any) string {
		for _, v := range m {
			return fmt.Sprint(v)
		}
		return ""
	}
	nanos := func(v any) int64 {
		switch t := v.(type) {
		case string:
			n, _ := strconv.ParseInt(t, 10, 64)
			return n
		case float64:
			return int64(t)
		}
		return 0
	}
	var out []Span
	for _, rs := range req.ResourceSpans {
		service := ""
		for _, a := range rs.Resource.Attributes {
			if a.Key == "service.name" {
				service = val(a.Value)
			}
		}
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				s := Span{TraceID: sp.TraceID, SpanID: sp.SpanID, ParentID: sp.ParentSpanID, Name: sp.Name, Kind: sp.Kind,
					Start: nanos(sp.Start), End: nanos(sp.End), Service: service}
				if len(sp.Attributes) > 0 {
					s.Attrs = map[string]string{}
					for _, a := range sp.Attributes {
						s.Attrs[a.Key] = val(a.Value)
					}
				}
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// DecodeOTLPProto decodes an OTLP protobuf ExportTraceServiceRequest — what
// the runtime's agent sends with OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf,
// the protocol `run --trace-otlp` sets. Only the fields a report reads are
// decoded; everything else is skipped by wire type, so newer agents that add
// fields still decode.
func DecodeOTLPProto(b []byte) ([]Span, error) {
	var out []Span
	err := eachField(b, func(num int, wt int, v []byte, _ uint64) error {
		if num != 1 || wt != 2 { // resource_spans
			return nil
		}
		service := ""
		var scopes [][]byte
		if err := eachField(v, func(num int, wt int, v []byte, _ uint64) error {
			switch {
			case num == 1 && wt == 2: // resource
				return eachField(v, func(num int, wt int, v []byte, _ uint64) error {
					if num == 1 && wt == 2 {
						k, val, err := decodeKV(v)
						if err == nil && k == "service.name" {
							service = val
						}
						return err
					}
					return nil
				})
			case num == 2 && wt == 2: // scope_spans
				scopes = append(scopes, v)
			}
			return nil
		}); err != nil {
			return err
		}
		for _, sc := range scopes {
			if err := eachField(sc, func(num int, wt int, v []byte, _ uint64) error {
				if num != 2 || wt != 2 { // spans
					return nil
				}
				s, err := decodeSpan(v)
				if err != nil {
					return err
				}
				s.Service = service
				out = append(out, s)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func decodeSpan(b []byte) (Span, error) {
	var s Span
	err := eachField(b, func(num int, wt int, v []byte, n uint64) error {
		switch {
		case num == 1 && wt == 2:
			s.TraceID = hex.EncodeToString(v)
		case num == 2 && wt == 2:
			s.SpanID = hex.EncodeToString(v)
		case num == 4 && wt == 2:
			s.ParentID = hex.EncodeToString(v)
		case num == 5 && wt == 2:
			s.Name = string(v)
		case num == 6 && wt == 0:
			s.Kind = int(n)
		case num == 7 && wt == 1:
			s.Start = int64(n)
		case num == 8 && wt == 1:
			s.End = int64(n)
		case num == 9 && wt == 2:
			k, val, err := decodeKV(v)
			if err != nil {
				return err
			}
			if s.Attrs == nil {
				s.Attrs = map[string]string{}
			}
			s.Attrs[k] = val
		}
		return nil
	})
	return s, err
}

func decodeKV(b []byte) (string, string, error) {
	var key, val string
	err := eachField(b, func(num int, wt int, v []byte, _ uint64) error {
		switch {
		case num == 1 && wt == 2:
			key = string(v)
		case num == 2 && wt == 2:
			return eachField(v, func(num int, wt int, v []byte, n uint64) error {
				switch {
				case num == 1 && wt == 2:
					val = string(v)
				case num == 2 && wt == 0:
					val = strconv.FormatBool(n != 0)
				case num == 3 && wt == 0:
					val = strconv.FormatInt(int64(n), 10)
				case num == 4 && wt == 1:
					val = strconv.FormatFloat(math.Float64frombits(n), 'f', -1, 64)
				}
				return nil
			})
		}
		return nil
	})
	return key, val, err
}

// eachField walks a protobuf message. For wire types 0 and 1 the value is in
// n; for 2 it is in v; wire type 5 is passed in n as well.
func eachField(b []byte, fn func(num int, wt int, v []byte, n uint64) error) error {
	for len(b) > 0 {
		tag, k := binary.Uvarint(b)
		if k <= 0 {
			return errors.New("bad protobuf tag")
		}
		b = b[k:]
		num, wt := int(tag>>3), int(tag&7)
		switch wt {
		case 0:
			n, k := binary.Uvarint(b)
			if k <= 0 {
				return errors.New("bad protobuf varint")
			}
			b = b[k:]
			if err := fn(num, wt, nil, n); err != nil {
				return err
			}
		case 1:
			if len(b) < 8 {
				return errors.New("short fixed64")
			}
			if err := fn(num, wt, nil, binary.LittleEndian.Uint64(b)); err != nil {
				return err
			}
			b = b[8:]
		case 2:
			l, k := binary.Uvarint(b)
			if k <= 0 || uint64(len(b)-k) < l {
				return errors.New("bad protobuf length")
			}
			v := b[k : k+int(l)]
			b = b[k+int(l):]
			if err := fn(num, wt, v, 0); err != nil {
				return err
			}
		case 5:
			if len(b) < 4 {
				return errors.New("short fixed32")
			}
			if err := fn(num, wt, nil, uint64(binary.LittleEndian.Uint32(b))); err != nil {
				return err
			}
			b = b[4:]
		default:
			return fmt.Errorf("unsupported protobuf wire type %d", wt)
		}
	}
	return nil
}

// ---------------------------------------------------------------- receiver

// Collector is a minimal OTLP/HTTP trace receiver: POST /v1/traces, protobuf
// or JSON, optionally gzipped. Each span is appended to a JSONL file as it
// arrives, so a crash loses nothing already received.
type Collector struct {
	mu    sync.Mutex
	out   *os.File
	count int
	srv   *http.Server
	ln    net.Listener
}

// StartCollector listens on addr (e.g. 127.0.0.1:4318) and appends spans to path.
func StartCollector(addr, path string) (*Collector, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("listening on %s: %w", addr, err)
	}
	c := &Collector{out: f, ln: ln}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", c.handle)
	c.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = c.srv.Serve(ln) }()
	return c, nil
}

// Addr is the address the collector listens on.
func (c *Collector) Addr() string { return c.ln.Addr().String() }

// Count is how many spans have been written.
func (c *Collector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// Close stops the server and closes the file.
func (c *Collector) Close() error {
	_ = c.srv.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.Close()
}

func (c *Collector) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body io.Reader = r.Body
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer gz.Close()
		body = gz
	}
	b, err := io.ReadAll(io.LimitReader(body, 64<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var spans []Span
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "json") {
		spans, err = DecodeOTLPJSON(b)
		w.Header().Set("Content-Type", "application/json")
	} else {
		spans, err = DecodeOTLPProto(b)
		w.Header().Set("Content-Type", "application/x-protobuf")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	enc := json.NewEncoder(c.out)
	for _, s := range spans {
		if enc.Encode(s) == nil {
			c.count++
		}
	}
	c.mu.Unlock()
	if strings.Contains(ct, "json") {
		_, _ = w.Write([]byte("{}"))
	}
}
