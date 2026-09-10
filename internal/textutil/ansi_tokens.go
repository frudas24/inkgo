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

func terminalTokens(s string) []terminalToken {
	if s == "" {
		return nil
	}
	out := make([]terminalToken, 0, len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			if seq, ok := escscan.NextSequence(s[i:]); ok {
				out = append(out, terminalToken{text: seq, escape: true})
				i += len(seq)
				continue
			}
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
