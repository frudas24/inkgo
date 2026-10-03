package engine

import (
	"bytes"
	"testing"
)

func TestCopySelectionWithoutOutputDoesNotReportSuccessOrClear(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "adversarial")
	root := Root(AlternateScreen(Text("selection")))
	rt := NewRuntime(root, bytes.NewReader(nil), nil, RenderOptions{Width: 16, Height: 2, Fullscreen: true})
	rt.Renderer.Render(root)
	rt.Selection.Anchor = Point{X: 0, Y: 0}
	rt.Selection.Focus = Point{X: 8, Y: 0}
	rt.Selection.FocusSet = true

	text, path, err := rt.CopySelection(true)
	if err == nil || !rt.Selection.HasSelection() {
		t.Fatalf(
			"FINDING: CopySelection reported text=%q path=%q err=%v and cleared selection=%v despite having no output writer",
			text, path, err, !rt.Selection.HasSelection(),
		)
	}
}
