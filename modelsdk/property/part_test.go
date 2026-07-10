package property

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
)

type testElement struct {
	element.Base
}

func TestPart(t *testing.T) {
	p := NewPart[*testElement]("child")

	if got := p.Get(); got != nil {
		t.Errorf("expected nil before Set(), got %v", got)
	}
	if p.Dirty() {
		t.Error("expected Dirty() == false before Set()")
	}

	child := &testElement{}
	p.Set(child)

	if got := p.Get(); got != child {
		t.Errorf("expected stored child after Set(), got %v", got)
	}
	if !p.Dirty() {
		t.Error("expected Dirty() == true after Set()")
	}
}

func TestPartSetFromDecode(t *testing.T) {
	p := NewPart[*testElement]("child")

	child := &testElement{}
	p.SetFromDecode(child)

	if got := p.Get(); got != child {
		t.Errorf("expected stored child after SetFromDecode(), got %v", got)
	}
	if p.Dirty() {
		t.Error("expected Dirty() == false after SetFromDecode()")
	}
}

func TestPartList(t *testing.T) {
	pl := NewPartList[*testElement]("children")

	if pl.Len() != 0 {
		t.Errorf("expected Len() == 0 initially, got %d", pl.Len())
	}
	if pl.Dirty() {
		t.Error("expected Dirty() == false initially")
	}

	a := &testElement{}
	b := &testElement{}
	pl.Append(a)
	pl.Append(b)

	if pl.Len() != 2 {
		t.Errorf("expected Len() == 2 after two Append calls, got %d", pl.Len())
	}
	if !pl.Dirty() {
		t.Error("expected Dirty() == true after Append()")
	}

	items := pl.Items()
	if items[0] != a {
		t.Errorf("expected items[0] == a, got %v", items[0])
	}
	if items[1] != b {
		t.Errorf("expected items[1] == b, got %v", items[1])
	}

	pl.Remove(0)

	if pl.Len() != 1 {
		t.Errorf("expected Len() == 1 after Remove(0), got %d", pl.Len())
	}
	if pl.Items()[0] != b {
		t.Errorf("expected remaining item to be b, got %v", pl.Items()[0])
	}
}

func TestPartListRemoveOutOfBounds(t *testing.T) {
	pl := NewPartList[*testElement]("children")
	pl.Remove(0)  // should be a no-op, not panic
	pl.Remove(-1) // should be a no-op, not panic

	if pl.Dirty() {
		t.Error("expected Dirty() == false after no-op Remove calls")
	}
}

func TestPartListAppendFromDecode(t *testing.T) {
	pl := NewPartList[*testElement]("children")
	child := &testElement{}
	pl.AppendFromDecode(child)

	if pl.Len() != 1 {
		t.Errorf("expected Len() == 1 after AppendFromDecode(), got %d", pl.Len())
	}
	if pl.Dirty() {
		t.Error("expected Dirty() == false after AppendFromDecode()")
	}
}
