package engine

import (
	"strings"
	"testing"
)

func parseNativeBytes(t *testing.T, data []byte) []ParsedInput {
	t.Helper()
	p := NewInputParser()
	got := p.Feed(data)
	if p.Pending() {
		got = append(got, p.Flush()...)
	}
	return got
}

func TestConsoleInputEncoderKeyModifiersAndRepeats(t *testing.T) {
	var encoder consoleInputEncoder
	cases := []struct {
		name  string
		event consoleKeyEvent
		key   Key
	}{
		{"plain", consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: 'A', Unicode: 'a'}, Key{Name: "a", Text: "a"}},
		{"shift", consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: 'A', Unicode: 'A', Control: consoleShiftPressed}, Key{Name: "a", Text: "A", Shift: true}},
		{"ctrl-c", consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: 'C', Unicode: 3, Control: consoleLeftCtrlPressed}, Key{Name: "c", Ctrl: true}},
		{"alt", consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: 'X', Unicode: 'x', Control: consoleLeftAltPressed}, Key{Name: "x", Text: "x", Alt: true, Meta: true}},
		{"shift-tab", consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: consoleVKTab, Unicode: '\t', Control: consoleShiftPressed}, Key{Name: "tab", Shift: true}},
		{"ctrl-pageup", consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: consoleVKPrior, Control: consoleLeftCtrlPressed}, Key{Name: "pageup", Ctrl: true}},
		{"shift-f5", consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: consoleVKF1 + 4, Control: consoleShiftPressed}, Key{Name: "f5", Shift: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseNativeBytes(t, encoder.key(tc.event))
			if len(got) != 1 || got[0].Kind != InputKey {
				t.Fatalf("parsed = %#v", got)
			}
			key := got[0].Key
			if key.Name != tc.key.Name || key.Text != tc.key.Text || key.Ctrl != tc.key.Ctrl || key.Alt != tc.key.Alt || key.Meta != tc.key.Meta || key.Shift != tc.key.Shift {
				t.Fatalf("key = %#v, want %#v", key, tc.key)
			}
		})
	}

	data := encoder.key(consoleKeyEvent{Down: true, Repeat: 3, VirtualKey: 'Z', Unicode: 'z'})
	got := parseNativeBytes(t, data)
	if len(got) != 3 {
		t.Fatalf("repeat count parsed %d keys", len(got))
	}
}

func TestConsoleInputEncoderUnicodeSurrogatesAndAltGr(t *testing.T) {
	var encoder consoleInputEncoder
	if got := encoder.key(consoleKeyEvent{Down: true, Unicode: 0xd83d}); len(got) != 0 {
		t.Fatalf("high surrogate emitted early: %q", got)
	}
	got := parseNativeBytes(t, encoder.key(consoleKeyEvent{Down: true, Unicode: 0xde00}))
	if len(got) != 1 || got[0].Key.Text != "😀" {
		t.Fatalf("surrogate pair = %#v", got)
	}

	// AltGr is RightAlt+Ctrl in Windows' control state, but a printable
	// character produced by that chord is ordinary text input.
	got = parseNativeBytes(t, encoder.key(consoleKeyEvent{Down: true, Unicode: '@', Control: consoleRightAltPressed | consoleLeftCtrlPressed}))
	if len(got) != 1 || got[0].Key.Text != "@" || got[0].Key.Alt || got[0].Key.Ctrl {
		t.Fatalf("AltGr = %#v", got)
	}
}

func TestConsoleInputEncoderMalformedSurrogateDoesNotDropFollowingRune(t *testing.T) {
	var encoder consoleInputEncoder
	if got := encoder.key(consoleKeyEvent{Down: true, Unicode: 0xd83d}); len(got) != 0 {
		t.Fatalf("high surrogate emitted early: %q", got)
	}
	inputs := parseNativeBytes(t, encoder.key(consoleKeyEvent{Down: true, Unicode: 'x'}))
	if len(inputs) != 2 || inputs[0].Key.Text != "�" || inputs[1].Key.Text != "x" {
		t.Fatalf("malformed surrogate recovery = %#v", inputs)
	}
}

