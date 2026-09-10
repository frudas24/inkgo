package interaction

import (
	"github.com/frudas24/inkgo/layout"
	"github.com/frudas24/inkgo/widgets"
	"testing"
)

func TestFocusHitAndKey(t *testing.T) {
	btn := widgets.Button(widgets.Style{Width: layout.Cells(5), Height: layout.Cells(1)}, func() {}, widgets.Text("ok"))
	root := widgets.Root(btn)
	layout.Compute(root, layout.Size{Width: 10, Height: 3}, true)
	fm := NewFocusManager(root)
	if fm.FocusNext() != btn {
		t.Fatal("focus")
	}
	if HitTest(root, layout.Point{X: 0, Y: 0}) == nil {
		t.Fatal("hit")
	}
	if DispatchKey(btn, Key{Name: "enter"}) == nil {
		t.Fatal("key")
	}
}
