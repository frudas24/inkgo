package ink

import (
	"fmt"
	"strconv"
	"strings"
)

type LengthUnit uint8

const (
	LengthUnset LengthUnit = iota
	LengthCells
	LengthPercent
	LengthAuto
)

// Length mirrors Ink's number | "N%" dimensions while keeping zero distinct
// from an unspecified value.
type Length struct {
	Unit  LengthUnit
	Value float64
}

func Cells(v float64) Length   { return Length{Unit: LengthCells, Value: v} }
func Percent(v float64) Length { return Length{Unit: LengthPercent, Value: v} }
func Auto() Length             { return Length{Unit: LengthAuto} }

func ParseLength(v string) (Length, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return Length{}, nil
	}
	if v == "auto" {
		return Auto(), nil
	}
	if strings.HasSuffix(v, "%") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		if err != nil {
			return Length{}, err
		}
		return Percent(n), nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return Length{}, err
	}
	return Cells(n), nil
}

func (l Length) Resolve(parent float64) (float64, bool) {
	switch l.Unit {
	case LengthCells:
		return l.Value, true
	case LengthPercent:
		if parent < 0 {
			return 0, false
		}
		return parent * l.Value / 100, true
	default:
		return 0, false
	}
}

type FlexDirection string

const (
	Row           FlexDirection = "row"
	RowReverse    FlexDirection = "row-reverse"
	Column        FlexDirection = "column"
	ColumnReverse FlexDirection = "column-reverse"
)

type FlexWrap string

const (
	NoWrap      FlexWrap = "nowrap"
	Wrap        FlexWrap = "wrap"
	WrapReverse FlexWrap = "wrap-reverse"
)

type Align string

const (
	AlignAuto      Align = "auto"
	AlignStretch   Align = "stretch"
	AlignFlexStart Align = "flex-start"
	AlignCenter    Align = "center"
	AlignFlexEnd   Align = "flex-end"
)

type Justify string

const (
	JustifyFlexStart    Justify = "flex-start"
	JustifyCenter       Justify = "center"
	JustifyFlexEnd      Justify = "flex-end"
	JustifySpaceBetween Justify = "space-between"
	JustifySpaceAround  Justify = "space-around"
	JustifySpaceEvenly  Justify = "space-evenly"
)

type Position string

const (
	PositionRelative Position = "relative"
	PositionAbsolute Position = "absolute"
)

type Overflow string

const (
	OverflowVisible Overflow = "visible"
	OverflowHidden  Overflow = "hidden"
	OverflowScroll  Overflow = "scroll"
)

type Display string

const (
	DisplayFlex Display = "flex"
	DisplayNone Display = "none"
)

type TextWrap string

const (
	TextWrapWrap           TextWrap = "wrap"
	TextWrapTrim           TextWrap = "wrap-trim"
	TextWrapEnd            TextWrap = "end"
	TextWrapMiddle         TextWrap = "middle"
	TextWrapTruncateEnd    TextWrap = "truncate-end"
	TextWrapTruncate       TextWrap = "truncate"
	TextWrapTruncateMiddle TextWrap = "truncate-middle"
	TextWrapTruncateStart  TextWrap = "truncate-start"
)

type BorderText struct {
	Content  string
	Position string // top | bottom
	Align    string // start | center | end
	Offset   int
}

type BorderChars struct {
	Top, Right, Bottom, Left                   string
	TopLeft, TopRight, BottomLeft, BottomRight string
}

type BorderStyle struct {
	Name  string
	Chars BorderChars
}

var (
	BorderSingle = BorderStyle{Name: "single", Chars: BorderChars{
		Top: "─", Right: "│", Bottom: "─", Left: "│",
		TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘",
	}}
	BorderDouble = BorderStyle{Name: "double", Chars: BorderChars{
		Top: "═", Right: "║", Bottom: "═", Left: "║",
		TopLeft: "╔", TopRight: "╗", BottomLeft: "╚", BottomRight: "╝",
	}}
	BorderRound = BorderStyle{Name: "round", Chars: BorderChars{
		Top: "─", Right: "│", Bottom: "─", Left: "│",
		TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯",
	}}
	BorderBold = BorderStyle{Name: "bold", Chars: BorderChars{
		Top: "━", Right: "┃", Bottom: "━", Left: "┃",
		TopLeft: "┏", TopRight: "┓", BottomLeft: "┗", BottomRight: "┛",
	}}
	BorderClassic = BorderStyle{Name: "classic", Chars: BorderChars{
		Top: "-", Right: "|", Bottom: "-", Left: "|",
		TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
	}}
	BorderDashed = BorderStyle{Name: "dashed", Chars: BorderChars{
		Top: "╌", Right: "╎", Bottom: "╌", Left: "╎",
		TopLeft: " ", TopRight: " ", BottomLeft: " ", BottomRight: " ",
	}}
)

