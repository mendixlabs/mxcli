// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/cmd/mxcli/verify"
)

var verifyCmd = &cobra.Command{
	Use:   "verify [script.mdl...]",
	Short: "One gate: check, lint and mx check, reporting only what a change made worse",
	Long: `Run the checks that answer "did my change break anything?" in one command, in
cost order, and report only the findings that are new against a baseline.

  check     mxcli check on the scripts given as arguments (skipped without any)
  lint      mxcli lint on the project, against its baseline
  mxcheck   mx check on a copy of the project, against its baseline

Every selected stage runs, so one result carries every stage's findings; use
--fail-fast to stop at the first stage that fails. mx check is the authority:
mxcli check cannot yet predict everything it rejects, so a project that passes
check and lint can still fail mxcheck.

Baselines live in --baseline-dir (default verify-baseline/ next to the project):
lint.json is a ` + "`mxcli lint --baseline`" + ` file, mxcheck.json records mx check's
findings by their element IDs. Without one, every finding is reported as new and
the stage says so. Record the current state with --update-baseline (commit the
files); the run then reports what it recorded and exits 0.

mx check reports errors in stages: an earlier-stage error hides later ones. A
finding that appears only because the baseline's earlier error was fixed is
"unmasked" and does not fail the gate (--strict makes it); a baseline finding
that disappears only because a new earlier-stage error hides it is "hidden", not
resolved.

Exit codes:
  0       nothing new
  1       usage error
  2       a stage could not run (no mx, ...), and nothing failed
  16+bits a stage failed; the bits say which: 1 check, 2 lint, 4 mxcheck
          (18 = lint failed, 22 = lint and mxcheck failed)

--format json prints one document: a list of stages with status, duration and
every new finding of each, and the exit code. The text output lists only the new
findings that fail the gate at --fail-on and counts the rest by severity, with the
command that accepts them.

Examples:
  mxcli verify -p app.mpr --update-baseline     # record the current state
  mxcli verify -p app.mpr                       # lint + mx check against it
  mxcli verify -p app.mpr script.mdl            # also check the script
  mxcli verify -p app.mpr --stages lint --format json
`,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(runVerify(cmd, args))
	},
}

func init() {
	verifyCmd.Flags().StringSlice("stages", verify.AllStages, "Stages to run: check, lint, mxcheck")
	verifyCmd.Flags().Bool("fail-fast", false, "Stop at the first stage that fails instead of running every stage")
	verifyCmd.Flags().String("fail-on", "error", "Lowest severity of a new finding that fails the gate: error or warning")
	verifyCmd.Flags().Bool("strict", false, "Also fail on unmasked mx check findings (errors an earlier baseline error was hiding)")
	verifyCmd.Flags().String("baseline-dir", "", "Where the baselines live (default verify-baseline/ next to the project)")
	verifyCmd.Flags().Bool("update-baseline", false, "Record the current lint and mx check findings as the baseline instead of comparing")
	verifyCmd.Flags().String("mxbuild-path", "", "Path to MxBuild/Mendix installation (used to find mx)")
	verifyCmd.Flags().StringP("format", "f", "text", "Output format: text, json")
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(cmd *cobra.Command, scripts []string) int {
	projectPath, _ := cmd.Flags().GetString("project")
	format := resolveFormat(cmd, "text")
	stagesFlag, _ := cmd.Flags().GetStringSlice("stages")
	failFast, _ := cmd.Flags().GetBool("fail-fast")
	failOn, _ := cmd.Flags().GetString("fail-on")
	strict, _ := cmd.Flags().GetBool("strict")
	baselineDir, _ := cmd.Flags().GetString("baseline-dir")
	update, _ := cmd.Flags().GetBool("update-baseline")
	mxbuildPath, _ := cmd.Flags().GetString("mxbuild-path")

	usage := func(format string, a ...any) int {
		fmt.Fprintf(os.Stderr, "Error: "+format+"\n", a...)
		return verify.ExitUsage
	}
	if projectPath == "" {
		return usage("--project (-p) is required")
	}
	if format != "text" && format != "json" {
		return usage("--format %q is not supported by verify (text, json)", format)
	}
	if failOn != "error" && failOn != "warning" {
		return usage("--fail-on takes error or warning, not %q", failOn)
	}
	stages, err := verify.ParseStages(stagesFlag)
	if err != nil {
		return usage("%v", err)
	}
	if baselineDir == "" {
		baselineDir = defaultVerifyBaselineDir(projectPath)
	}

	skip := map[string]string{}
	if len(scripts) == 0 {
		skip[verify.StageCheck] = "no scripts given"
	}
	self, err := os.Executable()
	if err != nil {
		return usage("cannot locate this executable: %v", err)
	}
	vr := &verifyRunner{self: self, project: projectPath, baselineDir: baselineDir, update: update, mxbuildPath: mxbuildPath, scripts: scripts}
	res := verify.Run(verify.Options{
		Project: projectPath, Stages: stages, FailFast: failFast, FailOn: failOn, Strict: strict, Skip: skip,
	}, map[string]verify.Stage{
		verify.StageCheck:   vr.check,
		verify.StageLint:    vr.lint,
		verify.StageMxCheck: vr.mxcheck,
	})
	if format == "json" {
		if err := verify.WriteJSON(os.Stdout, res); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return verify.ExitUsage
		}
	} else {
		verify.WriteText(os.Stdout, res, failOn)
	}
	return res.Exit
}

