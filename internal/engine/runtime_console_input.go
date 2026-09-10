package engine

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// consoleInputEncoder converts Windows console INPUT_RECORD payloads into the
// same VT byte stream understood by InputParser. Keeping the conversion in a
// platform-neutral file lets the behavior be tested on every CI runner while
// runtime_console_input_windows.go remains the only syscall boundary.
type consoleInputEncoder struct {
	pendingHighSurrogate uint16
	mouseButtons         uint32
	wheelRemainder       int
}

type consoleKeyEvent struct {
	Down       bool
	Repeat     uint16
	VirtualKey uint16
	Unicode    uint16
	Control    uint32
}

type consoleMouseEvent struct {
	X, Y       int16
	Buttons    uint32
	Control    uint32
	EventFlags uint32
}

const (
	consoleRightAltPressed  = 0x0001
	consoleLeftAltPressed   = 0x0002
	consoleRightCtrlPressed = 0x0004
	consoleLeftCtrlPressed  = 0x0008
	consoleShiftPressed     = 0x0010

	consoleMouseMoved    = 0x0001
	consoleMouseWheeled  = 0x0004
	consoleMouseHWheeled = 0x0008

	consoleVKBack   = 0x08
	consoleVKTab    = 0x09
	consoleVKReturn = 0x0d
	consoleVKEscape = 0x1b
	consoleVKSpace  = 0x20
	consoleVKPrior  = 0x21
	consoleVKNext   = 0x22
	consoleVKEnd    = 0x23
	consoleVKHome   = 0x24
	consoleVKLeft   = 0x25
	consoleVKUp     = 0x26
	consoleVKRight  = 0x27
	consoleVKDown   = 0x28
	consoleVKInsert = 0x2d
	consoleVKDelete = 0x2e
	consoleVKF1     = 0x70
	consoleVKF12    = 0x7b
)

func consoleModifier(control uint32) (modifier int, shift, alt, ctrl bool) {
	shift = control&consoleShiftPressed != 0
	alt = control&(consoleLeftAltPressed|consoleRightAltPressed) != 0
	ctrl = control&(consoleLeftCtrlPressed|consoleRightCtrlPressed) != 0
	modifier = 1
	if shift {
		modifier++
	}
	if alt {
		modifier += 2
	}
	if ctrl {
		modifier += 4
	}
	return modifier, shift, alt, ctrl
}

func appendRepeated(dst []byte, sequence string, repeat uint16) []byte {
	if repeat == 0 {
		repeat = 1
	}
	for i := uint16(0); i < repeat; i++ {
		dst = append(dst, sequence...)
	}
	return dst
}

func csiU(codepoint, modifier int) string {
	if modifier <= 1 {
		return fmt.Sprintf("\x1b[%du", codepoint)
	}
	return fmt.Sprintf("\x1b[%d;%du", codepoint, modifier)
}

func xtermFinal(final byte, modifier int) string {
	if modifier <= 1 {
		return "\x1b[" + string(final)
	}
	return fmt.Sprintf("\x1b[1;%d%c", modifier, final)
}

func xtermTilde(code, modifier int) string {
	if modifier <= 1 {
		return fmt.Sprintf("\x1b[%d~", code)
	}
	return fmt.Sprintf("\x1b[%d;%d~", code, modifier)
}

func functionKeySequence(vk uint16, modifier int) string {
	index := int(vk-consoleVKF1) + 1
	if index < 1 || index > 12 {
		return ""
	}
	if index <= 4 {
		final := byte('P' + index - 1)
		if modifier <= 1 {
			return "\x1bO" + string(final)
		}
		return fmt.Sprintf("\x1b[1;%d%c", modifier, final)
	}
	codes := [...]int{15, 17, 18, 19, 20, 21, 23, 24}
	return xtermTilde(codes[index-5], modifier)
}

func (e *consoleInputEncoder) runesFromUTF16(w uint16) []rune {
	if w >= 0xd800 && w <= 0xdbff {
		if e.pendingHighSurrogate != 0 {
			e.pendingHighSurrogate = w
			return []rune{utf8.RuneError}
		}
		e.pendingHighSurrogate = w
		return nil
	}
	if w >= 0xdc00 && w <= 0xdfff {
		if e.pendingHighSurrogate == 0 {
			return []rune{utf8.RuneError}
		}
		high := e.pendingHighSurrogate
		e.pendingHighSurrogate = 0
		return []rune{utf16.DecodeRune(rune(high), rune(w))}
	}
	if e.pendingHighSurrogate != 0 {
		e.pendingHighSurrogate = 0
		return []rune{utf8.RuneError, rune(w)}
	}
	return []rune{rune(w)}
}

func encodeConsoleRune(r rune, modifier int, altGr, ctrl, alt, shift bool) string {
	// Plain Ctrl+A..Ctrl+Z already arrives as a C0 codepoint. Preserve the
	// byte for compatibility with ordinary terminal input. When extra
	// modifiers are present, however, the raw C0 byte cannot carry them, so
	// reconstruct the printable base letter and use CSI-u instead.
	if r > 0 && r < 0x20 && ctrl {
		if !alt && !shift {
			return string(r)
		}
		if r >= 1 && r <= 26 {
			base := rune('a') + r - 1
			if shift {
				base = rune('A') + r - 1
			}
			return csiU(int(base), modifier)
		}
	}
	if modifier == 1 || altGr {
		return string(r)
	}
	return csiU(int(r), modifier)
}

