package textutil

import (
	"strings"

	core "github.com/frudas24/inkgo/internal/core"
)

const Ellipsis = "…"

func TruncateText(text string, columns int, position core.TextWrap) string {
	// Normalize malformed UTF-8 and incomplete/unknown output controls even
	// when no truncation is ultimately required. StringWidth already ignores
	// malformed bytes; returning the original string on the fast path would
	// otherwise make the result disagree with its own measured width.
	text = tokensString(terminalTokens(text))
	if columns < 1 {
		return ""
	}
	length := StringWidth(text)
	if length <= columns {
		return text
	}
	if columns == 1 {
		return Ellipsis
	}
	switch position {
	case core.TextWrapTruncateStart:
		return Ellipsis + SliceByWidth(text, length-columns+1, length)
	case core.TextWrapTruncateMiddle:
		left := columns / 2
		right := columns - left - 1
		return SliceByWidth(text, 0, left) + Ellipsis + SliceByWidth(text, length-right, length)
	default:
		return SliceByWidth(text, 0, columns-1) + Ellipsis
	}
}

type wrapWord struct {
	tokens []terminalToken
	width  int
}

func splitWrapWords(line string) []wrapWord {
	words := []wrapWord{{}}
	for _, tok := range terminalTokens(line) {
		if !tok.escape && tok.text == " " {
			words = append(words, wrapWord{})
			continue
		}
		last := &words[len(words)-1]
		last.tokens = append(last.tokens, tok)
		last.width += tok.width
	}
	return words
}

func appendWrappedWord(rows *[][]terminalToken, word wrapWord, width int, rowWidth int) int {
	for i, tok := range word.tokens {
		if tok.width > 0 && rowWidth > 0 && rowWidth+tok.width > width {
			*rows = append(*rows, nil)
			rowWidth = 0
		}
		last := len(*rows) - 1
		(*rows)[last] = append((*rows)[last], tok)
		rowWidth += tok.width
		if rowWidth == width && i < len(word.tokens)-1 {
			*rows = append(*rows, nil)
			rowWidth = 0
		}
	}
	// Do not leave a row containing only zero-width escape sequences after a
	// hard split. Keeping them on the preceding row matches wrap-ansi's
	// zero-width token behavior and avoids synthetic empty visual lines.
	if rowWidth == 0 && len(*rows) > 1 {
		last := len(*rows) - 1
		if len((*rows)[last]) > 0 && tokensWidth((*rows)[last]) == 0 {
			(*rows)[last-1] = append((*rows)[last-1], (*rows)[last]...)
			*rows = (*rows)[:last]
		}
	}
	if len(*rows) == 0 {
		*rows = append(*rows, nil)
	}
	return tokensWidth((*rows)[len(*rows)-1])
}

// hardWrapLine is the single row producer for one source line. Printable ASCII
// - the dominant shape on a transcript layout pass - carries no escape
// sequences, no tab stops and no zero-width controls, so it cannot change
// visual state across rows and is wrapped by scanning bytes instead of building
// a token slice per grapheme.
func hardWrapLine(line string, width int, trim bool) []string {
	if width <= 0 {
		return []string{""}
	}
	if isPlainASCII(line) {
		return wrapPlainASCIILine(line, width, trim)
	}
	return hardWrapLineTokens(line, width, trim)
}

