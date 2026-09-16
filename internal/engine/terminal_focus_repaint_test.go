package engine

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFocusReturnRepaintsUnchangedFullscreenContent(t *testing.T) {
	var out bytes.Buffer
	root := Root(AlternateScreen(Text("activity\nprompt draft\nenter send | tab mode")))
	rt := NewRuntime(root, strings.NewReader(""), &out, RenderOptions{Width: 50, Height: 8, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	rt.HandleInput([]byte(FocusOut))
	// Updates while hidden must not make the runtime assume the physical screen
	// survived a lock, keyboard overlay, or terminal restoration.
	if _, err := rt.Render(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	rt.HandleInput([]byte(FocusIn))
	if _, err := rt.Render(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"activity", "prompt draft", "enter send"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("focus return did not repaint %q: %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), EnterAltScreen) {
		t.Fatal("focus recovery re-entered alternate screen")
	}
	out.Reset()
	if _, err := rt.Render(); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("recovery disabled incremental rendering: %q", out.String())
	}
}

func TestInputAfterIdleGapRepaintsWithoutFocusNotification(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(Root(AlternateScreen(Text("unchanged footer"))), strings.NewReader(""), &out, RenderOptions{Width: 50, Height: 8, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	rt.lastInputTime = time.Now().Add(-2 * rt.ReassertAfter)
	out.Reset()
	rt.HandleInput([]byte("x"))
	if _, err := rt.Render(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unchanged footer") {
		t.Fatalf("idle recovery missed footer: %q", out.String())
	}
	if strings.Contains(out.String(), EnterAltScreen) {
		t.Fatal("idle recovery re-entered alternate screen")
	}
}
