package layout

import (
	"github.com/frudas24/inkgo/widgets"
	"testing"
)

func TestPublicLayout(t *testing.T) {
	root := widgets.Root(widgets.Box(Style{FlexDirection: Row, Width: Cells(10)}, widgets.Text("abc"), widgets.Text("de")))
	sz := Compute(root, Size{Width: 10, Height: 5}, true)
	if sz.Width != 10 || root.Children[0].Rect.Width != 10 {
		t.Fatalf("size=%+v rect=%+v", sz, root.Children[0].Rect)
	}
	if len(SortedRects(root)) < 3 {
		t.Fatal("rect diagnostics")
	}
}
