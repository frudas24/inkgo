package engine

import (
	"reflect"
	"testing"
)

func TestWideCellRejectedAtRightEdgePreservesExistingPair(t *testing.T) {
	s := NewScreen(2, 1)
	s.SetCell(0, 0, "界", 2, TextStyle{}, "")
	before := s.Clone()
	s.SetCell(1, 0, "語", 2, TextStyle{}, "")
	if !reflect.DeepEqual(s.Cells, before.Cells) {
		t.Fatalf("rejected write corrupted existing glyph: %+v", s.Cells)
	}
}

func TestFocusRejectsHiddenAndForeignNodes(t *testing.T) {
	visible := Button(Style{}, nil, Text("visible"))
	hidden := Button(Style{}, nil, Text("hidden"))
	root := Root(visible, Box(Style{Display: DisplayNone}, hidden))
	fm := NewFocusManager(root)
	if !fm.Focus(visible) {
		t.Fatal("visible node not focused")
	}
	for _, node := range []*Node{hidden, Button(Style{}, nil)} {
		if fm.Focus(node) {
			t.Error("focused hidden or foreign node")
		}
		if fm.Focused() != visible {
			t.Error("rejected focus changed current focus")
		}
	}
}

func TestAutoFocusPicksFirstVisibleCandidate(t *testing.T) {
	hidden := Button(Style{}, nil)
	first := Button(Style{}, nil)
	last := Button(Style{}, nil)
	hidden.AutoFocus = true
	first.AutoFocus = true
	last.AutoFocus = true
	root := Root(Box(Style{Display: DisplayNone}, hidden), first, last)
	fm := NewFocusManager(root)
	if got := fm.AutoFocus(); got != first || fm.Focused() != first {
		t.Fatalf("autofocus=%p focused=%p want first=%p", got, fm.Focused(), first)
	}
}
