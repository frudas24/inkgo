package inkgo

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func newRenderedRuntime(root *Node, width, height int) (*Runtime, *bytes.Buffer) {
	out := &bytes.Buffer{}
	rt := NewRuntime(root, bytes.NewReader(nil), out, RenderOptions{Width: width, Height: height, Fullscreen: true})
	_, _ = rt.Render()
	out.Reset()
	return rt, out
}

func TestClickFiresOnReleaseAndDragDoesNotClick(t *testing.T) {
	count := 0
	btn := Button(Style{Width: Cells(8), Height: Cells(1)}, func() { count++ }, Text("button"))
	rt, _ := newRenderedRuntime(Root(AlternateScreen(btn)), 12, 4)

	rt.HandleInput([]byte("\x1b[<0;2;1M"))
	if count != 0 {
		t.Fatalf("click fired on press: %d", count)
	}
	rt.HandleInput([]byte("\x1b[<0;2;1m"))
	if count != 1 {
		t.Fatalf("click did not fire on release: %d", count)
	}
	if rt.Focus.Focused() != btn {
		t.Fatal("click did not focus button")
	}

	// Break the multi-click chain and drag over the same button.
	rt.lastClickTime = time.Time{}
	rt.HandleInput([]byte("\x1b[<0;2;1M"))
	rt.HandleInput([]byte("\x1b[<32;4;1M"))
	rt.HandleInput([]byte("\x1b[<0;4;1m"))
	if count != 1 {
		t.Fatalf("drag activated button: %d", count)
	}
}

func TestDoubleTripleClickSelectionAndKeyboardExtension(t *testing.T) {
	root := Root(AlternateScreen(Text("hello world")))
	rt, _ := newRenderedRuntime(root, 20, 3)

	// First click: no selection.
	rt.HandleInput([]byte("\x1b[<0;2;1M\x1b[<0;2;1m"))
	if rt.Selection.HasSelection() {
		t.Fatal("bare click became a selection")
	}
	// Second press within the multi-click window selects the word immediately.
	rt.HandleInput([]byte("\x1b[<0;2;1M"))
	if !rt.Selection.HasSelection() {
		t.Fatal("double click did not select word")
	}
	if got := rt.Selection.Text(rt.selectionScreen()); got != "hello" {
		t.Fatalf("double-click word = %q", got)
	}
	rt.HandleInput([]byte("\x1b[<0;2;1m"))

	// Shift+right leaves word mode and extends by one cell.
	rt.dispatchKey(Key{Name: "right", Shift: true})
	if rt.Selection.Focus.X != 5 || rt.Selection.Span != nil {
		t.Fatalf("keyboard extension focus=%+v span=%#v", rt.Selection.Focus, rt.Selection.Span)
	}

	// Third press selects the whole visible line.
	rt.HandleInput([]byte("\x1b[<0;2;1M"))
	if rt.Selection.Span == nil || rt.Selection.Span.Mode != SelectionLine {
		t.Fatalf("triple click mode = %#v", rt.Selection.Span)
	}
	if got := rt.Selection.Text(rt.selectionScreen()); got != "hello world" {
		t.Fatalf("triple-click line = %q", got)
	}
}

func TestSelectionOverlayAndScrollCapture(t *testing.T) {
	scroll := ScrollBox(Style{Width: Cells(5), Height: Cells(2)}, false, Text("a\nb\nc\nd"))
	root := Root(AlternateScreen(scroll))
	r := NewRenderer(RenderOptions{Width: 5, Height: 3, Fullscreen: true})
	sel := &Selection{}
	r.SetSelection(sel)
	first := r.Render(root)
	sel.Start(0, 0)
	sel.Update(0, 1)
	second := r.Render(root)
	c, _ := second.Screen.CellAt(0, 0)
	if !c.Style.Inverse {
		t.Fatalf("selection overlay missing: %+v", c.Style)
	}
	if second.Damage.Changed == 0 {
		t.Fatal("selection overlay did not produce damage")
	}

	sel.Finish()
	scroll.ScrollBy(1)
	_ = r.Render(root)
	if len(sel.ScrolledOffAbove) == 0 || sel.ScrolledOffAbove[0] != "a" {
		t.Fatalf("scroll capture = %#v", sel.ScrolledOffAbove)
	}
	_ = first
}