func (e *consoleInputEncoder) key(event consoleKeyEvent) []byte {
	if !event.Down {
		return nil
	}
	repeat := event.Repeat
	if repeat == 0 {
		repeat = 1
	}
	modifier, shift, alt, ctrl := consoleModifier(event.Control)

	if event.Unicode != 0 {
		runes := e.runesFromUTF16(event.Unicode)
		if len(runes) == 0 {
			return nil
		}
		var out []byte
		for _, r := range runes {
			// AltGr is reported as RightAlt+Ctrl on Windows. When it produced a
			// printable Unicode character, it is text input, not an Alt+Ctrl chord.
			altGr := event.Control&consoleRightAltPressed != 0 && ctrl && r >= 0x20
			out = appendRepeated(out, encodeConsoleRune(r, modifier, altGr, ctrl, alt, shift), repeat)
		}
		return out
	}

	var seq string
	switch event.VirtualKey {
	case consoleVKBack:
		seq = csiU(127, modifier)
	case consoleVKTab:
		if shift && !alt && !ctrl {
			seq = "\x1b[Z"
		} else {
			seq = csiU(9, modifier)
		}
	case consoleVKReturn:
		seq = csiU(13, modifier)
	case consoleVKEscape:
		seq = csiU(27, modifier)
	case consoleVKSpace:
		seq = csiU(32, modifier)
	case consoleVKUp:
		seq = xtermFinal('A', modifier)
	case consoleVKDown:
		seq = xtermFinal('B', modifier)
	case consoleVKRight:
		seq = xtermFinal('C', modifier)
	case consoleVKLeft:
		seq = xtermFinal('D', modifier)
	case consoleVKHome:
		seq = xtermFinal('H', modifier)
	case consoleVKEnd:
		seq = xtermFinal('F', modifier)
	case consoleVKInsert:
		seq = xtermTilde(2, modifier)
	case consoleVKDelete:
		seq = xtermTilde(3, modifier)
	case consoleVKPrior:
		seq = xtermTilde(5, modifier)
	case consoleVKNext:
		seq = xtermTilde(6, modifier)
	default:
		if event.VirtualKey >= consoleVKF1 && event.VirtualKey <= consoleVKF12 {
			seq = functionKeySequence(event.VirtualKey, modifier)
		}
	}
	if seq == "" {
		return nil
	}
	return appendRepeated(nil, seq, repeat)
}

func mouseModifierBits(control uint32) int {
	bits := 0
	if control&consoleShiftPressed != 0 {
		bits |= 4
	}
	if control&(consoleLeftAltPressed|consoleRightAltPressed) != 0 {
		bits |= 8
	}
	if control&(consoleLeftCtrlPressed|consoleRightCtrlPressed) != 0 {
		bits |= 16
	}
	return bits
}

func firstConsoleMouseButton(state uint32) int {
	switch {
	case state&0x0001 != 0:
		return 0 // left
	case state&0x0004 != 0:
		return 1 // middle / second button
	case state&0x0002 != 0:
		return 2 // right
	default:
		return 3
	}
}

func sgrMouseSequence(button, col, row int, release bool) string {
	final := 'M'
	if release {
		final = 'm'
	}
	return fmt.Sprintf("\x1b[<%d;%d;%d%c", button, col, row, final)
}

func signedHighWord(v uint32) int16 { return int16(uint16(v >> 16)) }

func (e *consoleInputEncoder) mouse(event consoleMouseEvent) []byte {
	col, row := int(event.X)+1, int(event.Y)+1
	if col <= 0 || row <= 0 {
		return nil
	}
	mods := mouseModifierBits(event.Control)
	if event.EventFlags&consoleMouseWheeled != 0 {
		// Windows reports wheel distance in multiples or fractions of
		// WHEEL_DELTA (120). Accumulate sub-deltas and emit one terminal wheel
		// event for each complete increment instead of over/under-scrolling.
		e.wheelRemainder += int(signedHighWord(event.Buttons))
		steps := e.wheelRemainder / 120
		e.wheelRemainder %= 120
		if steps == 0 {
			return nil
		}
		button := 64
		if steps < 0 {
			button = 65
			steps = -steps
		}
		seq := sgrMouseSequence(button|mods, col, row, false)
		return appendRepeated(nil, seq, uint16(steps))
	}
	// Horizontal wheel is not part of the current public parser contract.
	if event.EventFlags&consoleMouseHWheeled != 0 {
		return nil
	}
	if event.EventFlags&consoleMouseMoved != 0 {
		button := firstConsoleMouseButton(event.Buttons)
		return []byte(sgrMouseSequence(button|32|mods, col, row, false))
	}

	previous, current := e.mouseButtons, event.Buttons
	e.mouseButtons = current
	var out strings.Builder
	for _, candidate := range []struct {
		mask uint32
		code int
	}{{0x0001, 0}, {0x0004, 1}, {0x0002, 2}} {
		wasDown := previous&candidate.mask != 0
		isDown := current&candidate.mask != 0
		if wasDown == isDown {
			continue
		}
		out.WriteString(sgrMouseSequence(candidate.code|mods, col, row, !isDown))
	}
	return []byte(out.String())
}
