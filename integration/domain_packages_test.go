package integration_test

import (
	"bytes"
	"testing"
	"time"

	tui "github.com/frudas24/inkgo"
	inputdomain "github.com/frudas24/inkgo/input"
	interactiondomain "github.com/frudas24/inkgo/interaction"
	layoutdomain "github.com/frudas24/inkgo/layout"
	renderdomain "github.com/frudas24/inkgo/render"
	schedulerdomain "github.com/frudas24/inkgo/scheduler"
	selectiondomain "github.com/frudas24/inkgo/selection"
	terminaldomain "github.com/frudas24/inkgo/terminal"
	textdomain "github.com/frudas24/inkgo/text"
	widgetdomain "github.com/frudas24/inkgo/widgets"
)

func TestDomainPackagesAreComposable(t *testing.T) {
	root := widgetdomain.Root(widgetdomain.Box(
		layoutdomain.Style{Width: layoutdomain.Cells(10)},
		widgetdomain.Text("domain api"),
	))
	r := renderdomain.New(renderdomain.RenderOptions{Width: 10, Height: 2})
	frame := r.Render(root)
	if frame.Screen.PlainText() != "domain api" {
		t.Fatalf("render = %q", frame.Screen.PlainText())
	}
	p := inputdomain.NewParser()
	if got := p.Feed([]byte("x")); len(got) != 1 {
		t.Fatalf("input = %#v", got)
	}
	var out bytes.Buffer
	rt := terminaldomain.NewRuntime(root, bytes.NewReader(nil), &out, tui.RenderOptions{Width: 10, Height: 2})
	if rt == nil || selectiondomain.Char != tui.SelectionChar {
		t.Fatal("domain aliases not composable")
	}
	if interactiondomain.HitTest(root, tui.Point{X: 1, Y: 0}) == nil {
		t.Fatal("interaction domain did not compose with widget tree")
	}
	if textdomain.StringWidth("界") != 2 {
		t.Fatal("text domain width mismatch")
	}
	clock := schedulerdomain.New(time.Millisecond)
	clock.Close()
}