func TestX10MouseAndRichTerminalResponses(t *testing.T) {
	p := NewInputParser()
	x10 := []byte{0x1b, '[', 'M', 32, 37, 35} // left press, col=5,row=3
	got := p.Feed(x10)
	if len(got) != 1 || got[0].Kind != InputMouse || got[0].Mouse.Col != 5 || got[0].Mouse.Row != 3 {
		t.Fatalf("x10 mouse = %#v", got)
	}

	cases := []struct {
		seq      string
		typeName string
		check    func(TerminalResponse) bool
	}{
		{"\x1b[?2026;1$y", "decrpm", func(r TerminalResponse) bool { return r.Mode == 2026 && r.Status == 1 }},
		{"\x1b[?3u", "kittyKeyboard", func(r TerminalResponse) bool { return r.Flags == 3 }},
		{"\x1b[?12;34R", "cursorPosition", func(r TerminalResponse) bool { return r.Row == 12 && r.Col == 34 }},
		{"\x1b]11;rgb:0000/0000/0000\x07", "osc", func(r TerminalResponse) bool { return r.Code == 11 && strings.HasPrefix(r.Data, "rgb:") }},
		{"\x1bP>|xterm.js(5.5.0)\x1b\\", "xtversion", func(r TerminalResponse) bool { return strings.Contains(r.Name, "xterm.js") }},
	}
	for _, tc := range cases {
		items := p.Feed([]byte(tc.seq))
		if len(items) != 1 || items[0].Kind != InputResponse || items[0].Response.Type != tc.typeName || !tc.check(items[0].Response) {
			t.Fatalf("response %s = %#v", tc.typeName, items)
		}
	}
}

func TestTerminalQuerierSentinelBarrier(t *testing.T) {
	var out bytes.Buffer
	q := NewTerminalQuerier(&out)
	resp := q.Send(QueryDECRQM(2026))
	flushed := q.Flush()
	if !strings.Contains(out.String(), "?2026$p") || !strings.HasSuffix(out.String(), CSI("c")) {
		t.Fatalf("query output = %q", out.String())
	}
	q.OnResponse(TerminalResponse{Type: "decrpm", Mode: 2026, Status: 1})
	if r := <-resp; r == nil || r.Status != 1 {
		t.Fatalf("query response = %#v", r)
	}
	q.OnResponse(TerminalResponse{Type: "da1"})
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("flush sentinel did not complete")
	}

	unsupported := q.Send(QueryDECRQM(2027))
	flushed2 := q.Flush()
	q.OnResponse(TerminalResponse{Type: "da1"})
	if r := <-unsupported; r != nil {
		t.Fatalf("unsupported query unexpectedly resolved: %#v", r)
	}
	<-flushed2
}

func TestTabDefaultActionCanBePrevented(t *testing.T) {
	b1 := Button(Style{}, func() {}, Text("one"))
	b2 := Button(Style{}, func() {}, Text("two"))
	b1.SetHandlers(EventHandlers{OnKeyDown: func(e *KeyboardEvent) {
		if e.Key.Name == "tab" {
			e.PreventDefault()
		}
	}})
	rt, _ := newRenderedRuntime(Root(b1, b2), 20, 3)
	rt.Focus.Focus(b1)
	rt.HandleInput([]byte("\t"))
	if rt.Focus.Focused() != b1 {
		t.Fatal("preventDefault did not cancel tab focus navigation")
	}
}

func TestPlainTextURLLookupStripsSentencePunctuation(t *testing.T) {
	s, _ := RenderToScreen(Root(Text("see https://example.com/foo(bar). now")), 50)
	url, ok := FindPlainTextURLAt(s, 15, 0)
	if !ok || url != "https://example.com/foo(bar)" {
		t.Fatalf("url = %q ok=%v", url, ok)
	}
}

