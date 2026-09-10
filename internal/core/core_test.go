package core

import "testing"

func TestGeometryAndEdges(t *testing.T) {
	a := Rect{X: 1, Y: 2, Width: 5, Height: 4}
	b := Rect{X: 3, Y: 1, Width: 4, Height: 4}
	if got := a.Intersect(b); got != (Rect{X: 3, Y: 2, Width: 3, Height: 3}) {
		t.Fatalf("intersect=%+v", got)
	}
	if !a.Contains(Point{X: 2, Y: 3}) || a.Contains(Point{X: 9, Y: 9}) {
		t.Fatal("contains invariant failed")
	}
	if got := UnionRect(a, b); got != (Rect{X: 1, Y: 1, Width: 6, Height: 5}) {
		t.Fatalf("union=%+v", got)
	}
	if got := ClampRect(Rect{X: -2, Y: -1, Width: 5, Height: 4}, Size{Width: 2, Height: 2}); got != (Rect{Width: 2, Height: 2}) {
		t.Fatalf("clamp=%+v", got)
	}
	if got := AddEdges(EdgeAll(1), EdgeXY(2, 3)); got != (Edges{Top: 3, Right: 4, Bottom: 3, Left: 4}) {
		t.Fatalf("edges=%+v", got)
	}
}

func TestLengthColorAndBorders(t *testing.T) {
	for _, tc := range []struct {
		in string
		u  LengthUnit
		v  float64
	}{{"", LengthUnset, 0}, {"auto", LengthAuto, 0}, {"25%", LengthPercent, 25}, {"12", LengthCells, 12}} {
		got, err := ParseLength(tc.in)
		if err != nil || got.Unit != tc.u || got.Value != tc.v {
			t.Fatalf("ParseLength(%q)=%+v,%v", tc.in, got, err)
		}
	}
	if _, err := ParseLength("wat"); err == nil {
		t.Fatal("expected invalid length")
	}
	if v, ok := Percent(50).Resolve(20); !ok || v != 10 {
		t.Fatalf("percent=%v,%v", v, ok)
	}
	if _, ok := Auto().Resolve(20); ok {
		t.Fatal("auto must not resolve")
	}

	if got := ANSI256(999); got.ANSI != 255 {
		t.Fatalf("ansi256=%+v", got)
	}
	if got, err := Hex("#abc"); err != nil || got != RGB(0xaa, 0xbb, 0xcc) {
		t.Fatalf("hex=%+v %v", got, err)
	}
	for _, in := range []string{"#102030", "ansi256(42)", "rgb(1, 2, 3)", ""} {
		if _, err := ParseColor(in); err != nil {
			t.Fatalf("ParseColor(%q): %v", in, err)
		}
	}
	for _, in := range []string{"#xx", "ansi256(x)", "rgb(1,2)", "rgb(1,2,999)"} {
		if _, err := ParseColor(in); err == nil {
			t.Fatalf("expected ParseColor(%q) failure", in)
		}
	}
	for _, name := range []string{"single", "double", "round", "bold", "classic", "dashed"} {
		if _, ok := BorderByName(name); !ok {
			t.Fatalf("missing border %s", name)
		}
	}
	if _, ok := BorderByName("missing"); ok {
		t.Fatal("unexpected border")
	}
}

func TestStyleDefaultsAndHelpers(t *testing.T) {
	s := Style{}
	ApplyDefaults(&s)
	if s.FlexDirection != Row || s.FlexWrap != NoWrap || s.AlignItems != AlignStretch {
		t.Fatalf("defaults=%+v", s)
	}
	s.Padding = I(1)
	s.MarginX = I(3)
	s.MarginY = I(2)
	s.Gap = I(4)
	if PaddingEdges(s) != EdgeAll(1) || MarginEdges(s) != EdgeXY(2, 3) {
		t.Fatal("edge helpers")
	}
	if GapMain(s, Row) != 4 || GapCross(s, Row) != 4 || GapMain(s, Column) != 4 {
		t.Fatal("gap helpers")
	}
}
