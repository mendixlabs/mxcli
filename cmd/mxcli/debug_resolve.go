// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// debug_resolve.go turns a microflow name + activity selector into the model GUID
// the debugger uses as object_id, and keeps a small local record of the
// breakpoints mxcli has set (the runtime exposes no "list breakpoints" call).
// Slice 2 of the microflow debugger.

// activityInfo is one microflow object, with the model GUID the debugger wants as
// its object_id (mxcli's activity GetID() already yields the Microsoft-GUID /
// bytes_le form the debugger expects — see types.BlobToUUID).
type activityInfo struct {
	Index    int    // 1-based, in object-collection order
	Type     string // struct name, e.g. ActionActivity / ExclusiveSplit / StartEvent
	Caption  string // Mendix caption, e.g. "Create 'Game'" (empty for plain events)
	ObjectID string // model GUID == debugger object_id
}

// flowKind distinguishes a server microflow from a client nanoflow. It matters
// for the debugger: a nanoflow breakpoint uses the nanoflow_name param and a
// paused nanoflow surfaces only via poll_events (findings — nanoflow debugging).
type flowKind string

const (
	flowMicroflow flowKind = "microflow"
	flowNanoflow  flowKind = "nanoflow"
)

// resolveFlowActivities opens the project and lists a microflow's OR nanoflow's
// objects, auto-detecting which it is (microflow first, then nanoflow). The
// object-collection format is identical, so extractActivities handles both.
func resolveFlowActivities(projectPath, qualifiedName string) ([]activityInfo, flowKind, error) {
	// Through the backend rather than a concrete reader: this needs
	// ParseMicroflowBSON, which is a backend method, so the raw reads come from
	// the same place instead of straddling two readers.
	r := modelsdkbackend.New()
	if err := r.ConnectReadOnly(projectPath); err != nil {
		return nil, "", err
	}
	defer func() { _ = r.Disconnect() }()

	if contents, err := r.GetRawMicroflowByName(qualifiedName); err == nil {
		mf, err := r.ParseMicroflowBSON(contents, "", "")
		if err != nil {
			return nil, "", fmt.Errorf("parsing microflow %s: %w", qualifiedName, err)
		}
		return extractActivities(mf), flowMicroflow, nil
	}
	// Not a microflow — try a nanoflow (same ObjectCollection shape).
	if u, err := r.GetRawUnitByName("nanoflow", qualifiedName); err == nil && u != nil {
		nf, err := r.ParseMicroflowBSON(u.Contents, "", "")
		if err != nil {
			return nil, "", fmt.Errorf("parsing nanoflow %s: %w", qualifiedName, err)
		}
		return extractActivities(nf), flowNanoflow, nil
	}
	return nil, "", fmt.Errorf("no microflow or nanoflow named %s", qualifiedName)
}

// extractActivities flattens a microflow's object collection into activityInfo.
func extractActivities(mf *microflows.Microflow) []activityInfo {
	var out []activityInfo
	if mf == nil || mf.ObjectCollection == nil {
		return out
	}
	for i, o := range mf.ObjectCollection.Objects {
		typeName, caption := objectTypeCaption(o)
		out = append(out, activityInfo{
			Index:    i + 1,
			Type:     typeName,
			Caption:  caption,
			ObjectID: string(o.GetID()),
		})
	}
	return out
}

// objectTypeCaption returns a microflow object's struct name and its Caption (if
// the concrete type carries one — action activities, splits, loops, annotations
// do; bare start/end events do not).
func objectTypeCaption(o microflows.MicroflowObject) (typeName, caption string) {
	v := reflect.ValueOf(o)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return fmt.Sprintf("%T", o), ""
	}
	typeName = v.Type().Name()
	if f := v.FieldByName("Caption"); f.IsValid() && f.Kind() == reflect.String {
		caption = f.String()
	}
	return typeName, caption
}

