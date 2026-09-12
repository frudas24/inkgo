package textutil

import (
	"strings"

	escscan "github.com/frudas24/inkgo/internal/escscan"
)

type terminalToken struct {
	text   string
	width  int
	escape bool
	// ansi records preserved escape sequences embedded inside a visible
	// grapheme token. ANSI controls are zero-width and therefore do not form a
	// grapheme boundary: e.g. "0\x1b[31m\u20e3" is still one keycap cluster.
	// Keeping the sequences here lets slicing/state restoration observe their
	// side effects without splitting the visual cluster used by wrapping.
	ansi []string
}

func scanOutputEscape(s string) (seq string, consumed int, preserve bool) {
	if s == "" || s[0] != 0x1b {
		return "", 0, false
	}
	if seq, ok := escscan.NextANSISequence(s); ok {
		return seq, len(seq), true
	}
	if len(s) == 1 {
		return "", 1, false
	}
	// CSI/OSC/DCS/APC introducers without a terminator/final are incomplete
	// output controls. Drop the remainder rather than emitting a dangling
	// terminal control into rendered text. Unknown forms drop only ESC so the
	// following byte can still be handled as text/control data.
	switch s[1] {
	case '[', ']', 'P', '_':
		return "", len(s), false
	default:
		return "", 1, false
	}
}

func terminalTokens(s string) []terminalToken {
	s = sanitizeUTF8(s)
	if s == "" {
		return nil
	}

	// First remove malformed/incomplete output controls and record valid ANSI
	// sequences by their byte position in the visible stream. Grapheme
	// segmentation must happen *after* that normalization: an ANSI sequence (or
	// a dropped stray ESC) between a base and a combining rune does not create a
	// Unicode grapheme boundary.
	type ansiEvent struct {
		pos int
		seq string
	}
	var visible strings.Builder
	events := make([]ansiEvent, 0, 8)
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			seq, consumed, preserve := scanOutputEscape(s[i:])
			if preserve {
				events = append(events, ansiEvent{pos: visible.Len(), seq: seq})
			}
			i += consumed
			continue
		}
		j := i + 1
		for j < len(s) && s[j] != 0x1b {
			j++
		}
		visible.WriteString(s[i:j])
		i = j
	}

	plain := visible.String()
	out := make([]terminalToken, 0, len(plain)+len(events))
	eventIndex := 0
	pos := 0

	emitBoundaryANSI := func(at int) {
		for eventIndex < len(events) && events[eventIndex].pos == at {
			out = append(out, terminalToken{text: events[eventIndex].seq, escape: true})
			eventIndex++
		}
	}

	for _, g := range Graphemes(plain) {
		start := pos
		end := start + len(g.Text)
		emitBoundaryANSI(start)

		// Preserve literal ASCII spaces as standalone delimiter tokens. The
		// existing word-wrap semantics intentionally split on those spaces, while
		// any combining suffix stays zero-width and follows the delimiter.
		if strings.HasPrefix(g.Text, " ") {
			out = append(out, terminalToken{text: " ", width: 1})
			cursor := start + 1
			if cursor < end {
				var b strings.Builder
				var embedded []string
				for eventIndex < len(events) && events[eventIndex].pos < end {
					ev := events[eventIndex]
					if ev.pos > cursor {
						b.WriteString(plain[cursor:ev.pos])
					}
					b.WriteString(ev.seq)
					embedded = append(embedded, ev.seq)
					cursor = ev.pos
					eventIndex++
				}
				if cursor < end {
					b.WriteString(plain[cursor:end])
				}
				if b.Len() > 0 {
					out = append(out, terminalToken{text: b.String(), ansi: embedded})
				}
			}
			pos = end
			continue
		}

		var b strings.Builder
		var embedded []string
		cursor := start
		for eventIndex < len(events) && events[eventIndex].pos < end {
			ev := events[eventIndex]
			if ev.pos > cursor {
				b.WriteString(plain[cursor:ev.pos])
			}
			b.WriteString(ev.seq)
			embedded = append(embedded, ev.seq)
			cursor = ev.pos
			eventIndex++
		}
		if cursor < end {
			b.WriteString(plain[cursor:end])
		}
		out = append(out, terminalToken{text: b.String(), width: g.Width, ansi: embedded})
		pos = end
	}

	emitBoundaryANSI(len(plain))
	// Defensive only: events are emitted in monotonically increasing visible
	// positions, so all of them should have been consumed above.
	for eventIndex < len(events) {
		out = append(out, terminalToken{text: events[eventIndex].seq, escape: true})
		eventIndex++
	}
	return out
}

type sliceANSIState struct {
	sgrHistory []string
	hyperlink  string
}

