package engine

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestCloseStartDoesNotCarryPartialEscape(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(Root(), bytes.NewReader(nil), &out, RenderOptions{Width: 8, Height: 2})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	if got := rt.HandleInput([]byte{'\x1b'}); len(got) != 0 {
		t.Fatalf("escape should remain pending, got %#v", got)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	got := rt.HandleInput([]byte("a"))
	if len(got) != 1 || got[0].Kind != InputKey || got[0].Key.Name != "a" || got[0].Key.Alt {
		t.Fatalf("partial input leaked across Close/Start: %#v", got)
	}
}

func TestStartAfterStopDoesNotEnterInertRuntime(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(Root(), bytes.NewReader(nil), &out, RenderOptions{Width: 8, Height: 2})
	rt.Stop()
	err := rt.Start()
	if err == nil || rt.Started() {
		t.Fatalf("Start after Stop produced inert runtime: err=%v started=%v stopped=%v", err, rt.Started(), rt.Stopped())
	}
}

func TestCloseStartResetsTransientPointerState(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(Root(Text("hello world")), bytes.NewReader(nil), &out, RenderOptions{Width: 20, Height: 2})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	rt.handleLeftPress(ParsedMouse{Button: 0, Action: "press"}, Point{X: 0, Y: 0})
	if !rt.Selection.Dragging || rt.clickCount != 1 || !rt.press.active {
		t.Fatalf("precondition failed: dragging=%v clicks=%d press=%+v", rt.Selection.Dragging, rt.clickCount, rt.press)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	if rt.Selection.Dragging || rt.Selecting || rt.press.active || rt.clickCount != 0 || !rt.lastClickTime.IsZero() {
		t.Fatalf("transient pointer state leaked across Close/Start: dragging=%v selecting=%v press=%+v clicks=%d lastClick=%v", rt.Selection.Dragging, rt.Selecting, rt.press, rt.clickCount, rt.lastClickTime)
	}
}

type auditToggleWriter struct {
	buf  bytes.Buffer
	fail bool
}

func (w *auditToggleWriter) Write(p []byte) (int, error) {
	if w.fail {
		return 0, errors.New("audit write failure")
	}
	return w.buf.Write(p)
}

func TestCloseReportsTerminalRestoreWriteFailure(t *testing.T) {
	w := &auditToggleWriter{}
	rt := NewRuntime(Root(AlternateScreen(Text("x"))), bytes.NewReader(nil), w, RenderOptions{Width: 8, Height: 2, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	w.fail = true
	if err := rt.Close(); err == nil {
		t.Fatal("Close silently reported success after terminal exit write failed")
	}
	if rt.Started() {
		t.Fatal("Close left runtime entered after write failure")
	}
}

type auditEOFReader struct {
	beforeEOF func()
}

func (r auditEOFReader) Read([]byte) (int, error) {
	if r.beforeEOF != nil {
		r.beforeEOF()
	}
	return 0, io.EOF
}

func TestRunReportsTerminalRestoreWriteFailure(t *testing.T) {
	w := &auditToggleWriter{}
	reader := auditEOFReader{beforeEOF: func() { w.fail = true }}
	rt := NewRuntime(Root(AlternateScreen(Text("x"))), reader, w, RenderOptions{Width: 8, Height: 2, Fullscreen: true})
	if err := rt.Run(); err == nil {
		t.Fatal("Run silently reported success after deferred terminal restore failed")
	}
	if rt.Started() {
		t.Fatal("Run left runtime entered after deferred restore failure")
	}
}

func TestResumeTerminalAfterStopIsNoOp(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(Root(AlternateScreen(Text("x"))), bytes.NewReader(nil), &out, RenderOptions{Width: 8, Height: 2, Fullscreen: true})
	rt.Stop()
	before := out.String()
	rt.ResumeTerminal()
	if rt.Started() {
		t.Fatal("ResumeTerminal resurrected a permanently stopped runtime")
	}
	if out.String() != before {
		t.Fatal("ResumeTerminal emitted terminal control bytes after Stop")
	}
}

func TestStartReportsRawModeFailure(t *testing.T) {
	in, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	rt := NewRuntime(Root(Text("x")), in, out, RenderOptions{Width: 8, Height: 2})
	rt.Terminal = &Terminal{In: in, Out: out}
	if err := rt.Start(); err == nil {
		t.Fatal("Start reported success when terminal input could not enter raw mode")
	}
	if rt.Started() {
		t.Fatal("failed Start marked runtime as owning terminal modes")
	}
}

func TestCloseRetriesTransientTerminalExitFailure(t *testing.T) {
	w := &auditToggleWriter{}
	rt := NewRuntime(Root(AlternateScreen(Text("x"))), bytes.NewReader(nil), w, RenderOptions{Width: 8, Height: 2, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	w.fail = true
	if err := rt.Close(); err == nil {
		t.Fatal("first Close unexpectedly succeeded")
	}
	if rt.Started() {
		t.Fatal("failed Close left runtime actively started")
	}
	if rt.pendingExitSequence == "" {
		t.Fatal("failed exit write was not retained for retry")
	}
	w.fail = false
	before := w.buf.Len()
	if err := rt.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if rt.pendingExitSequence != "" {
		t.Fatal("successful retry left exit pending")
	}
	if !strings.Contains(w.buf.String()[before:], ExitAltScreen) {
		t.Fatal("retry Close did not emit terminal exit sequence")
	}
}

func TestCloseRetriesOutputRestoreFailure(t *testing.T) {
	rt := NewRuntime(Root(), bytes.NewReader(nil), io.Discard, RenderOptions{Width: 8, Height: 2})
	calls := 0
	rt.outputRestore = func() error {
		calls++
		if calls == 1 {
			return errors.New("temporary restore failure")
		}
		return nil
	}
	if err := rt.Close(); err == nil {
		t.Fatal("first Close unexpectedly succeeded")
	}
	if rt.outputRestore == nil {
		t.Fatal("failed output restore was discarded instead of retained for retry")
	}
	if err := rt.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if rt.outputRestore != nil || calls != 2 {
		t.Fatalf("restore retry state: fn=%v calls=%d", rt.outputRestore != nil, calls)
	}
}

func TestResumeTerminalRawModeFailureDoesNotPublishStarted(t *testing.T) {
	in, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	rt := NewRuntime(Root(Text("x")), in, out, RenderOptions{Width: 8, Height: 2})
	rt.Terminal = &Terminal{In: in, Out: out}
	rt.ResumeTerminal()
	if rt.Started() {
		t.Fatal("ResumeTerminal published terminal ownership after raw-mode failure")
	}
}

func TestSuspendResumeRetriesTransientExitFailure(t *testing.T) {
	w := &auditToggleWriter{}
	rt := NewRuntime(Root(AlternateScreen(Text("x"))), bytes.NewReader(nil), w, RenderOptions{Width: 8, Height: 2, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	w.fail = true
	rt.SuspendTerminal()
	if rt.Started() {
		t.Fatal("SuspendTerminal left runtime started after failed exit write")
	}
	if rt.pendingExitSequence == "" {
		t.Fatal("SuspendTerminal discarded failed exit transition")
	}

	// Resume while the writer is still broken must not pretend terminal modes
	// were re-entered on top of the incomplete suspension.
	rt.ResumeTerminal()
	if rt.Started() {
		t.Fatal("ResumeTerminal entered while previous cleanup was still failing")
	}

	w.fail = false
	before := w.buf.Len()
	rt.ResumeTerminal()
	if !rt.Started() {
		t.Fatal("ResumeTerminal did not recover after cleanup became writable")
	}
	written := w.buf.String()[before:]
	if !strings.Contains(written, ExitAltScreen) || !strings.Contains(written, EnterAltScreen) {
		t.Fatalf("resume did not finish old exit before re-entry: %q", written)
	}
	if strings.Index(written, ExitAltScreen) > strings.Index(written, EnterAltScreen) {
		t.Fatal("ResumeTerminal re-entered before completing pending exit")
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStartFinishesPendingCleanupBeforeReentry(t *testing.T) {
	w := &auditToggleWriter{}
	rt := NewRuntime(Root(AlternateScreen(Text("x"))), bytes.NewReader(nil), w, RenderOptions{Width: 8, Height: 2, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	w.fail = true
	if err := rt.Close(); err == nil {
		t.Fatal("first Close unexpectedly succeeded")
	}
	w.fail = false
	before := w.buf.Len()
	if err := rt.Start(); err != nil {
		t.Fatalf("Start after recoverable cleanup failure: %v", err)
	}
	if !rt.Started() {
		t.Fatal("Start did not re-enter after completing pending cleanup")
	}
	written := w.buf.String()[before:]
	exitAt := strings.Index(written, ExitAltScreen)
	enterAt := strings.LastIndex(written, EnterAltScreen)
	if exitAt < 0 || enterAt < 0 || exitAt > enterAt {
		t.Fatalf("cleanup/re-entry ordering wrong: %q", written)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
}