// matchActivity selects one activity by an "#<index>" (1-based) or a
// case-insensitive caption substring that must match exactly one object.
func matchActivity(acts []activityInfo, selector string) (activityInfo, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return activityInfo{}, fmt.Errorf("empty --activity; use '#<index>' or a caption substring (see 'mxcli debug activities')")
	}
	if strings.HasPrefix(selector, "#") {
		n, err := strconv.Atoi(strings.TrimPrefix(selector, "#"))
		if err != nil || n < 1 || n > len(acts) {
			return activityInfo{}, fmt.Errorf("activity index %q out of range (1..%d)", selector, len(acts))
		}
		return acts[n-1], nil
	}
	low := strings.ToLower(selector)
	var matches []activityInfo
	for _, a := range acts {
		if a.Caption != "" && strings.Contains(strings.ToLower(a.Caption), low) {
			matches = append(matches, a)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return activityInfo{}, fmt.Errorf("no activity caption matches %q — run 'mxcli debug activities' or use --activity '#<index>'", selector)
	default:
		return activityInfo{}, fmt.Errorf("%q matches %d activities — be more specific or use --activity '#<index>'", selector, len(matches))
	}
}

// pausedFlowSummary is a best-effort extraction of one paused microflow from the
// get_paused_microflows result. Field names in the runtime response are not
// contract-stable across versions, so parsing is defensive and used only for the
// friendly summary + single-flow default — the full detail is always the raw JSON.
type pausedFlowSummary struct {
	DebugID   string
	Microflow string
}

// extractPausedFlows pulls {debug_id, microflow} pairs out of the paused-flows
// result, tolerating a top-level array or an array nested one level under an
// object key, and several field-name spellings.
func extractPausedFlows(raw []byte) []pausedFlowSummary {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	list := firstList(v)
	var out []pausedFlowSummary
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id := firstString(m, "debug_id", "debugId", "id")
		mf := firstString(m, "microflow_name", "microflowName", "microflow", "name")
		if id != "" || mf != "" {
			out = append(out, pausedFlowSummary{DebugID: id, Microflow: mf})
		}
	}
	return out
}

// extractPausedFromEvents pulls paused flows out of a poll_events response. A
// paused NANOFLOW does not appear in get_paused_microflows — it surfaces only as
// a poll_events entry {"type":"paused_microflow","data":{debug_id, microflow_name,
// …}} (the data field is named microflow_name even for a nanoflow). Parsed
// defensively: any object anywhere with type=="paused_microflow" and a data map.
func extractPausedFromEvents(raw []byte) []pausedFlowSummary {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	var out []pausedFlowSummary
	var walk func(any)
	walk = func(n any) {
		switch t := n.(type) {
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			if s, _ := t["type"].(string); s == "paused_microflow" {
				if data, ok := t["data"].(map[string]any); ok {
					id := firstString(data, "debug_id", "debugId", "id")
					mf := firstString(data, "microflow_name", "nanoflow_name", "microflowName", "name")
					if id != "" || mf != "" {
						out = append(out, pausedFlowSummary{DebugID: id, Microflow: mf})
					}
				}
			}
			for _, val := range t {
				walk(val)
			}
		}
	}
	walk(v)
	return out
}

// firstList returns v if it is an array, else the first array value found among a
// map's values (one level deep).
func firstList(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	if m, ok := v.(map[string]any); ok {
		for _, val := range m {
			if a, ok := val.([]any); ok {
				return a
			}
		}
	}
	return nil
}

// firstString returns the first non-empty string value among the given keys.
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// localBreakpoint is one breakpoint mxcli has set, recorded so 'breaks' can show
// the name→GUID reverse map (the runtime has no read-back).
type localBreakpoint struct {
	Microflow string `json:"microflow"`
	Activity  string `json:"activity"` // caption or type#index, for display
	ObjectID  string `json:"objectId"`
	Condition string `json:"condition,omitempty"`
}

// loadBreakpoints reads the local breakpoint record (missing file = empty).
func loadBreakpoints(path string) ([]localBreakpoint, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var bps []localBreakpoint
	if err := json.Unmarshal(b, &bps); err != nil {
		return nil, err
	}
	return bps, nil
}

// saveBreakpoints writes the local breakpoint record.
func saveBreakpoints(path string, bps []localBreakpoint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(bps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
