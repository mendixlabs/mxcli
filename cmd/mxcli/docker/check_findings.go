// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// A CheckFinding is one diagnostic line of `mx check`, parsed. It is the unit a
// baseline compares (PROPOSAL_verify_gate.md): the text of `mx check` is not
// stable between runs, so a caller compares Keys and counts, never lines.
//
// mx check prints one line per finding:
//
//	[error] [CE0117] "Error(s) in expression." at Microflow 'ACT_X'
type CheckFinding struct {
	Severity string // "error", "warning" or "deprecation", as mx prints it
	Code     string // "CE0117"
	Message  string // the quoted text; display only, never part of the Key
	Location string // text after `" at `, or "" when the line names none
	Kind     string // "Microflow", "Data grid 2", ... (the Location before its quoted name)
	Name     string // the quoted name in Location; "" when empty or absent
	Class    LocationClass
	Stage    int // MxBuild report stage 0..3, see StageOf
	Line     string
}

// LocationClass says how much of "which element" a finding's text carries,
// which decides how a baseline can match it (PROPOSAL_verify_gate.md,
// "Finding identity").
type LocationClass int

const (
	// Located: the line names the kind and name of the element.
	Located LocationClass = iota
	// DocumentOnly: the line names a document, not the element in it.
	DocumentOnly
	// Unlocated: the line names nothing that identifies it. Matched as a count
	// per distinct line, as recheckCE0463 already does.
	Unlocated
)

func (c LocationClass) String() string {
	switch c {
	case Located:
		return "located"
	case DocumentOnly:
		return "document-only"
	default:
		return "unlocated"
	}
}

// Key is what a baseline matches on: severity, code and location, never the
// message. For an Unlocated finding the location is the whole line's text, so
// the same line repeated is the same key and the caller counts it.
func (f CheckFinding) Key() string {
	return f.Severity + "\x00" + f.Code + "\x00" + f.Class.String() + "\x00" + f.Location
}

var findingRe = regexp.MustCompile(`^\[(error|warning|deprecation)\]\s+\[(C[EWD]\d+)\]\s+"`)

// locationRe splits "Microflow 'ACT_X'" into kind and name.
var locationRe = regexp.MustCompile(`^(.*?)\s*'(.*)'$`)

// ParseCheckOutput reads the diagnostics out of `mx check` output. Lines that
// are not findings (banners, the closing "The app contains: N errors.") are
// skipped. A finding the parser cannot split still counts: its Message is the
// rest of the line.
func ParseCheckOutput(output string) []CheckFinding {
	var out []CheckFinding
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		m := findingRe.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		f := CheckFinding{Severity: line[m[2]:m[3]], Code: line[m[4]:m[5]], Line: line}
		rest := line[m[1]:] // after the opening quote
		if i := strings.LastIndex(rest, `" at `); i >= 0 {
			f.Message, f.Location = rest[:i], strings.TrimSpace(rest[i+len(`" at `):])
		} else {
			f.Message = strings.TrimSuffix(rest, `"`)
		}
		if lm := locationRe.FindStringSubmatch(f.Location); lm != nil {
			f.Kind, f.Name = strings.TrimSpace(lm[1]), lm[2]
		} else {
			f.Kind = f.Location
		}
		f.Class = classify(f)
		f.Stage = StageOf(f.Code)
		out = append(out, f)
	}
	return out
}

// unlocatedCodes are codes whose text does not name the element, so two
// same-named elements on different documents print the same line. CE0463 does
// not name its page (docker/check.go, recheckCE0463).
var unlocatedCodes = map[string]bool{"CE0463": true}

// documentKinds are the kinds whose name is a document, so the line says which
// document but not what in it. A kind not listed names an element (a widget,
// an activity) and counts as Located. The list is the measured set, not a
// claim of completeness: an unlisted document kind errs towards Located.
var documentKinds = map[string]bool{
	"Microflow":              true,
	"Nanoflow":               true,
	"Page":                   true,
	"Snippet":                true,
	"Layout":                 true,
	"Domain model of module": true,
	"Enumeration":            true,
	"Workflow":               true,
	"Java action":            true,
	"Rule":                   true,
	"Constant":               true,
	"Import mapping":         true,
	"Export mapping":         true,
	"Published REST service": true,
	"Consumed REST service":  true,
}

func classify(f CheckFinding) LocationClass {
	switch {
	case unlocatedCodes[f.Code], f.Location == "", f.Name == "":
		return Unlocated
	case documentKinds[f.Kind]:
		return DocumentOnly
	default:
		return Located
	}
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

var appErrorsRe = regexp.MustCompile(`The app contains: (\d+) errors?\.`)

// CheckReport is the parsed result of one `mx check` run.
type CheckReport struct {
	Findings []CheckFinding
	// Declared is the error count mx printed on its closing line; HasDeclared
	// is false when that line is absent (mx did not complete).
	Declared    int
	HasDeclared bool
}

// Errors counts the findings of severity error.
func (r CheckReport) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == "error" {
			n++
		}
	}
	return n
}

// Verify cross-checks the parse against the count mx printed. A mismatch means
// the parser lost lines (or mx changed its format), and a baseline built from it
// would silently hide findings, so a caller must not treat the report as
// complete.
func (r CheckReport) Verify() error {
	if !r.HasDeclared {
		return fmt.Errorf("mx check did not print its closing error count; the report may be incomplete")
	}
	if got := r.Errors(); got != r.Declared {
		return fmt.Errorf("parsed %d errors but mx check reported %d; the output format may have changed", got, r.Declared)
	}
	return nil
}

// ParseCheckReport parses output and reads mx's own error count.
func ParseCheckReport(output string) CheckReport {
	r := CheckReport{Findings: ParseCheckOutput(output)}
	if m := appErrorsRe.FindStringSubmatch(output); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			r.Declared, r.HasDeclared = n, true
		}
	}
	return r
}

// CheckFindings runs `mx check` on a temporary copy of the project and returns
// its parsed report. It checks the project as stored (no update-widgets), never
// modifies it, and returns the report together with mx's own exit error, which
// is non-zero whenever the project has errors: a non-nil error is not a failure
// to run unless the report is empty and Verify fails.
func CheckFindings(opts CheckOptions) (CheckReport, error) {
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
	var out, errOut bytes.Buffer
	runErr := MxCheckOnCopy(mxPath, opts.ProjectPath, nil, &out, &errOut)
	return ParseCheckReport(out.String() + "\n" + errOut.String()), runErr
}
