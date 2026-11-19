// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// A CheckFinding is one problem `mx check` reports. It is the unit a baseline
// compares (PROPOSAL_verify_gate.md): the text of `mx check` is not stable
// between runs and does not identify the document (two microflows with the same
// defect print the same line), so findings are read from `mx check --json`,
// which names the module, document, element and unit by name and by ID.
type CheckFinding struct {
	Severity  string // "error", "warning" or "deprecation": the JSON list it came from
	Code      string // "CE0109"
	Message   string // display only, never part of the Key
	Locations []CheckLocation
	Stage     int // MxBuild report stage 0..3, see StageOf
}

// CheckLocation is where a finding is. IDs are what a baseline matches on; the
// names are for a reader and change when an element is renamed.
type CheckLocation struct {
	ModuleName   string `json:"module-name"`
	DocumentName string `json:"document-name"` // "Microflow 'F05'"
	ElementName  string `json:"element-name"`  // "End event"
	ElementID    string `json:"element-id"`
	UnitID       string `json:"unit-id"`
	ModuleID     string `json:"module-id"`
}

// Identified says whether the finding names an element by ID. One without a
// location (a project-level finding, such as the runtime settings) has nothing
// to key on but its code, so a baseline matches it by count per Key.
func (f CheckFinding) Identified() bool {
	for _, l := range f.Locations {
		if l.ElementID != "" || l.UnitID != "" {
			return true
		}
	}
	return false
}

// Key is what a baseline matches on: severity, code and the IDs of every
// location, never the message or a name. Two findings with the same Key are the
// same finding repeated, and a baseline compares how many of them there are.
func (f CheckFinding) Key() string {
	var ids []string
	for _, l := range f.Locations {
		ids = append(ids, l.UnitID+"/"+l.ElementID)
	}
	sort.Strings(ids)
	return f.Severity + "\x00" + f.Code + "\x00" + strings.Join(ids, ",")
}

// mxStages is MxBuild's report stage per code, as measured in
// PROPOSAL_check_diagnostics_catalog.md ("MxBuild reports errors in stages",
// MxBuild 11.16.1, microflows and pages only). An earlier stage hides the later
// ones: 0 everything, 1 and 2 the rest of the same microflow.
var mxStages = map[string]int{
	"CE1613": 0,
	"CE0111": 1,
	"CE0109": 2,
	"CE0117": 3,
	"CE0068": 3,
	"CE0038": 3,
	"CE0067": 3,
}

// StageOf is the MxBuild stage of code. A code outside the measured set is stage
// 3: it hides nothing and anything earlier may hide it, which is the cautious
// reading for a baseline diff.
func StageOf(code string) int {
	if s, ok := mxStages[code]; ok {
		return s
	}
	return 3
}

// checkJSONVersion is the `serialization_version` this reader understands.
const checkJSONVersion = 1

type checkJSONProblem struct {
	Code      string          `json:"code"`
	Message   string          `json:"message"`
	Locations []CheckLocation `json:"locations"`
}

type checkJSON struct {
	Version      int                `json:"serialization_version"`
	Errors       []checkJSONProblem `json:"errors"`
	Warnings     []checkJSONProblem `json:"warnings"`
	Deprecations []checkJSONProblem `json:"deprecations"`
	Total        *int               `json:"total-problems"`
}

// CheckReport is the parsed result of one `mx check --json` run.
type CheckReport struct {
	// MxPath is the mx binary that produced the report, so a caller can record
	// which checker (and version) a baseline belongs to.
	MxPath   string
	Findings []CheckFinding
	// Declared is the total mx printed, HasDeclared false when it printed none.
	Declared    int
	HasDeclared bool
}

// Count returns the findings of one severity.
func (r CheckReport) Count(severity string) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == severity {
			n++
		}
	}
	return n
}

// Verify cross-checks the findings against the total mx declared. A mismatch
// means a list this reader does not know about (or a format change), and a
// baseline built from it would silently omit findings, so a caller must not
// treat the report as complete.
func (r CheckReport) Verify() error {
	if !r.HasDeclared {
		return fmt.Errorf("mx check --json did not declare a total; the report may be incomplete")
	}
	if len(r.Findings) != r.Declared {
		return fmt.Errorf("read %d findings but mx check declared %d; the output format may have changed",
			len(r.Findings), r.Declared)
	}
	return nil
}

// ParseCheckJSON reads the file `mx check --json <path>` writes. It refuses a
// serialization_version it has not seen rather than guess at its fields.
func ParseCheckJSON(data []byte) (CheckReport, error) {
	var j checkJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return CheckReport{}, fmt.Errorf("read mx check json: %w", err)
	}
	if j.Version != checkJSONVersion {
		return CheckReport{}, fmt.Errorf("mx check json has serialization_version %d; this reader understands %d",
			j.Version, checkJSONVersion)
	}
	var r CheckReport
	add := func(severity string, ps []checkJSONProblem) {
		for _, p := range ps {
			r.Findings = append(r.Findings, CheckFinding{
				Severity: severity, Code: p.Code, Message: p.Message,
				Locations: p.Locations, Stage: StageOf(p.Code),
			})
		}
	}
	add("error", j.Errors)
	add("warning", j.Warnings)
	add("deprecation", j.Deprecations)
	if j.Total != nil {
		r.Declared, r.HasDeclared = *j.Total, true
	}
	return r, nil
}

// CheckKinds selects the findings beyond errors, which mx always reports.
type CheckKinds struct {
	Warnings, Deprecations, BestPractice bool
}

func (k CheckKinds) args(jsonPath string) []string {
	args := []string{"-j", jsonPath}
	if k.Warnings {
		args = append(args, "-w")
	}
	if k.Deprecations {
		args = append(args, "-d")
	}
	if k.BestPractice {
		args = append(args, "-b")
	}
	return args
}

// CheckFindings runs `mx check --json` on a temporary copy of the project and
// returns the parsed report. It checks the project as stored (no
// update-widgets) and never modifies it. mx exits non-zero whenever it finds
// anything, so a run error is returned alongside the report; it is a failure to
// run only when the report is empty and Verify fails.
func CheckFindings(opts CheckOptions, kinds CheckKinds) (CheckReport, error) {
	projectVersion := ""
	if opts.ProjectPath != "" {
		if reader, err := openReadOnly(opts.ProjectPath); err == nil {
			projectVersion = reader.ProjectVersion().ProductVersion
			reader.Disconnect()
		}
	}
	mxPath, err := resolveMxForCheck(opts.MxBuildPath, projectVersion)
	if err != nil {
		return CheckReport{}, err
	}
	// The report goes outside the temporary copy, which MxCheckOnCopy removes.
	f, err := os.CreateTemp("", "mxcheck-*.json")
	if err != nil {
		return CheckReport{}, err
	}
	jsonPath := f.Name()
	f.Close()
	defer os.Remove(jsonPath)

	var out, errOut bytes.Buffer
	runErr := MxCheckOnCopy(mxPath, opts.ProjectPath, kinds.args(jsonPath), &out, &errOut)
	data, readErr := os.ReadFile(jsonPath)
	if readErr != nil || len(bytes.TrimSpace(data)) == 0 {
		if runErr != nil {
			return CheckReport{}, fmt.Errorf("mx check wrote no json report: %w\n%s", runErr, strings.TrimSpace(errOut.String()))
		}
		return CheckReport{}, fmt.Errorf("mx check wrote no json report")
	}
	report, err := ParseCheckJSON(data)
	if err != nil {
		return CheckReport{}, err
	}
	report.MxPath = mxPath
	return report, runErr
}
