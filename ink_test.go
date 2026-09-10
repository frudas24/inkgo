package inkgo

import (
	"strings"
	"testing"
)

func TestBasicRenderAndIncrementalDamage(t *testing.T) {
	text := Text("hello")
	root := Root(text)
	r := NewRenderer(RenderOptions{Width: 10, Height: 5})
	first := r.Render(root)
	if got := first.Screen.PlainText(); got != "hello" {
		t.Fatalf("plain text = %q", got)
	}
	if first.Damage.Changed == 0 || first.Patch == "" {
		t.Fatal("first frame should paint")
	}
	second := r.Render(root)
	if second.Damage.Changed != 0 || second.Patch != "" {
		t.Fatalf("steady frame damage=%d patch=%q", second.Damage.Changed, second.Patch)
	}
	text.SetText("hallo")
	third := r.Render(root)
	if third.Damage.Changed != 1 {
		t.Fatalf("expected one changed cell, got %d (%+v)", third.Damage.Changed, third.Damage.Rect)
	}
}

func TestANSIHyperlinkAndWideCell(t *testing.T) {
	n := RawANSI("\x1b[1;31mR\x1b[0m \x1b]8;;https://example.com\x07界\x1b]8;;\x07")
	s, _ := RenderToScreen(Root(n), 10)
	c, _ := s.CellAt(0, 0)
	if !c.Style.Bold || c.Style.Color.Kind == ColorUnset {
		t.Fatalf("SGR lost: %+v", c.Style)
	}
	wide, _ := s.CellAt(2, 0)
	if wide.Char != "界" || wide.Width != CellWide || wide.Hyperlink != "https://example.com" {
		t.Fatalf("wide/link cell = %+v", wide)
	}
	tail, _ := s.CellAt(3, 0)
	if tail.Width != CellSpacerTail {
		t.Fatalf("wide tail = %+v", tail)
	}
}

func TestInputParserFragmentedPasteMouseAndKitty(t *testing.T) {
	p := NewInputParser()
	if got := p.Feed([]byte("\x1b[20")); len(got) != 0 {
		t.Fatalf("partial paste start emitted: %#v", got)
	}
	got := p.Feed([]byte("0~hello\nworld\x1b[201~"))
	if len(got) != 1 || got[0].Kind != InputPaste || got[0].Paste != "hello\nworld" {
		t.Fatalf("paste = %#v", got)
	}
	got = p.Feed([]byte("\x1b[<0;4;3M"))
	if len(got) != 1 || got[0].Kind != InputMouse || got[0].Mouse.Col != 4 || got[0].Mouse.Row != 3 {
		t.Fatalf("mouse = %#v", got)
	}
	got = p.Feed([]byte("\x1b[13;6u")) // Ctrl+Shift+Enter
	if len(got) != 1 || got[0].Key.Name != "return" || !got[0].Key.Ctrl || !got[0].Key.Shift {
		t.Fatalf("kitty = %#v", got)
	}
}

func TestScrollBoxDrainsAndClips(t *testing.T) {
	scroll := ScrollBox(Style{Width: Cells(5), Height: Cells(2)}, false, Text("a\nb\nc\nd"))
	root := Root(scroll)
	r := NewRenderer(RenderOptions{Width: 5, Height: 4})
	f := r.Render(root)
	if got := f.Screen.PlainText(); got != "a\nb" {
		t.Fatalf("initial scroll = %q; height=%d scrollHeight=%d", got, scroll.Rect.Height, scroll.ScrollHeight)
	}
	if scroll.ScrollHeight < 4 {
		t.Fatalf("scroll height %d", scroll.ScrollHeight)
	}
	scroll.ScrollBy(2)
	f = r.Render(root)
	if scroll.ScrollTop != 2 {
		t.Fatalf("scrollTop=%d pending=%d", scroll.ScrollTop, scroll.PendingScrollDelta)
	}
	if got := f.Screen.PlainText(); got != "c\nd" {
		t.Fatalf("scrolled = %q", got)
	}
}

func TestSearchSelectionNoSelect(t *testing.T) {
	gutter := NoSelectFromLeft(Style{Width: Cells(2)}, Text("> "))
	body := Text("hello hello")
	root := Root(Box(Style{}, gutter, body))
	s, _ := RenderToScreen(root, 20)
	m := ScanPositions(s, "hello")
	if len(m) != 2 {
		t.Fatalf("matches = %#v, text=%q", m, s.PlainText())
	}
	ApplySearchHighlight(s, "hello", 1)
	c, _ := s.CellAt(m[1].Col, m[1].Row)
	if !c.Style.Inverse || !c.Style.Bold || !c.Style.Underline {
		t.Fatalf("current match style = %+v", c.Style)
	}
	selected := (Selection{Anchor: Point{0, 0}, Focus: Point{19, 0}}).Text(s)
	if strings.Contains(selected, ">") {
		t.Fatalf("noSelect leaked into selection: %q", selected)
	}
}

