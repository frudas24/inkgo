package engine

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func awaitRuntimeEvent(t *testing.T, rt *Runtime) {
	t.Helper()
	select {
	case <-rt.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("runtime event did not arrive")
	}
}

func TestRuntimeTimeoutCallbacksUseUIOwner(t *testing.T) {
	for _, paste := range []bool{false, true} {
		name := "escape"
		if paste {
			name = "paste"
		}
		t.Run(name, func(t *testing.T) {
			root := Root(Text("before"))
			rt := NewRuntime(root, strings.NewReader(""), io.Discard, RenderOptions{})
			defer rt.Close()
			rt.EscapeTimeout = time.Millisecond
			rt.PasteTimeout = time.Millisecond
			calls := 0
			callback := func() { calls++; root.Children[0].SetText("after") }
			root.Handlers.OnKeyDown = func(e *KeyboardEvent) {
				if e.Key.Name != "escape" {
					t.Errorf("key = %s", e.Key.Name)
				}
				callback()
			}
			rt.OnPaste = func(text string) {
				if text != "partial" {
					t.Errorf("paste = %q", text)
				}
				callback()
			}
			chunk := "\x1b"
			if paste {
				chunk = PasteStart + "partial"
			}
			rt.HandleInput([]byte(chunk))
			awaitRuntimeEvent(t, rt)
			if calls != 0 || root.Children[0].Text != "before" {
				t.Fatal("timer dispatched outside UI owner")
			}
			if err := rt.ProcessEvents(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || root.Children[0].Text != "after" {
				t.Fatal("UI owner did not dispatch timeout")
			}
			if err := rt.ProcessEvents(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal("timeout dispatched twice")
			}
		})
	}
}

func TestRuntimeIgnoresObsoleteTimeout(t *testing.T) {
	root := Root(Text("x"))
	var keys []string
	root.Handlers.OnKeyDown = func(e *KeyboardEvent) { keys = append(keys, e.Key.Name) }
	rt := NewRuntime(root, strings.NewReader(""), io.Discard, RenderOptions{})
	defer rt.Close()
	rt.EscapeTimeout = time.Millisecond
	rt.HandleInput([]byte("\x1b"))
	awaitRuntimeEvent(t, rt)
	// Complete the old sequence and start a new incomplete one before processing
	// the queued timeout. The old timeout must not flush the new sequence.
	rt.EscapeTimeout = time.Hour
	rt.HandleInput([]byte("[A\x1b"))
	if err := rt.ProcessEvents(); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "up" || !rt.Parser.Pending() {
		t.Fatalf("keys=%v pending=%v", keys, rt.Parser.Pending())
	}
}

func TestRuntimeCloseDiscardsQueuedTimeout(t *testing.T) {
	rt := NewRuntime(Root(Text("x")), strings.NewReader(""), io.Discard, RenderOptions{})
	calls := 0
	rt.OnPaste = func(string) { calls++ }
	rt.PasteTimeout = time.Millisecond
	rt.HandleInput([]byte(PasteStart + "partial"))
	awaitRuntimeEvent(t, rt)
	rt.Close()
	if err := rt.ProcessEvents(); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("callback executed after Close")
	}
}

type regressionWriter struct {
	bytes.Buffer
	fail func(string) bool
	err  error
}