func TestRendererSearchIntegrationAndPositionedCurrent(t *testing.T) {
	root := Root(AlternateScreen(Text("alpha beta alpha")))
	r := NewRenderer(RenderOptions{Width: 24, Height: 3, Fullscreen: true})
	r.SetSearchHighlight("alpha", 1)
	f := r.Render(root)
	if len(f.SearchMatches) != 2 {
		t.Fatalf("search matches = %#v", f.SearchMatches)
	}
	first, _ := f.Screen.CellAt(f.SearchMatches[0].Col, 0)
	current, _ := f.Screen.CellAt(f.SearchMatches[1].Col, 0)
	if !first.Style.Inverse || first.Style.Bold {
		t.Fatalf("ordinary match style = %+v", first.Style)
	}
	if !current.Style.Inverse || !current.Style.Bold || !current.Style.Underline {
		t.Fatalf("current match style = %+v", current.Style)
	}

	// Virtualized/element-relative positions can own the current emphasis
	// independently from the visible-screen scan.
	r.SetSearchPositions([]MatchPosition{{Row: 0, Col: 6, Len: 4}}, 0, 0)
	f = r.Render(root)
	positioned, _ := f.Screen.CellAt(6, 0)
	if !positioned.Style.Bold || !positioned.Style.Underline {
		t.Fatalf("positioned current style = %+v", positioned.Style)
	}
}

func TestExtendedKeysAndReassertAreBalanced(t *testing.T) {
	root := Root(AlternateScreen(Text("x")))
	r := NewRenderer(RenderOptions{Width: 5, Height: 2, Fullscreen: true, ForceExtendedKeys: true})
	enter := r.EnterSequence(root)
	if !strings.Contains(enter, EnableKittyKeyboard) || !strings.Contains(enter, EnableModifyOtherKeys) {
		t.Fatalf("extended key enable missing: %q", enter)
	}
	reassert := r.ReassertSequence(root, false)
	want := DisableKittyKeyboard + EnableKittyKeyboard + EnableModifyOtherKeys
	if !strings.Contains(reassert, want) {
		t.Fatalf("reassert does not pop-before-push: %q", reassert)
	}
	if strings.Contains(reassert, EraseScreen) {
		t.Fatalf("non-destructive reassert erased screen: %q", reassert)
	}
	reenter := r.ReassertSequence(root, true)
	if !strings.Contains(reenter, EnterAltScreen+EraseScreen+CursorHome) || !strings.Contains(reenter, EnableMouseTracking) {
		t.Fatalf("alt-screen recovery incomplete: %q", reenter)
	}
	exit := r.ExitSequence(root)
	if !strings.Contains(exit, DisableModifyOtherKeys) || !strings.Contains(exit, DisableKittyKeyboard) {
		t.Fatalf("extended key disable missing: %q", exit)
	}
}

func TestRuntimeLongInputGapReassertsModes(t *testing.T) {
	root := Root(AlternateScreen(Text("x")))
	var out bytes.Buffer
	rt := NewRuntime(root, bytes.NewReader(nil), &out, RenderOptions{Width: 5, Height: 2, Fullscreen: true, ForceExtendedKeys: true})
	if err := rt.enterTerminal(); err != nil {
		t.Fatal(err)
	}
	defer rt.leaveTerminal()
	out.Reset()
	rt.ReassertAfter = time.Millisecond
	rt.lastInputTime = time.Now().Add(-time.Second)
	rt.HandleInput([]byte("x"))
	got := out.String()
	if !strings.Contains(got, DisableKittyKeyboard+EnableKittyKeyboard+EnableModifyOtherKeys) {
		t.Fatalf("input gap did not reassert modes: %q", got)
	}
	if strings.Contains(got, EraseScreen) {
		t.Fatalf("ordinary input gap destructively re-entered alt screen: %q", got)
	}
}

