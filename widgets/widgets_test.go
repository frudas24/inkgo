package widgets

import "testing"

func TestConstructors(t *testing.T) {
	clicked := false
	btn := Button(Style{}, func() { clicked = true }, Text("go"))
	root := Root(Box(Style{}, btn), ScrollBox(Style{}, false, Text("row")))
	if root == nil || len(root.Children) != 2 || btn.TabIndex != 0 {
		t.Fatal("tree")
	}
	btn.OnAction()
	if !clicked {
		t.Fatal("button action")
	}
	if root.Children[1].ScrollContent() == nil {
		t.Fatal("scroll content")
	}
}
