package engine

import (
	"math"

	core "github.com/frudas24/inkgo/internal/core"
)

type (
	Point = core.Point
	Size  = core.Size
	Rect  = core.Rect
	Edges = core.Edges
)

var (
	UnionRect = core.UnionRect
	ClampRect = core.ClampRect
	EdgeAll   = core.EdgeAll
	EdgeXY    = core.EdgeXY
	AddEdges  = core.AddEdges
)

func roundCell(v float64) int {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return int(math.Round(v))
}
