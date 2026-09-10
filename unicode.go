package inkgo

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// StripANSI removes CSI/OSC/DCS/APC escape sequences. It is deliberately
// streaming-safe enough for measuring complete render strings; the input parser
// has its own tokenizer for partial sequences.
func StripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			r, n := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && n == 1 {
				i++
				continue
			}
			b.WriteRune(r)
			i += n
			continue
		}
		if i+1 >= len(s) {
			break
		}
		switch s[i+1] {
		case '[': // CSI: final byte 0x40..0x7e
			i += 2
			for i < len(s) {
				c := s[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
		case ']': // OSC: BEL or ST
			i += 2
			for i < len(s) {
				if s[i] == 0x07 {
					i++
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		case 'P', '_': // DCS/APC to ST
			i += 2
			for i < len(s) {
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default:
			// ESC + intermediate(s) + final.
			i += 2
			for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
				i++
			}
			if i < len(s) && s[i] >= 0x30 && s[i] <= 0x7e {
				i++
			}
		}
	}
	return b.String()
}

func isZeroWidth(r rune) bool {
	if r == 0 {
		return true
	}
	if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
		return true
	}
	if r == 0x00ad || r == 0x200b || r == 0x200c || r == 0x200d || r == 0xfeff {
		return true
	}
	if (r >= 0xfe00 && r <= 0xfe0f) || (r >= 0xe0100 && r <= 0xe01ef) {
		return true
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return true
	}
	if r >= 0xe0000 && r <= 0xe007f {
		return true
	}
	return false
}

func isEmojiRune(r rune) bool {
	return (r >= 0x1f300 && r <= 0x1faff) ||
		(r >= 0x2600 && r <= 0x27bf) ||
		(r >= 0x1f1e6 && r <= 0x1f1ff) ||
		(r >= 0x2300 && r <= 0x23ff)
}

// isWideRune follows wcwidth's conventional East Asian W/F ranges and treats
// ambiguous characters as narrow, matching the TypeScript fork.
func isWideRune(r rune) bool {
	if r < 0x1100 {
		return false
	}
	return r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0x303e) ||
		(r >= 0x3040 && r <= 0xa4cf) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f200 && r <= 0x1f251) ||
		(r >= 0x20000 && r <= 0x3fffd)
}

func RuneWidth(r rune) int {
	if isZeroWidth(r) {
		return 0
	}
	if isEmojiRune(r) || isWideRune(r) {
		return 2
	}
	return 1
}

type Grapheme struct {
	Text  string
	Width int
}

// Graphemes is a lightweight grapheme clusterer designed for terminal cell
// allocation. It groups combining marks, VS selectors, emoji modifiers,
// regional-indicator pairs and ZWJ chains.
func Graphemes(s string) []Grapheme {
	if s == "" {
		return nil
	}
	out := make([]Grapheme, 0, len(s))
	var cur strings.Builder
	curWidth := 0
	joinNext := false
	riCount := 0
	hadEmoji := false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		w := curWidth
		if hadEmoji && w > 0 {
			w = 2
		}
		// One isolated regional indicator is narrow; a flag pair is wide.
		if riCount == 1 && cur.Len() > 0 {
			w = 1
		} else if riCount >= 2 {
			w = 2
		}
		out = append(out, Grapheme{Text: cur.String(), Width: w})
		cur.Reset()
		curWidth = 0
		joinNext = false
		riCount = 0
		hadEmoji = false
	}

	for _, r := range s {
		combining := isZeroWidth(r) && r != '\n' && r != '\r' && r != '\t'
		emojiMod := r >= 0x1f3fb && r <= 0x1f3ff
		regional := r >= 0x1f1e6 && r <= 0x1f1ff
		if cur.Len() > 0 && !combining && !emojiMod && !joinNext {
			if !(regional && riCount == 1) {
				flush()
			}
		}
		cur.WriteRune(r)
		if regional {
			riCount++
			hadEmoji = true
		}
		if isEmojiRune(r) {
			hadEmoji = true
		}
		if !isZeroWidth(r) {
			curWidth += RuneWidth(r)
		}
		if r == 0x200d {
			joinNext = true
		} else if joinNext && !combining {
			// Keep the chain alive only if this rune is followed by a ZWJ later;
			// the next normal rune will flush before being appended.
			joinNext = false
		}
	}
	flush()
	return out
}

func StringWidth(s string) int {
	if s == "" {
		return 0
	}
	if strings.IndexByte(s, 0x1b) >= 0 {
		s = StripANSI(s)
	}
	w := 0
	for _, g := range Graphemes(s) {
		switch g.Text {
		case "\n", "\r":
			continue
		case "\t":
			w += DefaultTabInterval - (w % DefaultTabInterval)
		default:
			w += g.Width
		}
	}
	return w
}

func WidestLine(s string) int {
	m := 0
	for _, line := range strings.Split(StripANSI(s), "\n") {
		m = max(m, StringWidth(line))
	}
	return m
}

func SliceByWidth(s string, start, end int) string {
	if end <= start || s == "" {
		return ""
	}
	var b strings.Builder
	pos := 0
	for _, g := range Graphemes(s) {
		next := pos + g.Width
		if g.Width == 0 {
			if pos >= start && pos < end {
				b.WriteString(g.Text)
			}
			continue
		}
		if pos >= start && next <= end {
			b.WriteString(g.Text)
		} else if pos < end && next > start {
			// Wide glyph straddling a boundary is intentionally omitted; this
			// matches the fork's "sliceFit" invariant of never overshooting.
		}
		pos = next
		if pos >= end {
			break
		}
	}
	return b.String()
}
