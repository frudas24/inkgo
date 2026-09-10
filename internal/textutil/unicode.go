package textutil

import (
	"strings"
	"unicode"
	"unicode/utf8"

	escscan "github.com/frudas24/inkgo/internal/escscan"
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
			b.WriteString(s[i : i+n])
			i += n
			continue
		}

		if seq, ok := escscan.NextANSISequence(s[i:]); ok {
			i += len(seq)
			continue
		}
		if i+1 >= len(s) {
			break // trailing ESC is an incomplete control introducer
		}
		// Unterminated string/CSI controls are unsafe to expose as visible text.
		// Drop their remaining payload. Unknown ESC forms drop only ESC so the
		// following byte can still be handled as ordinary text/control data.
		switch s[i+1] {
		case '[', ']', 'P', '_', '^':
			return b.String()
		default:
			i++
		}
	}
	return b.String()
}

func sanitizeUTF8(s string) string {
	if s == "" || utf8.ValidString(s) {
		return s
	}
	var clean strings.Builder
	clean.Grow(len(s))
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && n == 1 {
			s = s[1:]
			continue
		}
		clean.WriteString(s[:n])
		s = s[n:]
	}
	return clean.String()
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

func isEmojiCandidateRune(r rune) bool {
	return r == '#' || r == '*' || (r >= '0' && r <= '9') ||
		(r >= 0x2300 && r <= 0x23ff) ||
		(r >= 0x2600 && r <= 0x27bf) ||
		(r >= 0x1f000 && r <= 0x1faff)
}

// isExtendedPictographic approximates Unicode's Extended_Pictographic property
// for the terminal-relevant subset, which UAX #29 GB11 needs to decide whether
// a ZWJ joins the following rune. Keycap bases (ASCII digits, '#' and '*') are
// deliberately excluded: they are emoji candidates but never head a ZWJ
// sequence, and treating them as pictographic fuses digit ZWJ runs into a
// single cluster wider than the two-cell model can render.
func isExtendedPictographic(r rune) bool {
	if isEmojiPresentationRune(r) {
		return true
	}
	switch {
	case r == 0x00a9 || r == 0x00ae || r == 0x2122,
		r == 0x203c || r == 0x2049,
		r == 0x2139,
		r == 0x3030 || r == 0x303d,
		r == 0x3297 || r == 0x3299,
		r == 0x21a9 || r == 0x21aa,
		r >= 0x2194 && r <= 0x2199,
		r >= 0x2300 && r <= 0x23ff,
		r >= 0x25a0 && r <= 0x27bf,
		r >= 0x2900 && r <= 0x2bff:
		return true
	}
	return false
}

// isEmojiPresentationRune is the compact terminal-relevant subset of Unicode's
// Emoji_Presentation property. Text-default symbols such as U+26A0 WARNING SIGN
// intentionally remain narrow unless VS16 requests emoji presentation.
func isEmojiPresentationRune(r rune) bool {
	switch {
	case r == 0x231a || r == 0x231b,
		r >= 0x23e9 && r <= 0x23ec,
		r == 0x23f0 || r == 0x23f3,
		r >= 0x25fd && r <= 0x25fe,
		r >= 0x2614 && r <= 0x2615,
		r >= 0x2648 && r <= 0x2653,
		r == 0x267f || r == 0x2693 || r == 0x26a1,
		r >= 0x26aa && r <= 0x26ab,
		r >= 0x26bd && r <= 0x26be,
		r >= 0x26c4 && r <= 0x26c5,
		r == 0x26ce || r == 0x26d4 || r == 0x26ea,
		r >= 0x26f2 && r <= 0x26f3,
		r == 0x26f5 || r == 0x26fa || r == 0x26fd,
		r == 0x2705 || r == 0x270a || r == 0x270b || r == 0x2728,
		r == 0x274c || r == 0x274e,
		r >= 0x2753 && r <= 0x2755,
		r == 0x2757,
		r >= 0x2795 && r <= 0x2797,
		r == 0x27b0 || r == 0x27bf,
		r >= 0x2b1b && r <= 0x2b1c,
		r == 0x2b50 || r == 0x2b55,
		r == 0x1f004 || r == 0x1f0cf || r == 0x1f18e,
		r >= 0x1f191 && r <= 0x1f19a,
		r == 0x1f201 || r == 0x1f21a || r == 0x1f22f,
		r >= 0x1f232 && r <= 0x1f236,
		r >= 0x1f238 && r <= 0x1f23a,
		r >= 0x1f250 && r <= 0x1f251,
		r >= 0x1f300 && r <= 0x1faff,
		r >= 0x1f1e6 && r <= 0x1f1ff:
		return true
	default:
		return false
	}
}

