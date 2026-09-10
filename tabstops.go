package inkgo

import "strings"

const DefaultTabInterval = 8

// ExpandTabs mirrors the fork's Ghostty-inspired 8-column tab stops while
// preserving ANSI/OSC sequences verbatim and resetting the column on newline.
func ExpandTabs(text string, interval ...int) string {
	step := DefaultTabInterval
	if len(interval) > 0 && interval[0] > 0 {
		step = interval[0]
	}
	if !strings.Contains(text, "\t") {
		return text
	}
	var b strings.Builder
	col := 0
	for i := 0; i < len(text); {
		if text[i] == 0x1b {
			seq, ok := nextEscapeSequence(text[i:])
			if ok {
				b.WriteString(seq)
				i += len(seq)
				continue
			}
			b.WriteByte(text[i])
			i++
			continue
		}
		if text[i] == '\t' {
			n := step - (col % step)
			b.WriteString(strings.Repeat(" ", n))
			col += n
			i++
			continue
		}
		if text[i] == '\n' {
			b.WriteByte('\n')
			col = 0
			i++
			continue
		}
		if text[i] == '\r' {
			b.WriteByte('\r')
			col = 0
			i++
			continue
		}
		// Consume one grapheme from the next plain span to keep cell width exact.
		j := i
		for j < len(text) && text[j] != 0x1b && text[j] != '\t' && text[j] != '\n' && text[j] != '\r' {
			j++
		}
		gs := Graphemes(text[i:j])
		if len(gs) == 0 {
			i = j
			continue
		}
		g := gs[0]
		b.WriteString(g.Text)
		col += g.Width
		i += len(g.Text)
	}
	return b.String()
}
