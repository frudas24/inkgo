package inkgo

import core "github.com/frudas24/inkgo/internal/core"

type (
	LengthUnit    = core.LengthUnit
	Length        = core.Length
	FlexDirection = core.FlexDirection
	FlexWrap      = core.FlexWrap
	Align         = core.Align
	Justify       = core.Justify
	Position      = core.Position
	Overflow      = core.Overflow
	Display       = core.Display
	TextWrap      = core.TextWrap
	BorderText    = core.BorderText
	BorderChars   = core.BorderChars
	BorderStyle   = core.BorderStyle
	ColorKind     = core.ColorKind
	Color         = core.Color
	TextStyle     = core.TextStyle
	NoSelectMode  = core.NoSelectMode
	Style         = core.Style
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

	TextWrapWrap           = core.TextWrapWrap
	TextWrapTrim           = core.TextWrapTrim
	TextWrapEnd            = core.TextWrapEnd
	TextWrapMiddle         = core.TextWrapMiddle
	TextWrapTruncateEnd    = core.TextWrapTruncateEnd
	TextWrapTruncate       = core.TextWrapTruncate
	TextWrapTruncateMiddle = core.TextWrapTruncateMiddle
	TextWrapTruncateStart  = core.TextWrapTruncateStart

	ColorUnset   = core.ColorUnset
	ColorANSI    = core.ColorANSI
	ColorANSI256 = core.ColorANSI256
	ColorRGB     = core.ColorRGB

	Selectable           = core.Selectable
	NoSelect             = core.NoSelect
	NoSelectFromLeftEdge = core.NoSelectFromLeftEdge
)

var (
	BorderSingle  = core.BorderSingle
	BorderDouble  = core.BorderDouble
	BorderRound   = core.BorderRound
	BorderBold    = core.BorderBold
	BorderClassic = core.BorderClassic
	BorderDashed  = core.BorderDashed
)

var (
	Cells        = core.Cells
	Percent      = core.Percent
	Auto         = core.Auto
	ParseLength  = core.ParseLength
	BorderByName = core.BorderByName
	ANSIColor    = core.ANSIColor
	ANSI256      = core.ANSI256
	RGB          = core.RGB
	Hex          = core.Hex
	ParseColor   = core.ParseColor
	I            = core.I
	F            = core.F
	B            = core.B
)