func isKeycapBase(r rune) bool { return r == '#' || r == '*' || (r >= '0' && r <= '9') }

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
	if isEmojiPresentationRune(r) || isWideRune(r) {
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
	// Terminal text is UTF-8. Go strings may still contain arbitrary bytes, so
	// drop malformed bytes while preserving a genuinely encoded U+FFFD. This
	// keeps width, wrapping, slicing and tab expansion on one text model.
	s = sanitizeUTF8(s)
	if s == "" {
		return nil
	}
	out := make([]Grapheme, 0, len(s))
	var cur strings.Builder
	curWidth := 0
	joinNext := false
	riCount := 0
	emojiWide := false
	emojiCandidate := false
	hasVS16 := false
	hasVS15 := false
	keycapBase := false
	keycapComplete := false
	firstBase := rune(0)
	curIsControl := false
	// Extended_Pictographic base immediately before the pending ZWJ. UAX #29
	// GB11 joins only \p{Extended_Pictographic} Extend* ZWJ x \p{Extended_
	// Pictographic}; without this, ASCII digits or '#' joined by a ZWJ fuse
	// into one cluster wider than the two-cell model can render.
	lastPictographic := false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		w := curWidth
		switch {
		case keycapComplete:
			w = 2
		case hasVS15 && firstBase != 0:
			// VS15 explicitly requests text presentation. Ignore an otherwise
			// emoji-default width for the base, but retain East Asian W/F width.
			w = 1
			if isWideRune(firstBase) {
				w = 2
			}
		case hasVS16 && emojiCandidate && !keycapBase:
			w = 2
		case emojiWide && w > 0:
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
		emojiWide = false
		emojiCandidate = false
		hasVS16 = false
		hasVS15 = false
		keycapBase = false
		keycapComplete = false
		firstBase = 0
		curIsControl = false
		lastPictographic = false
	}

	for _, r := range s {
		control := r < 0x20 || (r >= 0x7f && r <= 0x9f)
		// Zero-width terminal controls are not combining marks. Keeping them
		// as standalone graphemes prevents CR/C0/C1 bytes from being hidden
		// inside a neighboring cluster while still assigning them zero cells.
		combining := isZeroWidth(r) && !control
		emojiMod := r >= 0x1f3fb && r <= 0x1f3ff
		if cur.Len() > 0 && curIsControl {
			flush()
		}
		regional := r >= 0x1f1e6 && r <= 0x1f1ff
		zwjJoins := joinNext && lastPictographic && isExtendedPictographic(r)
		if cur.Len() > 0 && !combining && !emojiMod && !zwjJoins {
			if !(regional && riCount == 1) {
				flush()
			}
		}
		if cur.Len() == 0 {
			curIsControl = control
		}
		cur.WriteRune(r)
		if regional {
			riCount++
			emojiWide = true
		}
		if isEmojiCandidateRune(r) {
			emojiCandidate = true
		}
		if isEmojiPresentationRune(r) {
			emojiWide = true
		}
		if r == 0xfe0f {
			hasVS16 = true
		}
		if r == 0xfe0e {
			hasVS15 = true
		}
		if r == 0x20e3 && keycapBase {
			keycapComplete = true
		}
		if !isZeroWidth(r) {
			if firstBase == 0 {
				firstBase = r
				keycapBase = isKeycapBase(r)
			}
			curWidth += RuneWidth(r)
			// Emoji modifiers are GB11 "Extend" bytes: they attach to the base
			// and must not break the pictographic chain before a following ZWJ.
			if !emojiMod {
				lastPictographic = isExtendedPictographic(r)
			}
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
		case "\n", "\r", "\t":
			continue
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
	tokens := terminalTokens(s)
	var b strings.Builder
	pos := 0
	started := false
	state := sliceANSIState{}

	startOutput := func() {
		if started {
			return
		}
		b.WriteString(state.opening())
		started = true
	}

	for _, tok := range tokens {
		if tok.escape {
			state.observe(tok.text)
			// Before the first selected grapheme, only reconstruct visual state at
			// the boundary; do not replay arbitrary cursor/erase/control history.
			if started && pos < end {
				b.WriteString(tok.text)
			}
			continue
		}
		next := pos + tok.width
		if tok.width == 0 {
			if pos >= start && pos < end {
				startOutput()
				b.WriteString(tok.text)
			}
			continue
		}
		if pos >= start && next <= end {
			startOutput()
			b.WriteString(tok.text)
		}
		// Wide glyphs straddling a boundary are omitted, matching the fork's
		// sliceFit invariant of never overshooting the requested cell range.
		pos = next
		if pos >= end {
			break
		}
	}
	if !started {
		return ""
	}
	b.WriteString(state.closing())
	return b.String()
}