func TestViewportVisibilityAccountsForScroll(t *testing.T) {
	first := Text("a")
	second := Text("b")
	scroll := ScrollBox(Style{Width: Cells(4), Height: Cells(1)}, false, first, second)
	root := Root(AlternateScreen(scroll))
	r := NewRenderer(RenderOptions{Width: 4, Height: 2, Fullscreen: true})
	_ = r.Render(root)
	if !r.IsVisible(first) || r.IsVisible(second) {
		t.Fatalf("visibility before scroll first=%v second=%v rects=%+v/%+v", r.IsVisible(first), r.IsVisible(second), first.Rect, second.Rect)
	}
	scroll.ScrollBy(1)
	_ = r.Render(root)
	if r.IsVisible(first) || !r.IsVisible(second) {
		t.Fatalf("visibility after scroll first=%v second=%v", r.IsVisible(first), r.IsVisible(second))
	}
}

func TestSelectionScrollDebtRoundTripAndFollowClear(t *testing.T) {
	screen, _ := RenderToScreen(Root(Text("a\nb\nc")), 3)
	sel := &Selection{Anchor: Point{X: 0, Y: 0}, Focus: Point{X: 0, Y: 1}, FocusSet: true}
	sel.CaptureScrolledRows(screen, 0, 0, true)
	sel.Shift(-1, 0, 2, 3)
	if sel.VirtualAnchorRow == nil || *sel.VirtualAnchorRow != -1 || len(sel.ScrolledOffAbove) != 1 {
		t.Fatalf("establish debt: anchor=%v captured=%#v", sel.VirtualAnchorRow, sel.ScrolledOffAbove)
	}
	sel.Shift(1, 0, 2, 3)
	if sel.VirtualAnchorRow != nil || len(sel.ScrolledOffAbove) != 0 || sel.Anchor.Y != 0 || sel.Focus.Y != 1 {
		t.Fatalf("reverse scroll did not repay debt: sel=%+v", sel)
	}

	sel = &Selection{Anchor: Point{X: 0, Y: 0}, Focus: Point{X: 1, Y: 0}, FocusSet: true}
	if !sel.ShiftForFollow(-1, 0, 2) || sel.HasSelection() {
		t.Fatalf("follow-scroll should clear fully offscreen selection: %+v", sel)
	}
}

func TestIncompleteEscapeAndPasteFlush(t *testing.T) {
	seen := []string{}
	root := Root(Box(Style{}, Text("x")))
	root.SetHandlers(EventHandlers{OnKeyDown: func(e *KeyboardEvent) { seen = append(seen, e.Key.Name) }})
	rt := NewRuntime(root, bytes.NewReader(nil), &bytes.Buffer{}, RenderOptions{Width: 5, Height: 2})
	if got := rt.HandleInput([]byte("\x1b")); len(got) != 0 {
		t.Fatalf("lone ESC emitted before timeout: %#v", got)
	}
	got := rt.flushIncompleteInput()
	if len(got) != 1 || got[0].Key.Name != "escape" || len(seen) != 1 || seen[0] != "escape" {
		t.Fatalf("ESC flush = %#v seen=%#v", got, seen)
	}

	pastes := []string{}
	rt.OnPaste = func(v string) { pastes = append(pastes, v) }
	if got := rt.HandleInput([]byte(PasteStart + "partial")); len(got) != 0 || !rt.Parser.InPaste() {
		t.Fatalf("partial paste state = %#v inPaste=%v", got, rt.Parser.InPaste())
	}
	got = rt.flushIncompleteInput()
	if len(got) != 1 || got[0].Kind != InputPaste || len(pastes) != 1 || pastes[0] != "partial" {
		t.Fatalf("paste flush = %#v callbacks=%#v", got, pastes)
	}
}

