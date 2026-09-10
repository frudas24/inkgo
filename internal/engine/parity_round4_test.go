package engine

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestColumnWrapUsesVerticalMargins(t *testing.T) {
	zero := 0.0
	mt := 1
	parent := Box(Style{
		Width:         Cells(10),
		Height:        Cells(5),
		FlexDirection: Column,
		FlexWrap:      Wrap,
	},
		Box(Style{Width: Cells(2), Height: Cells(2), MarginTop: &mt, FlexShrink: &zero}, Text("a")),
		Box(Style{Width: Cells(2), Height: Cells(2), MarginTop: &mt, FlexShrink: &zero}, Text("b")),
	)
	root := Root(parent)
	ComputeLayout(root, Size{Width: 10, Height: 5}, true)
	first, second := parent.Children[0], parent.Children[1]
	if second.Rect.X <= first.Rect.X {
		t.Fatalf("column wrap ignored vertical main-axis margins: first=%+v second=%+v", first.Rect, second.Rect)
	}
}

func TestFlexGrowRemainderNeverLeaksToZeroGrowSibling(t *testing.T) {
	one, zero := 1.0, 0.0
	parent := Box(Style{Width: Cells(10), Height: Cells(1), FlexDirection: Row},
		Box(Style{Width: Cells(1), Height: Cells(1), FlexGrow: &one}, Text("a")),
		Box(Style{Width: Cells(1), Height: Cells(1), FlexGrow: &one}, Text("b")),
		Box(Style{Width: Cells(1), Height: Cells(1), FlexGrow: &zero}, Text("c")),
	)
	ComputeLayout(Root(parent), Size{Width: 10, Height: 1}, true)
	if got := parent.Children[2].Rect.Width; got != 1 {
		t.Fatalf("zero-grow sibling absorbed rounding remainder: width=%d rects=%+v", got, SortedRects(parent))
	}
	if got := parent.Children[0].Rect.Width + parent.Children[1].Rect.Width + parent.Children[2].Rect.Width; got != 10 {
		t.Fatalf("grow allocation did not consume main axis: width=%d", got)
	}
}

func TestFlexShrinkRemainderNeverLeaksToZeroShrinkSibling(t *testing.T) {
	one, zero := 1.0, 0.0
	parent := Box(Style{Width: Cells(4), Height: Cells(1), FlexDirection: Row},
		Box(Style{Width: Cells(3), Height: Cells(1), FlexShrink: &one}, Text("a")),
		Box(Style{Width: Cells(3), Height: Cells(1), FlexShrink: &one}, Text("b")),
		Box(Style{Width: Cells(3), Height: Cells(1), FlexShrink: &zero}, Text("c")),
	)
	ComputeLayout(Root(parent), Size{Width: 4, Height: 1}, true)
	if got := parent.Children[2].Rect.Width; got != 3 {
		t.Fatalf("zero-shrink sibling absorbed rounding remainder: width=%d rects=%+v", got, SortedRects(parent))
	}
	if got := parent.Children[0].Rect.Width + parent.Children[1].Rect.Width + parent.Children[2].Rect.Width; got != 4 {
		t.Fatalf("shrink allocation did not fit main axis: width=%d", got)
	}
}

func TestWrappedRowNaturalHeightIncludesAllLines(t *testing.T) {
	zero := 0.0
	wrapped := Box(Style{Width: Cells(4), FlexDirection: Row, FlexWrap: Wrap},
		Box(Style{Width: Cells(2), Height: Cells(1), FlexShrink: &zero}, Text("a")),
		Box(Style{Width: Cells(2), Height: Cells(1), FlexShrink: &zero}, Text("b")),
		Box(Style{Width: Cells(2), Height: Cells(1), FlexShrink: &zero}, Text("c")),
	)
	root := Root(wrapped)
	sz := LayoutNatural(root, 4)
	if wrapped.Rect.Height != 2 || sz.Height != 2 {
		t.Fatalf("wrapped natural height underestimated: box=%+v root=%+v size=%+v", wrapped.Rect, root.Rect, sz)
	}
	if wrapped.Children[2].Rect.Y <= wrapped.Children[0].Rect.Y {
		t.Fatalf("third child did not move to second flex line: first=%+v third=%+v", wrapped.Children[0].Rect, wrapped.Children[2].Rect)
	}
}