func BorderByName(name string) (BorderStyle, bool) {
	switch name {
	case "single":
		return BorderSingle, true
	case "double":
		return BorderDouble, true
	case "round":
		return BorderRound, true
	case "bold":
		return BorderBold, true
	case "classic":
		return BorderClassic, true
	case "dashed":
		return BorderDashed, true
	default:
		return BorderStyle{}, false
	}
}

type ColorKind uint8

const (
	ColorUnset ColorKind = iota
	ColorANSI
	ColorANSI256
	ColorRGB
)

type Color struct {
	Kind    ColorKind
	ANSI    int
	R, G, B uint8
}

func ANSIColor(index int) Color { return Color{Kind: ColorANSI, ANSI: index} }
func ANSI256(index int) Color   { return Color{Kind: ColorANSI256, ANSI: max(0, min(index, 255))} }
func RGB(r, g, b uint8) Color   { return Color{Kind: ColorRGB, R: r, G: g, B: b} }

func Hex(s string) (Color, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = strings.Repeat(s[0:1], 2) + strings.Repeat(s[1:2], 2) + strings.Repeat(s[2:3], 2)
	}
	if len(s) != 6 {
		return Color{}, fmt.Errorf("invalid hex color %q", s)
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return Color{}, err
	}
	return RGB(uint8(n>>16), uint8(n>>8), uint8(n)), nil
}

func ParseColor(s string) (Color, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Color{}, nil
	}
	if strings.HasPrefix(s, "#") {
		return Hex(s)
	}
	if strings.HasPrefix(s, "ansi256(") && strings.HasSuffix(s, ")") {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(s, "ansi256("), ")"))
		if err != nil {
			return Color{}, err
		}
		return ANSI256(n), nil
	}
	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(s, "rgb("), ")"), ",")
		if len(parts) != 3 {
			return Color{}, fmt.Errorf("invalid rgb color %q", s)
		}
		vals := [3]int{}
		for i := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
			if err != nil || n < 0 || n > 255 {
				return Color{}, fmt.Errorf("invalid rgb color %q", s)
			}
			vals[i] = n
		}
		return RGB(uint8(vals[0]), uint8(vals[1]), uint8(vals[2])), nil
	}
	const prefix = "ansi:"
	if strings.HasPrefix(s, prefix) {
		name := strings.TrimPrefix(s, prefix)
		if n, ok := ansiColorName[name]; ok {
			return ANSIColor(n), nil
		}
	}
	return Color{}, fmt.Errorf("unsupported color %q", s)
}

var ansiColorName = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4, "magenta": 5, "cyan": 6, "white": 7,
	"blackBright": 8, "redBright": 9, "greenBright": 10, "yellowBright": 11,
	"blueBright": 12, "magentaBright": 13, "cyanBright": 14, "whiteBright": 15,
}

type TextStyle struct {
	Color           Color
	BackgroundColor Color
	Dim             bool
	Bold            bool
	Italic          bool
	Underline       bool
	Strikethrough   bool
	Inverse         bool
}

func (s TextStyle) IsZero() bool { return s == (TextStyle{}) }

type NoSelectMode uint8

const (
	Selectable NoSelectMode = iota
	NoSelect
	NoSelectFromLeftEdge
)

// Style is the Go equivalent of the fork's Styles type.
// Zero values mean the Ink defaults unless a Length has Unit != LengthUnset.
type Style struct {
	TextWrap TextWrap

	Position                 Position
	Top, Bottom, Left, Right Length

	ColumnGap, RowGap, Gap *int

	Margin, MarginX, MarginY                             *int
	MarginTop, MarginBottom, MarginLeft, MarginRight     *int
	Padding, PaddingX, PaddingY                          *int
	PaddingTop, PaddingBottom, PaddingLeft, PaddingRight *int

	FlexGrow, FlexShrink *float64
	FlexDirection        FlexDirection
	FlexBasis            Length
	FlexWrap             FlexWrap
	AlignItems           Align
	AlignSelf            Align
	JustifyContent       Justify

	Width, Height, MinWidth, MinHeight, MaxWidth, MaxHeight Length
	Display                                                 Display

	BorderStyle                                                                                      *BorderStyle
	BorderTop, BorderBottom, BorderLeft, BorderRight                                                 *bool
	BorderColor, BorderTopColor, BorderBottomColor, BorderLeftColor, BorderRightColor                Color
	BorderDimColor, BorderTopDimColor, BorderBottomDimColor, BorderLeftDimColor, BorderRightDimColor *bool
	BorderText                                                                                       *BorderText

	BackgroundColor Color
	Opaque          bool

	Overflow, OverflowX, OverflowY Overflow
	NoSelect                       NoSelectMode
}

