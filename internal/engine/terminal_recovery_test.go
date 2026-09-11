package engine

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestRuntimeReopenFrame(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(Root(Text("hello")), strings.NewReader(""), &out, RenderOptions{Width: 20, Height: 4, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	rt.Close()
	out.Reset()
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("reopen cleared screen without repaint: %q", out.String())
	}
}

func TestRuntimeCannotRestartAfterStop(t *testing.T) {
	rt := NewRuntime(Root(Text("hello")), strings.NewReader(""), io.Discard, RenderOptions{})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	rt.Stop()
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(); err == nil {
		t.Fatal("stopped runtime restarted")
	}
}

// eofWithData exercises readers that return the last bytes together with EOF.
type eofWithData struct{ io.Reader }

func (r eofWithData) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		err = io.EOF
	}
	return n, err
}

func TestRuntimeEOFFlush(t *testing.T) {
	for _, sameRead := range []bool{false, true} {
		for _, input := range []string{"\x1b", PasteStart + "partial"} {
			root := Root(Text("x"))
			var got []string
			root.Handlers.OnKeyDown = func(e *KeyboardEvent) { got = append(got, e.Key.Name) }
			var in io.Reader = strings.NewReader(input)
			if sameRead {
				in = eofWithData{in}
			}
			rt := NewRuntime(root, in, io.Discard, RenderOptions{})
			rt.OnPaste = func(text string) { got = append(got, text) }
			rt.EscapeTimeout = time.Hour
			rt.PasteTimeout = time.Hour
			if err := rt.Run(); err != nil {
				t.Fatal(err)
			}
			want := "escape"
			if input != "\x1b" {
				want = "partial"
			}
			if len(got) != 1 || got[0] != want || rt.Parser.Pending() {
				t.Fatalf("sameRead=%v input=%q got=%v pending=%v", sameRead, input, got, rt.Parser.Pending())
			}
		}
	}
}

func TestRuntimeResumeCallbackDeadlock(t *testing.T) {
	button := Button(Style{}, nil, Text("x"))
	rt := NewRuntime(Root(button), strings.NewReader(""), io.Discard, RenderOptions{})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	rt.SuspendTerminal()
	entered := make(chan struct{})
	button.ButtonState.Active = true
	button.ActiveUntil = time.Now().Add(-time.Second)
	button.ButtonRender = func(ButtonState) []*Node { close(entered); rt.Started(); return []*Node{Text("x")} }
	done := make(chan struct{})
	go func() { rt.ResumeTerminal(); close(done) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback not reached")
	}
	select {
	case <-done:
		rt.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("ResumeTerminal holds lifecycleMu while callback calls Started")
	}
}
func TestRuntimeRenderRetry(t *testing.T) {
	out := &regressionWriter{err: io.ErrClosedPipe}
	node := Text("before")
	rt := NewRuntime(Root(node), strings.NewReader(""), out, RenderOptions{Width: 20, Height: 4, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	node.SetText("after")
	out.fail = func(string) bool { return true }
	if _, err := rt.Render(); err == nil {
		t.Fatal("expected writer failure")
	}
	out.fail = nil
	out.Reset()
	if _, err := rt.Render(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "after") {
		t.Fatalf("retry does not resend failed frame: %q", out.String())
	}
}
