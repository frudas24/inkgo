package inkgo

import textutil "github.com/frudas24/inkgo/internal/textutil"

type Grapheme = textutil.Grapheme

var (
	StripANSI    = textutil.StripANSI
	RuneWidth    = textutil.RuneWidth
	StringWidth  = textutil.StringWidth
	WidestLine   = textutil.WidestLine
	SliceByWidth = textutil.SliceByWidth
	Graphemes    = textutil.Graphemes
)
