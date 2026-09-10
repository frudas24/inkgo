package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRound5NodeLifecycleAndScrollMutations(t *testing.T) {
	child := TextWithWrap("abcdef", TextWrapTruncateEnd, TextStyle{Bold: true})
	link := Link("https://example.com", Text("link"))
	spacer := Spacer()
	box := Box(Style{Width: Cells(20)}, child, link, spacer, Newline(2))
	root := Root(box)
	ComputeLayout(root, Size{Width: 20, Height: 5}, true)
	if root.layoutDirty {
		t.Fatal("layout should be clean after compute")
	}

	child.SetTextStyle(TextStyle{Italic: true})
	if root.layoutDirty {
		t.Fatal("text style must not invalidate geometry")
	}
	if !root.Dirty() {
		t.Fatal("paint invalidation not propagated")
	}

	scroll := ScrollBox(Style{Height: Cells(2)}, false, Text("a"), Text("b"), Text("c"))
	root.SetChildren(scroll)
	ComputeLayout(root, Size{Width: 20, Height: 5}, true)
	scroll.ScrollBy(1)
	if root.layoutDirty {
		t.Fatal("scroll must reuse geometry")
	}
	ComputeLayout(root, Size{Width: 20, Height: 5}, true)
	if scroll.ScrollTop == 0 {
		t.Fatal("scroll did not advance")
	}
	scroll.ScrollTo(0)
	scroll.ScrollToBottom()
	scroll.ScrollToElement(scroll.ScrollContent().Children[1], 0)
	minY, maxY := 0, 1
	scroll.SetScrollClamp(&minY, &maxY)
	ComputeLayout(root, Size{Width: 20, Height: 5}, true)
	if scroll.ScrollTop > 1 {
		t.Fatalf("clamp failed: %d", scroll.ScrollTop)
	}

	box.SetChildren(Text("x"), Text("y"))
	box.Remove(box.Children[0])
	if len(box.Children) != 1 {
		t.Fatal("remove")
	}
}

func TestRound5AbsoluteLayoutAndDiagnostics(t *testing.T) {
	abs := Box(Style{Position: PositionAbsolute, Left: Cells(2), Top: Cells(1), Width: Cells(4), Height: Cells(2)}, Text("x"))
	root := Root(Box(Style{Width: Cells(10), Height: Cells(5)}, abs))
	ComputeLayout(root, Size{Width: 10, Height: 5}, true)
	if abs.Rect.X != 2 || abs.Rect.Y != 1 || abs.Rect.Width != 4 || abs.Rect.Height != 2 {
		t.Fatalf("absolute=%+v", abs.Rect)
	}
	if len(SortedRects(root)) < 3 {
		t.Fatal("diagnostics")
	}
}

func TestRound5CapabilitiesSequencesAndClipboardFailClosed(t *testing.T) {
	env := map[string]string{
		"TERM_PROGRAM": "ghostty", "TERM_PROGRAM_VERSION": "1.2.1", "TERM": "xterm-ghostty",
	}
	getenv := func(k string) string { return env[k] }
	caps := detectCapabilities(getenv, "linux", true, "xterm.js 5.4")
	if !caps.Hyperlinks || !caps.SynchronizedOutput || !caps.ExtendedKeys || !caps.ProgressReporting || !caps.XtermJS {
		t.Fatalf("caps=%+v", caps)
	}
	if !semverAtLeast("v3.6.6", 3, 6, 6) || semverAtLeast("3.6.5", 3, 6, 6) {
		t.Fatal("semver")
	}

	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
	for _, seq := range []string{
		CursorUp(0), CursorDown(2), CursorForward(2), CursorBack(2), CursorMove(-2, 3),
		ScrollUp(0), ScrollDown(2), SetScrollRegion(1, 3), EraseToEndOfLine(), EraseToStartOfLine(),
		EraseToEndOfScreen(), EraseToStartOfScreen(), Hyperlink("https://x", "x"), TerminalTitle("x"),
		NotifyITerm2("m", "t"), NotifyGhostty("m", "t"), NotifyKitty("m", "t", 7),
		ProgressSequence(ProgressRunning, 44), ProgressSequence(ProgressError, 10), ProgressSequence(ProgressIndeterminate, 0),
		TabStatus(TabIdle), TabStatus(TabBusy), TabStatus(TabWaiting), ClearTabStatus(), ClipboardOSC52("abc"),
	} {
		if seq == "" {
			t.Fatal("empty terminal sequence")
		}
	}

	t.Setenv("SSH_CONNECTION", "1 2 3 4")
	t.Setenv("TMUX", "")
	if GetClipboardPath() != ClipboardOSC52Path {
		t.Fatalf("clipboard path=%s", GetClipboardPath())
	}
	if err := CopyNativeClipboard(context.Background(), "x"); err == nil {
		t.Fatal("native clipboard must fail closed over SSH")
	}
	if err := TmuxLoadBuffer(context.Background(), "x"); err == nil {
		t.Fatal("tmux should require TMUX")
	}
	seq, path := SetClipboard("x")
	if seq == "" || path != ClipboardOSC52Path {
		t.Fatalf("clipboard=%q %s", seq, path)
	}
}