func TestConsoleInputEncoderMouseSGR(t *testing.T) {
	var encoder consoleInputEncoder
	press := encoder.mouse(consoleMouseEvent{X: 4, Y: 2, Buttons: 1})
	move := encoder.mouse(consoleMouseEvent{X: 6, Y: 3, Buttons: 1, EventFlags: consoleMouseMoved})
	release := encoder.mouse(consoleMouseEvent{X: 6, Y: 3, Buttons: 0})
	wheel := encoder.mouse(consoleMouseEvent{X: 6, Y: 3, Buttons: uint32(uint16(120)) << 16, EventFlags: consoleMouseWheeled})

	inputs := parseNativeBytes(t, append(append(append(press, move...), release...), wheel...))
	if len(inputs) != 4 {
		t.Fatalf("mouse inputs = %#v", inputs)
	}
	if inputs[0].Kind != InputMouse || inputs[0].Mouse.Action != "press" || inputs[0].Mouse.Col != 5 || inputs[0].Mouse.Row != 3 {
		t.Fatalf("press = %#v", inputs[0])
	}
	if inputs[1].Kind != InputMouse || inputs[1].Mouse.Button&0x20 == 0 {
		t.Fatalf("move = %#v", inputs[1])
	}
	if inputs[2].Kind != InputMouse || inputs[2].Mouse.Action != "release" {
		t.Fatalf("release = %#v", inputs[2])
	}
	if inputs[3].Kind != InputKey || inputs[3].Key.Name != "wheelup" {
		t.Fatalf("wheel = %#v", inputs[3])
	}
}

func TestConsoleInputEncoderFocusSequenceRemainsParserCompatible(t *testing.T) {
	inputs := parseNativeBytes(t, []byte(FocusIn+FocusOut))
	if len(inputs) != 2 || inputs[0].Response.Type != "focus-in" || inputs[1].Response.Type != "focus-out" {
		t.Fatalf("focus = %#v", inputs)
	}
}

func TestNativeSpecialKeySequencesAreComplete(t *testing.T) {
	var encoder consoleInputEncoder
	for _, event := range []consoleKeyEvent{
		{Down: true, VirtualKey: consoleVKUp, Control: consoleShiftPressed},
		{Down: true, VirtualKey: consoleVKDelete, Control: consoleLeftAltPressed},
		{Down: true, VirtualKey: consoleVKF12, Control: consoleLeftCtrlPressed},
	} {
		data := encoder.key(event)
		if len(data) == 0 || !strings.HasPrefix(string(data), "\x1b[") {
			t.Fatalf("special key encoded as %q", data)
		}
		p := NewInputParser()
		if got := p.Feed(data); len(got) != 1 || p.Pending() {
			t.Fatalf("incomplete special sequence %q => %#v pending=%v", data, got, p.Pending())
		}
	}
}

func TestConsoleInputEncoderCombinedControlModifiers(t *testing.T) {
	cases := []struct {
		name  string
		event consoleKeyEvent
		want  Key
	}{
		{
			name:  "ctrl-shift-letter",
			event: consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: 'C', Unicode: 3, Control: consoleLeftCtrlPressed | consoleShiftPressed},
			want:  Key{Name: "c", Text: "C", Ctrl: true, Shift: true},
		},
		{
			name:  "ctrl-alt-letter",
			event: consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: 'C', Unicode: 3, Control: consoleLeftCtrlPressed | consoleLeftAltPressed},
			want:  Key{Name: "c", Text: "c", Ctrl: true, Alt: true, Meta: true},
		},
		{
			name:  "shift-backspace",
			event: consoleKeyEvent{Down: true, Repeat: 1, VirtualKey: consoleVKBack, Unicode: '\b', Control: consoleShiftPressed},
			want:  Key{Name: "backspace", Shift: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var encoder consoleInputEncoder
			got := parseNativeBytes(t, encoder.key(tc.event))
			if len(got) != 1 || got[0].Kind != InputKey {
				t.Fatalf("parsed = %#v", got)
			}
			key := got[0].Key
			if key.Name != tc.want.Name || key.Text != tc.want.Text || key.Ctrl != tc.want.Ctrl || key.Alt != tc.want.Alt || key.Meta != tc.want.Meta || key.Shift != tc.want.Shift {
				t.Fatalf("key = %#v, want %#v", key, tc.want)
			}
		})
	}
}

