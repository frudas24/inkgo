package selection

import (
	"github.com/frudas24/inkgo/render"
	"testing"
)

func TestSelectionAndSearch(t *testing.T) {
	s := render.NewScreen(12, 1)
	for i, r := range "hello world" {
		s.SetCell(i, 0, string(r), 1, render.TextStyle{}, "")
	}
	var sel State
	if !sel.SelectWordAt(s, 1, 0) {
		t.Fatal("select word")
	}
	if got := sel.Text(s); got != "hello" {
		t.Fatalf("selection=%q", got)
	}
	if got := ScanPositions(s, "world"); len(got) != 1 {
		t.Fatalf("matches=%v", got)
	}
}

func TestNewReturnsUsableState(t *testing.T) {
	s := New()
	s.Start(1, 0)
	s.Update(2, 0)
	if !s.HasSelection() {
		t.Fatal("new selection state is not usable")
	}
}
