package textutil

import (
	"strings"

	escscan "github.com/frudas24/inkgo/internal/escscan"
)

type terminalToken struct {
	text   string
	width  int
	escape bool
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
	out := make([]terminalToken, 0, len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			seq, consumed, preserve := scanOutputEscape(s[i:])
			if preserve {
				out = append(out, terminalToken{text: seq, escape: true})
			}
			i += consumed
			continue
		}

		j := i + 1
		for j < len(s) && s[j] != 0x1b {
			j++
		}
		// wrap-ansi splits words on literal ASCII spaces before grapheme
		// segmentation. Keep spaces as standalone tokens too; otherwise a
		// following zero-width control/combining rune can merge into the space
		// cluster and make trim/word-boundary detection miss it.
		plain := s[i:j]
		for len(plain) > 0 {
			space := strings.IndexByte(plain, ' ')
			if space < 0 {
				for _, g := range Graphemes(plain) {
					out = append(out, terminalToken{text: g.Text, width: g.Width})
				}
				break
			}
			if space > 0 {
				for _, g := range Graphemes(plain[:space]) {
					out = append(out, terminalToken{text: g.Text, width: g.Width})
				}
			}
			out = append(out, terminalToken{text: " ", width: 1})
			plain = plain[space+1:]
		}
		i = j
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

func restoreVisualStateAcrossRows(rows []string) []string {
	if len(rows) < 2 {
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
			if tok.escape {
				state.observe(tok.text)
			}
		}
		if i < len(rows)-1 && row != "" {
			b.WriteString(state.closing())
		}
		out[i] = b.String()
	}
	return out
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
			tokens = append(tokens[:i], tokens[i+1:]...)
			continue
		}
		if tok.width > 0 {
			seenVisible = true
		}
	}
	return tokens
}
