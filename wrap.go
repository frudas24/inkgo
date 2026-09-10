package inkgo

import textutil "github.com/frudas24/inkgo/internal/textutil"

const Ellipsis = textutil.Ellipsis

func truncateText(text string, columns int, position TextWrap) string {
	return textutil.TruncateText(text, columns, position)
}

var (
	WrapText      = textutil.WrapText
	MeasureText   = textutil.MeasureText
	WrapTextLines = textutil.WrapTextLines
)
