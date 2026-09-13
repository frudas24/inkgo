package engine

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// A terminal with mouse reporting enabled has no emulator-native selection. InkGo must
// therefore make a completed drag useful on its own: preserve the visual selection and
// synchronize it to the clipboard. SSH pins the deterministic OSC-52 route in this test.
func TestCopySelectionOnReleasePreservesSelectionAndWritesClipboard(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "test")
	t.Setenv("TMUX", "")
	root := Root(AlternateScreen(Text("alpha beta")))
	var out bytes.Buffer
	rt := NewRuntime(root, strings.NewReader(""), &out, RenderOptions{Width: 40, Height: 8, Fullscreen: true})
	rt.CopySelectionOnRelease = true
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if _, err := rt.RenderSettled(); err != nil {
		t.Fatal(err)
	}

	rt.handleLeftPress(ParsedMouse{}, Point{X: 0, Y: 0})
	rt.press.dragged = true
	rt.Selection.Extend(rt.selectionScreen(), 4, 0)
	rt.handleLeftRelease(Point{X: 4, Y: 0})

	if got := rt.SelectionText(); got != "alpha" {
		t.Fatalf("selection=%q, want alpha", got)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("alpha"))
	if !strings.Contains(out.String(), encoded) {
		t.Fatalf("completed selection was not copied through OSC-52: %q", out.String())
	}
}

func TestNoSelectPaneIsExcludedFromCrossRowSelection(t *testing.T) {
	left := Box(Style{FlexDirection: Column, Width: Cells(8)}, Text("left-one"), Text("left-two"))
	right := NoSelectBox(Style{FlexDirection: Column, Width: Cells(8)}, Text("SIDE-A"), Text("SIDE-B"))
	root := Root(Box(Style{FlexDirection: Row}, left, right))
	r := NewRenderer(RenderOptions{Width: 20, Height: 4})
	frame := r.Render(root)
	sel := &Selection{Anchor: Point{X: 0, Y: 0}, Focus: Point{X: 15, Y: 1}, FocusSet: true}
	got := sel.Text(frame.Screen)
	if strings.Contains(got, "SIDE-A") || strings.Contains(got, "SIDE-B") {
		t.Fatalf("selection crossed semantic pane boundary: %q", got)
	}
	if !strings.Contains(got, "left-one") || !strings.Contains(got, "left-two") {
		t.Fatalf("selection lost transcript text: %q", got)
	}
}
