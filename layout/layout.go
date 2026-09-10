// Package layout exposes the layout/style domain without terminal runtime concerns.
package layout

import tui "github.com/frudas24/inkgo"

type (
	Point         = tui.Point
	Size          = tui.Size
	Rect          = tui.Rect
	Edges         = tui.Edges
	Length        = tui.Length
	LengthUnit    = tui.LengthUnit
	FlexDirection = tui.FlexDirection
	FlexWrap      = tui.FlexWrap
	Align         = tui.Align
	Justify       = tui.Justify
	Position      = tui.Position
	Overflow      = tui.Overflow
	Display       = tui.Display
	Style         = tui.Style
	BorderStyle   = tui.BorderStyle
	BorderChars   = tui.BorderChars
	BorderText    = tui.BorderText
	Node          = tui.Node
)

const (
	LengthUnset   = tui.LengthUnset
	LengthCells   = tui.LengthCells
	LengthPercent = tui.LengthPercent
	LengthAuto    = tui.LengthAuto

	Row           = tui.Row
	RowReverse    = tui.RowReverse
	Column        = tui.Column
	ColumnReverse = tui.ColumnReverse

	NoWrap      = tui.NoWrap
	Wrap        = tui.Wrap
	WrapReverse = tui.WrapReverse

	AlignAuto      = tui.AlignAuto
	AlignStretch   = tui.AlignStretch
	AlignFlexStart = tui.AlignFlexStart
	AlignCenter    = tui.AlignCenter
	AlignFlexEnd   = tui.AlignFlexEnd

	JustifyFlexStart    = tui.JustifyFlexStart
	JustifyCenter       = tui.JustifyCenter
	JustifyFlexEnd      = tui.JustifyFlexEnd
	JustifySpaceBetween = tui.JustifySpaceBetween
	JustifySpaceAround  = tui.JustifySpaceAround
	JustifySpaceEvenly  = tui.JustifySpaceEvenly

	PositionRelative = tui.PositionRelative
	PositionAbsolute = tui.PositionAbsolute

	OverflowVisible = tui.OverflowVisible
	OverflowHidden  = tui.OverflowHidden
	OverflowScroll  = tui.OverflowScroll

	DisplayFlex = tui.DisplayFlex
	DisplayNone = tui.DisplayNone
)

var (
	Cells        = tui.Cells
	Percent      = tui.Percent
	Auto         = tui.Auto
	ParseLength  = tui.ParseLength
	EdgeAll      = tui.EdgeAll
	EdgeXY       = tui.EdgeXY
	AddEdges     = tui.AddEdges
	UnionRect    = tui.UnionRect
	ClampRect    = tui.ClampRect
	Compute      = tui.ComputeLayout
	Natural      = tui.LayoutNatural
	SortedRects  = tui.SortedRects
	BorderByName = tui.BorderByName
	I            = tui.I
	F            = tui.F
	B            = tui.B
)
