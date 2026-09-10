package engine

import (
	"io"
	"testing"
)

func TestScrollNodeIndexPreservesOrderAndDirtyFallback(t *testing.T) {
	first := ScrollBox(Style{Height: Cells(2)}, false, Text("a"), Text("b"))
	second := ScrollBox(Style{Height: Cells(2)}, false, Text("c"), Text("d"))
	root := Root(Box(Style{FlexDirection: Column}, Text("head"), first, Box(Style{}, second)))

	// Before the first layout, discovery must still work and preserve DFS order.
	nodes := layoutScrollNodes(root)
	if len(nodes) != 2 || nodes[0] != first || nodes[1] != second {
		t.Fatalf("pre-layout scroll order = %#v", nodes)
	}
	if got := firstScrollBox(root); got != first {
		t.Fatalf("first scroll before layout = %p, want %p", got, first)
	}

	ComputeLayout(root, Size{Width: 20, Height: 8}, true)
	if root.layoutDirty || !root.layoutCached {
		t.Fatal("layout index was not cached")
	}
	nodes = layoutScrollNodes(root)
	if len(nodes) != 2 || nodes[0] != first || nodes[1] != second {
		t.Fatalf("cached scroll order = %#v", nodes)
	}

	third := ScrollBox(Style{Height: Cells(2)}, false, Text("e"), Text("f"))
	root.Children[0].Append(third)
	if !root.layoutDirty {
		t.Fatal("topology mutation did not dirty layout")
	}
	nodes = layoutScrollNodes(root)
	if len(nodes) != 3 || nodes[0] != first || nodes[1] != second || nodes[2] != third {
		t.Fatalf("dirty-layout discovery order = %#v", nodes)
	}
}

func TestXtermScrollPolicyUsesAllDiscoveredScrollNodes(t *testing.T) {
	first := ScrollBox(Style{Height: Cells(2)}, false, Text("a"), Text("b"))
	second := ScrollBox(Style{Height: Cells(2)}, false, Text("c"), Text("d"))
	root := Root(Box(Style{FlexDirection: Column}, first, second))
	rt := NewRuntime(root, nil, io.Discard, RenderOptions{Width: 20, Height: 8, Fullscreen: true})
	rt.TerminalName = "xterm.js"

	if _, err := rt.Render(); err != nil {
		t.Fatal(err)
	}
	if !first.ScrollAdaptive || !second.ScrollAdaptive {
		t.Fatalf("xterm scroll policy not applied: first=%v second=%v", first.ScrollAdaptive, second.ScrollAdaptive)
	}
}
