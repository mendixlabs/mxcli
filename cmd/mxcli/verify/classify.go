// SPDX-License-Identifier: Apache-2.0

package verify

// Classify compares the findings of a run with a baseline, by Key and with
// multiplicity (the same key twice in the baseline accepts two findings, not
// every later one).
//
// With staging set (the mxcheck stage) it applies the MxBuild staging rule from
// PROPOSAL_verify_gate.md: mx check reports only the earliest stage that has an
// error, so a plain diff blames a change for errors it merely uncovered and
// calls errors resolved that it merely hid.
//
//	new       current and not in the baseline. This includes an error that an
//	          earlier-stage error in the SAME unit (microflow) was hiding: that
//	          error can only have appeared because the change fixed or removed the
//	          hider, so the change touched the unit, and it must leave it clean.
//	unmasked  current and not in the baseline, but a stage 0 error of the baseline
//	          (an unresolved reference) was hiding the whole project. Fixing that
//	          one error uncovers everything else; none of it is the change's.
//	resolved  in the baseline, gone, and nothing in the current run hides it
//	hidden    in the baseline, gone, but a current finding hides it: not resolved
func Classify(baseline, current []Finding, staging bool) Outcome {
	left := map[string]int{}
	for _, b := range baseline {
		left[b.Key]++
	}
	var out Outcome
	var candidates []Finding // current findings the baseline does not have
	for _, c := range current {
		if left[c.Key] > 0 {
			left[c.Key]--
			out.PreExisting++
			if out.PreExistingBy == nil {
				out.PreExistingBy = map[string]int{}
			}
			out.PreExistingBy[c.Severity]++
			continue
		}
		candidates = append(candidates, c)
	}
	for _, c := range candidates {
		if staging && hiddenByStage0(c, baseline) {
			out.Unmasked = append(out.Unmasked, c)
		} else {
			out.New = append(out.New, c)
		}
	}
	// What is left of the baseline is gone from the run.
	for _, b := range baseline {
		if left[b.Key] == 0 {
			continue
		}
		left[b.Key]--
		if staging && hiddenBy(b, current) {
			out.Hidden++
		} else {
			out.Resolved++
		}
	}
	return out
}

// hiddenByStage0 reports whether the report holds a stage 0 finding (an
// unresolved reference), which hides every later-stage finding in the project.
func hiddenByStage0(f Finding, report []Finding) bool {
	if f.MxStage <= 0 {
		return false
	}
	for _, h := range report {
		if h.MxStage == 0 {
			return true
		}
	}
	return false
}

// hiddenBy reports whether some finding of report hides f under MxBuild's
// staging: a stage 0 finding anywhere, or an earlier-stage finding in the same
// unit (stages 1 and 2 hide the later stages of that microflow).
func hiddenBy(f Finding, report []Finding) bool {
	if f.MxStage < 0 {
		return false
	}
	for _, h := range report {
		if h.MxStage < 0 || h.MxStage >= f.MxStage {
			continue
		}
		if h.MxStage == 0 || (h.Unit != "" && h.Unit == f.Unit) {
			return true
		}
	}
	return false
}
