// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A decision's two outgoing flows each keep their own curve (ako/mxcli#2592).
// One @curve per decision was stamped on both, so a true edge curved to the
// right bent the vertical false edge into an S, and DESCRIBE, which printed
// one, could not say "the true edge is curved and the false one is straight".
const outcomeCurveSrc = `create microflow Mod.ACT_curve_on_split ($Flag: Boolean)
begin
  @start(0, 200)
  @position(160, 200)
  @anchor(true: (from: right), false: (from: bottom))
  @curve(true: (from: (15, 0), to: (-30, 0)))
  if $Flag then
    @position(320, 200)
    log info node 'Repro' 'true';
    @position(480, 200)
    return;
  else
    @position(160, 320)
    @anchor(to: top)
    log info node 'Repro' 'false';
    @position(160, 440)
    @anchor(to: top)
    return;
  end if;
end;`

func splitOutcomeVectors(t *testing.T, oc *microflows.MicroflowObjectCollection) map[string]string {
	t.Helper()
	var split model.ID
	for _, o := range oc.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok {
			split = s.ID
		}
	}
	got := map[string]string{}
	for _, f := range oc.Flows {
		if f.OriginID == split {
			got[flowBranchKey(f)] = f.OriginControlVector + " / " + f.DestinationControlVector
		}
	}
	return got
}

func TestDecisionOutcomeCurve_2592(t *testing.T) {
	oc := buildMicroflowFromMDL(t, outcomeCurveSrc)
	got := splitOutcomeVectors(t, oc)
	if got["'true'"] != "15;0 / -30;0" {
		t.Errorf("true edge vectors %q, want 15;0 / -30;0", got["'true'"])
	}
	if f := got["'false'"]; f != "0;0 / 0;0" && f != " / " {
		t.Errorf("false edge vectors %q, want straight: the true outcome's curve was stamped on it", f)
	}

	// DESCRIBE says the same, and its output rebuilds to the same vectors.
	desc, _ := describeBuilt(t, outcomeCurveSrc)
	if !strings.Contains(desc, "@curve(true: (from: (15, 0), to: (-30, 0)))") {
		t.Fatalf("describe does not carry the per-outcome curve:\n%s", desc)
	}
	again := buildMicroflowFromMDL(t, "create microflow Mod.ACT_curve_on_split ($Flag: Boolean)\nbegin\n"+desc+"\nend;")
	if g := splitOutcomeVectors(t, again); g["'true'"] != got["'true'"] || g["'false'"] != got["'false'"] {
		t.Errorf("round trip: %v, want %v", g, got)
	}
}

// The control: the flat form still curves both outcomes alike, and a decision
// whose two flows are shaped alike describes in the flat form.
func TestDecisionFlatCurve_2592(t *testing.T) {
	src := strings.Replace(outcomeCurveSrc, "@curve(true: (from: (15, 0), to: (-30, 0)))", "@curve(from: (15, 0), to: (-30, 0))", 1)
	oc := buildMicroflowFromMDL(t, src)
	got := splitOutcomeVectors(t, oc)
	if got["'true'"] != "15;0 / -30;0" || got["'false'"] != "15;0 / -30;0" {
		t.Errorf("flat @curve: %v, want both outcomes 15;0 / -30;0", got)
	}
	desc, _ := describeBuilt(t, src)
	if !strings.Contains(desc, "@curve(from: (15, 0), to: (-30, 0))") || strings.Contains(desc, "@curve(true:") {
		t.Errorf("describe of two alike curves is not the flat form:\n%s", desc)
	}
}

// An outcome curve on anything but an IF is refused: it would be dropped.
func TestOutcomeCurveOffIfRefused_2592(t *testing.T) {
	prog, errs := visitor.Build(`create microflow Mod.M ()
begin
  @curve(true: (from: (15, 0)))
  log info node 'N' 'x';
end;`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	var found bool
	for _, v := range ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt)) {
		if v.RuleID == "MDL060" && strings.Contains(v.Message, "names an IF outcome") {
			found = true
		}
	}
	if !found {
		t.Error("no MDL060 for an outcome curve on a log statement")
	}
}
