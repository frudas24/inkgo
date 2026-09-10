package inkgo

import "testing"

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
