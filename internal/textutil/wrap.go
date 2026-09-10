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

func hardWrapLine(line string, width int, trim bool) []string {
	if width <= 0 {
		return []string{""}
	}
	if StringWidth(line) <= width {
		if trim {
			line = strings.TrimRight(line, " \t")
		}
		return []string{line}
	}
	gs := Graphemes(line)
	var lines []string
	var b strings.Builder
	current := 0
	lastSpaceByte := -1
	lastSpaceWidth := 0

	flush := func(forceWord bool) {
		s := b.String()
		if !forceWord && lastSpaceByte >= 0 {
			head := s[:lastSpaceByte]
			tail := strings.TrimLeft(s[lastSpaceByte:], " \t")
			if trim {
				head = strings.TrimRight(head, " \t")
			}
			lines = append(lines, head)
			b.Reset()
			b.WriteString(tail)
			current = StringWidth(tail)
		} else {
			if trim {
				s = strings.TrimRight(s, " \t")
			}
			lines = append(lines, s)
			b.Reset()
			current = 0
		}
		lastSpaceByte = -1
		lastSpaceWidth = 0
	}

	for _, g := range gs {
		if g.Text == "\t" {
			g.Text = "    "
			g.Width = 4
		}
		if current+g.Width > width && b.Len() > 0 {
			flush(lastSpaceByte < 0)
		}
		if g.Width > width && b.Len() == 0 {
			// A glyph wider than the viewport cannot be split. Emit it so cell
			// clipping decides what is visible rather than looping forever.
			b.WriteString(g.Text)
			current += g.Width
			flush(true)
			continue
		}
		if g.Text == " " || g.Text == "\t" {
			lastSpaceByte = b.Len()
			lastSpaceWidth = current
			_ = lastSpaceWidth
		}
		b.WriteString(g.Text)
		current += g.Width
		if current == width {
			flush(true)
		}
	}
	if b.Len() > 0 || len(lines) == 0 {
		s := b.String()
		if trim {
			s = strings.TrimRight(s, " \t")
		}
		lines = append(lines, s)
	}
	return lines
}

// WrapText mirrors Ink's wrap / wrap-trim / truncate-* behavior.
// "end" and "middle" are retained as compatibility aliases and do not wrap.
func WrapText(text string, maxWidth int, mode core.TextWrap) string {
	if maxWidth < 0 {
		maxWidth = 0
	}
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
