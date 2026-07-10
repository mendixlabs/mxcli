// SPDX-License-Identifier: Apache-2.0

// Tests for @anchor annotation parsing — sets OriginConnectionIndex /
// DestinationConnectionIndex hints on microflow statements so that DESCRIBE →
// re-execute round-trips preserve the visual side on which each SequenceFlow
// attaches.
package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func firstStatement(t *testing.T, src string) ast.MicroflowStatement {
	t.Helper()
	input := "create microflow MfTest.M () begin\n" + src + "\nend;"
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse: %v", e)
		}
		t.FailNow()
	}
	if len(prog.Statements) == 0 {
		t.Fatal("no statements parsed")
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if len(mf.Body) == 0 {
		t.Fatal("no body statements")
	}
	return mf.Body[0]
}

func TestAnchorAnnotation_FromAndTo(t *testing.T) {
	stmt := firstStatement(t, "@anchor(from: right, to: left)\nlog info node 'App' 'hi';")

	log, ok := stmt.(*ast.LogStmt)
	if !ok {
		t.Fatalf("expected LogStmt, got %T", stmt)
	}
	if log.Annotations == nil || log.Annotations.Anchor == nil {
		t.Fatal("expected Anchor annotation to be set")
	}
	if log.Annotations.Anchor.From != ast.AnchorSideRight {
		t.Errorf("From: got %v, want Right", log.Annotations.Anchor.From)
	}
	if log.Annotations.Anchor.To != ast.AnchorSideLeft {
		t.Errorf("To: got %v, want Left", log.Annotations.Anchor.To)
	}
}

func TestAnchorAnnotation_OnlyFrom(t *testing.T) {
	stmt := firstStatement(t, "@anchor(from: bottom)\nlog info node 'App' 'hi';")
	log := stmt.(*ast.LogStmt)
	if log.Annotations.Anchor == nil {
		t.Fatal("expected Anchor")
	}
	if log.Annotations.Anchor.From != ast.AnchorSideBottom {
		t.Errorf("From: got %v, want Bottom", log.Annotations.Anchor.From)
	}
	if log.Annotations.Anchor.To != ast.AnchorSideUnset {
		t.Errorf("To: got %v, want Unset", log.Annotations.Anchor.To)
	}
}

func TestAnchorAnnotation_AllFourSides(t *testing.T) {
	cases := []struct {
		name string
		side string
		want ast.AnchorSide
	}{
		{"top", "top", ast.AnchorSideTop},
		{"right", "right", ast.AnchorSideRight},
		{"bottom", "bottom", ast.AnchorSideBottom},
		{"left", "left", ast.AnchorSideLeft},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stmt := firstStatement(t, "@anchor(from: "+tc.side+")\nlog info node 'App' 'hi';")
			log := stmt.(*ast.LogStmt)
			if log.Annotations.Anchor.From != tc.want {
				t.Errorf("From for %s: got %v, want %v", tc.side, log.Annotations.Anchor.From, tc.want)
			}
		})
	}
}

func TestAnchorAnnotation_SplitBranches(t *testing.T) {
	src := `@anchor(true: (from: right, to: left), false: (from: bottom, to: top))
if true then
  log info node 'App' 'yes';
else
  log info node 'App' 'no';
end if;`
	stmt := firstStatement(t, src)
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected IfStmt, got %T", stmt)
	}
	if ifStmt.Annotations == nil {
		t.Fatal("expected Annotations")
	}
	if ifStmt.Annotations.TrueBranchAnchor == nil {
		t.Fatal("expected TrueBranchAnchor")
	}
	if ifStmt.Annotations.TrueBranchAnchor.From != ast.AnchorSideRight ||
		ifStmt.Annotations.TrueBranchAnchor.To != ast.AnchorSideLeft {
		t.Errorf("true branch: got from=%v to=%v; want right/left",
			ifStmt.Annotations.TrueBranchAnchor.From, ifStmt.Annotations.TrueBranchAnchor.To)
	}
	if ifStmt.Annotations.FalseBranchAnchor == nil {
		t.Fatal("expected FalseBranchAnchor")
	}
	if ifStmt.Annotations.FalseBranchAnchor.From != ast.AnchorSideBottom ||
		ifStmt.Annotations.FalseBranchAnchor.To != ast.AnchorSideTop {
		t.Errorf("false branch: got from=%v to=%v; want bottom/top",
			ifStmt.Annotations.FalseBranchAnchor.From, ifStmt.Annotations.FalseBranchAnchor.To)
	}
}

func TestAnchorAnnotation_MissingLeavesUnset(t *testing.T) {
	// A statement without @anchor should have Annotations.Anchor == nil,
	// signalling "builder picks the default".
	stmt := firstStatement(t, "log info node 'App' 'hi';")
	log := stmt.(*ast.LogStmt)
	if log.Annotations != nil && log.Annotations.Anchor != nil {
		t.Errorf("expected Anchor nil on statement without @anchor, got %+v", log.Annotations.Anchor)
	}
}

