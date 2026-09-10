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

// NextANSISequence returns the first complete ANSI/ECMA-48 output control
// sequence at the beginning of s. Unlike NextSequence, it does not treat
// ESC+arbitrary-rune as an Alt/meta input key. Text rendering uses this
// stricter scanner so controls such as TAB following a stray ESC remain text
// controls and are not hidden inside a zero-width token.
func NextANSISequence(s string) (string, bool) {
	if len(s) < 2 || s[0] != 0x1b {
		return "", false
	}
	switch s[1] {
	case '[': // CSI: final byte 0x40..0x7e
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return s[:i+1], true
			}
		}
		return "", false
	case ']': // OSC: BEL or ST
		if i := strings.IndexByte(s[2:], 0x07); i >= 0 {
			return s[:2+i+1], true
		}
		if i := strings.Index(s[2:], st); i >= 0 {
			return s[:2+i+len(st)], true
		}
		return "", false
	case 'P', '_', '^': // DCS/APC/PM to ST
		if i := strings.Index(s[2:], st); i >= 0 {
			return s[:2+i+len(st)], true
		}
		return "", false
	}

	// Generic 7-bit ESC sequence: zero or more intermediate bytes
	// (0x20..0x2f), followed by one final byte (0x30..0x7e).
	i := 1
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
		i++
	}
	if i < len(s) && s[i] >= 0x30 && s[i] <= 0x7e {
		return s[:i+1], true
	}
	return "", false
}