// defaultVerifyBaselineDir is where the baselines live unless --baseline-dir says
// otherwise. It is deliberately NOT under .mxcli/: `mxcli init` git-ignores that
// directory (it holds machine-local state such as constants), and a baseline that
// is not committed is gone with the next fresh clone or reaped session, after
// which every run reports every finding as new.
func defaultVerifyBaselineDir(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), "verify-baseline")
}

type verifyRunner struct {
	self, project, baselineDir, mxbuildPath string
	update                                  bool
	scripts                                 []string
}

// sub runs this executable with args and returns its stdout. Its stderr is
// returned too and echoed to ours, which is where progress belongs while stdout
// carries the result.
func (v *verifyRunner) sub(args ...string) (stdout, stderr []byte, err error) {
	c := exec.Command(v.self, args...)
	var o, e bytes.Buffer
	c.Stdout, c.Stderr = &o, &e
	err = c.Run()
	os.Stderr.Write(e.Bytes())
	return o.Bytes(), e.Bytes(), err
}

func (v *verifyRunner) check() (verify.Outcome, error) {
	args := append([]string{"check"}, v.scripts...)
	args = append(args, "-p", v.project, "--format", "json")
	stdout, stderr, err := v.sub(args...)
	findings, perr := parseViolations(verify.StageCheck, stdout)
	if perr != nil {
		return verify.Outcome{}, fmt.Errorf("mxcli check produced no readable report: %v%s", perr, verifyTail(stderr, err))
	}
	return verify.Outcome{New: findings}, nil
}

func (v *verifyRunner) lint() (verify.Outcome, error) {
	lintBaseline := filepath.Join(v.baselineDir, "lint.json")
	args := []string{"lint", "-p", v.project, "--format", "json"}
	var note string
	switch {
	case v.update:
		if err := os.MkdirAll(v.baselineDir, 0o755); err != nil {
			return verify.Outcome{}, err
		}
		args = append(args, "--baseline", lintBaseline, "--update-baseline")
	case verifyFileExists(lintBaseline):
		args = append(args, "--baseline", lintBaseline)
	default:
		note = "no lint baseline (" + lintBaseline + "): every finding is reported as new; record one with --update-baseline"
	}
	stdout, stderr, err := v.sub(args...)
	findings, perr := parseViolations(verify.StageLint, stdout)
	if perr != nil {
		return verify.Outcome{}, fmt.Errorf("mxcli lint produced no readable report: %v%s", perr, verifyTail(stderr, err))
	}
	if v.update {
		return verify.Outcome{Recorded: true, PreExisting: len(findings), Note: "baseline recorded at " + lintBaseline}, nil
	}
	pre, _ := parseLintBaselineSummary(string(stderr))
	return verify.Outcome{New: findings, PreExisting: pre, Note: note}, nil
}