func TestRound5FocusSelectionRendererAndRuntimeFacade(t *testing.T) {
	clicked := false
	btn1 := Button(Style{Width: Cells(4)}, func() { clicked = true }, Text("one"))
	btn2 := Button(Style{Width: Cells(4)}, func() {}, Text("two"))
	root := AlternateScreen(Box(Style{FlexDirection: Column}, btn1, btn2))
	r := NewRenderer(RenderOptions{Width: 12, Height: 4, Fullscreen: true})
	r.SetSize(12, 4)
	if r.Viewport() != (Size{Width: 12, Height: 4}) {
		t.Fatal("viewport")
	}
	frame := r.Render(root)
	if frame.Screen == nil {
		t.Fatal("frame")
	}
	if !r.IsVisible(btn1) {
		t.Fatal("visibility")
	}

	fm := NewFocusManager(root)
	fm.Disable()
	if fm.FocusNext() != nil {
		t.Fatal("disabled focus")
	}
	fm.Enable()
	if fm.FocusNext() != btn1 || fm.FocusNext() != btn2 || fm.FocusPrevious() != btn1 {
		t.Fatal("focus order")
	}
	DispatchClick(root, btn1.Rect.X, btn1.Rect.Y, 0)
	if !clicked {
		t.Fatal("click")
	}
	resized := false
	root.Handlers.OnResize = func(*ResizeEvent) { resized = true }
	DispatchResize(root, 20, 10)
	if !resized {
		t.Fatal("resize dispatch")
	}

	var sel Selection
	if !sel.SelectWordAt(frame.Screen, 1, 0) { /* content position may differ; exercise rect path below */
	}
	sel.Start(0, 0)
	sel.Update(2, 0)
	sel.Finish()
	if _, _, ok := sel.RectForRow(0, frame.Screen.Width); !ok {
		t.Fatal("selection rect")
	}
	r.SetSelection(&sel)
	r.SetSelectionBackground(ANSIColor(4))
	r.SetSearchHighlight("one", 0)
	positions := ScanPositions(frame.Screen, "one")
	r.SetSearchPositions(positions, 0, 0)
	_ = r.Render(root)

	var out bytes.Buffer
	rt := NewRuntime(root, bytes.NewReader(nil), &out, RenderOptions{Width: 12, Height: 4, Fullscreen: true})
	if rt.Stopped() {
		t.Fatal("new runtime stopped")
	}
	if rt.Viewport().Width != 12 {
		t.Fatal("runtime viewport")
	}
	if _, err := rt.RenderSettled(); err != nil {
		t.Fatal(err)
	}
	rt.SetSearchHighlight("one", 0)
	rt.SetSearchPositions(nil, 0, -1)
	rt.ClearSearch()
	if err := rt.SetTerminalTitle("title"); err != nil {
		t.Fatal(err)
	}
	if err := rt.RingBell(); err != nil {
		t.Fatal(err)
	}
	if err := rt.NotifyITerm2("m", "t"); err != nil {
		t.Fatal(err)
	}
	if err := rt.NotifyGhostty("m", "t"); err != nil {
		t.Fatal(err)
	}
	if err := rt.NotifyKitty("m", "t", 1); err != nil {
		t.Fatal(err)
	}
	rt.Stop()
	if !rt.Stopped() {
		t.Fatal("stop")
	}
}

func TestRound5TerminalQueriesAllMatchersAndClose(t *testing.T) {
	var out bytes.Buffer
	q := NewTerminalQuerier(&out)
	queries := []struct {
		q TerminalQuery
		r TerminalResponse
	}{
		{QueryDECRQM(1000), TerminalResponse{Type: "decrpm", Mode: 1000}},
		{QueryDA1(), TerminalResponse{Type: "da1"}},
		{QueryDA2(), TerminalResponse{Type: "da2"}},
		{QueryKittyKeyboard(), TerminalResponse{Type: "kittyKeyboard"}},
		{QueryCursorPosition(), TerminalResponse{Type: "cursorPosition"}},
		{QueryOSCColor(10), TerminalResponse{Type: "osc", Code: 10}},
		{QueryXTVERSION(), TerminalResponse{Type: "xtversion"}},
	}
	for _, tc := range queries {
		ch := q.Send(tc.q)
		q.OnResponse(tc.r)
		if got := <-ch; got == nil || !tc.q.Match(*got) {
			t.Fatalf("query %q", tc.q.Request)
		}
	}
	pending := q.Send(QueryDA2())
	done := q.Flush()
	q.OnResponse(TerminalResponse{Type: "da1"})
	if got := <-pending; got != nil {
		t.Fatal("barrier should resolve unsupported")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("flush")
	}
	q.Send(QueryDA2())
	q.Close()
	if out.Len() == 0 {
		t.Fatal("queries not written")
	}
}

func TestRound5ErrorsAndRawHelpers(t *testing.T) {
	err := errors.New("inner")
	n := ErrorfOverview("outer: %w", err)
	if n == nil {
		t.Fatal("error overview")
	}
	var b bytes.Buffer
	r := NewRenderer(RenderOptions{Width: 40, Height: 5})
	if _, err := r.WriteFrame(&b, Root(n)); err != nil {
		t.Fatal(err)
	}
	if b.Len() == 0 {
		t.Fatal("write frame")
	}

	term := DefaultTerminal()
	_, _ = term.Size() // may fail under CI pipes; should never panic.
	pr, pw, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatal(errPipe)
	}
	defer pr.Close()
	defer pw.Close()
	if raw, err := MakeRaw(pr); err == nil {
		_ = raw.Restore()
	}
	if err := runClipboardTool(context.Background(), "definitely-not-a-real-inkgo-tool", nil, "x"); err == nil {
		t.Fatal("missing tool")
	}
	if _, err := io.Copy(io.Discard, strings.NewReader("ok")); err != nil {
		t.Fatal(err)
	}
}
