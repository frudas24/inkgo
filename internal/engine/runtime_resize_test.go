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
