package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type mockView struct {
	mode ViewMode
}

func (m mockView) Update(tea.Msg) (View, tea.Cmd) { return m, nil }
func (m mockView) Render(w, h int) string         { return "" }
func (m mockView) Hints() []Hint                  { return nil }
func (m mockView) StatusInfo() StatusInfo         { return StatusInfo{} }
func (m mockView) Mode() ViewMode                 { return m.mode }

func TestNewViewStack_ActiveReturnsBase(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	vs := NewViewStack(base)

	if got := vs.Active(); got.Mode() != ModeBrowser {
		t.Errorf("Active() mode = %v, want %v", got.Mode(), ModeBrowser)
	}
}

func TestPush_ActiveReturnsTop(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	overlay := mockView{mode: ModeOverlay}
	vs := NewViewStack(base)

	vs.Push(overlay)

	if got := vs.Active(); got.Mode() != ModeOverlay {
		t.Errorf("Active() mode = %v, want %v", got.Mode(), ModeOverlay)
	}
}

func TestPop_RemovesTop(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	overlay := mockView{mode: ModeOverlay}
	vs := NewViewStack(base)
	vs.Push(overlay)

	popped, ok := vs.Pop()
	if !ok {
		t.Fatal("Pop() returned false, want true")
	}
	if popped.Mode() != ModeOverlay {
		t.Errorf("popped mode = %v, want %v", popped.Mode(), ModeOverlay)
	}
	if got := vs.Active(); got.Mode() != ModeBrowser {
		t.Errorf("Active() after Pop = %v, want %v", got.Mode(), ModeBrowser)
	}
}

func TestPop_EmptyStack_ReturnsFalse(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	vs := NewViewStack(base)

	_, ok := vs.Pop()
	if ok {
		t.Error("Pop() on empty stack returned true, want false")
	}
}

func TestDepth(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	vs := NewViewStack(base)

	if got := vs.Depth(); got != 1 {
		t.Errorf("Depth() = %d, want 1", got)
	}

	vs.Push(mockView{mode: ModeOverlay})
	if got := vs.Depth(); got != 2 {
		t.Errorf("Depth() = %d, want 2", got)
	}

	vs.Push(mockView{mode: ModeDiff})
	if got := vs.Depth(); got != 3 {
		t.Errorf("Depth() = %d, want 3", got)
	}

	vs.Pop()
	if got := vs.Depth(); got != 2 {
		t.Errorf("Depth() after Pop = %d, want 2", got)
	}
}

func TestSetActive_ReplacesTop(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	vs := NewViewStack(base)
	vs.Push(mockView{mode: ModeOverlay})

	vs.SetActive(mockView{mode: ModeDiff})

	if got := vs.Active(); got.Mode() != ModeDiff {
		t.Errorf("Active() = %v, want %v", got.Mode(), ModeDiff)
	}
	if got := vs.Depth(); got != 2 {
		t.Errorf("Depth() = %d, want 2 (SetActive should not change depth)", got)
	}
}

func TestSetActive_EmptyStack_ReplacesBase(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	vs := NewViewStack(base)

	vs.SetActive(mockView{mode: ModeCompare})

	if got := vs.Active(); got.Mode() != ModeCompare {
		t.Errorf("Active() = %v, want %v", got.Mode(), ModeCompare)
	}
	if got := vs.Depth(); got != 1 {
		t.Errorf("Depth() = %d, want 1", got)
	}
}

func TestBase_ReturnsBaseView(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	vs := NewViewStack(base)
	vs.Push(mockView{mode: ModeOverlay})

	if got := vs.Base(); got.Mode() != ModeBrowser {
		t.Errorf("Base() = %v, want %v", got.Mode(), ModeBrowser)
	}
}

func TestSetBase_ReplacesBaseView(t *testing.T) {
	base := mockView{mode: ModeBrowser}
	vs := NewViewStack(base)
	vs.Push(mockView{mode: ModeOverlay})

	vs.SetBase(mockView{mode: ModeCompare})

	if got := vs.Base(); got.Mode() != ModeCompare {
		t.Errorf("Base() = %v, want %v", got.Mode(), ModeCompare)
	}
	// Active should still be the stacked view
	if got := vs.Active(); got.Mode() != ModeOverlay {
		t.Errorf("Active() = %v, want %v (stack unaffected)", got.Mode(), ModeOverlay)
	}
}

func TestModeNames(t *testing.T) {
	vs := NewViewStack(mockView{mode: ModeBrowser})
	vs.Push(mockView{mode: ModeDiff})
	vs.Push(mockView{mode: ModeOverlay})

	names := vs.ModeNames()
	if len(names) != 3 {
		t.Fatalf("ModeNames() len = %d, want 3", len(names))
	}
	expected := []ViewMode{ModeBrowser, ModeDiff, ModeOverlay}
	for i, exp := range expected {
		if names[i] != exp.String() {
			t.Errorf("ModeNames()[%d] = %q, want %q", i, names[i], exp.String())
		}
	}
}
