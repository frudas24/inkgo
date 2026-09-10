// Package layout exposes style, geometry and flex layout without terminal runtime concerns.
package layout

import (
	tui "github.com/frudas24/inkgo"
	core "github.com/frudas24/inkgo/internal/core"
)

type (
	Point         = core.Point
	Size          = core.Size
	Rect          = core.Rect
	Edges         = core.Edges
	Length        = core.Length
	LengthUnit    = core.LengthUnit
	FlexDirection = core.FlexDirection
	FlexWrap      = core.FlexWrap
	Align         = core.Align
	Justify       = core.Justify
	Position      = core.Position
	Overflow      = core.Overflow
	Display       = core.Display
	Style         = core.Style
	BorderStyle   = core.BorderStyle
	BorderChars   = core.BorderChars
	BorderText    = core.BorderText
	Node          = tui.Node
)

const (
	LengthUnset   = core.LengthUnset
	LengthCells   = core.LengthCells
	LengthPercent = core.LengthPercent
	LengthAuto    = core.LengthAuto

	Row           = core.Row
	RowReverse    = core.RowReverse
	Column        = core.Column
	ColumnReverse = core.ColumnReverse

	NoWrap      = core.NoWrap
	Wrap        = core.Wrap
	WrapReverse = core.WrapReverse

	AlignAuto      = core.AlignAuto
	AlignStretch   = core.AlignStretch
	AlignFlexStart = core.AlignFlexStart
	AlignCenter    = core.AlignCenter
	AlignFlexEnd   = core.AlignFlexEnd

	JustifyFlexStart    = core.JustifyFlexStart
	JustifyCenter       = core.JustifyCenter
	JustifyFlexEnd      = core.JustifyFlexEnd
	JustifySpaceBetween = core.JustifySpaceBetween
	JustifySpaceAround  = core.JustifySpaceAround
	JustifySpaceEvenly  = core.JustifySpaceEvenly

	PositionRelative = core.PositionRelative
	PositionAbsolute = core.PositionAbsolute

	OverflowVisible = core.OverflowVisible
	OverflowHidden  = core.OverflowHidden
	OverflowScroll  = core.OverflowScroll

	DisplayFlex = core.DisplayFlex
	DisplayNone = core.DisplayNone
)

var (
	Cells        = core.Cells
	Percent      = core.Percent
	Auto         = core.Auto
	ParseLength  = core.ParseLength
	EdgeAll      = core.EdgeAll
	EdgeXY       = core.EdgeXY
	AddEdges     = core.AddEdges
	UnionRect    = core.UnionRect
	ClampRect    = core.ClampRect
	BorderByName = core.BorderByName
	I            = core.I
	F            = core.F
	B            = core.B

	Compute     = tui.ComputeLayout
	Natural     = tui.LayoutNatural
	SortedRects = tui.SortedRects
)