func I(v int) *int         { return &v }
func F(v float64) *float64 { return &v }
func B(v bool) *bool       { return &v }

func (s *Style) defaults() {
	if s.FlexDirection == "" {
		s.FlexDirection = Row
	}
	if s.FlexWrap == "" {
		s.FlexWrap = NoWrap
	}
	if s.AlignItems == "" {
		s.AlignItems = AlignStretch
	}
	if s.AlignSelf == "" {
		s.AlignSelf = AlignAuto
	}
	if s.JustifyContent == "" {
		s.JustifyContent = JustifyFlexStart
	}
	if s.Position == "" {
		s.Position = PositionRelative
	}
	if s.Display == "" {
		s.Display = DisplayFlex
	}
	if s.Overflow == "" {
		s.Overflow = OverflowVisible
	}
	if s.OverflowX == "" {
		s.OverflowX = s.Overflow
	}
	if s.OverflowY == "" {
		s.OverflowY = s.Overflow
	}
	if s.TextWrap == "" {
		s.TextWrap = TextWrapWrap
	}
	if s.FlexGrow == nil {
		s.FlexGrow = F(0)
	}
	if s.FlexShrink == nil {
		s.FlexShrink = F(1)
	}
}

func (s Style) marginEdges() Edges {
	all, x, y := 0, 0, 0
	if s.Margin != nil {
		all = *s.Margin
	}
	if s.MarginX != nil {
		x = *s.MarginX
	} else {
		x = all
	}
	if s.MarginY != nil {
		y = *s.MarginY
	} else {
		y = all
	}
	e := Edges{Top: y, Right: x, Bottom: y, Left: x}
	if s.MarginTop != nil {
		e.Top = *s.MarginTop
	}
	if s.MarginRight != nil {
		e.Right = *s.MarginRight
	}
	if s.MarginBottom != nil {
		e.Bottom = *s.MarginBottom
	}
	if s.MarginLeft != nil {
		e.Left = *s.MarginLeft
	}
	return e
}

func (s Style) paddingEdges() Edges {
	all, x, y := 0, 0, 0
	if s.Padding != nil {
		all = *s.Padding
	}
	if s.PaddingX != nil {
		x = *s.PaddingX
	} else {
		x = all
	}
	if s.PaddingY != nil {
		y = *s.PaddingY
	} else {
		y = all
	}
	e := Edges{Top: y, Right: x, Bottom: y, Left: x}
	if s.PaddingTop != nil {
		e.Top = *s.PaddingTop
	}
	if s.PaddingRight != nil {
		e.Right = *s.PaddingRight
	}
	if s.PaddingBottom != nil {
		e.Bottom = *s.PaddingBottom
	}
	if s.PaddingLeft != nil {
		e.Left = *s.PaddingLeft
	}
	return e
}

func (s Style) borderEdges() Edges {
	if s.BorderStyle == nil {
		return Edges{}
	}
	e := EdgeAll(1)
	if s.BorderTop != nil && !*s.BorderTop {
		e.Top = 0
	}
	if s.BorderRight != nil && !*s.BorderRight {
		e.Right = 0
	}
	if s.BorderBottom != nil && !*s.BorderBottom {
		e.Bottom = 0
	}
	if s.BorderLeft != nil && !*s.BorderLeft {
		e.Left = 0
	}
	return e
}

func (s Style) gapMain(dir FlexDirection) int {
	if dir == Row || dir == RowReverse {
		if s.ColumnGap != nil {
			return *s.ColumnGap
		}
	} else {
		if s.RowGap != nil {
			return *s.RowGap
		}
	}
	if s.Gap != nil {
		return *s.Gap
	}
	return 0
}

func (s Style) gapCross(dir FlexDirection) int {
	if dir == Row || dir == RowReverse {
		if s.RowGap != nil {
			return *s.RowGap
		}
	} else {
		if s.ColumnGap != nil {
			return *s.ColumnGap
		}
	}
	if s.Gap != nil {
		return *s.Gap
	}
	return 0
}