func TestWrappedColumnNaturalWidthIncludesAllLines(t *testing.T) {
	zero := 0.0
	wrapped := Box(Style{Height: Cells(2), FlexDirection: Column, FlexWrap: Wrap},
		Box(Style{Width: Cells(1), Height: Cells(1), FlexShrink: &zero}, Text("a")),
		Box(Style{Width: Cells(1), Height: Cells(1), FlexShrink: &zero}, Text("b")),
		Box(Style{Width: Cells(1), Height: Cells(1), FlexShrink: &zero}, Text("c")),
	)
	root := Root(wrapped)
	ComputeLayout(root, Size{Width: 10, Height: 2}, true)
	if wrapped.Rect.Width < 2 {
		t.Fatalf("wrapped column width underestimated: %+v", wrapped.Rect)
	}
	if wrapped.Children[2].Rect.X <= wrapped.Children[0].Rect.X {
		t.Fatalf("third child did not move to second flex column: first=%+v third=%+v", wrapped.Children[0].Rect, wrapped.Children[2].Rect)
	}
}

func TestFlexGrowRedistributesPastMaxConstraint(t *testing.T) {
	one := 1.0
	parent := Box(Style{Width: Cells(10), Height: Cells(1), FlexDirection: Row},
		Box(Style{Width: Cells(1), MaxWidth: Cells(2), Height: Cells(1), FlexGrow: &one}, Text("a")),
		Box(Style{Width: Cells(1), Height: Cells(1), FlexGrow: &one}, Text("b")),
	)
	ComputeLayout(Root(parent), Size{Width: 10, Height: 1}, true)
	if got := parent.Children[0].Rect.Width; got != 2 {
		t.Fatalf("maxWidth not honored during grow: %d", got)
	}
	if got := parent.Children[1].Rect.Width; got != 8 {
		t.Fatalf("unused grow share not redistributed: %d", got)
	}
}

func TestFlexShrinkRedistributesPastMinConstraint(t *testing.T) {
	one := 1.0
	parent := Box(Style{Width: Cells(5), Height: Cells(1), FlexDirection: Row},
		Box(Style{Width: Cells(4), MinWidth: Cells(3), Height: Cells(1), FlexShrink: &one}, Text("a")),
		Box(Style{Width: Cells(4), Height: Cells(1), FlexShrink: &one}, Text("b")),
	)
	ComputeLayout(Root(parent), Size{Width: 5, Height: 1}, true)
	if got := parent.Children[0].Rect.Width; got != 3 {
		t.Fatalf("minWidth not honored during shrink: %d", got)
	}
	if got := parent.Children[1].Rect.Width; got != 2 {
		t.Fatalf("unused shrink share not redistributed: %d", got)
	}
}

func TestSoftWrapSelectionPreservesSignificantSeparatorSpace(t *testing.T) {
	screen, _ := RenderToScreen(Root(Text("hello world")), 6)
	if len(screen.SoftWrap) < 2 || !screen.SoftWrap[1] || screen.SoftWrapEnd[1] != 6 {
		t.Fatalf("soft-wrap provenance missing: sw=%v end=%v", screen.SoftWrap, screen.SoftWrapEnd)
	}
	sel := Selection{Anchor: Point{X: 0, Y: 0}, Focus: Point{X: 5, Y: 1}, FocusSet: true}
	if got := sel.Text(screen); got != "hello world" {
		t.Fatalf("soft-wrap selection lost significant separator: %q", got)
	}
}

func TestShiftRowsPreservesSoftWrapProvenance(t *testing.T) {
	s := NewScreen(6, 3)
	s.SoftWrap[1] = true
	s.SoftWrapEnd[1] = 6
	s.SoftWrap[2] = true
	s.SoftWrapEnd[2] = 4
	s.ShiftRows(0, 2, 1)
	if !s.SoftWrap[0] || s.SoftWrapEnd[0] != 6 || !s.SoftWrap[1] || s.SoftWrapEnd[1] != 4 {
		t.Fatalf("soft-wrap provenance did not shift with rows: sw=%v end=%v", s.SoftWrap, s.SoftWrapEnd)
	}
	if s.SoftWrap[2] || s.SoftWrapEnd[2] != 0 {
		t.Fatalf("cleared row retained soft-wrap provenance: sw=%v end=%v", s.SoftWrap, s.SoftWrapEnd)
	}
}

