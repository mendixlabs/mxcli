// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// OQLOptions configures OQL query execution against a running Mendix runtime.
type OQLOptions struct {
	// Host is the hostname of the Mendix admin API (default: localhost).
	Host string

	// Port is the admin API port (default: 8090).
	Port int

	// Token is the M2EE admin password for authentication.
	Token string

	// ProjectPath is the path to the .mpr file (used to find .docker/.env).
	ProjectPath string

	// Direct bypasses docker exec and connects to the admin API directly.
	// By default (when false and ProjectPath is set), the request is routed
	// through "docker compose exec" to reach the container's loopback interface.
	Direct bool

	// Stdout for output.
	Stdout io.Writer

	// Stderr for status messages.
	Stderr io.Writer
}

// OQLResult holds the result of an OQL query execution.
type OQLResult struct {
	Columns []string
	Rows    [][]any
}

// ExecuteOQL runs an OQL query against the Mendix admin API using preview_execute_oql.
//
// By default, when ProjectPath is set, the request is routed through
// "docker compose exec" to reach the container's loopback admin API (port 8090
// binds to 127.0.0.1 inside the container and is unreachable from DinD).
// Set Direct=true to connect via HTTP directly (when the admin API is reachable).
func ExecuteOQL(opts OQLOptions, query string) (*OQLResult, error) {
	res, err := executeOQL(opts, query)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "action not found") {
		if v := runtimeVersion(m2eeOptionsFor(opts)); v != "" && majorVersion(v) > 0 && majorVersion(v) < 10 {
			return nil, fmt.Errorf("OQL over the admin API needs Mendix 10 or later; this runtime is %s, which has no "+
				"preview_execute_oql action, so no flag or restart makes it answer. Read its data from its database "+
				"instead (tables are named module$entity)", v)
		}
	}
	return res, err
}

// m2eeOptionsFor is the admin connection ExecuteOQL uses.
func m2eeOptionsFor(opts OQLOptions) M2EEOptions {
	return M2EEOptions{
		Host:        opts.Host,
		Port:        opts.Port,
		Token:       opts.Token,
		ProjectPath: opts.ProjectPath,
		Direct:      opts.Direct,
		Timeout:     10 * time.Second,
	}
}

// runtimeVersion is the running runtime's version from the M2EE `about`
// action, or "" when it cannot be read. Asked only once an OQL route is
// missing: a Mendix 9 runtime has no preview_execute_oql, and the dev-flags
// advice the missing action otherwise gets cannot help there
// (ako/mxcli#2341, mendixlabs/mxcli#1879).
func runtimeVersion(opts M2EEOptions) string {
	resp, err := CallM2EE(opts, "about", nil)
	if err != nil || resp.M2EEError() != "" {
		return ""
	}
	var about struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(resp.RawFeedback, &about) != nil {
		return ""
	}
	return about.Version
}

