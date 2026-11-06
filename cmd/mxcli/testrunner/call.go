// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CallCase is one runtime case for the endpoint's `call` route: a microflow
// call with parameters built from JSON, expectations on the actual values, and
// a rollback. Nothing is generated or built; the case is sent as data to the
// running app.
type CallCase struct {
	// Raw is the case exactly as written. It is what is sent, because the order
	// of the keys in "params" is the order the objects are built in (a later
	// parameter can $ref an earlier one), and a Go map loses it.
	Raw json.RawMessage `json:"-"`

	Name     string           `json:"name,omitempty"`
	Flow     string           `json:"flow"`
	Params   map[string]any   `json:"params,omitempty"`
	Setup    []map[string]any `json:"setup,omitempty"`
	Expect   []map[string]any `json:"expect,omitempty"`
	Throws   *string          `json:"throws,omitempty"`
	Rollback *bool            `json:"rollback,omitempty"`
}

type callCheck struct {
	Path     string `json:"path"`
	Op       string `json:"op"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Pass     bool   `json:"pass"`
}

type callResponse struct {
	Pass              bool        `json:"pass"`
	DurationMicros    int64       `json:"durationMicros"`
	FlowMicros        int64       `json:"flowMicros"`
	Result            string      `json:"result"`
	Error             string      `json:"error"`
	RollbackRequested bool        `json:"rollbackRequested"`
	RolledBack        bool        `json:"rolledBack"`
	Checks            []callCheck `json:"checks"`
}

// ParseCallCases reads a JSON array of cases, or a single case object.
func ParseCallCases(r io.Reader) ([]CallCase, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	s := strings.TrimSpace(string(body))
	var raws []json.RawMessage
	if strings.HasPrefix(s, "[") {
		err = json.Unmarshal([]byte(s), &raws)
	} else {
		raws = []json.RawMessage{json.RawMessage(s)}
	}
	if err != nil {
		return nil, fmt.Errorf("reading cases: %w", err)
	}
	var cases []CallCase
	for i, raw := range raws {
		var c CallCase
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("reading case %d: %w", i+1, err)
		}
		if c.Flow == "" {
			return nil, fmt.Errorf("case %d has no \"flow\"", i+1)
		}
		c.Raw = raw
		cases = append(cases, c)
	}
	return cases, nil
}

// RunCalls sends each case to the app hosting the test endpoint for the
// project, prints pass/fail with expected and actual, and reports whether all
// passed. repeat > 1 runs the list again and prints only the timing, to show
// the per-case latency of a warm app.
func RunCalls(projectPath string, cases []CallCase, repeat int, out io.Writer) (bool, error) {
	h, err := ReadHandshake(projectPath)
	if err != nil {
		return false, err
	}
	if err := checkModelNotStale(projectPath); err != nil {
		return false, err
	}
	c := newEndpointClient(h.AppPort, h.Token)
	if repeat < 1 {
		repeat = 1
	}
	allPass := true
	for round := 0; round < repeat; round++ {
		var total time.Duration
		for _, cs := range cases {
			b := []byte(cs.Raw)
			t0 := time.Now()
			var rr callResponse
			if err := c.get("call", url.Values{"case": {string(b)}}, &rr); err != nil {
				return false, err
			}
			wall := time.Since(t0)
			total += wall
			if round > 0 {
				if !rr.Pass {
					allPass = false
				}
				continue
			}
			name := cs.Name
			if name == "" {
				name = cs.Flow
			}
			status := "PASS"
			if !rr.Pass {
				status = "FAIL"
				allPass = false
			}
			fmt.Fprintf(out, "%s  %s  (%d ms wall, %d ms in app, flow %d ms)\n", status, name,
				wall.Milliseconds(), rr.DurationMicros/1000, rr.FlowMicros/1000)
			if rr.Error != "" {
				fmt.Fprintf(out, "      error: %s\n", rr.Error)
			}
			for _, k := range rr.Checks {
				mark := "ok  "
				if !k.Pass {
					mark = "FAIL"
				}
				fmt.Fprintf(out, "      %s %s %s %s   actual: %s\n", mark, k.Path, k.Op, k.Expected, k.Actual)
			}
			if cs.Rollback == nil || *cs.Rollback {
				if !rr.RolledBack {
					fmt.Fprintf(out, "      WARNING: rollback was requested but did not complete\n")
				}
			}
		}
		if repeat > 1 || round > 0 {
			fmt.Fprintf(out, "round %d: %d cases in %d ms (%.1f ms per case)\n", round+1, len(cases),
				total.Milliseconds(), float64(total.Microseconds())/1000/float64(len(cases)))
		}
	}
	return allPass, nil
}

// checkModelNotStale refuses to call into an app whose model was edited after
// the build it is running. The running app answers from the model it was built
// from, so a green result after a model edit would be a stale green. Compares
// the newest modification under the project's model files (App.mpr, mprcontents,
// javasource) with the built model (deployment/model/model.mdp).
func checkModelNotStale(projectPath string) error {
	dir := filepath.Dir(projectPath)
	built, err := os.Stat(filepath.Join(dir, "deployment", "model", "model.mdp"))
	if err != nil {
		return nil // no build to compare with (Docker, or a moved deployment): nothing to say
	}
	var newest time.Time
	var newestPath string
	note := func(p string, info fs.FileInfo) {
		if info.ModTime().After(newest) {
			newest, newestPath = info.ModTime(), p
		}
	}
	if info, err := os.Stat(projectPath); err == nil {
		note(projectPath, info)
	}
	for _, sub := range []string{"mprcontents", "javasource"} {
		_ = filepath.WalkDir(filepath.Join(dir, sub), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if info, err := d.Info(); err == nil {
				note(p, info)
			}
			return nil
		})
	}
	if newest.After(built.ModTime()) {
		return fmt.Errorf("the model changed after the running app was built (%s, %s after the build)\n"+
			"  a call now would test the OLD model and could report a stale green.\n"+
			"  Rebuild and restart first: mxcli run restart -p %s", filepath.Base(newestPath),
			newest.Sub(built.ModTime()).Round(time.Second), projectPath)
	}
	return nil
}