type overlapDetectWriter struct {
	active  atomic.Int32
	overlap atomic.Bool
	mu      sync.Mutex
	bytes   int
}

func (w *overlapDetectWriter) Write(p []byte) (int, error) {
	if w.active.Add(1) != 1 {
		w.overlap.Store(true)
	}
	time.Sleep(100 * time.Microsecond)
	w.mu.Lock()
	w.bytes += len(p)
	w.mu.Unlock()
	w.active.Add(-1)
	return len(p), nil
}

func TestRuntimeSerializesOutOfBandAndQueryWrites(t *testing.T) {
	out := &overlapDetectWriter{}
	rt := NewRuntime(Root(Text("x")), nil, out, RenderOptions{Width: 8, Height: 2})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = rt.WriteRaw("frame")
		}()
		go func() {
			defer wg.Done()
			_ = rt.Querier.Send(QueryDA2())
		}()
	}
	wg.Wait()
	if out.overlap.Load() {
		t.Fatal("runtime output writes overlapped; ANSI/query bytes can interleave")
	}
}

func TestRendererDefaultFrameScreenIsStableSnapshot(t *testing.T) {
	text := Text("one")
	r := NewRenderer(RenderOptions{Width: 8, Height: 2})
	first := r.Render(Root(text))
	before := first.Screen.PlainText()
	text.SetText("two")
	_ = r.Render(text.Parent)
	text.SetText("three")
	_ = r.Render(text.Parent)
	if got := first.Screen.PlainText(); got != before {
		t.Fatalf("default Frame.Screen mutated after later renders: before=%q after=%q", before, got)
	}
}

func TestRendererBorrowedScreenReusesDoubleBuffer(t *testing.T) {
	root := Root(Text("a"))
	r := NewRenderer(RenderOptions{Width: 8, Height: 2, BorrowFrameScreen: true})
	f1 := r.Render(root)
	f2 := r.Render(root)
	f3 := r.Render(root)
	if f1.Screen == f2.Screen {
		t.Fatal("borrowed renderer reused the active previous buffer too early")
	}
	if f1.Screen != f3.Screen {
		t.Fatal("borrowed renderer did not reuse its double buffer")
	}
}

func assertScreenWideCells(t *testing.T, s *Screen) {
	t.Helper()
	for y := 0; y < s.Height; y++ {
		for x := 0; x < s.Width; x++ {
			c, _ := s.CellAt(x, y)
			switch c.Width {
			case CellWide:
				tail, ok := s.CellAt(x+1, y)
				if !ok || tail.Width != CellSpacerTail {
					t.Fatalf("wide head missing tail at (%d,%d)", x, y)
				}
			case CellSpacerTail:
				head, ok := s.CellAt(x-1, y)
				if !ok || head.Width != CellWide {
					t.Fatalf("orphan spacer tail at (%d,%d)", x, y)
				}
			}
		}
	}
}

func FuzzScreenWideCellInvariants(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5})
	f.Add([]byte{7, 255, 8, 128, 9, 64})
	f.Add([]byte{1, 10, 0, 1, 11, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		const width, height = 12, 4
		s := NewScreen(width, height)
		// Independent operation/column/row bytes reach wide glyphs at even columns.
		// The previous b%width + b&1 encoding could never create those states.
		for i := 0; i+2 < len(data); i += 3 {
			op := data[i]
			x := int(data[i+1]) % width
			y := int(data[i+2]) % height
			if op&1 != 0 {
				s.SetCell(x, y, "界", 2, TextStyle{}, "")
			} else {
				s.SetCell(x, y, string(rune('a'+op%26)), 1, TextStyle{}, "")
			}
			assertScreenWideCells(t, s)
			if op&0x20 != 0 {
				s.ClearRegion(Rect{X: max(0, x-1), Y: y, Width: 2, Height: 1})
				assertScreenWideCells(t, s)
			}
		}
	})
}