func isSGRSequence(seq string) (string, bool) {
	if len(seq) < 3 || !strings.HasPrefix(seq, "\x1b[") || seq[len(seq)-1] != 'm' {
		return "", false
	}
	return seq[2 : len(seq)-1], true
}

func sgrContainsReset(params string) bool {
	if params == "" {
		return true
	}
	for _, part := range strings.Split(params, ";") {
		// Empty SGR parameters are reset (0) in ECMA-48.
		if part == "" || part == "0" {
			return true
		}
	}
	return false
}

func osc8URI(seq string) (uri string, ok bool) {
	if !strings.HasPrefix(seq, "\x1b]8;") {
		return "", false
	}
	payload := seq[2:]
	if strings.HasSuffix(payload, "\x07") {
		payload = payload[:len(payload)-1]
	} else if strings.HasSuffix(payload, "\x1b\\") {
		payload = payload[:len(payload)-2]
	} else {
		return "", false
	}
	parts := strings.SplitN(payload, ";", 3)
	if len(parts) != 3 || parts[0] != "8" {
		return "", false
	}
	return parts[2], true
}

func (s *sliceANSIState) observe(seq string) {
	if params, ok := isSGRSequence(seq); ok {
		if sgrContainsReset(params) {
			s.sgrHistory = s.sgrHistory[:0]
		}
		// Retaining the exact SGR after the last full reset reproduces selective
		// opens/closes at a slice boundary without needing a second style model.
		// A reset-only sequence is unnecessary when replaying from default.
		if params != "" && params != "0" {
			s.sgrHistory = append(s.sgrHistory, seq)
		}
		return
	}
	if uri, ok := osc8URI(seq); ok {
		if uri == "" {
			s.hyperlink = ""
		} else {
			s.hyperlink = seq
		}
	}
}

func (s *sliceANSIState) opening() string {
	var b strings.Builder
	for _, seq := range s.sgrHistory {
		b.WriteString(seq)
	}
	b.WriteString(s.hyperlink)
	return b.String()
}

func (s *sliceANSIState) closing() string {
	var b strings.Builder
	if s.hyperlink != "" {
		b.WriteString("\x1b]8;;\x07")
	}
	if len(s.sgrHistory) > 0 {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func observeTerminalTokenANSI(state *sliceANSIState, tok terminalToken) {
	if tok.escape {
		state.observe(tok.text)
		return
	}
	for _, seq := range tok.ansi {
		state.observe(seq)
	}
}

func restoreVisualStateAcrossRows(rows []string) []string {
	if len(rows) < 2 {
		return rows
	}
	// An escape-free row cannot carry visual state, so reopening the inherited
	// state and closing it again would change nothing - while re-tokenizing every
	// row to discover that cost a full grapheme segmentation pass per row on the
	// resize path, where a long transcript re-wraps every text node.
	if !anyEscapeSequence(rows) {
		return rows
	}
	state := sliceANSIState{}
	out := make([]string, len(rows))
	for i, row := range rows {
		var b strings.Builder
		// A generated non-empty continuation row must stand on its own. Reopen
		// the visual state inherited from the previous row; empty rows have
		// nothing to style and intentionally remain empty.
		if i > 0 && row != "" {
			b.WriteString(state.opening())
		}
		b.WriteString(row)
		for _, tok := range terminalTokens(row) {
			observeTerminalTokenANSI(&state, tok)
		}
		if i < len(rows)-1 && row != "" {
			b.WriteString(state.closing())
		}
		out[i] = b.String()
	}
	return out
}

func anyEscapeSequence(rows []string) bool {
	for _, row := range rows {
		if strings.IndexByte(row, 0x1b) >= 0 {
			return true
		}
	}
	return false
}

func tokensString(tokens []terminalToken) string {
	var b strings.Builder
	for _, tok := range tokens {
		b.WriteString(tok.text)
	}
	return b.String()
}

func tokensWidth(tokens []terminalToken) int {
	w := 0
	for _, tok := range tokens {
		w += tok.width
	}
	return w
}

func visibleTrimSpacesRight(tokens []terminalToken) []terminalToken {
	seenVisible := false
	for i := len(tokens) - 1; i >= 0; i-- {
		tok := tokens[i]
		if tok.escape {
			continue
		}
		if !seenVisible && tok.text == " " {
			// The tokenizer splits a literal space away from zero-width runes
			// that share its grapheme cluster (a variation selector or a
			// combining mark). Leaving those behind would let them re-attach to
			// the preceding base after the space is dropped - a trailing VS16
			// on a variation base widens it, so the row would measure wider
			// than the token accounting that produced it.
			end := i + 1
			for end < len(tokens) && !tokens[end].escape && tokens[end].width == 0 {
				end++
			}
			tokens = append(tokens[:i], tokens[end:]...)
			continue
		}
		if tok.width > 0 {
			seenVisible = true
		}
	}
	return tokens
}
