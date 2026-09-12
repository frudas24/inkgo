package engine

import (
	"fmt"
	"strings"
	"testing"
)

func benchmarkTree() *Node {
	rows := make([]*Node, 0, 40)
	for i := 0; i < 40; i++ {
		rows = append(rows, Box(Style{Width: Percent(100), FlexDirection: Row},
			Text("row "),
			Text("payload payload payload payload"),
		))
	}
	return Root(Box(Style{Width: Percent(100), FlexDirection: Column}, rows...))
}

func BenchmarkRendererStableSnapshot(b *testing.B) {
	root := benchmarkTree()
	r := NewRenderer(RenderOptions{Width: 100, Height: 50, Fullscreen: true})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Render(root)
	}
}

func BenchmarkRendererBorrowed(b *testing.B) {
	root := benchmarkTree()
	r := NewRenderer(RenderOptions{Width: 100, Height: 50, Fullscreen: true, BorrowFrameScreen: true})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Render(root)
	}
}

// BenchmarkRendererResizeLongTranscript measures the cost of one width step on
// the transcript shape a TUI holds: one wrapped text node per entry inside a
// ScrollBox, so every entry is re-measured and re-wrapped on a resize.
func BenchmarkRendererResizeLongTranscript(b *testing.B) {
	const entries = 200
	unit := "alpha bravo charlie delta echo foxtrot golf hotel "
	var sb strings.Builder
	for sb.Len() < 1024 {
		sb.WriteString(unit)
	}
	body := sb.String()[:1024]
	nodes := make([]*Node, 0, entries)
	for i := 0; i < entries; i++ {
		nodes = append(nodes, TextWithWrap(fmt.Sprintf("[%03d] ", i)+body, TextWrapWrap, TextStyle{}))
	}
	root := ScrollBox(Style{FlexGrow: F(1), FlexShrink: F(1)}, true, nodes...)
	r := NewRenderer(RenderOptions{Width: 120, Height: 40})
	_ = r.Render(root)
	_ = r.Render(root)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		width := 120
		if i%2 == 1 {
			width = 100
		}
		r.SetSize(width, 40)
		_ = r.Render(root)
	}
}
