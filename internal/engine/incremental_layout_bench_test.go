package engine

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkTextUpdateFrameCost measures what one TUI frame costs when a single
// leaf Text changes: a header Text, a ScrollBox (Style{FlexGrow: F(1),
// FlexShrink: F(1)}) holding n transcript entries and a footer Text in the
// shape of an activity spinner.
//
// Every iteration replaces the footer with a DIFFERENT string of the SAME width
// (SetText cannot short-circuit) and renders one frame, so the footer's measured
// geometry - and therefore the rect every sibling is offered - is unchanged. The
// benchmark isolates exactly what PERF-001 was about: a leaf text update must
// not make the frame cost grow with the transcript.
//
// The file uses only exported API (Root, Box, Text, ScrollBox, Style, F,
// NewRenderer, RenderOptions, Render, SetText) plus testing, so the identical
// copy dropped into a checkout of the parent commit compiles and produces the
// "before" column.
//
// Measured state as of this commit (AMD Ryzen 7 5800H, Go 1.26.2, 300x):
// the incremental layout prune (Node.subtreeGeomDirty + the prune in layoutNode)
// re-visits only 2 of 805 nodes after a footer edit, but the frame is still
// dominated by two costs it cannot remove, which is why the improvement here is
// ~1.1-1.4x instead of the expected >=20x:
//
//  1. Intrinsic measurement is NOT memoised for containers. measureNode
//     (layout.go) caches only NodeText/NodeRawANSI; a Box/ScrollBox re-walks its
//     whole subtree through measureFlowContent on every call (measured:
//     169 us/call for the n=800 ScrollBox vs 143 ns for a memoised Text). The
//     parent's flex pass measures its children BEFORE layoutNode gets to apply
//     the prune, so the O(n) wrap work survives the fix.
//  2. In non-fullscreen mode the renderer sizes the screen to the laid-out
//     content, so paint/diff is O(content): a no-mutation frame at n=800
//     allocates 20 MB/op and costs ~28 ms regardless of the layout prune.
//     BenchmarkTextUpdateFrameCostAltScreen takes Fullscreen: true (what an
//     alt-screen TUI such as reopgo actually runs, and the mode the guardian's
//     PERF-001 numbers came from) where offscreen entries are clipped and the
//     layout fix is the dominant term.
func BenchmarkTextUpdateFrameCost(b *testing.B) {
	for _, n := range []int{20, 200, 800} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			root, footer := benchTextUpdateTranscript(b, n)
			r := NewRenderer(RenderOptions{Width: 120, Height: 40})
			benchTextUpdateFrameLoop(b, r, root, footer)
		})
	}
}

// BenchmarkTextUpdateFrameCostAltScreen is the same loop against a fullscreen
// renderer: the screen stays 120x40, offscreen transcript entries are clipped
// away, and the frame cost is therefore governed by the layout pass rather than
// by screen-sized paint/diff work. This is the variant that shows what the
// incremental prune buys.
func BenchmarkTextUpdateFrameCostAltScreen(b *testing.B) {
	for _, n := range []int{20, 200, 800} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			root, footer := benchTextUpdateTranscript(b, n)
			r := NewRenderer(RenderOptions{Width: 120, Height: 40, Fullscreen: true})
			benchTextUpdateFrameLoop(b, r, root, footer)
		})
	}
}

// benchTextUpdateFrameLoop warms the caches with two renders, then measures one
// frame per same-width footer update.
func benchTextUpdateFrameLoop(b *testing.B, r *Renderer, root, footer *Node) {
	b.Helper()
	// Warm-up: the first render measures, wraps and caches every entry and the
	// second one reaches the steady state the loop then mutates. The benchmark
	// measures an in-place update of a rendered tree, not the cost of building
	// one.
	_ = r.Render(root)
	_ = r.Render(root)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		footer.SetText(benchTextUpdateSpinner(i))
		_ = r.Render(root)
	}
}

// benchTextUpdateSpinner returns one of two different footer strings of equal
// length, so the mutation always changes the text while leaving the footer's
// measured geometry unchanged.
func benchTextUpdateSpinner(i int) string {
	if i%2 == 0 {
		return "status: indexing"
	}
	return "status: thinking"
}

// benchTextUpdateTranscript builds the transcript shape. Every entry is a
// wrapped Text long enough to wrap over several lines at the renderer width, so
// a full layout pass has real measuring and wrapping work in each subtree the
// incremental prune is supposed to skip.
func benchTextUpdateTranscript(tb testing.TB, n int) (root, footer *Node) {
	tb.Helper()
	unit := "alpha bravo charlie delta echo foxtrot golf hotel "
	var sb strings.Builder
	for sb.Len() < 192 {
		sb.WriteString(unit)
	}
	body := sb.String()[:192]
	entries := make([]*Node, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, Text(fmt.Sprintf("[%03d] ", i)+body))
	}
	header := Text(fmt.Sprintf("inkgo transcript n=%d", n))
	footer = Text(benchTextUpdateSpinner(0))
	root = Root(
		header,
		ScrollBox(Style{FlexGrow: F(1), FlexShrink: F(1)}, true, entries...),
		footer,
	)
	return root, footer
}

// BenchmarkTextUpdateLayoutPassCost isolates the cost the incremental prune
// targets: one ComputeLayout pass after a same-width footer update. There is no
// screen, no paint and no diff here, so the numbers are comparable between a
// checkout with the prune and one without it.
func BenchmarkTextUpdateLayoutPassCost(b *testing.B) {
	for _, n := range []int{20, 200, 800} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			root, footer := benchTextUpdateTranscript(b, n)
			vp := Size{Width: 120, Height: 40}
			ComputeLayout(root, vp, true)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				footer.SetText(benchTextUpdateSpinner(i))
				ComputeLayout(root, vp, true)
			}
		})
	}
}

// BenchmarkTextUpdateFrameCostNoMutation is the floor for the two frame
// benchmarks: the same tree and renderer, rendered every iteration without any
// mutation. A mutated frame can never be cheaper than this, so the factor the
// prune can deliver end to end is bounded by (mutated floor / no-mutation
// floor).
func BenchmarkTextUpdateFrameCostNoMutation(b *testing.B) {
	for _, n := range []int{20, 200, 800} {
		for _, fullscreen := range []bool{false, true} {
			b.Run(fmt.Sprintf("n=%d/fullscreen=%v", n, fullscreen), func(b *testing.B) {
				root, _ := benchTextUpdateTranscript(b, n)
				r := NewRenderer(RenderOptions{Width: 120, Height: 40, Fullscreen: fullscreen})
				_ = r.Render(root)
				_ = r.Render(root)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_ = r.Render(root)
				}
			})
		}
	}
}
