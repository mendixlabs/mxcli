// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Geometry tests for the split builders (mendixlabs/mxcli#953).
//
// The split builders had no positional coverage at all before this file —
// grep for `Position` in the enum-split and inheritance-split tests returned
// nothing — which is why a type split shipped drawing the element after
// `END SPLIT` directly on top of its own merge. Nothing downstream can catch
// that: the model is valid, so `mx check` and the build both pass, and the
// overlap is only visible by opening the microflow in Studio Pro.

// logStmt is a one-activity branch body.
func geoLog(msg string) ast.MicroflowStatement {
	return &ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: msg}}
}

func newGeometryBuilder() *flowBuilder {
	return &flowBuilder{spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
}

func soleMerge(t *testing.T, fb *flowBuilder) *microflows.ExclusiveMerge {
	t.Helper()
	var found *microflows.ExclusiveMerge
	for _, obj := range fb.objects {
		if m, ok := obj.(*microflows.ExclusiveMerge); ok {
			if found != nil {
				t.Fatal("more than one merge; the fixture is not the shape this test assumes")
			}
			found = m
		}
	}
	if found == nil {
		t.Fatal("no merge was created")
	}
	return found
}

// typeSplit builds a type split whose branches each hold one activity.
func typeSplit(branchBodies ...[]ast.MicroflowStatement) *ast.InheritanceSplitStmt {
	s := &ast.InheritanceSplitStmt{Variable: "obj"}
	for i, body := range branchBodies {
		if i == len(branchBodies)-1 {
			s.ElseBody = body
			break
		}
		s.Cases = append(s.Cases, ast.InheritanceSplitCase{
			Entity: ast.QualifiedName{Module: "M", Name: "Sub"},
			Body:   body,
		})
	}
	return s
}

// The element after END SPLIT must not land on the merge.
//
// `addStructuredInheritanceSplit` ended with `fb.posX = mergeX` — the merge's
// own centre — so the end event was drawn on top of it, joined by a zero-length
// sequence flow. Reported from the field as "the end event renders on top of
// the merge in Studio Pro".
func TestTypeSplitLeavesRoomAfterTheMerge(t *testing.T) {
	fb := newGeometryBuilder()
	fb.addStructuredInheritanceSplit(typeSplit(
		[]ast.MicroflowStatement{geoLog("a")},
		[]ast.MicroflowStatement{geoLog("b")},
	))

	merge := soleMerge(t, fb)
	if fb.posX == merge.Position.X {
		t.Fatalf("the next element would be placed at x=%d, exactly on the merge", fb.posX)
	}

	// The spacing constants are centre-to-centre and tuned for a 40px edge gap,
	// so clearing a MergeSize-wide merge before a full-width activity needs
	// MergeSize + half a pitch. Same rule addIfStatement uses.
	if want := merge.Position.X + MergeSize + HorizontalSpacing/2; fb.posX != want {
		t.Errorf("next x = %d, want %d", fb.posX, want)
	}

	// The gap actually rendered, stated in edge terms so the intent survives a
	// change to the constants.
	gap := (fb.posX - ActivityWidth/2) - (merge.Position.X + MergeSize/2)
	if gap != HorizontalSpacing-ActivityWidth {
		t.Errorf("edge-to-edge gap after the merge = %d, want %d", gap, HorizontalSpacing-ActivityWidth)
	}
}

// The merge is placed from the WIDEST branch, not from all of them end to end.
//
// The width came from every branch body concatenated into one list and measured
// as a single run, so the merge slid right by an activity-plus-spacing for each
// extra branch — reported as "the merge x looks over-advanced relative to its
// branches". Branches are stacked vertically; only the widest one matters.
func TestTypeSplitMergeDoesNotDriftWithBranchCount(t *testing.T) {
	build := func(n int) *microflows.ExclusiveMerge {
		bodies := make([][]ast.MicroflowStatement, n)
		for i := range bodies {
			bodies[i] = []ast.MicroflowStatement{geoLog("x")}
		}
		fb := newGeometryBuilder()
		fb.addStructuredInheritanceSplit(typeSplit(bodies...))
		return soleMerge(t, fb)
	}

	two, five := build(2), build(5)
	if two.Position.X != five.Position.X {
		t.Errorf("merge moved from x=%d to x=%d when three more one-activity branches were added; "+
			"branch width is being summed across branches instead of maxed",
			two.Position.X, five.Position.X)
	}

	// And it sits where one branch's worth of room puts it, not somewhere merely
	// stable. splitX defaults to 0; the split box is ActivityWidth wide.
	branchStartX := ActivityWidth + HorizontalSpacing/2
	if want := branchStartX + ActivityWidth + HorizontalSpacing/2; two.Position.X != want {
		t.Errorf("merge x = %d, want %d (branch start + one activity + half a pitch)", two.Position.X, want)
	}
}

// Control for the test above: it must be the branch WIDTH that moves the merge,
// or "does not drift with branch count" would also pass against a builder that
// ignored the branches entirely.
func TestTypeSplitMergeDoesFollowBranchWidth(t *testing.T) {
	fb := newGeometryBuilder()
	fb.addStructuredInheritanceSplit(typeSplit(
		[]ast.MicroflowStatement{geoLog("a"), geoLog("b")}, // the wide one
		[]ast.MicroflowStatement{geoLog("c")},
	))
	wide := soleMerge(t, fb)

	fb2 := newGeometryBuilder()
	fb2.addStructuredInheritanceSplit(typeSplit(
		[]ast.MicroflowStatement{geoLog("a")},
		[]ast.MicroflowStatement{geoLog("c")},
	))
	narrow := soleMerge(t, fb2)

	if wide.Position.X <= narrow.Position.X {
		t.Errorf("merge at x=%d for a two-activity branch vs x=%d for a one-activity branch; "+
			"the branches are not being measured at all", wide.Position.X, narrow.Position.X)
	}
}

// The enum split is the construct the field report compared against. Pinned here so
// a later attempt to unify the two cannot move it silently: every existing enum
// split in every project would be re-laid out.
//
// Its merge still stands where one branch's width puts it, whatever the branch
// count. What DID change, deliberately: the advance past that merge. Half a pitch
// from the merge's CENTRE is 80px, and a merge is 40 wide against an activity's 120,
// so the next activity's left edge landed exactly on the merge's right edge — two
// touching elements on the main line of every enum split with a statement after it.
// The advance now clears the merge first and then leaves the ordinary gap, which is
// the arithmetic addIfStatement has always used after the merge that closes an IF.
func TestEnumSplitAdvanceClearsItsMerge(t *testing.T) {
	build := func(n int) (*microflows.ExclusiveMerge, int) {
		s := &ast.EnumSplitStmt{Variable: "kind"}
		for i := 0; i < n; i++ {
			s.Cases = append(s.Cases, ast.EnumSplitCase{
				Values: []string{"V"},
				Body:   []ast.MicroflowStatement{geoLog("x")},
			})
		}
		s.ElseBody = []ast.MicroflowStatement{geoLog("e")}
		fb := newGeometryBuilder()
		fb.addEnumSplit(s)
		return soleMerge(t, fb), fb.posX
	}

	two, nextTwo := build(2)
	five, _ := build(5)
	if two.Position.X != five.Position.X {
		t.Errorf("enum split merge moved with branch count: x=%d then x=%d", two.Position.X, five.Position.X)
	}
	if want := two.Position.X + MergeSize + HorizontalSpacing/2; nextTwo != want {
		t.Errorf("enum split next x = %d, want %d (clear the merge, then one gap)", nextTwo, want)
	}
	if gap := (nextTwo - ActivityWidth/2) - (two.Position.X + MergeSize/2); gap != laneGap {
		t.Errorf("gap between the merge and the next activity is %d, want %d — the same "+
			"space two activities have", gap, laneGap)
	}
}
