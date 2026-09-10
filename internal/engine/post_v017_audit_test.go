package engine

import (
	"bytes"
	"strings"
	"testing"
)

func TestSelectionExtendWordLineAndShiftAnchor(t *testing.T) {
	screen, _ := RenderToScreen(Root(Text("one two\nthree four")), 10)
	var sel Selection
	if !sel.SelectWordAt(screen, 1, 0) {
		t.Fatal("word selection failed")
	}
	sel.Extend(screen, 5, 0)
	if got := sel.Text(screen); got != "one two" {
		t.Fatalf("same-row word extension = %q", got)
	}
	sel.Extend(screen, 1, 1)
	if got := sel.Text(screen); got != "one two\nthree" {
		t.Fatalf("cross-row word extension = %q", got)
	}

	if !sel.SelectLineAt(screen, 0) {
		t.Fatal("line selection failed")
	}
	sel.Extend(screen, 0, 1)
	if got := sel.Text(screen); got != "one two\nthree four" {
		t.Fatalf("line extension = %q", got)
	}

	sel.ShiftAnchor(-1, 0, screen.Height-1)
	if sel.VirtualAnchorRow == nil || *sel.VirtualAnchorRow != -1 || sel.Anchor.Y != 0 {
		t.Fatalf("shift anchor did not preserve virtual row: %+v", sel)
	}
	sel.ShiftAnchor(1, 0, screen.Height-1)
	if sel.VirtualAnchorRow != nil || sel.Anchor.Y != 0 {
		t.Fatalf("shift anchor did not restore visible row: %+v", sel)
	}
}

func TestScreenSelectableTextAndNoSelect(t *testing.T) {
	s := NewScreen(6, 2)
	s.SetCell(0, 0, "a", 1, TextStyle{}, "")
	s.SetCell(1, 0, "界", 2, TextStyle{}, "")
	s.SetCell(3, 0, "b", 1, TextStyle{}, "")
	s.SetCell(0, 1, "x", 1, TextStyle{}, "")
	s.SetCell(1, 1, "y", 1, TextStyle{}, "")
	s.MarkNoSelect(Rect{X: 3, Y: 0, Width: 1, Height: 1})
	if got := s.SelectableText(Rect{X: 0, Y: 0, Width: 4, Height: 2}); got != "a界\nxy" {
		t.Fatalf("selectable text = %q", got)
	}
}

func TestRuntimeSelectionTextCopyAndRestart(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "audit") // force deterministic OSC52; no native clipboard goroutine
	root := Root(AlternateScreen(Text("hello")))
	var out bytes.Buffer
	rt := NewRuntime(root, bytes.NewReader(nil), &out, RenderOptions{Width: 8, Height: 2, Fullscreen: true})
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	frame, err := rt.RenderSettled()
	if err != nil {
		t.Fatal(err)
	}
	rt.Selection.Anchor = Point{X: 0, Y: 0}
	rt.Selection.Focus = Point{X: 4, Y: 0}
	rt.Selection.FocusSet = true
	if got := rt.SelectionText(); got != "hello" {
		t.Fatalf("selection text = %q", got)
	}
	if frame.Screen == nil {
		t.Fatal("missing screen")
	}
	text, path, err := rt.CopySelection(true)
	if err != nil || text != "hello" || path != ClipboardOSC52Path || rt.Selection.HasSelection() {
		t.Fatalf("copy text=%q path=%q err=%v selection=%+v", text, path, err, rt.Selection)
	}
	if !strings.Contains(out.String(), "]52;c;") {
		t.Fatalf("OSC52 not written: %q", out.String())
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	if !rt.Started() {
		t.Fatal("runtime did not restart after Close")
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEventImmediateStopState(t *testing.T) {
	var e Event
	if e.DefaultPrevented() || e.PropagationStopped() {
		t.Fatal("fresh event already stopped")
	}
	e.PreventDefault()
	e.StopImmediatePropagation()
	if !e.DefaultPrevented() || !e.PropagationStopped() {
		t.Fatal("event state not preserved")
	}
}