func FuzzLayoutAndRenderInvariants(f *testing.F) {
	f.Add([]byte{20, 8, 0, 1, 2, 3, 4, 5, 6, 7})
	f.Add([]byte{5, 3, 3, 255, 128, 64, 32, 16, 8, 4, 2, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}
		width := 1 + int(data[0]%40)
		height := 1 + int(data[1]%16)
		dirs := []FlexDirection{Row, RowReverse, Column, ColumnReverse}
		wraps := []FlexWrap{NoWrap, Wrap, WrapReverse}
		parentStyle := Style{
			Width:         Cells(float64(width)),
			Height:        Cells(float64(height)),
			FlexDirection: dirs[int(data[2])%len(dirs)],
			FlexWrap:      wraps[int(data[2]/4)%len(wraps)],
		}
		if len(data) > 3 {
			g := int(data[3] % 3)
			parentStyle.Gap = &g
		}
		parent := Box(parentStyle)
		count := min(10, max(1, (len(data)-4)/3))
		for i := 0; i < count; i++ {
			base := 4 + i*3
			if base+2 >= len(data) {
				break
			}
			cw := 1 + int(data[base]%12)
			ch := 1 + int(data[base+1]%6)
			grow := float64(data[base+2] % 3)
			shrink := float64((data[base+2] / 3) % 3)
			cs := Style{Width: Cells(float64(cw)), Height: Cells(float64(ch)), FlexGrow: &grow, FlexShrink: &shrink}
			if data[base+2]&0x20 != 0 {
				mx := max(1, cw/2)
				cs.MaxWidth = Cells(float64(mx))
			}
			if data[base+2]&0x40 != 0 {
				mn := min(cw, 1+cw/2)
				cs.MinWidth = Cells(float64(mn))
			}
			parent.Append(Box(cs, Text("x")))
		}
		root := Root(parent)
		r := NewRenderer(RenderOptions{Width: width, Height: height, Fullscreen: true, BorrowFrameScreen: true})
		frame := r.Render(root)
		if frame.Screen.Width != width || frame.Screen.Height != height {
			t.Fatalf("fullscreen screen mismatch: got=%dx%d want=%dx%d", frame.Screen.Width, frame.Screen.Height, width, height)
		}
		root.Walk(func(n *Node) bool {
			if n.Rect.Width < 0 || n.Rect.Height < 0 || n.ContentRect.Width < 0 || n.ContentRect.Height < 0 {
				t.Fatalf("negative geometry: id=%s rect=%+v content=%+v", n.ID, n.Rect, n.ContentRect)
			}
			return true
		})
		for y := 0; y < frame.Screen.Height; y++ {
			for x := 0; x < frame.Screen.Width; x++ {
				c, _ := frame.Screen.CellAt(x, y)
				if c.Width == CellWide {
					if x+1 >= frame.Screen.Width {
						t.Fatalf("renderer produced wide head at right edge")
					}
					n, _ := frame.Screen.CellAt(x+1, y)
					if n.Width != CellSpacerTail {
						t.Fatalf("renderer produced wide head without tail")
					}
				}
			}
		}
	})
}

func TestBlitTranslatesSoftWrapEnd(t *testing.T) {
	src := NewScreen(8, 2)
	src.SoftWrap[1] = true
	src.SoftWrapEnd[1] = 6
	dst := NewScreen(12, 2)
	dst.Blit(src, Rect{X: 1, Y: 0, Width: 6, Height: 2}, Point{X: 3, Y: 0})
	if !dst.SoftWrap[1] {
		t.Fatal("blit lost soft-wrap marker")
	}
	if got, want := dst.SoftWrapEnd[1], 8; got != want {
		t.Fatalf("blit did not translate soft-wrap end: got=%d want=%d", got, want)
	}
}

func TestTextCachesInvalidateThroughSetters(t *testing.T) {
	n := Text("abcdef")
	root := Root(n)
	r := NewRenderer(RenderOptions{Width: 3, Height: 2})
	first := r.Render(root)
	if got := first.Screen.PlainText(); got != "abc\ndef" {
		t.Fatalf("unexpected initial wrapped frame: %q", got)
	}

	n.SetText("xy")
	second := r.Render(root)
	if got := second.Screen.PlainText(); got != "xy" {
		t.Fatalf("SetText returned stale cached render: %q", got)
	}

	n.SetText("abcdef")
	n.SetStyle(Style{TextWrap: TextWrapTruncate})
	third := r.Render(root)
	if got := third.Screen.PlainText(); got != "ab…" {
		t.Fatalf("SetStyle returned stale wrapped cache: %q", got)
	}
}

func TestSetTextWithTabsIsIdempotentAfterExpansion(t *testing.T) {
	n := Text("a\tb")
	before := n.generation
	n.SetText("a\tb")
	if n.generation != before {
		t.Fatalf("equivalent tab-expanded text dirtied node: before=%d after=%d", before, n.generation)
	}
}