func TestXtermJSIdentitySuppressesDuplicateLinkOpen(t *testing.T) {
	root := Root(AlternateScreen(Text("https://example.com")))
	rt, _ := newRenderedRuntime(root, 30, 3)
	rt.TerminalName = "xterm.js(5.5.0)"
	opened := 0
	rt.OnHyperlink = func(string) { opened++ }
	rt.HandleInput([]byte("\x1b[<0;2;1M\x1b[<0;2;1m"))
	time.Sleep(2 * time.Millisecond)
	if opened != 0 || rt.pendingLink != nil {
		t.Fatalf("xterm.js duplicate link path armed/opened: opened=%d timer=%v", opened, rt.pendingLink != nil)
	}
}

func TestTerminalIdentityProbeAndCache(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(Root(Text("x")), bytes.NewReader(nil), &out, RenderOptions{})
	rt.ProbeTerminalIdentity()
	if !strings.Contains(out.String(), QueryXTVERSION().Request) || !strings.HasSuffix(out.String(), CSI("c")) {
		t.Fatalf("probe output = %q", out.String())
	}
	rt.dispatch(ParsedInput{Kind: InputResponse, Response: TerminalResponse{Type: "xtversion", Name: "xterm.js(9.0)"}})
	if rt.TerminalName != "xterm.js(9.0)" || !rt.IsXtermJS() {
		t.Fatalf("identity = %q xterm=%v", rt.TerminalName, rt.IsXtermJS())
	}
}

func TestPasteDispatchCaptureAndBubble(t *testing.T) {
	order := []string{}
	child := Box(Style{}, Text("target"))
	root := Root(child)
	root.SetHandlers(EventHandlers{
		OnPasteCapture: func(e *PasteEvent) { order = append(order, "root-capture:"+e.Text) },
		OnPaste:        func(e *PasteEvent) { order = append(order, "root-bubble:"+e.Text) },
	})
	child.SetHandlers(EventHandlers{
		OnPasteCapture: func(e *PasteEvent) { order = append(order, "child-capture:"+e.Text) },
		OnPaste:        func(e *PasteEvent) { order = append(order, "child-bubble:"+e.Text) },
	})
	rt := NewRuntime(root, bytes.NewReader(nil), &bytes.Buffer{}, RenderOptions{})
	rt.Focus = NewFocusManager(root)
	child.TabIndex = -1
	rt.Focus.Focus(child)
	rt.dispatch(ParsedInput{Kind: InputPaste, Paste: "hello"})
	want := []string{"root-capture:hello", "child-capture:hello", "child-bubble:hello", "root-bubble:hello"}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Fatalf("paste order=%#v want=%#v", order, want)
	}
}

func TestRenderSettledDrainsWheelBurst(t *testing.T) {
	lines := make([]*Node, 0, 40)
	for i := 0; i < 40; i++ {
		lines = append(lines, Text(fmt.Sprintf("%02d", i)))
	}
	scroll := ScrollBox(Style{Width: Cells(4), Height: Cells(5)}, false, lines...)
	root := Root(AlternateScreen(scroll))
	var out bytes.Buffer
	rt := NewRuntime(root, bytes.NewReader(nil), &out, RenderOptions{Width: 4, Height: 6, Fullscreen: true})
	rt.ScrollFrameInterval = 0
	_, _ = rt.Render()
	scroll.ScrollBy(20)
	f, err := rt.RenderSettled()
	if err != nil {
		t.Fatal(err)
	}
	if f.ScrollDrainPending || scroll.PendingScrollDelta != 0 || scroll.ScrollTop != 20 {
		t.Fatalf("settled frame pending=%v delta=%d top=%d", f.ScrollDrainPending, scroll.PendingScrollDelta, scroll.ScrollTop)
	}
}

