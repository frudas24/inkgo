package main

import (
	"io"
	"strings"
	"testing"
	"time"

	ink "github.com/frudas24/inkgo"
)

func TestSmokeExitKeys(t *testing.T) {
	for name, input := range map[string]string{"q": "q", "ctrl-c": "\x03", "escape": "\x1b"} {
		t.Run(name, func(t *testing.T) {
			rt := newSmokeRuntime(strings.NewReader(""), io.Discard, ink.Size{Width: 80, Height: 24})
			defer rt.Close()
			rt.EscapeTimeout = time.Millisecond
			rt.HandleInput([]byte(input))
			if name == "escape" {
				select {
				case <-rt.Events():
				case <-time.After(time.Second):
					t.Fatal("Escape timeout did not arrive")
				}
				if err := rt.ProcessEvents(); err != nil {
					t.Fatal(err)
				}
			}
			if !rt.Stopped() {
				t.Fatal("exit key did not stop the demo")
			}
		})
	}
}

func TestSmokeTabFocusIsVisible(t *testing.T) {
	rt := newSmokeRuntime(strings.NewReader(""), io.Discard, ink.Size{Width: 80, Height: 24})
	defer rt.Close()
	screen := func() string {
		frame, err := rt.Render()
		if err != nil {
			t.Fatal(err)
		}
		return frame.Screen.PlainText()
	}
	unfocused := screen()
	rt.HandleInput([]byte("\t"))
	first := screen()
	if first == unfocused || !strings.Contains(first, "[ button A ]") {
		t.Fatalf("Tab did not visibly focus the first button:\n%s", first)
	}
	rt.HandleInput([]byte("\t"))
	second := screen()
	if second == first || !strings.Contains(second, "[ button B ]") {
		t.Fatalf("Tab did not visibly move focus to the second button:\n%s", second)
	}
	rt.HandleInput([]byte("\x1b[Z"))
	if back := screen(); back != first {
		t.Fatalf("Shift+Tab did not return focus to the first button:\n%s", back)
	}
}

func TestSmokePasteFeedback(t *testing.T) {
	rt := newSmokeRuntime(strings.NewReader(""), io.Discard, ink.Size{Width: 80, Height: 24})
	defer rt.Close()
	rt.HandleInput([]byte("\x1b[200~hello pasted text\x1b[201~"))
	frame, err := rt.Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(frame.Screen.PlainText(), `Paste (17 bytes): "hello pasted text"`) {
		t.Fatalf("paste missing from screen: %s", frame.Screen.PlainText())
	}
}
