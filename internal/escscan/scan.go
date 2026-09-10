// Package escscan contains streaming-safe terminal escape boundary scanning.
// It deliberately does not interpret sequences; parsers and text transforms
// share it so ANSI preservation cannot drift between domains.
package escscan

import (
	"strings"
	"unicode/utf8"
)

const st = "\x1b\\"

// NextSequence returns the first complete escape sequence at the beginning of
// s. It recognizes CSI, OSC, DCS, X10 mouse, SS3 and Alt/meta UTF-8 keys.
func NextSequence(s string) (string, bool) {
	if len(s) < 2 {
		return "", false
	}
	if s[0] != 0x1b {
		return "", false
	}
	if s[1] == '[' {
		if strings.HasPrefix(s, "\x1b[M") {
			if len(s) < 6 {
				return "", false
			}
			return s[:6], true
		}
		for i := 2; i < len(s); i++ {
			b := s[i]
			if b >= 0x40 && b <= 0x7e {
				return s[:i+1], true
			}
		}
		return "", false
	}
	if s[1] == ']' {
		if i := strings.IndexByte(s[2:], 0x07); i >= 0 {
			return s[:2+i+1], true
		}
		if i := strings.Index(s[2:], st); i >= 0 {
			return s[:2+i+len(st)], true
		}
		return "", false
	}
	if s[1] == 'P' || s[1] == '_' {
		if i := strings.Index(s[2:], st); i >= 0 {
			return s[:2+i+len(st)], true
		}
		return "", false
	}
	if s[1] == 'O' {
		if len(s) < 3 {
			return "", false
		}
		return s[:3], true
	}
	_, n := utf8.DecodeRuneInString(s[1:])
	if n == 0 || (n == 1 && s[1] >= 0x80) {
		return "", false
	}
	return s[:1+n], true
}
