// Package text exposes terminal-cell measurement, wrapping, ANSI parsing, tab
// expansion, and software bidi helpers. All widths are terminal cell widths,
// not byte or rune counts.
package text

import (
	tui "github.com/frudas24/inkgo"
	core "github.com/frudas24/inkgo/internal/core"
	textutil "github.com/frudas24/inkgo/internal/textutil"
)

type (
	WrapMode       = core.TextWrap
	Size           = core.Size
	TextStyle      = core.TextStyle
	Grapheme       = textutil.Grapheme
	StyledGrapheme = tui.StyledGrapheme
)

const (
	Wrap           = core.TextWrapWrap
	Truncate       = core.TextWrapTruncate
	TruncateStart  = core.TextWrapTruncateStart
	TruncateMiddle = core.TextWrapTruncateMiddle
	TruncateEnd    = core.TextWrapTruncateEnd
)

var (
	RuneWidth     = textutil.RuneWidth
	StringWidth   = textutil.StringWidth
	WidestLine    = textutil.WidestLine
	SliceByWidth  = textutil.SliceByWidth
	Graphemes     = textutil.Graphemes
	ExpandTabs    = textutil.ExpandTabs
	StripANSI     = textutil.StripANSI
	WrapText      = textutil.WrapText
	WrapTextLines = textutil.WrapTextLines
	Measure       = textutil.MeasureText

	ParseANSI         = tui.ParseANSI
	HasRTLCharacters  = tui.HasRTLCharacters
	ReorderBidi       = tui.ReorderBidiGraphemes
	ReorderBidiStyled = tui.ReorderBidiStyled
)
