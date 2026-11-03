// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#1929 (mendixlabs/mxcli#1727): a decision one of whose branches
// ends the flow, while the others merge, is described as
//
//	end case;
//	join shared1;
//	merge shared1;
//
// — the label naming the decision's OWN merge. Building that description made
// the decision's merge and a second one for the label, wired merge -> merge at
// the same point, and every further describe -> drop + create added another
// (`join junctionN; merge junctionN;`). The runtime behaviour was the same; the
// model, its diagram and every diff were not.
//
// assertDescriptionRebuildsGraph cannot see this: it contracts merges away by
// design. So the merge count is compared directly, and the description must be
// a fixed point.

const endedBranchCaseMDL = `create microflow Probe.RT_Repro ($S: Probe.Shape) returns Integer as $N
begin
  declare $N Integer = 0;
  case $S/C
    when R then return 1;
    when G then set $N = 2;
    when (empty) then set $N = 3;
  end case;
  return $N;
end;`

const endedBranchSplitTypeMDL = `create microflow M.ST ($A: M.Animal) returns Integer as $N
begin
  declare $N Integer = 0;
  split type $A
    when M.Dog then
      return 1;
    when M.Cat then
      set $N = 2;
    when M.Animal then
      set $N = 3;
  end split;
  return $N;
end;`

const endedBranchElsifMDL = `create microflow M.EI ($X: Integer) returns Integer as $N
begin
  declare $N Integer = 0;
  if $X = 1 then
    set $N = 1;
  elsif $X = 2 then
    return 2;
  else
    set $N = 3;
  end if;
  return $N;
end;`

// roundTripMergeCounts builds src, then rebuilds its description `rounds`
// times, returning the merge count of each generation and the descriptions.
func roundTripMergeCounts(t *testing.T, src string, rounds int) ([]int, []string) {
	t.Helper()
	header := src[:strings.Index(src, "\nbegin")]
	var counts []int
	var outs []string
	cur := src
	for i := 0; i <= rounds; i++ {
		out, oc := describeBuilt(t, cur)
		counts = append(counts, countMerges(oc))
		outs = append(outs, out)
		if i == 0 {
			assertDescriptionRebuildsGraph(t, out, oc)
		}
		cur = header + "\nbegin\n" + out + "\nend;"
	}
	return counts, outs
}

func TestDescribeEndedBranch_RoundTripKeepsOneMerge(t *testing.T) {
	for name, src := range map[string]string{
		"case":       endedBranchCaseMDL,
		"split type": endedBranchSplitTypeMDL,
		"elsif":      endedBranchElsifMDL,
	} {
		t.Run(name, func(t *testing.T) {
			counts, outs := roundTripMergeCounts(t, src, 2)
			for i, n := range counts {
				if n != counts[0] {
					t.Errorf("generation %d has %d merges, the original %d — describe -> create added a merge:\n%s",
						i, n, counts[0], outs[i])
					break
				}
			}
			if outs[1] != outs[0] {
				t.Errorf("the description is not a fixed point:\nfirst:\n%s\nsecond:\n%s", outs[0], outs[1])
			}
			if strings.Contains(outs[len(outs)-1], "junction") {
				t.Errorf("a later round trip named an extra merge:\n%s", outs[len(outs)-1])
			}
		})
	}
}

// CONTROL: a real crossing, where a label is reached from more than one place
// and the decision's merge is a separate node, keeps every merge it had.
func TestDescribeEndedBranch_RealCrossingsKeepTheirMerges(t *testing.T) {
	for name, src := range map[string]string{
		"crossed inside branch": crossedInsideBranchMDL,
		"interleaved if":        interleavedIfMDL,
		"fully interleaved":     fullyInterleavedIfMDL,
	} {
		t.Run(name, func(t *testing.T) {
			counts, outs := roundTripMergeCounts(t, src, 1)
			if counts[1] != counts[0] {
				t.Errorf("merges %d -> %d across a round trip:\n%s\n---\n%s", counts[0], counts[1], outs[0], outs[1])
			}
		})
	}
}