func TestConsoleInputEncoderWheelDeltaAccumulation(t *testing.T) {
	wheel := func(delta int16) consoleMouseEvent {
		return consoleMouseEvent{X: 1, Y: 1, Buttons: uint32(uint16(delta)) << 16, EventFlags: consoleMouseWheeled}
	}

	var encoder consoleInputEncoder
	if got := encoder.mouse(wheel(60)); len(got) != 0 {
		t.Fatalf("half wheel delta emitted early: %q", got)
	}
	got := parseNativeBytes(t, encoder.mouse(wheel(60)))
	if len(got) != 1 || got[0].Key.Name != "wheelup" {
		t.Fatalf("two half deltas = %#v", got)
	}

	encoder = consoleInputEncoder{}
	got = parseNativeBytes(t, encoder.mouse(wheel(240)))
	if len(got) != 2 || got[0].Key.Name != "wheelup" || got[1].Key.Name != "wheelup" {
		t.Fatalf("+240 delta = %#v", got)
	}

	encoder = consoleInputEncoder{}
	got = parseNativeBytes(t, encoder.mouse(wheel(-240)))
	if len(got) != 2 || got[0].Key.Name != "wheeldown" || got[1].Key.Name != "wheeldown" {
		t.Fatalf("-240 delta = %#v", got)
	}

	encoder = consoleInputEncoder{}
	if got := encoder.mouse(wheel(60)); len(got) != 0 {
		t.Fatalf("positive half delta emitted early: %q", got)
	}
	if got := encoder.mouse(wheel(-60)); len(got) != 0 {
		t.Fatalf("opposite half delta should cancel: %q", got)
	}
}

func TestConsoleInputEncoderLetterModifierMatrix(t *testing.T) {
	for ch := 'A'; ch <= 'Z'; ch++ {
		for mask := 0; mask < 8; mask++ {
			shift := mask&1 != 0
			alt := mask&2 != 0
			ctrl := mask&4 != 0
			// Plain Ctrl+C0 aliases are intentionally terminal-compatible (for
			// example Ctrl+H is indistinguishable from Backspace on a byte stream).
			// Combined modifiers use CSI-u and must preserve their full identity.
			if ctrl && !shift && !alt {
				continue
			}
			var control uint32
			if shift {
				control |= consoleShiftPressed
			}
			if alt {
				control |= consoleLeftAltPressed
			}
			if ctrl {
				control |= consoleLeftCtrlPressed
			}
			unicode := uint16(ch + ('a' - 'A'))
			if shift {
				unicode = uint16(ch)
			}
			if ctrl {
				unicode = uint16(ch - 'A' + 1)
			}
			var encoder consoleInputEncoder
			got := parseNativeBytes(t, encoder.key(consoleKeyEvent{
				Down: true, Repeat: 1, VirtualKey: uint16(ch), Unicode: unicode, Control: control,
			}))
			if len(got) != 1 || got[0].Kind != InputKey {
				t.Fatalf("%c mask=%d parsed=%#v", ch, mask, got)
			}
			key := got[0].Key
			wantName := string(ch + ('a' - 'A'))
			if key.Name != wantName || key.Shift != shift || key.Alt != alt || key.Ctrl != ctrl {
				t.Fatalf("%c mask=%d key=%#v want name=%q shift=%v alt=%v ctrl=%v", ch, mask, key, wantName, shift, alt, ctrl)
			}
		}
	}
}
