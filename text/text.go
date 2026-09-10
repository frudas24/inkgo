// Package text exposes terminal-cell measurement, wrapping, ANSI parsing, tab
// expansion, and software bidi helpers. All widths are terminal cell widths,
// not byte or rune counts.
package text

import tui "github.com/frudas24/inkgo"

type (
	WrapMode       = tui.TextWrap
	Size           = tui.Size
	TextStyle      = tui.TextStyle
	StyledGrapheme = tui.StyledGrapheme
	Grapheme       = tui.Grapheme
)

const (
	Wrap           = tui.TextWrapWrap
	Truncate       = tui.TextWrapTruncate
	TruncateStart  = tui.TextWrapTruncateStart
	TruncateMiddle = tui.TextWrapTruncateMiddle
	TruncateEnd    = tui.TextWrapTruncateEnd
)

var (
	RuneWidth         = tui.RuneWidth
	StringWidth       = tui.StringWidth
	WidestLine        = tui.WidestLine
	SliceByWidth      = tui.SliceByWidth
	Graphemes         = tui.Graphemes
	ExpandTabs        = tui.ExpandTabs
	StripANSI         = tui.StripANSI
	WrapText          = tui.WrapText
	WrapTextLines     = tui.WrapTextLines
	Measure           = tui.MeasureText
	ParseANSI         = tui.ParseANSI
	HasRTLCharacters  = tui.HasRTLCharacters
	ReorderBidi       = tui.ReorderBidiGraphemes
	ReorderBidiStyled = tui.ReorderBidiStyled
)
