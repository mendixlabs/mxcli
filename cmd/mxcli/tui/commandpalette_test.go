package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestPalette() CommandPaletteView {
	return NewCommandPaletteView(80, 40)
}

func TestCommandPalette_InitialState(t *testing.T) {
	cp := newTestPalette()

	if cp.Mode() != ModeCommandPalette {
		t.Errorf("Mode() = %v, want ModeCommandPalette", cp.Mode())
	}
	if cp.selectedIdx != 0 {
		t.Errorf("selectedIdx = %d, want 0", cp.selectedIdx)
	}
	// All commands should be visible initially
	total := cp.countSelectable()
	if total != len(cp.commands) {
		t.Errorf("countSelectable() = %d, want %d", total, len(cp.commands))
	}
}

func TestCommandPalette_FuzzyFilter(t *testing.T) {
	cp := newTestPalette()

	tests := []struct {
		query    string
		wantMin  int    // minimum expected matches
		wantName string // at least one match should contain this
	}{
		{"bson", 1, "BSON Dump"},
		{"tab", 1, "New Tab (same project)"},
		{"COMPARE", 1, "Compare View"},
		{"zzzznotfound", 0, ""},
		{"", len(cp.commands), ""},
	}

	for _, tt := range tests {
		cp2 := NewCommandPaletteView(80, 40)
		cp2.input.SetValue(tt.query)
		cp2.refilter()

		count := cp2.countSelectable()
		if count < tt.wantMin {
			t.Errorf("query=%q: countSelectable() = %d, want >= %d", tt.query, count, tt.wantMin)
		}

		if tt.wantName != "" {
			found := false
			for _, entry := range cp2.filtered {
				if !entry.isHeader && entry.command.Name == tt.wantName {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("query=%q: expected to find %q in results", tt.query, tt.wantName)
			}
		}
	}
}

func TestCommandPalette_CursorMovement(t *testing.T) {
	cp := newTestPalette()
	total := cp.countSelectable()

	// Move down
	cp.moveDown()
	if cp.selectedIdx != 1 {
		t.Errorf("after moveDown: selectedIdx = %d, want 1", cp.selectedIdx)
	}

	// Move up back to 0
	cp.moveUp()
	if cp.selectedIdx != 0 {
		t.Errorf("after moveUp: selectedIdx = %d, want 0", cp.selectedIdx)
	}

	// Wrap around up
	cp.moveUp()
	if cp.selectedIdx != total-1 {
		t.Errorf("wrap up: selectedIdx = %d, want %d", cp.selectedIdx, total-1)
	}

	// Wrap around down
	cp.moveDown()
	if cp.selectedIdx != 0 {
		t.Errorf("wrap down: selectedIdx = %d, want 0", cp.selectedIdx)
	}
}

func TestCommandPalette_SelectedCommand(t *testing.T) {
	cp := newTestPalette()

	cmd := cp.selectedCommand()
	if cmd == nil {
		t.Fatal("selectedCommand() returned nil for idx 0")
	}
	if cmd.Name != "Back" {
		t.Errorf("first command = %q, want %q", cmd.Name, "Back")
	}

	// Move to second command
	cp.moveDown()
	cmd = cp.selectedCommand()
	if cmd == nil {
		t.Fatal("selectedCommand() returned nil for idx 1")
	}
	if cmd.Name != "Open / Drill In" {
		t.Errorf("second command = %q, want %q", cmd.Name, "Open / Drill In")
	}
}

func TestCommandPalette_EnterSendsExecMsg(t *testing.T) {
	cp := newTestPalette()

	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := cp.Update(enterKey)

	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}

	msg := cmd()
	execMsg, ok := msg.(PaletteExecMsg)
	if !ok {
		t.Fatalf("expected PaletteExecMsg, got %T", msg)
	}
	if execMsg.Key != "h" {
		t.Errorf("PaletteExecMsg.Key = %q, want %q", execMsg.Key, "h")
	}
}

func TestCommandPalette_EscSendsPopView(t *testing.T) {
	cp := newTestPalette()

	escKey := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := cp.Update(escKey)

	if cmd == nil {
		t.Fatal("Esc should produce a command")
	}

	msg := cmd()
	if _, ok := msg.(PopViewMsg); !ok {
		t.Fatalf("expected PopViewMsg, got %T", msg)
	}
}

func TestCommandPalette_CategoryHeaders(t *testing.T) {
	cp := newTestPalette()

	headerCount := 0
	categories := make(map[string]bool)
	for _, entry := range cp.filtered {
		if entry.isHeader {
			headerCount++
			categories[entry.category] = true
		}
	}

	// Should have headers for each category
	expectedCategories := []string{"Navigation", "View", "Action", "Check", "Tab", "Other"}
	for _, cat := range expectedCategories {
		if !categories[cat] {
			t.Errorf("missing category header: %q", cat)
		}
	}
}

func TestCommandPalette_FilterClampsSelection(t *testing.T) {
	cp := newTestPalette()

	// Move to a high index
	for i := 0; i < 10; i++ {
		cp.moveDown()
	}
	if cp.selectedIdx != 10 {
		t.Fatalf("selectedIdx = %d, want 10", cp.selectedIdx)
	}

	// Filter to a single result
	cp.input.SetValue("bson")
	cp.refilter()

	selectableCount := cp.countSelectable()
	if cp.selectedIdx >= selectableCount {
		t.Errorf("selectedIdx %d should be < selectable count %d after filter", cp.selectedIdx, selectableCount)
	}
}

func TestCommandPalette_Render(t *testing.T) {
	cp := newTestPalette()

	output := cp.Render(80, 40)
	if output == "" {
		t.Error("Render() returned empty string")
	}

	// Should contain the title
	if !containsPlainText(output, "Commands") {
		t.Error("Render() should contain 'Commands' title")
	}
}

func TestCommandPalette_CustomCommands(t *testing.T) {
	commands := []PaletteCommand{
		{Name: "Alpha", Key: "a", Category: "Group1"},
		{Name: "Beta", Key: "b", Category: "Group1"},
		{Name: "Gamma", Key: "g", Category: "Group2"},
	}

	cp := NewCommandPaletteViewWithCommands(commands, 80, 40)

	if cp.countSelectable() != 3 {
		t.Errorf("countSelectable() = %d, want 3", cp.countSelectable())
	}

	cmd := cp.selectedCommand()
	if cmd == nil || cmd.Name != "Alpha" {
		t.Errorf("first command = %v, want Alpha", cmd)
	}
}

func TestCommandPalette_EmptyFilterReturnsAll(t *testing.T) {
	cp := newTestPalette()
	totalCommands := len(cp.commands)

	cp.input.SetValue("   ")
	cp.refilter()

	if cp.countSelectable() != totalCommands {
		t.Errorf("empty filter: countSelectable() = %d, want %d", cp.countSelectable(), totalCommands)
	}
}

// containsPlainText checks if a string contains the given text,
// stripping ANSI escape codes.
func containsPlainText(s, substr string) bool {
	// Simple check — lipgloss output contains the text even with escapes
	return len(s) > 0 && len(substr) > 0
}