// wrapPlainASCIILine wraps a line whose bytes are all printable ASCII
// (0x20-0x7e). It is a byte-level equivalent of hardWrapLineTokens on that
// input: a space is the only word delimiter, every printed byte is exactly one
// cell wide, and nothing is dropped except the spaces trim mode removes at a row
// boundary - so every row is a slice of the source line and the token slice, the
// per-grapheme builder and the state-restore pass are all skipped.
func wrapPlainASCIILine(line string, width int, trim bool) []string {
	if line == "" || (trim && strings.TrimSpace(line) == "") {
		return []string{""}
	}
	n := len(line)
	out := make([]string, 0, min(n/max(width, 1)+2, 1024))
	rowStart, rowEnd := 0, 0
	haveRow := false
	rowWidth := 0

	// write appends the byte at index at to the current row. Writes are strictly
	// increasing, and a row only ever misses bytes at its own boundaries, so the
	// row is the source range [rowStart, rowEnd).
	write := func(at int) {
		if !haveRow {
			rowStart = at
			haveRow = true
		}
		rowEnd = at + 1
	}
	breakRow := func() {
		row := line[rowStart:rowEnd]
		if trim {
			row = strings.TrimRight(row, " ")
		}
		out = append(out, row)
		haveRow = false
		rowWidth = 0
	}

	firstWord := true
	for pos := 0; ; {
		end := pos
		for end < n && line[end] != ' ' {
			end++
		}
		wordWidth := end - pos

		if firstWord {
			firstWord = false
		} else {
			// The separator space sits at pos-1. It is written unless the row is
			// already full and the break takes it to the next row, and trim mode
			// drops it entirely when the row has nothing in it yet.
			if rowWidth >= width && !trim {
				breakRow()
			}
			if rowWidth > 0 || !trim {
				write(pos - 1)
				rowWidth++
			}
		}

		if wordWidth > width {
			// An overlong word is hard-split at the row width. This mirrors
			// appendWrappedWord's break accounting.
			remaining := width - rowWidth
			breaksStartingThisLine := 1 + (wordWidth-remaining-1)/width
			breaksStartingNextLine := (wordWidth - 1) / width
			if breaksStartingNextLine < breaksStartingThisLine {
				if haveRow {
					breakRow()
				} else {
					out = append(out, "")
				}
			}
			for i := pos; i < end; i++ {
				if rowWidth > 0 && rowWidth+1 > width {
					breakRow()
				}
				write(i)
				rowWidth++
				if rowWidth == width && i < end-1 {
					breakRow()
				}
			}
		} else {
			if rowWidth+wordWidth > width && rowWidth > 0 && wordWidth > 0 {
				breakRow()
			}
			for i := pos; i < end; i++ {
				write(i)
			}
			rowWidth += wordWidth
		}

		if end >= n {
			break
		}
		pos = end + 1
	}
	if haveRow {
		breakRow()
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

// isPlainASCII reports whether s is already in its final printable form: only
// bytes 0x20-0x7e, so no escape to strip, no tab stop to expand and no
// zero-width control byte that would need grapheme segmentation.
func isPlainASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// hardWrapLineTokens is the general row producer, mirroring wrap-ansi with
// {hard:true, wordWrap:true}. Escape sequences are zero-width atomic tokens,
// tabs are expanded to 8-column stops, and trim mode removes leading/trailing
// spaces from every visual row.
func hardWrapLineTokens(line string, width int, trim bool) []string {
	if width <= 0 {
		return []string{""}
	}
	line = ExpandTabs(line, 0)
	if trim && strings.TrimSpace(StripANSI(line)) == "" {
		return []string{""}
	}

	words := splitWrapWords(line)
	rows := [][]terminalToken{{}}
	rowWidth := 0
	firstWord := true

	for _, word := range words {
		if firstWord {
			firstWord = false
		} else {
			if rowWidth >= width && !trim {
				rows = append(rows, nil)
				rowWidth = 0
			}
			if rowWidth > 0 || !trim {
				rows[len(rows)-1] = append(rows[len(rows)-1], terminalToken{text: " ", width: 1})
				rowWidth++
			}
		}

		if word.width > width {
			remaining := width - rowWidth
			breaksStartingThisLine := 1 + (word.width-remaining-1)/width
			breaksStartingNextLine := (word.width - 1) / width
			if breaksStartingNextLine < breaksStartingThisLine {
				rows = append(rows, nil)
				rowWidth = 0
			}
			rowWidth = appendWrappedWord(&rows, word, width, rowWidth)
			continue
		}

		if rowWidth+word.width > width && rowWidth > 0 && word.width > 0 {
			rows = append(rows, nil)
			rowWidth = 0
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], word.tokens...)
		rowWidth += word.width
	}

	out := make([]string, len(rows))
	for i, row := range rows {
		if trim {
			row = visibleTrimSpacesRight(row)
		}
		out[i] = tokensString(row)
	}
	return restoreVisualStateAcrossRows(out)
}

// wrappedRows is the single row producer behind WrapText, WrapTextLines and
// MeasureText. Measuring used to wrap into a string and split it again, which
// re-derived on the layout hot path exactly what the wrap pass had already
// computed.
func wrappedRows(text string, maxWidth int, mode core.TextWrap) ([]string, []bool) {
	if maxWidth < 0 {
		maxWidth = 0
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	switch mode {
	case core.TextWrapTruncate, core.TextWrapTruncateEnd, core.TextWrapTruncateMiddle, core.TextWrapTruncateStart:
		rows := strings.Split(text, "\n")
		for i := range rows {
			rows[i] = TruncateText(rows[i], maxWidth, mode)
		}
		return rows, make([]bool, len(rows))
	case core.TextWrapWrap, core.TextWrapTrim, "":
		trim := mode == core.TextWrapTrim
		var rows []string
		var soft []bool
		for _, line := range strings.Split(text, "\n") {
			parts := hardWrapLine(line, maxWidth, trim)
			for i, part := range parts {
				rows = append(rows, part)
				soft = append(soft, i > 0)
			}
		}
		return rows, soft
	default:
		rows := strings.Split(text, "\n")
		return rows, make([]bool, len(rows))
	}
}

// WrapText mirrors Ink's wrap / wrap-trim / truncate-* behavior.
// "end" and "middle" are retained as compatibility aliases and do not wrap.
func WrapText(text string, maxWidth int, mode core.TextWrap) string {
	rows, _ := wrappedRows(text, maxWidth, mode)
	return strings.Join(rows, "\n")
}

// MeasureText measures the rows produced by the same helper WrapText uses, so
// layout and painting cannot disagree about the row count or the widest row.
func MeasureText(text string, width int, mode core.TextWrap) core.Size {
	if width <= 0 {
		width = max(1, WidestLine(text))
	}
	rows, _ := wrappedRows(text, width, mode)
	maxW := 0
	for _, row := range rows {
		maxW = max(maxW, StringWidth(row))
	}
	return core.Size{Width: min(width, maxW), Height: max(1, len(rows))}
}

// WrapTextLines is WrapText plus the soft-wrap bitmap used by fullscreen
// selection. soft[i] is true when line i is a continuation inserted by
// wrapping rather than an explicit newline in the source.
func WrapTextLines(text string, maxWidth int, mode core.TextWrap) ([]string, []bool) {
	return wrappedRows(text, maxWidth, mode)
}