func TestFocusButtonAndHitTestThroughScroll(t *testing.T) {
	count := 0
	b1 := Button(Style{Width: Cells(4)}, func() { count++ }, Text("one"))
	b2 := Button(Style{Width: Cells(4)}, func() { count++ }, Text("two"))
	root := Root(b1, b2)
	ComputeLayout(root, Size{Width: 10, Height: 5}, false)
	fm := NewFocusManager(root)
	if fm.FocusNext() != b1 || fm.FocusNext() != b2 {
		t.Fatal("focus order")
	}
	DispatchKey(b2, Key{Name: "return"})
	if count != 1 {
		t.Fatalf("button action count=%d", count)
	}
}

func TestBorderAndBackground(t *testing.T) {
	box := Box(Style{Width: Cells(8), Height: Cells(3), BorderStyle: &BorderRound, PaddingLeft: I(1), BackgroundColor: ANSIColor(4), BorderText: &BorderText{Content: "X", Position: "top", Align: "center"}}, Text("ok"))
	s, _ := RenderToScreen(Root(box), 8)
	got := s.PlainText()
	if !strings.Contains(got, "X") || !strings.Contains(got, "ok") {
		t.Fatalf("rendered box = %q", got)
	}
	c, _ := s.CellAt(0, 0)
	if c.Char != "╭" {
		t.Fatalf("border corner=%q", c.Char)
	}
}

func TestTabsBackgroundAndSoftWrapSelection(t *testing.T) {
	bg := ANSIColor(2)
	text := Text("a\tb")
	box := Box(Style{Width: Cells(10), BackgroundColor: bg}, text)
	s, _ := RenderToScreen(Root(box), 10)
	if got := s.PlainText(); got != "a       b" {
		t.Fatalf("tab expansion = %q", got)
	}
	c, _ := s.CellAt(8, 0)
	if c.Style.BackgroundColor != bg {
		t.Fatalf("background not inherited: %+v", c.Style.BackgroundColor)
	}

	wrapped, _ := RenderToScreen(Root(Text("abcdefgh")), 4)
	if len(wrapped.SoftWrap) < 2 || !wrapped.SoftWrap[1] {
		t.Fatalf("soft wrap bitmap = %#v", wrapped.SoftWrap)
	}
	sel := Selection{Anchor: Point{0, 0}, Focus: Point{3, 1}}
	if got := sel.Text(wrapped); got != "abcdefgh" {
		t.Fatalf("soft-wrapped selection inserted newline: %q", got)
	}
}

func TestFullscreenHardwareScrollAndDeclaredCursor(t *testing.T) {
	scroll := ScrollBox(Style{Width: Percent(100), Height: Cells(2)}, false, Text("a\nb\nc\nd"))
	alt := AlternateScreen(scroll)
	root := Root(alt)
	r := NewRenderer(RenderOptions{Width: 5, Height: 4, Fullscreen: true})
	_ = r.Render(root)
	scroll.ScrollBy(1)
	f := r.Render(root)
	if !strings.Contains(f.Patch, "\x1b[1;2r") || !strings.Contains(f.Patch, "\x1b[1S") {
		t.Fatalf("expected DECSTBM/SU scroll patch, got %q", f.Patch)
	}

	inputBox := Box(Style{Width: Cells(5), Height: Cells(1)}, Text("abc"))
	root2 := Root(inputBox)
	r2 := NewRenderer(RenderOptions{Width: 5, Height: 3})
	r2.DeclareCursor(inputBox, 0, 2, true)
	cf := r2.Render(root2)
	if !cf.Cursor.Visible || cf.Cursor.X != 2 || cf.Cursor.Y != 0 {
		t.Fatalf("cursor=%+v", cf.Cursor)
	}
	if !strings.Contains(cf.Patch, ShowCursor) {
		t.Fatalf("declared cursor not shown: %q", cf.Patch)
	}
}

func TestButtonRenderPropState(t *testing.T) {
	var states []ButtonState
	btn := ButtonWithState(Style{}, func() {}, func(s ButtonState) []*Node {
		states = append(states, s)
		label := "idle"
		if s.Focused {
			label = "focused"
		}
		if s.Active {
			label = "active"
		}
		return []*Node{Text(label)}
	})
	root := Root(btn)
	fm := NewFocusManager(root)
	fm.Focus(btn)
	if !btn.ButtonState.Focused || btn.Children[0].Text != "focused" {
		t.Fatalf("focused render=%+v child=%q", btn.ButtonState, btn.Children[0].Text)
	}
	DispatchKey(btn, Key{Name: "return"})
	if !btn.ButtonState.Active || btn.Children[0].Text != "active" {
		t.Fatalf("active render=%+v child=%q", btn.ButtonState, btn.Children[0].Text)
	}
	if len(states) < 3 {
		t.Fatalf("render prop calls=%d", len(states))
	}
}