func (w *regressionWriter) Write(p []byte) (int, error) {
	if w.fail != nil && w.fail(string(p)) {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}
func (w *regressionWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func TestRuntimeStartupFailureRestoresTerminal(t *testing.T) {
	for _, run := range []bool{false, true} {
		for _, entry := range []bool{false, true} {
			name := "Start"
			if run {
				name = "Run"
			}
			if entry {
				name += "/entry"
			} else {
				name += "/frame"
			}
			t.Run(name, func(t *testing.T) {
				failure := errors.New("output failure")
				out := &regressionWriter{err: failure}
				out.fail = func(s string) bool {
					if entry {
						return strings.Contains(s, EnterAltScreen)
					}
					return strings.Contains(s, "hello")
				}
				rt := NewRuntime(Root(Text("hello")), strings.NewReader(""), out, RenderOptions{Width: 20, Height: 4, Fullscreen: true})
				restored := 0
				rt.outputRestore = func() error { restored++; return nil }
				var err error
				if run {
					err = rt.Run()
				} else {
					err = rt.Start()
				}
				if !errors.Is(err, failure) {
					t.Fatalf("error = %v", err)
				}
				if rt.Started() || restored != 1 || !strings.Contains(out.String(), ExitAltScreen) {
					t.Fatalf("incomplete cleanup: started=%v restored=%d output=%q", rt.Started(), restored, out.String())
				}
				rt.Close()
				if restored != 1 {
					t.Fatal("restored twice")
				}
				out.fail = nil
				out.Reset()
				if err := rt.Start(); err != nil {
					t.Fatal(err)
				}
				defer rt.Close()
				if !strings.Contains(out.String(), "hello") {
					t.Fatal("retry omitted initial frame")
				}
			})
		}
	}
}

type regressionBlockingReader struct {
	entered  chan struct{}
	release  chan struct{}
	returned chan struct{}
	closed   bool
}

func (r *regressionBlockingReader) Read([]byte) (int, error) {
	close(r.entered)
	<-r.release
	close(r.returned)
	return 0, io.EOF
}
func (r *regressionBlockingReader) Close() error { r.closed = true; return nil }

func TestRuntimeStopWakesBlockedRun(t *testing.T) {
	in := &regressionBlockingReader{entered: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{})}
	defer close(in.release)
	rt := NewRuntime(Root(Text("x")), in, io.Discard, RenderOptions{Width: 10, Height: 2})
	done := make(chan error, 1)
	go func() { done <- rt.Run() }()
	select {
	case <-in.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("Read not entered")
	}
	rt.Stop()
	rt.Stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not wake Run")
	}
	if rt.Started() {
		t.Fatal("Run did not restore terminal")
	}
	if in.closed {
		t.Fatal("Run closed caller-owned reader")
	}
}

func TestRuntimeRunDispatchesEscapeWhileReadBlocked(t *testing.T) {
	in, out := io.Pipe()
	defer in.Close()
	defer out.Close()
	root := Root(Text("x"))
	rt := NewRuntime(root, in, io.Discard, RenderOptions{Width: 10, Height: 2})
	rt.EscapeTimeout = time.Millisecond
	root.Handlers.OnKeyDown = func(e *KeyboardEvent) {
		if e.Key.Name == "escape" {
			rt.Stop()
		}
	}
	done := make(chan error, 1)
	go func() { done <- rt.Run() }()
	if _, err := out.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		rt.Stop()
		t.Fatal("Run did not dispatch timeout")
	}
}

func TestRuntimeHyperlinkCallbacksUseUIOwner(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	for _, cancel := range []bool{false, true} {
		name := "dispatch"
		if cancel {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			rt, _ := newRenderedRuntime(Root(AlternateScreen(Text("https://example.com"))), 30, 3)
			defer rt.Close()
			rt.MultiClickTimeout = time.Millisecond
			opened := 0
			rt.OnHyperlink = func(string) { opened++ }
			rt.HandleInput([]byte("\x1b[<0;2;1M\x1b[<0;2;1m"))
			awaitRuntimeEvent(t, rt)
			if opened != 0 {
				t.Fatal("hyperlink callback escaped UI owner")
			}
			if cancel {
				rt.HandleInput([]byte("\x1b[<0;2;1M"))
			}
			if err := rt.ProcessEvents(); err != nil {
				t.Fatal(err)
			}
			want := 1
			if cancel {
				want = 0
			}
			if opened != want {
				t.Fatalf("opened=%d want=%d", opened, want)
			}
		})
	}
}

func TestRuntimeProcessEventsReturnsRenderError(t *testing.T) {
	failure := errors.New("render failed")
	out := &regressionWriter{err: failure}
	node := Text("before")
	rt := NewRuntime(Root(node), strings.NewReader(""), out, RenderOptions{Width: 20, Height: 4})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	rt.OnPaste = func(string) { node.SetText("after") }
	out.fail = func(string) bool { return true }
	rt.PasteTimeout = time.Millisecond
	rt.HandleInput([]byte(PasteStart + "partial"))
	awaitRuntimeEvent(t, rt)
	if err := rt.ProcessEvents(); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
}

func TestParseANSIConsumesGenericEscapeSequencesAtomically(t *testing.T) {
	items := ParseANSI("\x1b(0abc\x1b(B", TextStyle{})
	var visible strings.Builder
	for _, item := range items {
		visible.WriteString(item.Value)
	}
	if got := visible.String(); got != "abc" {
		t.Fatalf("generic ESC sequence leaked bytes: %q", got)
	}

	items = ParseANSI("a\x1b^private\x1b\\b", TextStyle{})
	visible.Reset()
	for _, item := range items {
		visible.WriteString(item.Value)
	}
	if got := visible.String(); got != "ab" {
		t.Fatalf("PM control leaked bytes: %q", got)
	}
}
