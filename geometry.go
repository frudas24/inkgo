package ink

import "math"

// Point is a zero-based terminal cell coordinate.
type Point struct {
	X int
	Y int
}

// Size is measured in terminal cells.
type Size struct {
	Width  int
	Height int
}

// Rect is a terminal rectangle.
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

func (r Rect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.Width && p.Y < r.Y+r.Height
}

func (r Rect) Intersect(b Rect) Rect {
	x1 := max(r.X, b.X)
	y1 := max(r.Y, b.Y)
	x2 := min(r.X+r.Width, b.X+b.Width)
	y2 := min(r.Y+r.Height, b.Y+b.Height)
	if x2 <= x1 || y2 <= y1 {
		return Rect{}
	}
	return Rect{X: x1, Y: y1, Width: x2 - x1, Height: y2 - y1}
}

func UnionRect(a, b Rect) Rect {
	if a.Empty() {
		return b
	}
	if b.Empty() {
		return a
	}
	x1 := min(a.X, b.X)
	y1 := min(a.Y, b.Y)
	x2 := max(a.X+a.Width, b.X+b.Width)
	y2 := max(a.Y+a.Height, b.Y+b.Height)
	return Rect{X: x1, Y: y1, Width: x2 - x1, Height: y2 - y1}
}

func ClampRect(r Rect, s Size) Rect {
	return r.Intersect(Rect{Width: max(0, s.Width), Height: max(0, s.Height)})
}

type Edges struct {
	Top, Right, Bottom, Left int
}

func EdgeAll(v int) Edges { return Edges{v, v, v, v} }

func EdgeXY(vertical, horizontal int) Edges {
	return Edges{Top: vertical, Right: horizontal, Bottom: vertical, Left: horizontal}
}

func AddEdges(a, b Edges) Edges {
	return Edges{a.Top + b.Top, a.Right + b.Right, a.Bottom + b.Bottom, a.Left + b.Left}
}

func clampFloat(v float64, lo, hi *float64) float64 {
	if lo != nil && v < *lo {
		v = *lo
	}
	if hi != nil && v > *hi {
		v = *hi
	}
	return v
}

func roundCell(v float64) int {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return int(math.Round(v))
}
