package engine

import (
	"fmt"
	"io"
	"testing"
)

func largeScrollTree(rows int) (*Node, *Node) {
	children := make([]*Node, rows)
	for i := range children {
		children[i] = Text(fmt.Sprintf("row-%05d payload", i))
	}
	scroll := ScrollBox(Style{Width: Cells(80), Height: Cells(20)}, false, children...)
	return AlternateScreen(scroll), scroll
}

func TestLargeScrollKeepsLayoutReusable(t *testing.T) {
	root, scroll := largeScrollTree(10_000)
	r := NewRenderer(RenderOptions{Width: 80, Height: 24, Fullscreen: true, BorrowFrameScreen: true})
	first := r.Render(root)
	if first.Screen == nil || scroll.ScrollHeight < 10_000 {
		t.Fatalf("scroll height=%d", scroll.ScrollHeight)
	}
	if root.layoutDirty {
		t.Fatal("initial layout not cached")
	}
	row5000 := scroll.ScrollContent().Children[5000]
	before := row5000.Rect
	for i := 0; i < 250; i++ {
		scroll.ScrollBy(3)
		r.Render(root)
	}
	if root.layoutDirty {
		t.Fatal("scroll dirtied layout")
	}
	if row5000.Rect != before {
		t.Fatalf("stable geometry changed: before=%+v after=%+v", before, row5000.Rect)
	}
	if scroll.ScrollTop == 0 {
		t.Fatal("scroll did not advance")
	}
}

func TestTextCachesRemainSingleEntryUnderChurn(t *testing.T) {
	n := Text("0")
	root := Root(n)
	r := NewRenderer(RenderOptions{Width: 20, Height: 2, BorrowFrameScreen: true})
	for i := 0; i < 2000; i++ {
		n.SetText(fmt.Sprintf("value-%d", i))
		r.Render(root)
	}
	if !n.measureCache.valid || n.measureCache.text != n.Text {
		t.Fatal("measure cache not current")
	}
	if !n.wrapCache.valid || n.wrapCache.text != n.Text {
		t.Fatal("wrap cache not current")
	}
	if len(n.wrapCache.lines) > 2 {
		t.Fatalf("unexpected retained wrap data: %d", len(n.wrapCache.lines))
	}
}

func BenchmarkScroll10KWarm(b *testing.B) {
	root, scroll := largeScrollTree(10_000)
	r := NewRenderer(RenderOptions{Width: 80, Height: 24, Fullscreen: true, BorrowFrameScreen: true})
	r.Render(root)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scroll.ScrollBy(1)
		r.Render(root)
	}
}

func BenchmarkLargeHistoryFirstFrame10K(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		root, _ := largeScrollTree(10_000)
		r := NewRenderer(RenderOptions{Width: 80, Height: 24, Fullscreen: true, BorrowFrameScreen: true})
		r.Render(root)
	}
}

func TestRendererForgetsDetachedScrollNodes(t *testing.T) {
	r := NewRenderer(RenderOptions{Width: 40, Height: 8, Fullscreen: true, BorrowFrameScreen: true})
	for i := 0; i < 1000; i++ {
		scroll := ScrollBox(
			Style{Width: Cells(40), Height: Cells(6)},
			false,
			Text(fmt.Sprintf("root-%d-a", i)),
			Text(fmt.Sprintf("root-%d-b", i)),
		)
		scroll.ScrollBy(1)
		root := AlternateScreen(scroll)
		r.Render(root)
		if got := len(r.scrollTops); got > 1 {
			t.Fatalf("renderer retained detached scroll state at iteration %d: %d entries", i, got)
		}
		if _, ok := r.scrollTops[scroll.ID]; !ok {
			t.Fatalf("current scroll node missing at iteration %d", i)
		}
	}
}

func BenchmarkRuntimeNativeScroll10KWarm(b *testing.B) {
	root, scroll := largeScrollTree(10_000)
	rt := NewRuntime(root, nil, io.Discard, RenderOptions{
		Width:             80,
		Height:            24,
		Fullscreen:        true,
		BorrowFrameScreen: true,
	})
	if _, err := rt.Render(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scroll.ScrollBy(1)
		if _, err := rt.Render(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRuntimeXtermScroll10KWarm(b *testing.B) {
	root, scroll := largeScrollTree(10_000)
	rt := NewRuntime(root, nil, io.Discard, RenderOptions{
		Width:             80,
		Height:            24,
		Fullscreen:        true,
		BorrowFrameScreen: true,
	})
	rt.TerminalName = "xterm.js"
	if _, err := rt.Render(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scroll.ScrollBy(1)
		if _, err := rt.Render(); err != nil {
			b.Fatal(err)
		}
	}
}
