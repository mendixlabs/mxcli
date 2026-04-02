package tui

import (
	"strings"
	"testing"
)

func TestRenderUnifiedDiff_NilResult(t *testing.T) {
	lines := RenderUnifiedDiff(nil, "")
	if lines != nil {
		t.Errorf("expected nil for nil result, got %d lines", len(lines))
	}
}

func TestRenderUnifiedDiff_EmptyResult(t *testing.T) {
	result := &DiffResult{}
	lines := RenderUnifiedDiff(result, "")
	if lines != nil {
		t.Errorf("expected nil for empty result, got %d lines", len(lines))
	}
}

func TestRenderUnifiedDiff_CorrectLineCount(t *testing.T) {
	result := ComputeDiff("aaa\nbbb\n", "aaa\nccc\n")
	rendered := RenderUnifiedDiff(result, "")

	if len(rendered) != len(result.Lines) {
		t.Errorf("rendered %d lines, want %d (one per diff line)", len(rendered), len(result.Lines))
	}
}

func TestRenderUnifiedDiff_PrefixAndContent(t *testing.T) {
	result := ComputeDiff("old\n", "new\n")
	rendered := RenderUnifiedDiff(result, "")

	for i, rl := range rendered {
		if rl.Prefix == "" {
			t.Errorf("line %d: Prefix should not be empty", i)
		}
		// Content may contain ANSI codes but should not be empty for non-blank lines
	}
}

func TestRenderUnifiedDiff_EqualLinesHaveBothLineNumbers(t *testing.T) {
	result := ComputeDiff("same\n", "same\n")
	rendered := RenderUnifiedDiff(result, "")

	if len(rendered) != 1 {
		t.Fatalf("expected 1 line, got %d", len(rendered))
	}
	// Prefix should contain "1" twice (old and new line number)
	prefix := rendered[0].Prefix
	if strings.Count(stripAnsi(prefix), "1") < 2 {
		t.Errorf("equal line prefix should contain line number 1 twice, got prefix: %q", stripAnsi(prefix))
	}
}

func TestRenderSideBySideDiff_NilResult(t *testing.T) {
	left, right := RenderSideBySideDiff(nil, "")
	if left != nil || right != nil {
		t.Error("expected nil for nil result")
	}
}

func TestRenderSideBySideDiff_EmptyResult(t *testing.T) {
	result := &DiffResult{}
	left, right := RenderSideBySideDiff(result, "")
	if left != nil || right != nil {
		t.Error("expected nil for empty result")
	}
}

func TestRenderSideBySideDiff_EqualLineCount(t *testing.T) {
	result := ComputeDiff("aaa\nbbb\n", "aaa\nccc\n")
	left, right := RenderSideBySideDiff(result, "")

	if len(left) != len(right) {
		t.Errorf("left (%d) and right (%d) should have same number of lines", len(left), len(right))
	}
}

func TestRenderSideBySideDiff_DeleteHasBlankOnRight(t *testing.T) {
	result := ComputeDiff("aaa\nbbb\n", "aaa\n")
	left, right := RenderSideBySideDiff(result, "")
	_ = left

	// The delete line should produce a blank on the right
	foundBlank := false
	for _, rl := range right {
		if rl.Blank {
			foundBlank = true
			break
		}
	}
	if !foundBlank {
		t.Error("expected a blank filler line on the right for a delete")
	}
}

func TestRenderSideBySideDiff_InsertHasBlankOnLeft(t *testing.T) {
	result := ComputeDiff("aaa\n", "aaa\nbbb\n")
	left, right := RenderSideBySideDiff(result, "")

	foundBlank := false
	for _, rl := range left {
		if rl.Blank {
			foundBlank = true
			break
		}
	}
	if !foundBlank {
		t.Error("expected a blank filler line on the left for an insert")
	}
	_ = right
}

func TestRenderPlainUnifiedDiff_NilResult(t *testing.T) {
	out := RenderPlainUnifiedDiff(nil, "old", "new")
	if out != "" {
		t.Errorf("expected empty string for nil result, got %q", out)
	}
}

func TestRenderPlainUnifiedDiff_EmptyResult(t *testing.T) {
	result := &DiffResult{}
	out := RenderPlainUnifiedDiff(result, "old", "new")
	if out != "" {
		t.Errorf("expected empty string for empty result, got %q", out)
	}
}

func TestRenderPlainUnifiedDiff_Headers(t *testing.T) {
	result := ComputeDiff("old\n", "new\n")
	out := RenderPlainUnifiedDiff(result, "file.txt", "file.txt")

	if !strings.HasPrefix(out, "--- a/file.txt\n+++ b/file.txt\n") {
		t.Errorf("missing unified diff headers, got:\n%s", out)
	}
}

func TestRenderPlainUnifiedDiff_ContainsHunkHeader(t *testing.T) {
	result := ComputeDiff("old\n", "new\n")
	out := RenderPlainUnifiedDiff(result, "a", "b")

	if !strings.Contains(out, "@@") {
		t.Errorf("expected @@ hunk header, got:\n%s", out)
	}
}

func TestRenderPlainUnifiedDiff_DeletesAndAdds(t *testing.T) {
	result := ComputeDiff("alpha\nbeta\n", "alpha\ngamma\n")
	out := RenderPlainUnifiedDiff(result, "a", "b")

	if !strings.Contains(out, "-beta") {
		t.Errorf("expected -beta line, got:\n%s", out)
	}
	if !strings.Contains(out, "+gamma") {
		t.Errorf("expected +gamma line, got:\n%s", out)
	}
	// Context line (unchanged)
	if !strings.Contains(out, " alpha") {
		t.Errorf("expected context line ' alpha', got:\n%s", out)
	}
}

func TestRenderPlainUnifiedDiff_IdenticalInputs(t *testing.T) {
	text := "same\nlines\nhere\n"
	result := ComputeDiff(text, text)
	out := RenderPlainUnifiedDiff(result, "a", "b")

	// Should have headers and "no differences" marker
	if !strings.Contains(out, "no differences") {
		t.Errorf("identical inputs should produce 'no differences' marker, got:\n%s", out)
	}
}

func TestRenderPlainUnifiedDiff_NoANSI(t *testing.T) {
	result := ComputeDiff("old\n", "new\n")
	out := RenderPlainUnifiedDiff(result, "a", "b")

	if strings.Contains(out, "\x1b[") {
		t.Error("plain diff should not contain ANSI escape sequences")
	}
}

func TestRenderSegments_EmptySegments(t *testing.T) {
	out := renderSegments(nil, DiffInsert)
	if out != "" {
		t.Errorf("expected empty string for nil segments, got %q", out)
	}
}

func TestRenderSegments_EqualType(t *testing.T) {
	segs := []DiffSegment{{Text: "hello", Changed: false}, {Text: " world", Changed: true}}
	out := renderSegments(segs, DiffEqual)

	// For DiffEqual, renderSegments just concatenates text without styling
	if out != "hello world" {
		t.Errorf("expected plain concatenation for DiffEqual, got %q", out)
	}
}

func TestHslice_NoSkip(t *testing.T) {
	out := hslice("hello", 0, 5)
	if out != "hello" {
		t.Errorf("expected 'hello', got %q", out)
	}
}

func TestHslice_Skip(t *testing.T) {
	out := hslice("hello world", 6, 5)
	if out != "world" {
		t.Errorf("expected 'world', got %q", out)
	}
}

func TestHslice_TruncateTake(t *testing.T) {
	out := hslice("hello world", 0, 5)
	if out != "hello" {
		t.Errorf("expected 'hello', got %q", out)
	}
}