// Parser/visitor regression coverage for the LOOP/WHILE form
// @anchor(iterator: (...), tail: (...)). Feeding real MDL text pins the
// grammar and visitor wiring so the AST fields IteratorAnchor /
// BodyTailAnchor are populated — the executor-side tests only exercise
// synthetic AST values and would not catch a grammar regression.

func TestAnchorAnnotation_LoopIteratorAndTail(t *testing.T) {
	src := `retrieve $items from MfTest.Entity;
@anchor(iterator: (from: bottom, to: top), tail: (from: right, to: bottom))
loop $item in $items begin
  log info node 'App' 'step';
end loop;`
	prog, errs := Build("create microflow MfTest.M () begin\n" + src + "\nend;")
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse: %v", e)
		}
		t.FailNow()
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	// Body[0] is the retrieve, Body[1] is the loop — the annotation sits on the loop.
	if len(mf.Body) < 2 {
		t.Fatalf("expected at least 2 body statements, got %d", len(mf.Body))
	}
	loop, ok := mf.Body[1].(*ast.LoopStmt)
	if !ok {
		t.Fatalf("expected Body[1] to be LoopStmt, got %T", mf.Body[1])
	}
	if loop.Annotations == nil {
		t.Fatal("expected LoopStmt.Annotations, got nil")
	}
	if loop.Annotations.IteratorAnchor == nil {
		t.Fatal("expected IteratorAnchor to be populated")
	}
	if loop.Annotations.IteratorAnchor.From != ast.AnchorSideBottom ||
		loop.Annotations.IteratorAnchor.To != ast.AnchorSideTop {
		t.Errorf("iterator anchor: got from=%v to=%v; want Bottom/Top",
			loop.Annotations.IteratorAnchor.From, loop.Annotations.IteratorAnchor.To)
	}
	if loop.Annotations.BodyTailAnchor == nil {
		t.Fatal("expected BodyTailAnchor to be populated")
	}
	if loop.Annotations.BodyTailAnchor.From != ast.AnchorSideRight ||
		loop.Annotations.BodyTailAnchor.To != ast.AnchorSideBottom {
		t.Errorf("tail anchor: got from=%v to=%v; want Right/Bottom",
			loop.Annotations.BodyTailAnchor.From, loop.Annotations.BodyTailAnchor.To)
	}
}

func TestAnchorAnnotation_WhileIteratorAndTail(t *testing.T) {
	src := `@anchor(iterator: (from: top, to: left), tail: (from: bottom, to: right))
while true begin
  log info node 'App' 'tick';
end while;`
	prog, errs := Build("create microflow MfTest.M () begin\n" + src + "\nend;")
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse: %v", e)
		}
		t.FailNow()
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	while, ok := mf.Body[0].(*ast.WhileStmt)
	if !ok {
		t.Fatalf("expected Body[0] to be WhileStmt, got %T", mf.Body[0])
	}
	if while.Annotations == nil {
		t.Fatal("expected WhileStmt.Annotations, got nil")
	}
	if while.Annotations.IteratorAnchor == nil {
		t.Fatal("expected IteratorAnchor to be populated")
	}
	if while.Annotations.IteratorAnchor.From != ast.AnchorSideTop ||
		while.Annotations.IteratorAnchor.To != ast.AnchorSideLeft {
		t.Errorf("iterator anchor: got from=%v to=%v; want Top/Left",
			while.Annotations.IteratorAnchor.From, while.Annotations.IteratorAnchor.To)
	}
	if while.Annotations.BodyTailAnchor == nil {
		t.Fatal("expected BodyTailAnchor to be populated")
	}
	if while.Annotations.BodyTailAnchor.From != ast.AnchorSideBottom ||
		while.Annotations.BodyTailAnchor.To != ast.AnchorSideRight {
		t.Errorf("tail anchor: got from=%v to=%v; want Bottom/Right",
			while.Annotations.BodyTailAnchor.From, while.Annotations.BodyTailAnchor.To)
	}
}

func TestAnchorAnnotation_LoopIteratorOnly(t *testing.T) {
	// Only iterator is provided — tail stays nil.
	src := `retrieve $items from MfTest.Entity;
@anchor(iterator: (from: top, to: left))
loop $item in $items begin
  log info node 'App' 'step';
end loop;`
	prog, errs := Build("create microflow MfTest.M () begin\n" + src + "\nend;")
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse: %v", e)
		}
		t.FailNow()
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	loop := mf.Body[1].(*ast.LoopStmt)
	if loop.Annotations.IteratorAnchor == nil {
		t.Fatal("expected IteratorAnchor, got nil")
	}
	if loop.Annotations.BodyTailAnchor != nil {
		t.Errorf("expected BodyTailAnchor nil when `tail:` omitted, got %+v",
			loop.Annotations.BodyTailAnchor)
	}
}