// majorVersion is the leading number of a version string, 0 when it has none.
func majorVersion(v string) int {
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func executeOQL(opts OQLOptions, query string) (*OQLResult, error) {
	m2eeOpts := M2EEOptions{
		Host:        opts.Host,
		Port:        opts.Port,
		Token:       opts.Token,
		ProjectPath: opts.ProjectPath,
		Direct:      opts.Direct,
		Timeout:     10 * time.Second,
	}

	params := map[string]any{
		"oql":            query,
		"numberHandling": "asString",
	}

	// Mendix 11.11+ serves OQL preview as a REST endpoint
	// (POST /dev/preview_execute_oql with the params as the body, returning
	// {"data":[...]} directly). Try it first and fall back to the legacy M2EE
	// action (POST / with {"action","params"}) when it is not there.
	//
	// "Not there" has two shapes, and only one of them is an HTTP 404. A runtime
	// older than 11.11 has no /dev/ route at all, so the admin API dispatches the
	// POST as an ordinary admin request, finds no "action" field in the body, and
	// answers **200** with {"result":<non-zero>,"message":"Action not found"}.
	// Treating only the 404 as absence meant the legacy action — which works
	// perfectly on those runtimes — was never tried, so `mxcli oql` failed on
	// every Mendix before 11.11 with a message telling the user to upgrade mxcli.
	// Measured against 11.6.6: the dev path returns that 200, and the legacy
	// action answers the same query.
	raw, err := previewOQLDev(m2eeOpts, params)
	if errors.Is(err, errDevEndpointNotFound) {
		return legacyOQL(m2eeOpts, params)
	}
	if err != nil {
		return nil, err
	}

	// The dev endpoint reports query failures as HTTP 200 with an {"error":"..."}
	// body (no "data"), so a bad query must be surfaced here rather than parsed
	// as an empty result.
	errMsg, absent := oqlDevErrorKind(raw)
	if absent {
		// The dev route is not mounted, so the legacy action is the real attempt
		// and its answer is the one that matters — including when it is an error.
		// Reporting the dev route's "Action not found" instead would blame the
		// transport for a query the runtime rejected on its merits: an unknown
		// entity would come back as "upgrade mxcli", which is neither true nor
		// actionable.
		res, lerr := legacyOQL(m2eeOpts, params)
		if lerr == nil {
			return res, nil
		}
		// Unless the legacy action is missing too — then the live-preview
		// servlets really are absent, and the dev message carries the hint that
		// says so.
		if !strings.Contains(strings.ToLower(lerr.Error()), "action not found") {
			return nil, lerr
		}
	}
	if errMsg != "" {
		return nil, fmt.Errorf("OQL error: %s", errMsg)
	}

	return parseOQLFeedback(raw)
}

// legacyOQL runs the query through the M2EE admin action, which is how every
// runtime before 11.11 serves OQL preview.
func legacyOQL(opts M2EEOptions, params map[string]any) (*OQLResult, error) {
	resp, err := CallM2EE(opts, "preview_execute_oql", params)
	if err != nil {
		return nil, err
	}
	if errMsg := resp.M2EEError(); errMsg != "" {
		return nil, fmt.Errorf("OQL error: %s", errMsg)
	}
	return parseOQLFeedback(resp.RawFeedback)
}

// oqlDevError returns the message from a dev-endpoint error response, or "" when
// the body is a valid result. Two error shapes are handled:
//   - {"error":"..."} — a query error (bad OQL) reported by the preview servlet.
//   - {"result":<non-zero>,"message":"..."} — the admin dispatcher's response
//     when the request never reached the preview servlet, e.g. the OQL preview
//     servlet isn't mounted ("Action not found") because the app wasn't started
//     with the live-preview dev flags, or auth failed. A successful result carries
//     "data" and no "result"; without this check these would be silently parsed
//     as 0 rows.
func oqlDevError(raw json.RawMessage) string {
	msg, _ := oqlDevErrorKind(raw)
	return msg
}

// oqlDevErrorKind is oqlDevError plus whether the response means the /dev/ route
// is not mounted at all — the admin dispatcher's "Action not found", which is
// the signal to try the legacy action rather than to give up.
func oqlDevErrorKind(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var env struct {
		Error   string          `json:"error"`
		Result  *int            `json:"result"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", false
	}
	if env.Error != "" {
		return env.Error, false
	}
	if len(env.Data) == 0 && env.Result != nil && *env.Result != 0 {
		msg := env.Message
		if msg == "" {
			msg = fmt.Sprintf("runtime returned result %d", *env.Result)
		}
		absent := strings.Contains(strings.ToLower(msg), "action not found")
		// "Action not found" means the OQL preview servlet isn't mounted — the app
		// must be started with the live-preview dev flags (mxcli docker does this).
		// A common cause is a stale docker-compose.yml: `mxcli docker init` skips an
		// existing compose file, so a project generated before the flags were added
		// keeps starting the runtime without them until it is regenerated.
		if strings.Contains(strings.ToLower(msg), "not found") {
			msg += " -- the running app does not expose the OQL preview endpoint, which needs the live-preview dev flags at boot." +
				" If it was started with `mxcli run --local`, upgrade mxcli to a build that boots the local runtime with live preview (nightly-93 and earlier do not)." +
				" If it runs under docker and your .docker/ predates this fix, regenerate it with `mxcli docker init --force`, then `mxcli docker build && mxcli docker up`."
		}
		return msg, absent
	}
	return "", false
}

// parseOQLFeedback extracts OQL results from the raw M2EE feedback JSON,
// preserving column order from the response.
func parseOQLFeedback(rawFeedback json.RawMessage) (*OQLResult, error) {
	if len(rawFeedback) == 0 {
		return &OQLResult{}, nil
	}

	// Parse the feedback to extract the data field as raw JSON.
	//
	// The error field matters as much as the data one: the legacy admin action
	// reports a bad query as {"feedback":{"error":"..."},"result":0} — inside the
	// feedback, with a **successful** result code. M2EEError() keys off the
	// result, so it says nothing, and without this check the error body parses as
	// an empty result and a rejected query is reported as "0 rows". Measured
	// against 11.6.6: `select count(*)` without an alias comes back exactly that
	// way ("All OQL select columns must have a name").
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.Unmarshal(rawFeedback, &envelope); err != nil {
		return nil, fmt.Errorf("parsing feedback: %w", err)
	}

	if envelope.Error != "" {
		return nil, fmt.Errorf("OQL error: %s", envelope.Error)
	}

	if len(envelope.Data) == 0 {
		return &OQLResult{}, nil
	}

	var rows []json.RawMessage
	if err := json.Unmarshal(envelope.Data, &rows); err != nil {
		return nil, fmt.Errorf("parsing result data: %w", err)
	}

	result := &OQLResult{}

	if len(rows) == 0 {
		return result, nil
	}

	// The column set is the union of the keys of every row, not the keys of the
	// first one: the runtime omits a column from a row's JSON object when its
	// value is null, so a column that happens to be null in row 1 is absent
	// there and would otherwise be dropped from the whole result — silently
	// answering a different query than the one that was asked.
	var columns []string
	known := make(map[string]bool)
	rowMaps := make([]map[string]any, 0, len(rows))
	for _, rawRow := range rows {
		var rowMap map[string]any
		if err := json.Unmarshal(rawRow, &rowMap); err != nil {
			return nil, fmt.Errorf("parsing row: %w", err)
		}
		rowMaps = append(rowMaps, rowMap)

		// Re-scanning a row for key order is only needed when it carries a
		// column not seen yet; the common case (every row has the same keys)
		// costs one length check.
		if !hasOnlyKnownKeys(rowMap, known) {
			keys, err := extractColumnOrder(rawRow)
			if err != nil {
				return nil, fmt.Errorf("extracting columns: %w", err)
			}
			columns = mergeColumnOrder(columns, keys)
			for _, col := range columns {
				known[col] = true
			}
		}
	}
	result.Columns = columns

	// Project each row onto the merged column order. A column missing from a
	// row is a null value, which formats as NULL.
	for _, rowMap := range rowMaps {
		row := make([]any, len(columns))
		for i, col := range columns {
			row[i] = rowMap[col]
		}
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

// hasOnlyKnownKeys reports whether every key of rowMap is already a known column.
func hasOnlyKnownKeys(rowMap map[string]any, known map[string]bool) bool {
	if len(rowMap) > len(known) {
		return false
	}
	for key := range rowMap {
		if !known[key] {
			return false
		}
	}
	return true
}

// mergeColumnOrder folds one row's key order into the accumulated column list.
//
// New keys are inserted directly after the last key that was already known,
// rather than appended, so a column absent from earlier rows still lands in its
// SELECT position: merging [A, C] with [A, B, C] yields [A, B, C], not
// [A, C, B].
func mergeColumnOrder(columns []string, rowKeys []string) []string {
	index := make(map[string]int, len(columns))
	for i, col := range columns {
		index[col] = i
	}

	insertAt := 0 // just past the last key of this row found in columns
	for _, key := range rowKeys {
		if pos, ok := index[key]; ok {
			insertAt = pos + 1
			continue
		}
		columns = append(columns, "")
		copy(columns[insertAt+1:], columns[insertAt:])
		columns[insertAt] = key
		for i := insertAt; i < len(columns); i++ {
			index[columns[i]] = i
		}
		insertAt++
	}
	return columns
}

// extractColumnOrder uses json.Decoder to preserve key order from a JSON object.
func extractColumnOrder(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))

	// Read opening brace
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected '{', got %v", t)
	}

	var columns []string
	for dec.More() {
		// Read key
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := t.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key, got %T", t)
		}
		columns = append(columns, key)

		// Skip value
		var discard json.RawMessage
		if err := dec.Decode(&discard); err != nil {
			return nil, err
		}
	}

	return columns, nil
}

// FormatOQLTable writes an OQL result as a pipe-delimited table to w.
func FormatOQLTable(w io.Writer, result *OQLResult) {
	if len(result.Columns) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = len(col)
	}
	for _, row := range result.Rows {
		for i, val := range row {
			s := formatOQLValue(val)
			if len(s) > widths[i] {
				widths[i] = len(s)
			}
		}
	}

	// Cap column widths at 50 characters
	for i := range widths {
		if widths[i] > 50 {
			widths[i] = 50
		}
	}

	// Print header
	fmt.Fprint(w, "|")
	for i, col := range result.Columns {
		fmt.Fprintf(w, " %-*s |", widths[i], truncateOQL(col, widths[i]))
	}
	fmt.Fprintln(w)

	// Print separator
	fmt.Fprint(w, "|")
	for _, wid := range widths {
		fmt.Fprintf(w, "-%s-|", strings.Repeat("-", wid))
	}
	fmt.Fprintln(w)

	// Print rows
	for _, row := range result.Rows {
		fmt.Fprint(w, "|")
		for i, val := range row {
			s := formatOQLValue(val)
			fmt.Fprintf(w, " %-*s |", widths[i], truncateOQL(s, widths[i]))
		}
		fmt.Fprintln(w)
	}
}

// FormatOQLJSON writes an OQL result as a JSON array of objects to w.
func FormatOQLJSON(w io.Writer, result *OQLResult) error {
	objects := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		obj := make(map[string]any, len(result.Columns))
		for i, col := range result.Columns {
			obj[col] = row[i]
		}
		objects = append(objects, obj)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(objects)
}

// formatOQLValue formats a value for table display.
func formatOQLValue(val any) string {
	if val == nil {
		return "NULL"
	}
	s := fmt.Sprintf("%v", val)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// truncateOQL truncates a string to max length with ellipsis.
func truncateOQL(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