func (v *verifyRunner) mxcheck() (verify.Outcome, error) {
	report, runErr := docker.CheckFindings(docker.CheckOptions{ProjectPath: v.project, MxBuildPath: v.mxbuildPath}, docker.CheckKinds{})
	if report.MxPath == "" || report.Verify() != nil {
		if runErr == nil {
			runErr = report.Verify()
		}
		return verify.Outcome{}, fmt.Errorf("mx check could not be run or read: %v (`mxcli setup mxbuild -p %s` installs the project's version)", runErr, v.project)
	}
	current := mxFindings(report)
	version := verifyMxVersionOf(report.MxPath)
	path := filepath.Join(v.baselineDir, "mxcheck.json")
	if v.update {
		if err := verify.WriteMxBaseline(path, "mx check", version, current); err != nil {
			return verify.Outcome{}, err
		}
		return verify.Outcome{Recorded: true, PreExisting: len(current), Note: "baseline recorded at " + path}, nil
	}
	base, err := verify.LoadMxBaseline(path)
	if err != nil {
		return verify.Outcome{}, err
	}
	if base == nil {
		return verify.Outcome{New: current, Note: "no mx check baseline (" + path + "): every finding is reported as new; record one with --update-baseline"}, nil
	}
	if err := base.CheckCompatible("mx check", version); err != nil {
		return verify.Outcome{}, err
	}
	return verify.Classify(base.Findings, current, true), nil
}

// mxFindings turns the errors of an mx check report into gate findings. Warnings
// and deprecations are not part of the gate: mx check exits non-zero for errors.
func mxFindings(r docker.CheckReport) []verify.Finding {
	var out []verify.Finding
	for _, f := range r.Findings {
		if f.Severity != "error" {
			continue
		}
		g := verify.Finding{Stage: verify.StageMxCheck, Code: f.Code, Severity: "error", Message: f.Message, Key: f.Key(), MxStage: f.Stage}
		if len(f.Locations) > 0 {
			g.Document, g.Element, g.Unit = f.Locations[0].DocumentName, f.Locations[0].ElementName, f.Locations[0].UnitID
			if f.Locations[0].ModuleName != "" {
				g.Document = f.Locations[0].ModuleName + "." + g.Document
			}
		}
		out = append(out, g)
	}
	return out
}

var verifyMxVersionRe = regexp.MustCompile(`(\d+\.\d+\.\d+(?:\.\d+)?)`)

// verifyMxVersionOf reads the version out of the path of a cached mx
// (~/.mxcli/mxbuild/11.16.1/modeler/mx); "" when the path has none.
func verifyMxVersionOf(mxPath string) string {
	for _, part := range strings.Split(filepath.ToSlash(mxPath), "/") {
		if m := verifyMxVersionRe.FindString(part); m != "" && m == part {
			return m
		}
	}
	return ""
}

// violationsJSON is the shape `check --format json` and `lint --format json` print.
type violationsJSON struct {
	Violations []struct {
		RuleID       string `json:"ruleId"`
		Severity     string `json:"severity"`
		Message      string `json:"message"`
		Document     string `json:"document"`
		DocumentType string `json:"documentType"`
		Suggestion   string `json:"suggestion"`
	} `json:"violations"`
}

func parseViolations(stage string, data []byte) ([]verify.Finding, error) {
	var j violationsJSON
	if err := json.Unmarshal(bytes.TrimSpace(data), &j); err != nil {
		return nil, err
	}
	out := []verify.Finding{}
	for _, v := range j.Violations {
		out = append(out, verify.Finding{
			Stage: stage, Code: v.RuleID, Severity: v.Severity, Document: v.Document,
			Message: v.Message, Suggestion: v.Suggestion, Key: v.RuleID + "\x00" + v.Document, MxStage: -1,
		})
	}
	return out, nil
}

var lintBaselineRe = regexp.MustCompile(`Lint baseline .*: (\d+) new, (\d+) already known`)

// parseLintBaselineSummary reads lint's own "Lint baseline F: N new, M already
// known[, K fixed]" line, which is where the count of pre-existing findings is.
func parseLintBaselineSummary(stderr string) (preExisting int, ok bool) {
	m := lintBaselineRe.FindStringSubmatch(stderr)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[2])
	return n, err == nil
}

func verifyFileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func verifyTail(stderr []byte, err error) string {
	s := strings.TrimSpace(string(stderr))
	if len(s) > 400 {
		s = s[len(s)-400:]
	}
	if s == "" && err != nil {
		return " (" + err.Error() + ")"
	}
	if s == "" {
		return ""
	}
	return "\n" + s
}
