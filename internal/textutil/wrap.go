package textutil

import (
	"strings"

	core "github.com/frudas24/inkgo/internal/core"
)

const Ellipsis = "…"

func TruncateText(text string, columns int, position core.TextWrap) string {
	if columns < 1 {
		return ""
	}
	if columns == 1 {
		return Ellipsis
	}
	length := StringWidth(text)
	if length <= columns {
		return text
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

// hardWrapLine mirrors wrap-ansi with {hard:true, wordWrap:true}. Escape
// sequences are zero-width atomic tokens, tabs are expanded to 8-column stops,
// and trim mode removes leading/trailing spaces from every visual row.
func hardWrapLine(line string, width int, trim bool) []string {
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
	return out
}

// WrapText mirrors Ink's wrap / wrap-trim / truncate-* behavior.
// "end" and "middle" are retained as compatibility aliases and do not wrap.
func WrapText(text string, maxWidth int, mode core.TextWrap) string {
	if maxWidth < 0 {
		maxWidth = 0
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	switch mode {
	case core.TextWrapTruncate, core.TextWrapTruncateEnd, core.TextWrapTruncateMiddle, core.TextWrapTruncateStart:
		lines := strings.Split(text, "\n")
		for i := range lines {
			lines[i] = TruncateText(lines[i], maxWidth, mode)
		}
		return strings.Join(lines, "\n")
	case core.TextWrapWrap, core.TextWrapTrim, "":
		trim := mode == core.TextWrapTrim
		var out []string
		for _, line := range strings.Split(text, "\n") {
			out = append(out, hardWrapLine(line, maxWidth, trim)...)
		}
		return strings.Join(out, "\n")
	default:
		return text
	}
}

func MeasureText(text string, width int, mode core.TextWrap) core.Size {
	if width <= 0 {
		width = max(1, WidestLine(text))
	}
	wrapped := WrapText(text, width, mode)
	lines := strings.Split(wrapped, "\n")
	maxW := 0
	for _, line := range lines {
		maxW = max(maxW, StringWidth(line))
	}
	return core.Size{Width: min(width, maxW), Height: max(1, len(lines))}
}

// WrapTextLines is WrapText plus the soft-wrap bitmap used by fullscreen
// selection. soft[i] is true when line i is a continuation inserted by
// wrapping rather than an explicit newline in the source.
func WrapTextLines(text string, maxWidth int, mode core.TextWrap) ([]string, []bool) {
	if maxWidth < 0 {
		maxWidth = 0
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if mode == core.TextWrapTruncate || mode == core.TextWrapTruncateEnd || mode == core.TextWrapTruncateMiddle || mode == core.TextWrapTruncateStart || (mode != core.TextWrapWrap && mode != core.TextWrapTrim && mode != "") {
		lines := strings.Split(WrapText(text, maxWidth, mode), "\n")
		return lines, make([]bool, len(lines))
	}
	trim := mode == core.TextWrapTrim
	var lines []string
	var soft []bool
	for _, src := range strings.Split(text, "\n") {
		parts := hardWrapLine(src, maxWidth, trim)
		for i, part := range parts {
			lines = append(lines, part)
			soft = append(soft, i > 0)
		}
	}
	return lines, soft
}
