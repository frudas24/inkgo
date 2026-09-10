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
	// East Asian W/F is cheap to classify and dominates ordinary CJK text.
	// Emoji_Presentation starts at U+231A, so Latin and other low code points
	// avoid a binary property-table lookup entirely.
	if isWideRune(r) || (r >= 0x231a && isEmojiPresentationRune(r)) {
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
	lastRegional := false
	emojiWide := false
	hasVS16 := false
	hasVS15 := false
	keycapStage := uint8(0) // 1=base, 2=base+VS16, 3=complete
	keycapComplete := false
	firstBase := rune(0)
	prevRune := rune(0)
	havePrev := false
	curIsControl := false
	// lastPictographic means the current suffix is Extended_Pictographic
	// followed only by Extend-like runes accepted by this terminal profile.
	// A following ZWJ consumes that suffix and arms exactly one GB11 join.
	lastPictographic := false
	// hasPictographicZWJ records that this cluster contains at least one
	// successful GB11 join. The screen model can represent a grapheme in at
	// most two cells, and the TypeScript source treats emoji/ZWJ graphemes as
	// a single terminal glyph. Text-default pictographs (for example ☀) are
	// otherwise narrow, so without this marker a chain such as ☀‍☀‍☀ would
	// accumulate width 3 even though it is one grapheme cluster.
	hasPictographicZWJ := false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		w := curWidth
		switch {
		case keycapComplete:
			w = 2
		case hasPictographicZWJ && w > 0:
			// A successful GB11 chain is one terminal emoji glyph. Selector
			// details inside the chain must not narrow the whole grapheme.
			w = 2
		case hasVS15 && firstBase != 0:
			// A *valid adjacent* VS15 requests text presentation. Ignore an
			// otherwise emoji-default width for the base, but retain East Asian
			// W/F width.
			w = 1
			if isWideRune(firstBase) {
				w = 2
			}
		case hasVS16 && firstBase != 0 && !isKeycapBase(firstBase):
			// A bare keycap base + VS16 is still incomplete and stays narrow.
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
		lastRegional = false
		emojiWide = false
		hasVS16 = false
		hasVS15 = false
		keycapStage = 0
		keycapComplete = false
		firstBase = 0
		prevRune = 0
		havePrev = false
		curIsControl = false
		lastPictographic = false
		hasPictographicZWJ = false
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
		// joinNext means the immediately preceding ZWJ was itself preceded by
		// Extended_Pictographic Extend*. GB11 permits the join only when the
		// *next* rune is Extended_Pictographic; even another Extend or ZWJ
		// after that ZWJ invalidates this opportunity.
		zwjJoins := joinNext && isExtendedPictographic(r)
		if joinNext && !zwjJoins {
			joinNext = false
		}
		if zwjJoins {
			hasPictographicZWJ = true
		}
		if cur.Len() > 0 && !combining && !emojiMod && !zwjJoins {
			// GB12/GB13 pair adjacent regional indicators only. riCount alone
			// is insufficient because GB9 can attach a ZWJ/Extend after an RI;
			// that intervening rune breaks the RI sequence before the next RI.
			if !(regional && lastRegional && riCount == 1) {
				flush()
			}
		}
		if cur.Len() == 0 {
			curIsControl = control
		}
		hadContent := cur.Len() > 0
		cur.WriteRune(r)
		if hadContent {
			switch keycapStage {
			case 1:
				switch r {
				case 0xfe0f:
					keycapStage = 2
				case 0x20e3:
					keycapStage = 3
					keycapComplete = true
				default:
					keycapStage = 0
				}
			case 2:
				if r == 0x20e3 {
					keycapStage = 3
					keycapComplete = true
				} else {
					keycapStage = 0
				}
			}
		}
		if regional {
			riCount++
			emojiWide = true
		}
		if isEmojiPresentationRune(r) {
			emojiWide = true
		}
		// UTS #51 variation selectors are two-code-point sequences. Do not
		// let a selector elsewhere in the grapheme retroactively change the
		// presentation of an earlier emoji base.
		if (r == 0xfe0f || r == 0xfe0e) && havePrev && isEmojiVariationBase(prevRune) {
			if r == 0xfe0f {
				hasVS16 = true
			} else {
				hasVS15 = true
			}
		}
		if !isZeroWidth(r) {
			if firstBase == 0 {
				firstBase = r
				if isKeycapBase(r) {
					keycapStage = 1
				}
			}
			curWidth += RuneWidth(r)
		}

		// Track the exact GB11 suffix shape needed by the next boundary.
		// lastPictographic means the current suffix is EP Extend*. A ZWJ
		// consumes that suffix and arms exactly one possible EP join.
		switch {
		case r == 0x200d:
			joinNext = lastPictographic
			lastPictographic = false
		case zwjJoins:
			joinNext = false
			lastPictographic = true
		case combining || emojiMod:
			// Extend before a ZWJ preserves EP Extend*. Extend after a ZWJ
			// already cleared joinNext above and cannot re-arm it.
		default:
			joinNext = false
			lastPictographic = isExtendedPictographic(r)
		}
		prevRune = r
		havePrev = true
		lastRegional = regional
	}
	flush()
	return out
}

func asciiStringWidth(s string) (int, bool) {
	width := 0
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= utf8.RuneSelf || b == 0x1b {
			return 0, false
		}
		if b > 0x1f && b != 0x7f {
			width++
		}
	}
	return width, true
}

func simpleStringWidth(s string) (int, bool) {
	width := 0
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && n == 1 {
			// Graphemes deliberately drops malformed UTF-8 bytes; fall back so
			// malformed input keeps exactly the same sanitization policy.
			return 0, false
		}
		switch {
		case r == 0x200d, // ZWJ / GB11
			r == 0xfe0e || r == 0xfe0f,   // text/emoji presentation selectors
			r == 0x20e3,                  // combining enclosing keycap
			r >= 0x1f1e6 && r <= 0x1f1ff, // regional-indicator pairing
			r >= 0x1f3fb && r <= 0x1f3ff: // emoji modifiers attach to a base
			return 0, false
		}
		width += RuneWidth(r)
		s = s[n:]
	}
	return width, true
}

func StringWidth(s string) int {
	if s == "" {
		return 0
	}
	// Mirror the source fork's hot-path contract: pure ASCII without ANSI does
	// not need grapheme segmentation. Controls (including TAB/CR/LF and DEL)
	// occupy zero terminal cells. Besides being substantially faster for the
	// common case, this keeps the exact Unicode property tables off ASCII paths.
	if width, ok := asciiStringWidth(s); ok {
		return width
	}
	if strings.IndexByte(s, 0x1b) >= 0 {
		s = StripANSI(s)
		// Colored ASCII is another common hot path. Once controls are stripped,
		// avoid rebuilding grapheme state just to count ordinary bytes.
		if width, ok := asciiStringWidth(s); ok {
			return width
		}
	}
	// Most Unicode text does not need cluster-dependent width rules. Combining
	// marks are already zero width and ordinary W/F characters can be summed
	// directly. Fall back only for sequences whose width depends on neighbors.
	if width, ok := simpleStringWidth(s); ok {
		return width
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