func TestAdaptiveScrollDrainMatchesXtermPolicy(t *testing.T) {
	n := ScrollBox(Style{Width: Cells(4), Height: Cells(10)}, false)
	n.ScrollAdaptive = true
	n.PendingScrollDelta = 5
	if got := drainScrollDelta(n, n.PendingScrollDelta, 10); got != 5 || n.PendingScrollDelta != 0 {
		t.Fatalf("small adaptive drain got=%d pending=%d", got, n.PendingScrollDelta)
	}
	n.PendingScrollDelta = 12
	if got := drainScrollDelta(n, n.PendingScrollDelta, 10); got != 3 || n.PendingScrollDelta != 9 {
		t.Fatalf("high adaptive drain got=%d pending=%d", got, n.PendingScrollDelta)
	}
}

func TestRuntimeSetRootClearAndFocusState(t *testing.T) {
	oldButton := Button(Style{}, nil, Text("old"))
	oldButton.AutoFocus = true
	var out bytes.Buffer
	rt := NewRuntime(Root(oldButton), bytes.NewReader(nil), &out, RenderOptions{Width: 20, Height: 4, Fullscreen: true})
	if rt.Focus.Focused() != oldButton {
		t.Fatal("initial autofocus failed")
	}

	newButton := Button(Style{}, nil, Text("new"))
	newButton.AutoFocus = true
	newRoot := Root(newButton)
	rt.SetRoot(newRoot)
	if rt.Root != newRoot || rt.Focus.Focused() != newButton {
		t.Fatalf("root swap focus=%p want=%p", rt.Focus.Focused(), newButton)
	}

	out.Reset()
	if err := rt.ClearTerminal(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), EraseScreen) {
		t.Fatalf("clear sequence=%q", out.String())
	}

	if !rt.TerminalFocused() || rt.TerminalFocusState() != TerminalFocusUnknown {
		t.Fatal("unknown focus should be treated as focused")
	}
	rt.dispatch(ParsedInput{Kind: InputResponse, Response: TerminalResponse{Type: "focus-out"}})
	if rt.TerminalFocused() || rt.TerminalFocusState() != TerminalBlurred {
		t.Fatal("focus-out state not retained")
	}
	rt.dispatch(ParsedInput{Kind: InputResponse, Response: TerminalResponse{Type: "focus-in"}})
	if !rt.TerminalFocused() || rt.TerminalFocusState() != TerminalFocused {
		t.Fatal("focus-in state not retained")
	}
}

func TestSharedClockKeepAliveAndPassiveSubscribers(t *testing.T) {
	clock := NewClock(2 * time.Millisecond)
	defer clock.Close()
	active := make(chan time.Duration, 4)
	passive := make(chan time.Duration, 4)
	unsubPassive := clock.Subscribe(func(now time.Duration) { passive <- now }, false)
	defer unsubPassive()

	select {
	case <-passive:
		t.Fatal("passive subscriber kept clock alive")
	case <-time.After(5 * time.Millisecond):
	}

	unsubActive := clock.Subscribe(func(now time.Duration) { active <- now }, true)
	defer unsubActive()
	var a, p time.Duration
	select {
	case a = <-active:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("active clock did not tick")
	}
	select {
	case p = <-passive:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("passive subscriber did not share active tick")
	}
	if a <= 0 || p <= 0 {
		t.Fatalf("tick times active=%v passive=%v", a, p)
	}
}

func TestErrorOverviewRendersErrorChain(t *testing.T) {
	err := fmt.Errorf("outer: %w", fmt.Errorf("inner"))
	root := Root(ErrorOverview(err))
	r := NewRenderer(RenderOptions{Width: 40, Height: 8})
	frame := r.Render(root)
	plain := frame.Screen.PlainText()
	if !strings.Contains(plain, "outer: inner") || !strings.Contains(plain, "Caused by:") || !strings.Contains(plain, "inner") {
		t.Fatalf("error overview=%q", plain)
	}
}

func TestKittyNotificationIncludesFocusAction(t *testing.T) {
	seq := NotifyKitty("body", "title", 42)
	if !strings.Contains(seq, "i=42:d=0:p=title") || !strings.Contains(seq, "i=42:p=body") || !strings.Contains(seq, "i=42:d=1:a=focus") {
		t.Fatalf("kitty notification=%q", seq)
	}
}
